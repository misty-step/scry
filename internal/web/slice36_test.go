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
	has(t, home, "Start the 7-day trial", "7 days free · no card needed · then $5 a month if you keep it")
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
