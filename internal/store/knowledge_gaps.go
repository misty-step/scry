package store

import (
	"context"
	"database/sql"
)

const observedGapAuthority = `(j.observation_id='' OR EXISTS(
 SELECT 1 FROM review_events e JOIN presentations p ON p.id=e.presentation_id
 WHERE e.id=j.observation_id AND e.presentation_id=j.target_presentation_id
 AND e.outcome='wrong' AND e.rating=1 AND e.assisted=0 AND p.practice=0
 AND json_extract(e.snapshot,'$.kind') IN('choice','recall')
 AND p.goal_id=j.goal_id AND p.material_id=j.target_material_id AND p.material_version=j.target_material_version
 AND NOT EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=e.id)
 AND NOT EXISTS(SELECT 1 FROM knowledge_corrections c WHERE
  (c.kind IN('material','coverage') AND c.entity_id=p.material_id AND c.before_version>=p.material_version)
  OR (c.kind='unit' AND EXISTS(SELECT 1 FROM material_links l WHERE l.material_id=p.material_id AND l.material_version=p.material_version AND l.unit_id=c.entity_id AND l.unit_version<=c.before_version))
  OR (c.kind='relation' AND EXISTS(SELECT 1 FROM relation_versions rv JOIN material_links l ON l.material_id=p.material_id AND l.material_version=p.material_version WHERE rv.relation_id=c.entity_id AND rv.version<=c.before_version AND ((rv.from_id=l.unit_id AND rv.from_version=l.unit_version) OR (rv.to_id=l.unit_id AND rv.to_version=l.unit_version)))))))`

func queueObservedGap(ctx context.Context, tx *sql.Tx, p Presentation, now int64) error {
	if p.ReviewID == "" || p.Rating != 1 || p.Outcome != "wrong" || p.Assisted || p.Practice || p.Material == nil {
		return nil
	}
	g, err := retainedGoal(ctx, tx, p)
	if err != nil {
		return err
	}
	current, err := materialMetadata(ctx, tx, p.Material.ID)
	if err != nil {
		return err
	}
	if current.Archived || current.Version != p.Material.Version {
		return nil
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE kind='enrich' AND observation_id<>'' AND goal_id=? AND goal_revision=? AND target_material_id=? AND target_material_version=?)`, g.ID, g.Revision, p.Material.ID, p.Material.Version).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	// Useful saved instruction or a foundational probe wins over paid work.
	b := Bridge{GoalID: g.ID, TargetMaterialID: p.Material.ID, TargetMaterialVersion: p.Material.Version, UpdatedAt: now}
	ids, err := bridgeCandidates(ctx, tx, b)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		m, err := materialMetadata(ctx, tx, ids[0])
		if err != nil {
			return err
		}
		b.Path, b.PathVersions = []string{m.ID}, []int{m.Version}
		return attachBridgePath(ctx, tx, b, now)
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,kind,goal_id,goal_revision,target_material_id,target_material_version,target_presentation_id,target_presentation_version,observation_id,status,created_at,updated_at,available_at)
	 VALUES(?,?,?,'enrich',?,?,?,?,?,?,?,'queued',?,?,?)`, id, g.SourceID, g.SourceRevision, g.ID, g.Revision, p.Material.ID, p.Material.Version, p.ID, p.Material.Version, p.ReviewID, now, now, now); err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, "SELECT "+observedGapAuthority+" FROM jobs j WHERE j.id=?", id).Scan(&valid); err != nil {
		return err
	}
	if valid {
		return nil
	}
	_, err = tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',error='Observed task has disputed or corrected scope; no automatic external request authorized' WHERE id=?", id)
	return err
}

func cancelDisputedGap(ctx context.Context, tx *sql.Tx, reviewID string, now int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active' AND job_id IN(SELECT id FROM jobs WHERE observation_id=? AND status='running')`, now, reviewID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Original failure was disputed; automatic gap preparation withdrawn; any in-flight charge remains accounted',lease_token='',lease_until=0,updated_at=? WHERE observation_id=? AND status IN('queued','running','retry','paused')`, now, reviewID)
	return err
}
