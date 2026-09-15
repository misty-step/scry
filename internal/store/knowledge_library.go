package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/learning"
	"strings"
)

func rowIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func knowledgeUnit(ctx context.Context, tx *sql.Tx, id string) (KnowledgeUnit, error) {
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM knowledge_units WHERE id=?", id).Scan(&version); err != nil {
		return KnowledgeUnit{}, notFound(err, "knowledge unit")
	}
	return knowledgeUnitVersion(ctx, tx, id, version)
}

func knowledgeUnitVersion(ctx context.Context, tx *sql.Tx, id string, version int) (KnowledgeUnit, error) {
	var u KnowledgeUnit
	var provenance string
	err := tx.QueryRowContext(ctx, `SELECT u.id,v.version,v.statement,v.kind,u.archived,v.provenance_json,u.created_at FROM knowledge_units u JOIN unit_versions v ON v.unit_id=u.id WHERE u.id=? AND v.version=?`, id, version).Scan(&u.ID, &u.Version, &u.Statement, &u.Kind, &u.Archived, &provenance, &u.CreatedAt)
	if err != nil {
		return u, notFound(err, "knowledge unit version")
	}
	err = json.Unmarshal([]byte(provenance), &u.Provenance)
	return u, err
}

func material(ctx context.Context, tx *sql.Tx, id string) (Material, error) {
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM materials WHERE id=?", id).Scan(&version); err != nil {
		return Material{}, notFound(err, "material")
	}
	return materialVersion(ctx, tx, id, version)
}

func materialVersion(ctx context.Context, tx *sql.Tx, id string, version int) (Material, error) {
	var m Material
	var content, provenance string
	var quizID sql.NullString
	var quizVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT m.id,m.source_id,v.version,m.kind,(m.archived OR s.archived),m.created_at,v.content,v.provenance_json,v.quiz_id,v.quiz_version,m.first_presented_at FROM materials m JOIN sources s ON s.id=m.source_id JOIN material_versions v ON v.material_id=m.id WHERE m.id=? AND v.version=?`, id, version).Scan(&m.ID, &m.SourceID, &m.Version, &m.Kind, &m.Archived, &m.CreatedAt, &content, &provenance, &quizID, &quizVersion, &m.FirstPresentedAt)
	if err != nil {
		return m, notFound(err, "material version")
	}
	var body Material
	if err = json.Unmarshal([]byte(content), &body); err != nil {
		return m, err
	}
	m.Title, m.Body, m.Basis, m.Evidence, m.ReferenceURL = body.Title, body.Body, body.Basis, body.Evidence, body.ReferenceURL
	m.StartSeconds, m.EndSeconds, m.EstimatedSeconds, m.Diagram, m.Level = body.StartSeconds, body.EndSeconds, body.EstimatedSeconds, body.Diagram, body.Level
	if err = json.Unmarshal([]byte(provenance), &m.Provenance); err != nil {
		return m, err
	}
	if quizID.Valid {
		var q Quiz
		var encoded string
		err = tx.QueryRowContext(ctx, `SELECT q.id,q.source_id,v.version,(q.archived OR src.archived),v.content,sc.due_at FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN quiz_versions v ON v.quiz_id=q.id JOIN schedules sc ON sc.quiz_id=q.id WHERE q.id=? AND v.version=?`, quizID.String, quizVersion.Int64).Scan(&q.ID, &q.SourceID, &q.Version, &q.Archived, &encoded, &q.DueAt)
		if err != nil {
			return m, err
		}
		var draft GeneratedQuiz
		if err = json.Unmarshal([]byte(encoded), &draft); err != nil {
			return m, err
		}
		q.Kind, q.Prompt, q.Answer, q.Explanation, q.Evidence, q.Basis = draft.Kind, draft.Prompt, draft.Answer, draft.Explanation, draft.Evidence, draft.Basis
		q.Choices, q.Variants = draft.Choices, draft.Variants
		m.Quiz, m.DueAt = &q, q.DueAt
	}
	m.Links = []CoverageLink{}
	rows, err := tx.QueryContext(ctx, `SELECT id,material_id,material_version,unit_id,unit_version,role,provenance_json FROM material_links WHERE material_id=? AND material_version=? ORDER BY id`, id, version)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var link CoverageLink
		var p string
		if err = rows.Scan(&link.ID, &link.MaterialID, &link.MaterialVersion, &link.UnitID, &link.UnitVersion, &link.Role, &p); err != nil {
			return m, err
		}
		if err = json.Unmarshal([]byte(p), &link.Provenance); err != nil {
			return m, err
		}
		m.Links = append(m.Links, link)
	}
	m.Unmapped = len(m.Links) == 0
	return m, rows.Err()
}

func (s *Store) Material(ctx context.Context, id string) (Material, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Material{}, err
	}
	defer tx.Rollback()
	m, err := material(ctx, tx, id)
	if err != nil {
		return m, err
	}
	return m, tx.Commit()
}

func (s *Store) Goals(ctx context.Context, query string) ([]Goal, error) {
	if err := validText("goal search", query, 1024, false); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(query)) + "%"
	ids, err := rowIDs(ctx, tx, `SELECT id FROM goals WHERE title LIKE ? ESCAPE '\' ORDER BY created_at DESC,id`, pattern)
	if err != nil {
		return nil, err
	}
	result := make([]Goal, 0, len(ids))
	for _, id := range ids {
		g, err := goalLibraryMetadata(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, tx.Commit()
}

func (s *Store) Goal(ctx context.Context, id string) (Goal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Goal{}, err
	}
	defer tx.Rollback()
	g, err := goal(ctx, tx, id)
	if err != nil {
		return g, err
	}
	if err = populateGoalEstimates(ctx, tx, &g, s.now()); err != nil {
		return g, err
	}
	return g, tx.Commit()
}

func goal(ctx context.Context, tx *sql.Tx, id string) (Goal, error) {
	g, err := goalHeader(ctx, tx, id)
	if err != nil {
		return g, err
	}
	ids, err := rowIDs(ctx, tx, `SELECT DISTINCT u.id FROM goal_units gu JOIN knowledge_units u ON u.id=gu.unit_id WHERE gu.goal_id=? ORDER BY u.created_at,u.id`, id)
	if err != nil {
		return g, err
	}
	g.Units = []KnowledgeUnit{}
	for _, uid := range ids {
		u, err := knowledgeUnit(ctx, tx, uid)
		if err != nil {
			return g, err
		}
		g.Units = append(g.Units, u)
	}
	ids, err = rowIDs(ctx, tx, `SELECT DISTINCT m.id FROM goal_materials gm JOIN materials m ON m.id=gm.material_id WHERE gm.goal_id=? ORDER BY m.created_at,m.rowid`, id)
	if err != nil {
		return g, err
	}
	g.Materials = []Material{}
	for _, mid := range ids {
		m, err := materialMetadata(ctx, tx, mid)
		if err != nil {
			return g, err
		}
		g.Materials = append(g.Materials, m)
	}
	ids, err = rowIDs(ctx, tx, "SELECT id FROM suggestions WHERE goal_id=? ORDER BY created_at,id", id)
	if err != nil {
		return g, err
	}
	g.Suggestions = []Suggestion{}
	for _, suggestionID := range ids {
		proposal, err := suggestion(ctx, tx, suggestionID)
		if err != nil {
			return g, err
		}
		g.Suggestions = append(g.Suggestions, proposal)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,goal_id,goal_revision,kind,reason,policy,material_id,material_version,suggestion_id,before_json,after_json,evidence_ids_json,unit_versions_json,reconsider_at,created_at,undone_at,undo_of FROM plan_decisions WHERE goal_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return g, err
	}
	g.Decisions = []PlanDecision{}
	for rows.Next() {
		var d PlanDecision
		var before, after, evidence, units string
		if err = rows.Scan(&d.ID, &d.GoalID, &d.GoalRevision, &d.Kind, &d.Reason, &d.Policy, &d.MaterialID, &d.MaterialVersion, &d.SuggestionID, &before, &after, &evidence, &units, &d.ReconsiderAt, &d.CreatedAt, &d.UndoneAt, &d.UndoOf); err != nil {
			rows.Close()
			return g, err
		}
		for _, v := range []struct {
			text   string
			target any
		}{{before, &d.Before}, {after, &d.After}, {evidence, &d.EvidenceIDs}, {units, &d.UnitVersions}} {
			if err = json.Unmarshal([]byte(v.text), v.target); err != nil {
				rows.Close()
				return g, err
			}
		}
		g.Decisions = append(g.Decisions, d)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return g, err
	}
	rows.Close()
	ids, err = rowIDs(ctx, tx, "SELECT id FROM jobs WHERE goal_id=? ORDER BY created_at,id", id)
	if err != nil {
		return g, err
	}
	g.Jobs = []Job{}
	for _, jid := range ids {
		j, err := jobMetadata(ctx, tx, jid)
		if err != nil {
			return g, err
		}
		g.Jobs = append(g.Jobs, j)
	}
	g.Estimates = []learning.Estimate{}
	for _, unit := range g.Units {
		var encoded string
		err := tx.QueryRowContext(ctx, `SELECT estimate_json FROM estimate_records WHERE unit_id=? AND unit_version=? AND mode='recall' ORDER BY created_at DESC,rowid DESC LIMIT 1`, unit.ID, unit.Version).Scan(&encoded)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return g, err
		}
		var estimate learning.Estimate
		if err = json.Unmarshal([]byte(encoded), &estimate); err != nil {
			return g, err
		}
		g.Estimates = append(g.Estimates, estimate)
	}
	g.UnitCount, g.MaterialCount = len(g.Units), len(g.Materials)
	return g, nil
}

func populateGoalEstimates(ctx context.Context, tx *sql.Tx, g *Goal, now int64) error {
	asOf := time.UnixMilli(now)
	evidence, err := knowledgeEvidence(ctx, tx, now)
	if err != nil {
		return err
	}
	_, relations, err := knowledgeRelations(ctx, tx, now)
	if err != nil {
		return err
	}
	g.Estimates = make([]learning.Estimate, 0, len(g.Units))
	for _, unit := range g.Units {
		g.Estimates = append(g.Estimates, learning.Infer(learning.UnitVersion{ID: unit.ID, Version: unit.Version}, "recall", asOf, asOf, evidence, relations))
	}
	return nil
}

func ensureGoal(ctx context.Context, tx *sql.Tx, sourceID string, now int64) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT id FROM goals WHERE source_id=?", sourceID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var src Source
	if err = tx.QueryRowContext(ctx, "SELECT text,kind,revision,archived FROM sources WHERE id=?", sourceID).Scan(&src.Text, &src.Kind, &src.Revision, &src.Archived); err != nil {
		return "", err
	}
	id = newID()
	title := strings.TrimSpace(src.Text)
	// A source is retained in full; the goal label is explicitly just its first line.
	if line, _, ok := strings.Cut(title, "\n"); ok {
		title = line
	}
	if len(title) > 240 {
		for len(title) > 237 {
			_, size := utf8.DecodeLastRuneInString(title)
			title = title[:len(title)-size]
		}
		title += "..."
	}
	settings := PlanSettings{TimeBudgetSeconds: 300, NewAssessmentsPerDay: 5, Focus: "goal", Reason: "Captured learning goal"}
	coverage := CoverageReport{Kind: "unmapped", Complete: false, Missing: []string{"Knowledge and material coverage has not yet been mapped"}}
	a, _ := marshal(settings)
	b, _ := marshal(coverage)
	_, err = tx.ExecContext(ctx, `INSERT INTO goals(id,source_id,source_revision,title,revision,archived,settings_json,coverage_json,created_at,updated_at) VALUES(?,?,?,?,1,?,?,?,?,?)`, id, sourceID, src.Revision, title, src.Archived, a, b, now, now)
	if err != nil {
		return "", err
	}
	snapshot, _ := marshal(Goal{ID: id, SourceID: sourceID, SourceRevision: src.Revision, Title: title, Revision: 1, Archived: src.Archived, Settings: settings, Coverage: coverage, CreatedAt: now, UpdatedAt: now})
	_, err = tx.ExecContext(ctx, "INSERT INTO goal_versions(goal_id,revision,snapshot,created_at) VALUES(?,1,?,?)", id, snapshot, now)
	return id, err
}

func appendCorrection(ctx context.Context, tx *sql.Tx, kind, id string, before, after int, reason string, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO knowledge_corrections(id,kind,entity_id,before_version,after_version,reason,created_at) VALUES(?,?,?,?,?,?,?)`, newID(), kind, id, before, after, reason, now)
	return err
}

func validateReason(reason string) error { return validText("correction reason", reason, 4096, true) }

func (s *Store) EditUnit(ctx context.Context, id string, expectedVersion int, statement, kind, reason, operationID string) (KnowledgeUnit, error) {
	if err := validOperation(operationID); err != nil {
		return KnowledgeUnit{}, err
	}
	if err := validateReason(reason); err != nil {
		return KnowledgeUnit{}, err
	}
	if err := validateUnit(GeneratedUnit{Key: "edit", Statement: statement, Kind: kind}); err != nil {
		return KnowledgeUnit{}, err
	}
	hash := payloadHash([]any{id, expectedVersion, statement, kind, reason})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KnowledgeUnit{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "edit-unit", hash)
	if err != nil {
		return KnowledgeUnit{}, err
	}
	if found {
		var u KnowledgeUnit
		err = json.Unmarshal([]byte(receipt), &u)
		return u, err
	}
	u, err := knowledgeUnit(ctx, tx, id)
	if err != nil {
		return u, err
	}
	if u.Version != expectedVersion || u.Archived {
		return u, fmt.Errorf("%w: knowledge unit changed or archived", ErrConflict)
	}
	now := s.now()
	u.Version++
	u.Statement, u.Kind = statement, kind
	u.Provenance = Provenance{Basis: "manual", Reason: reason, CreatedAt: now}
	p, _ := marshal(u.Provenance)
	if _, err = tx.ExecContext(ctx, "INSERT INTO unit_versions(unit_id,version,statement,kind,provenance_json,created_at) VALUES(?,?,?,?,?,?)", id, u.Version, statement, kind, p, now); err != nil {
		return u, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE knowledge_units SET version=? WHERE id=?", u.Version, id); err != nil {
		return u, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO goal_units(goal_id,unit_id,unit_version,origin_job_id,created_at) SELECT DISTINCT goal_id,unit_id,?,NULL,? FROM goal_units WHERE unit_id=?`, u.Version, now, id); err != nil {
		return u, err
	}
	if err = appendCorrection(ctx, tx, "unit", id, expectedVersion, u.Version, reason, now); err != nil {
		return u, err
	}
	if err = markGoalsUnmapped(ctx, tx, id, "Knowledge definition changed; existing material coverage remains pinned to its prior definition", now); err != nil {
		return u, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return u, err
	}
	if err = saveOperation(ctx, tx, operationID, "edit-unit", hash, id, u, now); err != nil {
		return u, err
	}
	return u, tx.Commit()
}

func markGoalsUnmapped(ctx context.Context, tx *sql.Tx, unitID, reason string, now int64) error {
	ids, err := rowIDs(ctx, tx, "SELECT DISTINCT g.id FROM goals g JOIN goal_units gu ON gu.goal_id=g.id WHERE gu.unit_id=? AND g.archived=0 ORDER BY g.id", unitID)
	if err != nil {
		return err
	}
	return invalidateGoalCoverage(ctx, tx, ids, reason, now)
}

func invalidateMaterialGoals(ctx context.Context, tx *sql.Tx, materialID, reason string, now int64) error {
	ids, err := rowIDs(ctx, tx, "SELECT DISTINCT g.id FROM goals g JOIN goal_materials gm ON gm.goal_id=g.id WHERE gm.material_id=? AND g.archived=0 ORDER BY g.id", materialID)
	if err != nil {
		return err
	}
	return invalidateGoalCoverage(ctx, tx, ids, reason, now)
}

func invalidateGoalCoverage(ctx context.Context, tx *sql.Tx, ids []string, reason string, now int64) error {
	for _, id := range ids {
		g, err := goal(ctx, tx, id)
		if err != nil {
			return err
		}
		before := g.Revision
		g.Coverage.Complete = false
		found := false
		for _, missing := range g.Coverage.Missing {
			found = found || missing == reason
		}
		if !found {
			g.Coverage.Missing = append(g.Coverage.Missing, reason)
		}
		encoded, err := marshal(g.Coverage)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE goals SET coverage_json=? WHERE id=?", encoded, id); err != nil {
			return err
		}
		if err = advanceGoalPlan(ctx, tx, &g, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Modeled goal scope changed; inspect the retained target before explicit retry',updated_at=? WHERE goal_id=? AND goal_revision<>? AND status IN('queued','retry','paused')`, now, g.ID, g.Revision); err != nil {
			return err
		}
		if err = appendCorrection(ctx, tx, "goal", id, before, g.Revision, reason, now); err != nil {
			return err
		}
		if err = refreshPendingSuggestions(ctx, tx, g, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EditMaterial(ctx context.Context, id string, expectedVersion int, draft GeneratedMaterial, reason, operationID string) (Material, error) {
	if err := validOperation(operationID); err != nil {
		return Material{}, err
	}
	if err := validateReason(reason); err != nil {
		return Material{}, err
	}
	hash := payloadHash([]any{id, expectedVersion, draft, reason})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Material{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "edit-material", hash)
	if err != nil {
		return Material{}, err
	}
	if found {
		var m Material
		err = json.Unmarshal([]byte(receipt), &m)
		return m, err
	}
	m, err := material(ctx, tx, id)
	if err != nil {
		return m, err
	}
	if m.Version != expectedVersion || m.Archived || m.Kind == "quiz" {
		return m, fmt.Errorf("%w: material changed, archived, or requires quiz editing", ErrConflict)
	}
	src, err := source(ctx, tx, m.SourceID, false)
	if err != nil {
		return m, err
	}
	if draft.Kind != m.Kind {
		return m, fmt.Errorf("%w: material kind cannot change", ErrInvalid)
	}
	if draft.Key != "" || draft.ReuseID != "" || len(draft.Links) > 0 {
		return m, fmt.Errorf("%w: edit material content separately from coverage", ErrInvalid)
	}
	if err = validateMaterial(draft, src, []string{m.ReferenceURL}); err != nil {
		return m, err
	}
	now := s.now()
	m.Version++
	m.Title, m.Body, m.Basis, m.Evidence, m.ReferenceURL = draft.Title, draft.Body, draft.Basis, draft.Evidence, draft.ReferenceURL
	m.StartSeconds, m.EndSeconds, m.EstimatedSeconds, m.Diagram = draft.StartSeconds, draft.EndSeconds, draft.EstimatedSeconds, draft.Diagram
	m.Provenance = Provenance{SourceID: src.ID, SourceRevision: src.Revision, Basis: draft.Basis, Evidence: draft.Evidence, Reason: reason, CreatedAt: now}
	if err = saveMaterialVersion(ctx, tx, m, m.Links, now); err != nil {
		return m, err
	}
	if err = appendCorrection(ctx, tx, "material", id, expectedVersion, m.Version, reason, now); err != nil {
		return m, err
	}
	if err = invalidateMaterialGoals(ctx, tx, id, "Instructional content changed; prior authored coverage requires review", now); err != nil {
		return m, err
	}
	if err = retireMaterial(ctx, tx, id, "", now); err != nil {
		return m, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return m, err
	}
	m, err = material(ctx, tx, id)
	if err != nil {
		return m, err
	}
	if err = saveOperation(ctx, tx, operationID, "edit-material", hash, id, m, now); err != nil {
		return m, err
	}
	return m, tx.Commit()
}

func (s *Store) EditCoverage(ctx context.Context, id string, expectedVersion int, links []CoverageLink, reason, operationID string) (Material, error) {
	if err := validOperation(operationID); err != nil {
		return Material{}, err
	}
	if err := validateReason(reason); err != nil {
		return Material{}, err
	}
	hash := payloadHash([]any{id, expectedVersion, links, reason})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Material{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "edit-coverage", hash)
	if err != nil {
		return Material{}, err
	}
	if found {
		var m Material
		err = json.Unmarshal([]byte(receipt), &m)
		return m, err
	}
	m, err := material(ctx, tx, id)
	if err != nil {
		return m, err
	}
	if m.Version != expectedVersion || m.Archived {
		return m, fmt.Errorf("%w: material changed or archived", ErrConflict)
	}
	if err = validateCoverage(ctx, tx, m.Kind, links); err != nil {
		return m, err
	}
	now := s.now()
	m.Version++
	m.Provenance.Reason = reason
	m.Provenance.CreatedAt = now
	if err = saveMaterialVersion(ctx, tx, m, links, now); err != nil {
		return m, err
	}
	if err = appendCorrection(ctx, tx, "coverage", id, expectedVersion, m.Version, reason, now); err != nil {
		return m, err
	}
	if err = invalidateMaterialGoals(ctx, tx, id, "Material coverage was corrected; previous completion no longer certifies the current map", now); err != nil {
		return m, err
	}
	if err = retireMaterial(ctx, tx, id, "", now); err != nil {
		return m, err
	}
	m, err = material(ctx, tx, id)
	if err != nil {
		return m, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return m, err
	}
	if err = saveOperation(ctx, tx, operationID, "edit-coverage", hash, id, m, now); err != nil {
		return m, err
	}
	return m, tx.Commit()
}

func validateCoverage(ctx context.Context, tx *sql.Tx, kind string, links []CoverageLink) error {
	if len(links) > MaxMaterialLinks {
		return fmt.Errorf("%w: too many coverage links", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, l := range links {
		if !validRole(l.Role) || (kind != "quiz" && l.Role == "assesses") {
			return fmt.Errorf("%w: incompatible coverage role", ErrInvalid)
		}
		key := fmt.Sprintf("%s/%d/%s", l.UnitID, l.UnitVersion, l.Role)
		if seen[key] {
			return fmt.Errorf("%w: duplicate coverage link", ErrInvalid)
		}
		seen[key] = true
		u, err := knowledgeUnitVersion(ctx, tx, l.UnitID, l.UnitVersion)
		if err != nil {
			return err
		}
		if u.Archived {
			return fmt.Errorf("%w: archived coverage unit", ErrConflict)
		}
	}
	return nil
}

func saveMaterialVersion(ctx context.Context, tx *sql.Tx, m Material, links []CoverageLink, now int64) error {
	body := m
	body.Links = nil
	body.Quiz = nil
	encoded, err := marshal(body)
	if err != nil {
		return err
	}
	p, err := marshal(m.Provenance)
	if err != nil {
		return err
	}
	var qid, qversion any
	if m.Quiz != nil {
		qid = m.Quiz.ID
		qversion = m.Quiz.Version
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO material_versions(material_id,version,content,quiz_id,quiz_version,provenance_json,created_at) VALUES(?,?,?,?,?,?,?)`, m.ID, m.Version, encoded, qid, qversion, p, now); err != nil {
		return err
	}
	for _, l := range links {
		lp := m.Provenance
		if l.Provenance.SourceID != "" {
			lp = l.Provenance
			lp.Reason = m.Provenance.Reason
			lp.CreatedAt = now
		}
		p, err := marshal(lp)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO material_links(id,material_id,material_version,unit_id,unit_version,role,provenance_json) VALUES(?,?,?,?,?,?,?)`, newID(), m.ID, m.Version, l.UnitID, l.UnitVersion, l.Role, p); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE materials SET version=? WHERE id=?", m.Version, m.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO goal_materials(goal_id,material_id,material_version,origin_job_id,created_at) SELECT DISTINCT goal_id,material_id,?,NULL,? FROM goal_materials WHERE material_id=?`, m.Version, now, m.ID)
	return err
}

func relation(ctx context.Context, tx *sql.Tx, id string) (KnowledgeRelation, error) {
	var r KnowledgeRelation
	var p string
	err := tx.QueryRowContext(ctx, `SELECT r.id,v.version,v.from_id,v.from_version,v.to_id,v.to_version,v.kind,v.proposed,r.archived,v.provenance_json FROM knowledge_relations r JOIN relation_versions v ON v.relation_id=r.id AND v.version=r.version WHERE r.id=?`, id).Scan(&r.ID, &r.Version, &r.FromID, &r.FromVersion, &r.ToID, &r.ToVersion, &r.Kind, &r.Proposed, &r.Archived, &p)
	if err != nil {
		return r, notFound(err, "knowledge relation")
	}
	err = json.Unmarshal([]byte(p), &r.Provenance)
	return r, err
}

func (s *Store) EditRelation(ctx context.Context, c RelationChange, operationID string) (KnowledgeRelation, error) {
	if err := validOperation(operationID); err != nil {
		return KnowledgeRelation{}, err
	}
	if err := validateReason(c.Reason); err != nil {
		return KnowledgeRelation{}, err
	}
	if !validRelationKind(c.Kind) || c.FromID == c.ToID {
		return KnowledgeRelation{}, fmt.Errorf("%w: invalid relation", ErrInvalid)
	}
	if err := validText("relation evidence", c.Evidence, 8192, false); err != nil {
		return KnowledgeRelation{}, err
	}
	hash := payloadHash(c)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KnowledgeRelation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "edit-relation", hash)
	if err != nil {
		return KnowledgeRelation{}, err
	}
	if found {
		var r KnowledgeRelation
		err = json.Unmarshal([]byte(receipt), &r)
		return r, err
	}
	for _, v := range []struct {
		id      string
		version int
	}{{c.FromID, c.FromVersion}, {c.ToID, c.ToVersion}} {
		u, err := knowledgeUnitVersion(ctx, tx, v.id, v.version)
		if err != nil {
			return KnowledgeRelation{}, err
		}
		if u.Archived {
			return KnowledgeRelation{}, fmt.Errorf("%w: archived relation unit", ErrConflict)
		}
	}
	now := s.now()
	id := c.ID
	version := 1
	if id == "" {
		if c.ExpectedVersion != 0 {
			return KnowledgeRelation{}, fmt.Errorf("%w: new relation has no prior version", ErrInvalid)
		}
		id = newID()
		_, err = tx.ExecContext(ctx, "INSERT INTO knowledge_relations(id,version,archived,created_at) VALUES(?,1,?,?)", id, c.Archived, now)
	} else {
		r, e := relation(ctx, tx, id)
		if e != nil {
			return r, e
		}
		if r.Version != c.ExpectedVersion {
			return r, fmt.Errorf("%w: relation changed", ErrConflict)
		}
		version = r.Version + 1
		_, err = tx.ExecContext(ctx, "UPDATE knowledge_relations SET version=?,archived=? WHERE id=?", version, c.Archived, id)
	}
	if err != nil {
		return KnowledgeRelation{}, err
	}
	p := Provenance{Basis: "manual", Evidence: c.Evidence, Reason: c.Reason, CreatedAt: now}
	encoded, _ := marshal(p)
	_, err = tx.ExecContext(ctx, `INSERT INTO relation_versions(relation_id,version,from_id,from_version,to_id,to_version,kind,proposed,provenance_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, version, c.FromID, c.FromVersion, c.ToID, c.ToVersion, c.Kind, c.Proposed, encoded, now)
	if err != nil {
		return KnowledgeRelation{}, err
	}
	if err = appendCorrection(ctx, tx, "relation", id, c.ExpectedVersion, version, c.Reason, now); err != nil {
		return KnowledgeRelation{}, err
	}
	affected, err := rowIDs(ctx, tx, `SELECT DISTINCT g.id FROM goals g JOIN goal_units gu ON gu.goal_id=g.id JOIN relation_versions rv ON rv.from_id=gu.unit_id OR rv.to_id=gu.unit_id WHERE rv.relation_id=? AND g.archived=0 ORDER BY g.id`, id)
	if err != nil {
		return KnowledgeRelation{}, err
	}
	if err = invalidateGoalCoverage(ctx, tx, affected, "Knowledge relationships changed; prior authored graph coverage requires review", now); err != nil {
		return KnowledgeRelation{}, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return KnowledgeRelation{}, err
	}
	r, err := relation(ctx, tx, id)
	if err != nil {
		return r, err
	}
	if err = saveOperation(ctx, tx, operationID, "edit-relation", hash, id, r, now); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
