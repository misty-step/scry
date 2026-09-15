package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func (s *Store) Capture(ctx context.Context, text, operationID string) (Source, error) {
	if err := validOperation(operationID); err != nil {
		return Source{}, err
	}
	if err := validText("source (split longer material into separate captures)", text, MaxSourceBytes, true); err != nil {
		return Source{}, err
	}
	hash := payloadHash(text)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	id, _, found, err := existingOperation(ctx, tx, operationID, "capture", hash)
	if err != nil {
		return Source{}, err
	}
	if !found {
		id = newID()
		now := s.now()
		kind := "topic"
		if len(strings.TrimSpace(text)) >= 280 || strings.Contains(strings.TrimSpace(text), "\n") {
			kind = "source"
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO sources(id,text,kind,revision,created_at) VALUES(?,?,?,1,?)", id, text, kind, now)
		if err != nil {
			return Source{}, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO source_revisions(source_id,revision,text,kind,created_at) VALUES(?,1,?,?,?)", id, text, kind, now)
		if err != nil {
			return Source{}, err
		}
		if err = enqueue(ctx, tx, id, 1, now); err != nil {
			return Source{}, err
		}
		if err = saveOperation(ctx, tx, operationID, "capture", hash, id, nil, now); err != nil {
			return Source{}, err
		}
	}
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

func (s *Store) RetrySource(ctx context.Context, id, operationID string) (Source, error) {
	if err := validOperation(operationID); err != nil {
		return Source{}, err
	}
	hash := payloadHash(id)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	_, _, found, err := existingOperation(ctx, tx, operationID, "retry", hash)
	if err != nil {
		return Source{}, err
	}
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if !found {
		if result.Archived || result.Job == nil || result.Job.Published != 0 ||
			(result.Job.Status != "failed" && result.Job.Status != "canceled" && result.Job.Status != "paused") {
			return Source{}, fmt.Errorf("%w: only unpublished failed or explicitly reconciled paused work can be retried", ErrConflict)
		}
		var jobs int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE source_id=?", id).Scan(&jobs); err != nil {
			return Source{}, err
		}
		if jobs >= 3 {
			return Source{}, fmt.Errorf("%w: this source has reached its three manual generation runs; inspect the failure before capturing a revised input", ErrConflict)
		}
		now := s.now()
		if result.Job.Status == "paused" {
			if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',lease_token='',lease_until=0,updated_at=? WHERE id=?", now, result.Job.ID); err != nil {
				return Source{}, err
			}
		}
		if err = enqueue(ctx, tx, id, result.Revision, now); err != nil {
			return Source{}, err
		}
		if err = saveOperation(ctx, tx, operationID, "retry", hash, id, nil, now); err != nil {
			return Source{}, err
		}
		result, err = source(ctx, tx, id, true)
		if err != nil {
			return Source{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

func (s *Store) Sources(ctx context.Context, query string) ([]Source, error) {
	if err := validText("search", query, 1024, false); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query) + "%"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT src.id FROM sources src WHERE ?='' OR src.text LIKE ? ESCAPE '\'
	 OR EXISTS(SELECT 1 FROM quizzes q JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 WHERE q.source_id=src.id AND (json_extract(v.content,'$.prompt') LIKE ? ESCAPE '\' OR json_extract(v.content,'$.answer') LIKE ? ESCAPE '\'
	 OR json_extract(v.content,'$.explanation') LIKE ? ESCAPE '\')) ORDER BY src.created_at DESC,src.id`, query, pattern, pattern, pattern, pattern)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	result := make([]Source, 0, len(ids))
	for _, id := range ids {
		src, err := source(ctx, tx, id, false)
		if err != nil {
			return nil, err
		}
		result = append(result, src)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) Source(ctx context.Context, id string) (Source, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

func source(ctx context.Context, tx *sql.Tx, id string, details bool) (Source, error) {
	var src Source
	err := tx.QueryRowContext(ctx, "SELECT id,text,kind,revision,archived,created_at FROM sources WHERE id=?", id).
		Scan(&src.ID, &src.Text, &src.Kind, &src.Revision, &src.Archived, &src.CreatedAt)
	if err != nil {
		return src, notFound(err, "source")
	}
	var jobID string
	err = tx.QueryRowContext(ctx, "SELECT id FROM jobs WHERE source_id=? ORDER BY created_at DESC,rowid DESC LIMIT 1", id).Scan(&jobID)
	if err == nil {
		j, err := job(ctx, tx, jobID)
		if err != nil {
			return src, err
		}
		src.Job, src.Status = &j, j.Status
		if src.Status == "complete" {
			src.Status = "ready"
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return src, err
	}
	if src.Archived {
		src.Status = "archived"
	}
	if details {
		rows, err := tx.QueryContext(ctx, "SELECT id FROM quizzes WHERE source_id=? ORDER BY created_at,rowid", id)
		if err != nil {
			return src, err
		}
		var ids []string
		for rows.Next() {
			var qid string
			if err = rows.Scan(&qid); err != nil {
				rows.Close()
				return src, err
			}
			ids = append(ids, qid)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return src, err
		}
		rows.Close()
		src.Quizzes = make([]Quiz, 0, len(ids))
		for _, qid := range ids {
			q, err := quiz(ctx, tx, qid)
			if err != nil {
				return src, err
			}
			src.Quizzes = append(src.Quizzes, q)
		}
	}
	return src, nil
}

func (s *Store) Quiz(ctx context.Context, id string) (Quiz, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Quiz{}, err
	}
	defer tx.Rollback()
	q, err := quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if err = tx.Commit(); err != nil {
		return Quiz{}, err
	}
	return q, nil
}

func quiz(ctx context.Context, tx *sql.Tx, id string) (Quiz, error) {
	var q Quiz
	var content string
	err := tx.QueryRowContext(ctx, `SELECT q.id,q.source_id,q.version,(q.archived OR src.archived),v.content,sc.due_at
	 FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 JOIN schedules sc ON sc.quiz_id=q.id WHERE q.id=?`, id).Scan(&q.ID, &q.SourceID, &q.Version, &q.Archived, &content, &q.DueAt)
	if err != nil {
		return q, notFound(err, "quiz")
	}
	var generated GeneratedQuiz
	if err = json.Unmarshal([]byte(content), &generated); err != nil {
		return q, err
	}
	q.Kind, q.Prompt, q.Answer, q.Explanation, q.Evidence, q.Basis = generated.Kind, generated.Prompt, generated.Answer, generated.Explanation, generated.Evidence, generated.Basis
	q.Choices, q.Variants = generated.Choices, generated.Variants
	return q, nil
}

func validateQuiz(q GeneratedQuiz, src Source) error {
	for _, field := range []struct {
		name, value string
		max         int
		required    bool
	}{
		{"prompt", q.Prompt, 4096, true}, {"answer", q.Answer, 1024, true}, {"explanation", q.Explanation, 8192, true}, {"evidence", q.Evidence, 8192, q.Basis == "source"},
	} {
		if err := validText(field.name, field.value, field.max, field.required); err != nil {
			return err
		}
	}
	if q.Basis != src.Kind {
		return fmt.Errorf("%w: quiz basis must honestly match the captured %s input", ErrInvalid, src.Kind)
	}
	if q.Basis == "source" && !strings.Contains(src.Text, q.Evidence) {
		return fmt.Errorf("%w: source evidence must be an exact quotation from the saved input", ErrInvalid)
	}
	if q.Basis == "topic" && q.Evidence != "" {
		return fmt.Errorf("%w: topic knowledge must not claim source evidence", ErrInvalid)
	}
	if len(q.Variants) > 16 {
		return fmt.Errorf("%w: at most 16 explicit answer variants", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, variant := range q.Variants {
		if err := validText("answer variant", variant, 1024, true); err != nil {
			return err
		}
		key := strings.TrimSpace(variant)
		if key == strings.TrimSpace(q.Answer) || seen[key] {
			return fmt.Errorf("%w: answer variants must be distinct", ErrInvalid)
		}
		seen[key] = true
	}
	switch q.Kind {
	case "choice":
		if len(q.Choices) < 2 || len(q.Choices) > 6 || len(q.Variants) != 0 {
			return fmt.Errorf("%w: a choice quiz needs 2–6 choices and no typed variants", ErrInvalid)
		}
		seen = map[string]bool{}
		matches := 0
		for _, choice := range q.Choices {
			if err := validText("choice", choice, 1024, true); err != nil {
				return err
			}
			key := strings.TrimSpace(choice)
			if seen[key] {
				return fmt.Errorf("%w: choices must be distinct", ErrInvalid)
			}
			seen[key] = true
			if choice == q.Answer {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%w: answer must exactly equal one displayed choice", ErrInvalid)
		}
	case "recall":
		if len(q.Choices) != 0 {
			return fmt.Errorf("%w: recall quizzes cannot contain choices", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: quiz kind must be choice or recall", ErrInvalid)
	}
	return nil
}

func (s *Store) EditQuiz(ctx context.Context, id string, expectedVersion int, content GeneratedQuiz) (Quiz, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Quiz{}, err
	}
	defer tx.Rollback()
	q, err := quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if q.Version != expectedVersion {
		return Quiz{}, fmt.Errorf("%w: the quiz was already edited; reload before editing", ErrConflict)
	}
	src, err := source(ctx, tx, q.SourceID, false)
	if err != nil {
		return Quiz{}, err
	}
	if err = validateQuiz(content, src); err != nil {
		return Quiz{}, err
	}
	encoded, err := marshal(content)
	if err != nil {
		return Quiz{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO quiz_versions(quiz_id,version,content,model,prompt_version,created_at) VALUES(?,?,?,'','manual-edit',?)", id, q.Version+1, encoded, s.now())
	if err != nil {
		return Quiz{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE quizzes SET version=version+1 WHERE id=?", id); err != nil {
		return Quiz{}, err
	}
	if err = retireUnanswered(ctx, tx, id, ""); err != nil {
		return Quiz{}, err
	}
	q, err = quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if err = tx.Commit(); err != nil {
		return Quiz{}, err
	}
	return q, nil
}

func (s *Store) ArchiveQuiz(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = quiz(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE quizzes SET archived=1 WHERE id=?", id); err != nil {
		return err
	}
	if err = retireUnanswered(ctx, tx, id, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ArchiveSource(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = source(ctx, tx, id, false); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE sources SET archived=1 WHERE id=?", id); err != nil {
		return err
	}
	now := s.now()
	if _, err = tx.ExecContext(ctx, `UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active'
	 AND job_id IN (SELECT id FROM jobs WHERE source_id=?)`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Source archived; any in-flight cost remains accounted',lease_token='',lease_until=0,updated_at=?
	 WHERE source_id=? AND status IN ('queued','running','retry','paused')`, now, id); err != nil {
		return err
	}
	if err = retireUnanswered(ctx, tx, "", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Dispute(ctx context.Context, reviewID, note string, reset bool) error {
	if err := validText("correction note", note, 4096, true); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var quizID string
	err = tx.QueryRowContext(ctx, "SELECT p.quiz_id FROM review_events e JOIN presentations p ON p.id=e.presentation_id WHERE e.id=?", reviewID).Scan(&quizID)
	if err != nil {
		return notFound(err, "review")
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM corrections WHERE review_id=? AND note=? AND reset=?)", reviewID, note, reset).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	var before, after any
	now := s.now()
	if reset {
		var card string
		if err = tx.QueryRowContext(ctx, "SELECT card FROM schedules WHERE quiz_id=?", quizID).Scan(&card); err != nil {
			return err
		}
		before = card
		encoded, err := marshal(learning.NewCard(time.UnixMilli(now)))
		if err != nil {
			return err
		}
		after = encoded
		if _, err = tx.ExecContext(ctx, "UPDATE schedules SET version=version+1,card=?,due_at=?,algorithm=? WHERE quiz_id=?", encoded, now, learning.Algorithm, quizID); err != nil {
			return err
		}
		if err = retireUnanswered(ctx, tx, quizID, ""); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO corrections(id,review_id,note,reset,created_at,schedule_before,schedule_after) VALUES(?,?,?,?,?,?,?)", newID(), reviewID, note, reset, now, before, after)
	if err != nil {
		return err
	}
	return tx.Commit()
}
