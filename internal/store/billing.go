package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SubscriptionStatus is the state of a learner's plan. The machine is
//
//	(none)            -> trialing | active    OpenPlan (StartTrial)
//	trialing|active|past_due -> active        an invoice was paid
//	trialing|active|past_due -> past_due      an invoice failed
//	trialing|active|past_due -> ended         the plan ended
//	ended             -> active               OpenPlan with a NEW subscription_ref
//
// ended is terminal for its subscription_ref: no processor event moves it
// again. Only an explicit OpenPlan, which opens a new subscription at the
// processor, starts another plan.
type SubscriptionStatus string

const (
	SubNone     SubscriptionStatus = "" // no plan row
	SubTrialing SubscriptionStatus = "trialing"
	SubActive   SubscriptionStatus = "active"
	SubPastDue  SubscriptionStatus = "past_due"
	SubEnded    SubscriptionStatus = "ended"
)

// SubscriptionStatuses lists every state a plan row can be in.
var SubscriptionStatuses = []SubscriptionStatus{SubTrialing, SubActive, SubPastDue, SubEnded}

// Live reports whether the plan is open at the processor (not ended).
func (s SubscriptionStatus) Live() bool {
	return s == SubTrialing || s == SubActive || s == SubPastDue
}

// SubscriptionEvent is what moves a plan: the processor's invoice and end
// events (billing.InvoicePaid, InvoiceFailed, SubscriptionEnded, mapped by
// the app) and the explicit opening of a plan.
type SubscriptionEvent uint8

const (
	SubEventInvoicePaid SubscriptionEvent = iota
	SubEventInvoiceFailed
	SubEventEnded
	SubEventOpenTrial // the first plan, as a trial
	SubEventOpenPaid  // a plan charged at once (after an ended one)
)

func (e SubscriptionEvent) String() string {
	switch e {
	case SubEventInvoicePaid:
		return "invoice_paid"
	case SubEventInvoiceFailed:
		return "invoice_failed"
	case SubEventEnded:
		return "subscription_ended"
	case SubEventOpenTrial:
		return "open_trial"
	case SubEventOpenPaid:
		return "open_paid"
	}
	return fmt.Sprintf("SubscriptionEvent(%d)", uint8(e))
}

// SubscriptionEvents lists every subscription event.
var SubscriptionEvents = []SubscriptionEvent{SubEventInvoicePaid, SubEventInvoiceFailed, SubEventEnded, SubEventOpenTrial, SubEventOpenPaid}

type subStep struct {
	from SubscriptionStatus
	ev   SubscriptionEvent
}

// subscriptionTransitions is the whole plan machine. A pair that is not
// here is illegal. ended -> ended is the idempotent end of a plan already
// over; it never leaves ended.
var subscriptionTransitions = map[subStep]SubscriptionStatus{
	{SubNone, SubEventOpenTrial}: SubTrialing,
	{SubNone, SubEventOpenPaid}:  SubActive,
	{SubEnded, SubEventOpenPaid}: SubActive,
	{SubEnded, SubEventEnded}:    SubEnded,

	{SubTrialing, SubEventInvoicePaid}:   SubActive,
	{SubTrialing, SubEventInvoiceFailed}: SubPastDue,
	{SubTrialing, SubEventEnded}:         SubEnded,
	{SubActive, SubEventInvoicePaid}:     SubActive,
	{SubActive, SubEventInvoiceFailed}:   SubPastDue,
	{SubActive, SubEventEnded}:           SubEnded,
	{SubPastDue, SubEventInvoicePaid}:    SubActive,
	{SubPastDue, SubEventInvoiceFailed}:  SubPastDue,
	{SubPastDue, SubEventEnded}:          SubEnded,
}

// SubscriptionTransitionError is a refused plan transition. It matches
// ErrIllegalTransition.
type SubscriptionTransitionError struct {
	From  SubscriptionStatus
	Event SubscriptionEvent
}

func (e *SubscriptionTransitionError) Error() string {
	from := string(e.From)
	if from == "" {
		from = "none"
	}
	return fmt.Sprintf("%v: plan %s on %s", ErrIllegalTransition, from, e.Event)
}

func (e *SubscriptionTransitionError) Is(target error) bool { return target == ErrIllegalTransition }

// TransitionSubscription is the state an event moves a plan to, or a
// *SubscriptionTransitionError when the plan cannot take it (an invoice
// for a plan that has ended, opening a plan over a live one).
func TransitionSubscription(from SubscriptionStatus, ev SubscriptionEvent) (SubscriptionStatus, error) {
	if to, ok := subscriptionTransitions[subStep{from, ev}]; ok {
		return to, nil
	}
	return from, &SubscriptionTransitionError{From: from, Event: ev}
}

// ErrNegativeCost is metered usage with a negative price.
var ErrNegativeCost = errors.New("usage cost is negative")

// Subscription is a learner's plan and allowance.
type Subscription struct {
	UserID          int64
	CustomerRef     string             // the processor's customer
	SubscriptionRef string             // the processor's subscription
	PaymentRef      string             // the processor's saved card
	Status          SubscriptionStatus // trialing, active, past_due, ended
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
// from top-up credit. The draw is clamped: the allowance never goes past
// its size and top-up credit never below zero, even when the real cost of
// a call is more than what was left (an unpriced model charged at the top
// rate, a long recording). The usage row keeps the full cost.
func (u *UserStore) Spend(ctx context.Context, use Usage, now time.Time) error {
	if use.CostMicros < 0 {
		return ErrNegativeCost
	}
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		if _, err := u.s.billingDB(ctx).ExecContext(ctx, `
		INSERT INTO usage (user_id, at, kind, model, input_tokens, output_tokens, seconds, cost_micros) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			u.uid, ts(now), use.Kind, use.Model, use.InputTokens, use.OutputTokens, use.Seconds, use.CostMicros); err != nil {
			return err
		}
		if _, err := u.s.billingDB(ctx).ExecContext(ctx, `
		UPDATE subscriptions SET
		  topup_micros = MAX(0, topup_micros - MAX(0, ?1 - MAX(0, allowance_micros - period_used_micros))),
		  period_used_micros = MAX(0, MIN(allowance_micros, period_used_micros + ?1))
		WHERE user_id = ?2`, use.CostMicros, u.uid); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) HasProcessedEvent(ctx context.Context, key string) (bool, error) {
	var n int
	err := s.billingDB(ctx).QueryRowContext(ctx, "SELECT count(*) FROM processed_events WHERE id=?", key).Scan(&n)
	return n > 0, err
}

// ChangeSubscription reads the latest row under the same write transaction
// as its update, preserving concurrent usage, credits and unrelated fields.
func (u *UserStore) ChangeSubscription(ctx context.Context, change func(*Subscription)) (Subscription, error) {
	var sub Subscription
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		var err error
		sub, err = u.Subscription(ctx)
		if err != nil {
			return err
		}
		change(&sub)
		return u.SaveSubscription(ctx, sub)
	})
	return sub, err
}

// OpenPlan only creates a missing plan or replaces an ended one with a new
// subscription (TransitionSubscription: none -> trialing|active, ended ->
// active). Credits are copied from the current row, never from a
// pre-provider stale snapshot.
func (u *UserStore) OpenPlan(ctx context.Context, next Subscription) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		current, err := u.Subscription(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		from := SubNone
		if err == nil {
			from = current.Status
		}
		ev := SubEventOpenPaid
		if next.Status == SubTrialing {
			ev = SubEventOpenTrial
		}
		to, terr := TransitionSubscription(from, ev)
		if terr != nil {
			return nil // already on a live plan: nothing to open
		}
		if err == nil {
			if next.SubscriptionRef != "" && next.SubscriptionRef == current.SubscriptionRef {
				return nil // an ended subscription is never reopened in place
			}
			next.TopupMicros = current.TopupMicros
		}
		next.Status = to
		return u.SaveSubscription(ctx, next)
	})
}
