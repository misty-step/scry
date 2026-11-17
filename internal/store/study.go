package store

import (
	"context"
	"database/sql"
	"time"

	"scry/internal/srs"
)

// Review is one judged answer in the learner's history.
type Review struct {
	ID        int64
	ItemID    int64
	SessionID int64
	At        time.Time
	Answer    string
	Verdict   srs.Verdict
	Reason    string // the one-line why
	Detail    string // JSON: close parts, or the word-for-word diff
	Via       string // typed or spoken
	Judge     string // which judge decided: a model id, or "exact"
	Practice  bool   // an "Again" attempt; it does not move the schedule
}

// SessionEntry is one item of a study session and, once answered, its review.
type SessionEntry struct {
	Position int
	Item     Item
	Review   *Review
}

// StudySession is a run of due items worked through in order.
type StudySession struct {
	ID              int64
	Started, Ended  time.Time
	PendingReviewID int64 // a judgment on screen waiting for Enter
	RetryItemID     int64 // an exact text being tried again
	Entries         []SessionEntry
}

// Current is the entry waiting for an answer, or nil when all are done.
func (s *StudySession) Current() *SessionEntry {
	for i := range s.Entries {
		if s.RetryItemID != 0 && s.Entries[i].Item.ID == s.RetryItemID {
			return &s.Entries[i]
		}
	}
	for i := range s.Entries {
		if s.Entries[i].Review == nil {
			return &s.Entries[i]
		}
	}
	return nil
}

// Reviewed counts answered entries.
func (s *StudySession) Reviewed() int {
	n := 0
	for _, e := range s.Entries {
		if e.Review != nil {
			n++
		}
	}
	return n
}

// DueItems lists items due before the given moment: reviews first, most
// overdue first, then items never seen, oldest first.
func (u *UserStore) DueItems(ctx context.Context, before time.Time, limit int) ([]Item, error) {
	return scanItems(u.s.db.QueryContext(ctx, `SELECT `+itemCols+itemFrom+`
		WHERE t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0 AND t.due_at < ?
		ORDER BY (t.reps = 0), CASE WHEN t.reps = 0 THEN t.id ELSE t.due_at END, t.id LIMIT ?`, u.uid, ts(before), limit))
}

// StartSession opens a session over the given items, in order.
func (u *UserStore) StartSession(ctx context.Context, itemIDs []int64, now time.Time) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE study_sessions SET ended_at = ? WHERE user_id = ? AND ended_at = 0`, ts(now), u.uid); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO study_sessions (user_id, started_at) VALUES (?, ?)`, u.uid, ts(now))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		for pos, item := range itemIDs {
			// The SELECT keeps other learners' items out of the session.
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO session_items (user_id, session_id, position, item_id)
				SELECT ?1, ?2, ?3, id FROM items WHERE id = ?4 AND user_id = ?1 AND deleted_at = 0`, u.uid, id, pos+1, item); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

// ActiveSession returns the open session, or nil when there is none.
func (u *UserStore) ActiveSession(ctx context.Context) (*StudySession, error) {
	return u.session(ctx, `ended_at = 0 ORDER BY id DESC LIMIT 1`)
}

// LastEndedSession returns the most recently finished session, or nil.
func (u *UserStore) LastEndedSession(ctx context.Context) (*StudySession, error) {
	return u.session(ctx, `ended_at > 0 ORDER BY ended_at DESC, id DESC LIMIT 1`)
}

func (u *UserStore) session(ctx context.Context, where string) (*StudySession, error) {
	var s StudySession
	var started, ended int64
	err := u.s.db.QueryRowContext(ctx, `
		SELECT id, started_at, ended_at, pending_review_id, retry_item_id FROM study_sessions WHERE user_id = ? AND `+where, u.uid).
		Scan(&s.ID, &started, &ended, &s.PendingReviewID, &s.RetryItemID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Started, s.Ended = tm(started), tm(ended)
	reviews, err := u.sessionReviews(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	rows, err := u.s.db.QueryContext(ctx, `SELECT si.position, si.review_id, `+itemCols+`
		FROM session_items si JOIN items t ON t.id = si.item_id AND t.user_id = si.user_id
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE si.session_id = ? AND si.user_id = ? AND (t.deleted_at = 0 OR si.review_id != 0)
		ORDER BY si.position`, s.ID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e SessionEntry
		var reviewID int64
		var it Item
		var exact int
		var created, due, last int64
		if err := rows.Scan(&e.Position, &reviewID, &it.ID, &it.IdeaID, &it.IdeaTitle, &it.Prompt, &it.Answer, &exact, &it.TextTitle,
			&it.LineNo, &it.Source, &created, &due, &it.Sched.Stability, &it.Sched.Difficulty, &it.Sched.Reps, &it.Sched.Lapses, &last); err != nil {
			return nil, err
		}
		it.Exact, it.Created = exact == 1, tm(created)
		it.Sched.Due, it.Sched.Last = tm(due), tm(last)
		e.Item = it
		if r, ok := reviews[reviewID]; ok {
			e.Review = &r
		}
		s.Entries = append(s.Entries, e)
	}
	return &s, rows.Err()
}

const reviewCols = `id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice`

func scanReview(row interface{ Scan(...any) error }) (Review, error) {
	var r Review
	var at int64
	var practice int
	var verdict string
	err := row.Scan(&r.ID, &r.ItemID, &r.SessionID, &at, &r.Answer, &verdict, &r.Reason, &r.Detail, &r.Via, &r.Judge, &practice)
	r.At, r.Verdict, r.Practice = tm(at), srs.Verdict(verdict), practice == 1
	return r, notFound(err)
}

func (u *UserStore) sessionReviews(ctx context.Context, sessionID int64) (map[int64]Review, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE session_id = ? AND user_id = ?`, sessionID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Review{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out[r.ID] = r
	}
	return out, rows.Err()
}

// Review loads one review from the learner's history.
func (u *UserStore) Review(ctx context.Context, id int64) (Review, error) {
	return scanReview(u.s.db.QueryRowContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE id = ? AND user_id = ?`, id, u.uid))
}

// History lists the reviews of one item, newest first.
func (u *UserStore) History(ctx context.Context, itemID int64) ([]Review, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE item_id = ? AND user_id = ? ORDER BY at DESC, id DESC LIMIT 50`, itemID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordReview writes a judged answer to the history, reschedules the item
// (unless it was practice), and marks the session entry answered. When
// hold is set the judgment stays on screen until the learner moves on.
func (u *UserStore) RecordReview(ctx context.Context, r Review, next srs.State, hold bool) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO reviews (user_id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice)
			SELECT ?1, id, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11 FROM items WHERE id = ?2 AND user_id = ?1`,
			u.uid, r.ItemID, r.SessionID, ts(r.At), r.Answer, string(r.Verdict), r.Reason, r.Detail, r.Via, r.Judge, b2i(r.Practice))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		id, _ = res.LastInsertId()
		if !r.Practice {
			if _, err := tx.ExecContext(ctx, `
				UPDATE items SET due_at = ?, stability = ?, difficulty = ?, reps = ?, lapses = ?, last_review = ?
				WHERE id = ? AND user_id = ?`,
				ts(next.Due), next.Stability, next.Difficulty, next.Reps, next.Lapses, ts(next.Last), r.ItemID, u.uid); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE session_items SET review_id = ? WHERE session_id = ? AND user_id = ? AND item_id = ? AND review_id = 0`,
				id, r.SessionID, u.uid, r.ItemID); err != nil {
				return err
			}
		}
		pending := int64(0)
		if hold {
			pending = id
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE study_sessions SET pending_review_id = ?, retry_item_id = 0 WHERE id = ? AND user_id = ?`, pending, r.SessionID, u.uid)
		return err
	})
	return id, err
}

// Advance clears the judgment on screen. retryItemID, when not zero, puts
// that item back up for another attempt.
func (u *UserStore) Advance(ctx context.Context, sessionID, retryItemID int64) error {
	_, err := u.s.db.ExecContext(ctx, `
		UPDATE study_sessions SET pending_review_id = 0, retry_item_id = ? WHERE id = ? AND user_id = ?`, retryItemID, sessionID, u.uid)
	return err
}

// EndSession closes a session.
func (u *UserStore) EndSession(ctx context.Context, id int64, now time.Time) error {
	_, err := u.s.db.ExecContext(ctx, `
		UPDATE study_sessions SET ended_at = ?, pending_review_id = 0, retry_item_id = 0 WHERE id = ? AND user_id = ? AND ended_at = 0`,
		ts(now), id, u.uid)
	return err
}

// RecentVerdicts counts the learner's recent verdicts, for shaping new
// material to how they are doing.
func (u *UserStore) RecentVerdicts(ctx context.Context, since time.Time) (right, total int, err error) {
	err = u.s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(verdict = 'right'), 0), COUNT(*) FROM reviews WHERE user_id = ? AND practice = 0 AND at >= ?`,
		u.uid, ts(since)).Scan(&right, &total)
	return
}
