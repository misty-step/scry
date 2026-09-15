package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func retryJobScope(ctx context.Context, tx *sql.Tx, j Job) (Goal, error) {
	var g Goal
	var settings string
	err := tx.QueryRowContext(ctx, `SELECT g.id,g.source_id,g.source_revision,g.revision,g.title,g.settings_json FROM goals g JOIN sources s ON s.id=g.source_id WHERE g.id=? AND g.archived=0 AND s.archived=0 AND s.revision=? AND g.source_revision=s.revision`, j.GoalID, j.SourceRevision).
		Scan(&g.ID, &g.SourceID, &g.SourceRevision, &g.Revision, &g.Title, &settings)
	if err != nil {
		return g, fmt.Errorf("%w: original source/goal scope is no longer active", ErrConflict)
	}
	if err = json.Unmarshal([]byte(settings), &g.Settings); err != nil {
		return g, err
	}
	if j.TargetMaterialID != "" {
		m, err := materialMetadata(ctx, tx, j.TargetMaterialID)
		if err != nil {
			return g, err
		}
		if m.Archived || m.Version != j.TargetMaterialVersion {
			return g, fmt.Errorf("%w: original target material changed", ErrConflict)
		}
		var member bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM goal_materials WHERE goal_id=? AND material_id=? AND material_version=?)", g.ID, j.TargetMaterialID, j.TargetMaterialVersion).Scan(&member); err != nil {
			return g, err
		}
		if !member {
			return g, fmt.Errorf("%w: original target is outside the chosen goal", ErrConflict)
		}
	}
	if j.Kind == "bridge" {
		var active bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bridges WHERE target_presentation_id=? AND target_material_id=? AND target_material_version=? AND status IN('pending','active','ready_return'))`, j.TargetPresentationID, j.TargetMaterialID, j.TargetMaterialVersion).Scan(&active); err != nil {
			return g, err
		}
		if !active {
			return g, fmt.Errorf("%w: retained bridge is no longer active", ErrConflict)
		}
	}
	if j.ObservationID != "" {
		var valid bool
		if err = tx.QueryRowContext(ctx, "SELECT "+observedGapAuthority+" FROM jobs j WHERE j.id=?", j.ID).Scan(&valid); err != nil {
			return g, err
		}
		if !valid {
			return g, fmt.Errorf("%w: original observed failure is disputed or corrected", ErrConflict)
		}
	}
	if j.Kind == "expand" {
		proposal, err := suggestion(ctx, tx, j.SuggestionID)
		if err != nil {
			return g, err
		}
		var accepted bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM plan_decisions WHERE suggestion_id=? AND kind='suggestion_accept' AND undone_at=0)`, proposal.ID).Scan(&accepted); err != nil {
			return g, err
		}
		fits, _, err := suggestionFits(ctx, tx, proposal, g)
		if err != nil {
			return g, err
		}
		if proposal.Status != "accepted" || !accepted || !fits {
			return g, fmt.Errorf("%w: accepted proposal scope changed or was undone", ErrConflict)
		}
	}
	return g, nil
}

func jobRetryMetadata(ctx context.Context, tx *sql.Tx, j *Job) error {
	if j.RetryRootID == "" {
		j.RetryRootID = j.ID
	}
	if j.Published != 0 || (j.Status != "failed" && j.Status != "canceled" && j.Status != "paused") {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE id=? OR retry_root_id=?", j.RetryRootID, j.RetryRootID).Scan(&count); err != nil {
		return err
	}
	if count >= 3 {
		j.RetryWarning = "This exact work has reached its three explicit runs; inspect and revise the goal instead."
		return nil
	}
	if _, err := retryJobScope(ctx, tx, *j); err != nil {
		j.RetryWarning = "The original source, retained target, or accepted scope is no longer compatible. Inspect the saved work before making a new choice."
		return nil
	}
	j.RetryAvailable = true
	j.RetryWarning = "A new bounded generation attempt may incur an additional charge; all prior usage remains accounted."
	if j.CostUnknown {
		j.RetryWarning = "Prior request usage is unknown. A new attempt may charge again; its prior reserved allowance remains fully accounted."
	}
	return nil
}

func (s *Store) RetryJob(ctx context.Context, jobID, operationID string) (Job, error) {
	if err := validOperation(operationID); err != nil {
		return Job{}, err
	}
	hash := payloadHash(jobID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "retry-job", hash)
	if err != nil {
		return Job{}, err
	}
	if found {
		var result Job
		err = json.Unmarshal([]byte(receipt), &result)
		hideJobContext(&result)
		return result, err
	}
	original, err := job(ctx, tx, jobID)
	if err != nil {
		return Job{}, err
	}
	if !original.RetryAvailable {
		return Job{}, fmt.Errorf("%w: work is not eligible for explicit retry", ErrConflict)
	}
	g, err := retryJobScope(ctx, tx, original)
	if err != nil {
		return Job{}, err
	}
	var live bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE retry_root_id=? AND status IN('queued','running','retry','paused') AND id<>?)`, original.RetryRootID, original.ID).Scan(&live); err != nil {
		return Job{}, err
	}
	if live {
		return Job{}, fmt.Errorf("%w: a successor for this exact work already exists", ErrConflict)
	}
	now := s.now()
	if original.Status == "paused" {
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',updated_at=? WHERE id=?", now, original.ID); err != nil {
			return Job{}, err
		}
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,kind,goal_id,goal_revision,target_material_id,target_material_version,target_presentation_id,target_presentation_version,suggestion_id,observation_id,parent_job_id,retry_root_id,status,created_at,updated_at,available_at)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'queued',?,?,?)`, id, original.SourceID, original.SourceRevision, original.Kind, g.ID, g.Revision, original.TargetMaterialID, original.TargetMaterialVersion, original.TargetPresentationID, original.TargetPresentationVersion, original.SuggestionID, original.ObservationID, original.ID, original.RetryRootID, now, now, now)
	if err != nil {
		return Job{}, err
	}
	if original.Kind == "bridge" {
		if _, err = tx.ExecContext(ctx, "UPDATE bridges SET job_id=?,status='pending',reason='Explicit bounded retry requested; original target remains retained',updated_at=? WHERE job_id=? AND status IN('pending','active','ready_return')", id, now, original.ID); err != nil {
			return Job{}, err
		}
	}
	result, err := jobMetadata(ctx, tx, id)
	if err != nil {
		return Job{}, err
	}
	if err = saveOperation(ctx, tx, operationID, "retry-job", hash, id, result, now); err != nil {
		return Job{}, err
	}
	return result, tx.Commit()
}

// Only explicit plan mutations call this. Unsent jobs can retain their identity
// while rebinding a compatible goal revision; attempted authority is immutable.
func rebindUnclaimedKnowledgeJobs(ctx context.Context, tx *sql.Tx, g Goal, now int64) error {
	ids, err := rowIDs(ctx, tx, `SELECT id FROM jobs WHERE goal_id=? AND goal_revision<>? AND status='queued' AND attempts=0 AND context_json='{}' AND NOT EXISTS(SELECT 1 FROM job_attempts a WHERE a.job_id=jobs.id)`, g.ID, g.Revision)
	if err != nil {
		return err
	}
	for _, id := range ids {
		j, err := job(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err = retryJobScope(ctx, tx, j); err != nil {
			continue
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO job_changes(id,job_id,before_goal_revision,after_goal_revision,reason,created_at) VALUES(?,?,?,?,?,?)", newID(), j.ID, j.GoalRevision, g.Revision, "Explicit plan change rebound compatible unsent work; no request, attempt, reservation, or paid context existed", now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET goal_revision=?,updated_at=? WHERE id=? AND status='queued' AND attempts=0", g.Revision, now, j.ID); err != nil {
			return err
		}
	}
	return nil
}
