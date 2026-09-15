package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/misty-step/scry/internal/learning"
)

func presentationScope(p Presentation) (inspectionScope, error) {
	if p.Material == nil {
		return inspectionScope{}, fmt.Errorf("%w: occurrence lacks pinned material", ErrInvalid)
	}
	m := p.Material
	scope := inspectionScope{
		Kind: "presentation", Sources: []string{m.SourceID},
		SourceVersions: []learning.UnitVersion{{ID: m.SourceID, Version: m.Provenance.SourceRevision}},
		Materials:      []learning.UnitVersion{{ID: m.ID, Version: m.Version}}, Units: []learning.UnitVersion{},
	}
	seen := map[learning.UnitVersion]bool{}
	for _, link := range m.Links {
		pin := learning.UnitVersion{ID: link.UnitID, Version: link.UnitVersion}
		if !seen[pin] {
			seen[pin] = true
			scope.Units = append(scope.Units, pin)
		}
	}
	return scope, nil
}

func endPresentationDisclosure(ctx context.Context, tx *sql.Tx, p Presentation, now int64) error {
	if p.Kind != "reference" && !p.Graded {
		return nil
	}
	scope, err := presentationScope(p)
	if err != nil {
		return err
	}
	_, err = saveInspectionScope(ctx, tx, "delivery", p.ID, scope, now)
	return err
}

func presentationAccess(ctx context.Context, tx *sql.Tx, id string, now int64) (InspectionAccess, error) {
	result := InspectionAccess{CheckedAt: now}
	p, err := presentation(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if p.Kind != "reference" && !p.Graded {
		return result, nil
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return result, err
	}
	if current.Valid && current.String == p.ID {
		result.Allowed = true
		return result, nil
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(max(expires_at),0) FROM inspection_events WHERE kind='delivery' AND entity_id=? AND expires_at>?", p.ID, now).Scan(&result.ExpiresAt); err != nil {
		return result, err
	}
	result.Allowed = result.ExpiresAt > now
	return result, nil
}

func activeMaterialExposure(ctx context.Context, tx *sql.Tx, m Material, includeFeedback bool) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.material_id,p.material_version,CASE WHEN p.reviewed_at>0 THEN p.reviewed_at ELSE p.created_at END FROM presentations p WHERE (p.kind='reference' OR (? AND p.graded=1))
	 AND (p.id=(SELECT current_id FROM review_session WHERE singleton=1) OR p.id IN(SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return')))`, includeFeedback)
	if err != nil {
		return 0, err
	}
	type pin struct {
		id      string
		version int
		at      int64
	}
	pins := []pin{}
	for rows.Next() {
		var value pin
		if err = rows.Scan(&value.id, &value.version, &value.at); err != nil {
			rows.Close()
			return 0, err
		}
		pins = append(pins, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	latest := int64(0)
	for _, pin := range pins {
		shown, err := materialMetadataVersion(ctx, tx, pin.id, pin.version)
		if err != nil {
			return 0, err
		}
		scope, err := presentationScope(Presentation{Material: &shown})
		if err != nil {
			return 0, err
		}
		if scopeMatchesMaterial(scope, m) {
			latest = max(latest, pin.at)
		}
	}
	return latest, nil
}

// Old receipts remain byte-for-byte immutable. Only the returned read model
// gains schema-v2 material metadata pinned to the original receipt's quiz.
func hydrateLegacyPresentationReceipt(ctx context.Context, tx *sql.Tx, p *Presentation) error {
	if p.Kind != "" {
		return nil
	}
	if p.Quiz.ID == "" || p.Quiz.Version < 1 {
		return fmt.Errorf("%w: malformed historical presentation receipt", ErrInvalid)
	}
	if err := tx.QueryRowContext(ctx, "SELECT goal_id,goal_revision,goal_pin_origin FROM presentations WHERE id=?", p.ID).Scan(&p.GoalID, &p.GoalRevision, &p.GoalPinOrigin); err != nil {
		return err
	}
	m, err := materialVersion(ctx, tx, p.Quiz.ID, p.Quiz.Version)
	if err != nil {
		return err
	}
	m.Quiz = &p.Quiz
	m.Archived = p.Quiz.Archived
	m.DueAt = p.DueAt
	p.Kind, p.Material = "quiz", &m
	p.Material.GoalID, p.Material.GoalRevision = p.GoalID, p.GoalRevision
	hideAnswer(p)
	return nil
}

func reviewReceipt(receipt string) (ReviewState, error) {
	var state ReviewState
	if err := json.Unmarshal([]byte(receipt), &state); err != nil {
		return state, err
	}
	if state.Bridge != nil && state.Bridge.Job != nil {
		hideJobContext(state.Bridge.Job)
	}
	return state, nil
}
