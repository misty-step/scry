package web_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"scry/internal/store"
)

// Slice 36: /healthz keeps every key monitors and scripts/verify-live.mjs
// read, and adds the model each AI surface calls and the speech route.
func TestSlice36_HealthzReportsModelsAndKeepsEveryKey(t *testing.T) {
	w := newWorld(t)
	resp, err := http.Get(w.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var h map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz %d %v", resp.StatusCode, err)
	}
	for _, k := range []string{"status", "build_sha", "mode", "auth_mode", "billing_mode", "database", "db", "grading", "drafting", "explainer", "speech", "providers", "warnings", "models", "speech_route"} {
		if _, ok := h[k]; !ok {
			t.Fatalf("/healthz lost key %q: %v", k, h)
		}
	}
	if h["status"] != "ok" || h["database"] != "ok" || h["db"] != true || h["mode"] != "test" || h["speech_route"] != "test" {
		t.Fatalf("healthz %v", h)
	}
	m, _ := h["models"].(map[string]any)
	if m["speech"] != "test-speech" || len(m) != 4 {
		t.Fatalf("models %v", m)
	}
	for _, k := range []string{"grading", "drafting", "explainer"} {
		if _, ok := m[k].(string); !ok {
			t.Fatalf("models[%s] missing: %v", k, m)
		}
	}
}

// Slice 36: the public pages tell the new trial truthfully: no card up
// front anywhere a newcomer reads before signing up.
func TestSlice36_PublicCopySaysNoCardUpFront(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	home := b.mustGet("/")
	has(t, home, "Start the 7-day trial", `7 days free · no card needed · then <span class="nowrap">$5 a month</span> if you keep it`)
	hasNot(t, home, "card up front")
	signup := b.mustGet("/signup")
	has(t, signup, "7-day trial · no card needed", "No card up front", "Start the 7-day trial")
	hasNot(t, signup, "Next: a card", "Step 1 of 2")
}

// Slice 36: when the trial's allowance is used up, every surface says so
// calmly and points at adding a card, never at a top-up it cannot make.
func TestSlice36_ExhaustedTrialPromptsForACard(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("spent@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("spent@example.com")
	if _, err := lib.ChangeSubscription(ctx, func(s *store.Subscription) { s.UsedMicros = s.AllowanceMicros }); err != nil {
		t.Fatal(err)
	}
	study := b.mustGet("/study")
	item, ok := b.current(study)
	if !ok {
		t.Fatal("nothing to study")
	}
	page := b.answer(item, "something not exact")
	has(t, page, "You've used the trial's allowance, so Scry can't judge this answer yet. Add a card to keep going.", `href="/signup/card">Add a card</a>`)
	hasNot(t, page, "Top up in Billing")

	status, out := b.speak(wav(16000, true))
	if status != http.StatusPaymentRequired || out["error"] != "You've used the trial's allowance, so Scry can't listen right now. Type your answer, or add a card in Billing to keep going." {
		t.Fatalf("speech %d %v", status, out)
	}
	has(t, b.mustGet("/learn"), "Not enough of the trial", "allowance left for a draft. Add a card in Billing to keep going.")
	billing := b.mustGet("/billing")
	has(t, billing, "You've used all of the trial's $0.50 allowance", "Add a card to keep going.", "Top-ups need a card on file.")
	hasNot(t, billing, `action="/billing/topup"`)
}

// Slice 36: the trial line in the chrome counts down by whole days, says
// the last day, and after the end says so with the one way on.
func TestSlice36_TrialChipCountsDown(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("chip@example.com")
	for _, tc := range []struct {
		after time.Duration
		want  string
	}{
		{0, "Trial · 7 days left"},
		{24*time.Hour + time.Minute, "Trial · 6 days left"},
		{4 * 24 * time.Hour, "Trial · 2 days left"},
		{24 * time.Hour, "Trial · last day"},
	} {
		w.advance(tc.after)
		has(t, b.mustGet("/library"), tc.want)
	}
	w.advance(24 * time.Hour) // 7 days and a minute in
	b.get("/library")
	if b.path != "/billing" {
		t.Fatalf("after the trial /library ends at %s", b.path)
	}
	has(t, b.mustGet("/billing"), "Trial ended", `class="trial-act">Add a card</span>`)
}

// Slice 36 pass 2 (critic finding 1): a card step loaded at T-1h05m says
// "nothing charged today" and carries billing_terms=trial. Submitted at
// T-30m, it is refused (409) without calling the processor or touching
// the trial, and shown again stating the $5 charge with
// billing_terms=immediate. A declined card on the confirmed form (402)
// still leaves the trial running, /study included, until its period_end;
// a good card then starts the paid plan with the full allowance.
func TestSlice36_StaleCardStepConfirmsBeforeChargingAndADeclineKeepsTheTrial(t *testing.T) {
	for _, path := range []string{"/signup/card", "/billing/card"} {
		t.Run(path, func(t *testing.T) {
			w := newWorld(t)
			b := w.browser()
			b.signUpNoCard("late@example.com")
			b.addSet("nato-alphabet")
			lib := w.lib("late@example.com")
			before, _ := lib.Subscription(ctx)
			w.advance(before.PeriodEnd.Sub(w.now()) - 65*time.Minute)
			page := b.mustGet(path)
			has(t, page, "Nothing is charged today", `name="billing_terms" value="trial"`)
			if b.terms != "trial" {
				t.Fatalf("card step terms %q", b.terms)
			}

			w.advance(35 * time.Minute) // T-30m: the page is now stale
			status, page := b.post(path, "number", goodCard, "expiry", "12/30", "cvc", "123")
			if status != http.StatusConflict {
				t.Fatalf("stale card step gave %d at %s", status, b.path)
			}
			has(t, page, "Nothing was charged.", "ends within the hour", "charges $5 now", `name="billing_terms" value="immediate"`, "$5 a month")
			hasNot(t, page, "Nothing is charged today", "nothing charged today")
			if after, _ := lib.Subscription(ctx); after != before {
				t.Fatalf("stale card step changed the trial:\n%+v\n%+v", before, after)
			}
			if led, _ := lib.Ledger(ctx); len(led) != 1 {
				t.Fatalf("charged: %+v", led)
			}
			b.mustGet("/study")
			if b.path != "/study" {
				t.Fatalf("after the refusal /study ends at %s", b.path)
			}

			// Confirmed, but the card is declined: 402, nothing changes.
			b.mustGet(path)
			if b.terms != "immediate" {
				t.Fatalf("card step at T-30m carries %q", b.terms)
			}
			status, page = b.post(path, "number", "4000 0000 0000 0341", "expiry", "12/30", "cvc", "123")
			if status != http.StatusPaymentRequired {
				t.Fatalf("declined card gave %d", status)
			}
			has(t, page, "Your card was declined.")
			if after, _ := lib.Subscription(ctx); after != before || !after.NoCardTrial() {
				t.Fatalf("a declined card changed the trial:\n%+v\n%+v", before, after)
			}
			b.mustGet("/study")
			if b.path != "/study" {
				t.Fatalf("after a decline in the last hour /study ends at %s, want /study", b.path)
			}
			w.advance(29 * time.Minute) // T-1m: still the trial
			b.mustGet("/study")
			if b.path != "/study" {
				t.Fatalf("at T-1m /study ends at %s", b.path)
			}

			// Confirmed with a good card: charged today, the paid plan.
			b.mustGet(path)
			b.post(path, "number", goodCard, "expiry", "12/30", "cvc", "123")
			sub, _ := lib.Subscription(ctx)
			led, _ := lib.Ledger(ctx)
			if sub.Status != store.SubActive || sub.NoCard() || sub.AllowanceMicros != w.app.Cfg.AllowanceMicros || sub.UsedMicros != 0 ||
				len(led) != 2 || led[0].Kind != "charge" || led[0].AmountCents != 500 {
				t.Fatalf("confirmed upgrade: %+v ledger %+v", sub, led)
			}
			w.advance(2 * time.Hour) // past the old trial end
			b.mustGet("/study")
			if b.path != "/study" {
				t.Fatalf("paid plan: /study ends at %s", b.path)
			}
		})
	}
}

// Slice 36 pass 2: a card step loaded after the trial ended already states
// the charge and carries billing_terms=immediate; a form with no terms at
// all (a stale or hand-made post) is never charged.
func TestSlice36_CardStepWithoutTermsIsNeverCharged(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("ended@example.com")
	lib := w.lib("ended@example.com")
	before, _ := lib.Subscription(ctx)
	w.advance(before.PeriodEnd.Sub(w.now()) + time.Minute)
	page := b.mustGet("/signup/card")
	has(t, page, "$5 today", `name="billing_terms" value="immediate"`)
	b.terms = "" // a post that did not come from this page
	if status, page := b.post("/signup/card", "number", goodCard, "expiry", "12/30", "cvc", "123"); status != http.StatusConflict {
		t.Fatalf("termless card post gave %d", status)
	} else {
		has(t, page, "Your trial has ended", `name="billing_terms" value="immediate"`)
	}
	if sub, _ := lib.Subscription(ctx); sub.Status != store.SubEnded || !sub.NoCard() {
		t.Fatalf("termless post changed the plan: %+v", sub)
	}
	if led, _ := lib.Ledger(ctx); len(led) != 1 {
		t.Fatalf("charged: %+v", led)
	}
}
