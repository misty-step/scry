package ai

import (
	"context"
	"encoding/json"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestG11_AnthropicShapeAndSingleAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/messages" || r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Errorf("bad endpoint or auth: %s %v", r.URL.Path, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_tokens"] != float64(1024) || body["model"] != DefaultJudgeModel || body["messages"] == nil || body["output_config"] == nil {
			t.Errorf("bad request: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"temporary"}}`))
	}))
	defer srv.Close()
	c := NewClaude("local-test-key", "", "", option.WithBaseURL(srv.URL))
	if _, _, err := c.Judge(context.Background(), JudgeInput{Question: "q", Expected: "a", Answer: ""}); err == nil {
		t.Fatal("expected provider failure")
	}
	if calls.Load() != 1 {
		t.Fatalf("judge sent %d requests for one answer", calls.Load())
	}
}
func TestG11_AnthropicParsesStructuredJudgment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"claude-haiku-5-5","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":20},"content":[{"type":"text","text":"{\"verdict\":\"right\",\"reason\":\"Same idea.\",\"close\":[]}"}]}`))
	}))
	defer srv.Close()
	c := NewClaude("local-test-key", "", "", option.WithBaseURL(srv.URL))
	j, u, err := c.Judge(context.Background(), JudgeInput{Question: "q", Expected: "a", Answer: "a"})
	if err != nil || j.Reason != "Same idea." || u.InputTokens != 10 {
		t.Fatalf("%+v %+v %v", j, u, err)
	}
}

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
	c := NewClaude("local-test-key", "", "", option.WithBaseURL(srv.URL))
	_, _, err := c.Draft(context.Background(), DraftInput{Query: "physics"}, DraftSink{})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}
