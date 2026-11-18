package web_test

import (
	"scry/internal/ai"
	"scry/internal/srs"
	"strings"
	"testing"
)

func TestD9_NewestRightReasonLifetime(t *testing.T) {
	w := newWorld(t)
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: "First reason."}, {Verdict: srs.Right, Reason: "Second reason."}, {Verdict: srs.Missed, Reason: "Miss reason."}}
	b := w.browser()
	b.signUp("reasons@example.com")
	b.addSet("nato-alphabet")
	item, _ := b.current(b.mustGet("/study"))
	body := b.answer(item, "first")
	has(t, body, "First reason.")
	item, _ = b.current(body)
	body = b.answer(item, "second")
	hasNot(t, body, "First reason.")
	has(t, body, "Second reason.")
	if strings.Count(body, `class="answered-why"`) != 1 {
		t.Fatal("expected only newest why")
	}
	item, _ = b.current(body)
	body = b.answer(item, "third")
	hasNot(t, body, `class="answered-why"`)
	has(t, body, "Miss reason.")
	body = b.next()
	hasNot(t, body, `class="answered-why"`)
}
