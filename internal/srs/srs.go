// Package srs schedules reviews. It follows the published FSRS-4.5 memory
// model (stability, difficulty, retrievability) with its default weights.
// The learner never grades themself: the judge's verdict is the grade.
//
// Scheduling is an explicit state machine. A card is in one of two
// states (StateNew, never reviewed; StateReview, reviewed at least once),
// and each (state, verdict) pair names one entry of a fixed transition
// table that says what a review does: initialise the memory state, grow
// it, or forget (a lapse). NextChecked applies that table strictly: it
// rejects a verdict outside the three, rejects a memory state that breaks
// the model's bounds (NaN, Inf, out-of-range stability or difficulty, more
// lapses than reviews, a due date not after the last review), and checks
// the state it produces against the same bounds before returning it.
package srs

import (
	"errors"
	"fmt"
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

// Verdicts is every verdict a judge may return, best first.
var Verdicts = []Verdict{Right, Close, Missed}

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

// CardState is where an item stands in the scheduling state machine.
type CardState uint8

const (
	// StateNew is an item never reviewed: Reps is 0 and its memory state
	// has not been initialised.
	StateNew CardState = iota
	// StateReview is an item reviewed at least once, with a stability,
	// difficulty, last review and due date inside the model's bounds.
	StateReview
)

func (c CardState) String() string {
	switch c {
	case StateNew:
		return "new"
	case StateReview:
		return "review"
	}
	return fmt.Sprintf("CardState(%d)", uint8(c))
}

// CardState reports the item's scheduling state.
func (s State) CardState() CardState {
	if s.Reps == 0 {
		return StateNew
	}
	return StateReview
}

// Bounds of a reviewed memory state.
const (
	MinStability  = 0.1
	MaxStability  = maxDays
	MinDifficulty = 1.0
	MaxDifficulty = 10.0
	// MinInterval is the shortest gap between a review and the next due
	// date: every verdict schedules the item at least a day out.
	MinInterval = 24 * time.Hour
	// MaxInterval is the longest gap between a review and the next due date.
	MaxInterval = time.Duration(maxDays) * 24 * time.Hour
)

// Errors of the scheduling state machine.
var (
	// ErrInvalidVerdict is a verdict outside right, close and missed.
	ErrInvalidVerdict = errors.New("srs: invalid verdict")
	// ErrInvalidState is a memory state outside the model's bounds.
	ErrInvalidState = errors.New("srs: invalid memory state")
)

// TransitionError says which scheduling transition was refused and why.
// It matches ErrInvalidVerdict or ErrInvalidState with errors.Is.
type TransitionError struct {
	From    CardState
	Verdict Verdict
	Err     error  // ErrInvalidVerdict or ErrInvalidState
	Detail  string // what was wrong
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%v: %s on a %s card: %s", e.Err, e.Verdict, e.From, e.Detail)
}

func (e *TransitionError) Unwrap() error { return e.Err }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// ValidateState reports whether s is a memory state the model can be in.
// Every state is finite with non-negative counts and no more lapses than
// reviews. A new state carries no lapses. A reviewed state has stability
// in [MinStability, MaxStability], difficulty in [MinDifficulty,
// MaxDifficulty], a last review, and a due date between MinInterval and
// MaxInterval after it.
func ValidateState(s State) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalidState}, args...)...)
	}
	switch {
	case !finite(s.Stability):
		return bad("stability %v is not finite", s.Stability)
	case !finite(s.Difficulty):
		return bad("difficulty %v is not finite", s.Difficulty)
	case s.Reps < 0:
		return bad("reps %d < 0", s.Reps)
	case s.Lapses < 0:
		return bad("lapses %d < 0", s.Lapses)
	case s.Lapses > s.Reps:
		return bad("lapses %d > reps %d", s.Lapses, s.Reps)
	}
	if s.CardState() == StateNew {
		return nil
	}
	switch {
	case s.Stability < MinStability || s.Stability > MaxStability:
		return bad("stability %v outside [%v, %v]", s.Stability, MinStability, MaxStability)
	case s.Difficulty < MinDifficulty || s.Difficulty > MaxDifficulty:
		return bad("difficulty %v outside [%v, %v]", s.Difficulty, MinDifficulty, MaxDifficulty)
	case s.Last.IsZero():
		return bad("a reviewed item has no last review")
	}
	if gap := s.Due.Sub(s.Last); gap < MinInterval || gap > MaxInterval {
		return bad("due %s is %s after the last review, outside [%s, %s]", s.Due.Format(time.RFC3339), gap, MinInterval, MaxInterval)
	}
	return nil
}

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

// RetrievabilityAt is the item's modelled chance of recall at now: the
// FSRS forgetting curve at the time elapsed since its last review. An item
// never reviewed has no memory to recall and reads 0; so does a reviewed
// item with no stored last review or stability (out of bounds), which
// makes it the most urgent rather than hiding it.
func (s State) RetrievabilityAt(now time.Time) float64 {
	if s.Reps == 0 || s.Last.IsZero() || !finite(s.Stability) || s.Stability <= 0 {
		return 0
	}
	elapsed := now.Sub(s.Last).Hours() / 24
	if elapsed < 0 {
		elapsed = 0
	}
	return Retrievability(elapsed, s.Stability)
}

// intervalDays is the gap that brings recall back down to the target retention.
func intervalDays(stability float64) float64 {
	d := stability / factor * (math.Pow(retention, 1/decay) - 1)
	return clamp(math.Round(d), minDays, maxDays)
}

// Effect is how one review moves the memory state.
type Effect uint8

const (
	// EffectInit gives a first review its initial stability and difficulty.
	EffectInit Effect = iota
	// EffectRecall grows stability after a recalled review.
	EffectRecall
	// EffectForget shrinks stability after a miss (a lapse).
	EffectForget
)

// Transition is one entry of the scheduling table.
type Transition struct {
	From   CardState
	On     Verdict
	To     CardState
	Effect Effect
	Lapse  bool    // the review counts as a lapse
	Hard   float64 // stability-growth penalty for a recall (w[15] for close, 1 for right)
	// MinInterval schedules the item exactly one day out, whatever its
	// stability: a missed item is revisited tomorrow, not hammered now.
	MinInterval bool
}

type transitionKey struct {
	from CardState
	on   Verdict
}

// transitions is the whole scheduling state machine. Every review moves a
// card to StateReview; what differs is how its memory state changes.
var transitions = map[transitionKey]Transition{
	{StateNew, Right}:     {From: StateNew, On: Right, To: StateReview, Effect: EffectInit},
	{StateNew, Close}:     {From: StateNew, On: Close, To: StateReview, Effect: EffectInit},
	{StateNew, Missed}:    {From: StateNew, On: Missed, To: StateReview, Effect: EffectInit, MinInterval: true},
	{StateReview, Right}:  {From: StateReview, On: Right, To: StateReview, Effect: EffectRecall, Hard: 1},
	{StateReview, Close}:  {From: StateReview, On: Close, To: StateReview, Effect: EffectRecall, Hard: w[15]},
	{StateReview, Missed}: {From: StateReview, On: Missed, To: StateReview, Effect: EffectForget, Lapse: true, MinInterval: true},
}

// LookupTransition returns the table entry for a review judged v of a card
// in state c. It reports false for a verdict outside the three.
func LookupTransition(c CardState, v Verdict) (Transition, bool) {
	t, ok := transitions[transitionKey{c, v}]
	return t, ok
}

// apply runs one table entry. s must be a state the entry's formulas are
// defined for: finite, and for EffectRecall/EffectForget, reviewed with a
// positive stability.
func apply(t Transition, s State, now time.Time) State {
	g := t.On.grade()
	n := s
	n.Reps = s.Reps + 1
	n.Last = now
	switch t.Effect {
	case EffectInit:
		n.Stability = w[int(g)-1]
		n.Difficulty = initDifficulty(g)
	default:
		elapsed := math.Max(now.Sub(s.Last).Hours()/24, 0)
		r := Retrievability(elapsed, s.Stability)
		n.Difficulty = clamp(w[7]*initDifficulty(3)+(1-w[7])*(s.Difficulty-w[6]*(g-3)), 1, 10)
		if t.Effect == EffectForget {
			forget := w[11] * math.Pow(s.Difficulty, -w[12]) * (math.Pow(s.Stability+1, w[13]) - 1) * math.Exp(w[14]*(1-r))
			n.Stability = clamp(math.Min(forget, s.Stability), MinStability, maxDays)
		} else {
			grow := math.Exp(w[8]) * (11 - s.Difficulty) * math.Pow(s.Stability, -w[9]) * (math.Exp(w[10]*(1-r)) - 1) * t.Hard
			n.Stability = clamp(s.Stability*(1+grow), MinStability, maxDays)
		}
	}
	if t.Lapse {
		n.Lapses = s.Lapses + 1
	}
	days := intervalDays(n.Stability)
	if t.MinInterval {
		days = minDays
	}
	n.Due = now.Add(time.Duration(days*24) * time.Hour)
	return n
}

// NextChecked returns the state after a review judged v at time now, or
// a *TransitionError: ErrInvalidVerdict for a verdict outside the three,
// ErrInvalidState when s (or, defensively, the result) breaks the model's
// bounds. On success the result is a valid StateReview state with one more
// rep, lapses grown only by a miss of a reviewed card, and a due date at
// least a day after now.
func NextChecked(s State, v Verdict, now time.Time) (State, error) {
	from := s.CardState()
	t, ok := LookupTransition(from, v)
	if !ok {
		return s, &TransitionError{From: from, Verdict: v, Err: ErrInvalidVerdict, Detail: fmt.Sprintf("%q is not right, close or missed", string(v))}
	}
	if err := ValidateState(s); err != nil {
		return s, &TransitionError{From: from, Verdict: v, Err: ErrInvalidState, Detail: err.Error()}
	}
	n := apply(t, s, now)
	if err := checkStep(s, n, t, now); err != nil {
		return s, &TransitionError{From: from, Verdict: v, Err: ErrInvalidState, Detail: "produced " + err.Error()}
	}
	return n, nil
}

// checkStep is the post-condition of one transition.
func checkStep(s, n State, t Transition, now time.Time) error {
	if err := ValidateState(n); err != nil {
		return err
	}
	switch {
	case n.CardState() != t.To:
		return fmt.Errorf("%w: state %s, want %s", ErrInvalidState, n.CardState(), t.To)
	case n.Reps != s.Reps+1:
		return fmt.Errorf("%w: reps %d -> %d", ErrInvalidState, s.Reps, n.Reps)
	case t.Lapse && n.Lapses != s.Lapses+1, !t.Lapse && n.Lapses != s.Lapses:
		return fmt.Errorf("%w: lapses %d -> %d", ErrInvalidState, s.Lapses, n.Lapses)
	case !n.Last.Equal(now):
		return fmt.Errorf("%w: last review %s, want %s", ErrInvalidState, n.Last, now)
	}
	return nil
}

// sanitize maps any state onto one the transition formulas are defined
// for, and the row of the table it takes. A valid state is returned
// unchanged with its own CardState. Broken counts are clamped; a reviewed
// state whose memory cannot be trusted (non-finite or non-positive
// stability, non-finite difficulty) takes the StateNew row, so its next
// review re-initialises it, as a non-positive stability always did; a
// finite difficulty out of range is clamped.
func sanitize(s State) (State, CardState) {
	s.Reps = max(s.Reps, 0)
	s.Lapses = min(max(s.Lapses, 0), s.Reps)
	if s.Reps == 0 {
		return s, StateNew
	}
	if !finite(s.Stability) || s.Stability <= 0 || !finite(s.Difficulty) {
		return s, StateNew
	}
	s.Difficulty = clamp(s.Difficulty, MinDifficulty, MaxDifficulty)
	return s, StateReview
}

// Next returns the state after a review judged v at time now.
// Every verdict schedules the item at least one day out, so a missed item
// is revisited tomorrow rather than hammered within the session.
//
// Next is the lenient form of NextChecked and goes through the same
// transition table; for valid inputs the two agree exactly. It never
// fails: a verdict outside the three is scheduled as Missed (the grade
// such a verdict always mapped to), so it also counts the lapse and the
// one-day interval a miss has; and a broken memory state is repaired
// (see sanitize) instead of propagating NaN or out-of-range values.
// Callers that can report an error should use NextChecked.
func Next(s State, v Verdict, now time.Time) State {
	if !v.Valid() {
		v = Missed
	}
	s, from := sanitize(s)
	t, _ := LookupTransition(from, v)
	return apply(t, s, now)
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
