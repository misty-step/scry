package learning

import (
	"reflect"
	"testing"
	"time"
)

// US-003: exact-form recall keeps local authority. A clear short miss is a miss,
// while a case-only difference or a long unmatched answer asks the learner to
// compare with the key instead of inventing either result.
func TestExactFormGradeBoundaries(t *testing.T) {
	cases := []struct {
		name, expected, answer string
		variants               []string
		reveal                 bool
		outcome                string
		rating                 int
	}{
		{"explicit variant", "sodium chloride", "NaCl", []string{"NaCl"}, false, "correct", 3},
		{"meaningful case goes to self-check", "Polish", "polish", nil, false, "selfcheck", 0},
		{"spacing-only difference goes to self-check", "New  York", "new york", nil, false, "selfcheck", 0},
		{"negation is not normalized away", "oxygen", "not oxygen", nil, false, "wrong", 1},
		{"short factual miss", "Paris", "Berlin", nil, false, "wrong", 1},
		{"punctuation is not discarded", "C++", "C", nil, false, "wrong", 1},
		{"long unmatched answer goes to self-check", "A process that releases stored chemical energy for cellular work", "Cells convert the energy they have stored into usable work", nil, false, "selfcheck", 0},
		{"help is never cold success", "oxygen", "oxygen", nil, true, "revealed", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, form := range []string{"", "exact"} {
				outcome, rating := Grade("recall", "exact", form, tc.expected, tc.variants, tc.answer, tc.reveal)
				if outcome != tc.outcome || rating != tc.rating {
					t.Fatalf("form %q: got %s/%d, want %s/%d", form, outcome, rating, tc.outcome, tc.rating)
				}
			}
		})
	}
}

// US-008: a flexible short answer that does not match exactly is left for the
// meaning check rather than marked wrong; exact matches stay local.
func TestFlexibleAndSemanticGradeDeferUnmatchedAnswers(t *testing.T) {
	if outcome, rating := Grade("recall", "", "flexible", "TLS", nil, "the TLS protocol", false); outcome != "ungraded" || rating != 0 {
		t.Fatalf("flexible synonym got %s/%d, want ungraded/0", outcome, rating)
	}
	if outcome, rating := Grade("recall", "", "flexible", "TLS", nil, "TLS", false); outcome != "correct" || rating != 3 {
		t.Fatalf("flexible exact match got %s/%d, want correct/3", outcome, rating)
	}
	if outcome, rating := Grade("recall", "semantic", "", "Paris", nil, "Berlin", false); outcome != "ungraded" || rating != 0 {
		t.Fatalf("semantic got %s/%d, want ungraded/0", outcome, rating)
	}
	if outcome, rating := Grade("choice", "", "", "IPv4 address", nil, "IPv6 address", false); outcome != "wrong" || rating != 1 {
		t.Fatalf("choice got %s/%d, want wrong/1", outcome, rating)
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
