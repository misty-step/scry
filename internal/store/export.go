package store

import (
	"context"
	"time"
)

// Export is everything Scry holds for one learner, as plain data.
type Export struct {
	Exported time.Time                   `json:"exported"`
	Email    string                      `json:"email"`
	Tables   map[string][]map[string]any `json:"tables"`
}

// Export gathers every row the learner owns. Secrets (token hashes) are
// left out.
func (u *UserStore) Export(ctx context.Context, now time.Time) (Export, error) {
	out := Export{Exported: now.UTC(), Tables: map[string][]map[string]any{}}
	if err := u.s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, u.uid).Scan(&out.Email); err != nil {
		return out, err
	}
	for _, t := range userTables {
		rows, err := u.s.db.QueryContext(ctx, `SELECT * FROM `+t+` WHERE user_id = ?`, u.uid)
		if err != nil {
			return out, err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return out, err
		}
		list := []map[string]any{}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return out, err
			}
			row := map[string]any{}
			for i, c := range cols {
				if c == "token_hash" || c == "user_id" {
					continue
				}
				if b, ok := vals[i].([]byte); ok {
					row[c] = string(b)
				} else {
					row[c] = vals[i]
				}
			}
			list = append(list, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
		out.Tables[t] = list
	}
	return out, nil
}
