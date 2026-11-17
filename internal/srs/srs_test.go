package srs

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

func days(s State, now time.Time) float64 { return s.Due.Sub(now).Hours() / 24 }

func TestMissedComesBackTomorrow(t *testing.T) {
	s := Next(State{}, Right, t0)
	s = Next(s, Right, s.Due)
	s = Next(s, Right, s.Due)
	at := s.Due
	m := Next(s, Missed, at)
	if got := days(m, at); got != 1 {
		t.Fatalf("missed item due in %v days, want 1", got)
	}
	if m.Lapses != 1 {
		t.Fatalf("lapses = %d, want 1", m.Lapses)
	}
	if m.Stability >= s.Stability {
		t.Fatalf("stability should fall after a miss: %v -> %v", s.Stability, m.Stability)
	}
}

func TestRightSpacesFurtherEachTime(t *testing.T) {
	s := State{}
	now := t0
	prev := 0.0
	for i := 0; i < 6; i++ {
		s = Next(s, Right, now)
		d := days(s, now)
		if d < 1 {
			t.Fatalf("review %d: interval %v < 1 day", i, d)
		}
		if d <= prev {
			t.Fatalf("review %d: interval %v did not grow past %v", i, d, prev)
		}
		prev = d
		now = s.Due
	}
}

func TestVerdictOrdering(t *testing.T) {
	base := Next(Next(State{}, Right, t0), Right, t0.Add(4*24*time.Hour))
	at := base.Due
	r, c, m := Next(base, Right, at), Next(base, Close, at), Next(base, Missed, at)
	if !(days(r, at) > days(c, at) && days(c, at) >= days(m, at)) {
		t.Fatalf("want right > close >= missed, got %v %v %v", days(r, at), days(c, at), days(m, at))
	}
	if !(r.Difficulty < c.Difficulty && c.Difficulty < m.Difficulty) {
		t.Fatalf("difficulty should rise with worse verdicts: %v %v %v", r.Difficulty, c.Difficulty, m.Difficulty)
	}
}

func TestFirstReview(t *testing.T) {
	for _, v := range []Verdict{Right, Close, Missed} {
		s := Next(State{}, v, t0)
		if s.Reps != 1 || s.Stability <= 0 || s.Difficulty < 1 || s.Difficulty > 10 {
			t.Fatalf("%s: bad first state %+v", v, s)
		}
		if days(s, t0) < 1 {
			t.Fatalf("%s: first interval under a day", v)
		}
	}
	if !Verdict("right").Valid() || Verdict("perfect").Valid() {
		t.Fatal("Valid() wrong")
	}
}

func TestRetrievability(t *testing.T) {
	if r := Retrievability(10, 10); r < 0.899 || r > 0.901 {
		t.Fatalf("R(S,S) = %v, want 0.9", r)
	}
	if Retrievability(0, 5) != 1 {
		t.Fatal("R(0) should be 1")
	}
}
