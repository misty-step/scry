package store

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

// This fixture uses the publication boundary rather than manufacturing review
// events or retroactively adding links to historical quiz snapshots.
func knowledgeFixture(t *testing.T, s *Store, quizzes int, instruction bool, suggestions int) Goal {
	t.Helper()
	ctx := context.Background()
	src, err := s.Capture(ctx, "Synthetic knowledge policy fixture", newID())
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 100, 100000)
	if err != nil || job == nil {
		t.Fatalf("claim fixture: job=%+v err=%v", job, err)
	}
	bundle := GenerationResult{Model: "authored-policy-fixture", PromptVersion: "knowledge-fixture-v1", Coverage: CoverageReport{Kind: "concepts", Complete: false, Missing: []string{"Extension resources intentionally not yet authored"}}, Units: []GeneratedUnit{{Key: "foundation", Statement: "The second synthetic choice is the answer.", Kind: "foundation"}, {Key: "extension", Statement: "Apply the synthetic distinction in a new setting.", Kind: "composition"}}, Relations: []GeneratedRelation{{From: "foundation", To: "extension", Kind: "composition", Evidence: "Proposed relationship: the synthetic distinction is a component of its transfer task."}}}
	for i := range quizzes {
		q := authoredChoice(fmt.Sprintf("Synthetic foundation probe %d", i))
		q.Key, q.Level, q.EstimatedSeconds = fmt.Sprintf("probe%d", i), "foundation", 30
		unitKey := "foundation"
		if i > 0 {
			unitKey = fmt.Sprintf("foundation%d", i)
			bundle.Units = append(bundle.Units, GeneratedUnit{Key: unitKey, Statement: fmt.Sprintf("Synthetic distinction %d has its own independent scope.", i), Kind: "foundation"})
		}
		q.Links = []GeneratedLink{{UnitKey: unitKey, Role: "assesses"}}
		bundle.Quizzes = append(bundle.Quizzes, q)
	}
	if instruction {
		bundle.Materials = []GeneratedMaterial{{Key: "instruction", Kind: "explanation", Title: "Synthetic foundation explanation", Body: "The second choice represents the fixture distinction.", Basis: "topic", EstimatedSeconds: 60, Links: []GeneratedLink{{UnitKey: "foundation", Role: "teaches"}}}}
	}
	for i := range suggestions {
		bundle.Suggestions = append(bundle.Suggestions, GeneratedSuggestion{Key: fmt.Sprintf("option%d", i), Kind: "advance", Title: fmt.Sprintf("Synthetic transfer option %d", i), Reason: "Extend the chosen synthetic goal into an application task.", UnitKeys: []string{"extension"}})
	}
	cost := int64(70)
	if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, bundle, &cost); err != nil {
		t.Fatal(err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Goal(ctx, src.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestKnowledgeEvidencePinsEncounteredCoverageAndKeepsOriginalHistory(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, false, 0)
	var foundation, extension KnowledgeUnit
	for _, unit := range g.Units {
		if unit.Kind == "foundation" {
			foundation = unit
		} else {
			extension = unit
		}
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil {
		t.Fatalf("review fixture: %+v %v", state, err)
	}
	*now = now.Add(time.Second)
	saved, err := s.Submit(ctx, state.Current.ID, newID(), "second", false)
	if err != nil {
		t.Fatal(err)
	}
	var originalSnapshot, originalCard string
	if err = s.db.QueryRow("SELECT snapshot,schedule_after FROM review_events WHERE id=?", saved.ReviewID).Scan(&originalSnapshot, &originalCard); err != nil {
		t.Fatal(err)
	}
	m, err := s.Material(ctx, state.Current.Material.ID)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	if _, err = s.EditCoverage(ctx, m.ID, m.Version, []CoverageLink{{UnitID: extension.ID, UnitVersion: extension.Version, Role: "assesses"}}, "The former coverage overclaimed the assessment scope", newID()); err != nil {
		t.Fatal(err)
	}
	oldEstimate, err := s.Unit(ctx, foundation.ID, "choice", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if oldEstimate.Estimate.State != "ambiguous" || len(oldEstimate.Estimate.Direct) != 1 || oldEstimate.Estimate.Direct[0].EvidenceID != saved.ReviewID || oldEstimate.Estimate.Direct[0].MaterialVersion != m.Version {
		t.Fatalf("original evidence lost pinned corrected scope: %+v", oldEstimate.Estimate)
	}
	if len(oldEstimate.Materials) != 1 {
		t.Fatalf("linked immutable material version was lost: %+v", oldEstimate.Materials)
	}
	linked := oldEstimate.Materials[0]
	if linked.ID != m.ID || linked.Version != m.Version || !linked.MetadataOnly || linked.Quiz != nil || linked.Body != "" || linked.Evidence != "" || linked.Provenance.Evidence != "" {
		t.Fatalf("unit listing disclosed material content or changed version identity: %+v", linked)
	}
	if len(linked.Links) != 1 || linked.Links[0].ID == "" || linked.Links[0].Role != "assesses" || linked.Links[0].UnitID != foundation.ID || linked.Links[0].UnitVersion != foundation.Version {
		t.Fatalf("metadata listing lost pinned coverage provenance: %+v", linked.Links)
	}
	newEstimate, err := s.Unit(ctx, extension.ID, "choice", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if newEstimate.Estimate.State != "unknown" || len(newEstimate.Estimate.EvidenceIDs) != 0 {
		t.Fatalf("latest coverage manufactured historical credit: %+v", newEstimate.Estimate)
	}
	var snapshot, card string
	if err = s.db.QueryRow("SELECT snapshot,schedule_after FROM review_events WHERE id=?", saved.ReviewID).Scan(&snapshot, &card); err != nil {
		t.Fatal(err)
	}
	if snapshot != originalSnapshot || card != originalCard {
		t.Fatal("content correction rewrote actual review or FSRS history")
	}
	*now = now.Add(time.Second)
	if err = s.Dispute(ctx, saved.ReviewID, "The original answer was misleading", false); err != nil {
		t.Fatal(err)
	}
	disputed, err := s.Unit(ctx, foundation.ID, "choice", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if disputed.Estimate.State != "disputed" || disputed.Estimate.RecallProbability != nil || len(disputed.Estimate.Direct[0].Corrections) < 2 {
		t.Fatalf("dispute or correction reason disappeared: %+v", disputed.Estimate)
	}
}

func TestKnowledgeReadsDoNotRecordInspectionOrChangeExport(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, true, 0)
	before, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = s.Unit(ctx, g.Units[0].ID, "recall", time.Time{}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Goal(ctx, g.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Material(ctx, g.Materials[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only derivation or library browsing changed durable export")
	}
	var interactions, inspections int
	if err = s.db.QueryRow("SELECT (SELECT count(*) FROM interactions),(SELECT count(*) FROM inspection_events)").Scan(&interactions, &inspections); err != nil {
		t.Fatal(err)
	}
	if interactions != 0 || inspections != 0 {
		t.Fatalf("library availability invented observation: interactions=%d inspections=%d", interactions, inspections)
	}
}

func TestReferenceDeliveryIsNotReadingAndContinueIsOneExposure(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, true, 0)
	var foundation KnowledgeUnit
	for _, unit := range g.Units {
		if unit.Kind == "foundation" {
			foundation = unit
		}
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "reference" {
		t.Fatalf("reference not selected: %+v %v", state, err)
	}
	delivery, err := s.Unit(ctx, foundation.ID, "recall", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(delivery.Estimate.EvidenceIDs) != 0 {
		t.Fatalf("delivery grant fabricated reading: %+v", delivery.Estimate)
	}
	*now = now.Add(time.Second)
	op := newID()
	continued, err := s.ContinueMaterial(ctx, state.Current.ID, op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ContinueMaterial(ctx, state.Current.ID, op); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM interactions WHERE presentation_id=? AND kind='continue'", state.Current.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || continued.Current == nil || continued.Current.Kind != "quiz" || !continued.Current.Practice {
		t.Fatalf("Continue did not produce one real exposure and an assisted probe: count=%d state=%+v", count, continued)
	}
	*now = now.Add(time.Second)
	if _, err = s.Submit(ctx, continued.Current.ID, newID(), "second", false); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM review_events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("post-instruction practice manufactured a direct FSRS review")
	}
	exposed, err := s.Unit(ctx, foundation.ID, "recall", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if exposed.Estimate.State == "demonstrated" || exposed.Estimate.RecallProbability != nil {
		t.Fatalf("assisted practice certified cold recall: %+v", exposed.Estimate)
	}
}

func TestGenerationEvidenceBoundRetainsActualIDsAndExactOmissions(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, false, 0)
	pin := learning.UnitVersion{ID: g.Materials[0].Links[0].UnitID, Version: g.Materials[0].Links[0].UnitVersion}
	var ids []string
	for range 5 {
		state, err := s.Review(ctx)
		if err != nil || state.Current == nil {
			t.Fatalf("review fixture: %+v %v", state, err)
		}
		saved, err := s.Submit(ctx, state.Current.ID, newID(), "second", false)
		if err != nil {
			t.Fatal(err)
		}
		if saved.ReviewID == "" || saved.Rating != 3 {
			t.Fatalf("fixture did not produce an actual cold review: %+v", saved)
		}
		ids = append(ids, saved.ReviewID)
		if _, err = s.Next(ctx, state.Current.ID); err != nil {
			t.Fatal(err)
		}
		*now = time.UnixMilli(saved.DueAt).Add(31 * time.Minute)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	evidence, omitted, err := knowledgeContextEvidence(ctx, tx, []learning.UnitVersion{pin}, now.UnixMilli(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if omitted != 3 || len(evidence) != 2 || evidence[0].ID != ids[3] || evidence[1].ID != ids[4] {
		t.Fatalf("bounded context lost original identities or hid omissions: omitted=%d evidence=%+v", omitted, evidence)
	}
	for _, event := range evidence {
		if len(event.Targets) != 1 || event.Targets[0].UnitID != pin.ID || event.Targets[0].UnitVersion != pin.Version || event.ScheduleAfter == nil {
			t.Fatalf("bounded context reconstructed a different assessment: %+v", event)
		}
	}
}
