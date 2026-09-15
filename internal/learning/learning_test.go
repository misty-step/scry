package learning

import (
	"reflect"
	"testing"
	"time"
)

func TestConservativeGradeBoundaries(t *testing.T) {
	cases := []struct {
		name, expected, answer string
		variants               []string
		reveal                 bool
		outcome                string
		rating                 int
	}{
		{"explicit variant", "sodium chloride", "NaCl", []string{"NaCl"}, false, "correct", 3},
		{"meaningful case", "Polish", "polish", nil, false, "close", 0},
		{"negation is not normalized away", "oxygen", "not oxygen", nil, false, "wrong", 1},
		{"short factual miss", "Paris", "Berlin", nil, false, "wrong", 1},
		{"punctuation is not discarded", "C++", "C", nil, false, "wrong", 1},
		{"semantic ambiguity", "A process that releases stored chemical energy for cellular work", "Cells convert the energy they have stored into usable work", nil, false, "ungraded", 0},
		{"help is never cold success", "oxygen", "oxygen", nil, true, "revealed", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, rating := Grade("recall", tc.expected, tc.variants, tc.answer, tc.reveal)
			if outcome != tc.outcome || rating != tc.rating {
				t.Fatalf("got %s/%d, want %s/%d", outcome, rating, tc.outcome, tc.rating)
			}
		})
	}
}

// This is the pinned dependency's published basic trajectory, exercising the
// adapter's actual learning/relearning transitions rather than its wiring.
func TestPinnedFSRSTrajectory(t *testing.T) {
	ratings := []int{3, 3, 3, 3, 3, 3, 1, 1, 3, 3, 3, 3, 3}
	wantDays := []uint64{0, 2, 11, 46, 163, 498, 0, 0, 2, 4, 7, 12, 21}
	now := time.Date(2022, 11, 29, 12, 30, 0, 0, time.UTC)
	card := NewCard(now)
	for i, rating := range ratings {
		next, err := Schedule(card, rating, now)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := Schedule(card, rating, now.In(time.FixedZone("offset", 3600)))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(next, replayed) {
			t.Fatalf("step %d changed under equivalent UTC time", i)
		}
		if next.ScheduledDays != wantDays[i] {
			t.Fatalf("step %d interval=%d, want %d", i, next.ScheduledDays, wantDays[i])
		}
		if !next.Due.After(now) {
			t.Fatalf("step %d did not schedule future work", i)
		}
		card, now = next, next.Due
	}
	if _, err := Schedule(card, 4, now); err == nil {
		t.Fatal("unearned Easy rating was accepted")
	}
}
