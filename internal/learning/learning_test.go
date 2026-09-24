package learning

import (
	"reflect"
	"testing"
	"time"
)

// US-003: the local grader only awards an exact or authored-variant match.
// An authored exact form keeps local authority, so a spelling or case slip is
// never handed to the meaning check; a legacy (unspecified) or flexible form
// leaves every other answer ungraded for that check.
func TestLocalGradeBoundaries(t *testing.T) {
	cases := []struct {
		name, expected, answer string
		variants               []string
		reveal                 bool
		exact                  string // outcome for answer form "exact"
		other                  string // outcome for "" and "flexible"
	}{
		{"exact match", "Water", "  Water ", nil, false, "correct", "correct"},
		{"explicit variant", "sodium chloride", "NaCl", []string{"NaCl"}, false, "correct", "correct"},
		{"case difference", "Polish", "polish", nil, false, "selfcheck", "ungraded"},
		{"spacing difference", "New  York", "new york", nil, false, "selfcheck", "ungraded"},
		{"spelling slip in an exact form", "Tchaikovsky", "Tchaikovski", nil, false, "wrong", "ungraded"},
		{"negation is not normalized away", "oxygen", "not oxygen", nil, false, "wrong", "ungraded"},
		{"short mismatch", "Water", "H2O", nil, false, "wrong", "ungraded"},
		{"punctuation is not discarded", "C++", "C", nil, false, "wrong", "ungraded"},
		{"long unmatched answer", "A process that releases stored chemical energy for cellular work", "Cells convert the energy they have stored into usable work", nil, false, "selfcheck", "ungraded"},
		{"help is never cold success", "oxygen", "oxygen", nil, true, "revealed", "revealed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, form := range []string{"exact", "", "flexible"} {
				want := tc.other
				if form == "exact" {
					want = tc.exact
				}
				if outcome, _ := Grade("recall", "exact", form, tc.expected, tc.variants, tc.answer, tc.reveal); outcome != want {
					t.Fatalf("form %q: got %s, want %s", form, outcome, want)
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
