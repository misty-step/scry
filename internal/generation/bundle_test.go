package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/store"
)

func knowledgeFixture(t *testing.T) (store.Job, store.GenerationResult) {
	t.Helper()
	bundle, err := decodeBundle([]byte(outputJSON(t, "concepts", topicDraft())))
	if err != nil {
		t.Fatal(err)
	}
	job := store.Job{
		Kind: "capture", SourceID: "source_energy", SourceKind: "topic", SourceText: "mitochondrial energy transfer", Attempts: 1,
		Context: store.KnowledgeContext{Version: store.KnowledgeContextVersion},
	}
	return job, bundle
}

func validateFixture(t *testing.T, job store.Job, bundle store.GenerationResult) (store.GenerationResult, []string, error) {
	t.Helper()
	return validateOutput(bundleJSON(t, bundle), &job, coveragePlan{Task: "infer", Units: []coverageUnit{}})
}

func TestBundleRejectsDanglingCoverageAndFabricatedIdentityAtomically(t *testing.T) {
	cases := []struct {
		name   string
		change func(*store.GenerationResult)
	}{
		{"unknown assessed unit", func(b *store.GenerationResult) { b.Quizzes[0].Links[0].UnitKey = "unseen" }},
		{"instruction posing as assessment", func(b *store.GenerationResult) { b.Materials[0].Links[0].Role = "assesses" }},
		{"duplicate assessed coverage", func(b *store.GenerationResult) {
			b.Quizzes[0].Links = append(b.Quizzes[0].Links, b.Quizzes[0].Links[0])
		}},
		{"arbitrary reused unit", func(b *store.GenerationResult) { b.Units[0].ReuseID = "not-in-context" }},
		{"arbitrary reused material", func(b *store.GenerationResult) { b.Materials[0].ReuseID = "not-in-context" }},
		{"unsupported assessment level", func(b *store.GenerationResult) { b.Quizzes[0].Level = "mastered" }},
		{"vague unit fragment", func(b *store.GenerationResult) { b.Units[0].Statement = "Understand energy" }},
		{"dangling suggestion", func(b *store.GenerationResult) {
			b.Suggestions = []store.GeneratedSuggestion{{Key: "next", Kind: "advance", Title: "Apply energy transfer", Reason: "Compare the represented energy mechanism with cellular work within this goal.", UnitKeys: []string{b.Units[0].Key}, MaterialKeys: []string{"missing"}}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			job, bundle := knowledgeFixture(t)
			test.change(&bundle)
			result, issues, err := validateFixture(t, job, bundle)
			if err == nil && len(issues) == 0 || len(result.Units)+len(result.Quizzes)+len(result.Materials) != 0 {
				t.Fatalf("invalid dependency was partially published: result=%+v issues=%v err=%v", result, issues, err)
			}
		})
	}
}

func TestRelationsRejectCyclesButKeepProposedComposition(t *testing.T) {
	job, bundle := knowledgeFixture(t)
	bundle.Units = append(bundle.Units, store.GeneratedUnit{Key: "unit_composition", Kind: "composition", Statement: "Explain how ATP transfer couples mitochondrial energy production to cellular work."})
	bundle.Relations = []store.GeneratedRelation{{From: bundle.Units[0].Key, To: "unit_composition", Kind: "composition", Evidence: "Proposed relationship: the chemical energy carrier supports explanation of coupled cellular work."}}
	accepted, issues, err := validateFixture(t, job, bundle)
	if err != nil || len(issues) != 0 || len(accepted.Relations) != 1 {
		t.Fatalf("useful proposed composition rejected: %v %v", issues, err)
	}
	bundle.Relations = append(bundle.Relations, store.GeneratedRelation{From: "unit_composition", To: bundle.Units[0].Key, Kind: "prerequisite", Evidence: "Proposed relationship: the integrated mechanism is needed before learning its energy carrier."})
	rejected, issues, err := validateFixture(t, job, bundle)
	if err == nil && len(issues) == 0 || len(rejected.Relations) != 0 {
		t.Fatalf("cyclic dependency accepted: %+v %v %v", rejected, issues, err)
	}
}

func referenceFixture(t *testing.T) (store.Job, store.GenerationResult) {
	t.Helper()
	job, bundle := knowledgeFixture(t)
	job.SourceText = "https://www.example.org/cellular-energy"
	bundle.Quizzes = []store.GeneratedQuiz{}
	bundle.Materials = []store.GeneratedMaterial{{Key: "article_energy", Kind: "article", Title: "Supplied energy reference", Basis: "reference", ReferenceURL: job.SourceText, EstimatedSeconds: 120, Links: []store.GeneratedLink{{UnitKey: bundle.Units[0].Key, Role: "teaches"}}}}
	return job, bundle
}

func TestActualReferenceRemainsUsefulWithoutInventedContent(t *testing.T) {
	job, bundle := referenceFixture(t)
	result, issues, err := validateFixture(t, job, bundle)
	if err != nil || len(issues) != 0 || len(result.Materials) != 1 || len(result.Quizzes) != 0 || result.Coverage.Complete || len(result.Coverage.Missing) == 0 || result.Materials[0].Body != "" {
		t.Fatalf("supplied reference became content or fake completion: %+v %v %v", result, issues, err)
	}
	for _, badURL := range []string{"https://www.example.org/invented", "javascript:alert(1)", "https://user:secret@www.example.org/cellular-energy", "https://localhost/private"} {
		bundle.Materials[0].ReferenceURL = badURL
		job.Context.References = []string{badURL} // Even supplied unsafe URLs are not usable resources.
		if badURL == "https://www.example.org/invented" {
			job.Context.References = nil
		}
		rejected, issues, err := validateFixture(t, job, bundle)
		if err == nil && len(issues) == 0 || len(rejected.Materials) != 0 {
			t.Fatalf("unsafe/invented reference accepted: %s %+v %v", badURL, rejected, err)
		}
	}
	job, bundle = referenceFixture(t)
	bundle.Materials[0].Body = "The article proves that the learner has mastered all relevant cellular processes."
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("a URL became a fetched article: %+v %v %v", result, issues, err)
	}
}

func TestSuppliedVideoSegmentCannotInventOrWidenItsBounds(t *testing.T) {
	job, bundle := referenceFixture(t)
	job.SourceText = "https://www.example.org/video#t=10,90"
	material := &bundle.Materials[0]
	material.Kind, material.ReferenceURL, material.StartSeconds, material.EndSeconds = "video", job.SourceText, 10, 90
	if result, issues, err := validateFixture(t, job, bundle); err != nil || len(issues) != 0 || len(result.Materials) != 1 {
		t.Fatalf("explicit supplied segment rejected: %+v %v %v", result, issues, err)
	}
	material.EndSeconds = 120
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("unseen video segment fabricated: %+v %v %v", result, issues, err)
	}
	material.StartSeconds, material.EndSeconds = 90, 10
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("reversed video bounds accepted: %+v %v %v", result, issues, err)
	}
}

func TestDiagramRequiresSafeConnectedTopologyAndTextualEquivalent(t *testing.T) {
	job, bundle := knowledgeFixture(t)
	material := &bundle.Materials[0]
	material.Kind, material.Title = "diagram", "Cellular energy transfer"
	material.Body = "Mitochondria produce ATP. ATP supplies energy for cellular work rather than storing genetic information."
	material.Diagram = &store.Diagram{Nodes: []store.DiagramNode{{ID: "mito", Label: "Mitochondria"}, {ID: "atp", Label: "ATP"}}, Edges: []store.DiagramEdge{{From: "mito", To: "atp", Label: "produce"}}, Caption: "An energy-producing relationship, not a complete cellular pathway."}
	if result, issues, err := validateFixture(t, job, bundle); err != nil || len(issues) != 0 || len(result.Materials) != 1 {
		t.Fatalf("structured diagram rejected: %+v %v %v", result, issues, err)
	}
	material.Diagram.Edges[0].To = "not-a-node"
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("dangling diagram accepted: %+v %v %v", result, issues, err)
	}
	material.Diagram.Edges[0].To = "atp"
	material.Diagram.Nodes = append(material.Diagram.Nodes, store.DiagramNode{ID: "work", Label: "cellular work"})
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("unconnected diagram node accepted: %+v %v %v", result, issues, err)
	}
	material.Diagram.Nodes = material.Diagram.Nodes[:2]
	material.Diagram.Nodes[0].Label = "<svg onload=alert(1)>"
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("raw diagram markup accepted: %+v %v %v", result, issues, err)
	}
}

func TestBackgroundInstructionCannotLaunderSourceTargetEvidence(t *testing.T) {
	job, bundle := knowledgeFixture(t)
	job.SourceKind, job.SourceText = "source", "The Calvin cycle uses ATP and NADPH to support carbon fixation."
	bundle.Quizzes[0].Basis, bundle.Quizzes[0].Level = "background", "foundation"
	bundle.Quizzes[0].Explanation = "Generated background: " + bundle.Quizzes[0].Explanation
	bundle.Materials[0].Basis = "background"
	bundle.Materials[0].Body = "Generated background: " + bundle.Materials[0].Body
	if result, issues, err := validateFixture(t, job, bundle); err != nil || len(issues) != 0 || len(result.Materials) != 1 {
		t.Fatalf("honest prerequisite background rejected: %+v %v %v", result, issues, err)
	}
	bundle.Quizzes[0].Level = "target"
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Quizzes) != 0 {
		t.Fatalf("target bypassed exact source evidence: %+v %v %v", result, issues, err)
	}
}

func bridgeFixture(t *testing.T) (store.Job, store.GenerationResult) {
	t.Helper()
	job, bundle := knowledgeFixture(t)
	job.Kind, job.TargetMaterialID, job.TargetMaterialVersion, job.TargetPresentationID = "bridge", "material_target", 2, "presentation_target"
	unit := store.KnowledgeUnit{ID: "known_target", Version: 3, Kind: "composition", Statement: "Explain how mitochondrial ATP generation supports chemical work throughout a cell."}
	job.Context.Units = []store.KnowledgeUnit{unit}
	job.Context.Materials = []store.Material{{ID: job.TargetMaterialID, SourceID: job.SourceID, Version: 2, Kind: "quiz", Level: "target", EstimatedSeconds: 45,
		Quiz:  &store.Quiz{ID: "target_quiz", Kind: "recall", Prompt: "Which organelle houses oxidative phosphorylation reactions in aerobic cells?", Answer: "mitochondria", Explanation: "Mitochondria couple substrate oxidation to ATP generation through oxidative phosphorylation.", Basis: "topic", Choices: []string{}, Variants: []string{}},
		Links: []store.CoverageLink{{UnitID: unit.ID, UnitVersion: unit.Version, Role: "assesses"}},
	}}
	bundle.Units = append(bundle.Units, store.GeneratedUnit{Key: "target_unit", ReuseID: unit.ID, Statement: unit.Statement, Kind: unit.Kind})
	bundle.Relations = []store.GeneratedRelation{{From: bundle.Units[0].Key, To: "target_unit", Kind: "composition", Evidence: "Proposed relationship: chemical energy transfer is necessary to explain cellular energy coupling."}}
	bundle.Quizzes[0].Level = "foundation"
	return job, bundle
}

func observedGapFixture(t *testing.T) (store.Job, store.GenerationResult) {
	t.Helper()
	job, bundle := bridgeFixture(t)
	job.Kind, job.ObservationID = "enrich", "review_trigger"
	job.GoalID, job.GoalRevision = "chosen_energy_goal", 4
	job.SourceRevision = 3
	job.Context.GoalID, job.Context.GoalRevision = job.GoalID, job.GoalRevision
	job.TargetPresentationVersion = job.TargetMaterialVersion
	// The selected goal may reuse a quiz from another source; its quiz version
	// is independent of the material version that pins coverage.
	job.Context.Materials[0].SourceID = "another_source"
	job.Context.Materials[0].Quiz.Version = 1
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	card, err := learning.Schedule(learning.NewCard(at), 1, at)
	if err != nil {
		t.Fatal(err)
	}
	job.Context.AsOf = at.Add(time.Hour)
	job.Context.Evidence = []learning.Evidence{{
		ID: job.ObservationID, MaterialID: job.TargetMaterialID, MaterialVersion: job.TargetMaterialVersion,
		At: at, Kind: "review", Mode: "recall", Outcome: "wrong", Rating: 1, Ambiguous: true,
		Targets:       []learning.EvidenceTarget{{UnitID: "known_target", UnitVersion: 3, Role: "assesses", CoverageID: "target_coverage", CoverageVersion: job.TargetMaterialVersion}},
		ScheduleAfter: &card, Algorithm: learning.Algorithm, DueAt: at.Add(time.Minute),
		CorrectionIDs: []string{}, Corrections: []learning.Correction{},
	}}
	return job, bundle
}

func TestBridgeAndObservedGapRequireInstructionPracticeAndConnectionToRetainedTarget(t *testing.T) {
	for _, kind := range []string{"bridge", "observed_gap"} {
		t.Run(kind, func(t *testing.T) {
			fixture := bridgeFixture
			if kind == "observed_gap" {
				fixture = observedGapFixture
			}
			job, bundle := fixture(t)
			if result, issues, err := validateFixture(t, job, bundle); err != nil || len(issues) != 0 || len(result.Materials) != 1 || len(result.Quizzes) != 1 {
				t.Fatalf("useful retained-target support rejected: %+v %v %v", result, issues, err)
			}
			bundle.Quizzes[0].Level = "extension"
			if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Quizzes) != 0 {
				t.Fatalf("another advanced question passed as target support: %+v %v %v", result, issues, err)
			}
			bundle.Quizzes[0].Level = "foundation"
			bundle.Relations[0].Kind = "contrast"
			if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Quizzes) != 0 {
				t.Fatalf("unrelated instruction passed as target support: %+v %v %v", result, issues, err)
			}
			job, bundle = fixture(t)
			bundle.Materials = []store.GeneratedMaterial{}
			if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Quizzes) != 0 {
				t.Fatalf("quiz-only target support accepted: %+v %v %v", result, issues, err)
			}
		})
	}
}

func TestReuseOnlyEnrichmentRejectsRewritingPreservedContent(t *testing.T) {
	job, bundle := knowledgeFixture(t)
	job.Kind = "enrich"
	quiz := bundle.Quizzes[0]
	job.Context.Materials = []store.Material{{ID: "old_material", SourceID: job.SourceID, Version: 1, Kind: "quiz", Unmapped: true, Level: quiz.Level, EstimatedSeconds: quiz.EstimatedSeconds,
		Quiz: &store.Quiz{ID: "old_quiz", Version: 1, Kind: quiz.Kind, Prompt: quiz.Prompt, Answer: quiz.Answer, Explanation: quiz.Explanation, Evidence: quiz.Evidence, Basis: quiz.Basis, Choices: quiz.Choices, Variants: quiz.Variants},
	}}
	bundle.Materials = []store.GeneratedMaterial{}
	bundle.Quizzes[0].ReuseID = "old_material"
	if result, issues, err := validateFixture(t, job, bundle); err != nil || len(issues) != 0 || len(result.Quizzes) != 1 || !result.Coverage.Complete {
		t.Fatalf("explicit reuse-only mapping rejected: %+v %v %v", result, issues, err)
	}
	bundle.Quizzes[0].Answer = "NADPH"
	if result, issues, err := validateFixture(t, job, bundle); err == nil && len(issues) == 0 || len(result.Quizzes) != 0 {
		t.Fatalf("old assessment was silently rewritten: %+v %v %v", result, issues, err)
	}
}

func TestObservedGapReusesFoundationsAndMapsOnlyItsUnmappedTarget(t *testing.T) {
	job, bundle := observedGapFixture(t)
	target := &job.Context.Materials[0]
	target.Unmapped, target.Links = true, nil
	job.Context.Evidence[0].Unmapped, job.Context.Evidence[0].Targets = true, nil
	bundle.Units[1].ReuseID = ""
	bundle.Units[0].ReuseID = "existing_foundation"
	job.Context.Units = []store.KnowledgeUnit{{ID: bundle.Units[0].ReuseID, Version: 1, Kind: bundle.Units[0].Kind, Statement: bundle.Units[0].Statement}}
	bundle.Materials[0].ReuseID, bundle.Quizzes[0].ReuseID = "existing_instruction", "existing_practice"
	material, practice := bundle.Materials[0], bundle.Quizzes[0]
	targetQuiz := target.Quiz
	bundle.Quizzes = append(bundle.Quizzes, store.GeneratedQuiz{
		Key: "retained_target", ReuseID: target.ID, Level: target.Level, EstimatedSeconds: target.EstimatedSeconds,
		Kind: targetQuiz.Kind, Prompt: targetQuiz.Prompt, Answer: targetQuiz.Answer, Explanation: targetQuiz.Explanation,
		Basis: targetQuiz.Basis, Evidence: targetQuiz.Evidence, Choices: targetQuiz.Choices, Variants: targetQuiz.Variants,
		Links: []store.GeneratedLink{{UnitKey: bundle.Units[1].Key, Role: "assesses"}},
	})
	job.Context.Materials = append(job.Context.Materials,
		store.Material{ID: material.ReuseID, SourceID: "instruction_origin", Version: 1, Kind: material.Kind, Title: material.Title, Body: material.Body, Basis: material.Basis, Evidence: material.Evidence, EstimatedSeconds: material.EstimatedSeconds},
		store.Material{ID: practice.ReuseID, SourceID: "practice_origin", Version: 1, Kind: "quiz", Level: practice.Level, EstimatedSeconds: practice.EstimatedSeconds,
			Quiz: &store.Quiz{ID: "practice_quiz", Version: 1, Kind: practice.Kind, Prompt: practice.Prompt, Answer: practice.Answer, Explanation: practice.Explanation, Basis: practice.Basis, Evidence: practice.Evidence, Choices: practice.Choices, Variants: practice.Variants}},
		store.Material{ID: "unrelated_unmapped", SourceID: job.SourceID, Version: 1, Kind: "quiz", Unmapped: true},
	)
	job.Context.Omitted.Materials = 2
	before, err := json.Marshal(job.Context.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareJob(&job)
	if err != nil {
		t.Fatal(err)
	}
	result, issues, err := validateFixture(t, prepared, bundle)
	if err != nil || len(issues) != 0 || !result.Coverage.Complete || len(result.Materials) != 1 || len(result.Quizzes) != 2 || result.Materials[0].ReuseID != material.ReuseID || result.Quizzes[0].ReuseID != practice.ReuseID || result.Quizzes[1].ReuseID != job.TargetMaterialID {
		t.Fatalf("bounded existing support or exact unmapped target was replaced: %+v %v %v", result, issues, err)
	}
	after, err := json.Marshal(job.Context.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !prepared.Context.Evidence[0].Unmapped || len(prepared.Context.Evidence[0].Targets) != 0 {
		t.Fatal("proposed target mapping rewrote the original unmapped observation")
	}
}

func TestContextSelectionMakesOmissionsExplicitAndFencesUnseenReuse(t *testing.T) {
	job, bundle := knowledgeFixture(t)
	job.Context.Omitted.Materials = 2
	for index := range 25 {
		job.Context.Materials = append(job.Context.Materials, store.Material{ID: fmt.Sprintf("material_%02d", index), SourceID: job.SourceID, Version: 1, Kind: "explanation", Title: fmt.Sprintf("Energy explanation %d", index), Body: strings.Repeat("A bounded complete sentence about chemical energy transfer. ", 12), Basis: "topic", EstimatedSeconds: 60})
	}
	prepared, err := prepareJob(&job)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Context.Materials)+prepared.Context.Omitted.Materials != 27 || prepared.Context.Omitted.Materials <= 2 || len(job.Context.Materials) != 25 {
		t.Fatalf("context selection lost or mutated its omission accounting: %+v", prepared.Context.Omitted)
	}
	bundle.Materials[0].ReuseID = "material_24"
	if result, issues, err := validateFixture(t, prepared, bundle); err == nil && len(issues) == 0 || len(result.Materials) != 0 {
		t.Fatalf("omitted ID remained reusable: %+v %v %v", result, issues, err)
	}
	job.Kind, job.TargetMaterialID, job.TargetMaterialVersion, job.TargetPresentationID = "bridge", "material_24", 2, "target_presentation"
	if _, err := prepareJob(&job); err == nil {
		t.Fatal("superseded target version was sent to the provider")
	}
}

func TestReferenceOnlyWorkerCompletionRecordsActualSpend(t *testing.T) {
	s := generationStore(t)
	_, bundle := referenceFixture(t)
	source, job := captureAndClaim(t, s, bundle.Materials[0].ReferenceURL, 100_000)
	server := responseServer(t, envelopeJSON(t, bundleJSON(t, bundle), "stop", json.RawMessage(`0.0000071`)), http.StatusOK)
	worker := New(s, localConfig(server.URL))
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Job == nil || saved.Job.Status != "partial" || saved.Job.CostMicros != 8 || saved.Job.CostUnknown || len(saved.Quizzes) != 0 || saved.Job.Published != 1 {
		t.Fatalf("useful reference-only result was lost or billed dishonestly: %+v", saved)
	}
}

func TestExpandWithoutAcceptedScopeIsRejectedBeforePaidTransmission(t *testing.T) {
	job, _ := knowledgeFixture(t)
	job.Kind = "expand"
	worker := New(nil, localConfig("http://127.0.0.1:1/completions"))
	result, cost, failure := worker.generate(context.Background(), &job)
	if failure == nil || failure.retry || cost == nil || *cost != 0 || len(result.Materials) != 0 {
		t.Fatalf("unsolicited expansion attempted paid work: %+v %v %+v", result, cost, failure)
	}
}

func TestBundleMandatoryFieldsRejectNullArraysAndNestedUnknownKeys(t *testing.T) {
	job, _ := knowledgeFixture(t)
	valid := outputJSON(t, "concepts", topicDraft())
	for _, malformed := range []string{
		strings.Replace(valid, `"relations":[]`, `"relations":null`, 1),
		strings.Replace(valid, `"diagram":null,`, ``, 1),
		strings.Replace(valid, `"role":"assesses"`, `"role":"assesses","confidence":1`, 1),
		strings.Replace(valid, `"kind":"foundation"`, `"kind":"foundation","kind":"concept"`, 1),
	} {
		if result, _, err := validateOutput(malformed, &job, coveragePlan{Task: "infer"}); err == nil || len(result.Units) != 0 {
			t.Fatalf("ambiguous bundle shape accepted: %+v %v", result, err)
		}
	}
}

func TestScopedJobKindsAcceptUsableBoundedProviderBundles(t *testing.T) {
	bridgeJob, bridgeBundle := bridgeFixture(t)
	enrichJob, enrichBundle := knowledgeFixture(t)
	enrichJob.Kind = "enrich"
	// A modeling-only gap can reuse an existing resource without inventing a
	// new assessment. Its old content is supplied, not reconstructed by a model.
	existing := enrichBundle.Materials[0]
	enrichJob.Context.Materials = []store.Material{{ID: "existing_instruction", SourceID: enrichJob.SourceID, Version: 1, Kind: existing.Kind, Title: existing.Title, Body: existing.Body, Basis: existing.Basis, Evidence: existing.Evidence, EstimatedSeconds: existing.EstimatedSeconds}}
	enrichBundle.Materials[0].ReuseID = "existing_instruction"
	enrichBundle.Quizzes = []store.GeneratedQuiz{}
	expandJob, expandBundle := referenceFixture(t)
	expandJob.Kind = "expand"
	expandJob.Context.Request = "Add the supplied energy reference as a lateral comparison within the chosen cellular-energy goal."
	for _, test := range []struct {
		job    store.Job
		bundle store.GenerationResult
	}{
		{bridgeJob, bridgeBundle},
		{enrichJob, enrichBundle},
		{expandJob, expandBundle},
	} {
		t.Run(test.job.Kind, func(t *testing.T) {
			server := responseServer(t, envelopeJSON(t, bundleJSON(t, test.bundle), "stop", json.RawMessage(`0.000031`)), http.StatusOK)
			worker := New(nil, localConfig(server.URL))
			result, cost, failure := worker.generate(context.Background(), &test.job)
			if failure != nil || cost == nil || *cost != 31 {
				t.Fatalf("authorized scoped work did not settle with its paid receipt: %+v %v %+v", result, cost, failure)
			}
			if test.job.Kind == "bridge" && (len(result.Materials) != 1 || len(result.Quizzes) != 1) {
				t.Fatalf("bridge omitted instruction or appropriate practice: %+v", result)
			}
			if test.job.Kind == "enrich" && (len(result.Quizzes) != 0 || len(result.Materials) != 1 || result.Materials[0].ReuseID != "existing_instruction") {
				t.Fatalf("reuse-only enrichment invented an assessment: %+v", result)
			}
			if test.job.Kind == "expand" && (result.Coverage.Complete || len(result.Quizzes) != 0 || len(result.Materials) != 1 || result.Materials[0].Body != "") {
				t.Fatalf("reference-only expansion claimed unseen article content: %+v", result)
			}
		})
	}
}

func TestDistinctSavedVideoSegmentsRetainSeparateMaterialIdentity(t *testing.T) {
	job, bundle := referenceFixture(t)
	first := bundle.Materials[0]
	first.Key, first.ReuseID, first.Kind, first.StartSeconds, first.EndSeconds = "segment_one", "saved_one", "video", 10, 20
	second := first
	second.Key, second.ReuseID, second.StartSeconds, second.EndSeconds = "segment_two", "saved_two", 30, 40
	bundle.Materials = []store.GeneratedMaterial{first, second}
	for _, material := range bundle.Materials {
		job.Context.Materials = append(job.Context.Materials, store.Material{ID: material.ReuseID, Version: 1, Kind: material.Kind, Title: material.Title, Basis: material.Basis, ReferenceURL: material.ReferenceURL, StartSeconds: material.StartSeconds, EndSeconds: material.EndSeconds, EstimatedSeconds: material.EstimatedSeconds})
	}
	result, issues, err := validateFixture(t, job, bundle)
	if err != nil || len(issues) != 0 || len(result.Materials) != 2 || result.Materials[0].StartSeconds == result.Materials[1].StartSeconds {
		t.Fatalf("distinct actual clips were collapsed as duplicate references: %+v %v %v", result, issues, err)
	}
}

func TestExactTextRetainsSuppliedNumericReferenceMarkers(t *testing.T) {
	line := "ATP carries chemical energy [1]"
	job := store.Job{Kind: "capture", SourceKind: "source", SourceText: "Recite this exact text:\n" + line}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := store.GeneratedQuiz{Kind: "recall", Basis: "source", Evidence: line, Prompt: "Recite the supplied line about cellular energy.", Answer: line, Explanation: "The supplied line identifies ATP as a chemical energy carrier, retaining its exact reference marker.", Choices: []string{}, Variants: []string{}, Links: []store.GeneratedLink{{UnitKey: "u1", Role: "assesses"}}}
	result, issues, err := validateOutput(outputJSON(t, "exact_text", quiz), &job, plan)
	if err != nil || len(issues) != 0 || !result.Coverage.Complete || len(result.Quizzes) != 1 || result.Quizzes[0].Answer != line {
		t.Fatalf("a real source marker was removed or treated as an invented citation: %+v %v %v", result, issues, err)
	}
}
