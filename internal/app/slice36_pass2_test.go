package app_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
)

// Slice 36 pass 2 (critic finding 1): a card step loaded while the trial
// still had over an hour left ("nothing charged today") and submitted
// with less left, or after the trial ended, is never turned into a
// charge. Nothing reaches the processor, the trial is untouched and
// still gives access, and the learner is asked to confirm the charge.
func TestSlice36_StaleTrialTermsNeverChargeSilently(t *testing.T) {
	for _, tc := range []struct {
		name   string
		at     func(end time.Time) time.Time
		access bool
		agreed app.BillingTerms
	}{
		{"30 minutes left, page said nothing today", func(end time.Time) time.Time { return end.Add(-30 * time.Minute) }, true, app.TermsTrial},
		{"30 minutes left, no terms submitted", func(end time.Time) time.Time { return end.Add(-30 * time.Minute) }, true, ""},
		{"30 minutes left, an update form", func(end time.Time) time.Time { return end.Add(-30 * time.Minute) }, true, app.TermsUpdate},
		{"after the end, page said nothing today", func(end time.Time) time.Time { return end.Add(time.Minute) }, false, app.TermsTrial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTrialWorld(t)
			before := w.sub(t)
			// The page was loaded with 65 minutes left: it promised the trial.
			w.set(before.PeriodEnd.Add(-65 * time.Minute))
			if got := w.app.CardTerms(w.sub(t)); got != app.TermsTrial {
				t.Fatalf("terms at T-65m = %q, want trial", got)
			}
			w.set(tc.at(before.PeriodEnd))
			if got := w.app.CardTerms(w.sub(t)); got != app.TermsImmediate {
				t.Fatalf("terms at submit = %q, want immediate", got)
			}
			err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), tc.agreed)
			if !errors.Is(err, app.ErrTermsChanged) {
				t.Fatalf("StartTrial = %v, want ErrTermsChanged", err)
			}
			if len(w.pay.subscribes) != 0 || len(w.pay.charges) != 0 {
				t.Fatalf("processor called: %+v %v", w.pay.subscribes, w.pay.charges)
			}
			if after := w.sub(t); after != before {
				t.Fatalf("plan changed by a refused card step:\n%+v\n%+v", before, after)
			}
			if led, _ := w.us.Ledger(w.ctx); len(led) != 1 || led[0].Kind != "trial" {
				t.Fatalf("ledger %+v", led)
			}
			_, perr := w.app.Plan(w.ctx, w.user.ID)
			if tc.access && perr != nil {
				t.Fatalf("trial with 30 minutes left lost access: %v", perr)
			}
			if !tc.access && !errors.Is(perr, app.ErrPlanEnded) {
				t.Fatalf("after the end: %v", perr)
			}
		})
	}
}

// Slice 36 pass 2 (critic finding 1): a declined card in the trial's last
// hour, on confirmed immediate terms, changes nothing: the no-card trial
// stays trialing and usable until its own period_end, then ends.
func TestSlice36_DeclineInTheLastHourKeepsTheTrial(t *testing.T) {
	for _, number := range []string{"4000 0000 0000 0341"} { // attaches, then every charge is declined
		w := newTrialWorld(t)
		before := w.sub(t)
		w.set(before.PeriodEnd.Add(-30 * time.Minute))
		tok, err := w.pay.Tokenize(number, "12/30", "123", w.now())
		if err != nil {
			t.Fatal(err)
		}
		err = w.app.StartTrial(w.ctx, w.user, tok, app.TermsImmediate)
		var d *billing.DeclineError
		if !errors.As(err, &d) {
			t.Fatalf("StartTrial = %v, want a decline", err)
		}
		if len(w.pay.subscribes) != 1 || !w.pay.subscribes[0].TrialEnd.IsZero() {
			t.Fatalf("processor asked for %+v", w.pay.subscribes)
		}
		after := w.sub(t)
		if after != before || !after.NoCardTrial() {
			t.Fatalf("a declined card changed the trial:\n%+v\n%+v", before, after)
		}
		if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
			t.Fatalf("trial lost access after a decline: %v", err)
		}
		// Still usable: a judgment's hold is admitted.
		if err := w.us.ReserveAt(w.ctx, "after-decline", w.app.JudgeBudget(), w.now(), 7*24*time.Hour); err != nil {
			t.Fatalf("hold after a decline: %v", err)
		}
		if err := w.us.Settle(w.ctx, "after-decline", store.Usage{Kind: "judge"}, w.now()); err != nil {
			t.Fatal(err)
		}
		// One second before the end it still gives access; at the end it ends.
		w.set(before.PeriodEnd.Add(-time.Second))
		if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
			t.Fatalf("one second before the end: %v", err)
		}
		w.set(before.PeriodEnd)
		if _, err := w.app.Plan(w.ctx, w.user.ID); !errors.Is(err, app.ErrPlanEnded) {
			t.Fatalf("at the end: %v", err)
		}
	}
}

// Slice 36 pass 2 (critic finding 1): a confirmed upgrade in the trial's
// last hour charges today and turns the trial into the paid plan in one
// write, with the full monthly allowance and nothing carried as used.
func TestSlice36_ConfirmedUpgradeInTheLastHour(t *testing.T) {
	w := newTrialWorld(t)
	before := w.sub(t)
	if _, err := w.us.ChangeSubscription(w.ctx, func(s *store.Subscription) { s.UsedMicros, s.OverageMicros = 400_000, 1_000 }); err != nil {
		t.Fatal(err)
	}
	w.set(before.PeriodEnd.Add(-30 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	s := w.sub(t)
	if len(w.pay.subscribes) != 1 || !w.pay.subscribes[0].TrialEnd.IsZero() {
		t.Fatalf("processor asked for a trial: %+v", w.pay.subscribes)
	}
	if s.Status != store.SubActive || s.NoCard() || s.AllowanceMicros != w.app.Cfg.AllowanceMicros || s.UsedMicros != 0 || s.OverageMicros != 0 ||
		!s.TrialEnd.Equal(before.TrialEnd) || s.SubscriptionRef == before.SubscriptionRef || s.CardLast4 != "4242" {
		t.Fatalf("paid plan: %+v", s)
	}
	led, _ := w.us.Ledger(w.ctx)
	if len(led) != 2 || led[0].Kind != "charge" || led[0].AmountCents != w.app.Cfg.PriceCents || led[0].Status != "paid" {
		t.Fatalf("ledger %+v", led)
	}
	// Past the old trial end the plan is not cut off.
	w.set(before.PeriodEnd.Add(time.Hour))
	if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
		t.Fatalf("paid plan after the trial's end: %v", err)
	}
	// A second submit changes nothing more.
	if err := w.app.StartTrial(w.ctx, w.user, goodToken(t, w), app.TermsImmediate); err != nil || len(w.pay.subscribes) != 1 {
		t.Fatalf("second submit: %v %d", err, len(w.pay.subscribes))
	}
}

// Slice 36 pass 2 (critic finding 2): a speech hold covers what a
// provider may really bill. 77 recordings each billed $0.0065 can no
// longer push a no-card trial past its $0.50: holds are at least $0.015,
// so admission stops while the actual spend is still inside the cap.
func TestSlice36_SpeechHoldsKeepTheTrialInsideItsCap(t *testing.T) {
	if got := app.SpeechBudgetMicros(120); got != 30_000 {
		t.Fatalf("120 s hold = %d, want 30,000", got)
	}
	if got := app.SpeechBudgetMicros(2); got != 15_000 {
		t.Fatalf("2 s hold = %d, want the 15,000 floor", got)
	}
	if got := app.SpeechBudgetMicros(60.2); got != 15_250 {
		t.Fatalf("60.2 s hold = %d, want 61 x 250", got)
	}
	w := newTrialWorld(t)
	w.app.Speech = speech.Fake{}
	var total int64
	calls := 0
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("speech-%d", i)
		err := w.us.ReserveAt(w.ctx, id, app.SpeechBudgetMicros(2), w.now(), 7*24*time.Hour)
		if errors.Is(err, store.ErrBudget) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		overage, err := w.us.SettleMetered(w.ctx, id, store.Usage{Kind: "speech", Model: "microsoft/mai-transcribe-2", Seconds: 2, CostMicros: 6_500}, w.now())
		if err != nil || overage != 0 {
			t.Fatalf("settle: overage %d, %v", overage, err)
		}
		total += 6_500
		calls++
	}
	s := w.sub(t)
	if total > w.app.Cfg.TrialMicros || s.UsedMicros != total || s.OverageMicros != 0 {
		t.Fatalf("%d calls spent %d (used %d, overage %d) against a %d cap", calls, total, s.UsedMicros, s.OverageMicros, w.app.Cfg.TrialMicros)
	}
	if calls != 75 {
		t.Fatalf("%d recordings admitted, want 75 (admitted while 6,500k + 15,000 <= 500,000)", calls)
	}
}

// Slice 36 pass 2 (critic finding 2): a call that costs more than its hold
// draws only the hold, so a concurrent hold keeps its backing; the
// overage is recorded on the plan and in the usage row, and on a no-card
// trial it counts against the cap, so once what the trial actually cost
// reaches the allowance no further hold is admitted.
func TestSlice36_OverageNeverStarvesAConcurrentHoldAndStopsTheTrial(t *testing.T) {
	w := newTrialWorld(t)
	cap := w.app.Cfg.TrialMicros
	grace := 7 * 24 * time.Hour
	// Two holds in flight: A (a recording) and B (a draft).
	if err := w.us.ReserveAt(w.ctx, "A", 15_000, w.now(), grace); err != nil {
		t.Fatal(err)
	}
	if err := w.us.ReserveAt(w.ctx, "B", 226_048, w.now(), grace); err != nil {
		t.Fatal(err)
	}
	// A's provider bills far more than its hold: $0.60.
	overage, err := w.us.SettleMetered(w.ctx, "A", store.Usage{Kind: "speech", Model: "vendor/expensive-stt", Seconds: 2, CostMicros: 600_000}, w.now())
	if err != nil || overage != 600_000-15_000 {
		t.Fatalf("overage %d, %v", overage, err)
	}
	s := w.sub(t)
	if s.UsedMicros != 15_000 || s.OverageMicros != 585_000 {
		t.Fatalf("after A: used %d overage %d", s.UsedMicros, s.OverageMicros)
	}
	var cost int64
	if err := w.st.DB().QueryRow(`SELECT cost_micros FROM usage WHERE user_id=? AND kind='speech'`, w.user.ID).Scan(&cost); err != nil || cost != 600_000 {
		t.Fatalf("usage row keeps the full cost: %d %v", cost, err)
	}
	// B is still held and fully backed: it settles at its whole hold.
	if r, _ := w.us.ReservedMicros(w.ctx); r != 226_048 {
		t.Fatalf("B's hold: %d", r)
	}
	if r := s.AllowanceMicros - s.UsedMicros; r < 226_048 {
		t.Fatalf("B lost its backing: %d left", r)
	}
	// No further hold, however small: the trial's actual cost is past its cap.
	if err := w.us.ReserveAt(w.ctx, "C", 1, w.now(), grace); !errors.Is(err, store.ErrBudget) {
		t.Fatalf("hold after the trial's actual cost reached its cap: %v", err)
	}
	if overage, err := w.us.SettleMetered(w.ctx, "B", store.Usage{Kind: "draft", Model: "openai/gpt-6.2-sol", CostMicros: 226_048}, w.now()); err != nil || overage != 0 {
		t.Fatalf("B: overage %d, %v", overage, err)
	}
	s = w.sub(t)
	if s.UsedMicros != 15_000+226_048 || s.UsedMicros > cap {
		t.Fatalf("after B: used %d", s.UsedMicros)
	}
	if err := w.us.ReserveAt(w.ctx, "D", 1, w.now(), grace); !errors.Is(err, store.ErrBudget) {
		t.Fatalf("hold after B: %v", err)
	}
	if app.UsedPercent(s) != 100 || app.CanSpend(s) || app.Unreserved(s, 0) >= 0 {
		t.Fatalf("billing shows the trial with room: %d%% %v %d", app.UsedPercent(s), app.CanSpend(s), app.Unreserved(s, 0))
	}
	// The same overage on a card-backed plan is recorded but does not
	// count against admission (the margin absorbs it).
	tok := goodToken(t, w)
	if err := w.app.StartTrial(w.ctx, w.user, tok, app.TermsTrial); err != nil {
		t.Fatal(err)
	}
	if s := w.sub(t); s.NoCard() || s.OverageMicros != 585_000 {
		t.Fatalf("card added: %+v", s)
	}
	if err := w.us.ReserveAt(w.ctx, "E", 1_000, w.now(), grace); err != nil {
		t.Fatalf("card trial hold: %v", err)
	}
}
