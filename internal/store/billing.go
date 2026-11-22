package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SubscriptionStatus is the state of a learner's plan. The machine is
//
//	(none)            -> trialing | active    OpenPlan (StartTrial), OpenNoCardTrial
//	trialing|active|past_due -> active        an invoice was paid
//	trialing|active|past_due -> past_due      an invoice failed
//	trialing|active|past_due -> ended         the plan ended
//	trialing (no card) -> trialing (card)     AddCardToTrial: same trial end
//	trialing|ended (no card) -> active        StartPaidFromNoCard: charged today
//	trialing (no card) -> ended               the no-card trial ran out (period_end)
//	ended             -> active               OpenPlan with a NEW subscription_ref
//
// ended is terminal for its subscription_ref: no processor event moves it
// again. Only an explicit OpenPlan, which opens a new subscription at the
// processor, starts another plan.
//
// A no-card trial is a trialing row with no processor customer
// (customer_ref ”): the 7-day trial every new account starts with. It
// exists only in Scry (no processor subscription, so no invoice or end
// event ever names it), it can never be charged or topped up, and it gives
// access only while now < period_end. Reserve refuses it from period_end
// on, in the same atomic statement as the hold, whether or not the row has
// been moved to ended yet.
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
	SubEventOpenTrial    // the first plan, as a trial
	SubEventOpenPaid     // a plan charged at once (after an ended one)
	SubEventAddCard      // a card added to a no-card trial: it keeps its end
	SubEventTrialExpired // a no-card trial reached its period_end
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
	case SubEventAddCard:
		return "add_card"
	case SubEventTrialExpired:
		return "trial_expired"
	}
	return fmt.Sprintf("SubscriptionEvent(%d)", uint8(e))
}

// SubscriptionEvents lists every subscription event.
var SubscriptionEvents = []SubscriptionEvent{SubEventInvoicePaid, SubEventInvoiceFailed, SubEventEnded, SubEventOpenTrial, SubEventOpenPaid, SubEventAddCard, SubEventTrialExpired}

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

	// Only a no-card trial takes these two; OpenPlan, AddCardToTrial and
	// EndExpiredNoCardTrial check customer_ref '' on the row.
	{SubTrialing, SubEventAddCard}:      SubTrialing,
	{SubTrialing, SubEventTrialExpired}: SubEnded,

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
	// OverageMicros is this period's metered cost that no hold covered:
	// what settles wrote off because a call cost more than it reserved
	// (Settle debits at most the hold). It is never charged. On a no-card
	// trial it counts against the allowance at admission (ReserveAt), so
	// once the trial's actual cost reaches its allowance no further hold
	// is taken. A new period starts it at zero.
	OverageMicros int64
}

// NoCard reports whether the plan has no processor customer behind it: a
// no-card trial, or one that has ended. Nothing about it may be sent to
// the card processor.
func (s Subscription) NoCard() bool { return s.CustomerRef == "" }

// NoCardTrial reports whether the plan is a no-card trial still trialing
// (whether or not its period_end has passed yet).
func (s Subscription) NoCardTrial() bool { return s.Status == SubTrialing && s.CustomerRef == "" }

// NoCardTrialRef is the subscription_ref of a learner's no-card trial.
// It names no processor object.
func NoCardTrialRef(userID int64) string { return fmt.Sprintf("trial-nocard-%d", userID) }

const subCols = `user_id, customer_ref, subscription_ref, payment_ref, status, trial_end, period_start, period_end,
	cancel_at_period_end, card_brand, card_last4, card_error, allowance_micros, period_used_micros, topup_micros, past_due_since,
	period_overage_micros`

func scanSub(row interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	var trial, ps, pe, pd int64
	var cancel int
	err := row.Scan(&s.UserID, &s.CustomerRef, &s.SubscriptionRef, &s.PaymentRef, &s.Status, &trial, &ps, &pe, &cancel,
		&s.CardBrand, &s.CardLast4, &s.CardError, &s.AllowanceMicros, &s.UsedMicros, &s.TopupMicros, &pd, &s.OverageMicros)
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
		INSERT INTO subscriptions (`+subCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET customer_ref = excluded.customer_ref, subscription_ref = excluded.subscription_ref,
		  payment_ref = excluded.payment_ref, status = excluded.status, trial_end = excluded.trial_end,
		  period_start = excluded.period_start, period_end = excluded.period_end,
		  cancel_at_period_end = excluded.cancel_at_period_end, card_brand = excluded.card_brand, card_last4 = excluded.card_last4,
		  card_error = excluded.card_error, allowance_micros = excluded.allowance_micros,
		  period_used_micros = excluded.period_used_micros, topup_micros = excluded.topup_micros, past_due_since = excluded.past_due_since,
		  period_overage_micros = excluded.period_overage_micros`,
		u.uid, s.CustomerRef, s.SubscriptionRef, s.PaymentRef, s.Status, ts(s.TrialEnd), ts(s.PeriodStart), ts(s.PeriodEnd),
		b2i(s.CancelAtEnd), s.CardBrand, s.CardLast4, s.CardError, s.AllowanceMicros, s.UsedMicros, s.TopupMicros, ts(s.PastDueSince), s.OverageMicros)
	return err
}

// SubscriptionByRef finds a plan by the processor's subscription id.
func (s *Store) SubscriptionByRef(ctx context.Context, ref string) (Subscription, error) {
	return scanSub(s.billingDB(ctx).QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE subscription_ref = ?`, ref))
}

// SubscriptionsDue lists live plans with a processor customer whose
// period ended at or before now. No-card trials are never renewed: they
// end (EndExpiredNoCardTrials).
func (s *Store) SubscriptionsDue(ctx context.Context, now time.Time) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE status != 'ended' AND customer_ref != '' AND period_end <= ?`, ts(now))
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

// OpenNoCardTrial gives a learner with no plan at all the no-card trial:
// trialing, no processor customer, allowance micro-dollars of metered
// usage, from now until end (trial_end = period_end = end), with its
// zero-amount history line. It reports whether it opened one; a learner
// who already has a plan row (live or ended) gets nothing, so the trial
// is had once.
func (u *UserStore) OpenNoCardTrial(ctx context.Context, now, end time.Time, allowance int64, line string) (bool, error) {
	opened := false
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		if _, err := u.Subscription(ctx); err == nil {
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		to, err := TransitionSubscription(SubNone, SubEventOpenTrial)
		if err != nil {
			return err
		}
		ref := NoCardTrialRef(u.uid)
		if err := u.SaveSubscription(ctx, Subscription{UserID: u.uid, SubscriptionRef: ref, Status: to,
			TrialEnd: end, PeriodStart: now, PeriodEnd: end, AllowanceMicros: allowance}); err != nil {
			return err
		}
		opened = true
		return u.AddLedger(ctx, LedgerEntry{At: now, Kind: "trial", Description: line, Status: "none", Ref: "trial-" + ref})
	})
	return opened, err
}

// ErrNotNoCardTrial is a card added to a plan that is not (or no longer)
// a live no-card trial.
var ErrNotNoCardTrial = errors.New("plan is not a no-card trial")

// AddCardToTrial puts a processor subscription behind the learner's
// no-card trial (SubEventAddCard: trialing -> trialing). The trial keeps
// its end, its start and what it has spent; it takes the processor's
// customer, subscription, card and period end, and its allowance rises to
// at least minAllowance. It refuses with ErrNotNoCardTrial, changing
// nothing, when the row is not a no-card trial whose period_end is after
// now (it expired, ended, or already has a card).
func (u *UserStore) AddCardToTrial(ctx context.Context, next Subscription, minAllowance int64, now time.Time) (Subscription, error) {
	var out Subscription
	err := u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.Subscription(ctx)
		if err != nil {
			return err
		}
		if !cur.NoCardTrial() || !now.Before(cur.PeriodEnd) {
			return ErrNotNoCardTrial
		}
		to, err := TransitionSubscription(cur.Status, SubEventAddCard)
		if err != nil {
			return err
		}
		if next.CustomerRef == "" || next.SubscriptionRef == "" || next.SubscriptionRef == cur.SubscriptionRef {
			return errors.New("a card added to a trial needs a processor customer and subscription")
		}
		cur.Status = to
		cur.CustomerRef, cur.SubscriptionRef, cur.PaymentRef = next.CustomerRef, next.SubscriptionRef, next.PaymentRef
		cur.CardBrand, cur.CardLast4, cur.CardError = next.CardBrand, next.CardLast4, ""
		if !next.PeriodEnd.IsZero() {
			cur.PeriodEnd = next.PeriodEnd
		}
		cur.AllowanceMicros = max(cur.AllowanceMicros, minAllowance)
		out = cur
		return u.SaveSubscription(ctx, cur)
	})
	return out, err
}

// StartPaidFromNoCard replaces the learner's no-card plan, a trial still
// trialing (in its last hour) or one that has ended, with the paid plan
// next (status active, charged today), in one transaction: trialing ->
// active is SubEventInvoicePaid, ended -> active SubEventOpenPaid. The
// new period starts afresh (nothing used, no overage); top-up credit and
// the trial's end date carry over. It refuses with ErrNotNoCardTrial,
// changing nothing, when the row is not a no-card plan any more (a card
// was added in between). Call it only after the processor took the card.
func (u *UserStore) StartPaidFromNoCard(ctx context.Context, next Subscription) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.Subscription(ctx)
		if err != nil {
			return err
		}
		if !cur.NoCard() || (cur.Status != SubTrialing && cur.Status != SubEnded) {
			return ErrNotNoCardTrial
		}
		ev := SubEventOpenPaid
		if cur.Status == SubTrialing {
			ev = SubEventInvoicePaid
		}
		to, err := TransitionSubscription(cur.Status, ev)
		if err != nil {
			return err
		}
		if to != SubActive || next.CustomerRef == "" || next.SubscriptionRef == "" || next.SubscriptionRef == cur.SubscriptionRef {
			return errors.New("a paid plan from a no-card trial needs a processor customer and a new subscription")
		}
		next.UserID, next.Status = u.uid, to
		next.UsedMicros, next.OverageMicros, next.PastDueSince = 0, 0, time.Time{}
		next.TopupMicros = cur.TopupMicros
		if next.TrialEnd.IsZero() {
			next.TrialEnd = cur.TrialEnd
		}
		return u.SaveSubscription(ctx, next)
	})
}

// EndExpiredNoCardTrial ends the learner's no-card trial if its
// period_end is at or before now (SubEventTrialExpired: trialing ->
// ended), in one guarded statement, and reports whether it did.
func (u *UserStore) EndExpiredNoCardTrial(ctx context.Context, now time.Time) (bool, error) {
	if _, err := TransitionSubscription(SubTrialing, SubEventTrialExpired); err != nil {
		return false, err
	}
	res, err := u.s.billingDB(ctx).ExecContext(ctx, `UPDATE subscriptions SET status='ended', cancel_at_period_end=0
		WHERE user_id=? AND status='trialing' AND customer_ref='' AND period_end<=?`, u.uid, now.Unix())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// EndExpiredNoCardTrials is the housekeeping sweep of
// EndExpiredNoCardTrial over every learner; it reports how many ended.
func (s *Store) EndExpiredNoCardTrials(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.billingDB(ctx).ExecContext(ctx, `UPDATE subscriptions SET status='ended', cancel_at_period_end=0
		WHERE status='trialing' AND customer_ref='' AND period_end<=?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
