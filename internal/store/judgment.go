package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DedupePolicyVersion identifies Jev "same concept?" judgments made while a
// plan job decides whether a proposed concept reuses one the learner has.
const DedupePolicyVersion = "dedupe-v1"

// BeginJudgment grants exactly one send for one generation-time judgment in
// the current job attempt, reserving its allowance first. Each send also keeps
// the owning job's lease alive for the rest of the attempt. A second call for
// the same attempt and index returns send=false and never resends.
func (s *Store) BeginJudgment(ctx context.Context, jobID, jobToken, purpose string, index int, subject any, model string, request []byte, spending SemanticSpending) (string, string, bool, error) {
	if purpose != "dedupe" {
		return "", "", false, fmt.Errorf("%w: unknown judgment purpose", ErrInvalid)
	}
	if validText("judgment model", model, 200, true) != nil || len(request) == 0 || len(request) > 256<<10 || !json.Valid(request) ||
		spending.ReservationMicros <= 0 || spending.ReservationMicros > spending.DailyBudgetMicros || index < 0 || index > MaxPlanConcepts*4 {
		return "", "", false, ErrInvalid
	}
	encoded, err := marshal(subject)
	if err != nil || len(encoded) > 64<<10 {
		return "", "", false, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", false, err
	}
	defer tx.Rollback()
	now := s.now()
	owner, err := claimAttempt(ctx, tx, jobID, jobToken, now)
	if err != nil {
		return "", "", false, err
	}
	if !owner.live {
		return "", "", false, ErrConflict
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO content_assessments(id,job_id,attempt_token,candidate_index,candidate_json,status,policy_version,request_model,request_json,created_at,purpose)
	 VALUES(?,?,?,?,?,'pending',?,'','{}',?,?)`, id, jobID, jobToken, index, encoded, DedupePolicyVersion, now, purpose)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return "", "", false, tx.Commit()
		}
		return "", "", false, err
	}
	spent, err := spentMicros(ctx, tx, now)
	if err != nil {
		return "", "", false, err
	}
	if spent > spending.DailyBudgetMicros || spending.ReservationMicros > spending.DailyBudgetMicros-spent {
		if _, err = tx.ExecContext(ctx, `UPDATE content_assessments SET status='failed',decision='ungraded',error=?,cost_micros=0,finished_at=? WHERE id=?`, assessmentAllowanceErr, now, id); err != nil {
			return "", "", false, err
		}
		if err = tx.Commit(); err != nil {
			return "", "", false, err
		}
		return "", "", false, ErrBudget
	}
	token := newID()
	if _, err = tx.ExecContext(ctx, `UPDATE content_assessments SET request_model=?,request_json=?,transmissions=1,reserved_micros=?,lease_token=?,lease_until=? WHERE id=?`,
		model, string(request), spending.ReservationMicros, token, now+semanticLease.Milliseconds(), id); err != nil {
		return "", "", false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET lease_until=max(lease_until,?) WHERE id=?`, now+criticJobLease.Milliseconds(), jobID); err != nil {
		return "", "", false, err
	}
	return id, token, true, tx.Commit()
}

// FinishJudgment records a generation-time judgment's outcome and usage.
// decision is the caller's policy result (for dedupe: "same:<concept id>" or
// "new"); a failure records "ungraded". Foreign tokens change nothing.
func (s *Store) FinishJudgment(ctx context.Context, id, token string, result AssessmentResult, decision string) error {
	if result.CostMicros != nil && *result.CostMicros < 0 {
		return ErrInvalid
	}
	if validText("judgment decision", decision, 200, false) != nil || result.InputTokens < 0 || result.OutputTokens < 0 || result.LatencyMS < 0 {
		return ErrInvalid
	}
	response, err := nullableJSON(result.ResponseJSON)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := readContent(ctx, tx, id)
	if err != nil {
		return err
	}
	if a.Status != "pending" {
		return tx.Commit()
	}
	if !ownsLease(Assessment{Status: a.Status, Transmissions: a.Transmissions, LeaseToken: a.LeaseToken}, token) {
		if err = reconcileContent(ctx, tx, &a, s.now()); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrConflict
	}
	status, failure := "judged", result.Error
	if failure != "" || decision == "" {
		status, decision = "failed", "ungraded"
		if failure == "" {
			failure = "malformed"
		}
	}
	reserved := settledReservation(Assessment{ReservedMicros: a.ReservedMicros}, result)
	cost := result.CostMicros
	if result.NoSend {
		zero := int64(0)
		cost, reserved = &zero, 0
	}
	_, err = tx.ExecContext(ctx, `UPDATE content_assessments SET status=?,response_model=?,response_json=?,decision=?,reasons='[]',error=?,input_tokens=?,output_tokens=?,
	 cost_micros=?,latency_ms=?,reserved_micros=?,lease_token='',lease_until=0,finished_at=? WHERE id=?`,
		status, result.ResponseModel, response, decision, failure, nullableCount(result.InputTokens), nullableCount(result.OutputTokens), cost, nullableLatency(result.LatencyMS), reserved, s.now(), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
