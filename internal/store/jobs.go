package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func enqueue(ctx context.Context, tx *sql.Tx, sourceID string, revision int, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,status,created_at,updated_at,available_at)
	 VALUES(?,?,?,'queued',?,?,?)`, newID(), sourceID, revision, now, now, now)
	return err
}

func job(ctx context.Context, tx *sql.Tx, id string) (Job, error) {
	var j Job
	err := tx.QueryRowContext(ctx, `SELECT j.id,j.source_id,j.status,j.error,j.model,j.lease_token,r.text,r.kind,j.source_revision,j.attempts,
	 j.created_at,j.updated_at,j.published,
	 COALESCE((SELECT sum(CASE WHEN a.state='active' THEN 0 ELSE COALESCE(a.cost_micros,a.reserved_micros) END) FROM job_attempts a WHERE a.job_id=j.id),0),
	 COALESCE((SELECT sum(a.reserved_micros) FROM job_attempts a WHERE a.job_id=j.id AND a.state='active'),0),
	 EXISTS(SELECT 1 FROM job_attempts a WHERE a.job_id=j.id AND a.state<>'active' AND a.cost_micros IS NULL)
	 FROM jobs j JOIN source_revisions r ON r.source_id=j.source_id AND r.revision=j.source_revision WHERE j.id=?`, id).
		Scan(&j.ID, &j.SourceID, &j.Status, &j.Error, &j.Model, &j.LeaseToken, &j.SourceText, &j.SourceKind, &j.SourceRevision,
			&j.Attempts, &j.CreatedAt, &j.UpdatedAt, &j.Published, &j.CostMicros, &j.ReservedMicros, &j.CostUnknown)
	if err == nil {
		var encoded string
		target := Quiz{SourceID: j.SourceID}
		targetErr := tx.QueryRowContext(ctx, `SELECT f.quiz_id,f.quiz_version,v.content FROM foundation_requests f JOIN quiz_versions v ON v.quiz_id=f.quiz_id AND v.version=f.quiz_version WHERE f.job_id=?`, id).Scan(&target.ID, &target.Version, &encoded)
		if targetErr == nil {
			if targetErr = json.Unmarshal([]byte(encoded), &target); targetErr != nil {
				return j, targetErr
			}
			j.FoundationTarget = &target
			j.Foundation = true
		} else if !errors.Is(targetErr, sql.ErrNoRows) {
			return j, targetErr
		}
	}
	return j, notFound(err, "generation job")
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
	var id string
	err = tx.QueryRowContext(ctx, `SELECT j.id FROM jobs j JOIN sources src ON src.id=j.source_id WHERE j.status IN ('queued','retry')
	 AND j.available_at<=? AND j.attempts<3 AND src.archived=0 AND src.revision=j.source_revision
	 AND NOT EXISTS(SELECT 1 FROM foundation_requests f JOIN quizzes q ON q.id=f.quiz_id WHERE f.job_id=j.id AND (q.archived=1 OR q.version<>f.quiz_version))
	 ORDER BY j.available_at,j.created_at,j.id LIMIT 1`, now).Scan(&id)
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
		if spent > dailyBudgetMicros || reservationMicros > dailyBudgetMicros-spent {
			if _, err = tx.ExecContext(ctx, "UPDATE jobs SET error='Waiting for daily generation allowance; prior and unknown usage remain accounted',updated_at=? WHERE id=? AND error=''", now, id); err != nil {
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%w: last 24h costs, unknown usage and reservations leave insufficient allowance", ErrBudget)
		}
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
	j, err := job(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &j, nil
}

func cancelSupersededFoundations(ctx context.Context, tx *sql.Tx, now int64) error {
	const stale = `SELECT f.job_id FROM foundation_requests f JOIN quizzes q ON q.id=f.quiz_id WHERE q.archived=1 OR q.version<>f.quiz_version`
	if _, err := tx.ExecContext(ctx, "UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active' AND job_id IN ("+stale+")", now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',error='Foundation target changed; prior usage retained',lease_token='',lease_until=0,updated_at=? WHERE status IN ('queued','running','retry','paused') AND id IN ("+stale+")", now)
	return err
}

func expireJobs(ctx context.Context, tx *sql.Tx, now int64) error {
	if err := cancelSupersededFoundations(ctx, tx, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active' AND job_id IN
	 (SELECT j.id FROM jobs j JOIN sources src ON src.id=j.source_id WHERE j.status='running'
	 AND (j.lease_until<=? OR src.archived=1 OR src.revision<>j.source_revision))`, now, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Source no longer eligible; prior usage retained',lease_token='',lease_until=0,updated_at=?
	 WHERE status IN ('queued','retry','running') AND EXISTS(SELECT 1 FROM sources src WHERE src.id=jobs.source_id AND (src.archived=1 OR src.revision<>jobs.source_revision))`, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN id IN (SELECT job_id FROM foundation_requests) THEN 'paused' WHEN attempts>=3 THEN 'failed' ELSE 'retry' END,
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
	err = tx.QueryRowContext(ctx, `SELECT (j.status='running' AND j.lease_token=? AND j.lease_until>? AND src.archived=0 AND src.revision=j.source_revision
	 AND NOT EXISTS(SELECT 1 FROM foundation_requests f JOIN quizzes q ON q.id=f.quiz_id WHERE f.job_id=j.id AND (q.archived=1 OR q.version<>f.quiz_version)))
	 FROM jobs j JOIN sources src ON src.id=j.source_id WHERE j.id=?`, token, now, jobID).Scan(&eligible)
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
	_, err := tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN EXISTS(SELECT 1 FROM sources src WHERE src.id=jobs.source_id AND (src.archived=1 OR src.revision<>jobs.source_revision))
	 OR EXISTS(SELECT 1 FROM foundation_requests f JOIN quizzes q ON q.id=f.quiz_id WHERE f.job_id=jobs.id AND (q.archived=1 OR q.version<>f.quiz_version))
	 THEN 'canceled' WHEN id IN (SELECT job_id FROM foundation_requests) THEN 'paused' WHEN attempts>=3 THEN 'failed' ELSE 'retry' END,
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
	var validation error
	if a.job.FoundationTarget != nil {
		validation = ValidateFoundation(result.Foundation)
		if len(result.Quizzes) != 0 {
			validation = fmt.Errorf("%w: foundation jobs cannot publish scheduled quizzes", ErrInvalid)
		}
		if validText("model attribution", result.Model, 200, true) != nil || validText("prompt version", result.PromptVersion, 200, true) != nil || validText("generation note", result.Note, 4096, result.Partial) != nil {
			validation = ErrInvalid
		}
	} else if result.Foundation != nil {
		validation = fmt.Errorf("%w: unexpected foundations in capture job", ErrInvalid)
	} else {
		validation = validateGeneration(result, Source{Text: a.job.SourceText, Kind: a.job.SourceKind})
	}
	if validation != nil {
		if err = settleAttempt(ctx, tx, leaseToken, hash, "invalid", costMicros, now); err != nil {
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
	for index, content := range result.Quizzes {
		id := newID()
		encoded, err := marshal(content)
		if err != nil {
			return err
		}
		card, err := marshal(learning.NewCard(time.UnixMilli(now)))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO quizzes(id,source_id,version,created_at,origin_job_id,origin_index) VALUES(?,?,1,?,?,?)", id, a.job.SourceID, now, jobID, index)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO quiz_versions(quiz_id,version,content,model,prompt_version,created_at) VALUES(?,1,?,?,?,?)", id, encoded, result.Model, result.PromptVersion, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO schedules(quiz_id,version,card,due_at,algorithm) VALUES(?,1,?,?,?)", id, card, now, learning.Algorithm)
		if err != nil {
			return err
		}
	}
	published := len(result.Quizzes)
	if a.job.FoundationTarget != nil {
		published, err = publishFoundation(ctx, tx, a.job, result, now)
		if err != nil {
			return err
		}
	}
	status := "complete"
	if result.Partial {
		status = "partial"
	}
	encoded, err := marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status=?,error=?,model=?,prompt_version=?,published=?,result_json=?,
	 lease_token='',lease_until=0,updated_at=? WHERE id=?`, status, result.Note, result.Model, result.PromptVersion, published, encoded, now, jobID)
	if err != nil {
		return err
	}
	if err = settleAttempt(ctx, tx, leaseToken, hash, "", costMicros, now); err != nil {
		return err
	}
	return tx.Commit()
}

func validateGeneration(result GenerationResult, src Source) error {
	if len(result.Quizzes) == 0 || len(result.Quizzes) > MaxGeneratedQuizzes {
		return fmt.Errorf("%w: generation must contain 1–%d complete quizzes; split larger source tasks", ErrInvalid, MaxGeneratedQuizzes)
	}
	if err := validText("model attribution", result.Model, 200, true); err != nil {
		return err
	}
	if err := validText("prompt version", result.PromptVersion, 200, true); err != nil {
		return err
	}
	if err := validText("generation note", result.Note, 4096, result.Partial); err != nil {
		return err
	}
	seen := make(map[string]bool, len(result.Quizzes))
	for i, q := range result.Quizzes {
		if err := validateQuiz(q, src); err != nil {
			return fmt.Errorf("quiz %d: %w", i+1, err)
		}
		key := strings.TrimSpace(q.Prompt)
		if seen[key] {
			return fmt.Errorf("%w: duplicate generated prompt", ErrInvalid)
		}
		seen[key] = true
	}
	return nil
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
		if a.job.FoundationTarget != nil && costMicros == nil {
			status = "paused"
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
