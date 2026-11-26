package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// [S111-1] The store side of the daily limit on new cards: StartStudy
// counts first presentations again inside its own transaction, so a plan
// read before another tab started a session can never push the day past
// its limit; a card already presented today is free; another learner's
// presentations never count; and an older database gains the table and the
// session scope column without losing anything.
func TestS111_1_Store_StartStudyNeverExceedsTheDailyLimit(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "s111.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	alice, _ := st.CreateUser(ctx, "alice@example.com", "alice", "UTC", now)
	bob, _ := st.CreateUser(ctx, "bob@example.com", "bob", "UTC", now)
	a, b := st.For(alice.ID), st.For(bob.ID)
	deck, _ := a.CreateIdea(ctx, 0, "Deck", "", "", now)
	var ids []int64
	for i := 0; i < 14; i++ {
		id, err := a.AddItem(ctx, NewItem{IdeaID: deck, Prompt: fmt.Sprintf("q%d", i), Answer: fmt.Sprintf("a%d", i)}, now)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	plan := func(items []int64) SessionPlan {
		p := SessionPlan{Items: items, New: map[int64]bool{}, Day: "2026-11-20", DailyNew: 10, Now: now, RequireOne: true}
		for _, id := range items {
			p.New[id] = true
		}
		return p
	}
	// Two tabs plan at once from the same empty count: 7 and 7. The second
	// replaces the first, whose waiting cards behind the one on screen
	// were never shown and are released: 1 (shown) + 7.
	if _, err := a.StartStudy(ctx, plan(ids[:7])); err != nil {
		t.Fatal(err)
	}
	s2, err := a.StartStudy(ctx, plan(ids[7:]))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := a.PresentedOn(ctx, "2026-11-20")
	if len(got) != 8 || got[ids[0]] == 0 || got[ids[1]] != 0 {
		t.Fatalf("presented = %v", got)
	}
	if err := a.EndSession(ctx, s2, now); err != nil {
		t.Fatal(err)
	}
	// Over a day with 8 presented, a plan of 7: the card already presented
	// is free, two more fit, the rest wait for tomorrow.
	s3, err := a.StartStudy(ctx, plan(ids[:7]))
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := a.ActiveSession(ctx)
	if sess.ID != s3 || len(sess.Entries) != 3 {
		t.Fatalf("session holds %d cards, want 3 (one already presented, two left in the day)", len(sess.Entries))
	}
	if got, _ := a.PresentedOn(ctx, "2026-11-20"); len(got) != 10 {
		t.Fatalf("%d first presentations, want 10", len(got))
	}
	if err := a.EndSession(ctx, s3, now); err != nil {
		t.Fatal(err)
	}
	// A plan with nothing placeable starts nothing.
	if _, err := a.StartStudy(ctx, plan(ids[3:7])); err != ErrNothingToStudy {
		t.Fatalf("over the limit = %v, want ErrNothingToStudy", err)
	}
	if open, _ := a.ActiveSession(ctx); open != nil {
		t.Fatal("a refused plan opened a session")
	}
	// The next day is a new count.
	next := plan(ids[3:7])
	next.Day = "2026-11-21"
	if _, err := a.StartStudy(ctx, next); err != nil {
		t.Fatal(err)
	}
	// Bob's day is his own.
	if got, _ := b.PresentedOn(ctx, "2026-11-20"); len(got) != 0 {
		t.Fatalf("bob sees %d of alice's presentations", len(got))
	}
	// The table is a learner's own data: exported and restored with them.
	found := false
	for _, tb := range UserTables() {
		found = found || tb == "first_presentations"
	}
	if !found {
		t.Fatal("first_presentations is not a learner table")
	}
}

// [S111-1] StudyCandidates: a deck is the top-level idea; a scope keeps
// only an idea and the ideas inside it; reviews come earliest due first
// and new cards oldest first; held, removed and not-yet-due cards are out.
func TestS111_1_Store_StudyCandidatesAndDecks(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "cand.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	u, _ := st.CreateUser(ctx, "c@example.com", "c", "UTC", now)
	us := st.For(u.ID)
	top, _ := us.CreateIdea(ctx, 0, "Top", "", "", now)
	sub, _ := us.CreateIdea(ctx, top, "Sub", "", "", now)
	other, _ := us.CreateIdea(ctx, 0, "Other", "", "", now)
	add := func(idea int64, i int) int64 {
		id, err := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: fmt.Sprintf("q%d", i), Answer: fmt.Sprintf("a%d", i)}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	n1, n2, n3 := add(sub, 1), add(top, 2), add(other, 3)
	r1 := add(sub, 4)
	st.DB().Exec(`UPDATE items SET reps = 1, stability = 2, last_review = ?, due_at = ? WHERE id = ?`, now.Add(-72*time.Hour).Unix(), now.Add(-time.Hour).Unix(), r1)
	later := add(other, 5)
	st.DB().Exec(`UPDATE items SET due_at = ? WHERE id = ?`, now.Add(48*time.Hour).Unix(), later)
	reviews, fresh, err := us.StudyCandidates(ctx, now.Add(time.Hour), 0, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ID != r1 || reviews[0].Deck != top {
		t.Fatalf("reviews = %+v", reviews)
	}
	if len(fresh) != 3 || fresh[0].ID != n1 || fresh[0].Deck != top || fresh[1].ID != n2 || fresh[2].ID != n3 || fresh[2].Deck != other {
		t.Fatalf("new = %+v", fresh)
	}
	_, scoped, _ := us.StudyCandidates(ctx, now.Add(time.Hour), sub, 200, 200)
	if len(scoped) != 1 || scoped[0].ID != n1 {
		t.Fatalf("scoped to Sub = %+v", scoped)
	}
}
