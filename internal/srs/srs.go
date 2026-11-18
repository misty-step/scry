// Package srs schedules reviews. It follows the published FSRS-4.5 memory
// model (stability, difficulty, retrievability) with its default weights.
// The learner never grades themself: the judge's verdict is the grade.
package srs

import (
	"math"
	"time"
)

// Verdict is the outcome of a judged answer.
type Verdict string

const (
	Right  Verdict = "right"
	Close  Verdict = "close"
	Missed Verdict = "missed"
)

// Valid reports whether v is one of the three verdicts.
func (v Verdict) Valid() bool { return v == Right || v == Close || v == Missed }

// grade maps a verdict onto the FSRS grades: again=1, hard=2, good=3.
func (v Verdict) grade() float64 {
	switch v {
	case Right:
		return 3
	case Close:
		return 2
	default:
		return 1
	}
}

// State is the memory state of one item.
type State struct {
	Stability  float64 // days until recall probability falls to 90%
	Difficulty float64 // 1 (easy) .. 10 (hard)
	Reps       int     // judged reviews so far
	Lapses     int     // times missed after first being learned
	Last       time.Time
	Due        time.Time
}

// New reports whether the item has never been reviewed.
func (s State) New() bool { return s.Reps == 0 }

var w = [17]float64{0.4872, 1.4003, 3.7145, 13.8206, 5.1618, 1.2298, 0.8975, 0.031,
	1.6474, 0.1367, 1.0461, 2.1072, 0.0793, 0.3246, 1.587, 0.2272, 2.8755}

const (
	decay     = -0.5
	factor    = 19.0 / 81.0
	retention = 0.9
	minDays   = 1.0
	maxDays   = 3650.0
)

func clamp(x, lo, hi float64) float64 { return math.Min(math.Max(x, lo), hi) }

func initDifficulty(g float64) float64 { return clamp(w[4]-(g-3)*w[5], 1, 10) }

// Retrievability is the modelled chance of recall after elapsed days.
func Retrievability(elapsedDays, stability float64) float64 {
	if stability <= 0 {
		return 0
	}
	return math.Pow(1+factor*elapsedDays/stability, decay)
}

// intervalDays is the gap that brings recall back down to the target retention.
func intervalDays(stability float64) float64 {
	d := stability / factor * (math.Pow(retention, 1/decay) - 1)
	return clamp(math.Round(d), minDays, maxDays)
}

// Next returns the state after a review judged v at time now.
// Every verdict schedules the item at least one day out, so a missed item
// is revisited tomorrow rather than hammered within the session.
func Next(s State, v Verdict, now time.Time) State {
	g := v.grade()
	n := s
	n.Reps = s.Reps + 1
	n.Last = now
	if s.New() || s.Stability <= 0 {
		n.Stability = w[int(g)-1]
		n.Difficulty = initDifficulty(g)
	} else {
		elapsed := math.Max(now.Sub(s.Last).Hours()/24, 0)
		r := Retrievability(elapsed, s.Stability)
		n.Difficulty = clamp(w[7]*initDifficulty(3)+(1-w[7])*(s.Difficulty-w[6]*(g-3)), 1, 10)
		if v == Missed {
			n.Lapses = s.Lapses + 1
			forget := w[11] * math.Pow(s.Difficulty, -w[12]) * (math.Pow(s.Stability+1, w[13]) - 1) * math.Exp(w[14]*(1-r))
			n.Stability = clamp(math.Min(forget, s.Stability), 0.1, maxDays)
		} else {
			hard := 1.0
			if v == Close {
				hard = w[15]
			}
			grow := math.Exp(w[8]) * (11 - s.Difficulty) * math.Pow(s.Stability, -w[9]) * (math.Exp(w[10]*(1-r)) - 1) * hard
			n.Stability = clamp(s.Stability*(1+grow), 0.1, maxDays)
		}
	}
	days := intervalDays(n.Stability)
	if v == Missed {
		days = minDays
	}
	n.Due = now.Add(time.Duration(days*24) * time.Hour)
	return n
}

// Label is the accepted learner-facing vocabulary; stored grades stay stable.
func (v Verdict) Label() string {
	switch v {
	case Right:
		return "correct"
	case Close:
		return "almost correct"
	case Missed:
		return "incorrect"
	}
	return string(v)
}
