package ai

import (
	"strings"
	"testing"

	"scry/internal/srs"
)

// Slice 70: the learner reads plain whys; the grader's criteria stay
// internal (stored, and given to the explainer) and are never shown.
func TestWhy_PlainSentencesReplaceTheGraderRubric(t *testing.T) {
	for _, v := range srs.Verdicts {
		plain := PlainWhy(v)
		if plain == "" || len(plain) > 60 || strings.Contains(strings.ToLower(plain), "grader") {
			t.Errorf("%s: plain why %q", v, plain)
		}
		if got := ShownWhy(v, CriterionSentence(v), "an answer"); got != plain {
			t.Errorf("%s: shown %q, want %q", v, got, plain)
		}
		if got := HistoryWhy(v, CriterionSentence(v)); got != plain {
			t.Errorf("%s: history %q, want %q", v, got, plain)
		}
		if got := ShownWhy(v, "", "an answer"); got != plain {
			t.Errorf("%s: missing reason shown as %q", v, got)
		}
	}
	// No criterion sentence, the abstain one included, is ever shown as is.
	for _, o := range judgeOptions {
		if got := ShownWhy(srs.Missed, o.Reason, "x"); got == o.Reason {
			t.Errorf("criterion %q reached the screen", o.Reason)
		}
	}
	// An empty answer judged Incorrect is a skip, model-graded or exact.
	if got := ShownWhy(srs.Missed, CriterionSentence(srs.Missed), "  "); got != SkipWhy || SkipWhy != "Skipped — review the answer above." {
		t.Errorf("skip: %q", got)
	}
	if got := ShownWhy(srs.Missed, "Three words missing.", ""); got != SkipWhy {
		t.Errorf("exact skip: %q", got)
	}
	// Other reasons (an exact text's word count) are kept.
	if got := ShownWhy(srs.Close, "One word missing, one swapped.", "the line"); got != "One word missing, one swapped." {
		t.Errorf("exact diff: %q", got)
	}
}
