package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func enqueue(ctx context.Context, tx *sql.Tx, sourceID string, revision int, now int64) error {
	_, err := enqueueKnowledge(ctx, tx, sourceID, revision, "capture", "", 0, "", 0, now)
	return err
}

func job(ctx context.Context, tx *sql.Tx, id string) (Job, error) {
	return readJob(ctx, tx, id, true)
}

func jobMetadata(ctx context.Context, tx *sql.Tx, id string) (Job, error) {
	return readJob(ctx, tx, id, false)
}

func readJob(ctx context.Context, tx *sql.Tx, id string, includeContext bool) (Job, error) {
	var j Job
	var contextJSON, coverageJSON string
	err := tx.QueryRowContext(ctx, `SELECT j.id,j.source_id,j.status,j.error,j.model,CASE WHEN ? THEN j.lease_token ELSE '' END,CASE WHEN ? THEN r.text ELSE '' END,r.kind,j.source_revision,j.attempts,
	 j.created_at,j.updated_at,j.published,
	 COALESCE((SELECT sum(CASE WHEN a.state='active' THEN 0 ELSE COALESCE(a.cost_micros,a.reserved_micros) END) FROM job_attempts a WHERE a.job_id=j.id),0),
	 COALESCE((SELECT sum(a.reserved_micros) FROM job_attempts a WHERE a.job_id=j.id AND a.state='active'),0),
	 EXISTS(SELECT 1 FROM job_attempts a WHERE a.job_id=j.id AND a.state<>'active' AND a.cost_micros IS NULL),
	 j.kind,j.goal_id,j.goal_revision,j.target_material_id,j.target_material_version,j.target_presentation_id,j.target_presentation_version,CASE WHEN ? THEN j.context_json ELSE '{}' END,j.new_materials,j.reused_materials,j.new_quizzes,CASE WHEN ? THEN j.coverage_json ELSE json_set(j.coverage_json,'$.missing',json('[]')) END,j.suggestion_id,j.parent_job_id,j.retry_root_id,j.observation_id
	 FROM jobs j JOIN source_revisions r ON r.source_id=j.source_id AND r.revision=j.source_revision WHERE j.id=?`, includeContext, includeContext, includeContext, includeContext, id).
		Scan(&j.ID, &j.SourceID, &j.Status, &j.Error, &j.Model, &j.LeaseToken, &j.SourceText, &j.SourceKind, &j.SourceRevision,
			&j.Attempts, &j.CreatedAt, &j.UpdatedAt, &j.Published, &j.CostMicros, &j.ReservedMicros, &j.CostUnknown,
			&j.Kind, &j.GoalID, &j.GoalRevision, &j.TargetMaterialID, &j.TargetMaterialVersion, &j.TargetPresentationID, &j.TargetPresentationVersion, &contextJSON, &j.NewMaterials, &j.ReusedMaterials, &j.NewQuizzes, &coverageJSON, &j.SuggestionID, &j.ParentJobID, &j.RetryRootID, &j.ObservationID)
	if err != nil {
		return j, notFound(err, "generation job")
	}
	if err = json.Unmarshal([]byte(contextJSON), &j.Context); err != nil {
		return j, err
	}
	err = json.Unmarshal([]byte(coverageJSON), &j.Coverage)
	if err != nil {
		return j, err
	}
	if !includeContext {
		hideJobContext(&j)
	}
	err = jobRetryMetadata(ctx, tx, &j)
	return j, err
}

func hideJobContext(j *Job) {
	j.MetadataOnly = true
	j.SourceText, j.LeaseToken = "", ""
	j.Context = KnowledgeContext{}
	j.Coverage.Missing = []string{}
	if j.Error != "" {
		j.Error = "A " + j.Status + " diagnostic is recorded; explicit full-export inspection is required for its unredacted details"
	}
}

// ClaimJob reserves before external work. A zero reservation is solely for a
// worker's no-network unconfigured failure; positive reservations require budget.
// Expired claims retain their allowance as unknown spend, never as free retries.
func (s *Store) ClaimJob(ctx context.Context, lease time.Duration, reservationMicros, dailyBudgetMicros int64) (*Job, error) {
	if lease < time.Second || lease > 10*time.Minute || reservationMicros < 0 || dailyBudgetMicros < 0 {
		return nil, fmt.Errorf("%w: lease must be 1s–10m and spending amounts nonnegative", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.now()
	if err = s.checkSchemaCookie(ctx, tx); err != nil {
		return nil, err
	}
	if err = queueEnrichment(ctx, tx, now); err != nil {
		return nil, err
	}
	if err = expireJobs(ctx, tx, now); err != nil {
		return nil, err
	}
	var running bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE status='running')").Scan(&running); err != nil {
		return nil, err
	}
	if running {
		return nil, tx.Commit()
	}
	var id, jobKind string
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.kind FROM jobs j WHERE j.status IN ('queued','retry')
	 AND j.available_at<=? AND j.attempts<3 AND `+eligibleKnowledgeJob+`
	 ORDER BY CASE j.kind WHEN 'bridge' THEN 0 WHEN 'capture' THEN 1 WHEN 'expand' THEN 2 ELSE 3 END,j.available_at,j.created_at,j.id LIMIT 1`, now).Scan(&id, &jobKind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	if reservationMicros > 0 {
		var spent int64
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(COALESCE(cost_micros,reserved_micros)),0) FROM job_attempts
		 WHERE started_at>=? OR state='active'`, now-int64(24*time.Hour/time.Millisecond)).Scan(&spent); err != nil {
			return nil, err
		}
		if spent > dailyBudgetMicros || reservationMicros > dailyBudgetMicros-spent || (jobKind == "enrich" && reservationMicros > (dailyBudgetMicros-spent)/2) {
			if _, err = tx.ExecContext(ctx, "UPDATE jobs SET error='Waiting for daily generation allowance; prior and unknown usage remain accounted',updated_at=? WHERE id=? AND error=''", now, id); err != nil {
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%w: last 24h costs, unknown usage and reservations leave insufficient allowance", ErrBudget)
		}
	}
	j, err := job(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	knowledge, err := generationContext(ctx, tx, j, now)
	if err != nil {
		return nil, err
	}
	encodedContext, err := marshal(knowledge)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET context_json=? WHERE id=?", encodedContext, id); err != nil {
		return nil, err
	}
	token := newID()
	_, err = tx.ExecContext(ctx, "UPDATE jobs SET status='running',attempts=attempts+1,lease_token=?,lease_until=?,updated_at=? WHERE id=?", token, now+lease.Milliseconds(), now, id)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO job_attempts(token,job_id,number,started_at,reserved_micros,state)
	 SELECT ?,id,attempts,?,?,'active' FROM jobs WHERE id=?`, token, now, reservationMicros, id)
	if err != nil {
		return nil, err
	}
	j, err = job(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &j, nil
}

func expireJobs(ctx context.Context, tx *sql.Tx, now int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active' AND job_id IN
	 (SELECT j.id FROM jobs j WHERE j.status='running' AND (j.lease_until<=? OR NOT (`+eligibleKnowledgeJob+`)))`, now, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs AS j SET status='canceled',error='Source, goal, or retained target changed; prior usage retained',lease_token='',lease_until=0,updated_at=?
	 WHERE status IN ('queued','retry','running') AND NOT (`+eligibleKnowledgeJob+`)`, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN attempts>=3 THEN 'failed' ELSE 'retry' END,
	 error='Previous attempt expired; usage unknown and reserved allowance retained',lease_token='',lease_until=0,updated_at=?,
	 available_at=?+CASE WHEN attempts=1 THEN 10000 ELSE 60000 END WHERE status='running' AND lease_until<=?`, now, now, now)
	return err
}

type attempt struct {
	job  Job
	live bool
	hash string
	code string
}

func claimAttempt(ctx context.Context, tx *sql.Tx, jobID, token string, now int64) (attempt, error) {
	var a attempt
	var oldHash, oldCode, state string
	err := tx.QueryRowContext(ctx, "SELECT finish_hash,finish_code,state FROM job_attempts WHERE token=? AND job_id=?", token, jobID).Scan(&oldHash, &oldCode, &state)
	if err != nil {
		return a, notFound(err, "job attempt")
	}
	a.hash, a.code = oldHash, oldCode
	a.job, err = job(ctx, tx, jobID)
	if err != nil {
		return a, err
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT (j.status='running' AND j.lease_token=? AND j.lease_until>? AND `+eligibleKnowledgeJob+`)
	 FROM jobs j WHERE j.id=?`, token, now, jobID).Scan(&eligible)
	if err != nil {
		return a, err
	}
	a.live = eligible && state == "active"
	return a, nil
}

func finishError(code string) error {
	switch code {
	case "invalid":
		return fmt.Errorf("%w: generation result rejected; cost recorded; inspect or retry the saved source", ErrInvalid)
	case "conflict":
		return fmt.Errorf("%w: generation claim expired or source changed; cost recorded without publication", ErrConflict)
	default:
		return nil
	}
}

func repeatedFinish(a attempt, hash string) (bool, error) {
	if a.hash == "" {
		return false, nil
	}
	if a.hash != hash {
		return true, fmt.Errorf("%w: finalized attempt reused with different output or usage", ErrConflict)
	}
	return true, finishError(a.code)
}

func settleAttempt(ctx context.Context, tx *sql.Tx, token, hash, code string, costMicros *int64, now int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE job_attempts SET cost_micros=?,state='settled',finished_at=?,finish_hash=?,finish_code=? WHERE token=? AND finish_hash=''`, costMicros, now, hash, code, token)
	return err
}

func finishStale(ctx context.Context, tx *sql.Tx, jobID, token string, now int64) error {
	// Do not touch a successor claim. If nobody has swept this expired claim
	// yet, recover it now; its already-settled actual/unknown cost remains.
	_, err := tx.ExecContext(ctx, `UPDATE jobs AS j SET status=CASE WHEN NOT (`+eligibleKnowledgeJob+`)
	 THEN 'canceled' WHEN attempts>=3 THEN 'failed' ELSE 'retry' END,
	 error='Expired or stale completion discarded; usage recorded',lease_token='',lease_until=0,updated_at=?,available_at=?+60000
	 WHERE id=? AND status='running' AND lease_token=?`, now, now, jobID, token)
	return err
}

func (s *Store) CompleteJob(ctx context.Context, jobID, leaseToken string, result GenerationResult, costMicros *int64) error {
	if costMicros != nil && *costMicros < 0 {
		return fmt.Errorf("%w: cost cannot be negative", ErrInvalid)
	}
	hash := payloadHash(struct {
		Kind   string
		Result GenerationResult
		Cost   *int64
	}{"complete", result, costMicros})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := claimAttempt(ctx, tx, jobID, leaseToken, now)
	if err != nil {
		return err
	}
	if repeated, err := repeatedFinish(a, hash); repeated {
		return err
	}
	if !a.live {
		if err = settleAttempt(ctx, tx, leaseToken, hash, "conflict", costMicros, now); err != nil {
			return err
		}
		if err = finishStale(ctx, tx, jobID, leaseToken, now); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return finishError("conflict")
	}
	src := Source{Text: a.job.SourceText, Kind: a.job.SourceKind}
	validation := validateGeneration(result, src)
	if validation == nil {
		validation = validateReuse(ctx, tx, a.job, result)
	}
	code := "invalid"
	if errors.Is(validation, ErrConflict) {
		code = "conflict"
	}
	if validation != nil {
		if err = settleAttempt(ctx, tx, leaseToken, hash, code, costMicros, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET status='failed',error=?,lease_token='',lease_until=0,updated_at=? WHERE id=?", validation.Error(), now, jobID)
		if err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return validation
	}
	if _, err = tx.ExecContext(ctx, "SAVEPOINT publish_bundle"); err != nil {
		return err
	}
	newMaterials, reusedMaterials, newQuizzes, err := publishKnowledge(ctx, tx, a.job, result, now)
	if err != nil {
		if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO publish_bundle; RELEASE publish_bundle"); rollbackErr != nil {
			return rollbackErr
		}
		if settleErr := settleAttempt(ctx, tx, leaseToken, hash, "invalid", costMicros, now); settleErr != nil {
			return settleErr
		}
		if _, saveErr := tx.ExecContext(ctx, "UPDATE jobs SET status='failed',error='Bundle publication rejected; no material published and prior usage retained',lease_token='',lease_until=0,updated_at=? WHERE id=?", now, jobID); saveErr != nil {
			return saveErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
		return fmt.Errorf("%w: bundle publication rejected: %v", ErrInvalid, err)
	}
	if _, err = tx.ExecContext(ctx, "RELEASE publish_bundle"); err != nil {
		return err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return err
	}
	status := "complete"
	if !result.Coverage.Complete {
		status = "partial"
	}
	encoded, err := marshal(result)
	if err != nil {
		return err
	}
	coverageJSON, err := marshal(result.Coverage)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status=?,error=?,model=?,prompt_version=?,published=?,result_json=?,
	 new_materials=?,reused_materials=?,new_quizzes=?,coverage_json=?,lease_token='',lease_until=0,updated_at=? WHERE id=?`, status, strings.Join(result.Coverage.Missing, "\n"), result.Model, result.PromptVersion, newMaterials+reusedMaterials, encoded,
		newMaterials, reusedMaterials, newQuizzes, coverageJSON, now, jobID)
	if err != nil {
		return err
	}
	if err = settleAttempt(ctx, tx, leaseToken, hash, "", costMicros, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailJob(ctx context.Context, jobID, leaseToken, message string, retry bool, costMicros *int64) error {
	if costMicros != nil && *costMicros < 0 {
		return fmt.Errorf("%w: cost cannot be negative", ErrInvalid)
	}
	if err := validText("safe generation error", message, 4096, true); err != nil {
		return err
	}
	hash := payloadHash(struct {
		Kind, Message string
		Retry         bool
		Cost          *int64
	}{"fail", message, retry, costMicros})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now()
	a, err := claimAttempt(ctx, tx, jobID, leaseToken, now)
	if err != nil {
		return err
	}
	if repeated, err := repeatedFinish(a, hash); repeated {
		return err
	}
	code := ""
	if !a.live {
		code = "conflict"
	}
	if err = settleAttempt(ctx, tx, leaseToken, hash, code, costMicros, now); err != nil {
		return err
	}
	if !a.live {
		if err = finishStale(ctx, tx, jobID, leaseToken, now); err != nil {
			return err
		}
	} else {
		status := "failed"
		delay := int64(0)
		if retry && a.job.Attempts < 3 {
			status = "retry"
			delay = int64(10 * time.Second / time.Millisecond)
			if a.job.Attempts > 1 {
				delay = int64(time.Minute / time.Millisecond)
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,error=?,lease_token='',lease_until=0,updated_at=?,available_at=? WHERE id=?", status, message, now, now+delay, jobID)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return finishError(code)
}

func (s *Store) PauseRestoredJobs(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now()
	if _, err = tx.ExecContext(ctx, "UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active'", now); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status='paused',error='Restored work paused: reconcile prior requests and spend before explicit retry',
	 lease_token='',lease_until=0,updated_at=? WHERE status IN ('queued','retry','running')`, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}
