package store

import (
	"context"
	"database/sql"

	"github.com/misty-step/scry/internal/learning"
)

const HistoryPageSize = 100

type activityPin struct {
	ID, Kind, MaterialID, PresentationID string
	MaterialVersion                      int
	At                                   int64
	Disclosure                           bool
}

// A pending explicit inspection is included before its immutable row is saved,
// so its own timeline entry cannot shift an unshown row into a disclosure grant.
func activitySelection(ctx context.Context, tx *sql.Tx, limit int, pendingID string, pendingAt int64) ([]activityPin, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,kind,material_id,material_version,presentation_id,at,disclosure FROM (
	 SELECT i.id,i.kind,i.material_id,i.material_version,i.presentation_id,i.at,
	 (i.kind='continue' OR (i.kind='review' AND e.rating>0) OR (i.kind='practice' AND p.graded=1 AND i.outcome IN('correct','wrong','revealed'))) AS disclosure
	 FROM interactions i JOIN presentations p ON p.id=i.presentation_id LEFT JOIN review_events e ON e.id=i.id
	 UNION ALL
	 SELECT id,'inspection','',0,'',at,0 FROM inspection_events WHERE kind<>'delivery'
	 UNION ALL
	 SELECT ?,'inspection','',0,'',?,0 WHERE ?<>''
	 ) ORDER BY at DESC,id DESC LIMIT ?`, pendingID, pendingAt, pendingID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []activityPin{}
	for rows.Next() {
		var pin activityPin
		if err = rows.Scan(&pin.ID, &pin.Kind, &pin.MaterialID, &pin.MaterialVersion, &pin.PresentationID, &pin.At, &pin.Disclosure); err != nil {
			return nil, err
		}
		result = append(result, pin)
	}
	return result, rows.Err()
}

func historyDisclosureScope(ctx context.Context, tx *sql.Tx, pendingID string, now int64) (inspectionScope, error) {
	c := inspectionScope{Kind: "history", EventID: pendingID, Sources: []string{}, SourceVersions: []learning.UnitVersion{}, Units: []learning.UnitVersion{}, Materials: []learning.UnitVersion{}}
	pins, err := activitySelection(ctx, tx, HistoryPageSize, pendingID, now)
	if err != nil {
		return c, err
	}
	for _, pin := range pins {
		if !pin.Disclosure {
			continue
		}
		m, err := materialMetadataVersion(ctx, tx, pin.MaterialID, pin.MaterialVersion)
		if err != nil {
			return c, err
		}
		p := Presentation{Material: &m}
		part, err := presentationScope(p)
		if err != nil {
			return c, err
		}
		c.Sources = append(c.Sources, part.Sources...)
		c.SourceVersions = append(c.SourceVersions, part.SourceVersions...)
		c.Materials = append(c.Materials, part.Materials...)
		c.Units = append(c.Units, part.Units...)
	}
	return c, nil
}
