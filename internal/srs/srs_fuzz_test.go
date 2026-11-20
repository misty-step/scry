package srs

import (
	"math"
	"testing"
	"time"
)

// fuzzTime keeps fuzzed instants within a span time arithmetic is exact in
// (1970 to about 2290), so the properties are about scheduling, not int64
// overflow in the time package.
func fuzzTime(sec int64) time.Time {
	const span = int64(10_000_000_000)
	sec %= span
	if sec < 0 {
		sec += span
	}
	return time.Unix(sec, 0).UTC()
}

func fuzzVerdict(b byte) Verdict {
	switch b % 4 {
	case 0:
		return Right
	case 1:
		return Close
	case 2:
		return Missed
	}
	return Verdict("perfect")
}

// checkNext is the contract of one review of any state, valid or not.
func checkNext(t *testing.T, s State, v Verdict, now time.Time) State {
	t.Helper()
	n := Next(s, v, now)
	if err := ValidateState(n); err != nil {
		t.Fatalf("Next(%+v, %q, %s) = %+v: %v", s, v, now, n, err)
	}
	if n.Reps != max(s.Reps, 0)+1 || !n.Last.Equal(now) {
		t.Fatalf("Next(%+v) reps/last = %d/%s", s, n.Reps, n.Last)
	}
	if gap := n.Due.Sub(now); gap < MinInterval || gap > MaxInterval {
		t.Fatalf("Next(%+v) due %s after the review", s, gap)
	}
	c, err := NextChecked(s, v, now)
	switch {
	case !v.Valid():
		if err == nil {
			t.Fatalf("NextChecked accepted verdict %q", v)
		}
	case ValidateState(s) != nil:
		if err == nil {
			t.Fatalf("NextChecked accepted invalid state %+v", s)
		}
	case err != nil:
		t.Fatalf("NextChecked(%+v, %s) refused a valid review: %v", s, v, err)
	case !sameState(c, n):
		t.Fatalf("Next %+v != NextChecked %+v", n, c)
	default:
		if !sameState(c, legacyNext(s, v, now)) {
			t.Fatalf("table disagrees with the legacy formulas for %+v %s", s, v)
		}
		if c.Lapses < s.Lapses || c.Lapses > c.Reps {
			t.Fatalf("lapses %d -> %d (reps %d)", s.Lapses, c.Lapses, c.Reps)
		}
	}
	return n
}

// FuzzFSRSNext reviews one arbitrary memory state (including NaN, Inf,
// negative counts and out-of-range values) with an arbitrary verdict.
func FuzzFSRSNext(f *testing.F) {
	f.Add(0.0, 0.0, 0, 0, int64(0), int64(0), int64(1_800_000_000), byte(0))
	f.Add(3.7, 5.0, 3, 1, int64(1_799_000_000), int64(1_799_900_000), int64(1_800_000_000), byte(2))
	f.Add(math.NaN(), 5.0, 2, 0, int64(1_799_000_000), int64(1_799_900_000), int64(1_800_000_000), byte(1))
	f.Add(math.Inf(1), math.Inf(-1), -3, 9, int64(-5), int64(7), int64(3), byte(3))
	f.Fuzz(func(t *testing.T, stab, diff float64, reps, lapses int, last, due, now int64, vb byte) {
		s := State{Stability: stab, Difficulty: diff, Reps: reps % 100000, Lapses: lapses % 100000}
		if last != 0 {
			s.Last = fuzzTime(last)
		}
		if due != 0 {
			s.Due = fuzzTime(due)
		}
		checkNext(t, s, fuzzVerdict(vb), fuzzTime(now))
	})
}

// FuzzFSRSSequence reviews a new card along an arbitrary sequence of
// verdicts and gaps: every step must stay valid, monotone and identical
// to the pre-table scheduler.
func FuzzFSRSSequence(f *testing.F) {
	f.Add([]byte{0, 0, 0, 2, 1, 0}, int64(1_800_000_000))
	f.Add([]byte{2, 2, 2, 2}, int64(0))
	f.Add([]byte{1, 3, 0, 255, 17, 64}, int64(-1))
	f.Fuzz(func(t *testing.T, script []byte, start int64) {
		if len(script) > 256 {
			script = script[:256]
		}
		s, now := State{}, fuzzTime(start)
		for i := 0; i+1 < len(script); i += 2 {
			v := fuzzVerdict(script[i])
			// Gap: relative to the due date, from two days early to 250 days late.
			gap := time.Duration(int(script[i+1])-48) * 24 * time.Hour
			if !s.Due.IsZero() {
				now = s.Due.Add(gap)
			}
			prev := s
			s = checkNext(t, s, v, now)
			if s.Reps != prev.Reps+1 || s.Lapses < prev.Lapses {
				t.Fatalf("step %d: %+v -> %+v", i/2, prev, s)
			}
		}
	})
}
