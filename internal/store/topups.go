package store

import (
	"context"
	"errors"
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
  status        TEXT NOT NULL CHECK (status IN ('pending','succeeded','failed')),
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

// Top-up attempt states. An attempt is only ever failed on a definitive
// answer (a decline, or a charge the processor reports as canceled or
// failed). A charge the processor has not shown yet stays pending, however
// long that takes: a late success must still be credited.
const (
	TopupPending   = "pending"
	TopupSucceeded = "succeeded"
	TopupFailed    = "failed"
)

// TopupCap is the number of $5 top-ups allowed per UTC calendar month ($20).
const TopupCap = 4

// ErrTopupCap is the monthly limit.
var ErrTopupCap = errors.New("Monthly $20 top-up limit reached")

// TopupAttempt is one top-up form submission and what became of it.
type TopupAttempt struct {
	UserID       int64
	Token        string
	Key          string // the processor idempotency key
	Status       string
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
// always credited, so an attempt already recorded as failed is corrected
// (its "card failed" history line is replaced by the paid one).
func (u *UserStore) SucceedTopup(ctx context.Context, token, chargeRef, description string, now time.Time) (bool, error) {
	granted := false
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		a, err := u.Topup(ctx, token)
		if err != nil {
			return err
		}
		if a.Status != TopupPending && a.Status != TopupFailed {
			return nil
		}
		db := u.s.billingDB(ctx)
		if _, err := db.ExecContext(ctx, `UPDATE topups SET status='succeeded', charge_ref=?, settled_at=?, reason='' WHERE user_id=? AND token=? AND status=?`,
			chargeRef, ts(now), u.uid, token, a.Status); err != nil {
			return err
		}
		if a.Status == TopupFailed {
			if _, err := db.ExecContext(ctx, `DELETE FROM ledger WHERE user_id=? AND ref=?`, u.uid, a.Key+"-failed"); err != nil {
				return err
			}
		}
		// One processor payment can only ever credit once, whatever the key.
		res, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO ledger (user_id,at,kind,description,amount_cents,status,ref) VALUES (?,?,?,?,?,?,?)",
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
	return granted && err == nil, err
}

// FailTopup releases a pending attempt the processor did not charge, so
// its slot under the monthly cap is free again. A decline reason is kept
// on the card and in the history.
func (u *UserStore) FailTopup(ctx context.Context, token, reason string, now time.Time) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		a, err := u.Topup(ctx, token)
		if err != nil {
			return err
		}
		if a.Status != TopupPending {
			return nil
		}
		db := u.s.billingDB(ctx)
		if _, err := db.ExecContext(ctx, `UPDATE topups SET status='failed', reason=?, settled_at=? WHERE user_id=? AND token=? AND status='pending'`,
			reason, ts(now), u.uid, token); err != nil {
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
