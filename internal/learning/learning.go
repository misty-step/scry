// Package learning owns Scry's deterministic grading and scheduling policy.
package learning

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// Algorithm pins the dependency, its default weights, and all policy choices.
// Recognition and recall share scheduling, but remain distinct in quiz history.
const Algorithm = "go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m;grading=exact-v1"

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

// Grade is deliberately local. Variants must be explicitly authored; punctuation,
// accents, case, negation and word order are never silently discarded. Short
// unmatched factual answers can be misses; a long semantic answer cannot be
// judged reliably by this policy and remains ungraded until retry or reveal.
func Grade(kind, expected string, variants []string, answer string, reveal bool) (outcome string, rating int) {
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
	// A possible case-only spelling difference deserves review, not a false
	// success (e.g. Polish/polish or a case-sensitive symbol).
	if strings.EqualFold(answer, expected) {
		return "close", 0
	}
	if shortFact(answer) && shortFact(expected) {
		return "wrong", int(fsrs.Again)
	}
	return "ungraded", 0
}

func shortFact(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 80 || strings.ContainsAny(s, "\n\r;?!") {
		return false
	}
	words := 0
	for range strings.FieldsSeq(s) {
		words++
		if words > 8 {
			return false
		}
	}
	return true
}
