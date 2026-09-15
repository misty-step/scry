package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func topicDraft() quizDraft {
	return quizDraft{
		Kind: "recall", Basis: "topic", Evidence: "",
		Prompt:      "Which molecule directly supplies energy for many cellular processes?",
		Answer:      "ATP",
		Explanation: "ATP transfers chemical energy through phosphate-group reactions rather than serving as long-term genetic storage.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{},
	}
}

func outputJSON(t *testing.T, kind string, drafts ...quizDraft) string {
	t.Helper()
	if drafts == nil {
		drafts = []quizDraft{}
	}
	body, err := json.Marshal(struct {
		Coverage outputCoverage `json:"coverage"`
		Quizzes  []quizDraft    `json:"quizzes"`
	}{outputCoverage{Kind: kind, Complete: true, Missing: []string{}}, drafts})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func envelopeJSON(t *testing.T, content, finish string, price json.RawMessage) string {
	t.Helper()
	envelope := map[string]any{
		"model": "boundary-model",
		"choices": []any{map[string]any{
			"finish_reason": finish,
			"message":       map[string]any{"role": "assistant", "content": content},
		}},
	}
	if price != nil {
		envelope["usage"] = map[string]any{"cost": price}
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func localConfig(endpoint string) Config {
	return Config{Endpoint: endpoint, Model: "boundary-model", DailyBudgetMicros: 1_000_000, ReservationMicros: 100_000}
}

func responseServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPaidRejectedResponsesRetainReportedCost(t *testing.T) {
	valid := outputJSON(t, "concepts", topicDraft())
	cases := []struct{ name, body string }{
		{"malformed quiz JSON", envelopeJSON(t, `{"coverage":`, "stop", json.RawMessage(`0.012345`))},
		{"malformed choice envelope", `{"choices":"not-an-array","usage":{"cost":0.012345}}`},
		{"truncated completion", envelopeJSON(t, valid, "length", json.RawMessage(`0.012345`))},
		{"choice-level provider error", strings.Replace(envelopeJSON(t, valid, "stop", json.RawMessage(`0.012345`)), `"finish_reason":"stop"`, `"error":{"message":"provider failed"},"finish_reason":"stop"`, 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := responseServer(t, test.body, http.StatusOK)
			worker := New(nil, localConfig(server.URL))
			result, cost, failure := worker.generate(context.Background(), &store.Job{SourceText: "mitochondria", SourceKind: "topic", Attempts: 1})
			if failure == nil || failure.retry || len(result.Quizzes) != 0 {
				t.Fatalf("malformed paid output must not publish or trigger an unearned retry: result=%+v failure=%+v", result, failure)
			}
			if cost == nil || *cost != 12_345 {
				t.Fatalf("rejected paid response lost its receipt: %v", cost)
			}
		})
	}
}

func TestUnknownPriceDoesNotBecomeFreeRetry(t *testing.T) {
	server := responseServer(t, `{"error":{"message":"provider-private-detail"}}`, http.StatusServiceUnavailable)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || failure.retry || response.cost != nil {
		t.Fatalf("unknown error spend must remain unknown without an automatic paid retry: cost=%v failure=%+v", response.cost, failure)
	}
	if strings.Contains(failure.message, "provider-private-detail") {
		t.Fatal("raw provider details leaked into durable user-visible failure")
	}
}

func TestExplicitFreeTransientRejectionCanRetry(t *testing.T) {
	server := responseServer(t, `{"error":{"message":"busy"},"usage":{"cost":0}}`, http.StatusTooManyRequests)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || !failure.retry || response.cost == nil || *response.cost != 0 {
		t.Fatalf("an explicitly free transient rejection should permit bounded durable backoff: cost=%v failure=%+v", response.cost, failure)
	}
}

func TestRedirectCannotDiscloseSourceOrCredentials(t *testing.T) {
	var reached atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	cfg := localConfig(redirect.URL)
	cfg.APIKey = "private-boundary-test-key"
	worker := New(nil, cfg)
	_, failure := worker.call(context.Background(), []byte(`{"source":"private-source"}`))
	if failure == nil || reached.Load() != 0 {
		t.Fatalf("redirect received private data: requests=%d failure=%+v", reached.Load(), failure)
	}
}

func TestOversizeResponseNeverPublishesOrInventsCost(t *testing.T) {
	server := responseServer(t, strings.Repeat(" ", maxResponseBytes+1), http.StatusOK)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || failure.retry || response.cost != nil || response.content != "" {
		t.Fatalf("oversized response escaped the bounded boundary: %+v %+v", response, failure)
	}
}

func TestQualityRepairRequiresKnownReceiptAndStopsAfterOnePass(t *testing.T) {
	bad := topicDraft()
	bad.Prompt = "Why does ATP directly supply cellular energy?" // Leaks the answer.
	server := responseServer(t, envelopeJSON(t, outputJSON(t, "concepts", bad), "stop", json.RawMessage(`0.001`)), http.StatusOK)
	worker := New(nil, localConfig(server.URL))
	job := &store.Job{SourceText: "mitochondria", SourceKind: "topic", Attempts: 1}
	_, cost, first := worker.generate(context.Background(), job)
	if first == nil || !first.retry || cost == nil || *cost != 1_000 {
		t.Fatalf("small paid quality failure did not request a separately reserved repair: %v %+v", cost, first)
	}
	job.Attempts, job.Error = 2, first.message
	_, cost, second := worker.generate(context.Background(), job)
	if second == nil || second.retry || cost == nil || *cost != 1_000 {
		t.Fatalf("quality repair did not stop after one additional paid claim: %v %+v", cost, second)
	}
}

func TestOversizeCompleteTaskIsRejectedBeforeAnyPaidTransmission(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	worker := New(nil, localConfig(server.URL))
	job := &store.Job{SourceKind: "source", SourceText: "Learn all entries:\n" + strings.Repeat("- entry\n", 61), Attempts: 1}
	result, cost, failure := worker.generate(context.Background(), job)
	if calls.Load() != 0 || failure == nil || failure.retry || cost == nil || *cost != 0 || len(result.Quizzes) != 0 {
		t.Fatalf("oversized complete task was billed or silently sampled: calls=%d result=%+v cost=%v failure=%+v", calls.Load(), result, cost, failure)
	}
}
