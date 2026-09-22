package semantic

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/learning"
)

func TestCriticRequestDeterministicMinimizedAndApplicable(t *testing.T) {
	candidate := CandidateState{Kind: "choice", Grading: "exact", Prompt: "Untrusted prompt", Answer: "answer", Explanation: "explanation", Choices: []string{"a", "b", "c"}, Basis: "source", Evidence: "evidence"}
	request := BuildCriticRequest("pinned-model", candidate)
	for _, key := range []string{"unsupported_by_evidence", "mcq_options_overlap"} {
		if request.Questions[key].Type != "noul" {
			t.Fatalf("missing %s", key)
		}
	}
	if _, ok := request.Questions["rubric_misaligned"]; ok {
		t.Fatal("exact question got rubric check")
	}
	for key, q := range request.Questions {
		if q.Type != "noul" || strings.Contains(key, candidate.Prompt) {
			t.Fatal(key, q)
		}
	}
	encoded, err := json.Marshal(request.State)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for key := range fields {
		switch key {
		case "prompt", "answer", "explanation", "choices", "rubric", "basis", "evidence":
		default:
			t.Fatalf("private/unrelated field %s", key)
		}
	}
	if !reflect.DeepEqual(request, BuildCriticRequest("pinned-model", candidate)) {
		t.Fatal("request is nondeterministic")
	}
	candidate.Kind, candidate.Grading, candidate.Basis = "recall", "semantic", "topic"
	candidate.Rubric = &learning.Rubric{Required: []learning.RubricIdea{{Text: "idea", Cue: "minimal cue"}}}
	request = BuildCriticRequest("", candidate)
	if request.Model != DefaultModel || request.Questions["rubric_misaligned"].Type != "noul" {
		t.Fatal(request)
	}
	for _, key := range []string{"unsupported_by_evidence", "mcq_options_overlap"} {
		if _, ok := request.Questions[key]; ok {
			t.Fatal(key)
		}
	}
}

func TestCriticJudgmentsRejectMixedHeads(t *testing.T) {
	params := learning.CriticParams{}
	value := .1
	response := Response{Answers: map[string]Answer{}}
	for _, key := range append(learning.CriticHardKeys(params), learning.CriticSoftKey) {
		response.Answers[key] = Answer{Type: "noul", Noul: &value}
	}
	if got := learning.JudgeCandidate(CriticJudgments(response, params), params); got.Decision != "accept" {
		t.Fatal(got)
	}
	response.Answers[learning.CriticSoftKey] = Answer{Type: "choice", Noul: &value}
	if got := learning.JudgeCandidate(CriticJudgments(response, params), params); got.Decision != "ungraded" {
		t.Fatal(got)
	}
}
