package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

// Review may establish the one current occurrence, but never clears feedback,
// grades an answer, or advances a schedule. A preview has no submission token.
func (s *Store) Review(ctx context.Context) (ReviewState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	state, err := reviewState(ctx, tx, s.now())
	if err != nil {
		return ReviewState{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReviewState{}, err
	}
	return state, nil
}

// Current reads only an already-established occurrence. Library/help guards use
// it without turning browsing into a new review or an assistance event.
func (s *Store) Current(ctx context.Context) (*Presentation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&id); err != nil {
		return nil, err
	}
	if !id.Valid {
		return nil, tx.Commit()
	}
	p, err := presentation(ctx, tx, id.String)
	if err != nil {
		return nil, err
	}
	hideAnswer(&p)
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Next(ctx context.Context, presentationID string) (ReviewState, error) {
	if presentationID == "" {
		return ReviewState{}, fmt.Errorf("%w: missing presentation ID", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	// The predicate is the entire advancement authority. Repeated/stale Next
	// cannot consume a new prompt, an ungraded response, or another held result.
	_, err = tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id=?
	 AND EXISTS(SELECT 1 FROM presentations WHERE id=? AND graded=1)`, presentationID, presentationID)
	if err != nil {
		return ReviewState{}, err
	}
	state, err := reviewState(ctx, tx, s.now())
	if err != nil {
		return ReviewState{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReviewState{}, err
	}
	return state, nil
}

func reviewState(ctx context.Context, tx *sql.Tx, now int64) (ReviewState, error) {
	var state ReviewState
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return state, err
	}
	if current.Valid {
		p, err := presentation(ctx, tx, current.String)
		if err != nil {
			return state, err
		}
		state.Current = &p
	} else {
		q, scheduleVersion, err := candidate(ctx, tx, now, "")
		if err != nil {
			return state, err
		}
		if q != nil {
			p := Presentation{ID: newID(), Quiz: *q, DueAt: q.DueAt}
			snapshot, err := marshal(q)
			if err != nil {
				return state, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at)
			 VALUES(?,?,?,?,?,?,?)`, p.ID, q.ID, q.Version, scheduleVersion, snapshot, now, q.DueAt)
			if err != nil {
				return state, err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=? WHERE singleton=1", p.ID); err != nil {
				return state, err
			}
			state.Current = &p
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(sc.due_at<=?),0),COALESCE(min(CASE WHEN sc.due_at>? THEN sc.due_at END),0)
	 FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id
	 WHERE q.archived=0 AND src.archived=0`, now, now).Scan(&state.Total, &state.Due, &state.NextDueAt); err != nil {
		return state, err
	}
	if state.Current != nil {
		q, _, err := candidate(ctx, tx, now, state.Current.Quiz.ID)
		if err != nil {
			return state, err
		}
		if q != nil {
			state.Preview = &Presentation{Quiz: *q, DueAt: q.DueAt}
			hideAnswer(state.Preview)
		}
		hideAnswer(state.Current)
	}
	return state, nil
}

func hideAnswer(p *Presentation) {
	if !p.Graded {
		p.Quiz.Answer = ""
		p.Quiz.Explanation = ""
		p.Quiz.Evidence = ""
		p.Quiz.Variants = nil
	}
}

func candidate(ctx context.Context, tx *sql.Tx, now int64, exclude string) (*Quiz, int, error) {
	var id string
	var scheduleVersion int
	err := tx.QueryRowContext(ctx, `SELECT q.id,sc.version FROM quizzes q JOIN schedules sc ON sc.quiz_id=q.id
	 JOIN sources src ON src.id=q.source_id WHERE q.archived=0 AND src.archived=0 AND sc.due_at<=? AND q.id<>?
	 ORDER BY sc.due_at,q.created_at,q.rowid LIMIT 1`, now, exclude).Scan(&id, &scheduleVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	q, err := quiz(ctx, tx, id)
	return &q, scheduleVersion, err
}

func presentation(ctx context.Context, tx *sql.Tx, id string) (Presentation, error) {
	var p Presentation
	var snapshot string
	err := tx.QueryRowContext(ctx, `SELECT p.id,p.snapshot,p.answer,p.outcome,p.assisted,p.graded,p.rating,p.due_at,p.reviewed_at,p.review_id,
	 EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=p.review_id) FROM presentations p WHERE p.id=?`, id).
		Scan(&p.ID, &snapshot, &p.Answer, &p.Outcome, &p.Assisted, &p.Graded, &p.Rating, &p.DueAt, &p.ReviewedAt, &p.ReviewID, &p.Disputed)
	if err != nil {
		return p, notFound(err, "presentation")
	}
	if err = json.Unmarshal([]byte(snapshot), &p.Quiz); err != nil {
		return p, fmt.Errorf("decode presented quiz: %w", err)
	}
	return p, nil
}

func (s *Store) Submit(ctx context.Context, presentationID, operationID, answer string, reveal bool) (Presentation, error) {
	if err := validOperation(operationID); err != nil {
		return Presentation{}, err
	}
	if err := validText("presentation ID", presentationID, 200, true); err != nil {
		return Presentation{}, err
	}
	if err := validText("answer", answer, 1024, !reveal); err != nil {
		return Presentation{}, err
	}
	hash := payloadHash(struct {
		Presentation, Answer string
		Reveal               bool
	}{presentationID, answer, reveal})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "submit", hash)
	if err != nil {
		return Presentation{}, err
	}
	if found {
		var p Presentation
		if err = json.Unmarshal([]byte(receipt), &p); err != nil {
			return p, err
		}
		return p, tx.Commit()
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return p, err
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return p, err
	}
	if !current.Valid || current.String != presentationID {
		return Presentation{}, fmt.Errorf("%w: this occurrence is no longer current; reload review", ErrConflict)
	}
	if p.Graded {
		// An assistance fence always wins over a second tab's late success.
		// Fresh operation IDs are not authority to grade an occurrence twice.
		return Presentation{}, fmt.Errorf("%w: this occurrence already has saved feedback; reload review", ErrConflict)
	}
	var contentVersion, scheduleVersion, presentedSchedule int
	var archived bool
	var cardJSON, algorithm string
	err = tx.QueryRowContext(ctx, `SELECT q.version,(q.archived OR src.archived),sc.version,sc.card,sc.algorithm,p.schedule_version
	 FROM presentations p JOIN quizzes q ON q.id=p.quiz_id JOIN sources src ON src.id=q.source_id
	 JOIN schedules sc ON sc.quiz_id=q.id WHERE p.id=?`, presentationID).
		Scan(&contentVersion, &archived, &scheduleVersion, &cardJSON, &algorithm, &presentedSchedule)
	if err != nil {
		return Presentation{}, err
	}
	if archived || contentVersion != p.Quiz.Version || scheduleVersion != presentedSchedule {
		return Presentation{}, fmt.Errorf("%w: question or schedule changed; reload review", ErrConflict)
	}
	if algorithm != learning.Algorithm {
		return Presentation{}, fmt.Errorf("%w: unsupported schedule algorithm %q", ErrConflict, algorithm)
	}
	if p.Quiz.Kind == "choice" && !reveal {
		valid := false
		for _, choice := range p.Quiz.Choices {
			if answer == choice {
				valid = true
				break
			}
		}
		if !valid {
			return Presentation{}, fmt.Errorf("%w: select one of the exact presented choices", ErrInvalid)
		}
	}
	outcome, rating := learning.Grade(p.Quiz.Kind, p.Quiz.Answer, p.Quiz.Variants, answer, reveal)
	now := s.now()
	p.Answer, p.Outcome, p.Rating = answer, outcome, rating
	p.Assisted, p.Graded = reveal, rating != 0
	p.ReviewedAt, p.ReviewID = now, newID()
	afterJSON := cardJSON
	afterVersion := scheduleVersion
	if p.Graded {
		var card learning.Card
		if err = json.Unmarshal([]byte(cardJSON), &card); err != nil {
			return Presentation{}, err
		}
		next, err := learning.Schedule(card, rating, time.UnixMilli(now))
		if err != nil {
			return Presentation{}, err
		}
		afterJSON, err = marshal(next)
		if err != nil {
			return Presentation{}, err
		}
		p.DueAt = next.Due.UnixMilli()
		afterVersion++
		result, err := tx.ExecContext(ctx, "UPDATE schedules SET version=?,card=?,due_at=? WHERE quiz_id=? AND version=?", afterVersion, afterJSON, p.DueAt, p.Quiz.ID, scheduleVersion)
		if err != nil {
			return Presentation{}, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return Presentation{}, err
		}
		if count != 1 {
			return Presentation{}, fmt.Errorf("%w: schedule changed", ErrConflict)
		}
	}
	snapshot, err := marshal(p.Quiz)
	if err != nil {
		return Presentation{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO review_events(id,presentation_id,snapshot,answer,outcome,rating,assisted,reviewed_at,due_at,algorithm,
	 schedule_before,schedule_after,schedule_version_before,schedule_version_after) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ReviewID, p.ID, snapshot, p.Answer, p.Outcome, p.Rating, p.Assisted, now, p.DueAt, learning.Algorithm, cardJSON, afterJSON, scheduleVersion, afterVersion)
	if err != nil {
		return Presentation{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE presentations SET answer=?,outcome=?,assisted=?,graded=?,rating=?,due_at=?,reviewed_at=?,review_id=? WHERE id=?`,
		p.Answer, p.Outcome, p.Assisted, p.Graded, p.Rating, p.DueAt, now, p.ReviewID, p.ID)
	if err != nil {
		return Presentation{}, err
	}
	hideAnswer(&p)
	if err = saveOperation(ctx, tx, operationID, "submit", hash, p.ID, p, now); err != nil {
		return Presentation{}, err
	}
	if err = tx.Commit(); err != nil {
		return Presentation{}, err
	}
	return p, nil
}

// Lifecycle changes retire only unanswered occurrences. A held graded snapshot
// remains visible even if its future content has been edited or archived.
func retireUnanswered(ctx context.Context, tx *sql.Tx, quizID, sourceID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id IN
	 (SELECT p.id FROM presentations p JOIN quizzes q ON q.id=p.quiz_id WHERE p.graded=0 AND (q.id=? OR q.source_id=?))`, quizID, sourceID)
	return err
}
