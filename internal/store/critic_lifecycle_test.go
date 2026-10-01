package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCriticOrphanLeasesAgeOutOfRollingAllowance(t *testing.T) {
	for _, mode := range []string{"archive", "restore", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			s, now := newTestStore(t)
			ctx := context.Background()
			j, _ := stageCritic(t, s, true, authoredChoice("Orphaned candidate?"))
			a := prepareCritic(t, s, j)[0]
			startCritic(t, s, j, a)
			switch mode {
			case "archive":
				if err := s.ArchiveSource(ctx, j.SourceID); err != nil {
					t.Fatal(err)
				}
			case "restore":
				if err := s.PauseRestoredJobs(ctx); err != nil {
					t.Fatal(err)
				}
			case "exhausted":
				if _, err := s.db.Exec(`UPDATE jobs SET attempts=3 WHERE id=?`, j.ID); err != nil {
					t.Fatal(err)
				}
			}
			*now = now.Add(25 * time.Hour)
			if _, err := legacyCapture(ctx, s, "Independent new source", "orphan-next"); err != nil {
				t.Fatal(err)
			}
			claim, err := s.ClaimJob(ctx, time.Minute, 100, 1000)
			if err != nil || claim == nil {
				t.Fatalf("expired critic held allowance forever: %+v %v", claim, err)
			}
			history, err := s.ContentHistory(ctx, j.ID)
			if err != nil || history[0].Status != "failed" || history[0].ReservedMicros != 2000 || history[0].Error != assessmentInterruptedErr {
				t.Fatal(history, err)
			}
			summary, err := s.Summary(ctx)
			if err != nil || !summary.CostUnknown || summary.CostMicros != 2200 {
				t.Fatal(summary, err)
			}
		})
	}
}

func TestCriticSavedUsageCannotBeOverwrittenByCompletionOrFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		s, _ := newTestStore(t)
		ctx := context.Background()
		j, result := stageCritic(t, s, false, authoredChoice("Immutable usage?"))
		zero := int64(0)
		var err error
		if failure {
			err = s.FailJob(ctx, j.ID, j.LeaseToken, "no cost rewrite", false, &zero)
		} else {
			err = s.CompleteJob(ctx, j.ID, j.LeaseToken, result, &zero)
		}
		if err != nil {
			t.Fatal(err)
		}
		summary, err := s.Summary(ctx)
		if err != nil || summary.CostMicros != 100 {
			t.Fatalf("usage overwritten: %+v %v", summary, err)
		}
	}
}

func TestCriticExplicitRetryKeepsBatchAndJudgedCandidates(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	j, result := stageCritic(t, s, true, authoredChoice("Already judged?"), authoredChoice("Still pending?"))
	entries := prepareCritic(t, s, j)
	token := startCritic(t, s, j, entries[0])
	if _, err := s.FinishContentAssessment(ctx, entries[0].ID, token, criticResult(t, entries[0].Candidate, "")); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseRestoredJobs(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, err := s.Source(ctx, j.SourceID)
	if err != nil || !stopped.CanRetry {
		t.Fatalf("paused candidate checks did not offer their retry: %+v %v", stopped, err)
	}
	source, err := s.RetrySource(ctx, j.SourceID, "manual-critic-retry")
	if err != nil || source.CanRetry || source.Job.ID != j.ID || source.Job.Candidates == nil || payloadHash(source.Job.Candidates.Result) != payloadHash(result) {
		t.Fatalf("manual retry discarded batch: %+v %v", source, err)
	}
	retry, err := s.ClaimJob(ctx, time.Minute, 200_000, 5000)
	if err != nil || retry == nil || retry.ReservedMicros != 0 {
		t.Fatal(retry, err)
	}
	again := prepareCritic(t, s, retry)
	if again[0].ID != entries[0].ID || again[1].ID == entries[1].ID {
		t.Fatal(again)
	}
}

func TestCandidateRetryAvailabilityUsesDurableLimits(t *testing.T) {
	for _, tc := range []struct {
		name, status, critic string
		attempts, published  int
		archived, allowed    bool
	}{
		{"paused", "paused", "pending", 2, 0, false, true},
		{"failed", "failed", "pending", 2, 0, false, true},
		{"exhausted", "paused", "pending", 3, 0, false, false},
		{"rejected", "paused", "judged", 2, 0, false, false},
		{"published", "paused", "pending", 2, 1, false, false},
		{"archived", "paused", "pending", 2, 0, true, false},
		{"working", "running", "pending", 2, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			ctx := context.Background()
			j, _ := stageCritic(t, s, true, authoredChoice("Saved candidate?"))
			if err := s.PauseRestoredJobs(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE jobs SET status=?,attempts=?,critic_status=?,published=? WHERE id=?`, tc.status, tc.attempts, tc.critic, tc.published, j.ID); err != nil {
				t.Fatal(err)
			}
			if tc.archived {
				if err := s.ArchiveSource(ctx, j.SourceID); err != nil {
					t.Fatal(err)
				}
			}
			src, err := s.Source(ctx, j.SourceID)
			if err != nil || src.CanRetry != tc.allowed {
				t.Fatalf("candidate retry availability = %v, want %v: %+v %v", src.CanRetry, tc.allowed, src, err)
			}
			retried, err := s.RetrySource(ctx, j.SourceID, "candidate-explicit-retry")
			if !tc.allowed {
				if !errors.Is(err, ErrConflict) {
					t.Fatalf("unavailable candidate retry passed the transaction: %v", err)
				}
			} else if err != nil || retried.CanRetry || retried.Job.ID != j.ID || retried.Job.Candidates == nil || retried.Job.Status != "queued" {
				t.Fatalf("available candidate retry did not resume its saved batch: %+v %v", retried, err)
			} else {
				claim, claimErr := s.ClaimJob(ctx, time.Minute, 200_000, 5000)
				if claimErr != nil || claim == nil || claim.ID != j.ID || claim.Attempts != 3 || claim.ReservedMicros != 0 {
					t.Fatalf("displayed retry could not claim its saved candidates without another generation reservation: %+v %v", claim, claimErr)
				}
			}
		})
	}
}
