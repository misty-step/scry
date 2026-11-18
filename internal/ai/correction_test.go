package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecisionCorrection_ConfigurableModelAndGradeOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Model     string
			State     map[string]any
			Questions map[string]struct{ Criteria map[string]string }
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Model != "cloudflare/clef" || len(b.Questions["judgment"].Criteria) != 4 || b.Questions["judgment"].Criteria["right"] == "" || b.State["accepted_answers"] == nil {
			t.Errorf("not a configurable grade classifier: %+v", b)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "cloudflare/clef", "answers": map[string]any{"judgment": map[string]any{"type": "choice", "choice": "right"}}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 1, "cost": 0.0000012}})
	}))
	defer srv.Close()
	j := &Jev{APIKey: "local", OpenRouter: true, BaseURL: srv.URL, Model: "cloudflare/clef"}
	got, use, err := j.Judge(context.Background(), JudgeInput{Expected: "Four", Answer: "4"})
	if err != nil || got.Verdict != "right" || use.CostUSD == nil || *use.CostUSD != 0.0000012 {
		t.Fatalf("%+v %+v %v", got, use, err)
	}
}
