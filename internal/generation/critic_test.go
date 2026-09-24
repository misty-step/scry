package generation

import (
	"context"
	"fmt"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func TestCriticBatchOverLimitFailsWithoutDroppingQuestions(t *testing.T) {
	ctx := context.Background()
	s := generationStore(t)
	source, err := s.Capture(ctx, store.CaptureInput{Text: "cell energy", Mode: "topic"}, "oversized-candidate-batch")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	result := store.GenerationResult{Model: "fixture", PromptVersion: "scry-questions-v1"}
	for i := 0; i <= store.MaxCriticCandidates; i++ {
		result.Quizzes = append(result.Quizzes, store.GeneratedQuiz{Kind: "recall", Prompt: fmt.Sprintf("Question %d: Which molecule carries cellular energy?", i), Answer: "ATP", Explanation: "ATP releases chemical energy when its phosphate bond changes.", Basis: "topic"})
	}
	worker := New(s, localConfig("http://127.0.0.1:1"))
	cost := int64(100)
	if err := worker.processCandidates(ctx, job, result, &cost); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || saved.Job == nil || saved.Job.Status != "failed" || saved.Job.CostMicros != 100 || len(saved.Quizzes) != 0 || saved.Job.Candidates != nil {
		t.Fatalf("oversized batch was truncated or silently accepted: %+v %v", saved, err)
	}
}
