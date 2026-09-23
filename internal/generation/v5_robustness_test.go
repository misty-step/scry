package generation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

func modelJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestV5PartialPlanPublishesOneWordConceptAndShortGoal(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	source, err := s.Capture(ctx, store.CaptureInput{Mode: "topic", Text: "TLS certificate trust"}, "robust-partial-plan")
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	research, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || research == nil || research.Kind != "research" {
		t.Fatalf("research claim: %+v %v", research, err)
	}
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Model: "unavailable", PromptVersion: "scry-research-v1", Note: "Web search was unavailable."}, &zero); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || job == nil || job.Kind != "plan" {
		t.Fatalf("plan claim: %+v %v", job, err)
	}
	plan := store.PlanContent{Goal: strings.Repeat("Learn certificate trust and its careful decisions ", 7), Concepts: []store.PlannedConcept{
		{Key: "c1", Name: "TLS", Summary: "Trust comes from a verified certificate chain.", Requires: []string{"c2"}, Note: &store.NoteContent{Level: "standard", Title: "Certificate trust", Body: standardNoteBody, Basis: "topic"}},
		{Key: "c2", Name: "Broken note", Summary: "This lacks an explanation.", Note: &store.NoteContent{Level: "standard", Title: "Incomplete", Body: "Short", Basis: "topic"}},
	}}
	result, err := validateV5Output(job, store.JobContext{}, modelJSON(t, plan))
	if err != nil || !result.Partial || len(result.Plan.Concepts) != 1 || result.Plan.Concepts[0].Name != "TLS" || len(result.Plan.Concepts[0].Requires) != 0 || !strings.Contains(result.Note, "invalid note content") || utf8.RuneCountInString(result.Plan.Goal) > 120 || !strings.HasSuffix(result.Plan.Goal, "…") {
		t.Fatalf("plan was not safely filtered and shortened: %+v %v", result, err)
	}
	result.Model, result.PromptVersion = "fixture", "scry-plan-v1"
	if err := s.CompleteJob(ctx, job.ID, job.LeaseToken, result, &zero); err != nil {
		t.Fatalf("filtered plan rejected at publication: %v", err)
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || saved.Jobs[1].Status != "partial" || saved.Jobs[1].Published != 1 {
		t.Fatalf("partial plan was not published: %+v %v", saved, err)
	}
}

func TestV5PlanKeepsWellFormedConceptBesideMalformedOne(t *testing.T) {
	job := &store.Job{Kind: "plan", SourceKind: "topic", SourceMode: "topic", SourceText: "TLS trust"}
	good := store.PlannedConcept{Key: "c1", Name: "TLS", Summary: "Certificate trust requires a name check.", Note: &store.NoteContent{Level: "standard", Title: "Certificate checks", Body: standardNoteBody, Basis: "topic"}}
	content := modelJSON(t, map[string]any{"goal": "Understand TLS", "concepts": []any{good, map[string]any{"key": "bad", "name": 27, "summary": "Invalid shape"}}})
	result, err := validateV5Output(job, store.JobContext{}, content)
	if err != nil || !result.Partial || len(result.Plan.Concepts) != 1 || !strings.Contains(result.Note, "invalid concept fields") {
		t.Fatalf("one malformed concept discarded its valid neighbor: %+v %v", result, err)
	}
}

func TestV5CurlyQuoteAndWhitespaceEvidenceSnapsBeforePublication(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	material := "A “TLS” client verifies a signed certificate.\nIt then checks the server name carefully."
	source, err := s.Capture(ctx, store.CaptureInput{Mode: "text", Text: material}, "robust-quote-snap")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || job == nil || job.Kind != "plan" {
		t.Fatalf("plan claim: %+v %v", job, err)
	}
	plan := store.PlanContent{Goal: "Understand certificate trust", Concepts: []store.PlannedConcept{{Key: "c1", Name: "TLS", Summary: "A client verifies a server certificate before trust.", Note: &store.NoteContent{Level: "standard", Title: "Certificate verification", Body: "A client verifies a signed certificate and then checks the server name. In practice these are separate checks, so a signature alone does not prove the expected server name.", Basis: "source", Evidence: []string{`A "TLS" client verifies a signed certificate. It then checks the server name carefully.`}}}}}
	result, err := validateV5Output(job, store.JobContext{}, modelJSON(t, plan))
	if err != nil || result.Plan.Concepts[0].Note.Evidence[0] != material {
		t.Fatalf("source quotation did not snap to original bytes: %+v %v", result, err)
	}
	result.Model, result.PromptVersion = "fixture", "scry-plan-v1"
	zero := int64(0)
	if err := s.CompleteJob(ctx, job.ID, job.LeaseToken, result, &zero); err != nil {
		t.Fatalf("snapped quotation rejected by store: %v", err)
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || saved.Jobs[0].Status != "complete" || saved.Jobs[0].Published != 1 {
		t.Fatalf("quoted concept did not publish: %+v %v", saved, err)
	}
}

func TestV5WebEvidenceAllowsUnquotedAnswerButNotInventedNumbers(t *testing.T) {
	job := &store.Job{Kind: "questions", SourceKind: "topic", SourceMode: "topic", SourceText: "certificate trust"}
	input := store.JobContext{Concepts: []store.ConceptContext{{ID: "c1"}}, Documents: []store.SourceDocument{{ID: "doc-1", Kind: "search_result", Title: "Certificates", URL: "https://example.test/certs", Text: "Certificates bind public keys to domain names."}}}
	question := map[string]any{"concept": "c1", "also": []string{}, "level": "recall", "answer_form": "exact", "kind": "recall", "prompt": "What system organizes trust in certificates and keys?", "answer": "PKI", "explanation": "PKI connects certificate authorities, public keys, and names within a trust system.", "basis": "web", "evidence": "Certificates bind public keys to domain names.", "choices": []string{}, "variants": []string{}, "choice_concepts": []string{}, "citations": []store.Citation{{DocumentID: "doc-1", Title: "Certificates", URL: "https://example.test/certs"}}, "required_ideas": []string{}, "covers": []string{}}
	content := func() string { return modelJSON(t, map[string]any{"quizzes": []any{question}}) }

	result, err := validateV5Output(job, input, content())
	if err != nil || len(result.Quizzes) != 1 || result.Quizzes[0].Basis != "web" {
		t.Fatalf("web answer needed an unnecessary substring match: %+v %v", result, err)
	}
	question["answer"] = "PKI 2029"
	if _, err := validateV5Output(job, input, content()); err == nil || !strings.Contains(err.Error(), "unsupported_source_answer") {
		t.Fatalf("invented number escaped web check: %v", err)
	}
}
func TestV5WebQuotationSnapsAndPublishesWithTrueCitation(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	_, err := s.Capture(ctx, store.CaptureInput{Mode: "topic", Text: "TLS certificate trust"}, "robust-web-snap")
	if err != nil {
		t.Fatal(err)
	}
	research, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	excerpt := "A “TLS” client verifies a signed certificate.\nIt checks the server name."
	doc := store.DocumentContent{Kind: "search_result", URL: "https://example.test/certificates", Title: "Certificate checks", Provider: "exa", Text: excerpt}
	zero := int64(0)
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Documents: []store.DocumentContent{doc}, Model: "exa", PromptVersion: "scry-research-v1", Note: "Web excerpts ready."}, &zero); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.JobContext(ctx, job.ID)
	if err != nil || len(input.Documents) != 1 {
		t.Fatalf("web material unavailable: %+v %v", input, err)
	}
	plan := store.PlanContent{Goal: "Understand certificate trust", Concepts: []store.PlannedConcept{{Key: "c1", Name: "TLS", Summary: "A client verifies both the certificate and the server name.", Note: &store.NoteContent{Level: "standard", Title: "Certificate checks", Body: "A client verifies a signed certificate and then checks the server name. These checks are separate, so a signature alone cannot prove that a response came from the expected server.", Basis: "web", Evidence: []string{`A "TLS" client verifies a signed certificate. It checks the server name.`}, Citations: []store.Citation{{DocumentID: "wrong", Title: "Unknown", URL: "https://invalid.test"}}}}}}
	result, err := validateV5Output(job, input, modelJSON(t, plan))
	if err != nil || result.Plan.Concepts[0].Note.Evidence[0] != excerpt || len(result.Plan.Concepts[0].Note.Citations) != 1 || result.Plan.Concepts[0].Note.Citations[0].DocumentID != input.Documents[0].ID {
		t.Fatalf("web evidence/citation not reconciled: %+v %v", result, err)
	}
	result.Model, result.PromptVersion = "fixture", "scry-plan-v1"
	if err := s.CompleteJob(ctx, job.ID, job.LeaseToken, result, &zero); err != nil {
		t.Fatalf("snapped web note rejected at publication: %v", err)
	}
}

func TestV5QuestionsDropInvalidSortLevelsClearCoversAndPublish(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	source, err := s.Capture(ctx, store.CaptureInput{Mode: "topic", Text: "cell energy"}, "robust-partial-questions")
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	research, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Model: "unavailable", PromptVersion: "scry-research-v1", Note: "Web search unavailable."}, &zero); err != nil {
		t.Fatal(err)
	}
	planJob, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	plan := store.PlanContent{Goal: "Understand cell energy", Concepts: []store.PlannedConcept{{Key: "c1", Name: "ATP", Summary: "ATP supports cellular energy transfer.", Note: &store.NoteContent{Level: "standard", Title: "ATP", Body: standardNoteBody, Basis: "topic"}}}}
	if err := s.CompleteJob(ctx, planJob.ID, planJob.LeaseToken, store.GenerationResult{Plan: &plan, Model: "fixture", PromptVersion: "scry-plan-v1", Note: "Ready."}, &zero); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.JobContext(ctx, job.ID)
	if err != nil || len(input.Concepts) != 1 {
		t.Fatalf("question context: %+v %v", input, err)
	}
	id := input.Concepts[0].ID
	makeQuestion := func(prompt, level, answer string) map[string]any {
		return map[string]any{"concept": id, "also": []string{}, "level": level, "answer_form": "exact", "kind": "recall", "prompt": prompt, "answer": answer, "explanation": "ATP transfers energy through its chemical reactions during cellular work.", "basis": "topic", "evidence": "", "choices": []string{}, "variants": []string{}, "choice_concepts": []string{}, "citations": []store.Citation{}, "required_ideas": []string{}, "covers": []string{}}
	}
	late := makeQuestion("Which molecule supports energy transfer during cellular work?", "explain", "ATP")
	bad := makeQuestion("ATP is the answer to which cell energy question?", "recall", "ATP")
	early := makeQuestion("Which molecule is an energy carrier in cells?", "recognize", "ATP")
	late["covers"], early["covers"] = []string{"u99"}, []string{"invented"}
	late["variants"] = []string{"ATP", "cellular work", "adenosine triphosphate"}
	result, err := validateV5Output(job, input, modelJSON(t, map[string]any{"quizzes": []any{late, bad, early}}))
	if err != nil || !result.Partial || len(result.Quizzes) != 2 || result.Quizzes[0].Level != "recognize" || len(result.Quizzes[1].Variants) != 1 || result.Quizzes[1].Variants[0] != "adenosine triphosphate" || !strings.Contains(result.Note, "answer_leakage_or_vague_prompt") {
		t.Fatalf("valid questions were lost or out of order: %+v %v", result, err)
	}
	result.Model, result.PromptVersion = "fixture", "scry-questions-v1"
	if err := s.CompleteJob(ctx, job.ID, job.LeaseToken, result, &zero); err != nil {
		t.Fatalf("filtered questions rejected by store: %v", err)
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || len(saved.Quizzes) != 2 || saved.Jobs[2].Status != "partial" {
		t.Fatalf("valid remainder did not reach learner: %+v %v", saved, err)
	}
}

func TestV5SourceQuestionDropsUnsupportedAndPromptCopiedVariants(t *testing.T) {
	material := "ATP (adenosine triphosphate) transfers cellular energy."
	job := &store.Job{Kind: "questions", SourceKind: "source", SourceMode: "text", SourceText: material}
	input := store.JobContext{Concepts: []store.ConceptContext{{ID: "atp"}}}
	question := map[string]any{
		"concept": "atp", "also": []string{}, "level": "recall", "answer_form": "flexible", "kind": "recall",
		"prompt": "Which molecule transfers cellular energy rather than the mentioned ADP?", "answer": "ATP",
		"explanation": "ATP transfers energy through chemical reactions during cellular work.",
		"basis":       "source", "evidence": material, "choices": []string{},
		"variants":        []string{"ADP", "ATP", "adenosine triphosphate", "adenosine triphosphate", " fictitious molecule ", "fictitious molecule", "bad*pattern"},
		"choice_concepts": []string{}, "citations": []store.Citation{}, "required_ideas": []string{}, "covers": []string{},
	}
	result, err := validateV5Output(job, input, modelJSON(t, map[string]any{"quizzes": []any{question}}))
	if err != nil || len(result.Quizzes) != 1 || len(result.Quizzes[0].Variants) != 1 || result.Quizzes[0].Variants[0] != "adenosine triphosphate" {
		t.Fatalf("safe question or variant was lost, or unsafe variant retained: %+v %v", result, err)
	}
}

func TestV5UnmatchedWebQuoteDowngradesTopicButDropsSource(t *testing.T) {
	doc := store.SourceDocument{ID: "web-1", Kind: "search_result", Title: "Energy", URL: "https://example.test/energy", Text: "ATP transfers energy in cells."}
	note := store.NoteContent{Level: "standard", Title: "ATP", Body: standardNoteBody, Basis: "web", Evidence: []string{"ATP guarantees perfect memory."}, Citations: []store.Citation{{DocumentID: doc.ID, Title: doc.Title, URL: doc.URL}}}
	topic := &store.Job{Kind: "plan", SourceKind: "topic", SourceMode: "topic", SourceText: "cell energy"}
	if err := validateV5Note(&note, topic, store.JobContext{Documents: []store.SourceDocument{doc}}); err != nil || note.Basis != "topic" || len(note.Evidence) != 0 || len(note.Citations) != 0 {
		t.Fatalf("topic was not honestly downgraded: %+v %v", note, err)
	}
	source := &store.Job{Kind: "plan", SourceKind: "source", SourceMode: "text", SourceText: "ATP transfers energy in cells."}
	note.Basis, note.Evidence, note.Citations = "web", []string{"ATP guarantees perfect memory."}, []store.Citation{{DocumentID: doc.ID, Title: doc.Title, URL: doc.URL}}
	if err := validateV5Note(&note, source, store.JobContext{Documents: []store.SourceDocument{doc}}); err == nil {
		t.Fatal("unmatched source-kind web claim was not dropped")
	}
}
