package learning

import (
	"testing"
	"time"
)

func reviewed(t *testing.T, ratings []int, start time.Time, gap time.Duration) (Card, []Attempt) {
	t.Helper()
	card := NewCard(start)
	now := start
	var attempts []Attempt
	for _, rating := range ratings {
		next, err := Schedule(card, rating, now)
		if err != nil {
			t.Fatal(err)
		}
		attempts = append(attempts, Attempt{At: now.UnixMilli(), Rating: rating})
		card = next
		now = now.Add(gap)
	}
	return card, attempts
}

// US-009: concept state reports evidence and a labeled estimate, never an
// invented mastery. No evidence is "new"; recent unaided successes build it up;
// a long gap after graduation reads as fading.
func TestConceptStateUS009(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	empty := ComputeConceptState(nil, start)
	if empty.Status != "new" || empty.Recall != -1 || empty.Brightness != 0 || len(empty.Tally) != 0 {
		t.Fatalf("no evidence produced %+v", empty)
	}

	card, attempts := reviewed(t, []int{3, 3, 3, 3, 3, 3}, start, 40*24*time.Hour)
	last := time.UnixMilli(attempts[len(attempts)-1].At)
	solid := ComputeConceptState([]QuestionEvidence{{Card: card, Level: "recall", Attempts: attempts}}, last.Add(time.Hour))
	if solid.Status != "solid" || solid.Recall < 0.9 || solid.Brightness < 4 || solid.Unaided != 6 {
		t.Fatalf("well-spaced unaided successes produced %+v", solid)
	}
	lightCard, lightAttempts := reviewed(t, []int{3, 3, 3}, start, 3*24*time.Hour)
	lightLast := time.UnixMilli(lightAttempts[len(lightAttempts)-1].At)
	faded := ComputeConceptState([]QuestionEvidence{{Card: lightCard, Level: "recall", Attempts: lightAttempts}}, lightLast.Add(2*365*24*time.Hour))
	if faded.Status != "fading" || faded.Recall >= 0.8 || faded.Brightness > 2 {
		t.Fatalf("two years after a few early reviews produced %+v", faded)
	}

	helpedCard, helped := reviewed(t, []int{1}, start, time.Hour)
	helped[0].Assisted, helped[0].Outcome = true, "revealed"
	missCard, missed := reviewed(t, []int{1}, start.Add(time.Minute), time.Hour)
	missed[0].Outcome = "wrong"
	mixed := ComputeConceptState([]QuestionEvidence{
		{Card: helpedCard, Level: "recognize", Attempts: helped},
		{Card: missCard, Level: "explain", Attempts: missed},
	}, start.Add(2*time.Hour))
	if mixed.Status != "learning" || mixed.Helped != 1 || mixed.Missed != 1 || mixed.Unaided != 0 {
		t.Fatalf("helped and missed attempts produced %+v", mixed)
	}
	if len(mixed.Tally) != 2 || mixed.Tally[0] != "h" || mixed.Tally[1] != "m" {
		t.Fatalf("tally = %v, want [h m] in time order", mixed.Tally)
	}

	pending := ComputeConceptState([]QuestionEvidence{{Card: NewCard(start), Attempts: []Attempt{{At: start.UnixMilli(), Rating: 0, Outcome: "selfcheck"}}}}, start)
	if pending.Status != "new" || len(pending.Tally) != 0 {
		t.Fatalf("an answer awaiting self-check counted as evidence: %+v", pending)
	}
}
