package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// [S111-1] One interleaved session across every deck.
//
// A deck is a top-level idea (a starter set, a book, a topic): an item's
// deck is its idea's parent, or the idea itself when it is top-level.
//
// first_presentations keeps the daily limit on new cards honest across
// every session, focused or interleaved. A row says: this never-reviewed
// card was put in front of the learner on this day (their local date, in
// their stored time zone). It is written in the same transaction that
// places the card in a session, and taken back only for a card the
// learner never saw: when a session is replaced (Study this idea, Study
// all decks) its new cards still waiting behind the one on screen are
// released, so switching sessions never spends the day's limit on cards
// that were not shown. A card answered, snoozed or removed while it was
// on screen stays counted. The table is additive (CREATE TABLE IF NOT
// EXISTS on every open), so an older database opens unchanged.
const firstPresentationsSchema = `CREATE TABLE IF NOT EXISTS first_presentations (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  day        TEXT NOT NULL,
  session_id INTEGER NOT NULL DEFAULT 0,
  at         INTEGER NOT NULL,
  PRIMARY KEY (user_id, day, item_id)
)`

// StudyCandidate is an item that may go into a study session, with the
// deck (top-level idea) it belongs to.
type StudyCandidate struct {
	Item
	Deck int64
}

// candidateScan scans an item row followed by its deck.
type candidateScan struct {
	rows *sql.Rows
	deck *int64
}

func (c candidateScan) Scan(dest ...any) error { return c.rows.Scan(append(dest, c.deck)...) }

// StudyCandidates lists what a session may hold: due reviews (reviewed
// at least once, due before the given moment), earliest due first, at
// most reviewLimit of them; and new cards (never reviewed) whose time has
// come, oldest first, at most newLimit. A card held by bridges, removed,
// or snoozed past the moment is left out. scope, when not zero, keeps only
// the items of that idea and of the ideas inside it (a focused session).
func (u *UserStore) StudyCandidates(ctx context.Context, before time.Time, scope int64, reviewLimit, newLimit int) (reviews, fresh []StudyCandidate, err error) {
	where := ` WHERE t.user_id = ?1 AND t.deleted_at = 0 AND i.deleted_at = 0 AND t.due_at < ?2 AND` + notHeld
	if scope != 0 {
		where += ` AND (i.id = ?3 OR i.parent_id = ?3)`
	}
	query := func(cond, order string, limit int) ([]StudyCandidate, error) {
		rows, err := u.s.db.QueryContext(ctx, `SELECT `+itemCols+`, COALESCE(i.parent_id, i.id)`+itemFrom+where+cond+order+` LIMIT ?4`,
			u.uid, ts(before), scope, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []StudyCandidate
		for rows.Next() {
			var c StudyCandidate
			it, err := scanItem(candidateScan{rows, &c.Deck})
			if err != nil {
				return nil, err
			}
			c.Item = it
			out = append(out, c)
		}
		return out, rows.Err()
	}
	if reviews, err = query(` AND t.reps > 0`, ` ORDER BY t.due_at, t.id`, reviewLimit); err != nil {
		return nil, nil, err
	}
	if fresh, err = query(` AND t.reps = 0`, ` ORDER BY t.created_at, t.id`, newLimit); err != nil {
		return nil, nil, err
	}
	return reviews, fresh, nil
}

// PresentedOn lists the never-reviewed cards first presented on day (the
// learner's local date, "2006-01-02"), in any session, each with the
// session that first presented it.
func (u *UserStore) PresentedOn(ctx context.Context, day string) (map[int64]int64, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT item_id, session_id FROM first_presentations WHERE user_id = ? AND day = ?`, u.uid, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, session int64
		if err := rows.Scan(&id, &session); err != nil {
			return nil, err
		}
		out[id] = session
	}
	return out, rows.Err()
}

// SessionPlan is a session to start: its items in order, which of them
// are new cards (first presentations), and the daily limit they count
// against.
type SessionPlan struct {
	Items    []int64
	New      map[int64]bool // never-reviewed cards among Items
	Day      string         // the learner's local date, for the daily limit
	DailyNew int            // first presentations allowed on Day, across every session
	Scope    int64          // the idea a focused session is over; 0 for every deck
	Now      time.Time
	// RequireOne refuses to start a session none of whose items could be
	// placed (ErrNothingToStudy), leaving the open session as it was.
	RequireOne bool
}

// ErrNothingToStudy is a session plan with nothing left to place.
var ErrNothingToStudy = errors.New("nothing to study")

// StartStudy opens a session over the plan's items, in order, ending the
// open one. In the same transaction it releases the open session's new
// cards the learner never saw, counts the day's first presentations
// again, places a new card only while the daily limit allows (a card
// already presented today is free), and records each one placed. Two tabs
// starting sessions at once therefore never exceed the limit. A plan whose
// every item has gone (removed, held, or over the limit) starts nothing
// when RequireOne is set: ErrNothingToStudy, and the open session stays as
// it was.
func (u *UserStore) StartStudy(ctx context.Context, p SessionPlan) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, pending_review_id, retry_item_id FROM study_sessions WHERE user_id = ? AND ended_at = 0`, u.uid)
		if err != nil {
			return err
		}
		var open []sessionRow
		var openIDs []int64
		for rows.Next() {
			var r sessionRow
			var sid int64
			if err := rows.Scan(&sid, &r.pending, &r.retry); err != nil {
				rows.Close()
				return err
			}
			open, openIDs = append(open, r), append(openIDs, sid)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i, sid := range openIDs {
			if err := u.releaseUnseen(ctx, tx, sid, open[i]); err != nil {
				return err
			}
		}
		// Starting a session ends the open one (EventEnd): like EndSession,
		// an ended session keeps no judgment on screen and no retry.
		if _, err := tx.ExecContext(ctx, `UPDATE study_sessions SET ended_at = ?, pending_review_id = 0, retry_item_id = 0
			WHERE user_id = ? AND ended_at = 0`, ts(p.Now), u.uid); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO study_sessions (user_id, started_at, scope_idea_id) VALUES (?, ?, ?)`, u.uid, ts(p.Now), p.Scope)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		var presented int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM first_presentations WHERE user_id = ? AND day = ?`, u.uid, p.Day).Scan(&presented); err != nil {
			return err
		}
		placed := 0
		for pos, item := range p.Items {
			fresh := p.New[item]
			already := false
			if fresh {
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM first_presentations WHERE user_id = ? AND day = ? AND item_id = ?`,
					u.uid, p.Day, item).Scan(&n); err != nil {
					return err
				}
				already = n > 0
				if !already && presented >= p.DailyNew {
					continue // the day's first presentations are spent
				}
			}
			// The SELECT keeps other learners' items out of the session.
			res, err := tx.ExecContext(ctx, `
				INSERT INTO session_items (user_id, session_id, position, item_id)
				SELECT ?1, ?2, ?3, t.id FROM items t WHERE t.id = ?4 AND t.user_id = ?1 AND t.deleted_at = 0 AND`+notHeld, u.uid, id, pos+1, item)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				continue
			}
			placed++
			if !fresh {
				continue
			}
			// A card presented earlier today keeps the session that first
			// presented it, so replacing this one never takes it back.
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO first_presentations (user_id, item_id, day, session_id, at) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (user_id, day, item_id) DO NOTHING`, u.uid, item, p.Day, id, ts(p.Now)); err != nil {
				return err
			}
			if !already {
				presented++
			}
		}
		if placed == 0 && p.RequireOne {
			return ErrNothingToStudy
		}
		return nil
	})
	return id, err
}

// releaseUnseen takes back the first presentations of a session's new
// cards that the learner never saw: entries still waiting behind the card
// on screen. In PhaseAsking the card on screen is the first waiting entry
// and stays counted; with a judgment on screen (or an exact text being
// retried) every waiting entry is unseen.
func (u *UserStore) releaseUnseen(ctx context.Context, tx *sql.Tx, sessionID int64, row sessionRow) error {
	live := ` FROM session_items si JOIN items t ON t.id = si.item_id AND t.user_id = si.user_id
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE si.session_id = ?1 AND si.user_id = ?2 AND si.review_id = 0 AND t.deleted_at = 0 AND i.deleted_at = 0 AND` + notHeld
	var shown int64
	if row.pending == 0 && row.retry == 0 {
		err := tx.QueryRowContext(ctx, `SELECT si.item_id`+live+` ORDER BY si.position LIMIT 1`, sessionID, u.uid).Scan(&shown)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM first_presentations WHERE user_id = ?2 AND session_id = ?1 AND item_id != ?3
		AND item_id IN (SELECT si.item_id`+live+`)`, sessionID, u.uid, shown)
	return err
}
