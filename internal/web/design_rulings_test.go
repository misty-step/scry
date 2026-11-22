package web_test

import (
	"scry/internal/ai"
	"scry/internal/srs"
	"strings"
	"testing"
)

// The one-line why belongs to the post-answer review: it is shown while
// the review of that answer is on screen, for right answers as for any
// other, and once the learner moves on the answered card is one receded
// line in the scrollback with its verdict tag and nothing else.
func TestD9_TheWhyLivesInTheReviewAndTheScrollbackStaysOneLine(t *testing.T) {
	w := newWorld(t)
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: "First reason."}, {Verdict: srs.Right, Reason: "Second reason."}, {Verdict: srs.Missed, Reason: "Miss reason."}}
	b := w.browser()
	b.signUp("reasons@example.com")
	b.addSet("nato-alphabet")
	item, _ := b.current(b.mustGet("/study"))
	body := b.answer(item, "first")
	has(t, body, "First reason.", "data-review", `data-verdict="right"`)
	body = b.next()
	hasNot(t, body, "First reason.", "data-review")
	if strings.Count(body, `<li class="answered">`) != 1 {
		t.Fatal("the answered card should be one line of scrollback")
	}
	item, _ = b.current(body)
	body = b.answer(item, "second")
	hasNot(t, body, "First reason.")
	has(t, body, "Second reason.")
	body = b.next()
	item, _ = b.current(body)
	body = b.answer(item, "third")
	hasNot(t, body, `class="answered-why"`, "Second reason.")
	has(t, body, "Miss reason.", `data-verdict="missed"`)
	body = b.next()
	hasNot(t, body, `class="answered-why"`, "Miss reason.")
}
