package web_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/srs"
)

// PRODUCT.md [S130-1]: every graded answer stops on its review with a
// verdict in words (Correct, Almost, Incorrect); the middle verdict reads
// simply "Almost", never "Almost Correct". DESIGN.md: "Almost" everywhere
// the learner sees it (badge, verdict tag, review trail, Held and bridge
// lines); only the shown word changed, and the stored value is still
// "close".
func TestStory_S130_1_TheMiddleVerdictReadsAlmost(t *testing.T) {
	w := newWorld(t)
	w.app.Explainer = &ai.FakeExplainer{}
	b := w.browser()
	b.signUp("almost@example.com")
	b.addSet("nato-alphabet")

	body := b.mustGet("/study")
	item, ok := b.current(body)
	if !ok {
		t.Fatal("no card is waiting")
	}
	w.model.script = []ai.Judgment{{Verdict: srs.Close, Reason: ai.CriterionSentence(srs.Close)}}
	review := b.answer(item, "Alpha-ish")
	// The badge, with its tilde icon and the close color class.
	has(t, review, `data-verdict="close"`, `<span data-verdict-label>Almost</span>`, `icon-verdict-close`,
		`aria-label="Recent verdicts, oldest first: Almost"`, "Ask Why?")
	noAlmostCorrect(t, "the review", review)

	// The verdict tag in the scrollback once the next card waits.
	after := b.next()
	has(t, after, `<span class="tag tag-close">almost</span>`)
	noAlmostCorrect(t, "the next waiting card", after)

	// The card's own page lists the answer under "almost".
	page := b.mustGet("/library/items/" + strconv.FormatInt(item.ID, 10))
	has(t, page, `<span class="tag tag-close">almost</span>`)
	noAlmostCorrect(t, "the card's page", page)

	// Only the word changed: the stored verdict is still "close".
	hist, err := w.lib("almost@example.com").History(ctx, item.ID)
	if err != nil || len(hist) != 1 || hist[0].Verdict != srs.Close {
		t.Fatalf("history %+v %v", hist, err)
	}
	if srs.Close != "close" || srs.Close.Label() != "almost" {
		t.Errorf("close verdict: stored %q, shown %q", srs.Close, srs.Close.Label())
	}
}

// No learner-facing source (templates, static files, web handlers) still
// spells the middle verdict "Almost Correct".
func TestDesign_S130_1_NoAlmostCorrectAnywhereTheLearnerSees(t *testing.T) {
	for _, root := range []string{"templates", "static"} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasPrefix(p, "static/fonts/") {
				return nil
			}
			if ext := filepath.Ext(p); ext == ".png" || ext == ".ico" {
				return nil
			}
			b, _ := os.ReadFile(p)
			noAlmostCorrect(t, p, string(b))
			return nil
		})
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		noAlmostCorrect(t, f, string(b))
	}
}

func noAlmostCorrect(t *testing.T, where, s string) {
	t.Helper()
	if strings.Contains(strings.ToLower(s), "almost correct") {
		t.Errorf("%s still says \"Almost Correct\"", where)
	}
}
