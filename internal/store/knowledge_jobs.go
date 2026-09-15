package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/misty-step/scry/internal/learning"
)

const eligibleKnowledgeJob = `
 EXISTS(SELECT 1 FROM sources src WHERE src.id=j.source_id AND src.archived=0 AND src.revision=j.source_revision)
 AND EXISTS(SELECT 1 FROM goals g WHERE g.id=j.goal_id AND g.archived=0 AND g.revision=j.goal_revision AND g.source_id=j.source_id AND g.source_revision=j.source_revision)
 AND (j.target_material_id='' OR EXISTS(SELECT 1 FROM materials m JOIN sources src ON src.id=m.source_id JOIN goal_materials gm ON gm.material_id=m.id AND gm.material_version=j.target_material_version WHERE m.id=j.target_material_id AND m.archived=0 AND src.archived=0 AND m.version=j.target_material_version AND gm.goal_id=j.goal_id))
 AND (j.target_presentation_id='' OR EXISTS(SELECT 1 FROM presentations p WHERE p.id=j.target_presentation_id AND p.material_id=j.target_material_id AND p.material_version=j.target_presentation_version AND p.goal_id=j.goal_id))
 AND (j.kind<>'bridge' OR EXISTS(SELECT 1 FROM bridges b WHERE b.job_id=j.id AND b.status IN('pending','active','ready_return')))
 AND (j.kind<>'expand' OR EXISTS(SELECT 1 FROM suggestions x JOIN plan_decisions d ON d.suggestion_id=x.id WHERE x.id=j.suggestion_id AND x.goal_id=j.goal_id AND x.status='accepted' AND d.kind='suggestion_accept' AND d.undone_at=0))
 AND (j.kind<>'enrich' OR j.target_material_id='' OR j.observation_id<>'')
 AND ` + observedGapAuthority

// enqueueSuggestionExpansion receives the post-decision goal revision. It binds
// paid work to one accepted proposal, never whichever proposal happens to be
// newest when a worker later claims the job.
func enqueueSuggestionExpansion(ctx context.Context, tx *sql.Tx, suggestionID string, g Goal, now int64) error {
	var unitsJSON string
	if err := tx.QueryRowContext(ctx, "SELECT unit_versions_json FROM suggestions WHERE id=? AND goal_id=? AND status='accepted'", suggestionID, g.ID).Scan(&unitsJSON); err != nil {
		return notFound(err, "accepted expansion suggestion")
	}
	var units []learning.UnitVersion
	if err := json.Unmarshal([]byte(unitsJSON), &units); err != nil {
		return err
	}
	if len(units) == 0 {
		return fmt.Errorf("%w: expansion proposal has no pinned knowledge scope", ErrInvalid)
	}
	// Reuse a genuinely relevant library resource for every proposed unit before
	// considering an external request. Membership itself creates no observation.
	allCovered := true
	for _, unit := range units {
		ids, err := rowIDs(ctx, tx, `SELECT DISTINCT m.id FROM materials m JOIN sources src ON src.id=m.source_id
		 JOIN material_links l ON l.material_id=m.id AND l.material_version=m.version
		 WHERE m.archived=0 AND src.archived=0 AND l.unit_id=? AND l.unit_version=? AND l.role IN('teaches','assesses') ORDER BY m.id`, unit.ID, unit.Version)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			allCovered = false
		}
		for _, id := range ids {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO goal_materials(goal_id,material_id,material_version,created_at)
			 SELECT ?,id,version,? FROM materials WHERE id=?`, g.ID, now, id); err != nil {
				return err
			}
		}
	}
	if allCovered {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE kind='expand' AND suggestion_id=? AND goal_revision=?)`, suggestionID, g.Revision).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,kind,goal_id,goal_revision,suggestion_id,status,created_at,updated_at,available_at)
	 VALUES(?,?,?,'expand',?,?,?,'queued',?,?,?)`, newID(), g.SourceID, g.SourceRevision, g.ID, g.Revision, suggestionID, now, now, now)
	return err
}

func suggestion(ctx context.Context, tx *sql.Tx, id string) (Suggestion, error) {
	var result Suggestion
	var units, materials, unitVersions, materialVersions string
	err := tx.QueryRowContext(ctx, `SELECT id,goal_id,goal_revision,kind,title,reason,unit_ids_json,material_ids_json,unit_versions_json,material_versions_json,status,origin_job_id,created_at,supersedes_id FROM suggestions WHERE id=?`, id).
		Scan(&result.ID, &result.GoalID, &result.GoalRevision, &result.Kind, &result.Title, &result.Reason, &units, &materials, &unitVersions, &materialVersions, &result.Status, &result.OriginJobID, &result.CreatedAt, &result.SupersedesID)
	if err != nil {
		return result, notFound(err, "suggestion")
	}
	for _, value := range []struct {
		text   string
		target any
	}{
		{units, &result.UnitIDs}, {materials, &result.MaterialIDs}, {unitVersions, &result.UnitVersions}, {materialVersions, &result.MaterialVersions},
	} {
		if err = json.Unmarshal([]byte(value.text), value.target); err != nil {
			return result, err
		}
	}
	return result, nil
}
