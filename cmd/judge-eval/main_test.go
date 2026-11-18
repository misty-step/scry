package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEval_LocalModelsMetrics(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if r.URL.Path != "/api/alpha/decisions" || b["model"] == nil {
			t.Error("wrong endpoint")
		}
		choice := "correct"
		if calls%2 == 0 {
			choice = "incorrect"
		}
		json.NewEncoder(w).Encode(map[string]any{"model": b["model"], "answers": map[string]any{"judgment": map[string]string{"type": "choice", "choice": choice}}, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "cost": 0.0000012}})
	}))
	defer srv.Close()
	rows := []Sample{{Prompt: "q", Accepted: []string{"a"}, Answer: "a", Label: "correct"}, {Prompt: "q", Accepted: []string{"a"}, Answer: "", Label: "incorrect"}}
	var out bytes.Buffer
	if err := evaluate(context.Background(), rows, []string{"cloudflare/clef", "liquid/d1.2"}, srv.URL, "local-key", &out); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatal(calls)
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r Report
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r.Accuracy != 1 || r.TotalCost == nil || *r.TotalCost != 0.0000024 || r.Confusion["correct"]["correct"] != 1 || r.P95 < r.P50 {
			t.Fatalf("%+v", r)
		}
	}
}
func TestEval_Dataset(t *testing.T) {
	rows, err := readSamples("../../testdata/judge-eval.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 60 {
		t.Fatal(len(rows))
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Category]++
	}
	for _, c := range []string{"correct", "partial", "synonym", "typo", "incorrect", "blank", "injection"} {
		if counts[c] == 0 {
			t.Error("missing", c)
		}
	}
}

func TestEval_RefusalsAreNotAnsweredAndUnknownCostIsNotFree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"model": "test-model", "answers": map[string]any{"judgment": map[string]string{"type": "refusal"}}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1}})
	}))
	defer srv.Close()
	var out bytes.Buffer
	err := evaluate(context.Background(), []Sample{{Prompt: "q", Accepted: []string{"a"}, Label: "correct"}}, []string{"test-model"}, srv.URL, "local-key", &out)
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	json.Unmarshal(out.Bytes(), &r)
	if r.Accuracy != 0 || r.TotalCost != nil || r.UnknownCosts != 1 || r.Confusion["correct"]["not_answered"] != 1 {
		t.Fatalf("%+v", r)
	}
}
