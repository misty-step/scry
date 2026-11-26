package app_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/store"
)

// [S113-1] at the app: one submission is one judgment against the
// allowance whatever path the grading chain takes, every path that ends in
// a verdict moves the FSRS schedule, and no non-exact answer is graded
// without a model.

// logReservations records every judgment hold the store takes, so a test
// can count them (a trigger in the test's own database file).
func logReservations(t *testing.T, w *studyWorld) {
	t.Helper()
	for _, q := range []string{
		`CREATE TABLE reservation_log(id TEXT, amount INTEGER)`,
		`CREATE TRIGGER reservation_logged AFTER INSERT ON reservations BEGIN INSERT INTO reservation_log VALUES (NEW.id, NEW.amount); END`,
	} {
		if _, err := w.st.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

// oneJudgment checks the submission took exactly one hold, of the whole
// chain's budget, settled once into one judge usage row at want micros.
func oneJudgment(t *testing.T, w *studyWorld, want int64) {
	t.Helper()
	if n := w.count(t, `SELECT COUNT(*) FROM reservation_log`); n != 1 {
		t.Fatalf("%d reservations, want one", n)
	}
	if n := w.count(t, `SELECT COUNT(*) FROM reservation_log WHERE amount = ?`, w.app.JudgeBudget()); n != 1 {
		t.Fatalf("the hold was not sized for the whole chain (%d micros)", w.app.JudgeBudget())
	}
	if n := w.count(t, `SELECT COUNT(*) FROM usage WHERE user_id = ? AND kind = 'judge'`, w.user.ID); n != 1 {
		t.Fatalf("%d judge usage rows, want one", n)
	}
	if reserved, err := w.us.ReservedMicros(w.ctx); err != nil || reserved != 0 {
		t.Fatalf("%d micros still held (%v)", reserved, err)
	}
	if sub, _ := w.us.Subscription(w.ctx); want >= 0 && sub.UsedMicros != want {
		t.Fatalf("charged %d micros, want %d", sub.UsedMicros, want)
	}
}

func TestStory_S113_1_EveryChainPathUpdatesTheScheduleAsOneJudgment(t *testing.T) {
	for _, path := range ai.FakeJudgePaths {
		t.Run(path, func(t *testing.T) {
			w := newStudyWorld(t, filepath.Join(t.TempDir(), "chain.db"))
			defer w.st.Close()
			fake := &ai.Fake{Paths: []string{path, "primary"}, Verdicts: []srs.Verdict{srs.Close}, Log: w.app.Log}
			w.app.Model = fake
			logReservations(t, w)
			items := w.addItems(t, false)
			if _, err := w.app.Study(w.ctx, w.user); err != nil {
				t.Fatal(err)
			}
			r, err := w.app.Answer(w.ctx, w.user, items[0], "A paragraph-long explanation.\nIt has two lines.", "typed")
			if path == "all-fail" {
				// Only when no model in the chain nor the escalation gives a
				// verdict: nothing is recorded, the schedule stays, and the
				// fixture's known (zero) cost is settled.
				if !errors.Is(err, app.ErrJudgeUnavailable) || len(w.reviews(t)) != 0 || w.schedule(t, items[0]).Reps != 0 {
					t.Fatalf("all-fail: %v", err)
				}
				oneJudgment(t, w, -1)
				// Enter retries; the next submission is judged and scheduled.
				if r, err = w.app.Answer(w.ctx, w.user, items[0], "A paragraph-long explanation.\nIt has two lines.", "typed"); err != nil {
					t.Fatal(err)
				}
				if r.Verdict != srs.Close || w.schedule(t, items[0]).Reps != 1 {
					t.Fatalf("retry %+v", r)
				}
				return
			}
			if err != nil || r.Verdict != srs.Close || r.Reason != ai.CriterionSentence(srs.Close) {
				t.Fatalf("%+v %v", r, err)
			}
			if s := w.schedule(t, items[0]); s.Reps != 1 || s.Due.IsZero() {
				t.Fatalf("schedule %+v", s)
			}
			if got := w.reviews(t)[r.ID]; got.Verdict != "close" || got.Answer != "A paragraph-long explanation.\nIt has two lines." {
				t.Fatalf("stored review %+v", got)
			}
			oneJudgment(t, w, -1)
		})
	}
}

// A submission that walks the whole chain and escalates, on the real Jev
// and OpenRouter adapters against local stand-ins: one hold sized for
// every model, settled once at the sum of every attempt (an attempt whose
// outcome is unknown at its worst case).
func TestStory_S113_1_AWalkThroughTheWholeChainIsChargedAsExactlyOneJudgment(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		json.NewDecoder(r.Body).Decode(&body)
		calls[body.Model]++
		switch {
		case r.URL.Path == "/api/v1/chat/completions":
			json.NewEncoder(rw).Encode(map[string]any{"model": body.Model, "choices": []map[string]any{{"message": map[string]any{"content": `{"verdict":"correct"}`}, "finish_reason": "stop"}},
				"usage": map[string]any{"prompt_tokens": 500, "completion_tokens": 10, "cost": 0.0004}})
		case body.Model == ai.DefaultOpenRouterJudgeModel:
			rw.WriteHeader(http.StatusServiceUnavailable) // no usage: outcome unknown
		default: // the first fallback answers "none of these": uncertain
			json.NewEncoder(rw).Encode(map[string]any{"model": body.Model, "answers": map[string]any{"judgment": map[string]any{"type": "choice", "choice": "none_of_these"}},
				"usage": map[string]any{"input_tokens": 100, "output_tokens": 3, "cost": 0.000005}})
		}
	}))
	defer srv.Close()
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "walk.db"))
	defer w.st.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	jev := func(m string) *ai.Jev {
		return &ai.Jev{Model: m, APIKey: "local", OpenRouter: true, BaseURL: srv.URL, Log: quiet}
	}
	llm, _ := ai.NewClaudeWithRouter("", "local", "", "")
	llm.BaseURL = srv.URL
	chain := &ai.Chain{Decisions: []ai.GradingStep{jev(""), jev("openai/gpt-6-luna-decisions"), jev("google/gemini-decision-1")},
		Escalation: &ai.LLMJudge{Client: llm, Model: ai.DefaultJudgeEscalationModel}, Log: quiet}
	w.app.Model = ai.Split{JudgeProvider: chain, DraftProvider: w.model}
	// The costliest path: every decisions model and the escalation, at
	// their rates: 1,946 + 1,946 + 1,946 + 14,336 micros.
	if got := w.app.JudgeBudget(); got != 20174 {
		t.Fatalf("judge budget %d, want 20,174 micros", got)
	}
	logReservations(t, w)
	items := w.addItems(t, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	r, err := w.app.Answer(w.ctx, w.user, items[0], "a long conceptual answer", "typed")
	if err != nil || r.Verdict != srs.Right || r.Judge != ai.DefaultJudgeEscalationModel {
		t.Fatalf("%+v %v", r, err)
	}
	if calls[ai.DefaultOpenRouterJudgeModel] != 1 || calls["openai/gpt-6-luna-decisions"] != 1 || calls["google/gemini-decision-1"] != 0 || calls[ai.DefaultJudgeEscalationModel] != 1 {
		t.Fatalf("calls %v: an uncertain verdict must go straight to the LLM", calls)
	}
	// The lost primary at its worst case (1,946), the fallback's reported
	// 5 micros, the escalation's reported 400: once.
	oneJudgment(t, w, 1946+5+400)
	if w.schedule(t, items[0]).Reps != 1 {
		t.Fatal("schedule did not move")
	}
}

// Models only: a non-exact answer identical to the correct answer still
// reaches a model; there is no string-equality shortcut anywhere.
func TestStory_S113_1_IdenticalNonExactAnswerStillReachesTheModel(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "same.db"))
	defer w.st.Close()
	items := w.addItems(t, false, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	r, err := w.app.Answer(w.ctx, w.user, items[0], "answer number 0", "typed")
	if err != nil || w.model.calls != 1 || r.Judge != "test-judge" {
		t.Fatalf("an identical answer was graded without the model: %+v %v (calls %d)", r, err, w.model.calls)
	}
	// The test adapter too: the fixture chain's primary is asked.
	fake := &ai.Fake{Log: w.app.Log}
	w.app.Model = fake
	if err := w.app.Next(w.ctx, w.user, false); err != nil {
		t.Fatal(err)
	}
	if r, err = w.app.Answer(w.ctx, w.user, items[1], "  answer   number 1 ", "typed"); err != nil || fake.Calls()[ai.FakeJudgeModel] != 1 || r.Judge != ai.FakeJudgeModel {
		t.Fatalf("%+v %v %v", r, err, fake.Calls())
	}
}

// If a review was recorded but the response was lost, retrying opens that
// review without another charge or schedule update.
func TestStory_S113_1_RetryAfterALostResponseOpensTheRecordedReviewWithoutASecondCharge(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "lost.db"))
	defer w.st.Close()
	logReservations(t, w)
	items := w.addItems(t, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	first, err := w.app.Answer(w.ctx, w.user, items[0], "my answer", "typed")
	if err != nil {
		t.Fatal(err)
	}
	sched := w.schedule(t, items[0])
	used, _ := w.us.Subscription(w.ctx)
	// The browser never saw the reply and presses Enter again.
	if _, err := w.app.Answer(w.ctx, w.user, items[0], "my answer", "typed"); !errors.Is(err, app.ErrStale) {
		t.Fatalf("retry: %v, want ErrStale (the review is already recorded)", err)
	}
	if w.model.calls != 1 || len(w.reviews(t)) != 1 || w.schedule(t, items[0]) != sched {
		t.Fatalf("the retry judged again (%d calls), recorded again or moved the schedule", w.model.calls)
	}
	oneJudgment(t, w, used.UsedMicros)
	s, err := w.us.ActiveSession(w.ctx)
	if err != nil || s == nil || s.Phase() != store.PhasePendingReview || s.PendingReviewID != first.ID {
		t.Fatalf("the retry does not open the recorded review: %+v %v", s, err)
	}
}

// S111-2: line breaks the learner types, pastes or dictates are kept in
// the stored review; an exact text's word diff still reads them as spaces.
func TestStory_S111_2_LineBreaksAreKeptInTheReview(t *testing.T) {
	for in, want := range map[string]string{
		"First line.\r\nSecond   line.\n\n\n\nThird\tline.  ": "First line.\nSecond line.\n\nThird line.",
		"\n\n  one paragraph  \n":                             "one paragraph",
		"a\rb":                                                "a\nb",
		"":                                                    "",
	} {
		if got := app.NormalizeAnswer(in); got != want {
			t.Errorf("NormalizeAnswer(%q) = %q, want %q", in, got, want)
		}
	}
	long := app.NormalizeAnswer(strings.Repeat("é", 1500))
	if len(long) > app.MaxAnswerLen || !utf8.ValidString(long) {
		t.Fatalf("cap: %d bytes, valid %v", len(long), utf8.ValidString(long))
	}
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "lines.db"))
	defer w.st.Close()
	items := w.addItems(t, false, true)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	r, err := w.app.Answer(w.ctx, w.user, items[0], "Line one.\r\nLine two.", "spoken")
	if err != nil || w.reviews(t)[r.ID].Answer != "Line one.\nLine two." {
		t.Fatalf("%+v %v", w.reviews(t)[r.ID], err)
	}
	if err := w.app.Next(w.ctx, w.user, false); err != nil {
		t.Fatal(err)
	}
	r, err = w.app.Answer(w.ctx, w.user, items[1], "answer\nnumber 1", "typed")
	if err != nil || r.Verdict != srs.Right || r.Judge != "exact" || w.reviews(t)[r.ID].Answer != "answer\nnumber 1" {
		t.Fatalf("exact text across a line break: %+v %v", r, err)
	}
}
