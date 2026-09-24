package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

func candidateState(q store.GeneratedQuiz) semantic.CandidateState {
	return semantic.CandidateState{Prompt: q.Prompt, Answer: q.Answer, Explanation: q.Explanation, Choices: q.Choices, Rubric: q.Rubric, Basis: q.Basis, Evidence: q.Evidence, Kind: q.Kind, Grading: q.Grading}
}

func (w *Worker) processCandidates(ctx context.Context, job *store.Job, result store.GenerationResult, cost *int64) error {
	if job.Candidates == nil {
		if len(result.Quizzes) > store.MaxCriticCandidates {
			return w.finishCandidates(ctx, job, result, cost, "Too many questions were returned for one study batch. Nothing was published; usage was retained.", false)
		}
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
		err := w.store.SaveCandidates(settleCtx, job.ID, job.LeaseToken, result, cost, w.cfg.Critic != nil)
		cancel()
		if err != nil {
			if !errors.Is(err, store.ErrConflict) && !errors.Is(err, store.ErrInvalid) && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("save critic candidates: %w", err)
			}
			// SaveCandidates never publishes. Settle paid usage even on a stale
			// or invalid generator completion, exactly as the old worker did.
			return w.finishCandidates(ctx, job, result, cost, "Validated candidates could not be saved; source changed or result was invalid.", false)
		}
		job.CriticStatus = "skipped"
		if w.cfg.Critic != nil {
			job.CriticStatus = "pending"
		}
	}
	if job.CriticStatus == "skipped" {
		return w.finishCandidates(ctx, job, result, cost, "", false)
	}
	if w.cfg.Critic == nil {
		return w.finishCandidates(ctx, job, result, cost, "Content critic is unavailable; saved candidates await assessment.", true)
	}
	assessments, err := w.store.PrepareContentAssessments(ctx, job.ID, job.LeaseToken)
	if errors.Is(err, store.ErrConflict) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, assessment := range assessments {
		if assessment.Status == "judged" {
			continue
		}
		if assessment.Status == "failed" || ctx.Err() != nil {
			return w.finishCandidates(ctx, job, result, cost, "Content critic pending; saved candidates and prior usage are retained.", true)
		}
		request := semantic.BuildCriticRequest(w.cfg.CriticModel, candidateState(assessment.Candidate))
		encoded, err := json.Marshal(request)
		if err != nil {
			return err
		}
		token, send, err := w.store.BeginContentTransmission(ctx, assessment.ID, job.LeaseToken, request.Model, encoded, w.cfg.CriticSpending)
		if errors.Is(err, store.ErrConflict) {
			return nil
		} // Another caller owns the send; do not retire its job.
		if errors.Is(err, store.ErrBudget) || (err == nil && !send) {
			return w.finishCandidates(ctx, job, result, cost, "Content critic pending; allowance or interrupted assessment requires a new attempt.", true)
		}
		if err != nil {
			return err
		}
		response, callErr := w.cfg.Critic.Decide(ctx, request)
		outcome := store.AssessmentResult{ResponseModel: response.Model, ResponseJSON: response.Raw, InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, CostMicros: response.Usage.CostMicros, LatencyMS: response.LatencyMS}
		if callErr != nil {
			outcome.Error = "malformed"
			switch {
			case errors.Is(callErr, semantic.ErrNotConfigured):
				outcome.Error, outcome.NoSend = "unconfigured", true
			case errors.Is(callErr, semantic.ErrUnavailable):
				outcome.Error = "unavailable"
			case errors.Is(callErr, semantic.ErrRejected):
				outcome.Error = "rejected"
			}
		}
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
		decision, finishErr := w.store.FinishContentAssessment(settleCtx, assessment.ID, token, outcome)
		cancel()
		if finishErr != nil {
			return finishErr
		}
		if decision.Decision == "ungraded" {
			return w.finishCandidates(ctx, job, result, cost, "Content critic pending; could not judge saved candidates. Prior usage remains accounted.", true)
		}
	}
	return w.finishCandidates(ctx, job, result, cost, "", false)
}

func (w *Worker) finishCandidates(ctx context.Context, job *store.Job, result store.GenerationResult, cost *int64, failure string, retry bool) error {
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	var err error
	if failure != "" {
		err = w.store.FailJob(settleCtx, job.ID, job.LeaseToken, failure, retry, cost)
	} else {
		err = w.store.CompleteJob(settleCtx, job.ID, job.LeaseToken, result, cost)
	}
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("settle content critic: %w", err)
	}
	return nil
}
