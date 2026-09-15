package store

import (
	"context"
	"database/sql"
	"fmt"
)

func retainedGoal(ctx context.Context, tx *sql.Tx, p Presentation) (Goal, error) {
	g, err := goalHeader(ctx, tx, p.GoalID)
	if err != nil {
		return g, err
	}
	if g.Archived || p.GoalRevision < 1 || p.Material == nil {
		return g, fmt.Errorf("%w: retained chosen goal is unavailable", ErrConflict)
	}
	var member bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM goal_materials WHERE goal_id=? AND material_id=? AND material_version=?)`, g.ID, p.Material.ID, p.Material.Version).Scan(&member); err != nil {
		return g, err
	}
	if !member {
		return g, fmt.Errorf("%w: retained material is not part of the chosen goal", ErrConflict)
	}
	// A later explicit preference change governs new preparation, while the
	// occurrence continues to report the original selection revision verbatim.
	return g, nil
}

func attachBridgePath(ctx context.Context, tx *sql.Tx, b Bridge, now int64) error {
	for index, id := range b.Path {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO goal_materials(goal_id,material_id,material_version,created_at) VALUES(?,?,?,?)`, b.GoalID, id, b.PathVersions[index], now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO goal_units(goal_id,unit_id,unit_version,created_at)
		 SELECT ?,unit_id,unit_version,? FROM material_links WHERE material_id=? AND material_version=?`, b.GoalID, now, id, b.PathVersions[index]); err != nil {
			return err
		}
	}
	return nil
}
