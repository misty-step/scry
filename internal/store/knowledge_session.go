package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

const inspectionPolicy = "inspection-v1;fixed-exposure=30m;source-and-exact-unit-coverage;anti-contamination-not-retention"
const exposureWindow = 30 * time.Minute

// A scope pins what was actually made available, not everything a goal or source
// might contain after later generation. Delivery grants are not reading events.
type inspectionScope struct {
	Kind           string
	EventID        string
	Sources        []string
	SourceVersions []learning.UnitVersion
	Units          []learning.UnitVersion
	ExpiresAt      int64
	Materials      []learning.UnitVersion
}

func inspectionScopeFor(ctx context.Context, tx *sql.Tx, kind, id string) (inspectionScope, error) {
	c := inspectionScope{Kind: kind, Sources: []string{}, SourceVersions: []learning.UnitVersion{}, Units: []learning.UnitVersion{}, Materials: []learning.UnitVersion{}}
	seenSources, seenUnits, seenMaterials := map[string]bool{}, map[string]bool{}, map[string]bool{}
	addSource := func(sid string) error {
		if seenSources[sid] {
			return nil
		}
		var rev int
		if err := tx.QueryRowContext(ctx, "SELECT revision FROM sources WHERE id=?", sid).Scan(&rev); err != nil {
			return notFound(err, "inspection source")
		}
		seenSources[sid] = true
		c.Sources = append(c.Sources, sid)
		c.SourceVersions = append(c.SourceVersions, learning.UnitVersion{ID: sid, Version: rev})
		return nil
	}
	addUnit := func(uid string, version int) {
		key := fmt.Sprintf("%s/%d", uid, version)
		if !seenUnits[key] {
			seenUnits[key] = true
			c.Units = append(c.Units, learning.UnitVersion{ID: uid, Version: version})
		}
	}
	addMaterial := func(m Material) error {
		key := fmt.Sprintf("%s/%d", m.ID, m.Version)
		if seenMaterials[key] {
			return nil
		}
		seenMaterials[key] = true
		c.Materials = append(c.Materials, learning.UnitVersion{ID: m.ID, Version: m.Version})
		if err := addSource(m.SourceID); err != nil {
			return err
		}
		for _, l := range m.Links {
			addUnit(l.UnitID, l.UnitVersion)
		}
		return nil
	}
	var ids []string
	var err error
	switch kind {
	case "presentation":
		p, e := presentation(ctx, tx, id)
		if e != nil {
			return c, e
		}
		if p.Kind != "reference" && !p.Graded {
			return c, fmt.Errorf("%w: cold question is not an answer-bearing presentation", ErrInvalid)
		}
		return presentationScope(p)
	case "source":
		if err = addSource(id); err != nil {
			return c, err
		}
		ids, err = rowIDs(ctx, tx, "SELECT id FROM materials WHERE source_id=? ORDER BY id", id)
	case "quiz", "material":
		m, e := materialMetadata(ctx, tx, id)
		if e != nil {
			return c, e
		}
		if kind == "quiz" && m.Kind != "quiz" {
			return c, fmt.Errorf("%w: not quiz material", ErrInvalid)
		}
		e = addMaterial(m)
		return c, e
	case "unit":
		u, e := knowledgeUnit(ctx, tx, id)
		if e != nil {
			return c, e
		}
		addUnit(u.ID, u.Version)
		ids, err = rowIDs(ctx, tx, "SELECT DISTINCT material_id FROM material_links WHERE unit_id=? ORDER BY material_id", id)
	case "goal":
		var sid string
		if err = tx.QueryRowContext(ctx, "SELECT source_id FROM goals WHERE id=?", id).Scan(&sid); err != nil {
			return c, notFound(err, "inspection goal")
		}
		if err = addSource(sid); err != nil {
			return c, err
		}
		uids, e := rowIDs(ctx, tx, "SELECT DISTINCT unit_id FROM goal_units WHERE goal_id=? ORDER BY unit_id", id)
		if e != nil {
			return c, e
		}
		for _, uid := range uids {
			u, e := knowledgeUnit(ctx, tx, uid)
			if e != nil {
				return c, e
			}
			addUnit(u.ID, u.Version)
		}
		ids, err = rowIDs(ctx, tx, "SELECT DISTINCT material_id FROM goal_materials WHERE goal_id=? ORDER BY material_id", id)
	case "library":
		if err = validText("library query", id, 1024, false); err != nil {
			return c, err
		}
		pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(id)) + "%"
		goalIDs, e := rowIDs(ctx, tx, `SELECT id FROM goals WHERE title LIKE ? ESCAPE '\' ORDER BY created_at DESC,id`, pattern)
		if e != nil {
			return c, e
		}
		for _, goalID := range goalIDs {
			var sourceID string
			if e = tx.QueryRowContext(ctx, "SELECT source_id FROM goals WHERE id=?", goalID).Scan(&sourceID); e != nil {
				return c, e
			}
			if e = addSource(sourceID); e != nil {
				return c, e
			}
			materialIDs, e := rowIDs(ctx, tx, "SELECT DISTINCT material_id FROM goal_materials WHERE goal_id=?", goalID)
			if e != nil {
				return c, e
			}
			ids = append(ids, materialIDs...)
		}
	case "history":
		return historyDisclosureScope(ctx, tx, "", 0)
	case "export":
		ids, err = rowIDs(ctx, tx, "SELECT id FROM materials ORDER BY id")
		sids, e := rowIDs(ctx, tx, "SELECT id FROM sources ORDER BY id")
		if e != nil {
			return c, e
		}
		for _, sid := range sids {
			if e = addSource(sid); e != nil {
				return c, e
			}
		}
	default:
		return c, fmt.Errorf("%w: unsupported inspection kind", ErrInvalid)
	}
	if err != nil {
		return c, err
	}
	for _, mid := range ids {
		m, e := materialMetadata(ctx, tx, mid)
		if e != nil {
			return c, e
		}
		if e = addMaterial(m); e != nil {
			return c, e
		}
	}
	// Full export/unit versions and actual graded feedback use exact pins,
	// rather than reconstructing old coverage from the current material.
	versionQuery := ""
	var args []any
	switch kind {
	case "export":
		versionQuery = "SELECT material_id,version FROM material_versions ORDER BY material_id,version"
	case "unit":
		versionQuery = "SELECT DISTINCT material_id,material_version FROM material_links WHERE unit_id=? ORDER BY material_id,material_version"
		args = []any{id}
	}
	if versionQuery != "" {
		rows, e := tx.QueryContext(ctx, versionQuery, args...)
		if e != nil {
			return c, e
		}
		versions := []learning.UnitVersion{}
		for rows.Next() {
			var v learning.UnitVersion
			if e = rows.Scan(&v.ID, &v.Version); e != nil {
				rows.Close()
				return c, e
			}
			versions = append(versions, v)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return c, e
		}
		rows.Close()
		for _, pin := range versions {
			m, e := materialMetadataVersion(ctx, tx, pin.ID, pin.Version)
			if e != nil {
				return c, e
			}
			if e = addMaterial(m); e != nil {
				return c, e
			}
		}
	}
	if kind == "unit" || kind == "export" {
		query := "SELECT unit_id,version FROM unit_versions"
		var args []any
		if kind == "unit" {
			query += " WHERE unit_id=?"
			args = []any{id}
		}
		rows, e := tx.QueryContext(ctx, query, args...)
		if e != nil {
			return c, e
		}
		for rows.Next() {
			var uid string
			var version int
			if e = rows.Scan(&uid, &version); e != nil {
				rows.Close()
				return c, e
			}
			addUnit(uid, version)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return c, e
		}
		rows.Close()
	}
	return c, nil
}

func scopeMatchesMaterial(c inspectionScope, m Material) bool {
	for _, pin := range c.SourceVersions {
		fullSource := c.Kind == "source" || c.Kind == "export"
		if c.Kind != "history" && c.Kind != "library" && (fullSource || len(m.Links) == 0) && pin.ID == m.SourceID && pin.Version == m.Provenance.SourceRevision {
			return true
		}
	}
	for _, v := range c.Materials {
		if v.ID == m.ID && v.Version == m.Version {
			return true
		}
	}
	for _, u := range c.Units {
		for _, l := range m.Links {
			if u.ID == l.UnitID && u.Version == l.UnitVersion {
				return true
			}
		}
	}
	return false
}

func inspectionTargets(ctx context.Context, tx *sql.Tx, c inspectionScope) ([]Presentation, error) {
	ids, err := rowIDs(ctx, tx, `SELECT p.id FROM presentations p WHERE p.kind='quiz' AND p.graded=0 AND p.practice=0 AND (p.id=(SELECT current_id FROM review_session WHERE singleton=1) OR p.id IN(SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return'))) ORDER BY p.created_at,p.id`)
	if err != nil {
		return nil, err
	}
	result := []Presentation{}
	for _, id := range ids {
		p, err := presentation(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if p.Material != nil && scopeMatchesMaterial(c, *p.Material) {
			result = append(result, p)
		}
	}
	return result, nil
}

func saveInspectionScope(ctx context.Context, tx *sql.Tx, kind, id string, c inspectionScope, now int64) (string, error) {
	a, _ := marshal(c.Sources)
	b, _ := marshal(c.Units)
	sources, _ := marshal(c.SourceVersions)
	materials, _ := marshal(c.Materials)
	eventID := c.EventID
	if eventID == "" {
		eventID = newID()
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO inspection_events(id,kind,entity_id,source_ids_json,unit_versions_json,source_versions_json,material_versions_json,at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, eventID, kind, id, a, b, sources, materials, now, now+exposureWindow.Milliseconds())
	return eventID, err
}

func recentInspectionScopes(ctx context.Context, tx *sql.Tx, now int64) ([]inspectionScope, error) {
	rows, err := tx.QueryContext(ctx, "SELECT source_ids_json,unit_versions_json,source_versions_json,material_versions_json,expires_at,kind FROM inspection_events WHERE expires_at>? ORDER BY at,id", now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []inspectionScope{}
	for rows.Next() {
		var c inspectionScope
		var sources, units, sourceVersions, materials string
		if err = rows.Scan(&sources, &units, &sourceVersions, &materials, &c.ExpiresAt, &c.Kind); err != nil {
			return nil, err
		}
		for _, v := range []struct {
			text   string
			target any
		}{{sources, &c.Sources}, {units, &c.Units}, {sourceVersions, &c.SourceVersions}, {materials, &c.Materials}} {
			if err = json.Unmarshal([]byte(v.text), v.target); err != nil {
				return nil, err
			}
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func recentlyExposed(ctx context.Context, tx *sql.Tx, m Material, now int64) (bool, error) {
	at, err := latestMaterialExposure(ctx, tx, m, now)
	return at > 0, err
}

func latestMaterialExposure(ctx context.Context, tx *sql.Tx, m Material, now int64) (int64, error) {
	return materialExposure(ctx, tx, m, now, true)
}

func materialExposure(ctx context.Context, tx *sql.Tx, m Material, now int64, includeFeedback bool) (int64, error) {
	activeAt, err := activeMaterialExposure(ctx, tx, m, includeFeedback)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT i.source_ids_json,i.source_versions_json,i.unit_versions_json,i.material_versions_json,CASE WHEN i.kind='delivery' THEN CASE WHEN p.reviewed_at>0 THEN p.reviewed_at ELSE p.created_at END ELSE i.at END AS exposure_at,i.kind FROM inspection_events i LEFT JOIN presentations p ON i.kind='delivery' AND p.id=i.entity_id WHERE i.expires_at>? AND (? OR i.kind<>'delivery' OR p.kind='reference') ORDER BY exposure_at DESC,i.id DESC`, now, includeFeedback)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var c inspectionScope
		var sources, sourceVersions, units, materials string
		var at int64
		if err = rows.Scan(&sources, &sourceVersions, &units, &materials, &at, &c.Kind); err != nil {
			return 0, err
		}
		for _, v := range []struct {
			text   string
			target any
		}{{sources, &c.Sources}, {sourceVersions, &c.SourceVersions}, {units, &c.Units}, {materials, &c.Materials}} {
			if err = json.Unmarshal([]byte(v.text), v.target); err != nil {
				return 0, err
			}
		}
		if scopeMatchesMaterial(c, m) {
			return max(at, activeAt), nil
		}
	}
	return activeAt, rows.Err()
}

func (s *Store) InspectionAccess(ctx context.Context, kind, id string) (InspectionAccess, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return InspectionAccess{}, err
	}
	defer tx.Rollback()
	result := InspectionAccess{CheckedAt: s.now()}
	if kind == "presentation" {
		access, err := presentationAccess(ctx, tx, id, result.CheckedAt)
		if err != nil {
			return access, err
		}
		return access, tx.Commit()
	}
	wanted, err := inspectionScopeFor(ctx, tx, kind, id)
	if err != nil {
		return result, err
	}
	targets, err := inspectionTargets(ctx, tx, wanted)
	if err != nil {
		return result, err
	}
	result.RequiresAssistance = len(targets) > 0
	scopes, err := recentInspectionScopes(ctx, tx, result.CheckedAt)
	if err != nil {
		return result, err
	}
	expires := int64(0)
	contains := func(want []learning.UnitVersion, field func(inspectionScope) []learning.UnitVersion) bool {
		for _, pin := range want {
			pinExpiry := int64(0)
			for _, scope := range scopes {
				if scope.Kind == "delivery" || (scope.Kind == "library" && kind != "library") {
					continue
				}
				for _, available := range field(scope) {
					if pin == available {
						pinExpiry = max(pinExpiry, scope.ExpiresAt)
					}
				}
			}
			if pinExpiry == 0 {
				return false
			}
			if expires == 0 || pinExpiry < expires {
				expires = pinExpiry
			}
		}
		return true
	}
	result.Allowed = !result.RequiresAssistance &&
		contains(wanted.SourceVersions, func(c inspectionScope) []learning.UnitVersion { return c.SourceVersions }) &&
		contains(wanted.Units, func(c inspectionScope) []learning.UnitVersion { return c.Units }) &&
		contains(wanted.Materials, func(c inspectionScope) []learning.UnitVersion { return c.Materials })
	if len(wanted.SourceVersions)+len(wanted.Units)+len(wanted.Materials) == 0 {
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE(max(expires_at),0) FROM inspection_events WHERE kind=? AND entity_id=? AND expires_at>?", kind, id, result.CheckedAt).Scan(&expires); err != nil {
			return result, err
		}
		result.Allowed = expires > 0 && !result.RequiresAssistance
	}
	if result.Allowed {
		result.ExpiresAt = expires
	}
	return result, tx.Commit()
}

func (s *Store) AssistInspection(ctx context.Context, kind, id, operationID string) (ReviewState, error) {
	if err := validOperation(operationID); err != nil {
		return ReviewState{}, err
	}
	hash := payloadHash([]string{kind, id})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "assist-inspection", hash)
	if err != nil {
		return ReviewState{}, err
	}
	if found {
		return reviewReceipt(receipt)
	}
	now := s.now()
	c, err := inspectionScopeFor(ctx, tx, kind, id)
	if err != nil {
		return ReviewState{}, err
	}
	if kind == "history" {
		c, err = historyDisclosureScope(ctx, tx, newID(), now)
		if err != nil {
			return ReviewState{}, err
		}
	}
	targets, err := inspectionTargets(ctx, tx, c)
	if err != nil {
		return ReviewState{}, err
	}
	for _, p := range targets {
		if _, err = tx.ExecContext(ctx, "UPDATE presentations SET assisted=1,practice=1,planning_reason=? WHERE id=?", inspectionPolicy+"; explicit "+kind+" inspection; later answer is practice, not a scheduled review", p.ID); err != nil {
			return ReviewState{}, err
		}
	}
	if _, err = saveInspectionScope(ctx, tx, kind, id, c, now); err != nil {
		return ReviewState{}, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return ReviewState{}, err
	}
	state, err := sessionState(ctx, tx, now, false)
	if err != nil {
		return state, err
	}
	if err = saveOperation(ctx, tx, operationID, "assist-inspection", hash, id, state, now); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func (s *Store) MarkAssisted(ctx context.Context, presentationID, reason, operationID string) (Presentation, error) {
	if err := validOperation(operationID); err != nil {
		return Presentation{}, err
	}
	if err := validText("assistance reason", reason, 4096, true); err != nil {
		return Presentation{}, err
	}
	hash := payloadHash([]string{presentationID, reason})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "mark-assisted", hash)
	if err != nil {
		return Presentation{}, err
	}
	if found {
		var p Presentation
		err = json.Unmarshal([]byte(receipt), &p)
		return p, err
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return p, err
	}
	var eligible bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM presentations p WHERE p.id=? AND (p.id=(SELECT current_id FROM review_session WHERE singleton=1) OR p.id IN(SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return'))))`, p.ID).Scan(&eligible); err != nil {
		return p, err
	}
	if !eligible || p.Kind != "quiz" || p.Graded {
		return p, fmt.Errorf("%w: assistance target is no longer cold and active", ErrConflict)
	}
	now := s.now()
	p.Assisted = true
	p.PlanningReason = reason
	if _, err = tx.ExecContext(ctx, "UPDATE presentations SET assisted=1,planning_reason=? WHERE id=?", reason, p.ID); err != nil {
		return p, err
	}
	if err = recordInteraction(ctx, tx, newID(), p, "assistance", "assisted", reason, now); err != nil {
		return p, err
	}
	hideAnswer(&p)
	if err = saveOperation(ctx, tx, operationID, "mark-assisted", hash, p.ID, p, now); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

func recordInteraction(ctx context.Context, tx *sql.Tx, id string, p Presentation, kind, outcome, reason string, now int64) error {
	if p.Material == nil {
		return fmt.Errorf("%w: interaction lacks material version", ErrInvalid)
	}
	// Immutable material/version is authoritative. '{}' is not a content copy;
	// adapters resolve the exact version, never the latest material.
	_, err := tx.ExecContext(ctx, `INSERT INTO interactions(id,presentation_id,material_id,material_version,kind,outcome,assisted,at,snapshot,reason,answer,mode,practice) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, p.ID, p.Material.ID, p.Material.Version, kind, outcome, p.Assisted, now, "{}", reason, p.Answer, p.Quiz.Kind, p.Practice)
	if err != nil {
		return err
	}
	return recordKnowledgeEstimates(ctx, tx, now)
}

func establishPresentation(ctx context.Context, tx *sql.Tx, m Material, scheduleVersion int, bridgeID, reason string, now int64) (Presentation, error) {
	p := Presentation{ID: newID(), GoalID: m.GoalID, GoalRevision: m.GoalRevision, GoalPinOrigin: "selected-plan", Material: &m, Kind: "reference", DueAt: m.DueAt, BridgeID: bridgeID, PlanningReason: reason}
	var authorized bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM goals g JOIN sources s ON s.id=g.source_id JOIN goal_materials gm ON gm.goal_id=g.id
	 WHERE g.id=? AND g.revision=? AND g.archived=0 AND s.archived=0 AND gm.material_id=? AND gm.material_version=?)`, m.GoalID, m.GoalRevision, m.ID, m.Version).Scan(&authorized); err != nil {
		return p, err
	}
	if !authorized {
		return p, fmt.Errorf("%w: selected goal/material scope changed; no occurrence established", ErrConflict)
	}
	var quizID any
	contentVersion := 0
	snapshot := "{}"
	if m.Quiz != nil {
		p.Kind = "quiz"
		p.Quiz = *m.Quiz
		quizID = p.Quiz.ID
		contentVersion = p.Quiz.Version
		var err error
		snapshot, err = marshal(p.Quiz)
		if err != nil {
			return p, err
		}
		p.Practice, err = recentlyExposed(ctx, tx, m, now)
		if err != nil {
			return p, err
		}
		p.Assisted = p.Practice
		if p.Practice {
			p.PlanningReason = inspectionPolicy + "; answer-assisted practice, not a scheduled review"
		}
	}
	encoded, err := marshal(m)
	if err != nil {
		return p, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at,kind,material_id,material_version,material_snapshot,bridge_id,planning_reason,practice,assisted,goal_id,goal_revision,goal_pin_origin)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, quizID, contentVersion, scheduleVersion, snapshot, now, p.DueAt, p.Kind, m.ID, m.Version, encoded, bridgeID, p.PlanningReason, p.Practice, p.Assisted, p.GoalID, p.GoalRevision, p.GoalPinOrigin)
	if err != nil {
		return p, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE materials SET first_presented_at=? WHERE id=? AND first_presented_at=0", now, m.ID); err != nil {
		return p, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=? WHERE singleton=1", p.ID); err != nil {
		return p, err
	}
	if p.Kind == "reference" {
		scope, err := presentationScope(p)
		if err != nil {
			return p, err
		}
		targets, err := inspectionTargets(ctx, tx, scope)
		if err != nil {
			return p, err
		}
		for _, target := range targets {
			if _, err = tx.ExecContext(ctx, "UPDATE presentations SET assisted=1,practice=1,planning_reason=? WHERE id=?", inspectionPolicy+"; instructional material delivered", target.ID); err != nil {
				return p, err
			}
		}
		if _, err = saveInspectionScope(ctx, tx, "delivery", p.ID, scope, now); err != nil {
			return p, err
		}
	}
	return p, nil
}

func bridge(ctx context.Context, tx *sql.Tx, id string) (Bridge, error) {
	var b Bridge
	var path, versions string
	err := tx.QueryRowContext(ctx, `SELECT id,goal_id,target_presentation_id,target_material_id,target_material_version,status,reason,path_json,path_versions_json,position,job_id,created_at,updated_at FROM bridges WHERE id=?`, id).Scan(&b.ID, &b.GoalID, &b.TargetPresentationID, &b.TargetMaterialID, &b.TargetMaterialVersion, &b.Status, &b.Reason, &path, &versions, &b.Position, &b.JobID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return b, notFound(err, "bridge")
	}
	if err = json.Unmarshal([]byte(path), &b.Path); err != nil {
		return b, err
	}
	if err = json.Unmarshal([]byte(versions), &b.PathVersions); err != nil {
		return b, err
	}
	if b.JobID != "" {
		j, err := jobMetadata(ctx, tx, b.JobID)
		if err != nil {
			return b, err
		}
		b.Job = &j
		if b.Status == "pending" && j.Error != "" {
			b.Reason = j.Error
		}
	}
	return b, nil
}

func activeBridge(ctx context.Context, tx *sql.Tx) (*Bridge, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT id FROM bridges WHERE status IN('pending','active','ready_return')").Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := bridge(ctx, tx, id)
	return &b, err
}

func bridgePath(ctx context.Context, tx *sql.Tx, b Bridge) ([]string, []int, error) {
	ids, err := bridgeCandidates(ctx, tx, b)
	if err != nil {
		return nil, nil, err
	}
	seconds, newRemaining, err := bridgeAllowance(ctx, tx, b.GoalID, b.UpdatedAt)
	if err != nil {
		return nil, nil, err
	}
	path, versions := []string{}, []int{}
	instruction := false
	for _, id := range ids {
		m, err := materialMetadata(ctx, tx, id)
		if err != nil {
			return nil, nil, err
		}
		if m.Archived || m.EstimatedSeconds > seconds {
			continue
		}
		if m.Kind == "quiz" {
			if !instruction {
				continue
			}
			attempted, err := materialPreviouslyAttempted(ctx, tx, id)
			if err != nil {
				return nil, nil, err
			}
			if !attempted {
				if newRemaining == 0 {
					continue
				}
				newRemaining--
			}
		}
		path = append(path, m.ID)
		versions = append(versions, m.Version)
		seconds -= m.EstimatedSeconds
		instruction = instruction || m.Kind != "quiz"
		if len(path) == 8 {
			break
		}
	}
	if !instruction {
		return []string{}, []int{}, nil
	}
	return path, versions, nil
}

func populatePendingBridge(ctx context.Context, tx *sql.Tx, jobID string, now int64) error {
	ids, err := rowIDs(ctx, tx, "SELECT id FROM bridges WHERE status='pending' AND (?='' OR job_id=?)", jobID, jobID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		b, err := bridge(ctx, tx, id)
		if err != nil {
			return err
		}
		b.UpdatedAt = now
		b.Path, b.PathVersions, err = bridgePath(ctx, tx, b)
		if err != nil {
			return err
		}
		if len(b.Path) == 0 {
			if _, err = tx.ExecContext(ctx, "UPDATE bridges SET reason=?,updated_at=? WHERE id=?", "Saved library has no suitable instruction within the chosen pacing; return or inspect missing coverage", now, b.ID); err != nil {
				return err
			}
			continue
		}
		if err = attachBridgePath(ctx, tx, b, now); err != nil {
			return err
		}
		path, err := marshal(b.Path)
		if err != nil {
			return err
		}
		versions, err := marshal(b.PathVersions)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE bridges SET path_json=?,path_versions_json=?,status='active',updated_at=? WHERE id=?", path, versions, now, b.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Existing saved instruction satisfied this retained bridge without another paid request',updated_at=? WHERE id=? AND status='queued' AND attempts=0`, now, b.JobID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) StartBridge(ctx context.Context, presentationID, operationID string) (ReviewState, error) {
	if err := validOperation(operationID); err != nil {
		return ReviewState{}, err
	}
	hash := payloadHash(presentationID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "start-bridge", hash)
	if err != nil {
		return ReviewState{}, err
	}
	if found {
		return reviewReceipt(receipt)
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return ReviewState{}, err
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return ReviewState{}, err
	}
	if !current.Valid || current.String != p.ID {
		return ReviewState{}, fmt.Errorf("%w: bridge target is no longer current", ErrConflict)
	}
	if b, err := activeBridge(ctx, tx); err != nil {
		return ReviewState{}, err
	} else if b != nil {
		return ReviewState{}, fmt.Errorf("%w: return from the retained bridge before starting another", ErrConflict)
	}
	now := s.now()
	if !p.Graded {
		p.Assisted = true
		if _, err = tx.ExecContext(ctx, "UPDATE presentations SET assisted=1,planning_reason='Too advanced request; assistance retained' WHERE id=?", p.ID); err != nil {
			return ReviewState{}, err
		}
	}
	request := p
	request.Assisted = true
	if err = recordInteraction(ctx, tx, newID(), request, "bridge_request", "requested", "Learner requested foundation instruction and practice", now); err != nil {
		return ReviewState{}, err
	}
	g, err := retainedGoal(ctx, tx, p)
	if err != nil {
		return ReviewState{}, err
	}
	b := Bridge{ID: newID(), GoalID: g.ID, TargetPresentationID: p.ID, TargetMaterialID: p.Material.ID, TargetMaterialVersion: p.Material.Version, Status: "pending", Reason: "Foundation instruction requested; original target is retained", CreatedAt: now, UpdatedAt: now}
	b.Path, b.PathVersions, err = bridgePath(ctx, tx, b)
	if err != nil {
		return ReviewState{}, err
	}
	if len(b.Path) > 0 {
		b.Status = "active"
		if err = attachBridgePath(ctx, tx, b, now); err != nil {
			return ReviewState{}, err
		}
	} else {
		b.JobID, err = enqueueKnowledge(ctx, tx, g.SourceID, g.SourceRevision, "bridge", p.Material.ID, p.Material.Version, p.ID, p.Material.Version, now)
		if err != nil {
			return ReviewState{}, err
		}
	}
	a, _ := marshal(b.Path)
	versions, _ := marshal(b.PathVersions)
	if _, err = tx.ExecContext(ctx, `INSERT INTO bridges(id,goal_id,target_presentation_id,target_material_id,target_material_version,status,reason,path_json,path_versions_json,position,job_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,0,?,?,?)`, b.ID, b.GoalID, p.ID, b.TargetMaterialID, b.TargetMaterialVersion, b.Status, b.Reason, a, versions, b.JobID, now, now); err != nil {
		return ReviewState{}, err
	}
	if err = endPresentationDisclosure(ctx, tx, p, now); err != nil {
		return ReviewState{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id=?", p.ID); err != nil {
		return ReviewState{}, err
	}
	state, err := sessionState(ctx, tx, now, true)
	if err != nil {
		return state, err
	}
	if err = saveOperation(ctx, tx, operationID, "start-bridge", hash, b.ID, state, now); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func advanceBridge(ctx context.Context, tx *sql.Tx, p Presentation, now int64) error {
	if p.BridgeID == "" {
		return nil
	}
	b, err := bridge(ctx, tx, p.BridgeID)
	if err != nil {
		return err
	}
	if b.Status != "active" || b.Position >= len(b.Path) || b.Path[b.Position] != p.Material.ID || b.PathVersions[b.Position] != p.Material.Version {
		return fmt.Errorf("%w: bridge position changed", ErrConflict)
	}
	status := "active"
	if b.Position+1 >= len(b.Path) {
		status = "ready_return"
	}
	_, err = tx.ExecContext(ctx, "UPDATE bridges SET position=position+1,status=?,updated_at=? WHERE id=? AND position=?", status, now, b.ID, b.Position)
	return err
}

func (s *Store) ContinueMaterial(ctx context.Context, presentationID, operationID string) (ReviewState, error) {
	if err := validOperation(operationID); err != nil {
		return ReviewState{}, err
	}
	hash := payloadHash(presentationID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "continue-material", hash)
	if err != nil {
		return ReviewState{}, err
	}
	if found {
		return reviewReceipt(receipt)
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return ReviewState{}, err
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return ReviewState{}, err
	}
	if !current.Valid || current.String != p.ID || p.Kind != "reference" || p.Graded {
		return ReviewState{}, fmt.Errorf("%w: reference occurrence is no longer current", ErrConflict)
	}
	m, err := material(ctx, tx, p.Material.ID)
	if err != nil {
		return ReviewState{}, err
	}
	if m.Archived || m.Version != p.Material.Version {
		return ReviewState{}, fmt.Errorf("%w: reference changed or archived", ErrConflict)
	}
	now := s.now()
	if err = recordInteraction(ctx, tx, newID(), p, "continue", "continued", "Explicit reference Continue; exposure is not demonstrated understanding", now); err != nil {
		return ReviewState{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE presentations SET graded=1,outcome='continued',reviewed_at=? WHERE id=?", now, p.ID); err != nil {
		return ReviewState{}, err
	}
	if err = endPresentationDisclosure(ctx, tx, p, now); err != nil {
		return ReviewState{}, err
	}
	if err = advanceBridge(ctx, tx, p, now); err != nil {
		return ReviewState{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id=?", p.ID); err != nil {
		return ReviewState{}, err
	}
	state, err := sessionState(ctx, tx, now, true)
	if err != nil {
		return state, err
	}
	if err = saveOperation(ctx, tx, operationID, "continue-material", hash, p.ID, state, now); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func (s *Store) ReturnToTarget(ctx context.Context, bridgeID, operationID string) (ReviewState, error) {
	if err := validOperation(operationID); err != nil {
		return ReviewState{}, err
	}
	hash := payloadHash(bridgeID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "return-target", hash)
	if err != nil {
		return ReviewState{}, err
	}
	if found {
		return reviewReceipt(receipt)
	}
	b, err := bridge(ctx, tx, bridgeID)
	if err != nil {
		return ReviewState{}, err
	}
	if b.Status != "pending" && b.Status != "active" && b.Status != "ready_return" && b.Status != "unavailable" {
		return ReviewState{}, fmt.Errorf("%w: bridge already ended", ErrConflict)
	}
	p, err := presentation(ctx, tx, b.TargetPresentationID)
	if err != nil {
		return ReviewState{}, err
	}
	m, err := material(ctx, tx, b.TargetMaterialID)
	if err != nil {
		return ReviewState{}, err
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return ReviewState{}, err
	}
	if current.Valid && current.String != p.ID {
		held, e := presentation(ctx, tx, current.String)
		if e != nil {
			return ReviewState{}, e
		}
		if held.Graded {
			return ReviewState{}, fmt.Errorf("%w: deliberately advance held feedback before returning", ErrConflict)
		}
	}
	if current.Valid && current.String != p.ID {
		leaving, err := presentation(ctx, tx, current.String)
		if err != nil {
			return ReviewState{}, err
		}
		if err = endPresentationDisclosure(ctx, tx, leaving, s.now()); err != nil {
			return ReviewState{}, err
		}
	}
	now := s.now()
	status, reason := "returned", "Deliberate return to retained target"
	returnObservation := p
	returnObservation.Answer = ""
	if err = recordInteraction(ctx, tx, newID(), returnObservation, "return", "returned", "Deliberate return navigation; no assessment or scheduling change", now); err != nil {
		return ReviewState{}, err
	}
	contentChanged := m.Version != b.TargetMaterialVersion
	if m.Quiz != nil && p.Material.Quiz != nil && m.Quiz.Version == p.Material.Quiz.Version {
		contentChanged = false
	}
	g, err := goalHeader(ctx, tx, p.GoalID)
	if err != nil {
		return ReviewState{}, err
	}
	if (!p.Graded && (m.Archived || contentChanged)) || g.Archived {
		status, reason = "unavailable", "Original target or chosen goal was edited or archived; historical occurrence and feedback are retained"
		if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=NULL WHERE singleton=1"); err != nil {
			return ReviewState{}, err
		}
	} else {
		if !p.Graded {
			var taught bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM presentations x WHERE x.bridge_id=? AND x.kind='reference')`, b.ID).Scan(&taught); err != nil {
				return ReviewState{}, err
			}
			if taught {
				if _, err = tx.ExecContext(ctx, "UPDATE presentations SET practice=1,assisted=1,planning_reason=? WHERE id=?", inspectionPolicy+"; retained target after bridge instruction is practice", p.ID); err != nil {
					return ReviewState{}, err
				}
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=? WHERE singleton=1", p.ID); err != nil {
			return ReviewState{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE bridges SET status=?,reason=?,updated_at=? WHERE id=?", status, reason, now, b.ID); err != nil {
		return ReviewState{}, err
	}
	if b.JobID != "" {
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',error='Learner returned before pending bridge publication',updated_at=? WHERE id=? AND status IN('queued','retry')", now, b.JobID); err != nil {
			return ReviewState{}, err
		}
	}
	state, err := sessionState(ctx, tx, now, true)
	if err != nil {
		return state, err
	}
	state.Bridge = &b
	state.Bridge.Status = status
	state.Bridge.Reason = reason
	if err = saveOperation(ctx, tx, operationID, "return-target", hash, b.ID, state, now); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func retireMaterial(ctx context.Context, tx *sql.Tx, id, sourceID string, now int64) error {
	ids, err := rowIDs(ctx, tx, `SELECT p.id FROM presentations p JOIN materials m ON m.id=p.material_id
	 WHERE p.graded=0 AND (m.id=? OR m.source_id=? OR p.goal_id IN(SELECT id FROM goals WHERE source_id=?)) AND (p.id=(SELECT current_id FROM review_session WHERE singleton=1) OR p.id IN(SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return')))`, id, sourceID, sourceID)
	if err != nil {
		return err
	}
	for _, pid := range ids {
		p, err := presentation(ctx, tx, pid)
		if err != nil {
			return err
		}
		if err = endPresentationDisclosure(ctx, tx, p, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id IN(SELECT p.id FROM presentations p JOIN materials m ON m.id=p.material_id WHERE p.graded=0 AND (m.id=? OR m.source_id=? OR p.goal_id IN(SELECT id FROM goals WHERE source_id=?)))`, id, sourceID, sourceID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE bridges SET status='unavailable',reason='Retained target or chosen goal changed or archived; original history remains available',updated_at=? WHERE status IN('pending','active','ready_return') AND target_presentation_id IN(SELECT p.id FROM presentations p JOIN materials m ON m.id=p.material_id WHERE p.graded=0 AND (m.id=? OR m.source_id=? OR p.goal_id IN(SELECT id FROM goals WHERE source_id=?)))`, now, id, sourceID, sourceID)
	return err
}

func previewMaterial(m Material) Presentation {
	p := Presentation{Kind: "reference", GoalID: m.GoalID, GoalRevision: m.GoalRevision, DueAt: m.DueAt, Material: &m}
	if m.Kind == "quiz" {
		p.Kind = "quiz"
		if m.Quiz != nil {
			p.Quiz = *m.Quiz
		}
	}
	hideAnswer(&p)
	p.Material.Body = ""
	p.Material.Evidence = ""
	p.Material.Diagram = nil
	p.Material.ReferenceURL = ""
	p.Material.Links = nil
	p.Material.Provenance = Provenance{}
	p.Material.PlanningReason = ""
	if p.Kind == "reference" {
		p.Material.Title = "Instruction ready"
	}
	return p
}

func sessionState(ctx context.Context, tx *sql.Tx, now int64, establish bool) (ReviewState, error) {
	state := ReviewState{Suspended: []*Presentation{}}
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return state, err
	}
	b, err := activeBridge(ctx, tx)
	if err != nil {
		return state, err
	}
	state.Bridge = b
	if b != nil {
		p, err := presentation(ctx, tx, b.TargetPresentationID)
		if err != nil {
			return state, err
		}
		hideAnswer(&p)
		state.Suspended = append(state.Suspended, &p)
	}
	if current.Valid {
		p, err := presentation(ctx, tx, current.String)
		if err != nil {
			return state, err
		}
		state.Current = &p
	} else if establish {
		var m *Material
		var scheduleVersion int
		bridgeID, reason := "", ""
		if b != nil && b.Status == "active" && b.Position < len(b.Path) {
			if len(b.Path) != len(b.PathVersions) {
				return state, fmt.Errorf("%w: bridge version path is corrupt", ErrInvalid)
			}
			candidate, err := material(ctx, tx, b.Path[b.Position])
			if err != nil {
				return state, err
			}
			g, err := goalHeader(ctx, tx, b.GoalID)
			if err != nil {
				return state, err
			}
			remaining, newRemaining, err := bridgeAllowance(ctx, tx, b.GoalID, now)
			if err != nil {
				return state, err
			}
			withinPacing := candidate.EstimatedSeconds <= remaining
			if candidate.Kind == "quiz" {
				attempted, err := materialPreviouslyAttempted(ctx, tx, candidate.ID)
				if err != nil {
					return state, err
				}
				withinPacing = withinPacing && (attempted || newRemaining > 0)
			}
			if candidate.Archived || candidate.Version != b.PathVersions[b.Position] || g.Archived || !withinPacing {
				if _, err = tx.ExecContext(ctx, "UPDATE bridges SET status='ready_return',reason='A planned bridge step or chosen goal changed; return or inspect the new version',updated_at=? WHERE id=?", now, b.ID); err != nil {
					return state, err
				}
				b.Status = "ready_return"
			} else {
				candidate.GoalID, candidate.GoalRevision = g.ID, g.Revision
				m = &candidate
				bridgeID = b.ID
				reason = b.Reason
				if m.Quiz != nil {
					if err = tx.QueryRowContext(ctx, "SELECT version FROM schedules WHERE quiz_id=?", m.Quiz.ID).Scan(&scheduleVersion); err != nil {
						return state, err
					}
				}
			}
		}
		if m == nil {
			m, scheduleVersion, err = knowledgeCandidate(ctx, tx, now, "")
			if err != nil {
				return state, err
			}
			if m != nil {
				reason = strings.TrimSpace(m.SelectionPolicy + "; " + m.PlanningReason)
			}
		}
		if m != nil {
			p, err := establishPresentation(ctx, tx, *m, scheduleVersion, bridgeID, reason, now)
			if err != nil {
				return state, err
			}
			state.Current = &p
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(sc.due_at<=? AND EXISTS(SELECT 1 FROM interactions i WHERE i.material_id=m.id AND i.kind IN('review','practice'))),0),COALESCE(min(CASE WHEN sc.due_at>? THEN sc.due_at END),0) FROM materials m JOIN sources src ON src.id=m.source_id LEFT JOIN schedules sc ON sc.quiz_id=m.id WHERE m.archived=0 AND src.archived=0`, now, now).Scan(&state.Total, &state.Due, &state.NextDueAt); err != nil {
		return state, err
	}
	if state.Current != nil {
		if establish {
			m, _, err := knowledgeCandidate(ctx, tx, now, state.Current.Material.ID)
			if err != nil {
				return state, err
			}
			if m != nil {
				p := previewMaterial(*m)
				state.Preview = &p
			}
		}
		hideAnswer(state.Current)
	}
	if state.Bridge != nil {
		p, err := presentation(ctx, tx, state.Bridge.TargetPresentationID)
		if err != nil {
			return state, err
		}
		hideAnswer(&p)
		state.Suspended = []*Presentation{&p}
	}
	return state, nil
}
