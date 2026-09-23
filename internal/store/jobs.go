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

// Job kinds. A capture starts a sequential chain (at most one live job per
// source): research or transcribe, then plan, then questions. On-demand kinds
// (questions for one concept, fix) reuse the same queue,
// lease, spend reservation, and settlement rules. "quizzes" is the legacy
// single-call kind kept for historical rows and critic-pending batches.
var jobKinds = map[string]bool{"quizzes": true, "research": true, "transcribe": true, "plan": true, "questions": true, "fix": true}

func quizKind(kind string) bool {
	return kind == "quizzes" || kind == "questions" || kind == "fix"
}

func enqueue(ctx context.Context, tx *sql.Tx, sourceID string, revision int, kind string, payload any, now int64) error {
	if !jobKinds[kind] {
		return fmt.Errorf("%w: unknown job kind", ErrInvalid)
	}
	if kind == "research" {
		// Web research is the privacy boundary: only a source the learner
		// captured as a Topic or a Link may ever be researched.
		var mode string
		if err := tx.QueryRowContext(ctx, "SELECT mode FROM sources WHERE id=?", sourceID).Scan(&mode); err != nil {
			return err
		}
		if mode != "topic" && mode != "link" {
			return fmt.Errorf("%w: only a Topic or a Link is researched on the web", ErrInvalid)
		}
	}
	encoded := "{}"
	if payload != nil {
		text, err := marshal(payload)
		if err != nil {
			return err
		}
		encoded = text
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,status,created_at,updated_at,available_at,kind,payload)
	 VALUES(?,?,?,'queued',?,?,?,?,?)`, newID(), sourceID, revision, now, now, now, kind, encoded)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("%w: this material is still being prepared; try again when it finishes", ErrConflict)
	}
	return err
}

const jobColumns = `j.id,j.source_id,j.kind,j.payload,j.status,j.error,j.model,j.lease_token,r.text,r.kind,src.mode,src.web,j.source_revision,j.attempts,
 j.created_at,j.updated_at,j.published,
 COALESCE((SELECT sum(CASE WHEN a.state='active' THEN COALESCE(a.cost_micros,0) ELSE COALESCE(a.cost_micros,a.reserved_micros) END) FROM job_attempts a WHERE a.job_id=j.id),0)
 +COALESCE((SELECT sum(CASE WHEN a.status='pending' THEN 0 ELSE COALESCE(a.cost_micros,a.reserved_micros) END) FROM content_assessments a WHERE a.job_id=j.id),0),
 COALESCE((SELECT sum(a.reserved_micros) FROM job_attempts a WHERE a.job_id=j.id AND a.state='active'),0)
 +COALESCE((SELECT sum(a.reserved_micros) FROM content_assessments a WHERE a.job_id=j.id AND a.status='pending'),0),
 (EXISTS(SELECT 1 FROM job_attempts a WHERE a.job_id=j.id AND a.state<>'active' AND a.cost_micros IS NULL)
 OR EXISTS(SELECT 1 FROM content_assessments a WHERE a.job_id=j.id AND a.transmissions>0 AND a.cost_micros IS NULL)),j.critic_status,j.candidates_json`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var candidates sql.NullString
	err := row.Scan(&j.ID, &j.SourceID, &j.Kind, &j.Payload, &j.Status, &j.Error, &j.Model, &j.LeaseToken, &j.SourceText, &j.SourceKind, &j.SourceMode, &j.SourceWeb,
		&j.SourceRevision, &j.Attempts, &j.CreatedAt, &j.UpdatedAt, &j.Published, &j.CostMicros, &j.ReservedMicros, &j.CostUnknown, &j.CriticStatus, &candidates)
	if err == nil && candidates.Valid {
		err = json.Unmarshal([]byte(candidates.String), &j.Candidates)
	}
	return j, err
}

func job(ctx context.Context, tx *sql.Tx, id string) (Job, error) {
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+`
	 FROM jobs j JOIN sources src ON src.id=j.source_id JOIN source_revisions r ON r.source_id=j.source_id AND r.revision=j.source_revision WHERE j.id=?`, id))
	return j, notFound(err, "generation job")
}

// retiredJob excludes the retired foundation experience's jobs from claims.
const retiredJob = `EXISTS(SELECT 1 FROM foundation_requests f WHERE f.job_id=j.id)`

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
	var criticOnly bool
	err = tx.QueryRowContext(ctx, `SELECT j.id,j.candidates_json IS NOT NULL FROM jobs j JOIN sources src ON src.id=j.source_id WHERE j.status IN ('queued','retry')
	 AND j.available_at<=? AND (j.attempts<3 OR (j.candidates_json IS NOT NULL AND j.status='queued' AND j.attempts<5)) AND src.archived=0 AND src.revision=j.source_revision
	 AND NOT `+retiredJob+` ORDER BY j.available_at,j.created_at,j.id LIMIT 1`, now).Scan(&id, &criticOnly)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	if criticOnly {
		// Candidate assessments reserve individually before send. This claim
		// must not reserve (or charge) another generation request.
		reservationMicros = 0
	}
	if reservationMicros > 0 {
		spent, err := spentMicros(ctx, tx, now)
		if err != nil {
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
	if criticOnly {
		if _, err = tx.ExecContext(ctx, "UPDATE job_attempts SET cost_micros=0 WHERE token=?", token); err != nil {
			return nil, err
		}
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

func expireJobs(ctx context.Context, tx *sql.Tx, now int64) error {
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
	err = tx.QueryRowContext(ctx, `SELECT (j.status='running' AND j.lease_token=? AND j.lease_until>? AND src.archived=0 AND src.revision=j.source_revision)
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
	// Saved candidates already committed generator usage. Later critic
	// completion/failure cannot replace it, including a known zero retry claim.
	_, err := tx.ExecContext(ctx, `UPDATE job_attempts SET cost_micros=CASE WHEN EXISTS(SELECT 1 FROM jobs WHERE id=job_attempts.job_id AND candidates_json IS NOT NULL) THEN cost_micros ELSE ? END,state='settled',finished_at=?,finish_hash=?,finish_code=? WHERE token=? AND finish_hash=''`, costMicros, now, hash, code, token)
	return err
}

func finishStale(ctx context.Context, tx *sql.Tx, jobID, token string, now int64) error {
	// Do not touch a successor claim. If nobody has swept this expired claim
	// yet, recover it now; its already-settled actual/unknown cost remains.
	_, err := tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN EXISTS(SELECT 1 FROM sources src WHERE src.id=jobs.source_id AND (src.archived=1 OR src.revision<>jobs.source_revision))
	 THEN 'canceled' WHEN attempts>=3 THEN 'failed' ELSE 'retry' END,
	 error='Expired or stale completion discarded; usage recorded',lease_token='',lease_until=0,updated_at=?,available_at=?+60000
	 WHERE id=? AND status='running' AND lease_token=?`, now, now, jobID, token)
	return err
}

// CompleteJob validates and publishes a job's result by kind in one
// transaction, then enqueues the next step of a capture chain. Publication
// rechecks the claim, the source revision, saved candidates, and every
// provenance and concept fence; invalid output publishes nothing.
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
	if a.job.Candidates != nil {
		if payloadHash(result) != payloadHash(a.job.Candidates.Result) {
			return fmt.Errorf("%w: saved candidates cannot be replaced at publication", ErrConflict)
		}
		if a.job.CriticStatus != "skipped" {
			result, err = judgedCandidates(ctx, tx, a.job)
			if err != nil {
				return err
			}
		}
	}
	published, validation := 0, error(nil)
	if validation = validResultAttribution(result); validation == nil {
		published, validation = publish(ctx, tx, a.job, result, now)
	}
	if validation != nil {
		if !errors.Is(validation, ErrInvalid) {
			return validation
		}
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
	status := "complete"
	if result.Partial {
		status = "partial"
	}
	encoded, err := marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status=?,error=?,model=?,prompt_version=?,published=?,result_json=?,
	 critic_status=CASE WHEN critic_status='pending' THEN 'judged' ELSE critic_status END,
	 lease_token='',lease_until=0,updated_at=? WHERE id=?`, status, result.Note, result.Model, result.PromptVersion, published, encoded, now, jobID)
	if err != nil {
		return err
	}
	if err = settleAttempt(ctx, tx, leaseToken, hash, "", costMicros, now); err != nil {
		return err
	}
	// The chain continues only after this attempt is settled, so exactly one
	// live job exists for the source at any moment.
	switch a.job.Kind {
	case "research", "transcribe":
		if err = enqueue(ctx, tx, a.job.SourceID, a.job.SourceRevision, "plan", nil, now); err != nil {
			return err
		}
	case "plan":
		if published > 0 {
			if err = enqueue(ctx, tx, a.job.SourceID, a.job.SourceRevision, "questions", nil, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func validResultAttribution(result GenerationResult) error {
	if err := validText("model attribution", result.Model, 200, true); err != nil {
		return err
	}
	if err := validText("prompt version", result.PromptVersion, 200, true); err != nil {
		return err
	}
	return validText("generation note", result.Note, 4096, result.Partial)
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
