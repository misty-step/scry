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
)

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
