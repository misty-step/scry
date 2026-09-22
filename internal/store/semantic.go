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
	 a.status,a.policy_version,a.request_model,a.request_json,a.transmissions,a.decision,a.detail,a.error,a.review_id,p.snapshot
	 FROM semantic_assessments a JOIN presentations p ON p.id=a.presentation_id WHERE a.id=?`, id).
		Scan(&a.ID, &a.PresentationID, &a.OperationID, &a.ContentVersion, &a.ScheduleVersion, &a.Answer,
			&a.Status, &a.PolicyVersion, &a.RequestModel, &a.RequestJSON, &a.Transmissions, &a.Decision, &a.Detail, &a.Error, &a.ReviewID, &snapshot)
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
func bindAssessment(p *Presentation, a Assessment) {
	p.Pending = a.Status == "pending"
	p.AssessmentID = a.ID
	p.AssessmentOperationID = a.OperationID
	p.AssessmentStatus = a.Status
	p.AssessmentDecision = a.Decision
	p.AssessmentDetail = a.Detail
}

// BeginAssessmentTransmission durably records the exact request before the
// caller performs HTTP. At most two transmissions can ever be recorded.
func (s *Store) BeginAssessmentTransmission(ctx context.Context, id, model string, requestJSON []byte) (Assessment, error) {
	if err := validText("semantic request model", model, 200, true); err != nil {
		return Assessment{}, err
	}
	if len(requestJSON) == 0 || len(requestJSON) > 256<<10 || !json.Valid(requestJSON) {
		return Assessment{}, fmt.Errorf("%w: semantic request must be valid bounded JSON", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Assessment{}, err
	}
	defer tx.Rollback()
	a, err := readAssessment(ctx, tx, id)
	if err != nil {
		return a, err
	}
	if a.Status != "pending" {
		return a, tx.Commit()
	}
	if a.Transmissions > 0 && (a.RequestModel != model || a.RequestJSON != string(requestJSON)) {
		return Assessment{}, fmt.Errorf("%w: semantic replay request changed", ErrConflict)
	}
	if a.Transmissions >= 2 {
		if _, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='failed',error='semantic transmission limit reached',finished_at=?
			WHERE id=? AND status='pending'`, s.now(), id); err != nil {
			return Assessment{}, err
		}
		a.Status = "failed"
		if err = tx.Commit(); err != nil {
			return Assessment{}, err
		}
		return a, nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE semantic_assessments SET request_model=?,request_json=?,transmissions=transmissions+1
		WHERE id=? AND status='pending' AND transmissions<2`, model, string(requestJSON), id)
	if err != nil {
		return Assessment{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Assessment{}, err
	}
	if changed != 1 {
		return Assessment{}, fmt.Errorf("%w: semantic assessment changed; reload review", ErrConflict)
	}
	a.RequestModel, a.RequestJSON, a.Transmissions = model, string(requestJSON), a.Transmissions+1
	if err = tx.Commit(); err != nil {
		return Assessment{}, err
	}
	return a, nil
}

// FailAssessment records a definite unavailable/rejected/malformed result. The
// learner answer stays on the ungraded presentation for an explicit new retry
// or reveal.
func (s *Store) FailAssessment(ctx context.Context, id string, result AssessmentResult) (Presentation, error) {
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
	if a.Status == "pending" {
		response, err := nullableJSON(result.ResponseJSON)
		if err != nil {
			return Presentation{}, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='failed',response_model=?,response_json=?,error=?,
		 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,finished_at=? WHERE id=? AND status='pending'`,
			result.ResponseModel, response, result.Error, nullableCount(result.InputTokens), nullableCount(result.OutputTokens), result.CostMicros,
			nullableLatency(result.LatencyMS), s.now(), id)
		if err != nil {
			return Presentation{}, err
		}
		a.Status, a.Error = "failed", result.Error
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
// superseded and cannot alter the occurrence, event history, or schedule.
func (s *Store) FinalizeAssessment(ctx context.Context, id string, result AssessmentResult) (Presentation, error) {
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
	if a.Status != "pending" {
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
		 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,finished_at=? WHERE id=? AND status='pending'`,
			result.ResponseModel, response, result.InputTokens, result.OutputTokens, result.CostMicros,
			result.LatencyMS, s.now(), id)
		if err != nil {
			return Presentation{}, err
		}
		p.Pending, p.AssessmentStatus = false, "superseded"
		p.AssessmentID = id
		hideAnswer(&p)
		if err = tx.Commit(); err != nil {
			return Presentation{}, err
		}
		return p, nil
	}

	decision := learning.GradeSemantic(result.Judgments, learning.SemanticV1Params())
	if p.Quiz.Rubric == nil || len(result.Judgments.Ideas) != len(p.Quiz.Rubric.Required) || len(result.Judgments.Contradictions) != len(p.Quiz.Rubric.Contradictions) {
		decision = learning.SemanticDecision{Decision: "ungraded", Outcome: "ungraded", MissingIdea: -1, Contradiction: -1}
	}
	detail := ""
	if decision.Decision == "incomplete" && decision.MissingIdea >= 0 && decision.MissingIdea < len(p.Quiz.Rubric.Required) {
		detail = p.Quiz.Rubric.Required[decision.MissingIdea].Cue
	} else if decision.Decision == "incorrect" && decision.Contradiction >= 0 && decision.Contradiction < len(p.Quiz.Rubric.Contradictions) {
		detail = p.Quiz.Rubric.Contradictions[decision.Contradiction].Feedback
	}
	now := s.now()
	p.Answer, p.Draft, p.Outcome, p.Rating = a.Answer, a.Answer, decision.Outcome, decision.Rating
	reviewID := ""
	switch decision.Decision {
	case "correct", "incorrect":
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
		_, err = tx.ExecContext(ctx, `INSERT INTO review_events(id,presentation_id,snapshot,answer,outcome,rating,assisted,reviewed_at,due_at,algorithm,
		 schedule_before,schedule_after,schedule_version_before,schedule_version_after,grading) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ReviewID, p.ID, snapshot, p.Answer, p.Outcome, p.Rating, p.Assisted, now, p.DueAt, learning.Algorithm,
			cardJSON, afterJSON, scheduleVersion, afterVersion, learning.SemanticPolicyVersion)
		if err != nil {
			return Presentation{}, err
		}
		p.Draft = ""
	case "incomplete":
		// The assistance fence and the cue become durable in this same write.
		p.Assisted = true
		p.Graded = false
	case "ungraded":
		p.Graded = false
	default:
		return Presentation{}, errors.New("unsupported semantic policy decision")
	}
	_, err = tx.ExecContext(ctx, `UPDATE presentations SET answer=?,outcome=?,assisted=?,graded=?,rating=?,due_at=?,reviewed_at=?,review_id=? WHERE id=?`,
		p.Answer, p.Outcome, p.Assisted, p.Graded, p.Rating, p.DueAt, p.ReviewedAt, p.ReviewID, p.ID)
	if err != nil {
		return Presentation{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE semantic_assessments SET status='judged',response_model=?,response_json=?,decision=?,detail=?,error='',
	 input_tokens=?,output_tokens=?,cost_micros=?,latency_ms=?,review_id=?,finished_at=? WHERE id=? AND status='pending'`,
		result.ResponseModel, response, decision.Decision, detail, result.InputTokens, result.OutputTokens, result.CostMicros,
		result.LatencyMS, reviewID, now, id)
	if err != nil {
		return Presentation{}, err
	}
	p.AssessmentID, p.AssessmentStatus, p.AssessmentDecision, p.AssessmentDetail = id, "judged", decision.Decision, detail
	p.Pending = false
	hideAnswer(&p)
	if err = tx.Commit(); err != nil {
		return Presentation{}, err
	}
	return p, nil
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
