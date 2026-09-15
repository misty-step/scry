package learning

import (
	"reflect"
	"testing"
	"time"
)

func inferenceSuccess(t *testing.T, id, mode string, at time.Time, units ...UnitVersion) Evidence {
	t.Helper()
	card, err := Schedule(NewCard(at), 3, at)
	if err != nil {
		t.Fatal(err)
	}
	e := Evidence{ID: id, MaterialID: "material-" + id, MaterialVersion: 1, At: at, Mode: mode, Kind: "review", Outcome: "correct", Rating: 3, ScheduleAfter: &card, DueAt: card.Due, Algorithm: Algorithm}
	for _, unit := range units {
		e.Targets = append(e.Targets, EvidenceTarget{UnitID: unit.ID, UnitVersion: unit.Version, Role: "assesses", CoverageID: "link-" + id + "-" + unit.ID, CoverageVersion: 1})
	}
	return e
}

func TestInferenceContextVersionsAndActualConditionalCard(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	unit := UnitVersion{ID: "reducing-power", Version: 2}
	choice := inferenceSuccess(t, "choice", "choice", at, unit)
	if estimate := Infer(unit, "recall", at, at, []Evidence{choice}, nil); estimate.State != "exposed" || estimate.RecallProbability != nil {
		t.Fatalf("recognition claimed uncued recall: %+v", estimate)
	}
	recall := inferenceSuccess(t, "recall", "recall", at.Add(time.Second), unit)
	estimate := Infer(unit, "choice", recall.At, recall.At.Add(time.Hour), []Evidence{choice, recall}, nil)
	if estimate.State != "demonstrated" || estimate.RecallProbability == nil || estimate.RecallEvidenceID != recall.ID || estimate.RecallCondition == "" {
		t.Fatalf("actual stronger-context card lost conditional provenance: %+v", estimate)
	}
	if estimate := Infer(UnitVersion{ID: unit.ID, Version: 3}, "recall", recall.At, recall.At, []Evidence{recall}, nil); estimate.State != "unknown" || estimate.RecallProbability != nil || len(estimate.EvidenceIDs) != 0 {
		t.Fatalf("old definition gained current-version credit: %+v", estimate)
	}
	if estimate := Infer(unit, "recall", recall.At, recall.At.Add(EvidenceFreshness), []Evidence{recall}, nil); estimate.State != "stale" {
		t.Fatalf("aged evidence became permanent mastery: %+v", estimate)
	}
	recall.ScheduleAfter = nil
	if estimate := Infer(unit, "recall", recall.At, recall.At, []Evidence{recall}, nil); estimate.RecallProbability != nil {
		t.Fatalf("probability invented without an actual card: %+v", estimate)
	}
}

func TestLatestAdverseAndLateCorrectionPrecedence(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	unit := UnitVersion{ID: "foundation", Version: 1}
	old := inferenceSuccess(t, "old", "recall", at, unit)
	middle := inferenceSuccess(t, "middle", "recall", at.Add(time.Minute), unit)
	old.Corrections = []Correction{{ID: "late-dispute", Kind: "dispute", Reason: "Expected answer is wrong", At: at.Add(2 * time.Minute)}}
	before := Infer(unit, "recall", middle.At, middle.At, []Evidence{old, middle}, nil)
	if before.State != "demonstrated" || len(before.CorrectionIDs) != 0 {
		t.Fatalf("future correction leaked into earlier derivation: %+v", before)
	}
	after := Infer(unit, "recall", at.Add(3*time.Minute), at.Add(3*time.Minute), []Evidence{old, middle}, nil)
	if after.State != "disputed" || after.RecallProbability != nil || !after.LatestAt.Equal(old.At) || !after.ContextAt.Equal(old.Corrections[0].At) {
		t.Fatalf("late dispute was hidden or rewrote observation time: %+v", after)
	}
	later := inferenceSuccess(t, "later-independent", "recall", at.Add(4*time.Minute), unit)
	if recovered := Infer(unit, "recall", later.At, later.At, []Evidence{later, old, middle}, nil); recovered.State != "demonstrated" || len(recovered.CorrectionIDs) != 1 {
		t.Fatalf("later independent success cannot recover with retained dispute provenance: %+v", recovered)
	}
	for _, adverse := range []Evidence{
		{ID: "failed", MaterialID: "probe", MaterialVersion: 1, At: later.At.Add(time.Second), Mode: "recall", Kind: "review", Outcome: "wrong", Rating: 1, Targets: later.Targets},
		{ID: "assisted", MaterialID: "probe", MaterialVersion: 1, At: later.At.Add(time.Second), Mode: "recall", Kind: "practice", Outcome: "correct", Assisted: true, Targets: later.Targets},
	} {
		estimate := Infer(unit, "recall", adverse.At, adverse.At, []Evidence{later, adverse}, nil)
		if estimate.State == "demonstrated" || estimate.RecallProbability != nil {
			t.Fatalf("adverse observation hidden behind prior success: %+v", estimate)
		}
	}
}

func TestOneObservationAcrossDiamondPathsAndDirectCoverage(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	base := UnitVersion{ID: "base", Version: 1}
	left, right, compound := UnitVersion{ID: "left", Version: 1}, UnitVersion{ID: "right", Version: 1}, UnitVersion{ID: "compound", Version: 1}
	relations := []Relation{
		{ID: "base-left", Version: 1, From: base, To: left, Kind: "prerequisite"},
		{ID: "base-right", Version: 2, From: base, To: right, Kind: "composition"},
		{ID: "left-compound", Version: 1, From: left, To: compound, Kind: "composition"},
		{ID: "right-compound", Version: 1, From: right, To: compound, Kind: "composition"},
		{ID: "cycle", Version: 1, From: right, To: base, Kind: "prerequisite"},
	}
	event := inferenceSuccess(t, "one-attempt", "recall", at, compound)
	estimate := Infer(base, "recall", at, at, []Evidence{event, event}, relations)
	if estimate.State != "supported" || len(estimate.EvidenceIDs) != 1 || len(estimate.Support) != 1 || estimate.RecallProbability != nil {
		t.Fatalf("graph paths manufactured independent evidence: %+v", estimate)
	}
	copyRelations := append([]Relation(nil), relations...)
	for i, j := 0, len(copyRelations)-1; i < j; i, j = i+1, j-1 {
		copyRelations[i], copyRelations[j] = copyRelations[j], copyRelations[i]
	}
	if reordered := Infer(base, "recall", at, at, []Evidence{event}, copyRelations); !reflect.DeepEqual(estimate, reordered) {
		t.Fatalf("ordering or duplicate input changed derivation:\n%+v\n%+v", estimate, reordered)
	}
	event.Targets = append(event.Targets, EvidenceTarget{UnitID: base.ID, UnitVersion: base.Version, Role: "assesses"})
	if direct := Infer(base, "recall", at, at, []Evidence{event}, relations); len(direct.Direct) != 1 || len(direct.Support) != 0 || len(direct.EvidenceIDs) != 1 {
		t.Fatalf("direct plus indirect paths double counted: %+v", direct)
	}
	event.Outcome, event.Rating = "wrong", 1
	if failure := Infer(base, "recall", at, at, []Evidence{event}, relations); failure.State != "ambiguous" {
		t.Fatalf("multi-target failure failed all components: %+v", failure)
	}
	for i := range relations {
		relations[i].Proposed = true
	}
	event.Targets = event.Targets[:1]
	if proposed := Infer(base, "recall", at, at, []Evidence{event}, relations); proposed.State != "unknown" {
		t.Fatalf("proposed graph became established evidence: %+v", proposed)
	}
}

func TestConflictingDuplicateCardsAndCorrectedSuccessFailClosed(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	unit := UnitVersion{ID: "unit", Version: 1}
	event := inferenceSuccess(t, "identity", "recall", at, unit)
	conflict := event
	card := *event.ScheduleAfter
	card.Stability *= 2
	conflict.ScheduleAfter = &card
	if estimate := Infer(unit, "recall", at, at, []Evidence{event, conflict}, nil); estimate.State != "ambiguous" || estimate.RecallProbability != nil || len(estimate.EvidenceIDs) != 1 {
		t.Fatalf("contradictory cards gained success credit: %+v", estimate)
	}
	event.Corrections = []Correction{{ID: "coverage-fix", Kind: "coverage", Reason: "This item did not assess the stated capability", At: at}}
	if estimate := Infer(unit, "recall", at, at, []Evidence{event}, nil); estimate.State != "ambiguous" || len(estimate.CorrectionIDs) != 1 {
		t.Fatalf("corrected assessment was silently converted to success: %+v", estimate)
	}
	instruction := Evidence{ID: "read", MaterialID: "explanation", MaterialVersion: 1, At: at, Kind: "continue", Outcome: "continued", Targets: []EvidenceTarget{{UnitID: unit.ID, UnitVersion: unit.Version, Role: "teaches"}}}
	if estimate := Infer(unit, "recall", at, at, []Evidence{instruction}, nil); estimate.State != "exposed" || len(estimate.Exposure) != 1 || estimate.RecallProbability != nil {
		t.Fatalf("reading manufactured recall: %+v", estimate)
	}
}
