package store

import (
	"context"
	"errors"
	"time"
)

// subscriptionAttemptsSchema is one row per attempt to open a paid or
// card-backed plan at the processor. The row is claimed as pending before
// the processor is called, with everything that call sends (its
// idempotency key and parameters), and settled when the outcome is known.
// subscription_attempts_pending allows a learner one pending attempt at a
// time, whatever the number of requests or processes: while the outcome of
// one is not known, another can only repeat it, with the same key, and
// never start a second subscription.
//
// payment_token, email and trial_end are the parameters sent under idem_key,
// fixed at the claim: the processor refuses a key repeated with different
// parameters, so a repeat sends these and not what the form carries now.
const subscriptionAttemptsSchema = `CREATE TABLE IF NOT EXISTS subscription_attempts (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idem_key         TEXT NOT NULL UNIQUE,
  status           TEXT NOT NULL CHECK (status IN ('pending','succeeded','failed')),
  email            TEXT NOT NULL,
  payment_token    TEXT NOT NULL,
  trial_end        INTEGER NOT NULL DEFAULT 0,
  created_at       INTEGER NOT NULL,
  settled_at       INTEGER NOT NULL DEFAULT 0,
  subscription_ref TEXT NOT NULL DEFAULT ''
)`

const subscriptionAttemptsIndex = `CREATE UNIQUE INDEX IF NOT EXISTS subscription_attempts_pending
  ON subscription_attempts(user_id) WHERE status = 'pending'`

// SubscriptionAttemptStatus is the state of one attempt. The machine is
//
//	pending -> succeeded   the plan it opened is recorded (same transaction)
//	pending -> failed      a definitive refusal, or the processor has nothing
//
// Both ends are final. An attempt whose outcome is not known stays pending.
type SubscriptionAttemptStatus string

const (
	SubscriptionAttemptPending   SubscriptionAttemptStatus = "pending"
	SubscriptionAttemptSucceeded SubscriptionAttemptStatus = "succeeded"
	SubscriptionAttemptFailed    SubscriptionAttemptStatus = "failed"
)

// SubscriptionAttempt is one attempt to subscribe a learner.
type SubscriptionAttempt struct {
	ID              int64
	UserID          int64
	Key             string // the processor idempotency key
	Status          SubscriptionAttemptStatus
	Email           string
	PaymentToken    string
	TrialEnd        time.Time // zero means charge now
	Created         time.Time
	Settled         time.Time
	SubscriptionRef string // the processor's subscription, once succeeded
}

const subscriptionAttemptCols = `id, user_id, idem_key, status, email, payment_token, trial_end, created_at, settled_at, subscription_ref`

func scanSubscriptionAttempt(row interface{ Scan(...any) error }) (SubscriptionAttempt, error) {
	var a SubscriptionAttempt
	var trial, created, settled int64
	err := row.Scan(&a.ID, &a.UserID, &a.Key, &a.Status, &a.Email, &a.PaymentToken, &trial, &created, &settled, &a.SubscriptionRef)
	a.TrialEnd, a.Created, a.Settled = tm(trial), tm(created), tm(settled)
	return a, notFound(err)
}

// ClaimSubscription claims the learner's one pending attempt, in one
// transaction. propose is given the plan row as it is under the write lock
// (the zero Subscription when there is none) and returns the attempt to
// make, or an error that refuses it and is returned unchanged. If an
// earlier attempt is still pending it is returned as it was stored, and
// fresh is false: its key and parameters must be repeated, not replaced.
// Otherwise the proposed attempt is stored as pending. It never talks to
// the processor.
func (u *UserStore) ClaimSubscription(ctx context.Context, propose func(cur Subscription) (SubscriptionAttempt, error)) (att SubscriptionAttempt, fresh bool, err error) {
	err = u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.Subscription(ctx)
		if errors.Is(err, ErrNotFound) {
			cur = Subscription{}
		} else if err != nil {
			return err
		}
		next, err := propose(cur)
		if err != nil {
			return err
		}
		db := u.s.billingDB(ctx)
		att, err = scanSubscriptionAttempt(db.QueryRowContext(ctx,
			`SELECT `+subscriptionAttemptCols+` FROM subscription_attempts WHERE user_id = ? AND status = 'pending'`, u.uid))
		if err == nil || !errors.Is(err, ErrNotFound) {
			return err
		}
		res, err := db.ExecContext(ctx, `INSERT INTO subscription_attempts (user_id, idem_key, status, email, payment_token, trial_end, created_at)
			VALUES (?, ?, 'pending', ?, ?, ?, ?)`, u.uid, next.Key, next.Email, next.PaymentToken, ts(next.TrialEnd), ts(next.Created))
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		att, err = scanSubscriptionAttempt(db.QueryRowContext(ctx,
			`SELECT `+subscriptionAttemptCols+` FROM subscription_attempts WHERE id = ?`, id))
		fresh = err == nil
		return err
	})
	return att, fresh, err
}

// FailSubscriptionAttempt releases a pending attempt the processor
// definitively refused (or has nothing for), so the next one takes a new
// key. It touches nothing else: the learner's plan is exactly as it was.
func (u *UserStore) FailSubscriptionAttempt(ctx context.Context, id int64, now time.Time) error {
	_, err := u.s.billingDB(ctx).ExecContext(ctx,
		`UPDATE subscription_attempts SET status='failed', settled_at=? WHERE id=? AND user_id=? AND status='pending'`, ts(now), id, u.uid)
	return err
}

// CommitSubscription records the plan a pending attempt opened (record)
// and marks the attempt succeeded, in one transaction: the plan is never
// recorded with the claim still pending, nor the claim cleared without the
// plan. record is given the plan row as it is under the write lock (the
// zero Subscription when there is none) and writes the plan; if it fails,
// nothing is written and the attempt stays pending. An attempt that is no
// longer pending was committed by another process: nothing is changed.
func (u *UserStore) CommitSubscription(ctx context.Context, id int64, subscriptionRef string, now time.Time, record func(ctx context.Context, cur Subscription) error) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		res, err := u.s.billingDB(ctx).ExecContext(ctx,
			`UPDATE subscription_attempts SET status='succeeded', settled_at=?, subscription_ref=? WHERE id=? AND user_id=? AND status='pending'`,
			ts(now), subscriptionRef, id, u.uid)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		cur, err := u.Subscription(ctx)
		if errors.Is(err, ErrNotFound) {
			cur = Subscription{}
		} else if err != nil {
			return err
		}
		return record(ctx, cur)
	})
}

// SubscriptionAttempts lists the learner's attempts, oldest first.
func (u *UserStore) SubscriptionAttempts(ctx context.Context) ([]SubscriptionAttempt, error) {
	rows, err := u.s.billingDB(ctx).QueryContext(ctx,
		`SELECT `+subscriptionAttemptCols+` FROM subscription_attempts WHERE user_id = ? ORDER BY id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionAttempt
	for rows.Next() {
		a, err := scanSubscriptionAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
