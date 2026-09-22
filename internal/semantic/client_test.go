package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func TestClientParsesDecisionAndRoundsCostUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-OpenRouter-Title") != "Scry" {
			t.Fatalf("unexpected request: %s %v", r.Method, r.Header)
		}
		var request Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != DefaultModel || len(request.Questions) != 3 {
			t.Fatalf("unexpected body: %+v %v", request, err)
		}
		w.Header().Set("X-Request-ID", "header-id")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"decision-id","model":"typesafe/jev-1.13-20260917","answers":{"idea_0":{"noul":0.97},"relation":{"choice":"equivalent","probabilities":{"equivalent":0.99,"partial":0.01}},"injection":{"noul":0.01}},"usage":{"input_tokens":456,"output_tokens":23,"cost":0.0000191}}`)
	}))
	defer server.Close()
	client := NewClient(Config{Endpoint: server.URL, APIKey: "secret", Model: DefaultModel, HTTPClient: server.Client()})
	request := BuildRecallRequest(DefaultModel, RecallState{
		Prompt: "What is an ETag?", ExpectedAnswer: "The entity tag", LearnerAnswer: "the entity tag",
		Rubric: learning.Rubric{Required: []learning.RubricIdea{{Text: "It is an entity tag"}}},
	})
	response, err := client.Decide(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "typesafe/jev-1.13-20260917" || response.RequestID != "decision-id" || response.Usage.InputTokens != 456 || response.Usage.OutputTokens != 23 || response.Usage.CostMicros == nil || *response.Usage.CostMicros != 20 || len(response.Raw) == 0 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestBuildRecallRequestUsesFrozenTemplates(t *testing.T) {
	request := BuildRecallRequest("model", RecallState{
		Prompt: "Explain caching.", ExpectedAnswer: "Validators avoid stale reuse.", LearnerAnswer: "ETags validate cached responses.",
		Variants: []string{"Validators check cached responses."},
		Rubric: learning.Rubric{
			Required:       []learning.RubricIdea{{Text: "Validators check whether cached content is current", Cue: "Think about freshness checks"}},
			Contradictions: []learning.RubricClaim{{Text: "Validators always prevent cache reuse", Feedback: "They permit reuse when unchanged."}},
		},
	})
	if request.Model != "model" || len(request.Questions) != 4 {
		t.Fatalf("unexpected request: %+v", request)
	}
	idea := request.Questions["idea_0"]
	instructions, ok := idea.Instructions.(map[string]string)
	if !ok || idea.Type != "noul" || instructions["question"] != "Does `learner_answer` correctly express `required_idea` as an answer to `prompt`? Judge only from the supplied state." || instructions["required_idea"] == "" {
		t.Fatalf("idea template changed: %+v", idea)
	}
	if request.Questions["contradiction_0"].Type != "noul" || request.Questions["relation"].Type != "choice" || request.Questions["injection"].Type != "noul" {
		t.Fatalf("question battery changed: %+v", request.Questions)
	}
	encoded, err := json.Marshal(request.State)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "Think about freshness") || strings.Contains(text, "They permit reuse") || !strings.Contains(text, `"learner_answer":"ETags validate cached responses."`) {
		t.Fatalf("state leaked cue/feedback or omitted answer: %s", text)
	}
}

func TestClientErrorClassesAndMalformedResponses(t *testing.T) {
	valid := `{"model":"jev","answers":{"x":{"noul":0.5}},"usage":{"input_tokens":1,"output_tokens":1}}`
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"rate limited", 429, `{}`, ErrUnavailable},
		{"provider 529", 529, `{}`, ErrUnavailable},
		{"server failure", 503, `{}`, ErrUnavailable},
		{"unauthorized", 401, `{}`, ErrRejected},
		{"forbidden", 403, `{}`, ErrRejected},
		{"unprocessable", 422, `{}`, ErrRejected},
		{"other client rejection", 409, `{}`, ErrRejected},
		{"invalid json", 200, `{`, ErrMalformed},
		{"duplicate member", 200, `{"model":"jev","model":"other","answers":{"x":{"noul":0.5}},"usage":{}}`, ErrMalformed},
		{"missing answers", 200, `{"model":"jev","answers":{},"usage":{}}`, ErrMalformed},
		{"invalid probability", 200, `{"model":"jev","answers":{"x":{"noul":1.1}},"usage":{}}`, ErrMalformed},
		{"success control", 200, valid, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := NewClient(Config{Endpoint: server.URL, HTTPClient: server.Client()}).Decide(context.Background(), Request{Model: "jev", State: map[string]string{}, Questions: map[string]Question{"x": {Type: "noul", Instructions: "x"}}})
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestClientUnavailableWithoutEndpointOrOnTimeout(t *testing.T) {
	request := Request{Model: "jev", State: struct{}{}, Questions: map[string]Question{"x": {Type: "noul", Instructions: "x"}}}
	// No endpoint is the one provable no-send failure; it is both
	// unavailable and not-configured so callers can release the reservation.
	if _, err := NewClient(Config{}).Decide(context.Background(), request); !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty endpoint: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{"model":"jev","answers":{"x":{"noul":0.5}},"usage":{}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	// A timeout may already have been accepted by the provider: unavailable,
	// but never classified as a no-send failure.
	if _, err := NewClient(Config{Endpoint: server.URL, HTTPClient: server.Client()}).Decide(ctx, request); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotConfigured) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestClientRefusesRedirectAndCapsResponse(t *testing.T) {
	request := Request{Model: "jev", State: struct{}{}, Questions: map[string]Question{"x": {Type: "noul", Instructions: "x"}}}
	var redirected atomic.Bool
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirected.Store(true)
			fmt.Fprint(w, `{"model":"jev","answers":{"x":{"noul":0.5}},"usage":{}}`)
			return
		}
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer redirectServer.Close()
	if _, err := NewClient(Config{Endpoint: redirectServer.URL, HTTPClient: redirectServer.Client()}).Decide(context.Background(), request); !errors.Is(err, ErrRejected) {
		t.Fatalf("redirect: %v", err)
	}
	if redirected.Load() {
		t.Fatal("semantic client followed redirect")
	}

	largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", maxResponseSize+1))
	}))
	defer largeServer.Close()
	if _, err := NewClient(Config{Endpoint: largeServer.URL, HTTPClient: largeServer.Client()}).Decide(context.Background(), request); !errors.Is(err, ErrMalformed) {
		t.Fatalf("oversized response: %v", err)
	}
}

func TestCostMicros(t *testing.T) {
	cases := []struct {
		raw  string
		want *int64
	}{
		{"0", int64ptr(0)}, {"0.000001", int64ptr(1)}, {"0.0000010001", int64ptr(2)}, {"1e-7", int64ptr(1)},
		{"null", nil}, {`"0.1"`, nil}, {"-0.1", nil}, {"1e99", nil},
	}
	for _, tc := range cases {
		got := costMicros(json.RawMessage(tc.raw))
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("costMicros(%s)=%v want %v", tc.raw, got, tc.want)
		}
	}
}

func int64ptr(value int64) *int64 { return &value }
