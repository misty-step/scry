package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

func TestCriticHTTPWorkerPersistsBeforeSendAndRetriesOnlyCritic(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	var generationCalls, criticCalls atomic.Int32
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generationCalls.Add(1)
		io.WriteString(w, envelopeJSON(t, outputJSON(t, "concepts", topicDraft()), "stop", json.RawMessage(`0.001`)))
	}))
	defer generator.Close()
	critic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := criticCalls.Add(1)
		var request semantic.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		// A real HTTP boundary, real SQLite, no transaction held across the call.
		saved, err := s.Source(context.Background(), source.ID)
		if err != nil || saved.Job.Candidates == nil || saved.Job.CriticStatus != "pending" || len(saved.Quizzes) != 0 {
			t.Errorf("before transmission: %+v %v", saved, err)
		}
		history, err := s.ContentHistory(context.Background(), job.ID)
		if err != nil || len(history) != int(call) || history[len(history)-1].ReservedMicros != 2000 || history[len(history)-1].Transmissions != 1 {
			t.Errorf("no reservation/lease: %+v %v", history, err)
		}
		if call == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		answers := map[string]any{}
		for key := range request.Questions {
			answers[key] = map[string]any{"type": "noul", "noul": .01}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "fixture-critic", "answers": answers, "usage": map[string]any{"cost": .00001, "input_tokens": 100, "output_tokens": 1}})
	}))
	defer critic.Close()
	cfg := localConfig(generator.URL)
	cfg.Critic = semantic.NewClient(semantic.Config{Endpoint: critic.URL})
	cfg.CriticSpending = store.SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 1_000_000}
	worker := New(s, cfg)
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil || saved.Job.Status != "retry" || saved.Job.CriticStatus != "pending" || saved.Job.Candidates == nil || len(saved.Quizzes) != 0 {
		t.Fatalf("failed critic: %+v %v", saved, err)
	}
	// Exercise the actual automatic retry deadline, not a fabricated claim.
	time.Sleep(10*time.Second + 20*time.Millisecond)
	retry, err := s.ClaimJob(context.Background(), jobLease, 200_000, 5000)
	if err != nil || retry == nil || retry.ReservedMicros != 0 {
		t.Fatalf("critic-only claim: %+v %v", retry, err)
	}
	// Generator config can disappear: saved candidates still take critic path.
	worker.configError = "generation deliberately unavailable"
	if err := worker.process(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	saved, err = s.Source(context.Background(), source.ID)
	if err != nil || len(saved.Quizzes) != 1 || saved.Job.CriticStatus != "judged" || generationCalls.Load() != 1 || criticCalls.Load() != 2 || saved.Job.CostMicros != 3010 || !saved.Job.CostUnknown {
		t.Fatalf("retry: %+v calls=%d/%d %v", saved, generationCalls.Load(), criticCalls.Load(), err)
	}
}

type countCritic struct{ calls int }

func (c *countCritic) Decide(ctx context.Context, req semantic.Request) (semantic.Response, error) {
	c.calls++
	answers := map[string]any{}
	for key := range req.Questions {
		answers[key] = map[string]any{"noul": .01}
	}
	raw, _ := json.Marshal(map[string]any{"answers": answers})
	cost := int64(1)
	return semantic.Response{Model: "synthetic-critic", Raw: raw, Usage: semantic.Usage{CostMicros: &cost}}, nil
}

func TestCriticBatchLimitIsTwelveAndHonestPartial(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	result := store.GenerationResult{Model: "authored-fixture", PromptVersion: "fixture-v1"}
	for _, prompt := range []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen"} {
		result.Quizzes = append(result.Quizzes, store.GeneratedQuiz{Kind: "recall", Prompt: prompt + ": Which molecule carries cellular energy?", Answer: "ATP", Explanation: "ATP releases chemical energy when its terminal phosphate bond is hydrolyzed.", Basis: "topic"})
	}
	critic := &countCritic{}
	worker := New(s, Config{Critic: critic, CriticSpending: store.SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 1_000_000}})
	cost := int64(100)
	if err := worker.processCandidates(context.Background(), job, result, &cost); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil || len(saved.Quizzes) != 12 || critic.calls != 12 || saved.Job.Status != "partial" || len(saved.Job.Candidates.Result.Quizzes) != 12 {
		t.Fatalf("batch: %+v calls=%d %v", saved, critic.calls, err)
	}
}
