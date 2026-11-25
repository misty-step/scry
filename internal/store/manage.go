package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Managing a card without leaving Study (S106-5): edit its text and its
// idea, snooze it until tomorrow, remove it and restore it.

// ItemRemovedAt reports when one of the learner's items was removed, or
// the zero time while it is in the library. Another learner's item, or one
// already purged, is ErrNotFound.
func (u *UserStore) ItemRemovedAt(ctx context.Context, id int64) (time.Time, error) {
	var del int64
	err := u.s.db.QueryRowContext(ctx, `SELECT deleted_at FROM items WHERE id = ? AND user_id = ?`, id, u.uid).Scan(&del)
	return tm(del), notFound(err)
}

// EditItemIdea changes a card's question and correct answer and moves it
// to the idea named ideaTitle, in one transaction. The idea is an existing
// live idea of this learner whose title matches case-insensitively: the
// card's own idea first, then one under the same top-level idea, then any
// other (oldest first). When none matches, a new idea is created: inside
// the same top-level idea when the card's idea sits inside one, otherwise
// at the top level. It reports whether a new idea was made. Whether the
// card is judged word for word, its schedule and its history stay.
func (u *UserStore) EditItemIdea(ctx context.Context, id int64, prompt, answer, ideaTitle string, now time.Time) (created bool, err error) {
	prompt, answer, ideaTitle = strings.TrimSpace(prompt), strings.TrimSpace(answer), strings.TrimSpace(ideaTitle)
	if prompt == "" || answer == "" || ideaTitle == "" {
		return false, errors.New("an item needs a question, an answer and an idea")
	}
	err = u.tx(ctx, func(tx *sql.Tx) error {
		var ideaID, parentID int64
		err := tx.QueryRowContext(ctx, `
			SELECT t.idea_id, COALESCE(i.parent_id, 0) FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
			WHERE t.id = ? AND t.user_id = ? AND t.deleted_at = 0`, id, u.uid).Scan(&ideaID, &parentID)
		if err != nil {
			return notFound(err)
		}
		top := ideaID
		if parentID != 0 {
			top = parentID
		}
		var target int64
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM ideas WHERE user_id = ? AND deleted_at = 0 AND title = ? COLLATE NOCASE
			ORDER BY (id = ?) DESC, (COALESCE(parent_id, id) = ?) DESC, id LIMIT 1`, u.uid, ideaTitle, ideaID, top).Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			target, created, err = u.ensureIdea(ctx, tx, parentID, ideaTitle, "", "", now)
		}
		if err != nil {
			return err
		}
		return one(tx.ExecContext(ctx, `
			UPDATE items SET prompt = ?, answer = ?, idea_id = ? WHERE id = ? AND user_id = ? AND deleted_at = 0`,
			prompt, answer, target, id, u.uid))
	})
	return created, err
}

// SnoozeItem puts a card away until the given moment (the start of the
// learner's tomorrow). It moves only the card's due time, and only later,
// never earlier; the rest of its schedule and its history are untouched.
// A card still waiting in the open session is taken out of it; an answered
// entry stays, with its review. A card not in the library is ErrNotFound.
func (u *UserStore) SnoozeItem(ctx context.Context, id int64, until time.Time) error {
	return u.tx(ctx, func(tx *sql.Tx) error {
		if err := one(tx.ExecContext(ctx, `
			UPDATE items SET due_at = MAX(due_at, ?) WHERE id = ? AND user_id = ? AND deleted_at = 0`, ts(until), id, u.uid)); err != nil {
			return err
		}
		return u.leaveOpenSession(ctx, tx, id)
	})
}

// leaveOpenSession takes an item that has not been answered out of the
// learner's open session.
func (u *UserStore) leaveOpenSession(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM session_items WHERE user_id = ?1 AND item_id = ?2 AND review_id = 0
		AND session_id IN (SELECT id FROM study_sessions WHERE user_id = ?1 AND ended_at = 0)`, u.uid, id)
	return err
}

// RemovedCard is a card removed on its own (its idea is still in the
// library), restorable until the sweep purges it.
type RemovedCard struct {
	ID        int64
	Prompt    string
	IdeaTitle string
	Removed   time.Time
}

// RemovedCards lists cards removed on their own, newest first. Cards
// removed with their idea come back with it, so they are listed under the
// idea instead.
func (u *UserStore) RemovedCards(ctx context.Context) ([]RemovedCard, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT t.id, t.prompt, i.title, t.deleted_at FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE t.user_id = ? AND t.deleted_at > 0 AND i.deleted_at = 0
		ORDER BY t.deleted_at DESC, t.id DESC LIMIT 200`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemovedCard
	for rows.Next() {
		var c RemovedCard
		var del int64
		if err := rows.Scan(&c.ID, &c.Prompt, &c.IdeaTitle, &del); err != nil {
			return nil, err
		}
		c.Removed = tm(del)
		out = append(out, c)
	}
	return out, rows.Err()
}

// RestoreItem undoes RemoveItem for a card whose idea is in the library:
// it is back in its idea and in study queues (and, if its open session
// still holds it unanswered, back in that session). Restoring a card that
// is already in the library changes nothing. A card of a removed idea is
// restored with its idea (RestoreIdea); here it is ErrNotFound, as is
// another learner's card.
func (u *UserStore) RestoreItem(ctx context.Context, id int64) error {
	return u.tx(ctx, func(tx *sql.Tx) error {
		var del, ideaDel int64
		err := tx.QueryRowContext(ctx, `
			SELECT t.deleted_at, i.deleted_at FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
			WHERE t.id = ? AND t.user_id = ?`, id, u.uid).Scan(&del, &ideaDel)
		if err != nil {
			return notFound(err)
		}
		if ideaDel != 0 {
			return ErrNotFound
		}
		if del == 0 {
			return nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE items SET deleted_at = 0 WHERE id = ? AND user_id = ?`, id, u.uid)
		return err
	})
}
