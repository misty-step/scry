package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

func TestV5TopicChainResearchPlanQuestions(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	source, err := s.Capture(ctx, store.CaptureInput{Text: "cell energy", Mode: "topic"}, "v5-topic-chain")
	if err != nil {
		t.Fatal(err)
	}
	plan := store.PlanContent{Goal: "Understand cell energy", Concepts: []store.PlannedConcept{{Key: "c1", Name: "Cell energy transfer", Summary: "ATP transfers energy during cellular work.", Note: &store.NoteContent{Title: "Cell energy transfer", Body: standardNoteBody, Basis: "topic", Evidence: []string{}, Citations: []store.Citation{}}}}}
	var conceptID string
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var content []byte
		switch {
		case containsSchema(body, "scry_plan"):
			content, err = json.Marshal(plan)
		case containsSchema(body, "scry_questions"):
			question := map[string]any{"concept": conceptID, "level": "recall", "answer_form": "exact", "kind": "recall", "prompt": "Which molecule transfers energy during cellular work?", "answer": "ATP", "explanation": "ATP transfers chemical energy when its phosphate groups participate in cellular reactions.", "basis": "topic", "evidence": "", "choices": []string{}, "variants": []string{}, "citations": []store.Citation{}, "required_ideas": []string{}, "covers": []string{}}
			recognize := withPrompt(question, "Which molecule is commonly used to transfer cellular energy?")
			recognize["kind"], recognize["level"], recognize["answer_form"] = "choice", "recognize", ""
			recognize["choices"] = []string{"ATP", "DNA", "cellulose"}
			content, err = json.Marshal(map[string]any{"quizzes": []any{recognize, question}})
		default:
			t.Errorf("unexpected generation schema: %s", body)
		}
		if err != nil {
			t.Error(err)
			return
		}
		io.WriteString(w, envelopeJSON(t, string(content), "stop", json.RawMessage(`0.001`)))
	}))
	defer server.Close()
	worker := New(s, localConfig(server.URL))
	for _, kind := range []string{"research", "plan", "questions"} {
		job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
		if err != nil || job == nil || job.Kind != kind {
			t.Fatalf("claim %s: %+v %v", kind, job, err)
		}
		if kind == "questions" {
			input, err := s.JobContext(ctx, job.ID)
			if err != nil || len(input.Concepts) != 1 {
				t.Fatalf("question context: %+v %v", input, err)
			}
			conceptID = input.Concepts[0].ID
		}
		if err := worker.process(ctx, job); err != nil {
			t.Fatalf("settle %s: %v", kind, err)
		}
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || len(saved.Quizzes) != 2 || saved.Quizzes[0].ConceptID != conceptID || len(saved.Jobs) != 3 || calls != 2 {
		t.Fatalf("incomplete concept chain: %+v %v calls=%d", saved, err, calls)
	}
	for _, job := range saved.Jobs {
		if job.Status != "complete" || job.CostUnknown {
			t.Errorf("unsettled job: %+v", job)
		}
	}
	if saved.Jobs[0].CostMicros != 0 || saved.Jobs[1].CostMicros != 1000 || saved.Jobs[2].CostMicros != 1000 {
		t.Fatalf("attempt costs not preserved: %+v", saved.Jobs)
	}
}

type dedupeFixture func(semantic.Request) (semantic.Response, error)

func (f dedupeFixture) Decide(_ context.Context, req semantic.Request) (semantic.Response, error) {
	return f(req)
}

func TestV5PlanDedupeReusesActiveConcept(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	first, err := s.Capture(ctx, store.CaptureInput{Mode: "topic", Text: "Cell energy transfer"}, "dedupe-first")
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	research, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || research == nil {
		t.Fatalf("research claim: %v", err)
	}
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Model: "exa", PromptVersion: "scry-research-v1", Note: "Web search unavailable."}, &zero); err != nil {
		t.Fatal(err)
	}
	original := store.PlanContent{Goal: "Learn cell energy", Concepts: []store.PlannedConcept{{Key: "c1", Name: "Cell energy transfer", Summary: "ATP transfers energy during cellular work.", Note: &store.NoteContent{Title: "Cell energy transfer", Body: standardNoteBody, Basis: "topic"}}}}
	firstPlan, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || firstPlan == nil {
		t.Fatalf("plan claim: %v", err)
	}
	if err := s.CompleteJob(ctx, firstPlan.ID, firstPlan.LeaseToken, store.GenerationResult{Plan: &original, Model: "fixture", PromptVersion: "scry-plan-v1"}, &zero); err != nil {
		t.Fatal(err)
	}
	// Finish the first source's queued question job before another claim.
	questions, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || questions == nil {
		t.Fatalf("questions claim: %v", err)
	}
	firstContext, err := s.JobContext(ctx, questions.ID)
	if err != nil || len(firstContext.Concepts) != 1 {
		t.Fatalf("concept context: %+v %v", firstContext, err)
	}
	existingID := firstContext.Concepts[0].ID
	quiz := store.GeneratedQuiz{Kind: "recall", Prompt: "Which molecule transfers cellular energy?", Answer: "ATP", Explanation: "ATP transfers chemical energy in reactions that help cells do work.", Basis: "topic", Concept: existingID, Level: "recall", AnswerForm: "exact"}
	if err := s.CompleteJob(ctx, questions.ID, questions.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{quiz}, Model: "fixture", PromptVersion: "scry-questions-v1"}, &zero); err != nil {
		t.Fatal(err)
	}
	second, err := s.Capture(ctx, store.CaptureInput{Mode: "topic", Text: "Cell energy transfer"}, "dedupe-second")
	if err != nil {
		t.Fatal(err)
	}
	research, err = s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || research == nil || research.SourceID != second.ID {
		t.Fatalf("second research: %+v %v", research, err)
	}
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Model: "exa", PromptVersion: "scry-research-v1", Note: "Web search unavailable."}, &zero); err != nil {
		t.Fatal(err)
	}
	planJob, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || planJob == nil {
		t.Fatalf("second plan: %v", err)
	}
	var calls int
	cfg := localConfig("http://127.0.0.1:1")
	cfg.CriticModel = "fixture-jev"
	cfg.CriticSpending = store.SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 1_000_000}
	cfg.Critic = dedupeFixture(func(req semantic.Request) (semantic.Response, error) {
		calls++
		state, ok := req.State.(struct {
			Proposed   semantic.DedupeConcept   `json:"proposed"`
			Candidates []semantic.DedupeConcept `json:"candidates"`
		})
		if !ok || len(state.Candidates) != 1 || state.Candidates[0].ID != existingID || state.Candidates[0].Summary == "" {
			t.Errorf("candidate summary or identity missing: %+v", req.State)
		}
		cost := int64(1)
		return semantic.Response{Model: "fixture-jev", Raw: json.RawMessage(`{"same":"c0"}`), Answers: map[string]semantic.Answer{"same": {Type: "choice", Choice: "c0", Probabilities: map[string]float64{"c0": .91, "none": .09}}}, Usage: semantic.Usage{CostMicros: &cost}}, nil
	})
	worker := New(s, cfg)
	proposed := &store.PlanContent{Goal: "Learn cell energy again", Concepts: []store.PlannedConcept{{Key: "c1", Name: "Cell energy transfer", Summary: "ATP transfers energy during cellular work.", Note: &store.NoteContent{Title: "Cell energy transfer", Body: standardNoteBody, Basis: "topic"}}}}
	if err := worker.dedupePlan(ctx, planJob, proposed); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || proposed.Concepts[0].ExistingID != existingID || proposed.Concepts[0].Note != nil {
		t.Fatalf("Jev match not applied: %+v calls=%d", proposed.Concepts[0], calls)
	}
	if err := s.CompleteJob(ctx, planJob.ID, planJob.LeaseToken, store.GenerationResult{Plan: proposed, Model: "fixture", PromptVersion: "scry-plan-v1"}, &zero); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(ctx, first.ID)
	if err != nil || len(saved.Quizzes) != 1 {
		t.Fatalf("first concept changed: %+v %v", saved, err)
	}
}

func containsSchema(body []byte, schema string) bool {
	return json.Valid(body) && bytes.Contains(body, []byte(`"name":"`+schema+`"`))
}
