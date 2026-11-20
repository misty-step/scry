package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"scry/internal/ai"
	"scry/internal/billing"
	"scry/internal/store"
)

// Errors about the plan that pages turn into words.
var (
	ErrNoPlan    = errors.New("no plan")           // signed up but never started the trial
	ErrPlanEnded = errors.New("plan has ended")    // cancelled or lapsed
	ErrAllowance = errors.New("allowance reached") // nothing left to spend this month
)

// rate is a model's price in micro-dollars per token.
type rate struct{ in, out float64 }

// rates is what the provider charges, per token. Usage is metered at
// cost; the margin lives in the plan's fixed prices.
var rates = map[string]rate{
	"claude-sonnet-5-5": {2, 10},
	"claude-opus-5-5":   {4, 20},
	"claude-fable-5-2":  {8, 40},
	"test-judge":        {0.10, 0.50},
	"test-draft":        {4, 20},
}

// speechMicrosPerSecond is the price of transcription ($0.003 a minute).
const speechMicrosPerSecond = 50

// Cost prices one model call in micro-dollars. A model Scry has no price
// for is charged at the highest rate it knows, so costs cannot run away.
func Cost(u ai.Usage) int64 {
	if u.CostMicros != nil {
		return *u.CostMicros
	}
	r, ok := rates[u.Model]
	if !ok {
		r = rates["claude-fable-5-2"]
	}
	return int64(math.Ceil(float64(u.InputTokens)*r.in + float64(u.OutputTokens)*r.out))
}

// DraftsPerAllowance estimates how many drafts the monthly allowance
// buys, so Billing can say what the allowance covers in plain terms.
func (c Config) DraftsPerAllowance() int {
	typical := Cost(ai.Usage{Model: ai.DefaultDraftModel, InputTokens: 1500, OutputTokens: 6000})
	return int(c.AllowanceMicros / typical)
}

// HasAccess reports whether a plan lets the learner use Scry now. There
// is no free tier: no plan, no access.
func (a *App) HasAccess(s store.Subscription) bool {
	switch s.Status {
	case "trialing", "active":
		return true
	case "past_due":
		return a.now().Sub(s.PastDueSince) <= time.Duration(a.Cfg.GraceDays)*24*time.Hour
	}
	return false
}

// CanSpend reports whether there is allowance or top-up credit left.
func CanSpend(s store.Subscription) bool {
	return s.UsedMicros < s.AllowanceMicros || s.TopupMicros > 0
}

// UsedPercent is how much of this period's allowance is spent, 0 to 100.
func UsedPercent(s store.Subscription) int {
	if s.AllowanceMicros <= 0 {
		return 100
	}
	p := int(s.UsedMicros * 100 / s.AllowanceMicros)
	return min(max(p, 0), 100)
}

// Plan loads the learner's plan, first bringing it up to date if its
// period has run out. It returns ErrNoPlan or ErrPlanEnded when the
// learner has no access.
func (a *App) Plan(ctx context.Context, userID int64) (store.Subscription, error) {
	us := a.Store.For(userID)
	sub, err := us.Subscription(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return sub, ErrNoPlan
	}
	if err != nil {
		return sub, err
	}
	if _, ok := a.Pay.(billing.Renewer); ok && sub.Status != "ended" && !sub.PeriodEnd.After(a.now()) {
		if err := a.renew(ctx, sub); err != nil {
			return sub, err
		}
		if sub, err = us.Subscription(ctx); err != nil {
			return sub, err
		}
	}
	if !a.HasAccess(sub) {
		return sub, ErrPlanEnded
	}
	return sub, nil
}

// StartTrial opens the 7-day trial with a card up front. A learner whose
// plan has ended starts again without a trial and is charged at once.
func (a *App) StartTrial(ctx context.Context, u store.User, cardToken string) error {
	us := a.Store.For(u.ID)
	now := a.now()
	old, err := us.Subscription(ctx)
	hadPlan := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if hadPlan && old.Status != "ended" {
		return nil // already on a plan
	}
	p := billing.SubscribeParams{Email: u.Email, CardToken: cardToken, Key: fmt.Sprintf("sub-%d-%d", u.ID, now.Unix()/60), Now: now}
	if !hadPlan {
		p.TrialEnd = now.AddDate(0, 0, a.Cfg.TrialDays)
	}
	res, err := a.Pay.Subscribe(ctx, p)
	if err != nil {
		var d *billing.DeclineError
		if !errors.As(err, &d) {
			a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		}
		return err
	}
	sub := store.Subscription{
		UserID: u.ID, CustomerRef: res.CustomerRef, SubscriptionRef: res.SubscriptionRef, PaymentRef: res.PaymentRef,
		CardBrand: res.Card.Brand, CardLast4: res.Card.Last4, PeriodStart: now, PeriodEnd: res.PeriodEnd,
		TopupMicros: old.TopupMicros,
	}
	if hadPlan {
		sub.Status, sub.AllowanceMicros, sub.TrialEnd = "active", a.Cfg.AllowanceMicros, old.TrialEnd
		if err := us.OpenPlan(ctx, sub); err != nil {
			return err
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: now, Kind: "charge", Description: "Scry, one month", AmountCents: a.Cfg.PriceCents, Status: "paid", Ref: res.ChargeRef})
	}
	sub.Status, sub.AllowanceMicros, sub.TrialEnd = "trialing", a.Cfg.TrialMicros, p.TrialEnd
	if err := us.OpenPlan(ctx, sub); err != nil {
		return err
	}
	return us.AddLedger(ctx, store.LedgerEntry{At: now, Kind: "trial", Description: fmt.Sprintf("%d-day trial started", a.Cfg.TrialDays), Status: "none", Ref: "trial-" + res.SubscriptionRef})
}

// Top-up errors the Billing page turns into words.
var (
	// ErrTopupToken is a top-up submitted without a valid form token.
	ErrTopupToken = errors.New("top-up form token is missing or invalid")
	// ErrTopupPending is a top-up whose charge has not been confirmed yet
	// (still in flight, still processing, or the processor's answer was
	// lost). It is settled by reconciliation or by the processor's webhook
	// and is never charged twice.
	ErrTopupPending = errors.New("top-up is still being confirmed")
	// ErrTopupUnconfirmed is a pending top-up the processor still shows no
	// charge for, well after it was sent. That is not proof that nothing was
	// or will be taken (the processor may not show it yet): it stays pending
	// (and keeps its place under the monthly cap) so a late charge is still
	// credited.
	ErrTopupUnconfirmed = errors.New("top-up could not be confirmed with the card processor yet; whether it was charged is not known")
	// ErrTopupReleased is a top-up the processor reports it did not take
	// (canceled); nothing was charged and its place under the monthly cap
	// is free again.
	ErrTopupReleased = errors.New("top-up was not charged by the card processor")
)

// TopupSettleWindow is how long after an attempt began the processor is
// expected to have answered. After it, a lookup that still finds nothing
// is reported as unconfirmed rather than pending; it is still never taken
// as proof that nothing was charged, because a processor can recover a
// request and create the charge later.
const TopupSettleWindow = 2 * time.Minute

// TopupWatchWindow is how long Billing and the sweep keep asking the
// processor about an attempt that is still pending. Beyond it the attempt
// stays pending and is settled by the processor's webhook.
const TopupWatchWindow = 35 * 24 * time.Hour

// topupChargeTimeout bounds one processor call for a top-up.
const topupChargeTimeout = 30 * time.Second

var tokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// NewTopupToken is a fresh operation token for one rendered top-up form.
func NewTopupToken() string { return randomToken(18) }

func topupKey(userID int64, token string) string { return fmt.Sprintf("topup-%d-%s", userID, token) }

// TopUp charges the card on file for one top-up and adds its credit.
// token identifies the form (one top-up intent): the same token never
// charges or credits twice, however many times or however far apart it
// is submitted.
func (a *App) TopUp(ctx context.Context, userID int64, token string) error {
	_, err := a.TopUpResult(ctx, userID, token)
	return err
}

// TopUpResult reports whether this request actually granted credit.
//
// The SQLite write lock is never held across the processor call:
//  1. a short transaction checks the token, that the plan may be charged,
//     and the UTC monthly cap, and reserves a pending attempt recording the
//     processor customer and card it will charge;
//  2. that customer and card are charged outside any transaction, with an
//     idempotency key scoped to the learner and the form token;
//  3. a short transaction finalizes the attempt: succeeded with credit,
//     failed and released on a decline, or left pending (with the charge's
//     id when known) for reconciliation and webhooks when the outcome is
//     unknown or still processing.
func (a *App) TopUpResult(ctx context.Context, userID int64, token string) (bool, error) {
	if !tokenRE.MatchString(token) {
		return false, ErrTopupToken
	}
	key := topupKey(userID, token)
	deadline := time.NewTimer(topupChargeTimeout + 5*time.Second)
	defer deadline.Stop()
	for {
		release, waiting := a.claimTopup(key)
		if waiting == nil {
			defer release()
			return a.topUpClaimed(ctx, userID, token, key)
		}
		// The same form is being charged by another request right now (a
		// double click): wait for it, then report what it did.
		select {
		case <-waiting:
		case <-ctx.Done():
			return false, ErrTopupPending
		case <-deadline.C:
			return false, ErrTopupPending
		}
	}
}

// claimTopup makes this request the only one in the process working on a
// top-up key. It returns either a release function, or a channel that
// closes when the current holder is done.
func (a *App) claimTopup(key string) (func(), <-chan struct{}) {
	ch := make(chan struct{})
	if held, loaded := a.topupsInFlight.LoadOrStore(key, ch); loaded {
		return nil, held.(chan struct{})
	}
	return func() { a.topupsInFlight.Delete(key); close(ch) }, nil
}

// topupEligible is the plan check made inside the reservation transaction,
// against the plan row as it is under the write lock.
func (a *App) topupEligible(sub store.Subscription) error {
	if !a.HasAccess(sub) {
		return ErrPlanEnded
	}
	return nil
}

func (a *App) topUpClaimed(ctx context.Context, userID int64, token, key string) (bool, error) {
	// Brings a plan whose period ran out up to date, and refuses early.
	// This read is not the authorization: that is repeated inside TX1.
	if _, err := a.Plan(ctx, userID); err != nil {
		return false, err
	}
	us := a.Store.For(userID)
	att, fresh, err := us.ReserveTopup(ctx, store.TopupAttempt{Token: token, Key: key, AmountCents: a.Cfg.TopupCents,
		CreditMicros: a.Cfg.TopupMicros, Created: a.now()}, a.topupEligible)
	if errors.Is(err, store.ErrNotFound) {
		return false, ErrNoPlan
	}
	if err != nil {
		return false, err
	}
	if !fresh {
		return a.settleExisting(ctx, us, att)
	}
	// The charge and what follows are not cut short by the browser going
	// away: a charge abandoned half way is exactly what must not happen.
	// The customer and card are the ones captured with the reservation.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), topupChargeTimeout)
	ref, err := a.Pay.Charge(cctx, att.CustomerRef, att.PaymentRef, att.AmountCents, "Scry top-up", key)
	cancel()
	return a.finishCharge(context.WithoutCancel(ctx), us, att, ref, err)
}

// finishCharge is the second short transaction, after the processor answered.
func (a *App) finishCharge(ctx context.Context, us *store.UserStore, att store.TopupAttempt, ref string, err error) (bool, error) {
	var d *billing.DeclineError
	var p *billing.PendingError
	switch {
	case errors.As(err, &d):
		// A final refusal. The refused PaymentIntent's id, when the
		// processor named it, is kept on the failed attempt.
		charge := d.Ref
		if charge == "" {
			charge = ref
		}
		if ferr := us.FailTopup(ctx, att.Token, charge, d.Reason, a.now()); ferr != nil {
			return false, ferr
		}
		return false, err
	case errors.As(err, &p):
		// Accepted, not finished: neither paid nor declined. Keep the
		// charge's id so the lookup (or its webhook) settles it.
		if nerr := us.NoteTopupCharge(ctx, att.Token, p.Ref); nerr != nil {
			return false, nerr
		}
		a.Log.InfoContext(ctx, "top-up still processing; kept pending", "key", att.Key, "charge", p.Ref)
		return false, ErrTopupPending
	case err != nil:
		// Indeterminate: the charge may or may not have happened. Keep the
		// attempt pending (it still holds its cap slot) and reconcile later,
		// by the intent's id if the processor's answer named one.
		var pe *billing.ProcessorError
		if errors.As(err, &pe) && pe.Ref != "" {
			if nerr := us.NoteTopupCharge(ctx, att.Token, pe.Ref); nerr != nil {
				return false, nerr
			}
		}
		a.Log.WarnContext(ctx, "top-up outcome unknown; kept pending for reconciliation", "err", err, "key", att.Key)
		a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		return false, ErrTopupPending
	}
	return a.succeed(ctx, us, att, ref)
}

// succeed credits a confirmed charge. If the processor's webhook already
// credited this very charge, the learner's request still succeeded.
func (a *App) succeed(ctx context.Context, us *store.UserStore, att store.TopupAttempt, ref string) (bool, error) {
	granted, err := us.SucceedTopup(ctx, att.Token, ref, "Top-up", a.now())
	if err != nil || granted || att.Status != store.TopupPending {
		return granted, err
	}
	if cur, cerr := us.Topup(ctx, att.Token); cerr == nil && cur.Status == store.TopupSucceeded && cur.ChargeRef == ref {
		return true, nil
	}
	return false, nil
}

// settleExisting answers a form token that was already submitted.
func (a *App) settleExisting(ctx context.Context, us *store.UserStore, att store.TopupAttempt) (bool, error) {
	switch att.Status {
	case store.TopupSucceeded:
		return false, nil // a duplicate: already charged once and credited once
	case store.TopupFailed:
		if att.Reason != "" {
			return false, &billing.DeclineError{Reason: att.Reason}
		}
		return false, ErrTopupReleased
	}
	return a.reconcile(ctx, us, att)
}

// chargeQuery is how an attempt is looked up: by what was recorded on it
// when it was made. A row from before those were recorded falls back to
// the plan's current customer and card, the only ones it can have had
// unless the plan was restarted since.
func chargeQuery(att store.TopupAttempt, sub store.Subscription) billing.ChargeQuery {
	q := billing.ChargeQuery{CustomerRef: att.CustomerRef, PaymentRef: att.PaymentRef, ChargeRef: att.ChargeRef, Key: att.Key, Since: att.Created}
	if q.CustomerRef == "" && q.PaymentRef == "" {
		q.CustomerRef, q.PaymentRef = sub.CustomerRef, sub.PaymentRef
	}
	return q
}

// reconcile settles a pending attempt that nobody in this process is
// charging any more: its reply was lost, it was still processing, or the
// app restarted mid-charge. It asks the processor what happened and never
// charges again. Only a definitive answer settles it; a failed or
// incomplete lookup, a charge still processing, and a charge the processor
// does not show (however late) all leave it pending.
func (a *App) reconcile(ctx context.Context, us *store.UserStore, att store.TopupAttempt) (bool, error) {
	var sub store.Subscription
	if att.CustomerRef == "" && att.PaymentRef == "" {
		sub, _ = us.Subscription(ctx)
	}
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	res, err := a.Pay.FindCharge(lctx, chargeQuery(att, sub))
	cancel()
	ctx = context.WithoutCancel(ctx)
	if err != nil {
		a.Log.WarnContext(ctx, "top-up reconciliation lookup was not conclusive; kept pending", "err", err, "key", att.Key)
		return false, ErrTopupPending
	}
	a.Log.InfoContext(ctx, "top-up reconciled", "key", att.Key, "status", string(res.Status))
	switch res.Status {
	case billing.ChargeSucceeded:
		return us.SucceedTopup(ctx, att.Token, res.Ref, "Top-up", a.now())
	case billing.ChargeFailed:
		// res.Ref is the intent the lookup found (by id or in the list); it
		// is kept on the failed attempt.
		if err := us.FailTopup(ctx, att.Token, res.Ref, res.Reason, a.now()); err != nil {
			return false, err
		}
		if res.Reason == "" {
			return false, ErrTopupReleased
		}
		return false, &billing.DeclineError{Reason: res.Reason}
	case billing.ChargePending:
		if err := us.NoteTopupCharge(ctx, att.Token, res.Ref); err != nil {
			return false, err
		}
	case billing.ChargeNotFound:
		if a.now().Sub(att.Created) >= TopupSettleWindow {
			a.Log.WarnContext(ctx, "top-up still not visible at the processor; kept pending", "key", att.Key, "age", a.now().Sub(att.Created).String())
			return false, ErrTopupUnconfirmed
		}
	}
	return false, ErrTopupPending
}

// ReconcileTopUps settles the learner's pending top-ups that are not being
// charged right now. Billing calls it before it shows the page, so paid
// credit appears and failed attempts free their cap slot. It reports how
// many attempts were credited.
func (a *App) ReconcileTopUps(ctx context.Context, userID int64) (int, error) {
	us := a.Store.For(userID)
	pending, err := us.PendingTopups(ctx, a.now().Add(-TopupWatchWindow))
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	credited := 0
	for _, att := range pending {
		release, busy := a.claimTopup(att.Key)
		if busy != nil {
			continue // still being charged in this process
		}
		// Re-read under the claim: the holder may have just finished it.
		cur, err := us.Topup(ctx, att.Token)
		if err == nil && cur.Status == store.TopupPending {
			if ok, _ := a.reconcile(ctx, us, cur); ok {
				credited++
			}
		}
		release()
	}
	return credited, nil
}

// ReconcileAllTopUps is the housekeeping sweep over every learner's
// pending top-ups older than a minute (and inside the watch window).
func (a *App) ReconcileAllTopUps(ctx context.Context) error {
	now := a.now()
	pending, err := a.Store.PendingTopups(ctx, now.Add(-TopupWatchWindow), now.Add(-time.Minute))
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, att := range pending {
		if seen[att.UserID] {
			continue
		}
		seen[att.UserID] = true
		if _, err := a.ReconcileTopUps(ctx, att.UserID); err != nil {
			return err
		}
	}
	return nil
}

// applyTopupEvent settles a top-up from the processor's verified webhook:
// a late success is credited exactly once (even after a lookup saw
// nothing, or after the attempt was recorded as failed), and a failure or
// cancellation releases a still-pending attempt. It runs inside the
// webhook's billing transaction, with the event's idempotency marker.
func (a *App) applyTopupEvent(ctx context.Context, ev billing.Event) error {
	att, err := a.Store.TopupByCharge(ctx, ev.ChargeRef, ev.ChargeKey)
	if errors.Is(err, store.ErrNotFound) {
		a.Log.WarnContext(ctx, "top-up event for unknown attempt", "event", ev.ID, "kind", ev.Kind)
		return nil
	}
	if err != nil {
		return err
	}
	if att.Key != ev.ChargeKey && att.ChargeRef != ev.ChargeRef {
		return nil
	}
	if att.CustomerRef != "" && ev.CustomerRef != "" && att.CustomerRef != ev.CustomerRef {
		a.Log.WarnContext(ctx, "top-up event customer does not match the attempt; ignored", "event", ev.ID)
		return nil
	}
	fresh, err := a.Store.MarkEvent(ctx, ev.ID, a.now())
	if err != nil || !fresh {
		return err
	}
	us := a.Store.For(att.UserID)
	switch ev.Kind {
	case billing.TopupPaid:
		granted, err := us.SucceedTopup(ctx, att.Token, ev.ChargeRef, "Top-up", a.now())
		if err == nil {
			a.Log.InfoContext(ctx, "top-up settled by webhook", "key", att.Key, "charge", ev.ChargeRef, "granted", granted)
		}
		return err
	case billing.TopupFailed:
		return us.FailTopup(ctx, att.Token, ev.ChargeRef, ev.Reason, a.now())
	}
	return nil
}

// SetCancel schedules the plan to end when the paid period (or the
// trial) runs out, or takes that back.
func (a *App) SetCancel(ctx context.Context, userID int64, cancel bool) (store.Subscription, error) {
	us := a.Store.For(userID)
	sub, err := a.Plan(ctx, userID)
	if err != nil {
		return sub, err
	}
	if err := a.Pay.SetCancelAtPeriodEnd(ctx, sub.SubscriptionRef, cancel); err != nil {
		a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		return sub, err
	}
	return us.ChangeSubscription(ctx, func(s *store.Subscription) { s.CancelAtEnd = cancel })
}

// UpdateCard replaces the card on file.
func (a *App) UpdateCard(ctx context.Context, userID int64, cardToken string) error {
	us := a.Store.For(userID)
	sub, err := us.Subscription(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNoPlan
	}
	if err != nil {
		return err
	}
	payRef, card, err := a.Pay.UpdateCard(ctx, sub.CustomerRef, sub.SubscriptionRef, cardToken)
	if err != nil {
		var d *billing.DeclineError
		if !errors.As(err, &d) {
			a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		}
		return err
	}
	sub, err = us.ChangeSubscription(ctx, func(s *store.Subscription) {
		s.PaymentRef, s.CardBrand, s.CardLast4, s.CardError = payRef, card.Brand, card.Last4, ""
	})
	if err != nil {
		return err
	}
	if sub.Status == "past_due" {
		if _, ok := a.Pay.(billing.Renewer); ok {
			return a.renew(ctx, sub) // the test processor retries the period now
		}
	}
	return nil
}

func (a *App) renew(ctx context.Context, sub store.Subscription) error {
	r, ok := a.Pay.(billing.Renewer)
	if !ok {
		return nil
	}
	ev := r.Renew(sub.SubscriptionRef, sub.PaymentRef, sub.CancelAtEnd, sub.Status == "past_due", sub.PeriodEnd, a.Cfg.PriceCents, a.now())
	if ev.Kind == billing.InvoiceFailed && sub.Status == "past_due" {
		return nil // already recorded; still retrying
	}
	if ev.Kind == billing.InvoicePaid && sub.Status == "past_due" {
		ev.ID += "-retry"
	}
	return a.ApplyEvent(ctx, ev)
}

func period(start, end time.Time) string {
	return start.Format("Jan 2") + " to " + end.Format("Jan 2")
}

// ApplyEvent brings a plan up to date with something the processor did.
// Events are applied once; a replay is ignored.
func (a *App) ApplyEvent(ctx context.Context, ev billing.Event) error {
	return a.Store.BillingTransaction(ctx, func(ctx context.Context) error { return a.applyEvent(ctx, ev) })
}
func (a *App) applyEvent(ctx context.Context, ev billing.Event) error {
	if ev.Kind == billing.TopupPaid || ev.Kind == billing.TopupFailed {
		return a.applyTopupEvent(ctx, ev)
	}
	sub, err := a.Store.SubscriptionByRef(ctx, ev.SubscriptionRef)
	if errors.Is(err, store.ErrNotFound) {
		a.Log.WarnContext(ctx, "billing event for unknown subscription", "event", ev.ID, "kind", ev.Kind)
		return nil
	}
	if err != nil {
		return err
	}
	fresh, err := a.Store.MarkEvent(ctx, ev.ID, a.now())
	if err != nil || !fresh {
		return err
	}
	us := a.Store.For(sub.UserID)
	at := ev.At
	if at.IsZero() {
		at = a.now()
	}
	switch ev.Kind {
	case billing.InvoicePaid:
		sub.Status, sub.CardError, sub.PastDueSince = "active", "", time.Time{}
		if !ev.PeriodEnd.IsZero() {
			sub.PeriodStart, sub.PeriodEnd = ev.PeriodStart, ev.PeriodEnd
		}
		sub.AllowanceMicros, sub.UsedMicros = a.Cfg.AllowanceMicros, 0
		if err := us.SaveSubscription(ctx, sub); err != nil {
			return err
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: at, Kind: "charge", Description: "Scry, " + period(sub.PeriodStart, sub.PeriodEnd),
			AmountCents: ev.AmountCents, Status: "paid", Ref: ev.ID})
	case billing.InvoiceFailed:
		sub.Status, sub.CardError = "past_due", strings.TrimSpace(ev.Reason)
		if sub.PastDueSince.IsZero() {
			sub.PastDueSince = at
		}
		if err := us.SaveSubscription(ctx, sub); err != nil {
			return err
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: at, Kind: "charge", Description: "Scry, one month (card failed)",
			AmountCents: ev.AmountCents, Status: "failed", Ref: ev.ID})
	case billing.SubscriptionEnded:
		sub.Status = "ended"
		return us.SaveSubscription(ctx, sub)
	}
	return nil
}

// RunRenewals brings every plan whose period has run out up to date. It
// only does anything with the test processor; Stripe reports renewals by
// webhook.
func (a *App) RunRenewals(ctx context.Context) error {
	if _, ok := a.Pay.(billing.Renewer); !ok {
		return nil
	}
	subs, err := a.Store.SubscriptionsDue(ctx, a.now())
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := a.renew(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// reserve claims a worst-case provider budget before any work starts.
func (a *App) reserve(ctx context.Context, userID int64, amount int64) (string, error) {
	id := randomToken(24)
	err := a.Store.For(userID).Reserve(ctx, id, amount)
	if errors.Is(err, store.ErrBudget) {
		return "", ErrAllowance
	}
	return id, err
}
func (a *App) settle(userID int64, id, kind string, u ai.Usage, seconds float64) {
	cost := Cost(u)
	if kind == "speech" {
		cost = int64(math.Ceil(seconds * speechMicrosPerSecond))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := a.Store.For(userID).Settle(ctx, id, store.Usage{Kind: kind, Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, Seconds: seconds, CostMicros: cost}, a.now())
	if err != nil {
		a.Log.ErrorContext(ctx, "reservation settlement failed", "err", err, "user", userID)
		a.Alert.Notify(ctx, "Usage settlement failed", "A reservation remains held; reconcile before releasing it.")
	}
}
