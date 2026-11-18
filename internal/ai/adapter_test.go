package ai

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestDraftHTTPStreamWithoutSDK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "local-key" || r.URL.Path != "/v1/messages" {
			t.Error("wrong wire request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []any{
			map[string]any{"type": "message_start", "message": map[string]any{"model": DefaultDraftModel, "usage": map[string]int{"input_tokens": 10, "output_tokens": 1}}},
			map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": "{\"kind\":\"topic\",\"title\":\"Physics\"}\n{\"idea\":{\"title\":\"Force\",\"items\":[{\"q\":\"Unit?\",\"a\":\"Newton\"}]}}\n{\"done\":true}\n"}},
			map[string]any{"type": "message_delta", "usage": map[string]int{"output_tokens": 40}},
			map[string]any{"type": "message_stop"},
		}
		for _, ev := range events {
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
	}))
	defer srv.Close()
	c := NewClaude("local-key", "")
	c.BaseURL = srv.URL
	head, ideas := 0, 0
	result, use, err := c.Draft(context.Background(), DraftInput{Query: "physics"}, DraftSink{
		Head: func(h DraftHead) error { head++; return nil }, Idea: func(i DraftIdea) error { ideas++; return nil },
	})
	if err != nil || head != 1 || ideas != 1 || result.Ideas != 1 || use.InputTokens != 10 || use.OutputTokens != 40 {
		t.Fatalf("%+v %+v %v head=%d ideas=%d", result, use, err, head, ideas)
	}
}
