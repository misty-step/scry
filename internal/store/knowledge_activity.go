package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Interactions and the browser disclosure fence share exactly one bounded
// timeline selection. Delivery grants are not reading observations.
func (s *Store) Interactions(ctx context.Context, limit int) ([]Interaction, error) {
	if limit <= 0 {
		limit = HistoryPageSize
	}
	if limit > 1000 {
		return nil, fmt.Errorf("%w: use export for more than 1000 activities", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pins, err := activitySelection(ctx, tx, limit, "", 0)
	if err != nil {
		return nil, err
	}
	result := make([]Interaction, 0, len(pins))
	for _, pin := range pins {
		var item Interaction
		if pin.Kind == "inspection" {
			var units, materials string
			if err = tx.QueryRowContext(ctx, `SELECT id,kind,entity_id,unit_versions_json,material_versions_json,at FROM inspection_events WHERE id=?`, pin.ID).Scan(&item.ID, &item.EntityKind, &item.EntityID, &units, &materials, &item.At); err != nil {
				return nil, err
			}
			item.Kind, item.Outcome, item.Assisted = "inspection", "inspected", true
			item.Policy, item.Reason = inspectionPolicy, "Explicit answer-bearing inspection; not demonstrated understanding"
			if err = json.Unmarshal([]byte(units), &item.UnitVersions); err != nil {
				return nil, err
			}
			if err = json.Unmarshal([]byte(materials), &item.MaterialVersions); err != nil {
				return nil, err
			}
			result = append(result, item)
			continue
		}
		err = tx.QueryRowContext(ctx, `SELECT i.id,i.presentation_id,i.material_id,i.material_version,i.kind,i.outcome,i.assisted,i.at,i.reason,i.answer,i.mode,i.practice,
		 COALESCE(e.id,''),EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=e.id),p.goal_id,p.goal_revision,p.goal_pin_origin
		 FROM interactions i JOIN presentations p ON p.id=i.presentation_id LEFT JOIN review_events e ON e.id=i.id WHERE i.id=?`, pin.ID).
			Scan(&item.ID, &item.PresentationID, &item.MaterialID, &item.MaterialVersion, &item.Kind, &item.Outcome, &item.Assisted, &item.At, &item.Reason, &item.Answer, &item.Mode, &item.Practice, &item.ReviewID, &item.Disputed, &item.GoalID, &item.GoalRevision, &item.GoalPinOrigin)
		if err != nil {
			return nil, err
		}
		m, err := materialVersion(ctx, tx, item.MaterialID, item.MaterialVersion)
		if err != nil {
			return nil, err
		}
		m.DueAt = 0
		m.GoalID, m.GoalRevision = item.GoalID, item.GoalRevision
		if m.Quiz != nil {
			m.Quiz.DueAt = 0
		}
		if item.ReviewID != "" {
			var raw string
			if err = tx.QueryRowContext(ctx, "SELECT snapshot FROM review_events WHERE id=?", item.ReviewID).Scan(&raw); err != nil {
				return nil, err
			}
			var quiz Quiz
			if err = json.Unmarshal([]byte(raw), &quiz); err != nil {
				return nil, err
			}
			m.Quiz = &quiz
		}
		if !pin.Disclosure {
			if m.Quiz != nil {
				p := Presentation{Kind: "quiz", Quiz: *m.Quiz, Material: &m}
				hideAnswer(&p)
				m = *p.Material
			} else {
				m, err = materialMetadataVersion(ctx, tx, item.MaterialID, item.MaterialVersion)
				if err != nil {
					return nil, err
				}
				m.Title = "Instruction activity"
				m.GoalID, m.GoalRevision = item.GoalID, item.GoalRevision
			}
			if item.Kind != "review" && item.Kind != "practice" {
				item.Reason = "Explicit " + item.Kind + "; not demonstrated understanding"
			}
		}
		item.Snapshot = m
		result = append(result, item)
	}
	return result, tx.Commit()
}
