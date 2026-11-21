package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/srs"
	"scry/internal/store"
)

// ---------------------------------------------------------------- processor

// rapidIntent is one PaymentIntent at the scripted processor. status ""
// means the request reached the processor but nothing is visible (yet).
type rapidIntent struct {
	id, key, customer string
	status            billing.PaymentIntentStatus
	reason            string
}

// rapidSub is one subscription at the processor: its first period ended at
// end0; invoice k covers [end0+(k-1) months, end0+k months).
type rapidSub struct {
	ref, customer string
	end0          time.Time
	invoiced      int // the latest invoice period billed (paid or failed)
	paid          int // the latest invoice period paid
	ended         bool
}

// rapidPay is a scripted card processor with no clock of its own (it is
// not a billing.Renewer): plans move only by the webhooks the test
// delivers. Every event it ever emitted stays deliverable, any number of
// times and in any order.
type rapidPay struct {
	mu         sync.Mutex
	next       string // the outcome of the next new Charge
	lookupFail bool   // FindCharge answers with an error
	intents    map[string]*rapidIntent
	subs       []*rapidSub
	emitted    []billing.Event
	n          int
}

func newRapidPay() *rapidPay { return &rapidPay{next: "succeeded", intents: map[string]*rapidIntent{}} }

func (p *rapidPay) seq(prefix string) string { p.n++; return fmt.Sprintf("%s_%d", prefix, p.n) }

func (p *rapidPay) Name() string                                               { return "rapid" }
func (p *rapidPay) PublishableKey() string                                     { return "" }
func (p *rapidPay) Tokenize(string, string, string, time.Time) (string, error) { return "tok_ok", nil }

func (p *rapidPay) Subscribe(_ context.Context, sp billing.SubscribeParams) (billing.Subscribed, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := &rapidSub{ref: p.seq("sub"), customer: p.seq("cus"), end0: sp.TrialEnd}
	out := billing.Subscribed{CustomerRef: s.customer, SubscriptionRef: s.ref, PaymentRef: p.seq("pm"),
		Card: billing.Card{Brand: "Visa", Last4: "4242"}, PeriodStart: sp.Now}
	if sp.TrialEnd.IsZero() {
		s.end0 = sp.Now.AddDate(0, 1, 0)
		out.ChargeRef = p.seq("ch_sub")
	}
	out.PeriodEnd = s.end0
	p.subs = append(p.subs, s)
	return out, nil
}

func (p *rapidPay) UpdateCard(context.Context, string, string, string) (string, billing.Card, error) {
	return "pm_new", billing.Card{Brand: "Visa", Last4: "4242"}, nil
}

func (p *rapidPay) SetCancelAtPeriodEnd(context.Context, string, bool) error { return nil }

func (p *rapidPay) ParseWebhook(context.Context, []byte, string, time.Time) ([]billing.Event, error) {
	return nil, errors.New("not used")
}

// emitIntent records the webhook a PaymentIntent status produces.
func (p *rapidPay) emitIntent(pi *rapidIntent) {
	kind, ok := billing.TopupEventFor(pi.status)
	if !ok {
		return
	}
	p.emitted = append(p.emitted, billing.Event{ID: p.seq("evt_pi"), Kind: kind, ChargeRef: pi.id, ChargeKey: pi.key,
		CustomerRef: pi.customer, Reason: pi.reason})
}

// Charge is idempotent by key, as the processor's idempotency keys are.
func (p *rapidPay) Charge(_ context.Context, customer, _ string, _ int64, _ string, key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pi, ok := p.intents[key]; ok {
		if pi.status == "" {
			return "", errors.New("connection reset")
		}
		return pi.id, billing.ChargeOutcome(pi.status, pi.id, pi.reason)
	}
	pi := &rapidIntent{id: p.seq("pi"), key: key, customer: customer}
	p.intents[key] = pi
	switch p.next {
	case "succeeded":
		pi.status = billing.PISucceeded
	case "processing":
		pi.status = billing.PIProcessing
	case "requires_action":
		pi.status = billing.PIRequiresAction
	case "decline":
		pi.status, pi.reason = billing.PIRequiresPaymentMethod, "Your card was declined."
	case "canceled":
		pi.status = billing.PICanceled
	case "error_ref":
		pi.status = billing.PIProcessing
		return "", &billing.ProcessorError{Status: 500, Code: "api_error", Message: "boom", Ref: pi.id}
	case "lost_created":
		pi.status = billing.PIProcessing
		return "", errors.New("reply lost")
	case "lost_none":
		return "", errors.New("timeout before the processor answered")
	}
	p.emitIntent(pi)
	return pi.id, billing.ChargeOutcome(pi.status, pi.id, pi.reason)
}

func (p *rapidPay) FindCharge(_ context.Context, q billing.ChargeQuery) (billing.ChargeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lookupFail {
		return billing.ChargeResult{}, billing.ErrLookupIncomplete
	}
	pi, ok := p.intents[q.Key]
	if !ok || pi.status == "" {
		return billing.ChargeResult{Status: billing.ChargeNotFound}, nil
	}
	st, reason := billing.ClassifyPaymentIntent(pi.status, pi.reason)
	return billing.ChargeResult{Status: st, Ref: pi.id, Reason: reason}, nil
}

// piMoves is the processor's PaymentIntent lifecycle (canAdvance in
// specs/billing_topup.qnt): "" is a request recovered late; a refused
// intent can still be confirmed later (a late authoritative success).
var piMoves = map[billing.PaymentIntentStatus][]billing.PaymentIntentStatus{
	"":                              {billing.PIProcessing, billing.PISucceeded, billing.PIRequiresPaymentMethod},
	billing.PIProcessing:            {billing.PIRequiresAction, billing.PISucceeded, billing.PIRequiresPaymentMethod, billing.PICanceled},
	billing.PIRequiresAction:        {billing.PISucceeded, billing.PIRequiresPaymentMethod, billing.PICanceled},
	billing.PIRequiresPaymentMethod: {billing.PISucceeded, billing.PICanceled},
}

// ---------------------------------------------------------------- world

type billingWorld struct {
	ctx   context.Context
	st    *store.Store
	app   *app.App
	pay   *rapidPay
	alert *alert.Recorder
	user  store.User
	us    *store.UserStore
	mu    sync.Mutex
	clock time.Time
}

func (w *billingWorld) now() time.Time { w.mu.Lock(); defer w.mu.Unlock(); return w.clock }

func (w *billingWorld) advance(d time.Duration) { w.mu.Lock(); w.clock = w.clock.Add(d); w.mu.Unlock() }

// newBillingWorld is a real SQLite-backed Scry whose learner has just
// started the trial with the scripted processor.
func newBillingWorld(t fataler, path string) *billingWorld {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w := &billingWorld{ctx: context.Background(), st: st, pay: newRapidPay(), alert: &alert.Recorder{},
		clock: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	w.app = &app.App{Store: st, Pay: w.pay, Model: &scriptedModel{verdict: srs.Right}, Alert: w.alert,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.now, Cfg: cfg}
	if w.user, err = st.CreateUser(w.ctx, "payer@example.com", "payer", "UTC", w.clock); err != nil {
		t.Fatal(err)
	}
	w.us = st.For(w.user.ID)
	if err := w.app.StartTrial(w.ctx, w.user, "tok_ok"); err != nil {
		t.Fatal(err)
	}
	return w
}

// ---------------------------------------------------------------- machine

var topupOutcomes = []string{"succeeded", "processing", "requires_action", "decline", "canceled", "error_ref", "lost_created", "lost_none"}

// topupReachable is the transitive closure of the top-up machine: an
// attempt seen in a state may only ever be seen again in one of these.
var topupReachable = map[store.TopupStatus][]store.TopupStatus{
	store.TopupStatusPending:   {store.TopupStatusPending, store.TopupStatusSucceeded, store.TopupStatusFailed, store.TopupStatusRefundDue},
	store.TopupStatusFailed:    {store.TopupStatusFailed, store.TopupStatusSucceeded, store.TopupStatusRefundDue},
	store.TopupStatusSucceeded: {store.TopupStatusSucceeded},
	store.TopupStatusRefundDue: {store.TopupStatusRefundDue},
}

type billingMachine struct {
	w      *billingWorld
	tokens []string
	// ghosts
	seen       map[string]store.TopupStatus // token -> last status seen
	drawn      int64                        // top-up micros spent by usage so far
	endedRefs  map[string]bool              // subscription refs seen ended
	lastRef    string
	lastEnd    time.Time
	lastUsed   int64
	refunds    int
	lateCredit int
}

func (m *billingMachine) sub(t *rapid.T) store.Subscription {
	sub, err := m.w.us.Subscription(m.w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// TopUp submits a top-up form: a new token, or a replay of one already
// submitted (which never charges again).
func (m *billingMachine) TopUp(t *rapid.T) {
	tok := rapid.SampledFrom(m.tokens).Draw(t, "token")
	m.w.pay.mu.Lock()
	m.w.pay.next = rapid.SampledFrom(topupOutcomes).Draw(t, "outcome")
	m.w.pay.lookupFail = rapid.IntRange(0, 4).Draw(t, "lookupFail") == 0
	m.w.pay.mu.Unlock()
	_, err := m.w.app.TopUpResult(m.w.ctx, m.w.user.ID, tok)
	var d *billing.DeclineError
	switch {
	case err == nil, errors.As(err, &d),
		errors.Is(err, app.ErrPlanEnded), errors.Is(err, store.ErrTopupCap),
		errors.Is(err, app.ErrTopupPending), errors.Is(err, app.ErrTopupUnconfirmed),
		errors.Is(err, app.ErrTopupReleased), errors.Is(err, store.ErrTopupCapExceededLate):
	default:
		t.Fatalf("top-up %s: unexpected error %v", tok, err)
	}
}

// TopUpBurst submits several forms in a row that the processor charges at
// once, so a month's cap is filled often (and a late success meets a full
// month).
func (m *billingMachine) TopUpBurst(t *rapid.T) {
	k := rapid.IntRange(2, 4).Draw(t, "burst")
	m.w.pay.mu.Lock()
	m.w.pay.next = "succeeded"
	m.w.pay.mu.Unlock()
	for i := 0; i < k; i++ {
		tok := rapid.SampledFrom(m.tokens).Draw(t, "token")
		_, err := m.w.app.TopUpResult(m.w.ctx, m.w.user.ID, tok)
		var d *billing.DeclineError
		switch {
		case err == nil, errors.As(err, &d),
			errors.Is(err, app.ErrPlanEnded), errors.Is(err, store.ErrTopupCap),
			errors.Is(err, app.ErrTopupPending), errors.Is(err, app.ErrTopupUnconfirmed),
			errors.Is(err, app.ErrTopupReleased), errors.Is(err, store.ErrTopupCapExceededLate):
		default:
			t.Fatalf("top-up %s: unexpected error %v", tok, err)
		}
	}
}

// Reconcile is the Billing page's (or the housekeeping sweep's) lookup of
// pending attempts.
func (m *billingMachine) Reconcile(t *rapid.T) {
	m.w.pay.mu.Lock()
	m.w.pay.lookupFail = rapid.IntRange(0, 4).Draw(t, "lookupFail") == 0
	m.w.pay.mu.Unlock()
	var err error
	if rapid.Bool().Draw(t, "sweep") {
		err = m.w.app.ReconcileAllTopUps(m.w.ctx)
	} else {
		_, err = m.w.app.ReconcileTopUps(m.w.ctx, m.w.user.ID)
	}
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

// ProcessorSettles moves one PaymentIntent on at the processor, which
// emits its webhook (not delivered yet).
func (m *billingMachine) ProcessorSettles(t *rapid.T) {
	p := m.w.pay
	p.mu.Lock()
	defer p.mu.Unlock()
	var keys []string
	for k, pi := range p.intents {
		if len(piMoves[pi.status]) > 0 {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		t.Skip("no intent can move")
	}
	sort.Strings(keys)
	pi := p.intents[rapid.SampledFrom(keys).Draw(t, "intent")]
	pi.status = rapid.SampledFrom(piMoves[pi.status]).Draw(t, "to")
	pi.reason = ""
	if pi.status == billing.PIRequiresPaymentMethod {
		pi.reason = "Your card has insufficient funds."
	}
	p.emitIntent(pi)
}

// Invoice bills the next period of the latest subscription, paid or
// failed. A failed invoice carries the period it was for (as Stripe's
// invoice.payment_failed lines do) or, drawn, no period at all (an
// invoice without lines, which the app cannot order).
func (m *billingMachine) Invoice(t *rapid.T) {
	p := m.w.pay
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.subs[len(p.subs)-1]
	if s.ended || s.invoiced >= 6 {
		t.Skip("no invoice to bill")
	}
	paid := rapid.Bool().Draw(t, "paid")
	k := s.paid + 1
	ev := billing.Event{ID: p.seq("evt_in"), SubscriptionRef: s.ref, AmountCents: 500, At: m.w.now()}
	if paid {
		ev.Kind, ev.PeriodStart, ev.PeriodEnd = billing.InvoicePaid, s.end0.AddDate(0, k-1, 0), s.end0.AddDate(0, k, 0)
		s.paid = k
	} else {
		ev.Kind, ev.Reason = billing.InvoiceFailed, "Your card was declined."
		if rapid.Bool().Draw(t, "failed_with_period") {
			ev.PeriodStart, ev.PeriodEnd = s.end0.AddDate(0, k-1, 0), s.end0.AddDate(0, k, 0)
		}
	}
	s.invoiced = k
	p.emitted = append(p.emitted, ev)
}

// EndPlan ends the latest subscription at the processor.
func (m *billingMachine) EndPlan(t *rapid.T) {
	p := m.w.pay
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.subs[len(p.subs)-1]
	if s.ended {
		t.Skip("already ended")
	}
	s.ended = true
	p.emitted = append(p.emitted, billing.Event{ID: p.seq("evt_end"), Kind: billing.SubscriptionEnded, SubscriptionRef: s.ref, At: m.w.now()})
}

// Deliver hands any event the processor ever emitted to the app's webhook
// path: duplicates, replays and out-of-order deliveries included.
func (m *billingMachine) Deliver(t *rapid.T) {
	m.w.pay.mu.Lock()
	evs := append([]billing.Event(nil), m.w.pay.emitted...)
	m.w.pay.mu.Unlock()
	if len(evs) == 0 {
		t.Skip("nothing emitted")
	}
	ev := evs[rapid.IntRange(0, len(evs)-1).Draw(t, "event")]
	if err := m.w.app.ApplyEvent(m.w.ctx, ev); err != nil {
		t.Fatalf("deliver %s %s: %v", ev.ID, ev.Kind, err)
	}
}

// Reopen starts a new plan after the old one ended (a new subscription).
func (m *billingMachine) Reopen(t *rapid.T) {
	if m.sub(t).Status != store.SubEnded {
		t.Skip("plan not ended")
	}
	if err := m.w.app.StartTrial(m.w.ctx, m.w.user, "tok_ok"); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s := m.sub(t); s.Status != store.SubActive {
		t.Fatalf("reopened plan is %s", s.Status)
	}
}

// Spend meters usage directly, sometimes far more than is left.
func (m *billingMachine) Spend(t *rapid.T) {
	cost := rapid.Int64Range(0, 9_000_000).Draw(t, "cost")
	before := m.sub(t)
	if err := m.w.us.Spend(m.w.ctx, store.Usage{Kind: "judge", Model: "test-judge", CostMicros: cost}, m.w.now()); err != nil {
		t.Fatal(err)
	}
	m.noteDraw(t, before)
}

// ReserveSettle holds a budget and settles it at an actual cost that may
// be larger than the hold.
func (m *billingMachine) ReserveSettle(t *rapid.T) {
	amount := rapid.Int64Range(1, 2_000_000).Draw(t, "amount")
	id := fmt.Sprintf("res-%d", rapid.Int64().Draw(t, "id"))
	sub := m.sub(t)
	err := m.w.us.ReserveAt(m.w.ctx, id, amount, m.w.now(), time.Duration(m.w.app.Cfg.GraceDays)*24*time.Hour)
	// The store's atomic guard agrees with app.HasAccess on every plan.
	if access := m.w.app.HasAccess(sub); errors.Is(err, store.ErrReservationNoAccess) == access {
		t.Fatalf("reserve on %s (past due since %v) = %v; HasAccess %v", sub.Status, sub.PastDueSince, err, access)
	}
	if errors.Is(err, store.ErrBudget) || errors.Is(err, store.ErrReservationNoAccess) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	before := m.sub(t)
	cost := rapid.Int64Range(0, 3*amount).Draw(t, "cost")
	if err := m.w.us.Settle(m.w.ctx, id, store.Usage{Kind: "draft", Model: "test-draft", CostMicros: cost}, m.w.now()); err != nil {
		t.Fatal(err)
	}
	m.noteDraw(t, before)
	if n, _ := m.w.us.ReservedMicros(m.w.ctx); n != 0 {
		t.Fatalf("settled reservation still held: %d", n)
	}
}

func (m *billingMachine) noteDraw(t *rapid.T, before store.Subscription) {
	after := m.sub(t)
	if d := before.TopupMicros - after.TopupMicros; d > 0 {
		m.drawn += d
	} else if d < 0 {
		t.Fatalf("usage added top-up credit: %d -> %d", before.TopupMicros, after.TopupMicros)
	}
}

// Wait moves the clock, across UTC month boundaries.
func (m *billingMachine) Wait(t *rapid.T) {
	m.w.advance(time.Duration(rapid.IntRange(1, 24*12).Draw(t, "hours")) * time.Hour)
}

type topupRow struct {
	token, chargeRef string
	status           store.TopupStatus
	created          time.Time
}

func (m *billingMachine) topups(t *rapid.T) []topupRow {
	rows, err := m.w.st.DB().Query(`SELECT token, charge_ref, status, created_at FROM topups WHERE user_id=?`, m.w.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []topupRow
	for rows.Next() {
		var r topupRow
		var at int64
		if err := rows.Scan(&r.token, &r.chargeRef, &r.status, &at); err != nil {
			t.Fatal(err)
		}
		r.created = time.Unix(at, 0).UTC()
		out = append(out, r)
	}
	return out
}

// Check holds after every step (the invariants of specs/billing_topup.qnt).
func (m *billingMachine) Check(t *rapid.T) {
	ctx, cfg := m.w.ctx, m.w.app.Cfg
	sub := m.sub(t)
	atts := m.topups(t)

	succeeded := map[string]bool{} // charge refs of succeeded attempts
	refundDue := 0
	perMonth := map[time.Time]int{}
	for _, a := range atts {
		if !a.status.Valid() {
			t.Fatalf("attempt %s in unknown state %q", a.token, a.status)
		}
		// Terminal states never regress; failed only moves forward.
		if prev, ok := m.seen[a.token]; ok && prev != a.status {
			okMove := false
			for _, s := range topupReachable[prev] {
				okMove = okMove || s == a.status
			}
			if !okMove {
				t.Fatalf("attempt %s regressed %s -> %s", a.token, prev, a.status)
			}
		}
		if a.status == store.TopupStatusRefundDue && m.seen[a.token] != store.TopupStatusRefundDue {
			m.refunds++
		}
		if a.status == store.TopupStatusSucceeded && m.seen[a.token] == store.TopupStatusFailed {
			m.lateCredit++
		}
		m.seen[a.token] = a.status
		switch a.status {
		case store.TopupStatusSucceeded:
			if a.chargeRef == "" {
				t.Fatalf("attempt %s succeeded with no charge_ref", a.token)
			}
			if succeeded[a.chargeRef] {
				t.Fatalf("charge %s credited to two attempts", a.chargeRef)
			}
			succeeded[a.chargeRef] = true
			fallthrough
		case store.TopupStatusPending:
			start, _ := store.MonthBounds(a.created)
			perMonth[start]++
		case store.TopupStatusRefundDue:
			refundDue++
			if a.chargeRef == "" {
				t.Fatalf("refund_due attempt %s has no charge_ref", a.token)
			}
		}
	}
	// The $20 UTC monthly cap: pending + succeeded per month.
	for month, n := range perMonth {
		if n > store.TopupCap {
			t.Fatalf("%d pending+succeeded top-ups in %s, over the cap of %d", n, month.Format("2006-01"), store.TopupCap)
		}
	}
	// Ledger: paid top-up lines are exactly the succeeded attempts' charges.
	led, err := m.w.us.Ledger(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paid := 0
	ledMonth := map[time.Time]int{}
	for _, e := range led {
		if e.Kind == "topup" && e.Status == "paid" {
			paid++
			if !succeeded[e.Ref] {
				t.Fatalf("paid top-up line %s has no succeeded attempt", e.Ref)
			}
			start, _ := store.MonthBounds(e.At)
			ledMonth[start]++
		}
	}
	if paid != len(succeeded) {
		t.Fatalf("%d paid top-up lines for %d succeeded attempts", paid, len(succeeded))
	}
	for month, n := range ledMonth {
		if n > store.TopupCap {
			t.Fatalf("%d paid top-ups booked in %s", n, month.Format("2006-01"))
		}
	}
	// Never double-credit: every micro of top-up credit ever granted is
	// still held or was drawn by usage, and equals one credit per success.
	if credited := sub.TopupMicros + m.drawn; credited != int64(len(succeeded))*cfg.TopupMicros {
		t.Fatalf("credited %d micros for %d succeeded top-ups (held %d, drawn %d)", credited, len(succeeded), sub.TopupMicros, m.drawn)
	}
	// Balances.
	if sub.UsedMicros < 0 || sub.UsedMicros > sub.AllowanceMicros || sub.TopupMicros < 0 {
		t.Fatalf("balances out of range: used %d of %d, top-up %d", sub.UsedMicros, sub.AllowanceMicros, sub.TopupMicros)
	}
	// The plan: ended never regresses for its subscription; the period
	// never moves back; a period's usage is never reset without a new one.
	if m.endedRefs[sub.SubscriptionRef] && sub.Status != store.SubEnded {
		t.Fatalf("ended subscription %s came back as %s", sub.SubscriptionRef, sub.Status)
	}
	if sub.Status == store.SubEnded {
		m.endedRefs[sub.SubscriptionRef] = true
	}
	if sub.SubscriptionRef == m.lastRef {
		if sub.PeriodEnd.Before(m.lastEnd) {
			t.Fatalf("period moved back: %v -> %v", m.lastEnd, sub.PeriodEnd)
		}
		if sub.PeriodEnd.Equal(m.lastEnd) && sub.UsedMicros < m.lastUsed {
			t.Fatalf("usage reset within the period: %d -> %d", m.lastUsed, sub.UsedMicros)
		}
	}
	if sub.Status == store.SubPastDue && sub.PastDueSince.IsZero() {
		t.Fatal("past_due with no past_due_since")
	}
	m.lastRef, m.lastEnd, m.lastUsed = sub.SubscriptionRef, sub.PeriodEnd, sub.UsedMicros
	// Every refund due was reported to ops.
	if refundDue > 0 && countTitle(m.w.alert, "Top-up refund due") == 0 {
		t.Fatal("refund due with no alert")
	}
}

func countTitle(r *alert.Recorder, title string) int {
	n := 0
	for _, t := range r.Titles() {
		if strings.HasPrefix(t, title) {
			n++
		}
	}
	return n
}

// TestRapidBillingStateMachine drives the real SQLite store and app with
// top-ups (new and replayed tokens, every processor outcome), processor-side
// settlement, reconciliation, subscription and top-up webhooks delivered
// duplicated and out of order, usage, month rollover and reopening, and
// checks the billing invariants of specs/billing_topup.qnt after every step.
func TestRapidBillingStateMachine(t *testing.T) {
	dir := rapidDir(t)
	var n atomic.Int64
	var refunds, late atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		w := newBillingWorld(t, filepath.Join(dir, fmt.Sprintf("billing%d.db", n.Add(1))))
		defer w.st.Close()
		count := rapid.IntRange(2, 9).Draw(t, "tokens")
		m := &billingMachine{w: w, seen: map[string]store.TopupStatus{}, endedRefs: map[string]bool{}}
		for i := 0; i < count; i++ {
			m.tokens = append(m.tokens, fmt.Sprintf("rapid-token-%010d", i))
		}
		t.Repeat(rapid.StateMachineActions(m))
		refunds.Add(int64(m.refunds))
		late.Add(int64(m.lateCredit))
	})
	t.Logf("runs: %d, refund_due transitions: %d, late failed->succeeded credits: %d", n.Load(), refunds.Load(), late.Load())
}

// ---------------------------------------------------------------- regressions

// Bug 1.1 end to end: a declined top-up's charge succeeds after four other
// top-ups filled the month. Its webhook must not credit a fifth $5.
func TestLateTopupSuccessOverCapIsRefundDue(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "late.db"))
	tok := func(i int) string { return fmt.Sprintf("late-cap-token-%04d", i) }
	w.pay.next = "decline"
	var d *billing.DeclineError
	if _, err := w.app.TopUpResult(w.ctx, w.user.ID, tok(0)); !errors.As(err, &d) {
		t.Fatalf("decline: %v", err)
	}
	w.pay.next = "succeeded"
	for i := 1; i <= 4; i++ {
		if ok, err := w.app.TopUpResult(w.ctx, w.user.ID, tok(i)); err != nil || !ok {
			t.Fatalf("top-up %d: %v %v", i, ok, err)
		}
	}
	key := fmt.Sprintf("topup-%d-%s", w.user.ID, tok(0))
	pi := w.pay.intents[key]
	pi.status, pi.reason = billing.PISucceeded, ""
	w.pay.emitIntent(pi)
	ev := w.pay.emitted[len(w.pay.emitted)-1]
	for range 2 { // and its replay
		if err := w.app.ApplyEvent(w.ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	att, _ := w.us.Topup(w.ctx, tok(0))
	sub, _ := w.us.Subscription(w.ctx)
	if att.Status != store.TopupStatusRefundDue || sub.TopupMicros != 4*w.app.Cfg.TopupMicros {
		t.Fatalf("late success: %s, credit %d; want refund_due and $16 of credit", att.Status, sub.TopupMicros)
	}
	if countTitle(w.alert, "Top-up refund due") != 1 {
		t.Fatalf("alerts: %v", w.alert.Titles())
	}
	// Resubmitting the form says so, and charges nothing.
	if _, err := w.app.TopUpResult(w.ctx, w.user.ID, tok(0)); !errors.Is(err, store.ErrTopupCapExceededLate) {
		t.Fatalf("resubmit: %v", err)
	}
}

func (w *billingWorld) deliver(t *testing.T, ev billing.Event) {
	t.Helper()
	if err := w.app.ApplyEvent(w.ctx, ev); err != nil {
		t.Fatal(err)
	}
}

// Bug 1.2: late invoice events never resurrect an ended plan.
func TestEndedPlanIsNotResurrectedByLateInvoices(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "ended.db"))
	sub, _ := w.us.Subscription(w.ctx)
	w.deliver(t, billing.Event{ID: "evt_end", Kind: billing.SubscriptionEnded, SubscriptionRef: sub.SubscriptionRef})
	w.deliver(t, billing.Event{ID: "evt_paid", Kind: billing.InvoicePaid, SubscriptionRef: sub.SubscriptionRef, AmountCents: 500,
		PeriodStart: sub.PeriodEnd, PeriodEnd: sub.PeriodEnd.AddDate(0, 1, 0)})
	w.deliver(t, billing.Event{ID: "evt_fail", Kind: billing.InvoiceFailed, SubscriptionRef: sub.SubscriptionRef, Reason: "declined"})
	got, _ := w.us.Subscription(w.ctx)
	if got.Status != store.SubEnded || !got.PeriodEnd.Equal(sub.PeriodEnd) {
		t.Fatalf("ended plan moved: %s %v", got.Status, got.PeriodEnd)
	}
	if _, err := w.app.Plan(w.ctx, w.user.ID); !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("access after end: %v", err)
	}
	if countTitle(w.alert, "Payment on an ended plan") != 1 {
		t.Fatalf("alerts: %v", w.alert.Titles())
	}
	// Only an explicit new plan opens again, under a new subscription.
	if err := w.app.StartTrial(w.ctx, w.user, "tok_ok"); err != nil {
		t.Fatal(err)
	}
	re, _ := w.us.Subscription(w.ctx)
	if re.Status != store.SubActive || re.SubscriptionRef == sub.SubscriptionRef {
		t.Fatalf("reopen: %+v", re)
	}
	// Events for the old subscription match nothing now.
	w.deliver(t, billing.Event{ID: "evt_end_again", Kind: billing.SubscriptionEnded, SubscriptionRef: sub.SubscriptionRef})
	if again, _ := w.us.Subscription(w.ctx); again.Status != store.SubActive {
		t.Fatalf("old subscription's event ended the new plan: %s", again.Status)
	}
}

// Bug 1.2: an out-of-order invoice for an older period never moves the
// period back or hands out a fresh allowance; past_due recovery and
// forward renewals still work.
func TestStaleInvoiceDoesNotRollBackThePeriod(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "stale.db"))
	sub, _ := w.us.Subscription(w.ctx)
	end0 := sub.PeriodEnd
	inv := func(id string, k int) billing.Event {
		return billing.Event{ID: id, Kind: billing.InvoicePaid, SubscriptionRef: sub.SubscriptionRef, AmountCents: 500,
			PeriodStart: end0.AddDate(0, k-1, 0), PeriodEnd: end0.AddDate(0, k, 0)}
	}
	w.deliver(t, inv("evt_p2", 2))
	if err := w.us.Spend(w.ctx, store.Usage{Kind: "judge", CostMicros: 1000}, w.now()); err != nil {
		t.Fatal(err)
	}
	w.deliver(t, inv("evt_p1", 1)) // older, delivered late
	got, _ := w.us.Subscription(w.ctx)
	if !got.PeriodEnd.Equal(end0.AddDate(0, 2, 0)) || got.UsedMicros != 1000 || got.Status != store.SubActive {
		t.Fatalf("stale invoice applied: end %v used %d %s", got.PeriodEnd, got.UsedMicros, got.Status)
	}
	// Both payments are in the history.
	led, _ := w.us.Ledger(w.ctx)
	charges := 0
	for _, e := range led {
		if e.Kind == "charge" && e.Status == "paid" {
			charges++
		}
	}
	if charges != 2 {
		t.Fatalf("%d paid charge lines, want 2", charges)
	}
	// past_due, then the next period is paid: active again, fresh period.
	w.deliver(t, billing.Event{ID: "evt_f3", Kind: billing.InvoiceFailed, SubscriptionRef: sub.SubscriptionRef, Reason: "declined"})
	if got, _ := w.us.Subscription(w.ctx); got.Status != store.SubPastDue {
		t.Fatalf("failure: %s", got.Status)
	}
	w.deliver(t, inv("evt_p3", 3))
	got, _ = w.us.Subscription(w.ctx)
	if got.Status != store.SubActive || !got.PeriodEnd.Equal(end0.AddDate(0, 3, 0)) || got.UsedMicros != 0 || !got.PastDueSince.IsZero() {
		t.Fatalf("recovery: %+v", got)
	}
}

// panicModel panics inside the draft call (after the provider was asked).
type panicModel struct{ scriptedModel }

func (*panicModel) Draft(context.Context, ai.DraftInput, ai.DraftSink) (ai.DraftResult, ai.Usage, error) {
	panic("model adapter bug")
}

// failingSpeech always fails.
type failingSpeech struct{}

func (failingSpeech) Name() string { return "test" }
func (failingSpeech) Transcribe(context.Context, speech.Audio) (string, error) {
	return "", errors.New("speech provider down")
}

// Bug 1.3: Transcribe never leaves its reservation held when the speech
// provider fails; the known cost of the recording is settled.
func TestTranscribeFailureSettlesItsReservation(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "speech.db"))
	w.app.Speech = failingSpeech{}
	if _, err := w.app.Transcribe(w.ctx, w.user, speech.Audio{Data: []byte("say:hello")}); !errors.Is(err, app.ErrSpeechUnavailable) {
		t.Fatalf("transcribe: %v", err)
	}
	if n, _ := w.us.ReservedMicros(w.ctx); n != 0 {
		t.Fatalf("reservation leaked: %d", n)
	}
	sub, _ := w.us.Subscription(w.ctx)
	if sub.UsedMicros != 50 { // one second of speech
		t.Fatalf("speech cost not settled: %d", sub.UsedMicros)
	}
}

// A draft that panics before the model is called releases its
// reservation; one that panics inside the model call keeps it held (the
// provider may have charged: G2), and the draft is marked failed.
func TestDraftPanicReservation(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "draft.db"))
	w.app.Model = &panicModel{}
	id, err := w.app.Learn(w.ctx, w.user, "physics")
	if err != nil {
		t.Fatal(err)
	}
	w.app.WaitDrafts()
	d, err := w.us.Draft(w.ctx, id)
	if err != nil || d.Status != "failed" {
		t.Fatalf("draft after panic: %+v %v", d, err)
	}
	if n, _ := w.us.ReservedMicros(w.ctx); n != ai.DraftBudgetMicros {
		t.Fatalf("held %d, want the draft budget held for an unknown outcome", n)
	}
}
