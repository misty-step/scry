package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestG11_AnthropicDraftRequestIsBoundedAndStreaming(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var b map[string]any
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		if b["stream"] != true || b["max_tokens"] != float64(16000) || b["model"] != DefaultDraftModel || b["fallbacks"] != nil {
			t.Errorf("unbounded/nonstreaming draft: %v", b)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local rejection"}}`))
	}))
	defer srv.Close()
	c := NewClaude("local-test-key", "")
	c.BaseURL = srv.URL
	_, _, err := c.Draft(context.Background(), DraftInput{Query: "physics"}, DraftSink{})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}
