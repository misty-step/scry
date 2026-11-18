package app

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	"claude-haiku-5-5":  {0.10, 0.50},
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

// TopUp charges the card on file for one top-up and adds its credit.
func (a *App) TopUp(ctx context.Context, userID int64) error {
	us := a.Store.For(userID)
	sub, err := a.Plan(ctx, userID)
	if err != nil {
		return err
	}
	now := a.now()
	// One key per learner per minute: a double-clicked button charges once.
	key := fmt.Sprintf("topup-%d-%d", userID, now.Unix()/60)
	if done, err := a.Store.HasProcessedEvent(ctx, key); err != nil || done {
		return err
	}
	ref, err := a.Pay.Charge(ctx, sub.CustomerRef, sub.PaymentRef, a.Cfg.TopupCents, "Scry top-up", key)
	var d *billing.DeclineError
	if errors.As(err, &d) {
		_, _ = us.ChangeSubscription(ctx, func(s *store.Subscription) { s.CardError = d.Reason })
		_ = us.AddLedger(ctx, store.LedgerEntry{At: now, Kind: "topup", Description: "Top-up (card failed)", AmountCents: a.Cfg.TopupCents, Status: "failed", Ref: key + "-failed"})
		return err
	}
	if err != nil {
		a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		return err
	}
	return us.CreditTopUp(ctx, store.LedgerEntry{At: now, Kind: "topup", Description: "Top-up", AmountCents: a.Cfg.TopupCents, Status: "paid", Ref: ref}, a.Cfg.TopupMicros, key)
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
