package store

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// S106-4: old suggestions never strand me in a dead queue. Agent
// suggestions still pending in the retired Inbox (draft 0) move into
// their learner's library when the store opens, with the same logic as a
// tap on Approve; draft items are left for their outline; the move is
// logged per learner and running it again changes nothing.
func TestS106_4_PendingInboxSuggestionsMigrateToLibrary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	alice, _ := st.CreateUser(ctx, "alice@example.com", "alice", "UTC", now)
	bob, _ := st.CreateUser(ctx, "bob@example.com", "bob", "UTC", now)
	a, b := st.For(alice.ID), st.For(bob.ID)
	add := func(us *UserStore, s Suggestion) int64 {
		t.Helper()
		id, err := us.AddSuggestion(ctx, s, now)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	add(a, Suggestion{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Faction", IdeaSummary: "Groups against the common good.",
		Prompt: "Who wrote Federalist No. 10?", Answer: "James Madison"})
	add(a, Suggestion{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Faction",
		Prompt: "Which Federalist paper is about faction?", Answer: "Federalist No. 10"})
	dismissed := add(a, Suggestion{Agent: "reading agent", IdeaTitle: "Faction", Prompt: "Dismissed earlier?", Answer: "Yes"})
	if err := a.Dismiss(ctx, dismissed, now); err != nil {
		t.Fatal(err)
	}
	add(b, Suggestion{Agent: "chat", IdeaTitle: "Bread", Prompt: "What is 75% hydration?", Answer: "75 g water per 100 g flour", Exact: true})
	// A draft item waits in its own outline, not in the Inbox.
	draft, _ := b.CreateDraft(ctx, "bread", "ready", now)
	add(b, Suggestion{DraftID: draft, ParentTitle: "Bread", IdeaTitle: "Crumb", Prompt: "What makes an open crumb?", Answer: "High hydration"})
	st.Close()

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b = st.For(alice.ID), st.For(bob.ID)
	check := func() {
		t.Helper()
		if left, _ := a.Suggestions(ctx, 0); len(left) != 0 {
			t.Errorf("alice still has %d inbox suggestions pending", len(left))
		}
		items, _ := a.Search(ctx, "Federalist")
		if len(items) != 2 || items[0].IdeaTitle != "Faction" || items[0].Source != "from reading agent · The Federalist Papers" {
			t.Errorf("alice's moved items = %+v", items)
		}
		if gone, _ := a.Search(ctx, "Dismissed earlier"); len(gone) != 0 {
			t.Errorf("a dismissed suggestion was moved: %+v", gone)
		}
		bread, _ := b.Search(ctx, "hydration")
		if len(bread) != 1 || !bread[0].Exact || bread[0].IdeaTitle != "Bread" {
			t.Errorf("bob's moved item = %+v", bread)
		}
		if pending, _ := b.Suggestions(ctx, draft); len(pending) != 1 {
			t.Errorf("the draft item should still wait in its outline: %+v", pending)
		}
	}
	check()
	if got := logs.String(); strings.Count(got, "moved pending inbox suggestions into the library") != 2 ||
		!strings.Contains(got, `"count":2`) || !strings.Contains(got, `"count":1`) {
		t.Errorf("the move is not logged per learner: %s", got)
	}
	st.Close()

	// Idempotent: opening again moves nothing and duplicates nothing.
	logs.Reset()
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, b = st.For(alice.ID), st.For(bob.ID)
	check()
	if strings.Contains(logs.String(), "moved pending inbox suggestions") {
		t.Errorf("a second open moved something: %s", logs.String())
	}
}

// S106-4: an API token's cards go straight into the library in one
// transaction, recorded as approved (never pending anywhere).
func TestS106_4_AddFromAgentFilesStraightIntoTheLibrary(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	u, _ := st.CreateUser(ctx, "a@example.com", "a", "UTC", now)
	us := st.For(u.ID)
	ids, err := us.AddFromAgent(ctx, []Suggestion{
		{Agent: "script", Source: "Notes", IdeaTitle: "Faction", IdeaSummary: "Groups.", Prompt: "Q1", Answer: "A1"},
		{Agent: "script", Source: "Notes", IdeaTitle: "Faction", Prompt: "Q2", Answer: "A2", Exact: true},
	}, now)
	if err != nil || len(ids) != 2 {
		t.Fatalf("AddFromAgent = %v, %v", ids, err)
	}
	if left, _ := us.Suggestions(ctx, 0); len(left) != 0 {
		t.Errorf("%d left pending", len(left))
	}
	// All or none: one bad item adds nothing.
	if _, err := us.AddFromAgent(ctx, []Suggestion{{IdeaTitle: "X", Prompt: "Q3", Answer: "A3"}, {IdeaTitle: "X", Prompt: "Q4"}}, now); err == nil {
		t.Fatal("an item without an answer was accepted")
	}
	if n, _, _ := us.Counts(ctx, now); n != 2 {
		t.Errorf("%d items, want 2 (the refused request added none)", n)
	}
}
