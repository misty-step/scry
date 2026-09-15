package learning

import (
	"reflect"
	"testing"
	"time"
)

func TestDeferralRequiresAllExactTargetsAndOtherDirectContext(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	a, b := UnitVersion{ID: "a", Version: 2}, UnitVersion{ID: "b", Version: 1}
	event := inferenceSuccess(t, "compound", "recall", at, a, b)
	input := DeferralInput{MaterialID: "foundation-probe", MaterialVersion: 3, Mode: "recall", Units: []UnitVersion{a, b}, AsOf: at.Add(time.Second), Evidence: []Evidence{event}}
	deferred := DeferPractice(input)
	if !deferred.Deferred || !deferred.ReconsiderAt.Equal(event.DueAt) || !reflect.DeepEqual(deferred.EvidenceIDs, []string{event.ID}) || len(deferred.Units) != 2 {
		t.Fatalf("eligible all-unit evidence not inspectably deferred: %+v", deferred)
	}
	cases := []struct {
		name   string
		change func(*DeferralInput)
	}{
		{"same material", func(in *DeferralInput) { in.MaterialID = event.MaterialID }},
		{"wrong unit version", func(in *DeferralInput) { in.Units = []UnitVersion{{ID: "a", Version: 3}, b} }},
		{"uncovered unit", func(in *DeferralInput) { in.Units = []UnitVersion{a, b, {ID: "c", Version: 1}} }},
		{"recognition cannot replace recall", func(in *DeferralInput) { in.Evidence[0].Mode = "choice" }},
		{"assistance", func(in *DeferralInput) { in.Evidence[0].Assisted = true }},
		{"dispute", func(in *DeferralInput) { in.Evidence[0].Disputed = true }},
		{"ambiguous target", func(in *DeferralInput) { in.Ambiguous = true }},
		{"unmapped target", func(in *DeferralInput) { in.Unmapped = true }},
		{"assumed not assessed", func(in *DeferralInput) {
			in.Evidence[0].Targets = []EvidenceTarget{{UnitID: a.ID, UnitVersion: a.Version, Role: "assumes"}, {UnitID: b.ID, UnitVersion: b.Version, Role: "assesses"}}
		}},
		{"reference not review", func(in *DeferralInput) { in.Evidence[0].Kind = "continue" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := input
			changed.Evidence = append([]Evidence(nil), input.Evidence...)
			tc.change(&changed)
			if result := DeferPractice(changed); result.Deferred {
				t.Fatalf("ineligible evidence deferred exact assessment: %+v", result)
			}
		})
	}
	laterMiss := Evidence{ID: "new-miss", MaterialID: input.MaterialID, MaterialVersion: input.MaterialVersion, At: at.Add(2 * time.Second), Kind: "review", Mode: "recall", Outcome: "wrong", Rating: 1, Targets: event.Targets}
	input.AsOf, input.Evidence = laterMiss.At, []Evidence{event, laterMiss}
	if result := DeferPractice(input); result.Deferred {
		t.Fatalf("later ambiguous miss hidden behind old composite success: %+v", result)
	}
}

func TestDeferralReconsiderationIsFixedAndBoundedByOriginalDue(t *testing.T) {
	at := time.Date(2022, 1, 1, 12, 0, 0, 0, time.UTC)
	card := NewCard(at)
	for range 8 {
		var err error
		card, err = Schedule(card, 3, at)
		if err != nil {
			t.Fatal(err)
		}
		at = card.Due
	}
	unit := UnitVersion{ID: "foundation", Version: 1}
	event := Evidence{ID: "actual-long-card", MaterialID: "other", MaterialVersion: 1, At: card.LastReview, Kind: "review", Mode: "recall", Outcome: "correct", Rating: 3, ScheduleAfter: &card, DueAt: card.Due, Algorithm: Algorithm, Targets: []EvidenceTarget{{UnitID: unit.ID, UnitVersion: unit.Version, Role: "assesses"}}}
	input := DeferralInput{MaterialID: "probe", MaterialVersion: 1, Mode: "choice", Units: []UnitVersion{unit}, AsOf: event.At.Add(time.Minute), Evidence: []Evidence{event}}
	first := DeferPractice(input)
	want := event.At.Add(DeferralHorizon)
	if !first.Deferred || !first.ReconsiderAt.Equal(want) || first.ReconsiderAt.After(event.DueAt) {
		t.Fatalf("fixed original-event bound missing: %+v; actual due %s", first, event.DueAt)
	}
	input.AsOf = event.At.Add(6 * 24 * time.Hour)
	if later := DeferPractice(input); !later.Deferred || !later.ReconsiderAt.Equal(first.ReconsiderAt) {
		t.Fatalf("read slid deferral forward: first=%+v later=%+v", first, later)
	}
	input.AsOf = want
	if expired := DeferPractice(input); expired.Deferred {
		t.Fatalf("expired support was permanently deferred: %+v", expired)
	}
}

func TestPacingSeparatesPublicationPresentationAndActualAssessment(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	newQuiz := Candidate{ID: "new", Version: 1, Kind: "quiz", Mode: "recall", EstimatedSeconds: 30, DueAt: at, AvailableAt: at, FirstPresentedAt: at.Add(-time.Minute), Selected: true}
	pacing := Pacing{AsOf: at, AvailableSeconds: 300, NewAssessments: 5, NewAssessmentsToday: 5}
	if selected := SelectMaterial([]Candidate{newQuiz}, pacing); selected.MaterialID != "" {
		t.Fatalf("abandoned occurrence bypassed new assessment limit: %+v", selected)
	}
	oldQuiz := newQuiz
	oldQuiz.ID, oldQuiz.Assessed = "actually-attempted", true
	if selected := SelectMaterial([]Candidate{newQuiz, oldQuiz}, pacing); selected.MaterialID != oldQuiz.ID {
		t.Fatalf("new cap suppressed actual due retrieval: %+v", selected)
	}
	pacing.SpentSeconds = 280
	if selected := SelectMaterial([]Candidate{oldQuiz}, pacing); selected.MaterialID != "" {
		t.Fatalf("selected task exceeded explicit estimated budget: %+v", selected)
	}
	oldQuiz.PacingOverride = true
	if selected := SelectMaterial([]Candidate{oldQuiz}, pacing); selected.MaterialID != oldQuiz.ID {
		t.Fatalf("explicit undo of this pacing effect was ignored: %+v", selected)
	}
	oldQuiz.DueAt = at.Add(time.Hour)
	if selected := SelectMaterial([]Candidate{oldQuiz}, pacing); selected.MaterialID != "" {
		t.Fatalf("pacing undo changed direct FSRS eligibility: %+v", selected)
	}
}

func TestMixedSelectionUsesInstructionThenMatchingProbe(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	pacing := Pacing{AsOf: at, AvailableSeconds: 300, NewAssessments: 5}
	instruction := Candidate{ID: "instruction", Version: 1, Kind: "explanation", EstimatedSeconds: 60, AvailableAt: at, Selected: true}
	another := instruction
	another.ID = "other-instruction"
	probe := Candidate{ID: "probe", Version: 1, Kind: "quiz", Mode: "recall", Level: "foundation", EstimatedSeconds: 30, DueAt: at, AvailableAt: at, Selected: true}
	if selected := SelectMaterial([]Candidate{probe, instruction, another}, pacing); selected.MaterialID != instruction.ID {
		t.Fatalf("unknown beginner did not receive selected instruction: %+v", selected)
	}
	instruction.Continued, probe.ReadyProbe = true, true
	pacing.SpentSeconds = instruction.EstimatedSeconds
	if selected := SelectMaterial([]Candidate{instruction, another, probe}, pacing); selected.MaterialID != probe.ID {
		t.Fatalf("unread reference backlog starved the taught foundation's probe: %+v", selected)
	}
}
