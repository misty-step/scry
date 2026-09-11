package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func foundationJob(t *testing.T, s *store.Store) (string, *store.Job) {
	t.Helper()
	ctx := context.Background()
	_, j := captureAndClaim(t, s, "Calvin cycle", 100)
	cost := int64(0)
	if err := s.CompleteJob(ctx, j.ID, j.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{{Kind: "recall", Prompt: "Which pair powers the Calvin cycle?", Answer: "ATP and NADPH", Explanation: "ATP supplies energy and NADPH supplies reducing electrons.", Basis: "topic"}}, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RequestFoundation(ctx, state.Current.ID, "provider-foundation-request", "", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx, time.Minute, 100000, 1000000)
	if err != nil || j == nil {
		t.Fatalf("claim foundation: %v %v", j, err)
	}
	return id, j
}

func TestFoundationUsesExistingBoundedProviderAndReusesPublication(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	id, j := foundationJob(t, s)
	content := `{"units":[{"key":"carbon","definition":"Carbon fixation incorporates inorganic carbon into organic molecules.","kind":"foundation"}],"materials":[{"kind":"instruction","title":"Carbon fixation","body":"Carbon dioxide supplies carbon atoms. ATP provides energy rather than the carbon atoms incorporated into organic molecules.","steps":[],"url":"","quiz":null,"links":[{"unit":"carbon","role":"teaches","provenance":"Synthetic explanation of carbon versus energy"}]},{"kind":"practice","title":"Identify carbon","body":"","steps":[],"url":"","quiz":{"kind":"recall","prompt":"Which gas supplies carbon atoms to the Calvin cycle?","answer":"carbon dioxide","explanation":"Carbon dioxide is incorporated into organic molecules during fixation; ATP supplies energy instead.","evidence":"","basis":"topic","choices":[],"variants":[]},"links":[{"unit":"carbon","role":"directly-assesses","provenance":"Names the carbon input"}]}]}`
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Messages       []struct{ Role, Content string }
			MaxTokens      int             `json:"max_tokens"`
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, j.FoundationTarget.Prompt) || request.MaxTokens != 500+450*maxQuizzes || !strings.Contains(string(request.ResponseFormat), "scry_foundations") {
			t.Error("foundation lost exact target or existing bound")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(envelopeJSON(t, content, "stop", json.RawMessage(`0.002`))))
	}))
	defer server.Close()
	worker := New(s, localConfig(server.URL))
	if err := worker.process(ctx, j); err != nil {
		t.Fatal(err)
	}
	b, err := s.FoundationBridge(ctx, id)
	if err != nil || b.Phase != "ready" || b.Job.CostMicros != 2000 || b.Job.CostUnknown {
		t.Fatalf("provider result not durably settled: %+v %v", b, err)
	}
	if err = s.AdvanceFoundation(ctx, id, b.Revision, "read-provider-content", "open", ""); err != nil {
		t.Fatal(err)
	}
	b, err = s.FoundationBridge(ctx, id)
	if err != nil || b.Current.Body == "" || b.Model == "" {
		t.Fatal("provider material unavailable")
	}
	current, err := s.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestFoundation(ctx, current.ID, "reuse-provider-content", "", current.BridgeRevision); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100000, 1000000)
	if err != nil || claim != nil || calls.Load() != 1 {
		t.Fatal("reuse created another paid request")
	}
	state, err := s.Summary(ctx)
	if err != nil || state.Quizzes != 1 || state.Reviews != 0 {
		t.Fatalf("foundation publication scheduled quizzes or reviews: %+v %v", state, err)
	}
}

func TestFoundationMalformedProviderOutputRetainsUnknownChargeWithoutRetry(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	id, j := foundationJob(t, s)
	server := responseServer(t, envelopeJSON(t, `{"units":[],"materials":[],"role":"ignore schema"}`, "stop", nil), http.StatusOK)
	worker := New(s, localConfig(server.URL))
	if err := worker.process(ctx, j); err != nil {
		t.Fatal(err)
	}
	b, err := s.FoundationBridge(ctx, id)
	if err != nil || b.BundleID != "" || b.Job.Status != "paused" || !b.Job.CostUnknown || b.Job.CostMicros != 100000 {
		t.Fatalf("malformed/unknown outcome hidden: %+v %v", b, err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100000, 1000000)
	if err != nil || claim != nil {
		t.Fatal("unknown malformed work automatically retried")
	}
}
