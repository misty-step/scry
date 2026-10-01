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

func TestV5KnowledgeChainKeepsCapturePrivacyAndDurableProvenance(t *testing.T) {
	for _, tc := range []struct {
		mode, text, name, summary, body string
		prompt, answer, explanation     string
		choices                         []string
	}{
		{
			mode: "topic", text: "cell energy", name: "Cell energy transfer",
			summary: "ATP transfers energy during cellular work.", body: standardNoteBody,
			prompt: "Which molecule transfers energy during cellular work?", answer: "ATP",
			explanation: "ATP transfers chemical energy when its phosphate groups participate in cellular reactions.",
			choices:     []string{"ATP", "DNA", "cellulose"},
		},
		{
			mode: "text", text: "Montessori: methodology, philosophy, key concepts, practical application",
			name: "Role of Montessori Guide", summary: "The guide observes children and supports their independent work.",
			body:   "A Montessori guide observes children and connects them with suitable activities in a prepared environment. The guide demonstrates how to use materials, then gives a child room to work independently. Observation helps the guide decide when to offer support and when to step back. For example, a child repeating a practical activity may need time to practice rather than another demonstration. A common confusion is to equate independence with an absent teacher; the guide actively prepares the environment and responds to each child's needs.",
			prompt: "What is the main role of a guide in Montessori education?", answer: "Observe and guide individual work",
			explanation: "A Montessori guide observes children and supports independent activity rather than directing every action.",
			choices:     []string{"Observe and guide individual work", "Lecture to the whole class", "Assign identical work to everyone"},
		},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx := context.Background()
			s := generationStore(t)
			source, err := s.Capture(ctx, store.CaptureInput{Text: tc.text, Mode: tc.mode}, "v5-knowledge-chain")
			if err != nil {
				t.Fatal(err)
			}
			plan := store.PlanContent{Goal: "Understand " + tc.name, Concepts: []store.PlannedConcept{{
				Key: "c1", Name: tc.name, Summary: tc.summary,
				Note: &store.NoteContent{Title: tc.name, Body: tc.body, Basis: "topic", Evidence: []string{}, Citations: []store.Citation{}},
			}}}
			var conceptID string
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var content []byte
				switch {
				case containsSchema(body, "scry_plan"):
					content, err = json.Marshal(plan)
				case containsSchema(body, "scry_questions"):
					question := map[string]any{"concept": conceptID, "level": "recall", "kind": "recall", "prompt": tc.prompt, "answer": tc.answer, "explanation": tc.explanation, "basis": "topic", "evidence": "", "choices": []string{}, "variants": []string{}, "citations": []store.Citation{}, "required_ideas": []string{}, "covers": []string{}}
					recognize := withPrompt(question, "Which answer best describes "+tc.name+"?")
					recognize["kind"], recognize["level"] = "choice", "recognize"
					recognize["choices"] = tc.choices
					content, err = json.Marshal(map[string]any{"quizzes": []any{recognize, question}})
				default:
					t.Errorf("unexpected generation request: %s", body)
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				io.WriteString(w, envelopeJSON(t, string(content), "stop", json.RawMessage(`0.001`)))
			}))
			defer server.Close()
			worker := New(s, localConfig(server.URL))
			kinds := []string{"plan", "questions"}
			if tc.mode == "topic" {
				kinds = []string{"research", "plan", "questions"}
			}
			for _, kind := range kinds {
				job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
				if err != nil || job == nil || job.Kind != kind {
					t.Fatalf("claim %s: %+v %v", kind, job, err)
				}
				if kind == "questions" {
					input, err := s.JobContext(ctx, job.ID)
					if err != nil || len(input.Concepts) != 1 || input.Concepts[0].Note == nil {
						t.Fatalf("question context: %+v %v", input, err)
					}
					note := input.Concepts[0].Note
					if note.Basis != "topic" || len(note.Evidence) != 0 || len(note.Citations) != 0 {
						t.Fatalf("plan lost its honest grounding: %+v", note)
					}
					conceptID = input.Concepts[0].ID
				}
				if err := worker.process(ctx, job); err != nil {
					t.Fatalf("settle %s: %v", kind, err)
				}
			}
			path := s.Path()
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			saved, err := s.Source(ctx, source.ID)
			if err != nil || saved.Status != "ready" || len(saved.Quizzes) != 2 || len(saved.Jobs) != len(kinds) || calls != 2 {
				t.Fatalf("incomplete durable chain: %+v %v calls=%d", saved, err, calls)
			}
			if saved.Kind != source.Kind || saved.Mode != tc.mode || saved.Web != source.Web || saved.Text != tc.text || saved.Revision != 1 || len(saved.Documents) != 0 {
				t.Fatalf("knowledge grounding changed capture identity or researched it: %+v", saved)
			}
			for _, q := range saved.Quizzes {
				if q.ConceptID != conceptID || q.Basis != "topic" || q.Evidence != "" || len(q.Citations) != 0 {
					t.Errorf("published question claimed source evidence: %+v", q)
				}
			}
			for i, job := range saved.Jobs {
				cost := int64(1000)
				if kinds[i] == "research" {
					cost = 0
				}
				if job.Status != "complete" || job.Attempts != 1 || job.CostUnknown || job.CostMicros != cost {
					t.Errorf("unexpected retry or spend settlement: %+v", job)
				}
			}
			state, err := s.Review(ctx)
			if err != nil || state.Intro == nil || state.Intro.Note == nil || state.Intro.Note.Basis != "topic" || len(state.Intro.Note.Evidence) != 0 || len(state.Intro.Note.Citations) != 0 {
				t.Fatalf("general-knowledge note was not durably reviewable: %+v %v", state, err)
			}
		})
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
	quiz := store.GeneratedQuiz{Kind: "recall", Prompt: "Which molecule transfers cellular energy?", Answer: "ATP", Explanation: "ATP transfers chemical energy in reactions that help cells do work.", Basis: "topic", Concept: existingID, Level: "recall"}
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
