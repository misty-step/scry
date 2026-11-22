package app_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/srs"
	"scry/internal/store"
)

// spyPay is the test processor, recording every call that could reach a
// card processor, so a test can prove a no-card trial never does.
type spyPay struct {
	billing.Fake
	mu         sync.Mutex
	subscribes []billing.SubscribeParams
	charges    []string // customer refs
	cancels    []string // subscription refs
	updates    []string // customer refs
}

func (p *spyPay) Subscribe(ctx context.Context, sp billing.SubscribeParams) (billing.Subscribed, error) {
	p.mu.Lock()
	p.subscribes = append(p.subscribes, sp)
	p.mu.Unlock()
	return p.Fake.Subscribe(ctx, sp)
}

func (p *spyPay) Charge(ctx context.Context, customer, payment string, cents int64, desc, key string) (string, error) {
	p.mu.Lock()
	p.charges = append(p.charges, customer)
	p.mu.Unlock()
	return p.Fake.Charge(ctx, customer, payment, cents, desc, key)
}

func (p *spyPay) SetCancelAtPeriodEnd(ctx context.Context, ref string, cancel bool) error {
	p.mu.Lock()
	p.cancels = append(p.cancels, ref)
	p.mu.Unlock()
	return p.Fake.SetCancelAtPeriodEnd(ctx, ref, cancel)
}

func (p *spyPay) UpdateCard(ctx context.Context, customer, sub, token string) (string, billing.Card, error) {
	p.mu.Lock()
	p.updates = append(p.updates, customer)
	p.mu.Unlock()
	return p.Fake.UpdateCard(ctx, customer, sub, token)
}

type trialWorld struct {
	ctx   context.Context
	st    *store.Store
	app   *app.App
	pay   *spyPay
	user  store.User
	us    *store.UserStore
	mu    sync.Mutex
	clock time.Time
}

func (w *trialWorld) now() time.Time { w.mu.Lock(); defer w.mu.Unlock(); return w.clock }

func (w *trialWorld) set(t time.Time) { w.mu.Lock(); w.clock = t; w.mu.Unlock() }

// newTrialWorld is a real SQLite-backed Scry whose learner has just signed
// up and so is on the no-card trial.
func newTrialWorld(t *testing.T) *trialWorld {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "trial.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	w := &trialWorld{ctx: context.Background(), st: st, pay: &spyPay{}, clock: time.Date(2026, 11, 21, 9, 30, 0, 500_000_000, time.UTC)}
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	w.app = &app.App{Store: st, Auth: nil, Pay: w.pay, Model: &ai.Fake{}, Explainer: &ai.FakeExplainer{}, Speech: speech.Fake{},
		Alert: &alert.Recorder{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.now, Cfg: cfg}
	if w.user, err = w.app.CreateUser(w.ctx, "trial@example.com", "trial-subject", "UTC"); err != nil {
		t.Fatal(err)
	}
	w.us = st.For(w.user.ID)
	return w
}

func (w *trialWorld) sub(t *testing.T) store.Subscription {
	t.Helper()
	s, err := w.us.Subscription(w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Slice 36: reservations are priced by the configured model, at its
// catalog rate, and an unknown model still reserves the $8/$40 ceiling.
func TestSlice36_ModelAwareReservationBudgets(t *testing.T) {
	jev := &ai.Jev{APIKey: "or-standin", OpenRouter: true}
	claude, err := ai.NewClaudeWithRouter("", "or-standin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a := &app.App{Model: ai.Split{JudgeProvider: jev, DraftProvider: claude}, Explainer: claude, Speech: speech.OpenRouter{APIKey: "or-standin"}, Cfg: app.DefaultConfig()}
	for _, tc := range []struct {
		name      string
		got, want int64
	}{
		{"judge typesafe/jev-1.14 at $0.10/$1", a.JudgeBudget(), 1946},        // 9,216 x 0.1 + 1,024 x 1
		{"draft openai/gpt-6.2-sol at $2/$10", a.DraftBudget(), 226048},       // 33,024 x 2 + 16,000 x 10
		{"explain openai/gpt-6-luna at $0.10/$0.50", a.ExplainBudget(), 768},  // 2,560 x 0.1 + 1,024 x 0.5
		{"speech microsoft/mai-transcribe-2, 120 s", a.SpeechBudget(), 30000}, // max(120 s x 250, 15,000)
	} {
		if tc.got != tc.want {
			t.Errorf("%s: %d micros, want %d", tc.name, tc.got, tc.want)
		}
	}
	// Unknown models (an override nobody priced) reserve the ceiling,
	// exactly the ai package's fixed budgets.
	jev.Model = "vendor/new-decider"
	claude.DraftModel, claude.ExplainModel = "vendor/new-writer", "vendor/new-explainer"
	if a.JudgeBudget() != ai.JudgeBudgetMicros || a.DraftBudget() != ai.DraftBudgetMicros || a.ExplainBudget() != ai.ExplainBudgetMicros {
		t.Fatalf("unknown models: judge %d draft %d explain %d; want the ceilings %d %d %d",
			a.JudgeBudget(), a.DraftBudget(), a.ExplainBudget(), ai.JudgeBudgetMicros, ai.DraftBudgetMicros, ai.ExplainBudgetMicros)
	}
	// An unwired or unrecognised adapter is priced at the ceiling too.
	if b := (&app.App{Model: &scriptedModel{}}); b.JudgeBudget() != ai.JudgeBudgetMicros || b.DraftBudget() != ai.DraftBudgetMicros || b.ExplainBudget() != ai.ExplainBudgetMicros {
		t.Fatalf("unrecognised adapters: %d %d %d", b.JudgeBudget(), b.DraftBudget(), b.ExplainBudget())
	}
	// Metered cost: a resolved, dated model id is priced as its model; an
	// unknown one at the ceiling; a provider-reported cost always wins.
	if c := app.Cost(ai.Usage{Model: "typesafe/jev-1.14-20261014", InputTokens: 400, OutputTokens: 10}); c != 50 {
		t.Errorf("dated jev cost %d, want 50", c)
	}
	if c := app.Cost(ai.Usage{Model: "openai/gpt-6.2-sol", InputTokens: 1500, OutputTokens: 6000}); c != 63000 {
		t.Errorf("gpt-6.2-sol draft cost %d, want 63000", c)
	}
	if c := app.Cost(ai.Usage{Model: "vendor/unknown", InputTokens: 1, OutputTokens: 1}); c != 48 {
		t.Errorf("unknown model cost %d, want the $8/$40 ceiling (48)", c)
	}
	// What Billing says the allowance covers reflects gpt-6.2-sol.
	if n := app.DefaultConfig().DraftsPerAllowance(); n != 47 {
		t.Errorf("DraftsPerAllowance = %d, want 47 ($3 / $0.063)", n)
	}
}

// Slice 36: the no-card trial's cap is small (at most $1, here $0.50) yet
// covers a worst-case draft, judgment, explanation and recording at once
// with the default models, so a trial learner can do everything.
func TestSlice36_TrialCapCoversEverySurfaceAndStaysSmall(t *testing.T) {
	jev := &ai.Jev{APIKey: "or-standin", OpenRouter: true}
	claude, _ := ai.NewClaudeWithRouter("", "or-standin", "", "")
	a := &app.App{Model: ai.Split{JudgeProvider: jev, DraftProvider: claude}, Explainer: claude, Speech: speech.OpenRouter{APIKey: "or-standin"}, Cfg: app.DefaultConfig()}
	cfg := a.Cfg
	if cfg.TrialMicros != 500_000 || cfg.TrialMicros > 1_000_000 {
		t.Fatalf("TrialMicros = %d", cfg.TrialMicros)
	}
	if all := a.DraftBudget() + a.JudgeBudget() + a.ExplainBudget() + a.SpeechBudget(); all > cfg.TrialMicros {
		t.Fatalf("one of each worst case (%d) does not fit the trial cap (%d)", all, cfg.TrialMicros)
	}
	// Typical use, at catalog rates: 50 judgments, 15 explanations, 20
	// drafts and 5 minutes of speech, all inside the cap.
	typical := 50*app.Cost(ai.Usage{Model: "typesafe/jev-1.14", InputTokens: 400, OutputTokens: 10}) +
		15*app.Cost(ai.Usage{Model: "openai/gpt-6-luna", InputTokens: 300, OutputTokens: 60}) +
		20*app.Cost(ai.Usage{Model: "openai/gpt-6.2-sol", InputTokens: 1000, OutputTokens: 900}) +
		300*50
	if typical > cfg.TrialMicros {
		t.Fatalf("typical trial use %d micros exceeds the cap %d", typical, cfg.TrialMicros)
	}
}

// Slice 36: a new account's no-card trial is had once, opens with no
// processor call, and gives access exactly until period_end: HasAccess,
// the store's atomic reserve guard and Plan all agree, Plan ends it, and
// neither renewal nor a processor event ever turns it into a paid plan.
func TestSlice36_NoCardTrialExpiresAtPeriodEnd(t *testing.T) {
	w := newTrialWorld(t)
	sub := w.sub(t)
	start := time.Unix(w.now().Unix(), 0).UTC()
	if !sub.NoCardTrial() || sub.CustomerRef != "" || sub.PaymentRef != "" || sub.CardLast4 != "" || sub.SubscriptionRef != store.NoCardTrialRef(w.user.ID) ||
		!sub.PeriodStart.Equal(start) || !sub.PeriodEnd.Equal(start.AddDate(0, 0, 7)) || !sub.TrialEnd.Equal(sub.PeriodEnd) ||
		sub.AllowanceMicros != w.app.Cfg.TrialMicros || sub.UsedMicros != 0 {
		t.Fatalf("no-card trial %+v", sub)
	}
	if len(w.pay.subscribes) != 0 {
		t.Fatal("starting the no-card trial called the processor")
	}
	if err := w.app.StartNoCardTrial(w.ctx, w.user.ID); err != nil || w.sub(t) != sub {
		t.Fatalf("a second trial: %v %+v", err, w.sub(t))
	}
	if led, _ := w.us.Ledger(w.ctx); len(led) != 1 || led[0].Kind != "trial" || led[0].AmountCents != 0 {
		t.Fatalf("ledger %+v", led)
	}
	grace := 7 * 24 * time.Hour
	end := sub.PeriodEnd
	for _, at := range []time.Time{end.Add(-time.Second), end.Add(-time.Nanosecond)} {
		w.set(at)
		if !w.app.HasAccess(sub) {
			t.Fatalf("no access at %v, before the trial's end %v", at, end)
		}
		id := fmt.Sprintf("before-%d", at.UnixNano())
		if err := w.us.ReserveAt(w.ctx, id, 1000, at, grace); err != nil {
			t.Fatalf("reserve at %v: %v", at, err)
		}
		if err := w.us.Settle(w.ctx, id, store.Usage{}, at); err != nil {
			t.Fatal(err)
		}
	}
	// From period_end on, before anything has moved the row: refused by
	// both, and the store refuses atomically.
	for _, at := range []time.Time{end, end.Add(time.Nanosecond), end.Add(999 * time.Millisecond), end.Add(time.Hour)} {
		w.set(at)
		if w.app.HasAccess(sub) {
			t.Fatalf("access at %v, at or after the trial's end %v", at, end)
		}
		if err := w.us.ReserveAt(w.ctx, "after", 1, at, grace); !errors.Is(err, store.ErrReservationNoAccess) {
			t.Fatalf("reserve at %v = %v; want ErrReservationNoAccess", at, err)
		}
	}
	// The test processor renews plans by the clock: a no-card trial is
	// never renewed into a paid plan, and nothing is charged.
	if err := w.app.RunRenewals(w.ctx); err != nil {
		t.Fatal(err)
	}
	if s := w.sub(t); s.Status != store.SubEnded || s.CustomerRef != "" || s.AllowanceMicros != w.app.Cfg.TrialMicros {
		t.Fatalf("after renewals: %+v", s)
	}
	if _, err := w.app.Plan(w.ctx, w.user.ID); !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("Plan after the trial = %v", err)
	}
	if _, err := w.app.Learn(w.ctx, w.user, "physics"); !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("Learn after the trial = %v", err)
	}
	// A forged or stray processor event naming the trial's ref never
	// brings it back.
	if err := w.app.ApplyEvent(w.ctx, billing.Event{ID: "evt_forged", Kind: billing.InvoicePaid, SubscriptionRef: sub.SubscriptionRef,
		AmountCents: 500, PeriodStart: end, PeriodEnd: end.AddDate(0, 1, 0)}); err != nil {
		t.Fatal(err)
	}
	if s := w.sub(t); s.Status != store.SubEnded || s.AllowanceMicros != w.app.Cfg.TrialMicros {
		t.Fatalf("processor event moved a no-card trial: %+v", s)
	}
	if led, _ := w.us.Ledger(w.ctx); len(led) != 1 || len(w.pay.charges) != 0 {
		t.Fatalf("an expired no-card trial was charged: %+v %v", led, w.pay.charges)
	}
}

// Slice 36: a processor event naming a live no-card trial is ignored too,
// and Plan ends an expired trial by itself (whatever the processor).
func TestSlice36_PlanEndsAnExpiredTrialAndEventsCannotMoveIt(t *testing.T) {
	w := newTrialWorld(t)
	sub := w.sub(t)
	if err := w.app.ApplyEvent(w.ctx, billing.Event{ID: "evt_stray", Kind: billing.InvoicePaid, SubscriptionRef: sub.SubscriptionRef, AmountCents: 500}); err != nil {
		t.Fatal(err)
	}
	if s := w.sub(t); !s.NoCardTrial() || s.AllowanceMicros != w.app.Cfg.TrialMicros {
		t.Fatalf("event moved the trial: %+v", s)
	}
	w.set(sub.PeriodEnd)
	got, err := w.app.Plan(w.ctx, w.user.ID)
	if !errors.Is(err, app.ErrPlanEnded) || got.Status != store.SubEnded {
		t.Fatalf("Plan at period_end: %+v %v", got, err)
	}
	// The housekeeping sweep ends the trials nobody visits.
	other, err := w.app.CreateUser(w.ctx, "idle@example.com", "idle-subject", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	w.set(w.now().AddDate(0, 0, 8))
	if n, err := w.app.EndExpiredTrials(w.ctx); err != nil || n != 1 {
		t.Fatalf("EndExpiredTrials = %d, %v", n, err)
	}
	if s, _ := w.st.For(other.ID).Subscription(w.ctx); s.Status != store.SubEnded {
		t.Fatalf("idle trial %+v", s)
	}
}

// Slice 36: the trial's cap is hard under concurrency, and no goroutine
// racing the trial's end can take a hold after it, even one that read the
// plan (or passed Plan) just before.
func TestSlice36_ConcurrentReservationsCannotPassTheCapOrTheEnd(t *testing.T) {
	w := newTrialWorld(t)
	sub := w.sub(t)
	grace := 7 * 24 * time.Hour
	var wg sync.WaitGroup
	var mu sync.Mutex
	held := int64(0)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			amount := int64(60_000)
			if err := w.us.ReserveAt(w.ctx, fmt.Sprintf("cap-%d", i), amount, sub.PeriodEnd.Add(-time.Minute), grace); err == nil {
				mu.Lock()
				held += amount
				mu.Unlock()
			} else if !errors.Is(err, store.ErrBudget) {
				t.Errorf("reserve: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if r, _ := w.us.ReservedMicros(w.ctx); r != held || held > w.app.Cfg.TrialMicros || held != 8*60_000 {
		t.Fatalf("held %d (store %d) against a cap of %d", held, r, w.app.Cfg.TrialMicros)
	}
	if err := w.us.ReserveAt(w.ctx, "over", w.app.Cfg.TrialMicros, sub.PeriodEnd.Add(-time.Minute), grace); !errors.Is(err, store.ErrBudget) {
		t.Fatalf("over the cap: %v", err)
	}
	for i := 0; i < 40; i++ { // a settle of an id that was never held is a no-op
		if err := w.us.Settle(w.ctx, fmt.Sprintf("cap-%d", i), store.Usage{}, w.now()); err != nil {
			t.Fatal(err)
		}
	}
	// Racing the end: holds asked for at or after period_end are all
	// refused while EndExpiredNoCardTrial runs alongside.
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			at := sub.PeriodEnd.Add(time.Duration(i) * time.Millisecond)
			if err := w.us.ReserveAt(w.ctx, fmt.Sprintf("late-%d", i), 1, at, grace); !errors.Is(err, store.ErrReservationNoAccess) {
				t.Errorf("reserve at %v after the end = %v", at, err)
			}
		}(i)
		go func() {
			defer wg.Done()
			if _, err := w.us.EndExpiredNoCardTrial(w.ctx, sub.PeriodEnd); err != nil {
				t.Errorf("end: %v", err)
			}
		}()
	}
	wg.Wait()
	if r, _ := w.us.ReservedMicros(w.ctx); r != 0 {
		t.Fatalf("%d micros held after the trial ended", r)
	}
}

// Slice 36: with the trial allowance used up, judging, explaining,
// drafting and speech are refused before any provider call, and a top-up
// is refused without a charge: the only way on is adding a card.
func TestSlice36_ExhaustedTrialRefusesEveryAISurface(t *testing.T) {
	w := newTrialWorld(t)
	if _, err := w.us.ChangeSubscription(w.ctx, func(s *store.Subscription) { s.UsedMicros = s.AllowanceMicros }); err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.Transcribe(w.ctx, w.user, speech.Audio{Data: wav16k(16000), MIME: "audio/wav"}); !errors.Is(err, app.ErrAllowance) {
		t.Fatalf("speech on an exhausted trial = %v", err)
	}
	id, err := w.app.Learn(w.ctx, w.user, "physics")
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := w.us.Draft(w.ctx, id); d.Status != "allowance" {
		t.Fatalf("draft on an exhausted trial: %+v", d)
	}
	if _, err := w.app.TopUpResult(w.ctx, w.user.ID, "exhausted-trial-token-01"); !errors.Is(err, app.ErrTopupNeedsCard) {
		t.Fatalf("top-up = %v", err)
	}
	if len(w.pay.charges) != 0 {
		t.Fatal("a no-card trial reached the processor's charge")
	}
	if r, _ := w.us.ReservedMicros(w.ctx); r != 0 {
		t.Fatalf("%d held", r)
	}
}

// Slice 36: adding a card. Inside the window the trial keeps its end and
// nothing is charged; with under an hour left, or after it ended, the paid
// plan starts at once and is charged today, with no second free trial.
// Cancel and card update never send a no-card trial's empty refs anywhere.
func TestSlice36_UpgradeFromTheNoCardTrial(t *testing.T) {
	t.Run("inside the window keeps the trial end", func(t *testing.T) {
		w := newTrialWorld(t)
		old := w.sub(t)
		w.set(w.now().Add(3 * 24 * time.Hour))
		if _, err := w.us.ChangeSubscription(w.ctx, func(s *store.Subscription) { s.UsedMicros = 450_000 }); err != nil {
			t.Fatal(err)
		}
		if _, err := w.app.SetCancel(w.ctx, w.user.ID, true); err != nil || len(w.pay.cancels) != 0 || w.sub(t).CancelAtEnd {
			t.Fatalf("cancel on a no-card trial: %v %v", err, w.pay.cancels)
		}
		if err := w.app.UpdateCard(w.ctx, w.user.ID, "tok_x"); !errors.Is(err, app.ErrNoCardOnFile) || len(w.pay.updates) != 0 {
			t.Fatalf("UpdateCard on a no-card trial: %v %v", err, w.pay.updates)
		}
		if err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), app.TermsTrial); err != nil {
			t.Fatal(err)
		}
		s := w.sub(t)
		if len(w.pay.subscribes) != 1 || !w.pay.subscribes[0].TrialEnd.Equal(old.PeriodEnd) {
			t.Fatalf("processor asked for %+v, want trial_end %v", w.pay.subscribes, old.PeriodEnd)
		}
		if s.Status != store.SubTrialing || s.NoCard() || s.CardLast4 != "4242" || !s.TrialEnd.Equal(old.TrialEnd) || !s.PeriodEnd.Equal(old.PeriodEnd) ||
			!s.PeriodStart.Equal(old.PeriodStart) || s.UsedMicros != 450_000 || s.AllowanceMicros != w.app.Cfg.CardTrialMicros {
			t.Fatalf("card added: %+v", s)
		}
		if led, _ := w.us.Ledger(w.ctx); len(led) != 1 {
			t.Fatalf("charged today: %+v", led)
		}
		// Now a top-up charges the card on file, and a second card add is a no-op.
		if ok, err := w.app.TopUpResult(w.ctx, w.user.ID, "after-card-token-0001"); err != nil || !ok || len(w.pay.charges) != 1 || w.pay.charges[0] != s.CustomerRef {
			t.Fatalf("top-up with a card: %v %v %v", ok, err, w.pay.charges)
		}
		if err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), app.TermsTrial); err != nil || len(w.pay.subscribes) != 1 {
			t.Fatalf("second card add: %v %d", err, len(w.pay.subscribes))
		}
		// At the trial's end the test processor renews it as a paid plan.
		w.set(old.PeriodEnd.Add(time.Minute))
		if err := w.app.RunRenewals(w.ctx); err != nil {
			t.Fatal(err)
		}
		if s := w.sub(t); s.Status != store.SubActive || s.AllowanceMicros != w.app.Cfg.AllowanceMicros {
			t.Fatalf("after the trial: %+v", s)
		}
	})
	for _, tc := range []struct {
		name string
		at   func(end time.Time) time.Time
	}{
		{"under an hour left charges today", func(end time.Time) time.Time { return end.Add(-59 * time.Minute) }},
		{"after the end charges today", func(end time.Time) time.Time { return end.Add(2 * 24 * time.Hour) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTrialWorld(t)
			old := w.sub(t)
			w.set(tc.at(old.PeriodEnd))
			w.app.Plan(w.ctx, w.user.ID) // a visit, as the card page makes
			if err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), app.TermsImmediate); err != nil {
				t.Fatal(err)
			}
			s := w.sub(t)
			if len(w.pay.subscribes) != 1 || !w.pay.subscribes[0].TrialEnd.IsZero() {
				t.Fatalf("processor asked for a trial: %+v", w.pay.subscribes)
			}
			led, _ := w.us.Ledger(w.ctx)
			if s.Status != store.SubActive || s.NoCard() || s.AllowanceMicros != w.app.Cfg.AllowanceMicros || s.UsedMicros != 0 ||
				len(led) != 2 || led[0].Kind != "charge" || led[0].AmountCents != 500 || led[0].Status != "paid" {
				t.Fatalf("paid plan: %+v ledger %+v", s, led)
			}
		})
	}
}

func goodToken(t *testing.T, w *trialWorld) string {
	t.Helper()
	tok, err := w.pay.Tokenize("4242 4242 4242 4242", "12/30", "123", w.now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// wav16k is a mono 16 kHz PCM recording of n audible samples.
func wav16k(n int) []byte {
	b := make([]byte, 44+n*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(n*2))
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(int16(4000*((i%32)-16)/16)))
	}
	return b
}

// Slice 36 integration: App.Transcribe through the real OpenRouter speech
// adapter on both routes. The default model uploads the recording as
// audio.wav to /api/v1/audio/transcriptions; a chat-audio model sends it
// as input_audio to /api/v1/chat/completions. Each recording is metered at
// the larger of its per-second price and OpenRouter's usage.cost, under
// the configured model's name, and a provider failure is still metered.
func TestSlice36_TranscribeMetersBothSpeechRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, model, path, body string
		status                  int
		wantText                string
		wantErr                 error
		wantCost                int64
	}{
		{"transcriptions, cost under the per-second price", "", "/api/v1/audio/transcriptions", `{"text":" four "}`, 200, "four", nil, 100},
		{"transcriptions, provider cost above it", "", "/api/v1/audio/transcriptions", `{"text":"four","usage":{"cost":0.0042}}`, 200, "four", nil, 4200},
		{"transcriptions, provider failure still metered", "", "/api/v1/audio/transcriptions", `{"error":{"message":"boom"}}`, 502, "", app.ErrSpeechUnavailable, 100},
		{"chat input_audio", "google/gemini-3.1-flash-lite", "/api/v1/chat/completions", `{"choices":[{"message":{"content":"four"},"finish_reason":"stop"}],"usage":{"cost":0.00001}}`, 200, "four", nil, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotFile string
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if r.URL.Path == "/api/v1/audio/transcriptions" {
					if err := r.ParseMultipartForm(1 << 20); err == nil {
						if f, h, err := r.FormFile("file"); err == nil {
							gotFile = h.Filename
							f.Close()
						}
					}
				}
				rw.WriteHeader(tc.status)
				fmt.Fprint(rw, tc.body)
			}))
			defer srv.Close()
			w := newTrialWorld(t)
			w.app.Speech = speech.OpenRouter{APIKey: "sk-or-v1-SPEECHstandin", Model: tc.model, BaseURL: srv.URL, HTTP: srv.Client()}
			text, err := w.app.Transcribe(w.ctx, w.user, speech.Audio{Data: wav16k(32000), MIME: "audio/webm"}) // 2 s; sent on as WAV
			if text != tc.wantText || !errors.Is(err, tc.wantErr) || gotPath != tc.path {
				t.Fatalf("text %q err %v path %s", text, err, gotPath)
			}
			if tc.path == "/api/v1/audio/transcriptions" && gotFile != "audio.wav" {
				t.Fatalf("uploaded file %q, want audio.wav", gotFile)
			}
			model := tc.model
			if model == "" {
				model = speech.DefaultOpenRouterModel
			}
			var gotModel string
			var cost int64
			if err := w.st.DB().QueryRowContext(w.ctx, `SELECT model, cost_micros FROM usage WHERE user_id=? AND kind='speech'`, w.user.ID).Scan(&gotModel, &cost); err != nil {
				t.Fatal(err)
			}
			if gotModel != model || cost != tc.wantCost {
				t.Fatalf("metered %s at %d micros, want %s at %d", gotModel, cost, model, tc.wantCost)
			}
			if s := w.sub(t); s.UsedMicros != tc.wantCost {
				t.Fatalf("used %d", s.UsedMicros)
			}
			if r, _ := w.us.ReservedMicros(w.ctx); r != 0 {
				t.Fatalf("%d still held", r)
			}
		})
	}
}

// Slice 36: a judgment on the trial reserves the model-aware budget, so a
// trial learner with less than the old $0.115 ceiling left can still be
// graded.
func TestSlice36_TrialGradesWithAModelAwareHold(t *testing.T) {
	w := newTrialWorld(t)
	w.app.Model = &ai.Fake{Verdicts: []srs.Verdict{srs.Right}}
	if _, err := w.us.ChangeSubscription(w.ctx, func(s *store.Subscription) { s.UsedMicros = s.AllowanceMicros - 2000 }); err != nil {
		t.Fatal(err)
	}
	if w.app.JudgeBudget() > 2000 || ai.JudgeBudgetMicros <= 2000 {
		t.Fatalf("judge budget %d", w.app.JudgeBudget())
	}
	id := fmt.Sprintf("judge-%d", time.Now().UnixNano())
	if err := w.us.ReserveAt(w.ctx, id, w.app.JudgeBudget(), w.now(), 7*24*time.Hour); err != nil {
		t.Fatalf("model-aware judge hold on a nearly spent trial: %v", err)
	}
}
