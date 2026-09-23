package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func TestV5ModelJobsUseKindSpecificSingleTransmission(t *testing.T) {
	question := map[string]any{"concept": "a", "level": "recall", "answer_form": "exact", "kind": "recall", "prompt": "Which molecule supplies energy during many cellular processes?", "answer": "ATP", "explanation": "ATP transfers chemical energy when its phosphate groups participate in cellular reactions.", "basis": "topic", "evidence": "", "choices": []string{}, "variants": []string{}, "citations": []string{}, "required_ideas": []string{}, "covers": []string{}}
	recognize := withPrompt(question, "Which molecule is commonly used to transfer cellular energy?")
	recognize["kind"], recognize["level"], recognize["answer_form"] = "choice", "recognize", ""
	recognize["choices"] = []string{"ATP", "DNA", "cellulose"}
	plan := store.PlanContent{Goal: "Understand cell energy", Concepts: []store.PlannedConcept{{Key: "c1", Name: "Cell energy transfer", Summary: "ATP transfers energy during cellular work.", Note: &store.NoteContent{Title: "Cell energy transfer", Body: standardNoteBody, Basis: "topic", Evidence: []string{}, Citations: []store.Citation{}}}}}
	cases := []struct {
		kind    string
		content any
		input   store.JobContext
	}{
		{"plan", plan, store.JobContext{}},
		{"questions", map[string]any{"quizzes": []any{recognize, question}}, store.JobContext{Concepts: []store.ConceptContext{{ID: "a", Name: "Cell energy transfer"}}}},
		{"fix", map[string]any{"quizzes": []any{question}}, store.JobContext{Quiz: &store.Quiz{ConceptID: "a"}, Concepts: []store.ConceptContext{{ID: "a"}}}},
		{"transcribe", map[string]any{"title": "My notebook", "text": "ATP transfers energy."}, store.JobContext{Image: &store.CaptureImage{MIME: "image/png", Bytes: []byte("synthetic-image")}}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			content, err := json.Marshal(tc.content)
			if err != nil {
				t.Fatal(err)
			}
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if !strings.Contains(string(data), `"name":"scry_`+tc.kind+`"`) || !strings.Contains(string(data), `"require_parameters":true`) {
					t.Error("strict provider routing omitted")
				}
				if tc.kind == "transcribe" && !strings.Contains(string(data), "data:image/png;base64,") {
					t.Error("vision image URL missing")
				}
				io.WriteString(w, envelopeJSON(t, string(content), "stop", json.RawMessage(`0.001`)))
			}))
			defer server.Close()
			cfg := localConfig(server.URL)
			cfg.Provider = "openrouter"
			worker := New(nil, cfg)
			job := &store.Job{Kind: tc.kind, SourceText: "cell energy", SourceKind: "topic", SourceMode: "topic"}
			result, cost, failure := worker.generateV5(context.Background(), job, tc.input)
			if failure != nil || cost == nil || *cost != 1000 || calls != 1 {
				t.Fatalf("dispatch %s: result=%+v cost=%v failure=%+v calls=%d", tc.kind, result, cost, failure, calls)
			}
			if result.PromptVersion != "scry-"+tc.kind+"-v1" {
				t.Errorf("wrong prompt version: %q", result.PromptVersion)
			}
		})
	}
}

func withPrompt(question map[string]any, prompt string) map[string]any {
	clone := make(map[string]any, len(question))
	for key, value := range question {
		clone[key] = value
	}
	clone["prompt"] = prompt
	return clone
}
