package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

const MaxCriticCandidates = 12
const criticJobLease = 2 * time.Minute

// SaveCandidates is the durable boundary between generation and criticism.
// It preserves the paid generator result, settles its reservation, and keeps the
// job lease for publication. Replays cannot alter the candidates or skip policy.
func (s *Store) SaveCandidates(ctx context.Context, jobID, token string, result GenerationResult, cost *int64, configured bool) error {
	if cost != nil && *cost < 0 {
		return ErrInvalid
	}
	if configured && len(result.Quizzes) > MaxCriticCandidates {
		return fmt.Errorf("%w: critic batch exceeds twelve candidates", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := claimAttempt(ctx, tx, jobID, token, now)
	if err != nil {
		return err
	}
	if !a.live || a.job.Foundation {
		return ErrConflict
	}
	batch := CandidateBatch{Result: result, CostMicros: cost}
	if a.job.Candidates != nil {
		if payloadHash(batch) != payloadHash(*a.job.Candidates) {
			return ErrConflict
		}
		return tx.Commit()
	}
	if err = validateGeneration(result, Source{Text: a.job.SourceText, Kind: a.job.SourceKind}); err != nil {
		return err
	}
	encoded, err := marshal(batch)
	if err != nil {
		return err
	}
	status := "skipped"
	if configured {
		status = "pending"
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET candidates_json=?,critic_status=?,model=?,prompt_version=?,lease_until=?,updated_at=? WHERE id=?`, encoded, status, result.Model, result.PromptVersion, now+criticJobLease.Milliseconds(), now, jobID)
	if err != nil {
		return err
	}
	// Keep the job-attempt ownership active, but record generation usage now.
	_, err = tx.ExecContext(ctx, `UPDATE job_attempts SET cost_micros=?,reserved_micros=CASE WHEN ? IS NULL THEN reserved_micros ELSE 0 END WHERE token=?`, cost, cost, token)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type ContentAssessment struct {
	ID             string          `json:"id"`
	JobID          string          `json:"job_id"`
	AttemptToken   string          `json:"-"`
	CandidateIndex int             `json:"candidate_index"`
	Candidate      GeneratedQuiz   `json:"candidate"`
	Status         string          `json:"status"`
	PolicyVersion  string          `json:"policy_version"`
	Transmissions  int             `json:"transmissions"`
	ReservedMicros int64           `json:"reserved_micros"`
	LeaseToken     string          `json:"-"`
	LeaseUntil     int64           `json:"lease_until"`
	Decision       string          `json:"decision"`
	Reasons        string          `json:"reasons"`
	Error          string          `json:"error"`
	ResponseJSON   json.RawMessage `json:"response"`
}

func CriticParams(q GeneratedQuiz) learning.CriticParams {
	return learning.CriticParams{Source: q.Basis == "source", Choice: q.Kind == "choice", Semantic: q.Grading == "semantic"}
}

const contentColumns = `id,job_id,attempt_token,candidate_index,candidate_json,status,policy_version,transmissions,reserved_micros,lease_token,lease_until,decision,reasons,error,COALESCE(response_json,'null')`

func scanContent(row interface{ Scan(...any) error }) (ContentAssessment, error) {
	var a ContentAssessment
	var candidate, response string
	err := row.Scan(&a.ID, &a.JobID, &a.AttemptToken, &a.CandidateIndex, &candidate, &a.Status, &a.PolicyVersion, &a.Transmissions, &a.ReservedMicros, &a.LeaseToken, &a.LeaseUntil, &a.Decision, &a.Reasons, &a.Error, &response)
	if err != nil {
		return a, err
	}
	a.ResponseJSON = json.RawMessage(response)
	err = json.Unmarshal([]byte(candidate), &a.Candidate)
	return a, err
}

func readContent(ctx context.Context, tx *sql.Tx, id string) (ContentAssessment, error) {
	return scanContent(tx.QueryRowContext(ctx, `SELECT `+contentColumns+` FROM content_assessments WHERE id=?`, id))
}

func latestContent(ctx context.Context, tx *sql.Tx, jobID string, index int) (ContentAssessment, error) {
	return scanContent(tx.QueryRowContext(ctx, `SELECT `+contentColumns+` FROM content_assessments WHERE job_id=? AND candidate_index=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, jobID, index))
}

// PrepareContentAssessments reuses judged results. A retry may create a NEW row
// for an unjudged candidate, but cannot reuse a transmitted row for another send.
func (s *Store) PrepareContentAssessments(ctx context.Context, jobID, token string) ([]ContentAssessment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := claimAttempt(ctx, tx, jobID, token, now)
	if err != nil {
		return nil, err
	}
	if !a.live || a.job.Candidates == nil || a.job.CriticStatus != "pending" {
		return nil, ErrConflict
	}
	quizzes := a.job.Candidates.Result.Quizzes
	if len(quizzes) == 0 || len(quizzes) > MaxCriticCandidates {
		return nil, ErrInvalid
	}
	assessments := make([]ContentAssessment, 0, len(quizzes))
	for index, candidate := range quizzes {
		prior, readErr := latestContent(ctx, tx, jobID, index)
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return nil, readErr
		}
		if readErr == nil {
			if err = reconcileContent(ctx, tx, &prior, now); err != nil {
				return nil, err
			}
			if prior.Status == "judged" || prior.AttemptToken == token {
				assessments = append(assessments, prior)
				continue
			}
			if prior.Status == "pending" && prior.LeaseUntil > now {
				return nil, ErrConflict
			}
			// Superseded, untransmitted work carries no paid outcome.
			if prior.Status == "pending" {
				if _, err = tx.ExecContext(ctx, `UPDATE content_assessments SET status='failed',error='superseded before transmission',finished_at=? WHERE id=?`, now, prior.ID); err != nil {
					return nil, err
				}
			}
		}
		encoded, err := marshal(candidate)
		if err != nil {
			return nil, err
		}
		id := newID()
		_, err = tx.ExecContext(ctx, `INSERT INTO content_assessments(id,job_id,attempt_token,candidate_index,candidate_json,status,policy_version,request_model,request_json,created_at) VALUES(?,?,?,?,?,'pending',?,'','{}',?)`, id, jobID, token, index, encoded, learning.CriticPolicyVersion, now)
		if err != nil {
			return nil, err
		}
		entry, err := readContent(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		assessments = append(assessments, entry)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET lease_until=? WHERE id=?`, now+criticJobLease.Milliseconds(), jobID); err != nil {
		return nil, err
	}
	return assessments, tx.Commit()
}

func reconcileContent(ctx context.Context, tx *sql.Tx, a *ContentAssessment, now int64) error {
	if a.Status != "pending" || a.Transmissions == 0 || a.LeaseUntil > now {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE content_assessments SET status='failed',decision='ungraded',error=?,lease_token='',lease_until=0,finished_at=? WHERE id=?`, assessmentInterruptedErr, now, a.ID)
	if err == nil {
		a.Status, a.Decision, a.Error, a.LeaseToken, a.LeaseUntil = "failed", "ungraded", assessmentInterruptedErr, "", 0
	}
	return err
}

// BeginContentTransmission is the sole reservation/send authorization point.
func (s *Store) BeginContentTransmission(ctx context.Context, id, jobToken, model string, request []byte, spending SemanticSpending) (string, bool, error) {
	if validText("critic model", model, 200, true) != nil || len(request) == 0 || len(request) > 256<<10 || !json.Valid(request) || spending.ReservationMicros <= 0 || spending.ReservationMicros > spending.DailyBudgetMicros {
		return "", false, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := readContent(ctx, tx, id)
	if err != nil {
		return "", false, err
	}
	if err = reconcileContent(ctx, tx, &a, now); err != nil {
		return "", false, err
	}
	if a.Status != "pending" {
		return "", false, tx.Commit()
	}
	if a.LeaseUntil > now {
		return "", false, ErrConflict
	}
	owner, err := claimAttempt(ctx, tx, a.JobID, jobToken, now)
	if err != nil {
		return "", false, err
	}
	if !owner.live || a.AttemptToken != jobToken || owner.job.CriticStatus != "pending" {
		return "", false, ErrConflict
	}
	spent, err := spentMicros(ctx, tx, now)
	if err != nil {
		return "", false, err
	}
	if spent > spending.DailyBudgetMicros || spending.ReservationMicros > spending.DailyBudgetMicros-spent {
		_, err = tx.ExecContext(ctx, `UPDATE content_assessments SET status='failed',decision='ungraded',error=?,cost_micros=0,finished_at=? WHERE id=?`, assessmentAllowanceErr, now, id)
		if err != nil {
			return "", false, err
		}
		if err = tx.Commit(); err != nil {
			return "", false, err
		}
		return "", false, ErrBudget
	}
	token := newID()
	_, err = tx.ExecContext(ctx, `UPDATE content_assessments SET request_model=?,request_json=?,transmissions=1,reserved_micros=?,lease_token=?,lease_until=? WHERE id=? AND transmissions=0`, model, string(request), spending.ReservationMicros, token, now+semanticLease.Milliseconds(), id)
	if err != nil {
		return "", false, err
	}
	return token, true, tx.Commit()
}

// contentDecision reads only Noul heads. Publication recomputes the policy from
// the stored response and candidate, not a provider or caller verdict string.
func contentDecision(raw []byte, candidate GeneratedQuiz) learning.CriticDecision {
	var envelope struct {
		Answers map[string]struct {
			Type          string             `json:"type"`
			Noul          *float64           `json:"noul"`
			Choice        string             `json:"choice"`
			Score         *float64           `json:"score"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
	}
	values := map[string]float64{}
	if json.Unmarshal(raw, &envelope) == nil {
		for key, answer := range envelope.Answers {
			if answer.Noul == nil || (answer.Type != "" && answer.Type != "noul") || answer.Choice != "" || answer.Score != nil || len(answer.Probabilities) != 0 {
				return learning.CriticDecision{Decision: "ungraded"}
			}
			values[key] = *answer.Noul
		}
	}
	return learning.JudgeCandidate(values, CriticParams(candidate))
}

// FinishContentAssessment records usage even if the source/job has changed.
// Publication owns the separate source/lease fence. Foreign tokens never write.
func (s *Store) FinishContentAssessment(ctx context.Context, id, token string, result AssessmentResult) (learning.CriticDecision, error) {
	if result.CostMicros != nil && *result.CostMicros < 0 {
		return learning.CriticDecision{}, ErrInvalid
	}
	response, err := nullableJSON(result.ResponseJSON)
	if err != nil {
		return learning.CriticDecision{}, err
	}
	if result.InputTokens < 0 || result.OutputTokens < 0 || result.LatencyMS < 0 {
		return learning.CriticDecision{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return learning.CriticDecision{}, err
	}
	defer tx.Rollback()
	a, err := readContent(ctx, tx, id)
	if err != nil {
		return learning.CriticDecision{}, err
	}
	decision := contentDecision(a.ResponseJSON, a.Candidate)
	if a.Status != "pending" {
		return decision, tx.Commit()
	}
	if !ownsLease(Assessment{Status: a.Status, Transmissions: a.Transmissions, LeaseToken: a.LeaseToken}, token) {
		if err = reconcileContent(ctx, tx, &a, s.now()); err != nil {
			return decision, err
		}
		if err = tx.Commit(); err != nil {
			return decision, err
		}
		return learning.CriticDecision{Decision: "ungraded"}, ErrConflict
	}
	decision = contentDecision(result.ResponseJSON, a.Candidate)
	status, failure := "judged", result.Error
	if failure != "" || decision.Decision == "ungraded" {
		status = "failed"
		decision = learning.CriticDecision{Decision: "ungraded", Reasons: []string{}}
		if failure == "" {
			failure = "malformed"
		}
	}
	reasons, err := marshal(decision.Reasons)
	if err != nil {
		return decision, err
	}
	reserved := settledReservation(Assessment{ReservedMicros: a.ReservedMicros}, result)
	cost := result.CostMicros
	if result.NoSend {
		zero := int64(0)
		cost, reserved = &zero, 0
	}
	_, err = tx.ExecContext(ctx, `UPDATE content_assessments SET status=?,response_model=?,response_json=?,decision=?,reasons=?,error=?,input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,reserved_micros=?,lease_token='',lease_until=0,finished_at=? WHERE id=?`, status, result.ResponseModel, response, decision.Decision, reasons, failure, result.InputTokens, result.OutputTokens, cost, result.LatencyMS, reserved, s.now(), id)
	if err != nil {
		return decision, err
	}
	return decision, tx.Commit()
}

func judgedCandidates(ctx context.Context, tx *sql.Tx, job Job) (GenerationResult, error) {
	result := job.Candidates.Result
	result.Quizzes = nil
	rejected := 0
	for index, candidate := range job.Candidates.Result.Quizzes {
		a, err := latestContent(ctx, tx, job.ID, index)
		if errors.Is(err, sql.ErrNoRows) {
			return result, fmt.Errorf("%w: candidate is not judged", ErrConflict)
		}
		if err != nil {
			return result, err
		}
		decision := contentDecision(a.ResponseJSON, candidate)
		if a.Status != "judged" || a.PolicyVersion != learning.CriticPolicyVersion || payloadHash(a.Candidate) != payloadHash(candidate) || decision.Decision == "ungraded" {
			return result, fmt.Errorf("%w: candidate is not judged", ErrConflict)
		}
		if decision.Decision == "accept" {
			result.Quizzes = append(result.Quizzes, candidate)
		} else {
			rejected++
		}
	}
	if rejected > 0 {
		result.Partial = true
		result.Note += fmt.Sprintf(" Critic rejected %d candidate(s); defect reasons and paid usage are retained.", rejected)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET critic_status='judged' WHERE id=?`, job.ID); err != nil {
		return result, err
	}
	return result, nil
}

// ContentHistory includes every candidate attempt, including rejected and
// ungraded ones. Review-event history remains the immutable learner history.
func (s *Store) ContentHistory(ctx context.Context, jobID string) ([]ContentAssessment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+contentColumns+` FROM content_assessments WHERE job_id=? ORDER BY created_at,rowid`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ContentAssessment{}
	for rows.Next() {
		a, err := scanContent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
