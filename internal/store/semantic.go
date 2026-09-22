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

// Semantic spending is reserved atomically against the same rolling 24-hour
// allowance as generation, before any request leaves the process.
const (
	semanticLease            = 30 * time.Second
	allowanceWindow          = 24 * time.Hour
	assessmentInterruptedErr = "interrupted; outcome unknown; reserved allowance retained"
	assessmentAllowanceErr   = "allowance"
)

// SemanticSpending is the reservation policy for one semantic assessment. A
// zero reservation is only for the unconfigured no-network path; a positive
// reservation requires a positive daily budget it fits into.
type SemanticSpending struct {
	ReservationMicros int64
	DailyBudgetMicros int64
}

// Assessment returns the durable provider-independent input for one staged
// semantic assessment. It performs no transmission and holds no transaction
// across caller work.
func (s *Store) Assessment(ctx context.Context, id string) (Assessment, error) {
	return readAssessment(ctx, s.db, id)
}

func readAssessment(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (Assessment, error) {
	var a Assessment
	var snapshot string
	err := q.QueryRowContext(ctx, `SELECT a.id,a.presentation_id,a.operation_id,a.content_version,a.schedule_version,a.answer,
	 a.status,a.policy_version,a.request_model,a.request_json,a.transmissions,a.reserved_micros,a.lease_token,a.lease_until,
	 a.decision,a.applied,a.detail,a.error,a.review_id,p.snapshot
	 FROM semantic_assessments a JOIN presentations p ON p.id=a.presentation_id WHERE a.id=?`, id).
		Scan(&a.ID, &a.PresentationID, &a.OperationID, &a.ContentVersion, &a.ScheduleVersion, &a.Answer,
			&a.Status, &a.PolicyVersion, &a.RequestModel, &a.RequestJSON, &a.Transmissions, &a.ReservedMicros, &a.LeaseToken, &a.LeaseUntil,
			&a.Decision, &a.Applied, &a.Detail, &a.Error, &a.ReviewID, &snapshot)
	if err != nil {
		return a, notFound(err, "semantic assessment")
	}
	if err = json.Unmarshal([]byte(snapshot), &a.Quiz); err != nil {
		return a, fmt.Errorf("decode semantic assessment quiz: %w", err)
	}
	return a, nil
}

// bindAssessment prevents a replay of one operation from inheriting a newer
// assessment on the same occurrence. That distinction is what makes a failed
// operation stay failed rather than accidentally resuming another operation.
// A shadow decision is never surfaced: only an applied decision reaches the UI.
func bindAssessment(p *Presentation, a Assessment) {
	p.Pending = a.Status == "pending"
	p.AssessmentID = a.ID
	p.AssessmentOperationID = a.OperationID
	p.AssessmentStatus = a.Status
	p.AssessmentDecision, p.AssessmentDetail = "", ""
	if a.Applied {
		p.AssessmentDecision, p.AssessmentDetail = a.Decision, a.Detail
	}
}

// reconcileInterrupted turns a transmitted assessment whose lease expired
// without a result into a definite failure that keeps its unknown spend. It
// never resends: the provider may have accepted and billed the lost request.
func reconcileInterrupted(ctx context.Context, tx *sql.Tx, a *Assessment, now int64) error {
	if a.Status != "pending" || a.Transmissions == 0 || a.LeaseUntil > now {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='failed',error=?,lease_token='',lease_until=0,finished_at=?
		WHERE id=? AND status='pending'`, assessmentInterruptedErr, now, a.ID); err != nil {
		return err
	}
	a.Status, a.Error, a.LeaseToken, a.LeaseUntil = "failed", assessmentInterruptedErr, "", 0
	return nil
}

// spentMicros is the rolling-window spend that every reservation must fit
// under: measured cost when known, otherwise the reservation, across generation
// attempts and both assessment tables, plus everything still active or pending.
func spentMicros(ctx context.Context, tx *sql.Tx, now int64) (int64, error) {
	// A canceled, restored, or exhausted job might never revisit its content
	// rows. Expired send leases still need a terminal unknown-cost record.
	if _, err := tx.ExecContext(ctx, `UPDATE content_assessments SET status='failed',decision='ungraded',error=?,lease_token='',lease_until=0,finished_at=? WHERE status='pending' AND transmissions=1 AND lease_until<=?`, assessmentInterruptedErr, now, now); err != nil {
		return 0, err
	}
	since := now - allowanceWindow.Milliseconds()
	var spent int64
	err := tx.QueryRowContext(ctx, `SELECT
	 COALESCE((SELECT sum(COALESCE(cost_micros,reserved_micros)) FROM job_attempts WHERE started_at>=? OR state='active'),0)+
	 COALESCE((SELECT sum(COALESCE(cost_micros,reserved_micros)) FROM semantic_assessments WHERE created_at>=? OR status='pending'),0)+
	 COALESCE((SELECT sum(COALESCE(cost_micros,reserved_micros)) FROM content_assessments WHERE created_at>=? OR status='pending'),0)`,
		since, since, since).Scan(&spent)
	return spent, err
}

// BeginAssessmentTransmission grants at most one lease, ever, for one
// assessment. Under the lease it records the exact request and reserves the
// configured allowance atomically, so a duplicate caller, a replay, or a
// crash after the send can never produce a second model request. Callers
// without Send must return the durable state instead of calling the provider.
func (s *Store) BeginAssessmentTransmission(ctx context.Context, id, model string, requestJSON []byte, spending SemanticSpending) (AssessmentLease, error) {
	if err := validText("semantic request model", model, 200, true); err != nil {
		return AssessmentLease{}, err
	}
	if len(requestJSON) == 0 || len(requestJSON) > 256<<10 || !json.Valid(requestJSON) {
		return AssessmentLease{}, fmt.Errorf("%w: semantic request must be valid bounded JSON", ErrInvalid)
	}
	if spending.ReservationMicros < 0 || spending.DailyBudgetMicros < 0 || spending.ReservationMicros > spending.DailyBudgetMicros {
		return AssessmentLease{}, fmt.Errorf("%w: semantic reservation must be nonnegative and within the daily budget", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AssessmentLease{}, err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := readAssessment(ctx, tx, id)
	if err != nil {
		return AssessmentLease{}, err
	}
	if err = reconcileInterrupted(ctx, tx, &a, now); err != nil {
		return AssessmentLease{}, err
	}
	if a.Status != "pending" {
		return AssessmentLease{Assessment: a}, tx.Commit()
	}
	if a.LeaseUntil > now {
		return AssessmentLease{}, fmt.Errorf("%w: this answer is still being checked", ErrConflict)
	}
	if spending.ReservationMicros > 0 {
		spent, spendErr := spentMicros(ctx, tx, now)
		if spendErr != nil {
			return AssessmentLease{}, spendErr
		}
		if spent > spending.DailyBudgetMicros || spending.ReservationMicros > spending.DailyBudgetMicros-spent {
			if _, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='failed',error=?,finished_at=? WHERE id=? AND status='pending'`,
				assessmentAllowanceErr, now, id); err != nil {
				return AssessmentLease{}, err
			}
			a.Status, a.Error = "failed", assessmentAllowanceErr
			if err = tx.Commit(); err != nil {
				return AssessmentLease{}, err
			}
			return AssessmentLease{Assessment: a}, fmt.Errorf("%w: last 24h costs, unknown usage and reservations leave insufficient allowance", ErrBudget)
		}
	}
	token := newID()
	result, err := tx.ExecContext(ctx, `UPDATE semantic_assessments SET request_model=?,request_json=?,transmissions=1,reserved_micros=?,lease_token=?,lease_until=?
		WHERE id=? AND status='pending' AND transmissions=0`, model, string(requestJSON), spending.ReservationMicros, token, now+semanticLease.Milliseconds(), id)
	if err != nil {
		return AssessmentLease{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return AssessmentLease{}, err
	}
	if changed != 1 {
		return AssessmentLease{}, fmt.Errorf("%w: semantic assessment changed; reload review", ErrConflict)
	}
	a.RequestModel, a.RequestJSON, a.Transmissions = model, string(requestJSON), 1
	a.ReservedMicros, a.LeaseToken, a.LeaseUntil = spending.ReservationMicros, token, now+semanticLease.Milliseconds()
	if err = tx.Commit(); err != nil {
		return AssessmentLease{}, err
	}
	return AssessmentLease{Assessment: a, Token: token, Send: true}, nil
}

// ownsLease checks that the caller finishing an assessment is the one that
// sent its request. A stale or foreign token cannot record any outcome.
func ownsLease(a Assessment, token string) bool {
	return a.Status == "pending" && a.Transmissions == 1 && token != "" && a.LeaseToken == token
}

// FailAssessment records a definite unavailable/rejected/malformed result. The
// learner answer stays on the ungraded presentation for an explicit new retry
// or reveal. A no-send failure (nothing left the process) releases its
// reservation; any other failure keeps unknown spend accounted.
func (s *Store) FailAssessment(ctx context.Context, id, token string, result AssessmentResult) (Presentation, error) {
	if err := validText("semantic failure", result.Error, 4096, true); err != nil {
		return Presentation{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	a, err := readAssessment(ctx, tx, id)
	if err != nil {
		return Presentation{}, err
	}
	if ownsLease(a, token) {
		response, err := nullableJSON(result.ResponseJSON)
		if err != nil {
			return Presentation{}, err
		}
		reserved := a.ReservedMicros
		if result.NoSend {
			reserved = 0
		}
		_, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='failed',response_model=?,response_json=?,error=?,
		 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,reserved_micros=?,lease_token='',lease_until=0,finished_at=? WHERE id=? AND status='pending' AND lease_token=?`,
			result.ResponseModel, response, result.Error, nullableCount(result.InputTokens), nullableCount(result.OutputTokens), result.CostMicros,
			nullableLatency(result.LatencyMS), reserved, s.now(), id, token)
		if err != nil {
			return Presentation{}, err
		}
		a.Status, a.Error, a.ReservedMicros = "failed", result.Error, reserved
	} else if err = reconcileInterrupted(ctx, tx, &a, s.now()); err != nil {
		return Presentation{}, err
	}
	p, err := presentation(ctx, tx, a.PresentationID)
	if err != nil {
		return Presentation{}, err
	}
	bindAssessment(&p, a)
	hideAnswer(&p)
	if err = tx.Commit(); err != nil {
		return Presentation{}, err
	}
	return p, nil
}

// FinalizeAssessment is the second grading transaction. Every authority fence
// is rechecked after the provider call; a stale assessment is only marked
// superseded and cannot alter the occurrence, event history, or schedule. A
// caller without the lease receives the durable state and changes nothing.
func (s *Store) FinalizeAssessment(ctx context.Context, id, token string, result AssessmentResult) (Presentation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	a, err := readAssessment(ctx, tx, id)
	if err != nil {
		return Presentation{}, err
	}
	p, err := presentation(ctx, tx, a.PresentationID)
	if err != nil {
		return Presentation{}, err
	}
	if !ownsLease(a, token) {
		if err = reconcileInterrupted(ctx, tx, &a, s.now()); err != nil {
			return Presentation{}, err
		}
		bindAssessment(&p, a)
		hideAnswer(&p)
		if err = tx.Commit(); err != nil {
			return Presentation{}, err
		}
		return p, nil
	}
	response, err := nullableJSON(result.ResponseJSON)
	if err != nil {
		return Presentation{}, err
	}
	var current sql.NullString
	var contentVersion, scheduleVersion int
	var archived bool
	var cardJSON, algorithm string
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err == nil {
		err = tx.QueryRowContext(ctx, `SELECT q.version,(q.archived OR src.archived),sc.version,sc.card,sc.algorithm
		 FROM presentations p JOIN quizzes q ON q.id=p.quiz_id JOIN sources src ON src.id=q.source_id
		 JOIN schedules sc ON sc.quiz_id=q.id WHERE p.id=?`, p.ID).
			Scan(&contentVersion, &archived, &scheduleVersion, &cardJSON, &algorithm)
	}
	if err != nil {
		return Presentation{}, err
	}
	if !current.Valid || current.String != p.ID || p.Graded || archived || contentVersion != a.ContentVersion ||
		scheduleVersion != a.ScheduleVersion || algorithm != learning.Algorithm || a.PolicyVersion != learning.SemanticPolicyVersion {
		_, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='superseded',response_model=?,response_json=?,error='',
		 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,reserved_micros=?,lease_token='',lease_until=0,finished_at=? WHERE id=? AND status='pending' AND lease_token=?`,
			result.ResponseModel, response, result.InputTokens, result.OutputTokens, result.CostMicros,
			result.LatencyMS, settledReservation(a, result), s.now(), id, token)
		if err != nil {
			return Presentation{}, err
		}
		a.Status, a.Applied = "superseded", false
		bindAssessment(&p, a)
		hideAnswer(&p)
		if err = tx.Commit(); err != nil {
			return Presentation{}, err
		}
		return p, nil
	}

	decision := learning.GradeSemantic(result.Judgments, s.semanticParams())
	if p.Quiz.Rubric == nil || len(result.Judgments.Ideas) != len(p.Quiz.Rubric.Required) || len(result.Judgments.Contradictions) != len(p.Quiz.Rubric.Contradictions) {
		decision = learning.SemanticDecision{Decision: "ungraded", Outcome: "ungraded", MissingIdea: -1, Contradiction: -1}
	}
	detail, exposure := "", ""
	if decision.Decision == "incomplete" && decision.MissingIdea >= 0 && decision.MissingIdea < len(p.Quiz.Rubric.Required) {
		detail, exposure = p.Quiz.Rubric.Required[decision.MissingIdea].Cue, "cue"
	} else if decision.Decision == "incorrect" && decision.Contradiction >= 0 && decision.Contradiction < len(p.Quiz.Rubric.Contradictions) {
		detail, exposure = p.Quiz.Rubric.Contradictions[decision.Contradiction].Feedback, "feedback"
	}
	now := s.now()
	p.Answer, p.Draft, p.Outcome, p.Rating = a.Answer, a.Answer, decision.Outcome, decision.Rating
	reviewID := ""
	switch {
	case decision.Applied && (decision.Decision == "correct" || decision.Decision == "incorrect"):
		p.Graded = true
		if p.Assisted && decision.Decision == "correct" {
			p.Outcome, p.Rating = "warm_correct", 0
		}
		p.ReviewedAt, p.ReviewID = now, newID()
		reviewID = p.ReviewID
		afterJSON, afterVersion := cardJSON, scheduleVersion
		if p.Rating != 0 {
			var card learning.Card
			if err = json.Unmarshal([]byte(cardJSON), &card); err != nil {
				return Presentation{}, err
			}
			next, scheduleErr := learning.Schedule(card, p.Rating, time.UnixMilli(now))
			if scheduleErr != nil {
				return Presentation{}, scheduleErr
			}
			afterJSON, err = marshal(next)
			if err != nil {
				return Presentation{}, err
			}
			p.DueAt = next.Due.UnixMilli()
			afterVersion++
			update, updateErr := tx.ExecContext(ctx, "UPDATE schedules SET version=?,card=?,due_at=? WHERE quiz_id=? AND version=?", afterVersion, afterJSON, p.DueAt, p.Quiz.ID, scheduleVersion)
			if updateErr != nil {
				return Presentation{}, updateErr
			}
			if changed, rowsErr := update.RowsAffected(); rowsErr != nil || changed != 1 {
				if rowsErr != nil {
					return Presentation{}, rowsErr
				}
				return Presentation{}, fmt.Errorf("%w: schedule changed", ErrConflict)
			}
		}
		snapshot, marshalErr := marshal(p.Quiz)
		if marshalErr != nil {
			return Presentation{}, marshalErr
		}
		// The event names the policy that actually produced it; the schedule
		// card keeps the frozen scheduler identity it was fenced against.
		_, err = tx.ExecContext(ctx, `INSERT INTO review_events(id,presentation_id,snapshot,answer,outcome,rating,assisted,reviewed_at,due_at,algorithm,
		 schedule_before,schedule_after,schedule_version_before,schedule_version_after,grading) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ReviewID, p.ID, snapshot, p.Answer, p.Outcome, p.Rating, p.Assisted, now, p.DueAt, learning.EventAlgorithm(learning.SemanticPolicyVersion),
			cardJSON, afterJSON, scheduleVersion, afterVersion, learning.SemanticPolicyVersion)
		if err != nil {
			return Presentation{}, err
		}
		p.Draft = ""
		if decision.Decision == "incorrect" && exposure != "" && detail != "" {
			if err = recordExposure(ctx, tx, p, exposure, now); err != nil {
				return Presentation{}, err
			}
		}
	case decision.Applied && decision.Decision == "incomplete":
		// The assistance fence, the exposure record, and the cue become durable
		// in this same write, before any rendering can show the cue.
		p.Assisted, p.Graded = true, false
		if err = recordExposure(ctx, tx, p, "cue", now); err != nil {
			return Presentation{}, err
		}
	case !decision.Applied:
		// Shadow or ungraded: the learner sees plain ungraded; nothing is charged.
		p.Outcome, p.Rating, p.Graded = "ungraded", 0, false
	default:
		return Presentation{}, errors.New("unsupported semantic policy decision")
	}
	_, err = tx.ExecContext(ctx, `UPDATE presentations SET answer=?,outcome=?,assisted=?,graded=?,rating=?,due_at=?,reviewed_at=?,review_id=? WHERE id=?`,
		p.Answer, p.Outcome, p.Assisted, p.Graded, p.Rating, p.DueAt, p.ReviewedAt, p.ReviewID, p.ID)
	if err != nil {
		return Presentation{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='judged',response_model=?,response_json=?,decision=?,applied=?,detail=?,error='',
	 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,reserved_micros=?,lease_token='',lease_until=0,review_id=?,finished_at=? WHERE id=? AND status='pending' AND lease_token=?`,
		result.ResponseModel, response, decision.Decision, decision.Applied, detail, result.InputTokens, result.OutputTokens, result.CostMicros,
		result.LatencyMS, settledReservation(a, result), reviewID, now, id, token)
	if err != nil {
		return Presentation{}, err
	}
	a.Status, a.Decision, a.Applied, a.Detail = "judged", decision.Decision, decision.Applied, detail
	bindAssessment(&p, a)
	hideAnswer(&p)
	if err = tx.Commit(); err != nil {
		return Presentation{}, err
	}
	return p, nil
}

// settledReservation replaces the reservation with the measured cost once the
// provider reported usage; an unknown cost keeps the reservation accounted.
func settledReservation(a Assessment, result AssessmentResult) int64 {
	if result.CostMicros != nil {
		return 0
	}
	return a.ReservedMicros
}

// recordExposure durably notes that authored help text was shown for this
// content version, so a later occurrence within the exposure window is warm.
func recordExposure(ctx context.Context, tx *sql.Tx, p Presentation, kind string, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO assistance_exposures(id,presentation_id,quiz_id,content_version,kind,created_at) VALUES(?,?,?,?,?,?)`,
		newID(), p.ID, p.Quiz.ID, p.Quiz.Version, kind, now)
	return err
}

func nullableJSON(raw []byte) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 256<<10 || !json.Valid(raw) {
		return nil, fmt.Errorf("%w: semantic response must be valid bounded JSON", ErrInvalid)
	}
	return string(raw), nil
}

func nullableCount(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableLatency(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
