package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// topupSchema is one row per top-up attempt. A rendered top-up form
// carries an operation token; (user_id, token) identifies the attempt, so
// resubmitting the same form can never become a second charge. The row is
// reserved as pending before the processor is called, and finalized after
// it answers (or after reconciliation, when the answer was lost).
//
// customer_ref and payment_ref are the processor customer and card the
// attempt was charged to, captured in the reservation transaction; they
// never change, so reconciliation does not depend on the learner's current
// (mutable) plan. charge_ref is the processor's id for the charge, kept as
// soon as it is known, even while the attempt is still pending. Those two
// columns are added by topupColumns.
const topupSchema = `CREATE TABLE IF NOT EXISTS topups (
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token         TEXT NOT NULL,
  idem_key      TEXT NOT NULL UNIQUE,
  status        TEXT NOT NULL CHECK (status IN ('pending','succeeded','failed','refund_due')),
  amount_cents  INTEGER NOT NULL,
  credit_micros INTEGER NOT NULL,
  created_at    INTEGER NOT NULL,
  settled_at    INTEGER NOT NULL DEFAULT 0,
  charge_ref    TEXT NOT NULL DEFAULT '',
  reason        TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, token)
)`

// topupColumns are the columns added to topups after it was first created.
var topupColumns = [][2]string{
	{"customer_ref", "TEXT NOT NULL DEFAULT ''"},
	{"payment_ref", "TEXT NOT NULL DEFAULT ''"},
}

// TopupStatus is the state of one top-up attempt. The machine is
//
//	pending   -> succeeded   the processor confirmed the charge (credited)
//	pending   -> failed      a definitive refusal or cancellation (slot freed)
//	failed    -> succeeded   a late authoritative success, with a free slot
//	failed    -> refund_due  a late authoritative success over the month's cap
//
// succeeded and refund_due are terminal; failed only ever moves forward.
// An attempt is only ever failed on a definitive answer (a decline, or a
// charge the processor reports as canceled or failed). A charge the
// processor has not shown yet stays pending, however long that takes: a
// late success must still be credited, or, past the cap, refunded.
type TopupStatus string

const (
	TopupStatusPending   TopupStatus = "pending"
	TopupStatusSucceeded TopupStatus = "succeeded"
	TopupStatusFailed    TopupStatus = "failed"
	// TopupStatusRefundDue is money the processor took after the attempt
	// had failed and its month's cap had since been used up by other
	// top-ups. It is never credited; the charge is owed back.
	TopupStatusRefundDue TopupStatus = "refund_due"
)

// The original names of the top-up states, kept for callers.
const (
	TopupPending   = TopupStatusPending
	TopupSucceeded = TopupStatusSucceeded
	TopupFailed    = TopupStatusFailed
	TopupRefundDue = TopupStatusRefundDue
)

// TopupStatuses lists every top-up state.
var TopupStatuses = []TopupStatus{TopupStatusPending, TopupStatusSucceeded, TopupStatusFailed, TopupStatusRefundDue}

// Valid reports whether s is a known top-up state.
func (s TopupStatus) Valid() bool {
	for _, v := range TopupStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Terminal reports whether no transition leaves s.
func (s TopupStatus) Terminal() bool {
	return s == TopupStatusSucceeded || s == TopupStatusRefundDue
}

// topupTransitions is the whole top-up machine: every legal (from, to).
var topupTransitions = map[TopupStatus][]TopupStatus{
	TopupStatusPending: {TopupStatusSucceeded, TopupStatusFailed},
	TopupStatusFailed:  {TopupStatusSucceeded, TopupStatusRefundDue},
}

// CanTransitionTopup reports whether an attempt in from may move to to.
// Staying in the same state is not a transition.
func CanTransitionTopup(from, to TopupStatus) bool {
	for _, v := range topupTransitions[from] {
		if v == to {
			return true
		}
	}
	return false
}

// TopupTransitionError is a refused top-up transition. It matches
// ErrIllegalTransition.
type TopupTransitionError struct {
	From, To TopupStatus
}

func (e *TopupTransitionError) Error() string {
	return fmt.Sprintf("%v: top-up %s -> %s", ErrIllegalTransition, e.From, e.To)
}

func (e *TopupTransitionError) Is(target error) bool { return target == ErrIllegalTransition }

// TransitionTopup checks one top-up transition: nil when it is legal,
// otherwise a *TopupTransitionError.
func TransitionTopup(from, to TopupStatus) error {
	if CanTransitionTopup(from, to) {
		return nil
	}
	return &TopupTransitionError{From: from, To: to}
}

// TopupCap is the number of $5 top-ups allowed per UTC calendar month ($20).
const TopupCap = 4

// ErrTopupCap is the monthly limit.
var ErrTopupCap = errors.New("Monthly $20 top-up limit reached")

// ErrTopupCapExceededLate is a late authoritative success for an attempt
// that had already failed, arriving when its UTC month's cap is full. The
// attempt is recorded as refund_due with the charge's id, nothing is
// credited (the month never passes $20), and the charge must be refunded.
// It is reported after the refund_due row is committed.
var ErrTopupCapExceededLate = errors.New("top-up charged after the monthly $20 limit was reached; not credited, refund due")

// ErrNoChargeRef is returned when a top-up would be marked paid without
// the processor's id for the payment, from the caller or saved on the
// attempt.
var ErrNoChargeRef = errors.New("top-up settlement has no processor charge id")

// TopupAttempt is one top-up form submission and what became of it.
type TopupAttempt struct {
	UserID       int64
	Token        string
	Key          string // the processor idempotency key
	Status       TopupStatus
	AmountCents  int64
	CreditMicros int64
	Created      time.Time
	Settled      time.Time
	ChargeRef    string // the processor's id for the charge, once known
	Reason       string // the processor's decline sentence, when failed
	CustomerRef  string // the processor customer charged, fixed at reservation
	PaymentRef   string // the card charged, fixed at reservation
}

// MonthBounds is the UTC calendar month containing t. Every top-up cap
// calculation (the charge path and the Billing page) goes through it, so a
// server or learner clock in another zone cannot move the month boundary.
func MonthBounds(t time.Time) (start, next time.Time) {
	u := t.UTC()
	start = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

const topupCols = `user_id, token, idem_key, status, amount_cents, credit_micros, created_at, settled_at, charge_ref, reason, customer_ref, payment_ref`

func scanTopup(row interface{ Scan(...any) error }) (TopupAttempt, error) {
	var t TopupAttempt
	var created, settled int64
	err := row.Scan(&t.UserID, &t.Token, &t.Key, &t.Status, &t.AmountCents, &t.CreditMicros, &created, &settled, &t.ChargeRef, &t.Reason,
		&t.CustomerRef, &t.PaymentRef)
	t.Created, t.Settled = tm(created), tm(settled)
	return t, notFound(err)
}

// Topup loads one of the learner's attempts by its form token.
func (u *UserStore) Topup(ctx context.Context, token string) (TopupAttempt, error) {
	return scanTopup(u.s.billingDB(ctx).QueryRowContext(ctx, `SELECT `+topupCols+` FROM topups WHERE user_id = ? AND token = ?`, u.uid, token))
}

// MonthTopups counts the top-ups that use up this UTC month's cap: paid
// top-ups in the ledger plus attempts still pending with the processor.
// A pending attempt holds its slot until it is finalized, so concurrent
// forms cannot overshoot the cap while a charge is in flight.
func (u *UserStore) MonthTopups(ctx context.Context, now time.Time) (int, error) {
	start, next := MonthBounds(now)
	var paid, pending int
	db := u.s.billingDB(ctx)
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='paid' AND at>=? AND at<?",
		u.uid, ts(start), ts(next)).Scan(&paid); err != nil {
		return 0, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM topups WHERE user_id=? AND status='pending' AND created_at>=? AND created_at<?",
		u.uid, ts(start), ts(next)).Scan(&pending); err != nil {
		return 0, err
	}
	return paid + pending, nil
}

// ReserveTopup is the first short transaction of a top-up. It returns the
// existing attempt for a form token that was already submitted (fresh is
// false). Otherwise, under the same write lock, it reads the learner's plan
// and asks eligible whether it may be charged (an ended plan may not),
// checks the UTC monthly cap, and reserves a new pending attempt recording
// the processor customer and card it will charge. Eligibility, the cap and
// the charge's payment details are therefore one atomic decision: a plan
// ended by a concurrent writer is seen here. ErrNotFound means no plan.
// It never talks to the processor.
func (u *UserStore) ReserveTopup(ctx context.Context, a TopupAttempt, eligible func(Subscription) error) (TopupAttempt, bool, error) {
	var out TopupAttempt
	fresh := false
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		existing, err := u.Topup(ctx, a.Token)
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		sub, err := u.Subscription(ctx)
		if err != nil {
			return err
		}
		if eligible != nil {
			if err := eligible(sub); err != nil {
				return err
			}
		}
		count, err := u.MonthTopups(ctx, a.Created)
		if err != nil {
			return err
		}
		if count >= TopupCap {
			return ErrTopupCap
		}
		a.CustomerRef, a.PaymentRef = sub.CustomerRef, sub.PaymentRef
		if _, err := u.s.billingDB(ctx).ExecContext(ctx, `INSERT INTO topups (`+topupCols+`) VALUES (?, ?, ?, 'pending', ?, ?, ?, 0, '', '', ?, ?)`,
			u.uid, a.Token, a.Key, a.AmountCents, a.CreditMicros, ts(a.Created), a.CustomerRef, a.PaymentRef); err != nil {
			return err
		}
		a.UserID, a.Status = u.uid, TopupPending
		out, fresh = a, true
		return nil
	})
	return out, fresh, err
}

// NoteTopupCharge records the processor's id for a pending attempt's
// charge as soon as it is known (a charge still processing), so later
// lookups and webhooks find it directly.
func (u *UserStore) NoteTopupCharge(ctx context.Context, token, chargeRef string) error {
	if chargeRef == "" {
		return nil
	}
	_, err := u.s.billingDB(ctx).ExecContext(ctx, `UPDATE topups SET charge_ref=? WHERE user_id=? AND token=? AND status='pending' AND charge_ref=''`,
		chargeRef, u.uid, token)
	return err
}

// SucceedTopup is the second short transaction after a confirmed charge:
// the attempt is marked succeeded, its ledger line is written and its
// credit is granted, all at once and only once. The ledger line is dated
// when the attempt was made, so it counts toward the month whose cap it
// reserved. It reports whether credit was granted now.
//
// The processor confirming a payment is authoritative: money it took is
// never silently kept. A pending attempt holds its cap slot, so it is
// always credited. An attempt already recorded as failed gave its slot
// back, so the UTC month's cap is checked again under the write lock: with
// a free slot it is corrected to succeeded and credited (its "card failed"
// history line is replaced by the paid one); with the month full it
// becomes refund_due, nothing is credited, and ErrTopupCapExceededLate is
// returned once that is committed, so ops refund the charge.
// succeeded and refund_due attempts are final: a replay changes nothing.
//
// An empty chargeRef never clears the attempt's saved charge_ref: the
// saved one is used. With neither, nothing is changed and an error is
// returned, so no attempt succeeds and no paid line is booked without the
// processor's id (the ledger's ref is what makes a payment credit once).
func (u *UserStore) SucceedTopup(ctx context.Context, token, chargeRef, description string, now time.Time) (bool, error) {
	granted, overCap := false, false
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		a, err := u.Topup(ctx, token)
		if err != nil {
			return err
		}
		if !CanTransitionTopup(a.Status, TopupStatusSucceeded) {
			return nil // already final: a replay
		}
		if chargeRef == "" {
			chargeRef = a.ChargeRef
		}
		if chargeRef == "" {
			return ErrNoChargeRef
		}
		to := TopupStatusSucceeded
		if a.Status == TopupStatusFailed {
			count, err := u.MonthTopups(ctx, a.Created)
			if err != nil {
				return err
			}
			if count >= TopupCap {
				to = TopupStatusRefundDue
			}
		}
		if err := TransitionTopup(a.Status, to); err != nil {
			return err
		}
		db := u.s.billingDB(ctx)
		res, err := db.ExecContext(ctx, `UPDATE topups SET status=?, charge_ref=?, settled_at=?, reason='' WHERE user_id=? AND token=? AND status=?`,
			to, chargeRef, ts(now), u.uid, token, a.Status)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return &TopupTransitionError{From: a.Status, To: to}
		}
		if a.Status == TopupStatusFailed {
			if _, err := db.ExecContext(ctx, `DELETE FROM ledger WHERE user_id=? AND ref=?`, u.uid, a.Key+"-failed"); err != nil {
				return err
			}
		}
		if to == TopupStatusRefundDue {
			overCap = true
			return nil
		}
		// One processor payment can only ever credit once, whatever the key.
		res, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO ledger (user_id,at,kind,description,amount_cents,status,ref) VALUES (?,?,?,?,?,?,?)",
			u.uid, ts(a.Created), "topup", description, a.AmountCents, "paid", chargeRef)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		if _, err := u.s.MarkEvent(ctx, a.Key, now); err != nil {
			return err
		}
		granted = true
		_, err = db.ExecContext(ctx, "UPDATE subscriptions SET topup_micros=topup_micros+?, card_error='' WHERE user_id=?", a.CreditMicros, u.uid)
		return err
	})
	if err != nil {
		return false, err
	}
	if overCap {
		return false, ErrTopupCapExceededLate
	}
	return granted, nil
}

// FailTopup releases a pending attempt the processor did not charge, so
// its slot under the monthly cap is free again. A decline reason is kept
// on the card and in the history. chargeRef is the processor's id for the
// refused charge, when known: it is recorded unless the attempt already
// has one (an empty chargeRef never clears it), so a later webhook for
// that charge still finds the attempt by id. Any attempt that is not
// pending is left as it is: a failure never undoes a success, a refund
// due, or an earlier failure (an out-of-order or replayed webhook).
func (u *UserStore) FailTopup(ctx context.Context, token, chargeRef, reason string, now time.Time) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		a, err := u.Topup(ctx, token)
		if err != nil {
			return err
		}
		if !CanTransitionTopup(a.Status, TopupStatusFailed) {
			return nil
		}
		db := u.s.billingDB(ctx)
		if _, err := db.ExecContext(ctx, `UPDATE topups SET status='failed', reason=?, settled_at=?,
			charge_ref=CASE WHEN charge_ref='' THEN ? ELSE charge_ref END
			WHERE user_id=? AND token=? AND status='pending'`,
			reason, ts(now), chargeRef, u.uid, token); err != nil {
			return err
		}
		if reason == "" {
			return nil
		}
		if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET card_error=? WHERE user_id=?", reason, u.uid); err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO ledger (user_id,at,kind,description,amount_cents,status,ref) VALUES (?,?,?,?,?,?,?)",
			u.uid, ts(a.Created), "topup", "Top-up (card failed)", a.AmountCents, "failed", a.Key+"-failed")
		return err
	})
}

// RefundDueTopups lists every attempt whose charge must be refunded: money
// the processor took after the attempt had failed and its month was full.
func (s *Store) RefundDueTopups(ctx context.Context) ([]TopupAttempt, error) {
	return queryTopups(ctx, s.billingDB(ctx), `SELECT `+topupCols+` FROM topups WHERE status='refund_due' ORDER BY created_at`)
}

// topupsAllowRefundDue reports whether the topups table's CHECK already
// admits refund_due (a database created before it did needs rebuilding).
func (s *Store) topupsAllowRefundDue() (bool, error) {
	var sql string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='topups'`).Scan(&sql); err != nil {
		return false, err
	}
	return strings.Contains(sql, "'refund_due'"), nil
}

// migrateTopupStatuses rebuilds a topups table created before refund_due
// existed, so its CHECK constraint admits the new state. SQLite cannot
// alter a CHECK in place: the rows are copied, by column name, into a
// table of the current shape inside one transaction, and the old table is
// replaced. Nothing references topups, so foreign keys are unaffected.
// Its indexes are recreated by migrateAdded.
func (s *Store) migrateTopupStatuses() error {
	ok, err := s.topupsAllowRefundDue()
	if err != nil || ok {
		return err
	}
	cols, err := s.columns(context.Background(), s.db, "main", "topups")
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{`DROP TABLE IF EXISTS topups_rebuild`,
		strings.Replace(topupSchema, "CREATE TABLE IF NOT EXISTS topups (", "CREATE TABLE topups_rebuild (", 1)}
	for _, c := range topupColumns {
		stmts = append(stmts, `ALTER TABLE topups_rebuild ADD COLUMN `+c[0]+` `+c[1])
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
	}
	list := strings.Join(quoted, ", ")
	stmts = append(stmts,
		`INSERT INTO topups_rebuild (`+list+`) SELECT `+list+` FROM topups`,
		`DROP TABLE topups`,
		`ALTER TABLE topups_rebuild RENAME TO topups`)
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("rebuild topups: %w", err)
		}
	}
	return tx.Commit()
}

// PendingTopups lists the learner's attempts made since then that are
// still waiting on the processor.
func (u *UserStore) PendingTopups(ctx context.Context, since time.Time) ([]TopupAttempt, error) {
	return queryTopups(ctx, u.s.billingDB(ctx), `SELECT `+topupCols+` FROM topups WHERE user_id=? AND status='pending' AND created_at>=? ORDER BY created_at`,
		u.uid, ts(since))
}

// PendingTopups lists every learner's attempts created between after and
// before that are still pending, for the housekeeping reconciliation sweep.
func (s *Store) PendingTopups(ctx context.Context, after, before time.Time) ([]TopupAttempt, error) {
	return queryTopups(ctx, s.db, `SELECT `+topupCols+` FROM topups WHERE status='pending' AND created_at>=? AND created_at<=? ORDER BY created_at`,
		ts(after), ts(before))
}

// TopupByCharge finds the attempt a processor charge belongs to, by the
// charge id recorded on it or by the idempotency key the charge carries.
// It is for the processor's webhooks, which name no learner.
func (s *Store) TopupByCharge(ctx context.Context, chargeRef, key string) (TopupAttempt, error) {
	return scanTopup(s.billingDB(ctx).QueryRowContext(ctx, `SELECT `+topupCols+` FROM topups
		WHERE (charge_ref != '' AND charge_ref = ?) OR (idem_key != '' AND idem_key = ?) LIMIT 1`, chargeRef, key))
}

func queryTopups(ctx context.Context, db billingQueries, q string, args ...any) ([]TopupAttempt, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopupAttempt
	for rows.Next() {
		a, err := scanTopup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
