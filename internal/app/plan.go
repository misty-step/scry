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

// rates is what the provider charges, per token, by model. Usage is
// metered at cost (the provider's own usage.cost when it reports one);
// the margin lives in the plan's fixed prices. The OpenRouter entries are
// the catalog prices of the defaults (https://openrouter.ai/api/v1/models,
// read 2026-11-21): openai/gpt-6.2-sol $2/$10 per million tokens,
// openai/gpt-6-luna $0.10/$0.50. typesafe/jev-1.14 lists $0.042 per million
// input tokens and free output; it is priced here at $0.10/$1, a margin
// over that, so its reservations stay conservative. A model with no entry
// is priced at the ceiling, claude-fable-5-2's $8/$40.
var rates = map[string]rate{
	"claude-sonnet-5-5": {2, 10},
	"claude-opus-5-5":   {4, 20},
	"claude-fable-5-2":  {8, 40},

	ai.DefaultOpenRouterDraftModel:   {2, 10},    // openai/gpt-6.2-sol
	ai.DefaultOpenRouterExplainModel: {0.1, 0.5}, // openai/gpt-6-luna
	ai.DefaultOpenRouterJudgeModel:   {0.1, 1},   // typesafe/jev-1.14
	"anthropic/claude-sonnet-5.5":    {2, 10},

	"test-judge":   {0.10, 0.50},
	"test-draft":   {4, 20},
	"test-explain": {0.10, 0.50},
}

// ceilingRate prices a model Scry has no entry for, so costs cannot run away.
var ceilingRate = rates["claude-fable-5-2"]

// datedModel matches a resolved model id with a version date suffix,
// such as typesafe/jev-1.14-20261014.
var datedModel = regexp.MustCompile(`^(.+)-\d{8}$`)

// rateFor is a model's per-token price: its own entry, else the entry of
// the same model without a version date, else the ceiling.
func rateFor(model string) rate {
	if r, ok := rates[model]; ok {
		return r
	}
	if m := datedModel.FindStringSubmatch(model); m != nil {
		if r, ok := rates[m[1]]; ok {
			return r
		}
	}
	return ceilingRate
}

// speechMicrosPerSecond is what transcription is metered at by default
// ($0.003 a minute). microsoft/mai-transcribe-2 lists "$0.1" in the
// catalog with no unit; read as per hour it is about 28 micros a second,
// so 50 is a margin over it. A provider-reported usage.cost above the
// per-second price is metered instead (settle).
const speechMicrosPerSecond = 50

// speechRates is the per-second price by speech model; any other model
// is metered at speechMicrosPerSecond.
var speechRates = map[string]float64{
	"microsoft/mai-transcribe-2": speechMicrosPerSecond,
	"test-speech":                speechMicrosPerSecond,
}

func speechRate(model string) float64 {
	if r, ok := speechRates[model]; ok {
		return r
	}
	return speechMicrosPerSecond
}

// MaxSpeechSeconds is the longest recording Scry sends on (speech.Inspect).
const MaxSpeechSeconds = 120

// Cost prices one model call in micro-dollars. A model Scry has no price
// for is charged at the highest rate it knows, so costs cannot run away.
func Cost(u ai.Usage) int64 {
	if u.CostMicros != nil {
		return *u.CostMicros
	}
	r := rateFor(u.Model)
	return int64(math.Ceil(float64(u.InputTokens)*r.in + float64(u.OutputTokens)*r.out))
}

// Worst-case token bounds of one call per surface; the adapters enforce
// them (Jev's 8 KiB state, a draft's 32,000-byte input and 16,000 output
// tokens, an explanation's input cap and ExplainMaxTokens).
const (
	judgeInTokens, judgeOutTokens     = 8192 + 1024, 1024
	draftInTokens, draftOutTokens     = 32000 + 1024, 16000
	explainInTokens, explainOutTokens = 2560, ai.ExplainMaxTokens
)

// BudgetMicros is the worst case of one call of a model with the given
// token bounds, at that model's rate (the ceiling for an unknown model).
// At the ceiling it equals the ai package's fixed budgets.
func BudgetMicros(model string, inTokens, outTokens int64) int64 {
	r := rateFor(model)
	return int64(math.Ceil(float64(inTokens)*r.in + float64(outTokens)*r.out))
}

// JudgeBudget is what one judgment reserves with the configured grader
// (about $0.002 for typesafe/jev-1.14; $0.115 for an unknown model).
func (a *App) JudgeBudget() int64 {
	return BudgetMicros(ai.JudgeModel(a.Model), judgeInTokens, judgeOutTokens)
}

// DraftBudget is what one draft reserves with the configured drafting
// model (about $0.226 for openai/gpt-6.2-sol; $0.904 for an unknown model).
func (a *App) DraftBudget() int64 {
	return BudgetMicros(ai.DraftModel(a.Model), draftInTokens, draftOutTokens)
}

// ExplainBudget is what one "Why?" reserves with the configured explainer
// (under $0.001 for openai/gpt-6-luna; $0.061 for an unknown model).
func (a *App) ExplainBudget() int64 {
	return BudgetMicros(ai.ExplainModel(a.Explainer), explainInTokens, explainOutTokens)
}

// Speech holds are sized for what a provider may really bill, not for
// the per-second meter: a dedicated STT model (mai-transcribe, whisper,
// scribe) or an audio-in chat model (gemini) can report a usage.cost well
// above speechMicrosPerSecond, and settle meters the larger of the two.
// A settle never debits more than its hold (store.SettleMetered), so the
// hold is the admission-time ceiling on what one recording can draw:
// speechHoldMicrosPerSecond per whole second, at least speechHoldFloorMicros
// (a provider's per-request minimum).
const (
	speechHoldMicrosPerSecond = 250    // 5x the meter; $0.015 a minute
	speechHoldFloorMicros     = 15_000 // $0.015
)

// SpeechBudgetMicros is what a recording of the given length reserves:
// max(ceil(seconds) x 250, 15,000) micros, so $0.03 for the longest
// (MaxSpeechSeconds) clip, well inside the no-card trial's $0.50.
func SpeechBudgetMicros(seconds float64) int64 {
	return max(int64(math.Ceil(max(seconds, 0)))*speechHoldMicrosPerSecond, speechHoldFloorMicros)
}

// SpeechBudget is what the longest recording Scry sends on reserves.
func (a *App) SpeechBudget() int64 {
	return SpeechBudgetMicros(MaxSpeechSeconds)
}

// DraftsPerAllowance estimates how many drafts the monthly allowance
// buys, so Billing can say what the allowance covers in plain terms.
func (c Config) DraftsPerAllowance() int {
	return c.draftsPer(c.AllowanceMicros, ai.DefaultOpenRouterDraftModel)
}

// draftsPer is how many typical drafts (1,500 tokens in, 6,000 out) of a
// model an amount of allowance buys.
func (c Config) draftsPer(micros int64, model string) int {
	typical := Cost(ai.Usage{Model: model, InputTokens: 1500, OutputTokens: 6000})
	if typical <= 0 {
		return 0
	}
	return int(micros / typical)
}

// HasAccess reports whether a plan lets the learner use Scry now. There
// is no free tier: no plan, no access, and a no-card trial only until its
// period_end (the same predicate as store.ReserveAt's atomic guard).
func (a *App) HasAccess(s store.Subscription) bool {
	switch s.Status {
	case store.SubActive:
		return true
	case store.SubTrialing:
		return !s.NoCard() || a.now().Before(s.PeriodEnd)
	case store.SubPastDue:
		return a.now().Sub(s.PastDueSince) <= a.grace()
	}
	return false
}

// spent is what counts against the allowance this period: what was drawn
// and, on a no-card trial, the overage its settles wrote off (the trial's
// cap is on what it actually cost; store.reserveBalance).
func spent(s store.Subscription) int64 {
	if s.NoCard() {
		return s.UsedMicros + s.OverageMicros
	}
	return s.UsedMicros
}

// Unreserved is what a new hold could draw on before the holds in flight
// (reserved) are taken off: the store's reserveBalance less reserved, so
// on a no-card trial the period's overage counts as spent and top-up
// credit never applies. It can be negative.
func Unreserved(s store.Subscription, reserved int64) int64 {
	if s.NoCard() {
		return s.AllowanceMicros - spent(s) - reserved
	}
	return s.AllowanceMicros - s.UsedMicros + s.TopupMicros - reserved
}

// CanSpend reports whether there is allowance or top-up credit left.
func CanSpend(s store.Subscription) bool {
	return spent(s) < s.AllowanceMicros || (!s.NoCard() && s.TopupMicros > 0)
}

// UsedPercent is how much of this period's allowance is spent, 0 to 100.
func UsedPercent(s store.Subscription) int {
	if s.AllowanceMicros <= 0 {
		return 100
	}
	p := int(spent(s) * 100 / s.AllowanceMicros)
	return min(max(p, 0), 100)
}

// Plan loads the learner's plan, first bringing it up to date if its
// period has run out: an account with no plan at all starts its no-card
// trial, and a no-card trial at its period_end ends. It returns
// ErrPlanEnded when the learner has no access (ErrNoPlan only if a trial
// could not be started).
func (a *App) Plan(ctx context.Context, userID int64) (store.Subscription, error) {
	us := a.Store.For(userID)
	sub, err := us.Subscription(ctx)
	if errors.Is(err, store.ErrNotFound) {
		// An account from before every sign-up started a trial, or one
		// whose first request raced its creation: it starts now.
		if err := a.StartNoCardTrial(ctx, userID); err != nil {
			return sub, err
		}
		sub, err = us.Subscription(ctx)
		if errors.Is(err, store.ErrNotFound) {
			return sub, ErrNoPlan
		}
	}
	if err != nil {
		return sub, err
	}
	if sub.NoCardTrial() && !a.now().Before(sub.PeriodEnd) {
		if _, err := us.EndExpiredNoCardTrial(ctx, a.now()); err != nil {
			return sub, err
		}
		if sub, err = us.Subscription(ctx); err != nil {
			return sub, err
		}
	}
	if _, ok := a.Pay.(billing.Renewer); ok && sub.Status != "ended" && !sub.NoCard() && !sub.PeriodEnd.After(a.now()) {
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

// StartNoCardTrial gives a learner with no plan the 7-day no-card trial
// (Cfg.TrialDays, capped at Cfg.TrialMicros of metered usage). It does
// nothing for a learner who already has a plan, live or ended: the trial
// is had once.
func (a *App) StartNoCardTrial(ctx context.Context, userID int64) error {
	now := a.now()
	opened, err := a.Store.For(userID).OpenNoCardTrial(ctx, now, now.AddDate(0, 0, a.Cfg.TrialDays), a.Cfg.TrialMicros,
		fmt.Sprintf("%d-day trial started", a.Cfg.TrialDays))
	if err == nil && opened {
		a.Log.InfoContext(ctx, "no-card trial started", "user", userID, "days", a.Cfg.TrialDays, "allowance_micros", a.Cfg.TrialMicros)
	}
	return err
}

// MinCardTrialLeft is the least trial a card can be added to and still
// keep: with less left, adding a card starts the paid plan at once, so the
// processor is never asked for a trial that ends within the hour.
const MinCardTrialLeft = time.Hour

// BillingTerms is what the card step told the learner adding a card costs
// today. The page carries it in the form (billing_terms), and StartTrial
// only charges today when the learner submitted "immediate": a page that
// said "nothing charged today" can never turn into a charge.
type BillingTerms string

const (
	// TermsTrial: nothing is charged today (a trial, or the same trial
	// continuing with the card on file).
	TermsTrial BillingTerms = "trial"
	// TermsImmediate: the plan starts at once and the price is charged
	// today.
	TermsImmediate BillingTerms = "immediate"
	// TermsUpdate: a card-backed plan with access swaps its card.
	TermsUpdate BillingTerms = "update"
)

// ParseBillingTerms reads a submitted billing_terms field. Anything else,
// including a missing field, is "" and never counts as consent to a charge.
func ParseBillingTerms(v string) BillingTerms {
	switch t := BillingTerms(strings.TrimSpace(v)); t {
	case TermsTrial, TermsImmediate, TermsUpdate:
		return t
	}
	return ""
}

// ErrTermsChanged is a card step submitted on terms that no longer hold:
// the page said nothing would be charged today, but by the time it was
// submitted the trial had ended or had less than MinCardTrialLeft to go,
// so adding the card now would charge today. Nothing was sent to the
// processor and the plan is unchanged; the learner must confirm the
// immediate charge (billing_terms "immediate").
var ErrTermsChanged = errors.New("adding a card now charges today; the learner must confirm")

// CardTerms is what adding a card to this plan costs today, as of now:
//
//   - no plan at all (Status ""): TermsTrial, the trial with a card;
//   - a no-card trial with at least MinCardTrialLeft to go: TermsTrial,
//     the same trial continuing;
//   - a no-card trial with less left, an ended no-card trial, or a plan
//     with no access: TermsImmediate, charged today;
//   - a card-backed plan with access: TermsUpdate.
func (a *App) CardTerms(s store.Subscription) BillingTerms {
	switch {
	case s.Status == store.SubNone:
		return TermsTrial
	case s.NoCardTrial() && s.PeriodEnd.Sub(a.now()) >= MinCardTrialLeft:
		return TermsTrial
	case s.NoCard() || !a.HasAccess(s):
		return TermsImmediate
	}
	return TermsUpdate
}

// StartTrial puts a card behind the learner's plan, on the terms the card
// step showed them (agreed).
//
//   - No plan at all: the 7-day trial, with the card on file
//     (Cfg.CardTrialMicros), first charged when it ends.
//   - A no-card trial with at least MinCardTrialLeft to go: the card is
//     added and the same trial continues to the same end date, with
//     nothing charged today; the trial allowance rises to
//     Cfg.CardTrialMicros (what was spent stays spent), top-ups are
//     available, and the first monthly charge is on the trial's end date.
//   - A no-card trial with less left, or one that has ended: there is no
//     second free trial. The plan starts at once, charged today, with the
//     full monthly allowance.
//   - A plan that has ended: the same, charged today.
//
// Charging today needs agreed == TermsImmediate: otherwise StartTrial
// returns ErrTermsChanged without asking the processor for anything. The
// learner's plan row is only changed after the processor has taken the
// card, so a declined card (or a processor error) leaves a no-card trial
// exactly as it was, usable until its own period_end.
//
// A learner already on a card-backed plan is left as they are.
func (a *App) StartTrial(ctx context.Context, u store.User, cardToken string, agreed BillingTerms) error {
	us := a.Store.For(u.ID)
	now := a.now()
	old, err := us.Subscription(ctx)
	hadPlan := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if !hadPlan {
		old = store.Subscription{}
	}
	terms := a.CardTerms(old)
	if terms == TermsImmediate && agreed != TermsImmediate {
		a.Log.InfoContext(ctx, "card step refused: terms changed to an immediate charge", "user", u.ID, "agreed", string(agreed))
		return ErrTermsChanged
	}
	fromNoCard := hadPlan && old.NoCard() && (old.NoCardTrial() || old.Status == store.SubEnded)
	switch {
	case hadPlan && old.NoCardTrial() && terms == TermsTrial:
		return a.addCardToTrial(ctx, u, us, old, cardToken, now)
	case hadPlan && !fromNoCard && old.Status != store.SubEnded:
		return nil // already on a plan
	}
	p := billing.SubscribeParams{Email: u.Email, CardToken: cardToken, Key: fmt.Sprintf("sub-%d-%d", u.ID, now.Unix()/60), Now: now}
	if !hadPlan {
		p.TrialEnd = now.AddDate(0, 0, a.Cfg.TrialDays)
	}
	// Nothing about the plan has changed yet: if the processor refuses the
	// card, the learner keeps what they had.
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
		sub.Status, sub.AllowanceMicros, sub.TrialEnd = store.SubActive, a.Cfg.AllowanceMicros, old.TrialEnd
		if fromNoCard {
			// The no-card trial (live in its last hour, or ended) becomes the
			// paid plan in one guarded write, only now that it is paid for.
			if err := us.StartPaidFromNoCard(ctx, sub); err != nil {
				a.Log.ErrorContext(ctx, "card charged at the processor but the no-card plan changed", "err", err, "user", u.ID, "subscription", res.SubscriptionRef)
				a.Alert.Notify(ctx, "Card charged for a plan that changed", "Subscription "+res.SubscriptionRef+" was charged for a no-card plan that had already changed: "+err.Error())
				return err
			}
		} else if err := us.OpenPlan(ctx, sub); err != nil {
			return err
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: now, Kind: "charge", Description: "Scry, one month", AmountCents: a.Cfg.PriceCents, Status: "paid", Ref: res.ChargeRef})
	}
	sub.Status, sub.AllowanceMicros, sub.TrialEnd = "trialing", a.Cfg.CardTrialMicros, p.TrialEnd
	if err := us.OpenPlan(ctx, sub); err != nil {
		return err
	}
	return us.AddLedger(ctx, store.LedgerEntry{At: now, Kind: "trial", Description: fmt.Sprintf("%d-day trial started", a.Cfg.TrialDays), Status: "none", Ref: "trial-" + res.SubscriptionRef})
}

// addCardToTrial subscribes the learner at the processor with the no-card
// trial's own end as the processor's trial end, then records the card on
// the same trial (store.AddCardToTrial). Nothing is charged today.
func (a *App) addCardToTrial(ctx context.Context, u store.User, us *store.UserStore, old store.Subscription, cardToken string, now time.Time) error {
	res, err := a.Pay.Subscribe(ctx, billing.SubscribeParams{Email: u.Email, CardToken: cardToken,
		Key: fmt.Sprintf("sub-%d-%d", u.ID, now.Unix()/60), Now: now, TrialEnd: old.PeriodEnd})
	if err != nil {
		var d *billing.DeclineError
		if !errors.As(err, &d) {
			a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		}
		return err
	}
	_, err = us.AddCardToTrial(ctx, store.Subscription{CustomerRef: res.CustomerRef, SubscriptionRef: res.SubscriptionRef,
		PaymentRef: res.PaymentRef, CardBrand: res.Card.Brand, CardLast4: res.Card.Last4, PeriodEnd: res.PeriodEnd}, a.Cfg.CardTrialMicros, now)
	if err != nil {
		// The processor now has a trialing subscription Scry did not record
		// (the trial ended or changed in between): ops must look.
		a.Log.ErrorContext(ctx, "card added at the processor but not to the trial", "err", err, "user", u.ID, "subscription", res.SubscriptionRef)
		a.Alert.Notify(ctx, "Card added to a trial that changed", "Subscription "+res.SubscriptionRef+" was created for a no-card trial that was no longer open: "+err.Error())
		return err
	}
	a.Log.InfoContext(ctx, "card added to no-card trial", "user", u.ID, "trial_end", old.PeriodEnd)
	return nil
}

// EndExpiredTrials ends every no-card trial whose period_end has passed,
// whatever the card processor (housekeeping; Plan does it per learner).
func (a *App) EndExpiredTrials(ctx context.Context) (int64, error) {
	n, err := a.Store.EndExpiredNoCardTrials(ctx, a.now())
	if err == nil && n > 0 {
		a.Log.InfoContext(ctx, "no-card trials ended", "count", n)
	}
	return n, err
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

// ErrTopupNeedsCard is a top-up asked for on a plan with no card on file
// (a no-card trial): there is nothing to charge, so nothing is sent to
// the processor. Adding a card comes first.
var ErrTopupNeedsCard = errors.New("top-ups need a card on file")

// topupEligible is the plan check made inside the reservation transaction,
// against the plan row as it is under the write lock.
func (a *App) topupEligible(sub store.Subscription) error {
	if !a.HasAccess(sub) {
		return ErrPlanEnded
	}
	if sub.NoCard() {
		return ErrTopupNeedsCard
	}
	return nil
}

func (a *App) topUpClaimed(ctx context.Context, userID int64, token, key string) (bool, error) {
	// Brings a plan whose period ran out up to date, and refuses early.
	// This read is not the authorization: that is repeated inside TX1.
	if sub, err := a.Plan(ctx, userID); err != nil {
		return false, err
	} else if sub.NoCard() {
		return false, ErrTopupNeedsCard
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
	var c *billing.CanceledError
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
	case errors.As(err, &c):
		// Canceled with no card fault: nothing was taken. The attempt is
		// released (keeping the intent's id) with no decline sentence, so no
		// card fault or failed history line is recorded, exactly as when a
		// lookup finds the intent canceled.
		charge := c.Ref
		if charge == "" {
			charge = ref
		}
		if ferr := us.FailTopup(ctx, att.Token, charge, "", a.now()); ferr != nil {
			return false, ferr
		}
		return false, ErrTopupReleased
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
	if errors.Is(err, store.ErrTopupCapExceededLate) {
		a.refundDue(ctx, att, ref)
		return false, err
	}
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
	case store.TopupRefundDue:
		return false, store.ErrTopupCapExceededLate // charged once, not credited, owed back
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
		granted, err := us.SucceedTopup(ctx, att.Token, res.Ref, "Top-up", a.now())
		if errors.Is(err, store.ErrTopupCapExceededLate) {
			a.refundDue(ctx, att, res.Ref)
		}
		return granted, err
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
	if ev.Kind == billing.TopupPaid && ev.ChargeRef == "" && att.ChargeRef == "" {
		// Never paid without the processor's id (the adapter drops such events).
		a.Log.WarnContext(ctx, "top-up paid event names no charge; ignored", "event", ev.ID)
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
		if errors.Is(err, store.ErrTopupCapExceededLate) {
			// Recorded as refund_due inside this transaction: commit it.
			a.refundDue(ctx, att, ev.ChargeRef)
			return nil
		}
		if err == nil {
			a.Log.InfoContext(ctx, "top-up settled by webhook", "key", att.Key, "charge", ev.ChargeRef, "granted", granted)
		}
		return err
	case billing.TopupFailed:
		return us.FailTopup(ctx, att.Token, ev.ChargeRef, ev.Reason, a.now())
	}
	return nil
}

// refundDue reports a charge the processor took after its attempt had
// failed and the month's $20 cap was used up since: it was not credited
// and is recorded as refund_due, and ops must refund it.
func (a *App) refundDue(ctx context.Context, att store.TopupAttempt, chargeRef string) {
	if chargeRef == "" {
		chargeRef = att.ChargeRef
	}
	a.Log.ErrorContext(ctx, "late top-up success over the monthly cap; refund due", "key", att.Key, "charge", chargeRef, "user", att.UserID)
	a.Alert.Notify(ctx, "Top-up refund due",
		fmt.Sprintf("Charge %s (%s) succeeded after its attempt failed and the month's top-up limit was reached. It was not credited: refund it.", chargeRef, att.Key))
}

// SetCancel schedules the plan to end when the paid period (or the
// trial) runs out, or takes that back.
func (a *App) SetCancel(ctx context.Context, userID int64, cancel bool) (store.Subscription, error) {
	us := a.Store.For(userID)
	sub, err := a.Plan(ctx, userID)
	if err != nil {
		return sub, err
	}
	if sub.NoCard() {
		// A no-card trial has nothing at the processor and is never
		// charged: it simply ends on its date. There is nothing to cancel.
		return sub, nil
	}
	if err := a.Pay.SetCancelAtPeriodEnd(ctx, sub.SubscriptionRef, cancel); err != nil {
		a.Alert.Notify(ctx, "Card processor is failing", err.Error())
		return sub, err
	}
	return us.ChangeSubscription(ctx, func(s *store.Subscription) { s.CancelAtEnd = cancel })
}

// ErrNoCardOnFile is a card update on a plan that has none yet.
var ErrNoCardOnFile = errors.New("no card on file to update")

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
	if sub.NoCard() {
		return ErrNoCardOnFile // there is no customer to update: StartTrial adds the first card
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
	se, ok := subscriptionEvent(ev.Kind)
	if !ok {
		a.Log.WarnContext(ctx, "billing event of unknown kind ignored", "event", ev.ID, "kind", ev.Kind)
		return nil
	}
	sub, err := a.Store.SubscriptionByRef(ctx, ev.SubscriptionRef)
	if errors.Is(err, store.ErrNotFound) {
		a.Log.WarnContext(ctx, "billing event for unknown subscription", "event", ev.ID, "kind", ev.Kind)
		return nil
	}
	if err != nil {
		return err
	}
	if sub.NoCard() {
		// A no-card trial has no processor subscription, so no genuine
		// event names it; it never moves on one (only by adding a card or
		// by reaching its end).
		a.Log.WarnContext(ctx, "billing event for a no-card trial ignored", "event", ev.ID, "kind", ev.Kind)
		return nil
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
	paidLine := func(start, end time.Time) error {
		desc := "Scry, one month"
		if !end.IsZero() {
			desc = "Scry, " + period(start, end)
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: at, Kind: "charge", Description: desc,
			AmountCents: ev.AmountCents, Status: "paid", Ref: ev.ID})
	}
	to, terr := store.TransitionSubscription(sub.Status, se)
	if terr != nil {
		// An invoice for a plan that has ended (a late or reordered
		// webhook) never brings it back: only an explicit new plan does.
		// Money the processor says it took is still kept in the history.
		a.Log.WarnContext(ctx, "billing event for an ended plan ignored", "event", ev.ID, "kind", ev.Kind, "err", terr)
		if se == store.SubEventInvoicePaid {
			a.Alert.Notify(ctx, "Payment on an ended plan", "Invoice "+ev.ID+" was paid after the plan ended; it may need a refund.")
			return paidLine(ev.PeriodStart, ev.PeriodEnd)
		}
		return nil
	}
	switch se {
	case store.SubEventInvoicePaid:
		advances := ev.PeriodEnd.IsZero() || ev.PeriodEnd.After(sub.PeriodEnd)
		if !advances && (sub.Status == store.SubActive || ev.PeriodEnd.Before(sub.PeriodEnd)) {
			// A stale invoice, for a period older than (or, on a plan already
			// active, the same as) the current one, arriving out of order. It
			// never moves the period back or hands out a fresh allowance.
			a.Log.WarnContext(ctx, "stale invoice ignored", "event", ev.ID, "period_end", ev.PeriodEnd, "current", sub.PeriodEnd)
			return paidLine(ev.PeriodStart, ev.PeriodEnd)
		}
		sub.Status, sub.CardError, sub.PastDueSince = to, "", time.Time{}
		if advances {
			// A new period: its allowance starts afresh.
			if !ev.PeriodEnd.IsZero() {
				sub.PeriodStart, sub.PeriodEnd = ev.PeriodStart, ev.PeriodEnd
			}
			sub.AllowanceMicros, sub.UsedMicros, sub.OverageMicros = a.Cfg.AllowanceMicros, 0, 0
		}
		// Otherwise the current period's unpaid invoice was paid late: the
		// plan recovers, and what was already spent of it stays spent.
		if err := us.SaveSubscription(ctx, sub); err != nil {
			return err
		}
		return paidLine(sub.PeriodStart, sub.PeriodEnd)
	case store.SubEventInvoiceFailed:
		if !ev.PeriodEnd.IsZero() && !ev.PeriodEnd.After(sub.PeriodEnd) {
			// A failure for a period already settled is not the open invoice.
			a.Log.WarnContext(ctx, "stale invoice failure ignored", "event", ev.ID, "period_end", ev.PeriodEnd, "current", sub.PeriodEnd)
			return nil
		}
		sub.Status, sub.CardError = to, strings.TrimSpace(ev.Reason)
		if sub.PastDueSince.IsZero() {
			sub.PastDueSince = at
		}
		if err := us.SaveSubscription(ctx, sub); err != nil {
			return err
		}
		return us.AddLedger(ctx, store.LedgerEntry{At: at, Kind: "charge", Description: "Scry, one month (card failed)",
			AmountCents: ev.AmountCents, Status: "failed", Ref: ev.ID})
	case store.SubEventEnded:
		if sub.Status == to {
			return nil
		}
		sub.Status = to
		return us.SaveSubscription(ctx, sub)
	}
	return nil
}

// subscriptionEvent maps a processor event about a plan onto the plan's
// state machine.
func subscriptionEvent(k billing.EventKind) (store.SubscriptionEvent, bool) {
	switch k {
	case billing.InvoicePaid:
		return store.SubEventInvoicePaid, true
	case billing.InvoiceFailed:
		return store.SubEventInvoiceFailed, true
	case billing.SubscriptionEnded:
		return store.SubEventEnded, true
	}
	return 0, false
}

// RunRenewals brings every plan whose period has run out up to date: it
// ends expired no-card trials, and renews the rest with the test
// processor (Stripe reports renewals by webhook).
func (a *App) RunRenewals(ctx context.Context) error {
	if _, err := a.EndExpiredTrials(ctx); err != nil {
		return err
	}
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

// grace is how long a past_due plan keeps access (HasAccess).
func (a *App) grace() time.Duration { return time.Duration(a.Cfg.GraceDays) * 24 * time.Hour }

// ReservationTTL is how long a hold may stay unsettled before the hourly
// housekeeping sweep releases it. It is far longer than the longest
// provider call (a draft, bounded at 5 minutes), so it only ever releases
// holds nobody will settle: an abandoned request, or a provider outcome
// that stayed unknown (held on purpose, so a retry cannot spend unmetered).
const ReservationTTL = time.Hour

// reserve claims a worst-case provider budget before any work starts. The
// store checks access (HasAccess, by this app's clock and grace) and the
// budget in one atomic statement, so a plan that ended or ran out of grace
// after the caller's Plan() read is still refused (ErrPlanEnded).
func (a *App) reserve(ctx context.Context, userID int64, amount int64) (string, error) {
	id := randomToken(24)
	err := a.Store.For(userID).ReserveAt(ctx, id, amount, a.now(), a.grace())
	switch {
	case errors.Is(err, store.ErrBudget):
		return "", ErrAllowance
	case errors.Is(err, store.ErrReservationNoAccess):
		return "", ErrPlanEnded
	case err != nil:
		return "", err
	}
	return id, nil
}
func (a *App) settle(userID int64, id, kind string, u ai.Usage, seconds float64) {
	cost := Cost(u)
	if kind == "speech" {
		// Priced by length, or by what the provider reported if that is more.
		cost = int64(math.Ceil(seconds * speechRate(u.Model)))
		if u.CostMicros != nil && *u.CostMicros > cost {
			cost = *u.CostMicros
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	overage, err := a.Store.For(userID).SettleMetered(ctx, id, store.Usage{Kind: kind, Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, Seconds: seconds, CostMicros: cost}, a.now())
	if err == nil && overage > 0 {
		// The call cost more than its hold: only the hold was drawn, so no
		// other hold in flight lost its backing, and the rest is written
		// off (period_overage_micros; on a no-card trial it counts against
		// the cap, so no further hold is admitted once it is reached). The
		// hold sizes should make this rare: ops should look at the model.
		a.Log.WarnContext(ctx, "usage cost over its reservation; overage written off", "user", userID, "kind", kind,
			"model", u.Model, "cost_micros", cost, "overage_micros", overage)
	}
	if err != nil {
		a.Log.ErrorContext(ctx, "reservation settlement failed", "err", err, "user", userID)
		a.Alert.Notify(ctx, "Usage settlement failed", "A reservation remains held; reconcile before releasing it.")
	}
}
