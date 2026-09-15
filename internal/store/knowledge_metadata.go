package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/misty-step/scry/internal/learning"
)

func goalHeader(ctx context.Context, tx *sql.Tx, id string) (Goal, error) {
	var g Goal
	var settings, coverage string
	err := tx.QueryRowContext(ctx, `SELECT g.id,g.source_id,g.source_revision,g.title,g.revision,(g.archived OR s.archived),g.settings_json,g.coverage_json,g.created_at,g.updated_at
	 FROM goals g JOIN sources s ON s.id=g.source_id WHERE g.id=?`, id).Scan(&g.ID, &g.SourceID, &g.SourceRevision, &g.Title, &g.Revision, &g.Archived, &settings, &coverage, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return g, notFound(err, "goal")
	}
	if err = json.Unmarshal([]byte(settings), &g.Settings); err != nil {
		return g, err
	}
	if err = json.Unmarshal([]byte(coverage), &g.Coverage); err != nil {
		return g, err
	}
	g.Settings.ExpectedRevision = g.Revision
	g.MissingCoverageCount = len(g.Coverage.Missing)
	return g, nil
}

func materialMetadata(ctx context.Context, tx *sql.Tx, id string) (Material, error) {
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM materials WHERE id=?", id).Scan(&version); err != nil {
		return Material{}, notFound(err, "material")
	}
	return materialMetadataVersion(ctx, tx, id, version)
}

func materialMetadataVersion(ctx context.Context, tx *sql.Tx, id string, version int) (Material, error) {
	var m Material
	err := tx.QueryRowContext(ctx, `SELECT m.id,m.source_id,v.version,m.kind,(m.archived OR s.archived),m.created_at,m.first_presented_at,
	 json_extract(v.content,'$.title'),json_extract(v.content,'$.estimated_seconds'),COALESCE(json_extract(v.content,'$.level'),''),COALESCE(sc.due_at,0),
	 COALESCE(json_extract(v.provenance_json,'$.source_id'),''),COALESCE(json_extract(v.provenance_json,'$.source_revision'),0),COALESCE(json_extract(v.provenance_json,'$.job_id'),''),COALESCE(json_extract(v.provenance_json,'$.created_at'),0)
	 FROM materials m JOIN sources s ON s.id=m.source_id JOIN material_versions v ON v.material_id=m.id
	 LEFT JOIN schedules sc ON sc.quiz_id=v.quiz_id WHERE m.id=? AND v.version=?`, id, version).Scan(&m.ID, &m.SourceID, &m.Version, &m.Kind, &m.Archived, &m.CreatedAt, &m.FirstPresentedAt, &m.Title, &m.EstimatedSeconds, &m.Level, &m.DueAt, &m.Provenance.SourceID, &m.Provenance.SourceRevision, &m.Provenance.JobID, &m.Provenance.CreatedAt)
	if err != nil {
		return m, notFound(err, "material metadata")
	}
	m.MetadataOnly = true
	m.Links = []CoverageLink{}
	rows, err := tx.QueryContext(ctx, `SELECT id,material_id,material_version,unit_id,unit_version,role,
	 COALESCE(json_extract(provenance_json,'$.source_id'),''),COALESCE(json_extract(provenance_json,'$.source_revision'),0),COALESCE(json_extract(provenance_json,'$.job_id'),''),COALESCE(json_extract(provenance_json,'$.created_at'),0)
	 FROM material_links WHERE material_id=? AND material_version=? ORDER BY id`, id, version)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var link CoverageLink
		if err = rows.Scan(&link.ID, &link.MaterialID, &link.MaterialVersion, &link.UnitID, &link.UnitVersion, &link.Role, &link.Provenance.SourceID, &link.Provenance.SourceRevision, &link.Provenance.JobID, &link.Provenance.CreatedAt); err != nil {
			return m, err
		}
		m.Links = append(m.Links, link)
	}
	m.Unmapped = len(m.Links) == 0
	return m, rows.Err()
}

func goalLibraryMetadata(ctx context.Context, tx *sql.Tx, id string) (Goal, error) {
	g, err := goalHeader(ctx, tx, id)
	if err != nil {
		return g, err
	}
	g.MetadataOnly = true
	g.Settings.Reason = ""
	g.Coverage.Missing = nil
	g.Units = []KnowledgeUnit{}
	g.Estimates = []learning.Estimate{}
	g.Materials = []Material{}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT u.id,u.version,v.kind,u.archived,u.created_at FROM goal_units gu
	 JOIN knowledge_units u ON u.id=gu.unit_id JOIN unit_versions v ON v.unit_id=u.id AND v.version=u.version
	 WHERE gu.goal_id=? ORDER BY u.created_at,u.id`, id)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var unit KnowledgeUnit
		if err = rows.Scan(&unit.ID, &unit.Version, &unit.Kind, &unit.Archived, &unit.CreatedAt); err != nil {
			rows.Close()
			return g, err
		}
		g.Units = append(g.Units, unit)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return g, err
	}
	rows.Close()
	ids, err := rowIDs(ctx, tx, `SELECT DISTINCT m.id FROM goal_materials gm JOIN materials m ON m.id=gm.material_id WHERE gm.goal_id=? ORDER BY m.created_at,m.id`, id)
	if err != nil {
		return g, err
	}
	for _, mid := range ids {
		m, err := materialMetadata(ctx, tx, mid)
		if err != nil {
			return g, err
		}
		g.Materials = append(g.Materials, m)
	}
	g.UnitCount, g.MaterialCount = len(g.Units), len(g.Materials)
	return g, nil
}
