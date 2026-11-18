package store

import (
	"context"
	"time"
)

// Subscription is a learner's plan and allowance.
type Subscription struct {
	UserID          int64
	CustomerRef     string // the processor's customer
	SubscriptionRef string // the processor's subscription
	PaymentRef      string // the processor's saved card
	Status          string // trialing, active, past_due, ended
	TrialEnd        time.Time
	PeriodStart     time.Time
	PeriodEnd       time.Time
	CancelAtEnd     bool
	CardBrand       string
	CardLast4       string
	CardError       string // why the last charge failed, in the processor's words
	AllowanceMicros int64  // this period's allowance
	UsedMicros      int64  // spent from the allowance this period
	TopupMicros     int64  // top-up credit left; it does not expire
	PastDueSince    time.Time
}

const subCols = `user_id, customer_ref, subscription_ref, payment_ref, status, trial_end, period_start, period_end,
	cancel_at_period_end, card_brand, card_last4, card_error, allowance_micros, period_used_micros, topup_micros, past_due_since`

func scanSub(row interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	var trial, ps, pe, pd int64
	var cancel int
	err := row.Scan(&s.UserID, &s.CustomerRef, &s.SubscriptionRef, &s.PaymentRef, &s.Status, &trial, &ps, &pe, &cancel,
		&s.CardBrand, &s.CardLast4, &s.CardError, &s.AllowanceMicros, &s.UsedMicros, &s.TopupMicros, &pd)
	s.TrialEnd, s.PeriodStart, s.PeriodEnd, s.PastDueSince, s.CancelAtEnd = tm(trial), tm(ps), tm(pe), tm(pd), cancel == 1
	return s, notFound(err)
}

// Subscription loads the learner's plan; ErrNotFound if they have none.
func (u *UserStore) Subscription(ctx context.Context) (Subscription, error) {
	return scanSub(u.s.billingDB(ctx).QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE user_id = ?`, u.uid))
}

// SaveSubscription writes the learner's plan.
func (u *UserStore) SaveSubscription(ctx context.Context, s Subscription) error {
	_, err := u.s.billingDB(ctx).ExecContext(ctx, `
		INSERT INTO subscriptions (`+subCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET customer_ref = excluded.customer_ref, subscription_ref = excluded.subscription_ref,
		  payment_ref = excluded.payment_ref, status = excluded.status, trial_end = excluded.trial_end,
		  period_start = excluded.period_start, period_end = excluded.period_end,
		  cancel_at_period_end = excluded.cancel_at_period_end, card_brand = excluded.card_brand, card_last4 = excluded.card_last4,
		  card_error = excluded.card_error, allowance_micros = excluded.allowance_micros,
		  period_used_micros = excluded.period_used_micros, topup_micros = excluded.topup_micros, past_due_since = excluded.past_due_since`,
		u.uid, s.CustomerRef, s.SubscriptionRef, s.PaymentRef, s.Status, ts(s.TrialEnd), ts(s.PeriodStart), ts(s.PeriodEnd),
		b2i(s.CancelAtEnd), s.CardBrand, s.CardLast4, s.CardError, s.AllowanceMicros, s.UsedMicros, s.TopupMicros, ts(s.PastDueSince))
	return err
}

// SubscriptionByRef finds a plan by the processor's subscription id.
func (s *Store) SubscriptionByRef(ctx context.Context, ref string) (Subscription, error) {
	return scanSub(s.billingDB(ctx).QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE subscription_ref = ?`, ref))
}

// SubscriptionsDue lists live plans whose period ended at or before now.
func (s *Store) SubscriptionsDue(ctx context.Context, now time.Time) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE status != 'ended' AND period_end <= ?`, ts(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		sub, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// LedgerEntry is one line of the billing history.
type LedgerEntry struct {
	ID          int64
	At          time.Time
	Kind        string // trial, charge, topup
	Description string
	AmountCents int64
	Status      string // paid, failed, none
	Ref         string // the processor's id; unique, so replays are harmless
}

// AddLedger appends a line to the billing history, once per ref.
func (u *UserStore) AddLedger(ctx context.Context, e LedgerEntry) error {
	_, err := u.s.billingDB(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO ledger (user_id, at, kind, description, amount_cents, status, ref) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.uid, ts(e.At), e.Kind, e.Description, e.AmountCents, e.Status, e.Ref)
	return err
}

// Ledger lists the billing history, newest first.
func (u *UserStore) Ledger(ctx context.Context) ([]LedgerEntry, error) {
	rows, err := u.s.billingDB(ctx).QueryContext(ctx, `
		SELECT id, at, kind, description, amount_cents, status, ref FROM ledger WHERE user_id = ? ORDER BY at DESC, id DESC`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Description, &e.AmountCents, &e.Status, &e.Ref); err != nil {
			return nil, err
		}
		e.At = tm(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Usage is one metered call to an outside model.
type Usage struct {
	Kind         string // judge, draft, speech
	Model        string
	InputTokens  int64
	OutputTokens int64
	Seconds      float64
	CostMicros   int64
}

// Spend records metered usage and draws it from the allowance first, then
// from top-up credit.
func (u *UserStore) Spend(ctx context.Context, use Usage, now time.Time) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		if _, err := u.s.billingDB(ctx).ExecContext(ctx, `
		INSERT INTO usage (user_id, at, kind, model, input_tokens, output_tokens, seconds, cost_micros) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			u.uid, ts(now), use.Kind, use.Model, use.InputTokens, use.OutputTokens, use.Seconds, use.CostMicros); err != nil {
			return err
		}
		if _, err := u.s.billingDB(ctx).ExecContext(ctx, `
		UPDATE subscriptions SET
		  topup_micros = topup_micros - MAX(0, ?1 - MAX(0, allowance_micros - period_used_micros)),
		  period_used_micros = MIN(allowance_micros, period_used_micros + ?1)
		WHERE user_id = ?2`, use.CostMicros, u.uid); err != nil {
			return err
		}
		return nil
	})
}

// CreditTopUp atomically deduplicates a paid operation and grants its credit.
func (u *UserStore) CreditTopUp(ctx context.Context, e LedgerEntry, credit int64) error {
	tx, err := u.s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO ledger (user_id,at,kind,description,amount_cents,status,ref) VALUES (?,?,?,?,?,?,?)", u.uid, ts(e.At), e.Kind, e.Description, e.AmountCents, e.Status, e.Ref)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		if _, err = tx.ExecContext(ctx, "UPDATE subscriptions SET topup_micros=topup_micros+?, card_error='' WHERE user_id=?", credit, u.uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (u *UserStore) HasLedgerRef(ctx context.Context, ref string) (bool, error) {
	var n int
	err := u.s.billingDB(ctx).QueryRowContext(ctx, "SELECT count(*) FROM ledger WHERE user_id=? AND ref=?", u.uid, ref).Scan(&n)
	return n > 0, err
}
