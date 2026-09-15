package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func bridgeAllowance(ctx context.Context, tx *sql.Tx, goalID string, now int64) (int, int, error) {
	g, err := goalHeader(ctx, tx, goalID)
	if err != nil {
		return 0, 0, err
	}
	at := time.UnixMilli(now).UTC()
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	var since, spent int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(created_at),?) FROM plan_decisions WHERE goal_id=? AND kind='plan' AND undone_at=0 AND created_at>=?`, day, goalID, day).Scan(&since); err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(CAST(json_extract(v.content,'$.estimated_seconds') AS INTEGER)),0) FROM presentations p JOIN material_versions v ON v.material_id=p.material_id AND v.version=p.material_version WHERE p.goal_id=? AND p.created_at>=?`, goalID, since).Scan(&spent); err != nil {
		return 0, 0, err
	}
	var introduced, reserved int
	if err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT i.material_id) FROM interactions i WHERE i.kind IN('review','practice') AND i.at>=? AND i.at<=? AND NOT EXISTS(SELECT 1 FROM interactions prior WHERE prior.material_id=i.material_id AND prior.kind IN('review','practice') AND prior.at<?)`, day, now, day).Scan(&introduced); err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT p.material_id) FROM presentations p WHERE p.kind='quiz' AND p.graded=0 AND p.id IN(SELECT current_id FROM review_session WHERE singleton=1 UNION SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return')) AND NOT EXISTS(SELECT 1 FROM interactions i WHERE i.material_id=p.material_id AND i.kind IN('review','practice'))`).Scan(&reserved); err != nil {
		return 0, 0, err
	}
	return max(0, g.Settings.TimeBudgetSeconds-int(spent)), max(0, g.Settings.NewAssessmentsPerDay-introduced-reserved), nil
}

func bridgeCandidates(ctx context.Context, tx *sql.Tx, b Bridge) ([]string, error) {
	target, err := materialMetadataVersion(ctx, tx, b.TargetMaterialID, b.TargetMaterialVersion)
	if err != nil {
		return nil, err
	}
	pins := []learning.UnitVersion{}
	for _, link := range target.Links {
		if link.Role == "assesses" || link.Role == "assumes" {
			pins = append(pins, learning.UnitVersion{ID: link.UnitID, Version: link.UnitVersion})
		}
	}
	if len(pins) == 0 {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT gu.unit_id,gu.unit_version FROM goal_units gu WHERE gu.goal_id=?`, b.GoalID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pin learning.UnitVersion
			if err = rows.Scan(&pin.ID, &pin.Version); err != nil {
				rows.Close()
				return nil, err
			}
			pins = append(pins, pin)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	encoded, err := marshal(pins)
	if err != nil {
		return nil, err
	}
	return rowIDs(ctx, tx, `WITH RECURSIVE prerequisites(id,version) AS (
	 SELECT json_extract(value,'$.id'),json_extract(value,'$.version') FROM json_each(?)
	 UNION
	 SELECT rv.from_id,rv.from_version FROM prerequisites p JOIN relation_versions rv ON rv.to_id=p.id AND rv.to_version=p.version
	 JOIN knowledge_relations r ON r.id=rv.relation_id AND r.version=rv.version WHERE r.archived=0 AND rv.kind IN('prerequisite','composition'))
	 SELECT DISTINCT m.id FROM prerequisites p JOIN material_links l ON l.unit_id=p.id AND l.unit_version=p.version
	 JOIN materials m ON m.id=l.material_id AND m.version=l.material_version JOIN sources s ON s.id=m.source_id
	 JOIN material_versions v ON v.material_id=m.id AND v.version=m.version
	 WHERE m.archived=0 AND s.archived=0 AND m.id<>? AND l.role IN('teaches','assesses')
	 AND json_extract(v.content,'$.basis')<>'reference' AND (m.kind<>'quiz' OR json_extract(v.content,'$.level')='foundation')
	 ORDER BY (m.kind='quiz'),CAST(json_extract(v.content,'$.estimated_seconds') AS INTEGER),m.created_at,m.id LIMIT 64`, encoded, b.TargetMaterialID)
}

func materialPreviouslyAttempted(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var attempted bool
	err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM interactions WHERE material_id=? AND kind IN('review','practice'))", id).Scan(&attempted)
	return attempted, err
}
