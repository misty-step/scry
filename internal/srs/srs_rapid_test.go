package srs

import (
	"errors"
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// legacyNext is Next exactly as it was before scheduling became an
// explicit transition table. For every valid verdict and valid state the
// table must agree with it bit for bit.
func legacyNext(s State, v Verdict, now time.Time) State {
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

func sameState(a, b State) bool {
	return math.Float64bits(a.Stability) == math.Float64bits(b.Stability) &&
		math.Float64bits(a.Difficulty) == math.Float64bits(b.Difficulty) &&
		a.Reps == b.Reps && a.Lapses == b.Lapses && a.Last.Equal(b.Last) && a.Due.Equal(b.Due)
}

var verdictGen = rapid.SampledFrom(Verdicts)

// TestRapidVerdictSequences drives one card through a random sequence of
// verdicts at random gaps (early, on time, late, even before its last
// review) and checks the formal FSRS invariants of specs/study_session.qnt
// after every step: reps and lapses never fall, lapses <= reps, the state
// stays inside the model's bounds, the next due date is 1..3650 days after
// the review, Next agrees with NextChecked and with the pre-table formulas.
func TestRapidVerdictSequences(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := State{}
		now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
		steps := rapid.IntRange(1, 40).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			v := verdictGen.Draw(t, "verdict")
			gapHours := rapid.IntRange(-48, 24*800).Draw(t, "gapHours")
			at := now.Add(time.Duration(gapHours) * time.Hour)
			if i == 0 || rapid.Bool().Draw(t, "onDue") {
				at = s.Due.Add(time.Duration(gapHours%48) * time.Hour)
				if s.Due.IsZero() {
					at = now
				}
			}
			n, err := NextChecked(s, v, at)
			if err != nil {
				t.Fatalf("step %d: NextChecked(%+v, %s) failed: %v", i, s, v, err)
			}
			if lenient := Next(s, v, at); !sameState(n, lenient) {
				t.Fatalf("step %d: Next %+v != NextChecked %+v", i, lenient, n)
			}
			if old := legacyNext(s, v, at); !sameState(n, old) {
				t.Fatalf("step %d: table %+v != legacy formulas %+v", i, n, old)
			}
			if err := ValidateState(n); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
			if n.Reps != s.Reps+1 || n.Lapses < s.Lapses || n.Lapses > n.Reps {
				t.Fatalf("step %d: counts %d/%d -> %d/%d", i, s.Reps, s.Lapses, n.Reps, n.Lapses)
			}
			if wantLapse := v == Missed && s.Reps > 0; (n.Lapses == s.Lapses+1) != wantLapse {
				t.Fatalf("step %d: %s on reps=%d moved lapses %d -> %d", i, v, s.Reps, s.Lapses, n.Lapses)
			}
			if v == Missed && n.Due.Sub(at) != 24*time.Hour {
				t.Fatalf("step %d: missed item due in %s", i, n.Due.Sub(at))
			}
			s, now = n, at
		}
	})
}

// TestRapidInvalidVerdictsAreRejected: NextChecked refuses any verdict
// outside the three, typed, without touching the state; Next schedules it
// exactly as Missed.
func TestRapidInvalidVerdictsAreRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := Verdict(rapid.String().Draw(t, "verdict"))
		if v.Valid() {
			t.Skip("valid verdict")
		}
		s := Next(State{}, Right, time.Unix(1_800_000_000, 0).UTC())
		at := s.Due
		got, err := NextChecked(s, v, at)
		var te *TransitionError
		if !errors.Is(err, ErrInvalidVerdict) || !errors.As(err, &te) || te.From != StateReview {
			t.Fatalf("NextChecked(%q) err = %v", v, err)
		}
		if !sameState(got, s) {
			t.Fatalf("a refused transition changed the state")
		}
		if !sameState(Next(s, v, at), Next(s, Missed, at)) {
			t.Fatalf("Next(%q) is not scheduled as missed", v)
		}
	})
}

func TestTransitionTableIsComplete(t *testing.T) {
	for _, c := range []CardState{StateNew, StateReview} {
		for _, v := range Verdicts {
			tr, ok := LookupTransition(c, v)
			if !ok || tr.From != c || tr.On != v || tr.To != StateReview {
				t.Fatalf("(%s, %s): %+v %v", c, v, tr, ok)
			}
			if tr.Lapse != (c == StateReview && v == Missed) || tr.MinInterval != (v == Missed) {
				t.Fatalf("(%s, %s): lapse/min-interval wrong: %+v", c, v, tr)
			}
		}
		if _, ok := LookupTransition(c, "perfect"); ok {
			t.Fatalf("(%s, perfect) is in the table", c)
		}
	}
}

func TestValidateStateRejectsBrokenStates(t *testing.T) {
	good := Next(State{}, Right, t0)
	cases := map[string]State{
		"NaN stability":     {Stability: math.NaN(), Difficulty: 5, Reps: 1, Last: t0, Due: t0.Add(48 * time.Hour)},
		"Inf difficulty":    {Stability: 3, Difficulty: math.Inf(1), Reps: 1, Last: t0, Due: t0.Add(48 * time.Hour)},
		"negative reps":     {Reps: -1},
		"lapses over reps":  {Stability: 3, Difficulty: 5, Reps: 1, Lapses: 2, Last: t0, Due: t0.Add(48 * time.Hour)},
		"stability too low": {Stability: 0.01, Difficulty: 5, Reps: 1, Last: t0, Due: t0.Add(48 * time.Hour)},
		"difficulty high":   {Stability: 3, Difficulty: 11, Reps: 1, Last: t0, Due: t0.Add(48 * time.Hour)},
		"due before last":   {Stability: 3, Difficulty: 5, Reps: 1, Last: t0, Due: t0.Add(-time.Hour)},
		"due within a day":  {Stability: 3, Difficulty: 5, Reps: 1, Last: t0, Due: t0.Add(time.Hour)},
		"no last review":    {Stability: 3, Difficulty: 5, Reps: 1, Due: t0},
	}
	for name, s := range cases {
		if err := ValidateState(s); !errors.Is(err, ErrInvalidState) {
			t.Errorf("%s: ValidateState = %v", name, err)
		}
		if _, err := NextChecked(s, Right, t0); !errors.Is(err, ErrInvalidState) {
			t.Errorf("%s: NextChecked = %v", name, err)
		}
		if err := ValidateState(Next(s, Right, t0.Add(72*time.Hour))); err != nil {
			t.Errorf("%s: Next did not repair: %v", name, err)
		}
	}
	if err := ValidateState(good); err != nil || ValidateState(State{}) != nil {
		t.Fatalf("valid states rejected: %v", err)
	}
}
