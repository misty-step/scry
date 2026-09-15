package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

// Unit derives a read-only estimate at an explicit target time. It never stores
// a projection, changes an export, or turns a library inspection into an event.
func (s *Store) Unit(ctx context.Context, id, mode string, at time.Time) (KnowledgeUnitDetail, error) {
	if err := validText("unit ID", id, 200, true); err != nil {
		return KnowledgeUnitDetail{}, err
	}
	if mode != "choice" && mode != "recall" {
		return KnowledgeUnitDetail{}, fmt.Errorf("%w: unit context must be choice or recall", ErrInvalid)
	}
	now := s.now()
	if at.IsZero() {
		at = time.UnixMilli(now)
	}
	if at.Before(time.UnixMilli(now)) {
		return KnowledgeUnitDetail{}, fmt.Errorf("%w: estimate target time cannot precede its as-of time", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return KnowledgeUnitDetail{}, err
	}
	defer tx.Rollback()
	var result KnowledgeUnitDetail
	result.Unit, err = knowledgeUnit(ctx, tx, id)
	if err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT version FROM unit_versions WHERE unit_id=? ORDER BY version", id)
	if err != nil {
		return result, err
	}
	var versions []int
	for rows.Next() {
		var version int
		if err = rows.Scan(&version); err != nil {
			rows.Close()
			return result, err
		}
		versions = append(versions, version)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, version := range versions {
		unit, err := knowledgeUnitVersion(ctx, tx, id, version)
		if err != nil {
			return result, err
		}
		result.Versions = append(result.Versions, unit)
	}
	relations, inferenceRelations, err := knowledgeRelations(ctx, tx, now)
	if err != nil {
		return result, err
	}
	for _, relation := range relations {
		if relation.FromID == id || relation.ToID == id {
			result.Relations = append(result.Relations, relation)
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT DISTINCT material_id,material_version FROM material_links WHERE unit_id=? ORDER BY material_id,material_version`, id)
	if err != nil {
		return result, err
	}
	type materialKey struct {
		id      string
		version int
	}
	var materials []materialKey
	for rows.Next() {
		var key materialKey
		if err = rows.Scan(&key.id, &key.version); err != nil {
			rows.Close()
			return result, err
		}
		materials = append(materials, key)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, key := range materials {
		item, err := materialMetadataVersion(ctx, tx, key.id, key.version)
		if err != nil {
			return result, err
		}
		result.Materials = append(result.Materials, item)
	}
	evidence, err := knowledgeEvidence(ctx, tx, now)
	if err != nil {
		return result, err
	}
	result.Estimate = learning.Infer(learning.UnitVersion{ID: id, Version: result.Unit.Version}, mode, time.UnixMilli(now), at, evidence, inferenceRelations)
	corrections, err := knowledgeCorrections(ctx, tx, now)
	if err != nil {
		return result, err
	}
	relevant := make(map[string]bool)
	for _, correctionID := range result.Estimate.CorrectionIDs {
		relevant[correctionID] = true
	}
	for _, correction := range corrections {
		if (correction.Kind == "unit" && correction.EntityID == id) || relevant[correction.ID] {
			result.Corrections = append(result.Corrections, correction)
		}
	}
	if err = tx.Commit(); err != nil {
		return KnowledgeUnitDetail{}, err
	}
	return result, nil
}

// knowledgeEvidence reads original interaction identities and pinned coverage.
// Review cards come only from that identity's immutable review event; an old
// unmapped quiz never borrows links from a subsequently edited material version.
func knowledgeEvidence(ctx context.Context, tx *sql.Tx, asOf int64) ([]learning.Evidence, error) {
	return loadKnowledgeEvidence(ctx, tx, asOf, "")
}

func loadKnowledgeEvidence(ctx context.Context, tx *sql.Tx, asOf int64, scope string) ([]learning.Evidence, error) {
	return selectedKnowledgeEvidence(ctx, tx, asOf, scope, "")
}

func selectedKnowledgeEvidence(ctx context.Context, tx *sql.Tx, asOf int64, scope, onlyIDs string) ([]learning.Evidence, error) {
	query := `SELECT i.id,i.material_id,i.material_version,i.kind,i.outcome,i.assisted,i.at,
	 json_object('id',json_extract(i.snapshot,'$.id'),'version',json_extract(i.snapshot,'$.version')),i.reason,
	 e.rating,e.schedule_after,e.due_at,e.algorithm,
	 CASE WHEN e.id IS NOT NULL THEN json_object('id',json_extract(e.snapshot,'$.id'),'version',json_extract(e.snapshot,'$.version'),'kind',json_extract(e.snapshot,'$.kind')) ELSE NULL END
	 FROM interactions i LEFT JOIN review_events e ON e.id=i.id WHERE i.at<=? AND i.kind<>'return'`
	args := []any{asOf}
	if scope != "" {
		query += ` AND (i.material_id,i.material_version) IN (SELECT l.material_id,l.material_version FROM material_links l
		 JOIN json_each(?) u ON l.unit_id=json_extract(u.value,'$.id') AND l.unit_version=json_extract(u.value,'$.version'))`
		args = append(args, scope)
	}
	if onlyIDs != "" {
		query += " AND i.id IN(SELECT value FROM json_each(?))"
		args = append(args, onlyIDs)
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY i.at,i.id", args...)
	if err != nil {
		return nil, err
	}
	type pinnedEvidence struct {
		e            learning.Evidence
		snapshot     string
		quizSnapshot sql.NullString
	}
	var pinned []pinnedEvidence
	for rows.Next() {
		var row pinnedEvidence
		var timestamp int64
		var reason string
		var rating, due sql.NullInt64
		var card, algorithm sql.NullString
		if err = rows.Scan(&row.e.ID, &row.e.MaterialID, &row.e.MaterialVersion, &row.e.Kind, &row.e.Outcome, &row.e.Assisted, &timestamp, &row.snapshot, &reason, &rating, &card, &due, &algorithm, &row.quizSnapshot); err != nil {
			rows.Close()
			return nil, err
		}
		row.e.At = time.UnixMilli(timestamp).UTC()
		if reason != "" {
			row.e.AssistanceReasons = []string{reason}
		}
		if row.e.Kind == "review" {
			if !rating.Valid || !card.Valid || !due.Valid || !algorithm.Valid || !row.quizSnapshot.Valid {
				rows.Close()
				return nil, fmt.Errorf("review interaction %s has no immutable review event", row.e.ID)
			}
			row.e.Rating, row.e.Algorithm = int(rating.Int64), algorithm.String
			row.e.DueAt = time.UnixMilli(due.Int64).UTC()
			var state learning.Card
			if err = json.Unmarshal([]byte(card.String), &state); err != nil {
				rows.Close()
				return nil, fmt.Errorf("decode observed card %s: %w", row.e.ID, err)
			}
			row.e.ScheduleAfter = &state
		}
		pinned = append(pinned, row)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	type versionKey struct {
		id      string
		version int
	}
	cache := make(map[versionKey]Material)
	result := make([]learning.Evidence, 0, len(pinned))
	for _, row := range pinned {
		var snapshot Material
		if err = json.Unmarshal([]byte(row.snapshot), &snapshot); err != nil {
			return nil, fmt.Errorf("decode observed material %s: %w", row.e.ID, err)
		}
		if snapshot.ID != "" && (snapshot.ID != row.e.MaterialID || snapshot.Version != row.e.MaterialVersion) {
			return nil, fmt.Errorf("observed material identity mismatch for interaction %s", row.e.ID)
		}
		key := versionKey{id: row.e.MaterialID, version: row.e.MaterialVersion}
		var ok bool
		if snapshot, ok = cache[key]; !ok {
			snapshot, err = evidenceMaterialVersion(ctx, tx, key.id, key.version)
			if err != nil {
				return nil, err
			}
			cache[key] = snapshot
		}
		row.e.Unmapped = snapshot.Unmapped || len(snapshot.Links) == 0
		if snapshot.Quiz != nil {
			row.e.Mode = snapshot.Quiz.Kind
		}
		if row.quizSnapshot.Valid {
			var quiz Quiz
			if err = json.Unmarshal([]byte(row.quizSnapshot.String), &quiz); err != nil {
				return nil, fmt.Errorf("decode immutable assessment %s: %w", row.e.ID, err)
			}
			row.e.Mode = quiz.Kind
			if snapshot.Quiz == nil || quiz.ID != snapshot.Quiz.ID || quiz.Version != snapshot.Quiz.Version {
				return nil, fmt.Errorf("observed assessment identity mismatch for interaction %s", row.e.ID)
			}
		}
		for _, link := range snapshot.Links {
			if link.MaterialID != row.e.MaterialID || link.MaterialVersion != row.e.MaterialVersion || link.UnitID == "" || link.UnitVersion < 1 {
				return nil, fmt.Errorf("observed coverage identity mismatch for interaction %s", row.e.ID)
			}
			row.e.Targets = append(row.e.Targets, learning.EvidenceTarget{UnitID: link.UnitID, UnitVersion: link.UnitVersion, Role: link.Role, CoverageID: link.ID, CoverageVersion: link.MaterialVersion})
		}
		if row.e.Kind != "review" && row.e.Kind != "continue" {
			row.e.Assisted = true
		}
		result = append(result, row.e)
	}
	// An inspection without an active presentation is still one real exposure,
	// tied only to the exact unit versions captured by that action.
	query = "SELECT id,kind,entity_id,unit_versions_json,at FROM inspection_events WHERE at<=? AND kind NOT IN('delivery','return')"
	args = []any{asOf}
	if scope != "" {
		query += ` AND EXISTS(SELECT 1 FROM json_each(inspection_events.unit_versions_json) observed JOIN json_each(?) requested
		 ON json_extract(observed.value,'$.id')=json_extract(requested.value,'$.id') AND json_extract(observed.value,'$.version')=json_extract(requested.value,'$.version'))`
		args = append(args, scope)
	}
	if onlyIDs != "" {
		query += " AND id IN(SELECT value FROM json_each(?))"
		args = append(args, onlyIDs)
	}
	rows, err = tx.QueryContext(ctx, query+" ORDER BY at,id", args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var event learning.Evidence
		var kind, entity, targets string
		var timestamp int64
		if err = rows.Scan(&event.ID, &kind, &entity, &targets, &timestamp); err != nil {
			rows.Close()
			return nil, err
		}
		var units []learning.UnitVersion
		if err = json.Unmarshal([]byte(targets), &units); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode inspection coverage: %w", err)
		}
		event.Kind, event.Outcome, event.Assisted = "inspection", "inspected", true
		event.At = time.UnixMilli(timestamp).UTC()
		event.AssistanceReasons = []string{"Explicit " + kind + " inspection: " + entity}
		for _, unit := range units {
			event.Targets = append(event.Targets, learning.EvidenceTarget{UnitID: unit.ID, UnitVersion: unit.Version, Role: "teaches"})
		}
		event.Unmapped = len(units) == 0
		result = append(result, event)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	byID := make(map[string]int, len(result))
	var eventIDs, entityIDs []string
	for i := range result {
		byID[result[i].ID] = i
		eventIDs = append(eventIDs, result[i].ID)
		entityIDs = append(entityIDs, result[i].ID, result[i].MaterialID)
		for _, target := range result[i].Targets {
			entityIDs = append(entityIDs, target.UnitID)
		}
	}
	encodedEvents, err := marshal(eventIDs)
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,review_id,note,created_at FROM corrections WHERE created_at<=? AND review_id IN(SELECT value FROM json_each(?)) ORDER BY created_at,id", asOf, encodedEvents)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, reviewID, reason string
		var timestamp int64
		if err = rows.Scan(&id, &reviewID, &reason, &timestamp); err != nil {
			rows.Close()
			return nil, err
		}
		if index, ok := byID[reviewID]; ok {
			e := &result[index]
			e.Disputed = true
			e.CorrectionIDs = append(e.CorrectionIDs, id)
			e.Corrections = append(e.Corrections, learning.Correction{ID: id, Kind: "dispute", Reason: reason, At: time.UnixMilli(timestamp).UTC()})
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	encodedEntities, err := marshal(entityIDs)
	if err != nil {
		return nil, err
	}
	corrections, err := loadKnowledgeCorrections(ctx, tx, asOf, encodedEntities)
	if err != nil {
		return nil, err
	}
	for i := range result {
		e := &result[i]
		for _, correction := range corrections {
			matches := (correction.Kind == "material" || correction.Kind == "coverage") && correction.EntityID == e.MaterialID && correction.BeforeVersion == e.MaterialVersion
			matches = matches || correction.Kind == "evidence" && correction.EntityID == e.ID
			if correction.Kind == "unit" {
				for _, target := range e.Targets {
					matches = matches || correction.EntityID == target.UnitID && correction.BeforeVersion == target.UnitVersion
				}
			}
			if matches {
				e.Ambiguous = true
				e.CorrectionIDs = append(e.CorrectionIDs, correction.ID)
				e.Corrections = append(e.Corrections, learning.Correction{ID: correction.ID, Kind: correction.Kind, Reason: correction.Reason, At: time.UnixMilli(correction.CreatedAt).UTC()})
			}
		}
	}
	return result, nil
}

func knowledgeCorrections(ctx context.Context, tx *sql.Tx, asOf int64) ([]KnowledgeCorrection, error) {
	return loadKnowledgeCorrections(ctx, tx, asOf, "")
}

func loadKnowledgeCorrections(ctx context.Context, tx *sql.Tx, asOf int64, entities string) ([]KnowledgeCorrection, error) {
	query := "SELECT id,kind,entity_id,before_version,after_version,reason,created_at FROM knowledge_corrections WHERE created_at<=?"
	args := []any{asOf}
	if entities != "" {
		query += " AND entity_id IN(SELECT value FROM json_each(?))"
		args = append(args, entities)
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY created_at,id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []KnowledgeCorrection
	for rows.Next() {
		var correction KnowledgeCorrection
		if err = rows.Scan(&correction.ID, &correction.Kind, &correction.EntityID, &correction.BeforeVersion, &correction.AfterVersion, &correction.Reason, &correction.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, correction)
	}
	return result, rows.Err()
}

// Relations are selected by immutable version chronology at the derivation's
// as-of time, never by rewriting the relation versions pinned in old estimates.
func knowledgeRelations(ctx context.Context, tx *sql.Tx, asOf int64) ([]KnowledgeRelation, []learning.Relation, error) {
	return loadKnowledgeRelations(ctx, tx, asOf, "")
}

func loadKnowledgeRelations(ctx context.Context, tx *sql.Tx, asOf int64, scope string) ([]KnowledgeRelation, []learning.Relation, error) {
	query := `SELECT rv.relation_id,rv.version,rv.from_id,rv.from_version,rv.to_id,rv.to_version,rv.kind,rv.proposed,rv.provenance_json,kr.archived
	 FROM relation_versions rv JOIN knowledge_relations kr ON kr.id=rv.relation_id
	 WHERE rv.created_at<=? AND NOT EXISTS(SELECT 1 FROM relation_versions newer WHERE newer.relation_id=rv.relation_id AND newer.created_at<=? AND newer.version>rv.version)`
	args := []any{asOf, asOf}
	if scope != "" {
		query += ` AND (rv.from_id,rv.from_version) IN(SELECT json_extract(value,'$.id'),json_extract(value,'$.version') FROM json_each(?))`
		args = append(args, scope)
	}
	rows, err := tx.QueryContext(ctx, query+" ORDER BY rv.relation_id", args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var models []KnowledgeRelation
	var inference []learning.Relation
	for rows.Next() {
		var relation KnowledgeRelation
		var provenance string
		if err = rows.Scan(&relation.ID, &relation.Version, &relation.FromID, &relation.FromVersion, &relation.ToID, &relation.ToVersion, &relation.Kind, &relation.Proposed, &provenance, &relation.Archived); err != nil {
			return nil, nil, err
		}
		if err = json.Unmarshal([]byte(provenance), &relation.Provenance); err != nil {
			return nil, nil, fmt.Errorf("decode relation provenance: %w", err)
		}
		models = append(models, relation)
		if !relation.Archived {
			inference = append(inference, learning.Relation{ID: relation.ID, Version: relation.Version, From: learning.UnitVersion{ID: relation.FromID, Version: relation.FromVersion}, To: learning.UnitVersion{ID: relation.ToID, Version: relation.ToVersion}, Kind: relation.Kind, Proposed: relation.Proposed})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()
	var relationIDs []string
	for _, relation := range models {
		relationIDs = append(relationIDs, relation.ID)
	}
	encodedRelations, err := marshal(relationIDs)
	if err != nil {
		return nil, nil, err
	}
	corrections, err := loadKnowledgeCorrections(ctx, tx, asOf, encodedRelations)
	if err != nil {
		return nil, nil, err
	}
	for i := range inference {
		for _, correction := range corrections {
			if correction.Kind == "relation" && correction.EntityID == inference[i].ID && correction.AfterVersion <= inference[i].Version {
				inference[i].CorrectionIDs = append(inference[i].CorrectionIDs, correction.ID)
			}
		}
		sort.Strings(inference[i].CorrectionIDs)
	}
	return models, inference, nil
}

// recordKnowledgeEstimates is called only by acknowledged state-changing
// transactions. It retains superseded derivations for inspection and recovery;
// read paths never invoke it and observations/cards remain untouched.
func recordKnowledgeEstimates(ctx context.Context, tx *sql.Tx, now int64) error {
	// Only exact units touched by this transaction seed the projection update.
	// UNION deduplicates paths and observations; no timestamp becomes evidence.
	rows, err := tx.QueryContext(ctx, `WITH changed(id,version) AS (
	 SELECT l.unit_id,l.unit_version FROM interactions i JOIN material_links l ON l.material_id=i.material_id AND l.material_version=i.material_version WHERE i.at=? AND i.kind<>'return'
	 UNION SELECT json_extract(u.value,'$.id'),json_extract(u.value,'$.version') FROM inspection_events i,json_each(i.unit_versions_json) u WHERE i.at=? AND i.kind NOT IN('delivery','return')
	 UNION SELECT unit_id,version FROM unit_versions WHERE created_at=?
	 UNION SELECT l.unit_id,l.unit_version FROM knowledge_corrections c JOIN material_links l ON l.material_id=c.entity_id AND l.material_version IN(c.before_version,c.after_version) WHERE c.created_at=? AND c.kind IN('material','coverage')
	 UNION SELECT l.unit_id,l.unit_version FROM corrections c JOIN interactions i ON i.id=c.review_id JOIN material_links l ON l.material_id=i.material_id AND l.material_version=i.material_version WHERE c.created_at=?
	 UNION SELECT rv.from_id,rv.from_version FROM relation_versions rv WHERE rv.created_at=?
	 UNION SELECT rv.to_id,rv.to_version FROM relation_versions rv WHERE rv.created_at=?
	 UNION SELECT l.unit_id,l.unit_version FROM knowledge_corrections c JOIN interactions i ON i.id=c.entity_id JOIN material_links l ON l.material_id=i.material_id AND l.material_version=i.material_version WHERE c.created_at=? AND c.kind='evidence'
	 UNION SELECT uv.unit_id,uv.version FROM knowledge_corrections c JOIN unit_versions uv ON uv.unit_id=c.entity_id AND uv.version IN(c.before_version,c.after_version) WHERE c.created_at=? AND c.kind='unit'
	 UNION SELECT rv.from_id,rv.from_version FROM knowledge_corrections c JOIN relation_versions rv ON rv.relation_id=c.entity_id AND rv.version IN(c.before_version,c.after_version) WHERE c.created_at=? AND c.kind='relation'
	 UNION SELECT rv.to_id,rv.to_version FROM knowledge_corrections c JOIN relation_versions rv ON rv.relation_id=c.entity_id AND rv.version IN(c.before_version,c.after_version) WHERE c.created_at=? AND c.kind='relation'
	 ) SELECT id,version FROM changed ORDER BY id,version`, now, now, now, now, now, now, now, now, now, now, now)
	if err != nil {
		return err
	}
	var units []learning.UnitVersion
	for rows.Next() {
		var unit learning.UnitVersion
		if err = rows.Scan(&unit.ID, &unit.Version); err != nil {
			rows.Close()
			return err
		}
		units = append(units, unit)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(units) == 0 {
		return nil
	}
	affected, err := knowledgeReachable(ctx, tx, units, now, true)
	if err != nil {
		return err
	}
	relevant, err := knowledgeReachable(ctx, tx, affected, now, false)
	if err != nil {
		return err
	}
	scope, err := marshal(relevant)
	if err != nil {
		return err
	}
	evidence, err := loadKnowledgeEvidence(ctx, tx, now, scope)
	if err != nil {
		return err
	}
	_, relations, err := loadKnowledgeRelations(ctx, tx, now, scope)
	if err != nil {
		return err
	}
	at := time.UnixMilli(now).UTC()
	for _, unit := range affected {
		for _, mode := range []string{"choice", "recall"} {
			estimate := learning.Infer(unit, mode, at, at, evidence, relations)
			encoded, err := marshal(estimate)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO estimate_records(id,unit_id,unit_version,mode,as_of,target_at,policy,estimate_json,created_at)
			 SELECT ?,?,?,?,?,?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM estimate_records WHERE unit_id=? AND unit_version=? AND mode=? AND as_of=? AND estimate_json=?)`,
				newID(), unit.ID, unit.Version, mode, now, now, estimate.Policy, encoded, now, unit.ID, unit.Version, mode, now, encoded); err != nil {
				return err
			}
		}
	}
	return nil
}

// knowledgeReachable traverses only the affected exact-version component. The
// reverse direction finds estimates influenced by a compound observation; the
// forward direction finds the evidence those estimates can legitimately use.
func knowledgeReachable(ctx context.Context, tx *sql.Tx, units []learning.UnitVersion, asOf int64, reverse bool) ([]learning.UnitVersion, error) {
	encoded, err := marshal(units)
	if err != nil {
		return nil, err
	}
	from, to := "from", "to"
	if reverse {
		from, to = to, from
	}
	query := `WITH RECURSIVE reachable(id,version) AS (
	 SELECT json_extract(value,'$.id'),json_extract(value,'$.version') FROM json_each(?)
	 UNION SELECT rv.` + to + `_id,rv.` + to + `_version FROM reachable r JOIN relation_versions rv ON rv.` + from + `_id=r.id AND rv.` + from + `_version=r.version
	 JOIN knowledge_relations kr ON kr.id=rv.relation_id
	 WHERE kr.archived=0 AND rv.proposed=0 AND rv.kind IN('prerequisite','composition') AND rv.created_at<=?
	 AND NOT EXISTS(SELECT 1 FROM relation_versions newer WHERE newer.relation_id=rv.relation_id AND newer.created_at<=? AND newer.version>rv.version)
	 ) SELECT id,version FROM reachable ORDER BY id,version`
	rows, err := tx.QueryContext(ctx, query, encoded, asOf, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []learning.UnitVersion
	for rows.Next() {
		var unit learning.UnitVersion
		if err = rows.Scan(&unit.ID, &unit.Version); err != nil {
			return nil, err
		}
		result = append(result, unit)
	}
	return result, rows.Err()
}

// Evidence needs identity, actual task context and pinned coverage, not an
// explanation body, article text, or a fully hydrated current library object.
func evidenceMaterialVersion(ctx context.Context, tx *sql.Tx, id string, version int) (Material, error) {
	var m Material
	var quizID, quizMode sql.NullString
	var quizVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT m.id,m.source_id,v.version,m.kind,v.quiz_id,v.quiz_version,json_extract(qv.content,'$.kind')
	 FROM material_versions v JOIN materials m ON m.id=v.material_id LEFT JOIN quiz_versions qv ON qv.quiz_id=v.quiz_id AND qv.version=v.quiz_version
	 WHERE v.material_id=? AND v.version=?`, id, version).Scan(&m.ID, &m.SourceID, &m.Version, &m.Kind, &quizID, &quizVersion, &quizMode)
	if err != nil {
		return m, notFound(err, "observed material version")
	}
	if quizID.Valid {
		m.Quiz = &Quiz{ID: quizID.String, Version: int(quizVersion.Int64), Kind: quizMode.String}
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,material_id,material_version,unit_id,unit_version,role FROM material_links WHERE material_id=? AND material_version=? ORDER BY id", id, version)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var link CoverageLink
		if err = rows.Scan(&link.ID, &link.MaterialID, &link.MaterialVersion, &link.UnitID, &link.UnitVersion, &link.Role); err != nil {
			return m, err
		}
		m.Links = append(m.Links, link)
	}
	m.Unmapped = len(m.Links) == 0
	return m, rows.Err()
}

func knowledgeContextEvidence(ctx context.Context, tx *sql.Tx, units []learning.UnitVersion, now int64, limit int) ([]learning.Evidence, int, error) {
	if limit < 1 || limit > 1000 {
		return nil, 0, fmt.Errorf("%w: context evidence bound must be 1..1000", ErrInvalid)
	}
	if len(units) == 0 {
		return []learning.Evidence{}, 0, nil
	}
	scope, err := marshal(units)
	if err != nil {
		return nil, 0, err
	}
	query := `WITH relevant AS (
	 SELECT i.id,i.at FROM interactions i WHERE i.at<=? AND i.kind<>'return'
	 AND (i.material_id,i.material_version) IN(SELECT l.material_id,l.material_version FROM material_links l JOIN json_each(?) u
	   ON l.unit_id=json_extract(u.value,'$.id') AND l.unit_version=json_extract(u.value,'$.version'))
	 UNION SELECT i.id,i.at FROM inspection_events i WHERE i.at<=? AND i.kind NOT IN('delivery','return')
	 AND EXISTS(SELECT 1 FROM json_each(i.unit_versions_json) observed JOIN json_each(?) requested
	   ON json_extract(observed.value,'$.id')=json_extract(requested.value,'$.id') AND json_extract(observed.value,'$.version')=json_extract(requested.value,'$.version'))
	 ) `
	var total int
	if err = tx.QueryRowContext(ctx, query+"SELECT count(*) FROM relevant", now, scope, now, scope).Scan(&total); err != nil {
		return nil, 0, err
	}
	ids, err := rowIDs(ctx, tx, query+"SELECT id FROM relevant ORDER BY at DESC,id DESC LIMIT ?", now, scope, now, scope, limit)
	if err != nil {
		return nil, 0, err
	}
	encodedIDs, err := marshal(ids)
	if err != nil {
		return nil, 0, err
	}
	result, err := selectedKnowledgeEvidence(ctx, tx, now, scope, encodedIDs)
	return result, total - len(ids), err
}
