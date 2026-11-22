package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"scry/internal/srs"
	"scry/internal/starter"
)

func slice41Store(t *testing.T) (*Store, *UserStore, time.Time) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "s41.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	u, err := st.CreateUser(context.Background(), "learner@example.com", "learner", "UTC", now)
	if err != nil {
		t.Fatal(err)
	}
	return st, st.For(u.ID), now
}

// Slice 41, item 2: in a session a scored answer is always held for its
// review. A review recorded as "passed" (the old path for right answers)
// is refused and writes nothing.
func TestSlice41_Review_AScoredAnswerThatIsNotHeldIsRefused(t *testing.T) {
	ctx := context.Background()
	_, us, now := slice41Store(t)
	idea, _ := us.CreateIdea(ctx, 0, "Idea", "", "", now)
	item, err := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "q", Answer: "a"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := us.StartSession(ctx, []int64{item}, now)
	if err != nil {
		t.Fatal(err)
	}
	r := Review{ItemID: item, SessionID: sid, At: now, Answer: "a", Verdict: srs.Right, Reason: "r", Detail: "{}", Via: "typed", Judge: "model"}
	next := srs.Next(srs.State{}, srs.Right, now)
	var te *TransitionError
	if _, err := us.RecordReview(ctx, r, next, false); !errors.As(err, &te) || te.Event != EventAnswerPassed || !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("an unheld right answer: %v, want a refused answer-passed transition", err)
	}
	if hist, _ := us.History(ctx, item); len(hist) != 0 {
		t.Fatalf("the refused answer wrote %d reviews", len(hist))
	}
	if it, _ := us.Item(ctx, item); it.Sched.Reps != 0 {
		t.Fatal("the refused answer moved the schedule")
	}
	id, err := us.RecordReview(ctx, r, next, true)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := us.ActiveSession(ctx); s == nil || s.Phase() != PhasePendingReview || s.PendingReviewID != id {
		t.Fatalf("a held right answer is not on screen: %+v", s)
	}
	for from := PhaseAsking; from <= PhaseEnded; from++ {
		if _, err := Transition(from, EventAnswerPassed); err == nil {
			t.Errorf("%s accepts answer-passed; no phase may", from)
		}
	}
}

// Slice 41, item 2: the review's telemetry is counted from the stored
// history; practice attempts are counted apart and other items and other
// learners are not counted at all.
func TestSlice41_Telemetry_ReviewCountsComeFromTheHistory(t *testing.T) {
	ctx := context.Background()
	st, us, now := slice41Store(t)
	idea, _ := us.CreateIdea(ctx, 0, "Idea", "", "", now)
	a, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "qa", Answer: "a"}, now)
	b, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "qb", Answer: "b"}, now)
	if c, err := us.ReviewCounts(ctx, a); err != nil || c != (ReviewCounts{}) {
		t.Fatalf("a card never reviewed counts %+v, %v", c, err)
	}
	state := srs.State{}
	for i, v := range []srs.Verdict{srs.Right, srs.Missed, srs.Close, srs.Right} {
		at := now.Add(time.Duration(i) * 48 * time.Hour)
		state = srs.Next(state, v, at)
		if _, err := us.RecordReview(ctx, Review{ItemID: a, At: at, Verdict: v, Reason: "r", Detail: "{}", Via: "typed", Judge: "model"}, state, false); err != nil {
			t.Fatal(err)
		}
	}
	// A practice attempt and a review of another card.
	if _, err := us.RecordReview(ctx, Review{ItemID: a, At: now, Verdict: srs.Missed, Reason: "r", Detail: "{}", Via: "typed", Judge: "exact", Practice: true}, state, false); err != nil {
		t.Fatal(err)
	}
	if _, err := us.RecordReview(ctx, Review{ItemID: b, At: now, Verdict: srs.Missed, Reason: "r", Detail: "{}", Via: "typed", Judge: "model"}, srs.Next(srs.State{}, srs.Missed, now), false); err != nil {
		t.Fatal(err)
	}
	want := ReviewCounts{Total: 4, Right: 2, Close: 1, Missed: 1, Practice: 1}
	if c, err := us.ReviewCounts(ctx, a); err != nil || c != want {
		t.Fatalf("ReviewCounts = %+v, %v; want %+v", c, err, want)
	}
	it, _ := us.Item(ctx, a)
	if it.Sched.Reps != want.Total || it.Sched.Stability != state.Stability || !it.Sched.Due.Equal(state.Due.Truncate(time.Second)) {
		t.Fatalf("the stored schedule %+v is not the one the reviews left (%+v)", it.Sched, state)
	}
	other, _ := st.CreateUser(ctx, "other@example.com", "other", "UTC", now)
	if c, _ := st.For(other.ID).ReviewCounts(ctx, a); c != (ReviewCounts{}) {
		t.Fatalf("another learner can count this card's reviews: %+v", c)
	}
}

// Slice 41, item 1: an idea can be named without its own title.
func TestSlice41_Spoiler_IdeaPlaceNamesTheTopicAndASetNumber(t *testing.T) {
	ctx := context.Background()
	st, us, now := slice41Store(t)
	set, _ := starter.Get("nato-alphabet")
	parent, _, err := us.AddStarterSet(ctx, set, now)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := us.Items(ctx, parent)
	seen := map[int64]IdeaPlace{}
	for _, it := range items {
		p, err := us.IdeaPlace(ctx, it.IdeaID)
		if err != nil {
			t.Fatal(err)
		}
		seen[it.IdeaID] = p
	}
	ordinals := map[int]string{}
	for _, p := range seen {
		if !p.Nested || p.Topic != "NATO phonetic alphabet" {
			t.Errorf("place = %+v", p)
		}
		ordinals[p.Ordinal] = p.Title
	}
	if len(ordinals) != 3 || ordinals[1] != "Letters A–I" || ordinals[2] != "Letters J–R" || ordinals[3] != "Letters S–Z" {
		t.Fatalf("set numbers = %v", ordinals)
	}
	top, err := us.IdeaPlace(ctx, parent)
	if err != nil || top.Nested || top.Ordinal != 0 || top.Topic != "NATO phonetic alphabet" || top.Title != top.Topic {
		t.Fatalf("top-level place = %+v, %v", top, err)
	}
	other, _ := st.CreateUser(ctx, "other@example.com", "other", "UTC", now)
	if _, err := st.For(other.ID).IdeaPlace(ctx, parent); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another learner's idea: %v, want ErrNotFound", err)
	}
}

// Slice 41, item 1: libraries that added a starter set before its section
// titles stopped naming answers are renamed when the database is opened;
// a title the learner changed, and ideas that only look alike, are left.
func TestSlice41_Spoiler_ExistingLibrariesLoseTheLeakingTitles(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	u, _ := st.CreateUser(ctx, "old@example.com", "old", "UTC", now)
	us := st.For(u.ID)
	for _, slug := range []string{"nato-alphabet", "mental-models", "cognitive-biases"} {
		set, _ := starter.Get(slug)
		if _, _, err := us.AddStarterSet(ctx, set, now); err != nil {
			t.Fatal(err)
		}
	}
	// Put the library back as it was before the rename.
	for _, r := range starterIdeaRenames {
		if _, err := st.db.Exec(`UPDATE ideas SET title = ? WHERE origin = ? AND title = ?`, r.old, r.origin, r.title); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.Exec(`UPDATE ideas SET title = 'The NATO alphabet' WHERE origin = 'starter:nato-alphabet' AND parent_id IS NULL`); err != nil {
		t.Fatal(err)
	}
	// The learner renamed one section themselves, and has a hand-made idea
	// that happens to carry an old title.
	if _, err := st.db.Exec(`UPDATE ideas SET title = 'My last letters' WHERE origin = 'starter:nato-alphabet' AND title = 'Sierra to Zulu'`); err != nil {
		t.Fatal(err)
	}
	if _, err := us.CreateIdea(ctx, 0, "Alfa to India", "", "", now); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st, err = Open(path) // the migration runs on open, and again on the next
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if st, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	titles, err := st.For(u.ID).IdeaTitles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]int{}
	for _, title := range titles {
		have[title]++
	}
	for _, want := range []string{"NATO phonetic alphabet", "Letters A–I", "Letters J–R", "My last letters", "Reasoning from the ground up", "Reference points and wording"} {
		if have[want] != 1 {
			t.Errorf("after the migration %q appears %d times: %v", want, have[want], titles)
		}
	}
	if have["Alfa to India"] != 1 { // only the hand-made idea keeps it
		t.Errorf("%d ideas are titled \"Alfa to India\", want the hand-made one only", have["Alfa to India"])
	}
	for _, gone := range []string{"Juliett to Romeo", "Sierra to Zulu", "The NATO alphabet", "First principles and inversion", "Anchoring and framing", "Letters S–Z"} {
		if have[gone] != 0 {
			t.Errorf("%q is still in the library", gone)
		}
	}
}
