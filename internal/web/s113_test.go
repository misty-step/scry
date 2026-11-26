package web_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/srs"
)

// [S113-1] Through Study: with the test chain forced down each path, a
// multi-sentence conceptual answer shows its verdict and reason and moves
// the stored schedule, and no response is a 503. Only when every grading
// path fails does Study say "Couldn't check that just now", keeping the
// answer; Enter then retries and a verdict is recorded. If the response
// was lost after the review was recorded, retrying opens that review with
// no second judgment.
func TestStory_S113_1_StudyThroughEveryChainPath(t *testing.T) {
	answer := "Supply and demand set the price. When demand rises and supply holds, the price rises.\nWhen supply rises, it falls."
	for _, path := range ai.FakeJudgePaths {
		if path == "all-fail" {
			continue
		}
		t.Run(path, func(t *testing.T) {
			w := newWorld(t)
			fake := &ai.Fake{Paths: []string{path}, Verdicts: []srs.Verdict{srs.Close}, Log: w.app.Log}
			w.app.Model = fake
			b := w.browser()
			b.signUp("chain@example.com")
			b.addSet("nato-alphabet")
			it, _ := b.current(b.mustGet("/study"))
			status, page := b.post("/study/answer", "item_id", strconv.FormatInt(it.ID, 10), "answer", answer)
			if status != http.StatusOK {
				t.Fatalf("status %d, want 200 on a fallback or escalation path", status)
			}
			has(t, page, `data-verdict="close"`, "Almost Correct", `class="why"`)
			hist, err := w.lib("chain@example.com").History(ctx, it.ID)
			if err != nil || len(hist) != 1 || hist[0].Answer != answer || hist[0].Reason != ai.CriterionSentence(srs.Close) {
				t.Fatalf("history %+v %v", hist, err)
			}
			if s, err := w.lib("chain@example.com").Item(ctx, it.ID); err != nil || s.Sched.Reps != 1 {
				t.Fatalf("schedule %+v %v", s.Sched, err)
			}
			// A lost response: Enter again opens the recorded review, with no
			// second judgment.
			calls := fake.Calls()[ai.FakeJudgeModel]
			status, page = b.post("/study/answer", "item_id", strconv.FormatInt(it.ID, 10), "answer", answer)
			if status != http.StatusOK || !strings.Contains(page, "data-review") || fake.Calls()[ai.FakeJudgeModel] != calls {
				t.Fatalf("retry after a lost response: %d, judged again %v", status, fake.Calls()[ai.FakeJudgeModel] != calls)
			}
			if hist, _ := w.lib("chain@example.com").History(ctx, it.ID); len(hist) != 1 {
				t.Fatalf("%d reviews after the retry", len(hist))
			}
		})
	}
	t.Run("all-fail", func(t *testing.T) {
		w := newWorld(t)
		w.app.Model = &ai.Fake{Paths: []string{"all-fail", "primary"}, Verdicts: []srs.Verdict{srs.Right}, Log: w.app.Log}
		b := w.browser()
		b.signUp("down@example.com")
		b.addSet("nato-alphabet")
		it, _ := b.current(b.mustGet("/study"))
		_, page := b.post("/study/answer", "item_id", strconv.FormatInt(it.ID, 10), "answer", answer)
		has(t, page, "Couldn't check that just now. Your answer is still here; press Enter to try again.", "Supply and demand set the price.")
		if hist, _ := w.lib("down@example.com").History(ctx, it.ID); len(hist) != 0 {
			t.Fatal("an unjudged answer was recorded")
		}
		has(t, b.answer(it, answer), `data-verdict="right"`)
	})
}
