// Package learning owns Scry's deterministic grading and scheduling policy.
package learning

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// Scheduler pins the FSRS dependency, its default weights, and every scheduling
// policy choice. It is the identity of the schedule-card contract.
const Scheduler = "go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m"

// Algorithm is the historical combined identity: the scheduler plus the exact
// grading policy. Its value is frozen because schedule cards and every review
// event written before semantic grading carry it verbatim. Recognition and
// recall share scheduling, but remain distinct in quiz history.
const Algorithm = Scheduler + ";grading=exact-v1"

// EventAlgorithm names the effective policy behind one review event: the pinned
// scheduler joined with the grading policy that produced the event. Exact events
// keep the historical Algorithm value; semantic events name their own policy.
func EventAlgorithm(grading string) string { return Scheduler + ";grading=" + grading }

// Card is the complete portable scheduler state, not a second scheduling model.
type Card = fsrs.Card

var scheduler, schedulerError = pinnedScheduler()

func pinnedScheduler() (*fsrs.FSRS, error) {
	p := fsrs.DefaultParam()
	p.EnableFuzz = false
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pinned scheduler parameters: %w", err)
	}
	// go-fsrs copies its Parameters per pass; this instance remains immutable.
	return fsrs.NewFSRS(p), nil
}

// NewCard starts a genuinely new (or explicitly reset) schedule.
func NewCard(now time.Time) Card { return fsrs.NewCard(now.UTC()) }

// Schedule accepts only ratings Scry can substantiate: Again or Good. No timer,
// animation, near-match, or assistance can manufacture a Hard/Easy success.
func Schedule(card Card, rating int, now time.Time) (Card, error) {
	if rating != int(fsrs.Again) && rating != int(fsrs.Good) {
		return Card{}, fmt.Errorf("unsupported learning rating %d", rating)
	}
	if schedulerError != nil {
		return Card{}, schedulerError
	}
	result, err := scheduler.Next(card, now.UTC(), fsrs.Rating(rating))
	if err != nil {
		return Card{}, fmt.Errorf("schedule review: %w", err)
	}
	return result.Card, nil
}

// Grade is deliberately local and only ever awards an exact match. Variants
// must be explicitly authored; punctuation, accents, case, negation and word
// order are never silently discarded here.
//
// answerForm "exact" is authored only when identity matters (numbers, names,
// symbols, spellings, verbatim text), so it keeps local authority: a case- or
// spacing-only difference asks the learner (Polish is not polish), a short
// clear mismatch is a miss, and a long unmatched answer asks the learner. It
// is never sent to the meaning check, which tolerates spelling slips.
//
// Any other recall answer (flexible, or a legacy question with no authored
// form) is left ungraded for the short-answer or rubric assessor, so "water"
// for "Water", or "H2O", can pass on meaning. When no assessor is configured
// the staged check fails closed to a learner self-check.
func Grade(kind, grading, answerForm, expected string, variants []string, answer string, reveal bool) (outcome string, rating int) {
	if reveal {
		return "revealed", int(fsrs.Again)
	}
	if kind == "choice" {
		if answer == expected {
			return "correct", int(fsrs.Good)
		}
		return "wrong", int(fsrs.Again)
	}
	answer = strings.TrimSpace(answer)
	expected = strings.TrimSpace(expected)
	if answer == expected {
		return "correct", int(fsrs.Good)
	}
	for _, variant := range variants {
		if answer == strings.TrimSpace(variant) {
			return "correct", int(fsrs.Good)
		}
	}
	if grading == "semantic" || answerForm != "exact" {
		return "ungraded", 0
	}
	if strings.EqualFold(collapseSpace(answer), collapseSpace(expected)) {
		return "selfcheck", 0
	}
	if shortFact(answer) && shortFact(expected) {
		return "wrong", int(fsrs.Again)
	}
	return "selfcheck", 0
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func shortFact(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 80 || strings.ContainsAny(s, "\n\r;?!") {
		return false
	}
	return len(strings.Fields(s)) <= 8
}
