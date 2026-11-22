package app_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/store"
)

// Slice 41, item 2: every submitted answer stops on its post-answer
// review. A right answer used to move straight on (and end the session
// after the last card); now nothing advances until the learner says so.
func TestSlice41_Review_EveryVerdictIsHeldUntilNext(t *testing.T) {
	for _, v := range srs.Verdicts {
		t.Run(string(v), func(t *testing.T) {
			w := newStudyWorld(t, filepath.Join(t.TempDir(), "held.db"))
			defer w.st.Close()
			ids := w.addItems(t, false, false)
			w.model.set(v)
			s, err := w.app.Study(w.ctx, w.user)
			if err != nil || s == nil || s.Current().Item.ID != ids[0] {
				t.Fatalf("Study: %+v %v", s, err)
			}
			r, err := w.app.Answer(w.ctx, w.user, ids[0], "an answer", "typed")
			if err != nil || r.Verdict != v {
				t.Fatalf("Answer: %+v %v", r, err)
			}
			held, _ := w.us.ActiveSession(w.ctx)
			if held == nil || held.Phase() != store.PhasePendingReview || held.PendingReviewID != r.ID {
				t.Fatalf("a %s answer was not held for review: %+v", v, held)
			}
			// Opening Study again shows the same review; it does not skip ahead.
			again, err := w.app.Study(w.ctx, w.user)
			if err != nil || again.ID != held.ID || again.PendingReviewID != r.ID {
				t.Fatalf("Study moved off the review: %+v %v", again, err)
			}
			// An answer to the next card while the review is up is stale.
			if _, err := w.app.Answer(w.ctx, w.user, ids[1], "early", "typed"); !errors.Is(err, app.ErrStale) {
				t.Fatalf("answering past a review: %v, want ErrStale", err)
			}
			if err := w.app.Next(w.ctx, w.user, false); err != nil {
				t.Fatal(err)
			}
			next, _ := w.us.ActiveSession(w.ctx)
			if next == nil || next.Phase() != store.PhaseAsking || next.Current().Item.ID != ids[1] {
				t.Fatalf("Next did not move to the second card: %+v", next)
			}
			// The last card is held too: the session ends on Next, not on the answer.
			last, err := w.app.Answer(w.ctx, w.user, ids[1], "an answer", "typed")
			if err != nil {
				t.Fatal(err)
			}
			if s, _ := w.us.ActiveSession(w.ctx); s == nil || s.PendingReviewID != last.ID {
				t.Fatal("the last answer ended the session before its review was read")
			}
			if err := w.app.Next(w.ctx, w.user, false); err != nil {
				t.Fatal(err)
			}
			if s, _ := w.us.ActiveSession(w.ctx); s != nil {
				t.Fatalf("the session is still open after the last Next: %+v", s)
			}
		})
	}
}

// Slice 41, item 2: "Edit card" persists the question and answer, checks
// ownership and emptiness, and changes nothing else.
func TestSlice41_EditCard_PersistsTextAndNothingElse(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "edit.db"))
	defer w.st.Close()
	ids := w.addItems(t, false, true)
	w.model.set(srs.Missed)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	r, err := w.app.Answer(w.ctx, w.user, ids[0], "a wrong answer", "typed")
	if err != nil {
		t.Fatal(err)
	}
	reviews, sched := w.reviews(t), w.schedule(t, ids[0])

	it, err := w.app.EditCard(w.ctx, w.user, ids[0], "  What is asked now?  ", " The better answer ")
	if err != nil || it.Prompt != "What is asked now?" || it.Answer != "The better answer" || it.Exact {
		t.Fatalf("EditCard = %+v, %v", it, err)
	}
	// Persisted: a fresh read from SQLite has the new text.
	if got, err := w.us.Item(w.ctx, ids[0]); err != nil || got.Prompt != "What is asked now?" || got.Answer != "The better answer" {
		t.Fatalf("stored item = %+v, %v", got, err)
	}
	// The grade on screen, its graded snapshot and the schedule stand.
	if got := w.reviews(t); len(got) != len(reviews) || got[r.ID] != reviews[r.ID] ||
		got[r.ID].GradedPrompt != "question 0" || got[r.ID].GradedExpected != "answer number 0" {
		t.Fatalf("the edit changed the recorded review: %+v", got[r.ID])
	}
	if w.schedule(t, ids[0]) != sched {
		t.Fatal("the edit moved the schedule")
	}
	if s, _ := w.us.ActiveSession(w.ctx); s == nil || s.PendingReviewID != r.ID {
		t.Fatal("the edit moved the session off its review")
	}
	// "Ask Why?" still explains what was graded, not the edited text.
	if ex, err := w.app.Explain(w.ctx, w.user, r.ID); err != nil || !strings.Contains(ex.Text, "e=answer number 0") {
		t.Fatalf("explanation after the edit: %+v %v", ex, err)
	}
	// An exact text stays exact: the edit form has no say over that.
	if ex, err := w.app.EditCard(w.ctx, w.user, ids[1], "line cue", "the line"); err != nil || !ex.Exact {
		t.Fatalf("editing an exact card: %+v %v", ex, err)
	}

	// Empty or over-long text is refused in words and changes nothing.
	for name, c := range map[string][2]string{
		"empty question": {"   ", "kept"},
		"empty answer":   {"kept", "\n\t "},
		"both empty":     {"", ""},
		"long question":  {strings.Repeat("q", app.MaxPromptLen+1), "kept"},
		"long answer":    {"kept", strings.Repeat("a", app.MaxAnswerLen+1)},
	} {
		var fe *app.FieldError
		if _, err := w.app.EditCard(w.ctx, w.user, ids[0], c[0], c[1]); !errors.As(err, &fe) || fe.Msg == "" {
			t.Errorf("%s: %v, want a FieldError with words", name, err)
		}
	}
	if got, _ := w.us.Item(w.ctx, ids[0]); got.Prompt != "What is asked now?" || got.Answer != "The better answer" {
		t.Fatalf("a refused edit changed the card: %+v", got)
	}

	// Another learner cannot edit it, and learns nothing about it.
	mallory, err := w.st.CreateUser(w.ctx, "mallory@example.com", "mallory", "UTC", w.now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.EditCard(w.ctx, mallory, ids[0], "mine now", "mine now"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another learner's edit: %v, want ErrNotFound", err)
	}
	if got, _ := w.us.Item(w.ctx, ids[0]); got.Prompt != "What is asked now?" {
		t.Fatal("another learner changed the card")
	}
	// A removed card cannot be edited either.
	if err := w.us.RemoveItem(w.ctx, ids[0], w.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.EditCard(w.ctx, w.user, ids[0], "back", "again"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("editing a removed card: %v, want ErrNotFound", err)
	}
}

// Slice 41, item 2: "Ask Why?" also explains a right answer, and never an
// exact text; asking changes neither the review nor the schedule.
func TestSlice41_AskWhy_ExplainsARightAnswerWithoutChangingIt(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "why.db"))
	defer w.st.Close()
	ids := w.addItems(t, false, true)
	w.model.set(srs.Right)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	r, err := w.app.Answer(w.ctx, w.user, ids[0], "answer number 0", "typed")
	if err != nil || r.Verdict != srs.Right {
		t.Fatalf("Answer: %+v %v", r, err)
	}
	reviews, sched := w.reviews(t), w.schedule(t, ids[0])
	ex, err := w.app.Explain(w.ctx, w.user, r.ID)
	if err != nil || ex.Fallback || ex.Text != explanationOf("question 0", "answer number 0", "answer number 0", srs.Right) {
		t.Fatalf("Explain(right) = %+v, %v", ex, err)
	}
	if w.reviews(t)[r.ID] != reviews[r.ID] || w.schedule(t, ids[0]) != sched {
		t.Fatal("asking why changed the review or the schedule")
	}
	if err := w.app.Next(w.ctx, w.user, false); err != nil {
		t.Fatal(err)
	}
	exact, err := w.app.Answer(w.ctx, w.user, ids[1], "answer number 1", "typed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.Explain(w.ctx, w.user, exact.ID); !errors.Is(err, app.ErrNotExplainable) {
		t.Fatalf("Explain(exact text) = %v, want ErrNotExplainable", err)
	}
}
