package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestExpiredClaimsKeepSpendAndFenceLatePublication(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src, err := s.Capture(ctx, "Synthetic topic", "capture-once")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Capture(ctx, "Synthetic topic", "capture-once")
	if err != nil || duplicate.ID != src.ID || duplicate.Job.ID != src.Job.ID {
		t.Fatal("capture retry duplicated durable generation")
	}
	if _, err = s.Capture(ctx, "Different topic", "capture-once"); !errors.Is(err, ErrConflict) {
		t.Fatalf("capture operation reused for different input: %v", err)
	}
	first, err := s.ClaimJob(ctx, time.Second, 100, 200)
	if err != nil || first == nil {
		t.Fatalf("first claim: %v %v", first, err)
	}
	path := s.Path()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Second)
	s = openAt(t, path, now)
	if claim, err := s.ClaimJob(ctx, time.Second, 100, 200); err != nil || claim != nil {
		t.Fatalf("expired job skipped recovery delay: %v %v", claim, err)
	}
	summary, err := s.Summary(ctx)
	if err != nil || summary.CostMicros != 100 || !summary.CostUnknown {
		t.Fatalf("unknown prior spend was lost: %+v %v", summary, err)
	}
	*now = now.Add(11 * time.Second)
	if _, err = s.ClaimJob(ctx, time.Minute, 100, 199); !errors.Is(err, ErrBudget) {
		t.Fatalf("unknown prior usage did not constrain budget: %v", err)
	}
	second, err := s.ClaimJob(ctx, time.Minute, 100, 200)
	if err != nil || second == nil || second.Attempts != 2 || second.LeaseToken == first.LeaseToken {
		t.Fatalf("replacement claim lacked a new fence: %+v %v", second, err)
	}
	result := GenerationResult{Quizzes: []GeneratedQuiz{authoredChoice("Only published once?")}, Model: "authored-test-fixture", PromptVersion: "fixture-v1"}
	lateCost := int64(20)
	if err = s.CompleteJob(ctx, first.ID, first.LeaseToken, result, &lateCost); !errors.Is(err, ErrConflict) {
		t.Fatalf("late provider completion published: %v", err)
	}
	if err = s.CompleteJob(ctx, first.ID, first.LeaseToken, result, &lateCost); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate late completion changed its result: %v", err)
	}
	cost := int64(30)
	if err = s.CompleteJob(ctx, second.ID, second.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteJob(ctx, second.ID, second.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil || len(src.Quizzes) != 1 || src.Status != "ready" || src.Job.Attempts != 2 {
		t.Fatalf("publication duplicated or successor claim overwritten: %+v %v", src, err)
	}
	summary, err = s.Summary(ctx)
	if err != nil || summary.CostMicros != 50 || summary.CostUnknown {
		t.Fatalf("late actual usage reconciliation double charged: %+v %v", summary, err)
	}
	result.Quizzes[0].Prompt = "Changed final output?"
	if err = s.CompleteJob(ctx, second.ID, second.LeaseToken, result, &cost); !errors.Is(err, ErrConflict) {
		t.Fatalf("finalized attempt accepted changed output: %v", err)
	}
}

func TestFailureAccountingRetryBoundAndInvalidOutput(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src, err := s.Capture(ctx, "Synthetic bounded retry", "capture-retry")
	if err != nil {
		t.Fatal(err)
	}
	for index := range 3 {
		claim, err := s.ClaimJob(ctx, time.Minute, 100, 1000)
		if err != nil || claim == nil {
			t.Fatalf("claim %d: %+v %v", index, claim, err)
		}
		var cost *int64
		known := int64(5 + index)
		if index != 1 {
			cost = &known
		}
		if err = s.FailJob(ctx, claim.ID, claim.LeaseToken, "safe synthetic provider failure", true, cost); err != nil {
			t.Fatal(err)
		}
		if err = s.FailJob(ctx, claim.ID, claim.LeaseToken, "safe synthetic provider failure", true, cost); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(61 * time.Second)
	}
	if claim, err := s.ClaimJob(ctx, time.Minute, 100, 1000); err != nil || claim != nil {
		t.Fatalf("automatic attempts exceeded bound: %+v %v", claim, err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil || src.Job.Status != "failed" || src.Job.Attempts != 3 || src.Job.CostMicros != 112 || !src.Job.CostUnknown {
		t.Fatalf("exhaustion lost known/unknown cost: %+v %v", src, err)
	}
	retried, err := s.RetrySource(ctx, src.ID, "explicit-retry")
	if err != nil || retried.Job.ID == src.Job.ID {
		t.Fatalf("explicit retry was not durable new work: %+v %v", retried, err)
	}
	again, err := s.RetrySource(ctx, src.ID, "explicit-retry")
	if err != nil || again.Job.ID != retried.Job.ID {
		t.Fatal("explicit retry operation duplicated work")
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 1000)
	if err != nil || claim == nil {
		t.Fatalf("claim retry: %+v %v", claim, err)
	}
	cost := int64(11)
	bad := GenerationResult{Quizzes: []GeneratedQuiz{authoredChoice("Rejected content?")}, Model: "authored-test-fixture", PromptVersion: "fixture-v1"}
	bad.Quizzes[0].Answer = "not a displayed choice"
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, bad, &cost); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid output was accepted: %v", err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil || src.Job.Status != "failed" || src.Job.CostMicros != 11 || len(src.Quizzes) != 0 {
		t.Fatalf("invalid output wasn't charged and terminal: %+v %v", src, err)
	}
}

func TestSnapshotPreservesReviewAndPausesRestoredBilling(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredChoice("Snapshot question?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Submit(ctx, state.Current.ID, "snapshot-answer", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.Capture(ctx, "Uncertain synthetic pending job", "pending")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 1000)
	if err != nil || claim == nil {
		t.Fatalf("claim pending: %+v %v", claim, err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err = s.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(ctx, path); err == nil {
		t.Fatal("backup overwrote a completed snapshot")
	}
	restored := openAt(t, path, now)
	if err = restored.PauseRestoredJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if job, err := restored.ClaimJob(ctx, time.Minute, 100, 1000); err != nil || job != nil {
		t.Fatalf("restored work auto-billed: %+v %v", job, err)
	}
	paused, err := restored.Source(ctx, pending.ID)
	if err != nil || paused.Job.Status != "paused" || !paused.Job.CostUnknown || paused.Job.CostMicros != 100 {
		t.Fatalf("restore lost uncertain claim: %+v %v", paused, err)
	}
	state, err = restored.Review(ctx)
	if err != nil || state.Current == nil || state.Current.ReviewID != receipt.ReviewID {
		t.Fatalf("consistent snapshot lost acknowledged feedback: %+v %v", state, err)
	}
	original, err := s.Source(ctx, pending.ID)
	if err != nil || original.Job.Status != "running" {
		t.Fatal("isolated restore altered the original database")
	}
	if err = restored.ArchiveSource(ctx, pending.ID); err != nil {
		t.Fatal(err)
	}
	lateCost := int64(40)
	if err = restored.CompleteJob(ctx, claim.ID, claim.LeaseToken, GenerationResult{Quizzes: []GeneratedQuiz{authoredChoice("Archived output?")}, Model: "authored-test-fixture", PromptVersion: "fixture-v1"}, &lateCost); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived restored source resurrected: %v", err)
	}
}
