package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Bridge cards (Slice 75, the Strugglometer). When a card keeps being
// missed, the learner can add two or three smaller prerequisite cards
// ("bridges") and hold the card they lead to (the "parent") out of every
// due queue until each bridge has been answered Correct or Almost Correct
// once. The parent's schedule is never touched: holding only hides it, and
// when the hold ends it is due exactly when it was.
//
// One row per bridge card. A row is active while ended_at is 0; it ends
// when the bridge is first answered well (mastered), when the learner
// releases the hold by hand (released) or when either card is removed
// (removed). A parent is held while it has an active row whose bridge card
// is still in the library (neither it nor its idea removed), so a bridge
// trashed with its idea stops holding, and one restored with it holds
// again until it is mastered.
//
// The table is additive (CREATE TABLE IF NOT EXISTS on every open): no
// existing table or column changes, so a database from before Slice 75
// opens unchanged and simply has no holds.
const bridgesSchema = `CREATE TABLE IF NOT EXISTS item_bridges (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  parent_item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  bridge_item_id INTEGER NOT NULL UNIQUE REFERENCES items(id) ON DELETE CASCADE,
  accept_key     TEXT NOT NULL,
  position       INTEGER NOT NULL,
  created_at     INTEGER NOT NULL,
  ended_at       INTEGER NOT NULL DEFAULT 0,
  end_reason     TEXT NOT NULL DEFAULT '' CHECK (end_reason IN ('', 'mastered', 'released', 'removed')),
  UNIQUE (user_id, accept_key, position)
)`

const bridgesIndex = `CREATE INDEX IF NOT EXISTS item_bridges_parent ON item_bridges(user_id, parent_item_id, ended_at)`

// activeBridges is the FROM/WHERE of the active bridges holding the item
// aliased t: rows not yet ended whose bridge card and its idea are still in
// the library.
const activeBridges = ` FROM item_bridges hb
	JOIN items hbi ON hbi.id = hb.bridge_item_id AND hbi.user_id = hb.user_id
	JOIN ideas hbd ON hbd.id = hbi.idea_id AND hbd.user_id = hbi.user_id
	WHERE hb.user_id = t.user_id AND hb.parent_item_id = t.id AND hb.ended_at = 0
	  AND hbi.deleted_at = 0 AND hbd.deleted_at = 0`

// notHeld is a condition on the item aliased t: it is not held by bridges.
// Every due queue path (DueItems, StartSession, the entries of an open
// session, the guard on an answer, the due count and the next due date)
// applies it.
const notHeld = ` NOT EXISTS (SELECT 1` + activeBridges + `)`

// Errors of the bridge store.
var (
	// ErrAlreadyHeld is a new set of bridges for a card that is already
	// held by bridges (a second tab, or a second preview accepted).
	ErrAlreadyHeld = errors.New("that card is already held by bridge cards")
	// ErrBridgeKeyReused is an accept key already used for another card.
	ErrBridgeKeyReused = errors.New("that bridge preview was already used for another card")
	// ErrIsBridge is a card that is itself an active bridge: bridges do not
	// nest.
	ErrIsBridge = errors.New("a bridge card cannot have bridges of its own")
)

// BridgeCard is one bridge to add: a question and its answer.
type BridgeCard struct{ Prompt, Answer string }

// BridgeAccept is the outcome of AcceptBridges.
type BridgeAccept struct {
	Bridges   []int64 // the bridge items, in order
	Duplicate bool    // the same accept key was already applied: nothing new was written
}

// BridgeState is what a card's bridges say about it.
type BridgeState struct {
	HeldBy    int   // active bridges holding this card; 0 when it is not held
	BridgeFor int64 // the parent this card is an active bridge for, or 0
}

// AcceptBridges adds bridge cards to a parent card's idea and holds the
// parent, all in one write transaction. key identifies one preview: the
// same key applied twice (a retried or double-submitted form) writes
// nothing the second time and reports the bridges the first made. A parent
// already held by other bridges is ErrAlreadyHeld; a parent that is itself
// an active bridge is ErrIsBridge; another learner's card, or one not in
// the library, is ErrNotFound.
func (u *UserStore) AcceptBridges(ctx context.Context, parentID int64, key string, cards []BridgeCard, now time.Time) (BridgeAccept, error) {
	var out BridgeAccept
	if strings.TrimSpace(key) == "" || len(cards) == 0 {
		return out, errors.New("bridges need an accept key and at least one card")
	}
	err := u.tx(ctx, func(tx *sql.Tx) error {
		var ideaID int64
		err := tx.QueryRowContext(ctx, `SELECT t.idea_id FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
			WHERE t.id = ? AND t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0`, parentID, u.uid).Scan(&ideaID)
		if err != nil {
			return notFound(err)
		}
		// A retry of the same preview: report what it made.
		rows, err := tx.QueryContext(ctx, `SELECT parent_item_id, bridge_item_id FROM item_bridges WHERE user_id = ? AND accept_key = ? ORDER BY position`, u.uid, key)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p, b int64
			if err := rows.Scan(&p, &b); err != nil {
				rows.Close()
				return err
			}
			if p != parentID {
				rows.Close()
				return ErrBridgeKeyReused
			}
			out.Bridges = append(out.Bridges, b)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out.Bridges) > 0 {
			out.Duplicate = true
			return nil
		}
		var held int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items t WHERE t.id = ? AND t.user_id = ? AND NOT`+notHeld, parentID, u.uid).Scan(&held); err != nil {
			return err
		}
		if held > 0 {
			return ErrAlreadyHeld
		}
		var isBridge int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_bridges WHERE user_id = ? AND bridge_item_id = ? AND ended_at = 0`, u.uid, parentID).Scan(&isBridge); err != nil {
			return err
		}
		if isBridge > 0 {
			return ErrIsBridge
		}
		for pos, c := range cards {
			p, a := strings.TrimSpace(c.Prompt), strings.TrimSpace(c.Answer)
			if p == "" || a == "" {
				return errors.New("a bridge card needs a question and an answer")
			}
			id, err := u.addItem(ctx, tx, NewItem{IdeaID: ideaID, Prompt: p, Answer: a, Source: "bridge"}, now)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO item_bridges (user_id, parent_item_id, bridge_item_id, accept_key, position, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`, u.uid, parentID, id, key, pos+1, ts(now)); err != nil {
				return err
			}
			out.Bridges = append(out.Bridges, id)
		}
		return nil
	})
	if err != nil {
		return BridgeAccept{}, err
	}
	return out, nil
}

// ReleaseHold ends every active bridge of a parent card by hand. The
// bridge cards stay in the library as ordinary cards and the parent's
// schedule is untouched, so it is due again exactly when it was. Releasing
// a card that is not held changes nothing and is not an error; another
// learner's card is ErrNotFound. It returns how many bridges it ended.
//
// Only bridges that hold the card now are ended, by the same predicate as
// the hold (activeBridges) in the same statement: a bridge suspended in
// Trash with its idea is left alone, so a stale release form sent after
// the idea was trashed is a no-op and the hold is back on restore.
func (u *UserStore) ReleaseHold(ctx context.Context, parentID int64, now time.Time) (int, error) {
	var n int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM items WHERE id = ? AND user_id = ?`, parentID, u.uid).Scan(&one); err != nil {
			return notFound(err)
		}
		res, err := tx.ExecContext(ctx, `UPDATE item_bridges SET ended_at = ?, end_reason = 'released'
			WHERE user_id = ? AND parent_item_id = ? AND ended_at = 0
			  AND EXISTS (SELECT 1 FROM items t WHERE t.id = item_bridges.parent_item_id AND t.user_id = item_bridges.user_id
			    AND item_bridges.id IN (SELECT hb.id`+activeBridges+`))`,
			ts(now), u.uid, parentID)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		return nil
	})
	return int(n), err
}

// masterBridge ends the bridge row of an item answered Correct or Almost
// Correct, inside the transaction that records the answer. Once a parent's
// last active bridge is mastered it is no longer held.
func (u *UserStore) masterBridge(ctx context.Context, tx *sql.Tx, itemID int64, at time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE item_bridges SET ended_at = ?, end_reason = 'mastered'
		WHERE user_id = ? AND bridge_item_id = ? AND ended_at = 0`, ts(at), u.uid, itemID)
	return err
}

// endBridgesOf ends, as removed, every active bridge row a removed item
// takes part in: as a bridge (it no longer holds its parent) or as a
// parent (its hold has nothing left to hold).
func (u *UserStore) endBridgesOf(ctx context.Context, tx *sql.Tx, itemID int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE item_bridges SET ended_at = ?, end_reason = 'removed'
		WHERE user_id = ? AND ended_at = 0 AND (bridge_item_id = ? OR parent_item_id = ?)`, ts(now), u.uid, itemID, itemID)
	return err
}

// BridgeStates reports, for each of the given items, how many active
// bridges hold it and which parent it is an active bridge for. Items with
// neither are left out of the map.
func (u *UserStore) BridgeStates(ctx context.Context, itemIDs []int64) (map[int64]BridgeState, error) {
	out := map[int64]BridgeState{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	want := map[int64]bool{}
	for _, id := range itemIDs {
		want[id] = true
	}
	rows, err := u.s.db.QueryContext(ctx, `SELECT hb.parent_item_id, hb.bridge_item_id FROM item_bridges hb
		JOIN items hbi ON hbi.id = hb.bridge_item_id AND hbi.user_id = hb.user_id
		JOIN ideas hbd ON hbd.id = hbi.idea_id AND hbd.user_id = hbi.user_id
		WHERE hb.user_id = ? AND hb.ended_at = 0 AND hbi.deleted_at = 0 AND hbd.deleted_at = 0`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p, b int64
		if err := rows.Scan(&p, &b); err != nil {
			return nil, err
		}
		if want[p] {
			s := out[p]
			s.HeldBy++
			out[p] = s
		}
		if want[b] {
			s := out[b]
			s.BridgeFor = p
			out[b] = s
		}
	}
	return out, rows.Err()
}

// BridgeState reports one item's bridge state.
func (u *UserStore) BridgeState(ctx context.Context, itemID int64) (BridgeState, error) {
	m, err := u.BridgeStates(ctx, []int64{itemID})
	return m[itemID], err
}

// Bridges lists the active bridge cards holding a parent, in order.
func (u *UserStore) Bridges(ctx context.Context, parentID int64) ([]Item, error) {
	return scanItems(u.s.db.QueryContext(ctx, `SELECT `+itemCols+itemFrom+`
		JOIN item_bridges hb ON hb.bridge_item_id = t.id AND hb.user_id = t.user_id
		WHERE t.user_id = ? AND hb.parent_item_id = ? AND hb.ended_at = 0 AND t.deleted_at = 0 AND i.deleted_at = 0
		ORDER BY hb.created_at, hb.position`, u.uid, parentID))
}

// LatestVerdicts returns, for each of the given items, the verdicts of its
// most recent graded reviews (practice attempts left out), newest first, at
// most n of them.
func (u *UserStore) LatestVerdicts(ctx context.Context, itemIDs []int64, n int) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(itemIDs) == 0 || n <= 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(itemIDs)), ",")
	args := []any{u.uid}
	for _, id := range itemIDs {
		args = append(args, id)
	}
	args = append(args, n)
	rows, err := u.s.db.QueryContext(ctx, `SELECT item_id, verdict FROM (
		SELECT item_id, verdict, ROW_NUMBER() OVER (PARTITION BY item_id ORDER BY at DESC, id DESC) AS k
		FROM reviews WHERE user_id = ? AND practice = 0 AND item_id IN (`+ph+`)) WHERE k <= ? ORDER BY item_id, k`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = append(out[id], v)
	}
	return out, rows.Err()
}

// BridgeEnd reports the latest bridge row of an item that is (or was) a
// bridge card: the parent it leads to, why it ended ("" while active) and
// when. An item that was never a bridge reports parent 0.
func (u *UserStore) BridgeEnd(ctx context.Context, bridgeItemID int64) (parent int64, reason string, at time.Time, err error) {
	var ended int64
	err = u.s.db.QueryRowContext(ctx, `SELECT parent_item_id, end_reason, ended_at FROM item_bridges
		WHERE user_id = ? AND bridge_item_id = ? ORDER BY id DESC LIMIT 1`, u.uid, bridgeItemID).Scan(&parent, &reason, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", time.Time{}, nil
	}
	return parent, reason, tm(ended), err
}
