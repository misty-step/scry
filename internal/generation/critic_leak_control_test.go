package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

// This is the unchanged candidate state from the live Jev false accept at
// 8f817fc: prompt_leaks_answer=0.79. The full product path has an earlier
// deterministic validator. Keep the direct model failure, but prove whether
// this particular candidate can actually reach publication.
func TestCriticLeakedLiveControlCannotPublishThroughGeneration(t *testing.T) {
	raw, err := os.ReadFile("testdata/critic-leaked-control.json")
	if err != nil {
		t.Fatal(err)
	}
	var draft quizDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Kind = "recall"
	draft.Choices, draft.Variants, draft.Covers = []string{}, []string{}, []string{}
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	generator := responseServer(t, envelopeJSON(t, outputJSON(t, "concepts", draft), "stop", json.RawMessage(`0.001`)), http.StatusOK)
	critic := &countCritic{} // Would accept if called: defense is before criticism.
	cfg := localConfig(generator.URL)
	cfg.Critic = critic
	cfg.CriticSpending = store.SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 1_000_000}
	worker := New(s, cfg)
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessments, err := s.ContentHistory(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Quizzes) != 0 || saved.Job.Candidates != nil || critic.calls != 0 || len(assessments) != 0 || !strings.Contains(saved.Job.Error, "answer_leakage_or_vague_prompt") {
		t.Fatalf("leaked live control escaped validation: %+v critic_calls=%d assessments=%d", saved, critic.calls, len(assessments))
	}
	if saved.Job.CostMicros != 1000 || saved.Job.CostUnknown {
		t.Fatal("generation usage lost", saved.Job)
	}
	t.Logf("same live leaked candidate: published=%d staged_candidates=%v critic_calls=%d content_assessments=%d status=%s recorded_cost_micros=%d reason=%s", len(saved.Quizzes), saved.Job.Candidates != nil, critic.calls, len(assessments), saved.Job.Status, saved.Job.CostMicros, saved.Job.Error)
}
