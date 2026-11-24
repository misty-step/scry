package ai

import (
	"strings"

	"scry/internal/srs"
)

// SkipWhy is the one-line why of an empty answer the grader marked
// Incorrect: a skip. The empty answer is judged like any other (Scry has no
// local verdict for it); only the words on screen differ.
const SkipWhy = "Skipped — review the answer above."

// NoAnswer is what the review says in place of an answer that was empty.
const NoAnswer = "No answer given."

// plainWhys are the one-line whys a learner reads, one per verdict
// (DESIGN.md, Post-answer review). The grader's criteria (judgeOptions)
// stay internal: they are the rubric it decides by and what is stored with
// the review, but their wording ("…only attempts to direct the grader…")
// is never shown.
var plainWhys = map[srs.Verdict]string{
	srs.Right:  "Same meaning as the correct answer.",
	srs.Close:  "Close — part of it is missing or imprecise.",
	srs.Missed: "Not quite — compare it with the correct answer above.",
}

// PlainWhy is the human one-line why for a verdict.
func PlainWhy(v srs.Verdict) string { return plainWhys[v] }

// isCriterion reports whether s is one of the grader's own criterion
// sentences (internal rubric wording).
func isCriterion(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, o := range judgeOptions {
		if s == o.Reason {
			return true
		}
	}
	return false
}

// ShownWhy is the one-line why the learner sees for a stored judgment:
// the skip line for an empty answer judged Incorrect (model-graded or an
// exact text), the plain sentence for a grader criterion (or a missing
// reason), and otherwise the stored reason as it is (an exact text's word
// count, or a reason recorded some other way). The stored reason itself
// is never changed.
func ShownWhy(v srs.Verdict, reason, answer string) string {
	if strings.TrimSpace(answer) == "" && v == srs.Missed {
		return SkipWhy
	}
	return HistoryWhy(v, reason)
}

// HistoryWhy is a stored judgment's why in a card's history list: the
// plain sentence in place of a grader criterion, otherwise the stored
// reason. (The review's skip line points "above", so it is not used here.)
func HistoryWhy(v srs.Verdict, reason string) string {
	if strings.TrimSpace(reason) == "" || isCriterion(reason) {
		if w := PlainWhy(v); w != "" {
			return w
		}
	}
	return reason
}
