package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/misty-step/scry/internal/learning"
)

func (s *Store) History(ctx context.Context, limit int) ([]ReviewEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, fmt.Errorf("%w: history supports up to 1000 recent attempts; use export for all history", ErrInvalid)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.presentation_id,e.snapshot,e.answer,e.outcome,e.rating,e.assisted,e.reviewed_at,e.due_at,e.algorithm,
	 EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=e.id) FROM review_events e ORDER BY e.reviewed_at DESC,e.rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]ReviewEvent, 0)
	for rows.Next() {
		var event ReviewEvent
		var snapshot string
		if err = rows.Scan(&event.ID, &event.PresentationID, &snapshot, &event.Answer, &event.Outcome, &event.Rating, &event.Assisted,
			&event.ReviewedAt, &event.DueAt, &event.Algorithm, &event.Disputed); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(snapshot), &event.Quiz); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) Summary(ctx context.Context) (Summary, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	defer tx.Rollback()
	var result Summary
	now := s.now()
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE archived=0").Scan(&result.Sources); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(sc.due_at<=?),0),COALESCE(min(CASE WHEN sc.due_at>? THEN sc.due_at END),0)
	 FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id WHERE q.archived=0 AND src.archived=0`, now, now).
		Scan(&result.Quizzes, &result.Due, &result.NextDueAt); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(assisted),0),COALESCE(sum(EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=e.id)),0)
	 FROM review_events e`).Scan(&result.Reviews, &result.Assisted, &result.Disputed); err != nil {
		return result, err
	}
	// CostMicros is conservatively accounted lifetime spend: actual usage when
	// known, otherwise the reservation. CostUnknown includes pending claims.
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(sum(COALESCE(cost_micros,reserved_micros)),0),EXISTS(SELECT 1 FROM job_attempts WHERE cost_micros IS NULL) FROM job_attempts").
		Scan(&result.CostMicros, &result.CostUnknown); err != nil {
		return result, err
	}
	var backup BackupRecord
	err = tx.QueryRowContext(ctx, "SELECT id,path,remote_key,sha256,error,created_at,bytes,remote FROM backups ORDER BY created_at DESC,rowid DESC LIMIT 1").
		Scan(&backup.ID, &backup.Path, &backup.RemoteKey, &backup.SHA256, &backup.Error, &backup.CreatedAt, &backup.Bytes, &backup.Remote)
	if err == nil {
		result.LastBackup = &backup
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return Summary{}, err
	}
	return result, nil
}

// Export is a coherent read-only JSON snapshot, not a second persistence layer.
// Content and operation/version history are retained. Runtime credentials are
// never stored here; private object URLs/paths and live claim tokens are omitted.
// FSRS card times retain the upstream RFC3339 format; other timestamps are Unix
// milliseconds. JSON-valued columns are objects rather than doubly encoded text.
func (s *Store) Export(ctx context.Context) ([]byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := map[string]any{
		"format": "scry-personal-export", "format_version": 1, "schema_version": SchemaVersion,
		"exported_at": s.now(), "algorithm": learning.Algorithm,
	}
	for _, section := range []struct{ name, query string }{
		{"sources", "SELECT id,text,kind,revision,archived,created_at FROM sources ORDER BY created_at,id"},
		{"source_revisions", "SELECT * FROM source_revisions ORDER BY source_id,revision"},
		{"quizzes", "SELECT * FROM quizzes ORDER BY created_at,id"},
		{"quiz_versions", "SELECT * FROM quiz_versions ORDER BY quiz_id,version"},
		{"schedules", "SELECT * FROM schedules ORDER BY quiz_id"},
		{"presentations", "SELECT * FROM presentations ORDER BY created_at,id"},
		{"review_session", "SELECT * FROM review_session"},
		{"review_events", "SELECT * FROM review_events ORDER BY reviewed_at,rowid"},
		{"corrections", "SELECT * FROM corrections ORDER BY created_at,id"},
		{"operations", "SELECT * FROM operations ORDER BY created_at,id"},
		{"jobs", "SELECT id,source_id,source_revision,status,error,model,prompt_version,attempts,created_at,updated_at,available_at,lease_until,published,result_json FROM jobs ORDER BY created_at,id"},
		{"job_attempts", "SELECT job_id,number,started_at,finished_at,reserved_micros,cost_micros,state,finish_code FROM job_attempts ORDER BY started_at,job_id,number"},
		{"backups", "SELECT id,sha256,error,created_at,bytes,remote FROM backups ORDER BY created_at,id"},
	} {
		data, err := exportRows(ctx, tx, section.query)
		if err != nil {
			return nil, err
		}
		result[section.name] = data
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return encoded, nil
}

func exportRows(ctx context.Context, tx *sql.Tx, query string) ([]map[string]any, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		if err = rows.Scan(pointers...); err != nil {
			return nil, err
		}
		record := make(map[string]any, len(columns))
		for i, column := range columns {
			value := decodeExportValue(column, values[i])
			if err, ok := value.(error); ok {
				return nil, err
			}
			record[column] = value
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func decodeExportValue(column string, value any) any {
	switch column {
	case "content", "card", "snapshot", "schedule_before", "schedule_after", "result_json":
		if value == nil {
			return nil
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("invalid JSON storage in %s", column)
		}
		return json.RawMessage(text)
	case "archived", "assisted", "graded", "disputed", "remote":
		switch n := value.(type) {
		case int64:
			return n != 0
		case nil:
			return false
		default:
			return fmt.Errorf("invalid boolean storage in %s", column)
		}
	default:
		return value
	}
}
