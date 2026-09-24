package learning

import (
	"reflect"
	"testing"
	"time"
)

// US-003: one local rule. The exact key or an authored variant is correct and
// a wrong choice is a miss; every other recall answer, however close or far,
// is left for the assessor rather than decided by similarity here.
func TestGradeHasOneLocalRule(t *testing.T) {
	cases := []struct {
		name, kind, expected, answer string
		variants                     []string
		reveal                       bool
		outcome                      string
		rating                       int
	}{
		{"exact key", "recall", "Water", "  Water ", nil, false, "correct", 3},
		{"authored variant", "recall", "Water", "H2O", []string{"H2O"}, false, "correct", 3},
		{"case difference goes to the check", "recall", "Water", "water", nil, false, "ungraded", 0},
		{"spelling slip goes to the check", "recall", "Tchaikovsky", "Tchaikovski", nil, false, "ungraded", 0},
		{"short mismatch goes to the check", "recall", "Paris", "Berlin", nil, false, "ungraded", 0},
		{"negation is never normalized", "recall", "oxygen", "not oxygen", nil, false, "ungraded", 0},
		{"right choice", "choice", "IPv4 address", "IPv4 address", nil, false, "correct", 3},
		{"wrong choice", "choice", "IPv4 address", "IPv6 address", nil, false, "wrong", 1},
		{"help is never cold success", "recall", "oxygen", "oxygen", nil, true, "revealed", 1},
	}
	for _, tc := range cases {
		if outcome, rating := Grade(tc.kind, tc.expected, tc.variants, tc.answer, tc.reveal); outcome != tc.outcome || rating != tc.rating {
			t.Errorf("%s: got %s/%d, want %s/%d", tc.name, outcome, rating, tc.outcome, tc.rating)
		}
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
