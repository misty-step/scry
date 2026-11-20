package web_test

// One test per PRODUCT.md business rule (and the AGENTS.md rules that
// bind the product), each named for its rule.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/store"
)

// Rule: No free tier. There's a 7-day trial with a card up front.
func TestRule_NoFreeTier_SevenDayTrialWithACardUpFront(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.get("/signup")
	b.post("/signup", "email", "nocard@example.com", "password", "correct horse")

	// Signed up but no card: every part of the product leads to the card step.
	for _, path := range []string{"/study", "/library", "/shelf", "/inbox", "/learn", "/billing"} {
		b.get(path)
		if b.path != "/signup/card" {
			t.Errorf("without a card, %s ends at %s, want /signup/card", path, b.path)
		}
	}
	if status, _ := b.post("/shelf/add", "slug", "nato-alphabet"); b.path != "/signup/card" {
		t.Errorf("without a card, adding a set gave %d at %s", status, b.path)
	}
	if n, _, _ := w.lib("nocard@example.com").Counts(ctx, w.now()); n != 0 {
		t.Fatal("material was added without a plan")
	}

	// The card comes first; the trial then runs exactly seven days.
	b.get("/signup/card")
	b.post("/signup/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	lib := w.lib("nocard@example.com")
	sub, _ := lib.Subscription(ctx)
	if sub.Status != "trialing" || sub.TrialEnd.Sub(w.now()) != 7*24*time.Hour || sub.CardLast4 != "4242" {
		t.Fatalf("trial = %+v", sub)
	}
	if led, _ := lib.Ledger(ctx); len(led) != 1 || led[0].AmountCents != 0 {
		t.Fatalf("the trial start should cost nothing: %+v", led)
	}
	w.advance(6*24*time.Hour + 23*time.Hour)
	b.mustGet("/library")
	if led, _ := lib.Ledger(ctx); len(led) != 1 {
		t.Fatalf("charged before the trial ended: %+v", led)
	}
	// On day seven the card is charged the monthly base and the plan is active.
	w.advance(2 * time.Hour)
	b.get("/shelf")
	sub, _ = lib.Subscription(ctx)
	led, _ := lib.Ledger(ctx)
	if sub.Status != "active" || len(led) != 2 || led[0].Kind != "charge" || led[0].AmountCents != 500 || led[0].Status != "paid" {
		t.Fatalf("after the trial: %+v, ledger %+v", sub, led)
	}

	// There is no second trial: a learner who lets the plan end pays to return.
	b.get("/billing")
	b.post("/billing/cancel")
	w.advance(32 * 24 * time.Hour)
	b.get("/signin")
	b.post("/signin", "email", "nocard@example.com", "password", "correct horse")
	b.get("/study")
	if b.path != "/billing" {
		t.Fatalf("ended plan: at %s", b.path)
	}
	has(t, b.mustGet("/billing/card"), "Start again", "Your card is charged $5 today")
	_, page := b.post("/billing/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	has(t, page, "Welcome back.")
	sub, _ = lib.Subscription(ctx)
	led, _ = lib.Ledger(ctx)
	if sub.Status != "active" || led[0].AmountCents != 500 || led[0].Status != "paid" || len(led) != 3 {
		t.Errorf("restart: %+v, ledger %+v", sub, led)
	}
}

func TestRule_NoFreeTier_AFailedCardPausesThePlan(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.get("/signup")
	b.post("/signup", "email", "failing@example.com", "password", "correct horse")
	// This test card attaches, then every charge is declined.
	b.post("/signup/card", "number", "4000 0000 0000 0341", "expiry", "12/30", "cvc", "123")
	if b.path != "/shelf" {
		t.Fatalf("card step ended at %s", b.path)
	}
	lib := w.lib("failing@example.com")
	w.advance(8 * 24 * time.Hour)

	// Card failed: Billing gives the reason and "Update card".
	page := b.mustGet("/billing")
	has(t, page, "Card failed.", "Your card was declined.", "Update card")
	sub, _ := lib.Subscription(ctx)
	if sub.Status != "past_due" {
		t.Fatalf("status = %s", sub.Status)
	}
	// Access is kept for a short grace period while the card is retried...
	b.mustGet("/shelf")
	if b.path != "/shelf" {
		t.Errorf("in grace, /shelf ends at %s", b.path)
	}
	// ...and then the plan stops.
	w.advance(8 * 24 * time.Hour)
	b.get("/shelf")
	if b.path != "/billing" {
		t.Errorf("after grace, /shelf ends at %s, want /billing", b.path)
	}
	// A working card starts it again.
	b.get("/billing/card")
	_, page = b.post("/billing/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	sub, _ = lib.Subscription(ctx)
	if sub.Status != "active" || sub.CardError != "" {
		t.Errorf("after a new card: %+v", sub)
	}
	hasNot(t, page, "Card failed.")

	// Updating the card during the grace period settles the unpaid month.
	c := w.browser()
	c.get("/signup")
	c.post("/signup", "email", "fixes@example.com", "password", "correct horse")
	c.post("/signup/card", "number", "4000 0000 0000 0341", "expiry", "12/30", "cvc", "123")
	w.advance(8 * 24 * time.Hour)
	has(t, c.mustGet("/billing"), "Card failed.")
	c.get("/billing/card")
	_, page = c.post("/billing/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	has(t, page, "Your card was updated.")
	if s, _ := w.lib("fixes@example.com").Subscription(ctx); s.Status != "active" {
		t.Errorf("after updating the card in grace: %+v", s)
	}
}

// Rule: Prices cover costs plus a small margin, only to protect against
// abuse and runaway costs: a small monthly base with a capped allowance,
// and top-ups beyond it.
func TestRule_Prices_SmallBaseCappedAllowanceAndTopUpsBeyondIt(t *testing.T) {
	w := newWorld(t)
	cfg := w.app.Cfg
	// A small base; the allowance and the top-up credit are each worth
	// less than their price, so every sale covers its cost with a margin.
	if cfg.PriceCents > 1000 || cfg.AllowanceMicros >= cfg.PriceCents*10_000 || cfg.TopupMicros >= cfg.TopupCents*10_000 {
		t.Fatalf("prices do not cover costs: %+v", cfg)
	}
	if cfg.TopupMicros < cfg.TopupCents*10_000*7/10 {
		t.Errorf("the top-up margin is not small: %d credit for %d cents", cfg.TopupMicros, cfg.TopupCents)
	}
	// Usage is metered at what the provider charges.
	if got := app.Cost(ai.Usage{Model: "claude-opus-5-5", InputTokens: 1000, OutputTokens: 1000}); got != 24_000 {
		t.Errorf("opus cost = %d micro-dollars, want 24000", got)
	}
	if got := app.Cost(ai.Usage{Model: "test-judge", InputTokens: 1000, OutputTokens: 1000}); got != 600 {
		t.Errorf("fixture judge cost = %d micro-dollars, want 600", got)
	}
	// A model with no known price is charged at the top rate: costs cannot run away.
	if app.Cost(ai.Usage{Model: "mystery", InputTokens: 1000}) < app.Cost(ai.Usage{Model: "claude-opus-5-5", InputTokens: 1000}) {
		t.Error("an unknown model is priced too low")
	}

	b := w.browser()
	b.signUp("spender@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("spender@example.com")
	w.advance(8 * 24 * time.Hour) // past the trial, on the monthly allowance
	body := b.mustGet("/study")
	first, _ := b.current(body)

	// Each judged answer draws on the allowance.
	b.answer(first, first.Answer)
	sub, _ := lib.Subscription(ctx)
	if sub.UsedMicros <= 0 || sub.AllowanceMicros != cfg.AllowanceMicros {
		t.Fatalf("judging was not metered: %+v", sub)
	}

	// Near the limit, the sentence says so.
	sub.UsedMicros = cfg.AllowanceMicros * 86 / 100
	_ = lib.SaveSubscription(ctx, sub)
	has(t, b.mustGet("/billing"), "You've used 86% of this month's allowance, so you're near its limit.")

	// The allowance is capped: at the cap, model work stops...
	sub.UsedMicros = cfg.AllowanceMicros
	_ = lib.SaveSubscription(ctx, sub)
	calls := w.model.judgeCalls()
	second, _ := b.current(b.mustGet("/study"))
	status, page := b.post("/study/answer", "item_id", fmt.Sprint(second.ID), "answer", second.Answer)
	if status != http.StatusPaymentRequired || w.model.judgeCalls() != calls {
		t.Errorf("at the cap: status %d, model calls %d -> %d", status, calls, w.model.judgeCalls())
	}
	has(t, page, "You've reached this month's allowance", "Top up in Billing")
	b.get("/learn")
	b.post("/learn", "q", "philosophy")
	w.app.WaitDrafts()
	has(t, b.mustGet(b.path), "Not enough allowance left for a draft. Scry can't draft this yet.", `href="/billing"`)
	if len(w.model.drafts) != 0 {
		t.Error("a draft was written past the cap")
	}
	has(t, b.mustGet("/billing"), "You've used all of this month's allowance, so judging, speaking and drafting are paused.")

	// ...until a top-up, which is spent only after the allowance.
	b.post("/billing/topup")
	has(t, b.answer(second, second.Answer), "tag-right")
	sub, _ = lib.Subscription(ctx)
	if sub.TopupMicros >= cfg.TopupMicros || sub.UsedMicros != cfg.AllowanceMicros {
		t.Errorf("the answer was not drawn from top-up credit: %+v", sub)
	}
	has(t, b.mustGet("/billing"), "You've used all of this month's allowance and are now drawing on your top-up credit.")

	// The allowance resets with the next month; top-up credit carries over.
	credit := sub.TopupMicros
	w.advance(31 * 24 * time.Hour)
	b.get("/signin")
	b.post("/signin", "email", "spender@example.com", "password", "correct horse")
	b.mustGet("/billing")
	sub, _ = lib.Subscription(ctx)
	if sub.UsedMicros != 0 || sub.TopupMicros != credit || sub.Status != "active" {
		t.Errorf("after renewal: %+v", sub)
	}
}

// Rule: Phaedrus is the first customer, with no special powers in the
// product.
func TestRule_PhaedrusIsTheFirstCustomerWithNoSpecialPowers(t *testing.T) {
	w := newWorld(t)
	if err := w.app.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	phaedrus := w.user(app.DemoEmail)
	if phaedrus.ID != 1 {
		t.Errorf("Phaedrus should be the first customer, has id %d", phaedrus.ID)
	}
	// There is one kind of account: the schema has nowhere to keep a role.
	rows, err := w.store.DB().Query(`SELECT name FROM pragma_table_info('users')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		_ = rows.Scan(&col)
		for _, bad := range []string{"role", "admin", "staff", "owner", "super", "plan_override", "free"} {
			if strings.Contains(col, bad) {
				t.Errorf("users has a %q column", col)
			}
		}
	}

	other := w.browser()
	other.signUp("someone@example.com")
	other.addSet("kitchen-numbers")
	theirs, _ := w.lib("someone@example.com").Outline(ctx)

	p := w.browser()
	p.get("/signin")
	p.post("/signin", "email", app.DemoEmail, "password", app.DemoPassword)
	// Phaedrus cannot see anyone else's library...
	if status, _ := p.get(fmt.Sprintf("/library/ideas/%d", theirs[0].ID)); status != http.StatusNotFound {
		t.Errorf("Phaedrus read another learner's idea: %d", status)
	}
	// ...has no back office...
	for _, path := range []string{"/admin", "/admin/users", "/users", "/debug", "/metrics"} {
		if status, _ := p.get(path); status != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, status)
		}
	}
	// ...pays the same price and has the same allowance as everyone...
	sub, _ := w.lib(app.DemoEmail).Subscription(ctx)
	if sub.AllowanceMicros != w.app.Cfg.AllowanceMicros {
		t.Errorf("Phaedrus has a different allowance: %d", sub.AllowanceMicros)
	}
	has(t, p.mustGet("/billing"), "Scry is $5 a month")
	// ...is stopped by the allowance cap like anyone...
	sub.UsedMicros, sub.TopupMicros = sub.AllowanceMicros, 0
	_ = w.lib(app.DemoEmail).SaveSubscription(ctx, sub)
	p.get("/learn")
	p.post("/learn", "q", "philosophy")
	has(t, p.mustGet(p.path), "Not enough allowance left for a draft")
	// ...and loses access like anyone when the plan ends.
	sub.Status = "ended"
	_ = w.lib(app.DemoEmail).SaveSubscription(ctx, sub)
	p.get("/study")
	if p.path != "/billing" {
		t.Errorf("with an ended plan Phaedrus is at %s, want /billing", p.path)
	}
}

// Rule: Reliable from day one: outages are noticed and fixed without
// anyone being asked.
// Rule (AGENTS.md): expose /healthz and structured logs so outages are
// noticed.
func TestRule_Reliability_HealthzStructuredLogsAndAlertsWithoutBeingAsked(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("nato-alphabet")

	// /healthz proves the database can be written and says so as JSON.
	resp, err := http.Get(w.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	var health map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()
	if resp.StatusCode != 200 || health["status"] != "ok" || health["database"] != "ok" {
		t.Fatalf("healthz = %d %v", resp.StatusCode, health)
	}

	// Every request is one structured log line.
	var sawRequest bool
	sc := bufio.NewScanner(strings.NewReader(w.logs.String()))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		var line map[string]any
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("log line is not JSON: %s", sc.Text())
		}
		if line["msg"] == "request" && line["path"] == "/shelf/add" {
			sawRequest = true
			for _, key := range []string{"time", "level", "request_id", "method", "status", "duration_ms", "user"} {
				if _, ok := line[key]; !ok {
					t.Errorf("request log line has no %q: %v", key, line)
				}
			}
		}
	}
	if !sawRequest {
		t.Error("no structured request log line was written")
	}

	// A crash inside a request is contained, logged and alerted.
	first, _ := b.current(b.mustGet("/study"))
	w.model.panics = true
	status, page := b.post("/study/answer", "item_id", fmt.Sprint(first.ID), "answer", "x")
	w.model.panics = false
	if status != http.StatusInternalServerError {
		t.Errorf("panic: status %d", status)
	}
	has(t, page, "Something broke.")
	if !strings.Contains(strings.Join(w.alerts.Titles(), "\n"), "A request panicked") || !strings.Contains(w.logs.String(), `"msg":"panic"`) {
		t.Errorf("the panic was not alerted and logged: %v", w.alerts.Titles())
	}
	// The server is still serving afterwards.
	b.mustGet("/study")

	// Add room for independently testing multiple failed providers: unknown
	// outcomes correctly retain their conservative allowance reservations.
	if err := w.app.TopUp(ctx, w.user("learner@example.com").ID, app.NewTopupToken()); err != nil {
		t.Fatal(err)
	}
	// A failing dependency is alerted the moment it is noticed.
	w.model.err = fmt.Errorf("provider is down")
	b.post("/study/answer", "item_id", fmt.Sprint(first.ID), "answer", "x")
	b.get("/learn")
	b.post("/learn", "q", "physics")
	w.app.WaitDrafts()
	w.model.err = nil
	alerts := strings.Join(w.alerts.Titles(), "\n")
	if !strings.Contains(alerts, "Answers cannot be judged") || !strings.Contains(alerts, "Drafting is failing") {
		t.Errorf("dependency failures were not alerted: %s", alerts)
	}
	has(t, b.mustGet(b.path), "The draft didn't finish.", "Ask again")

	// The watchdog alerts on its own when the database stops working, and
	// /healthz turns 503 so the platform restarts Scry.
	before := len(w.alerts.Titles())
	w.store.Close()
	w.app.Watch(ctx)
	if titles := w.alerts.Titles(); len(titles) <= before || !strings.Contains(titles[len(titles)-1], "Scry is unhealthy") {
		t.Errorf("the watchdog raised no alert: %v", titles)
	}
	resp, err = http.Get(w.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("healthz with a dead database = %d, want 503", resp.StatusCode)
	}
}

// Rule (AGENTS.md): per-user data is private and recoverable.
func TestRule_DataDurability_SnapshotsAndPerLearnerRecovery(t *testing.T) {
	w := newWorld(t)
	alice, bob := w.browser(), w.browser()
	alice.signUp("alice@example.com")
	alice.addSet("nato-alphabet")
	first, _ := alice.current(alice.mustGet("/study"))
	alice.answer(first, first.Answer)
	bob.signUp("bob@example.com")
	bob.addSet("kitchen-numbers")
	a, bo := w.lib("alice@example.com"), w.lib("bob@example.com")

	// A snapshot is taken while the app runs.
	dir := filepath.Join(t.TempDir(), "backups")
	w.app.Cfg.BackupDir = dir
	snap, err := w.app.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	copyDB, err := store.Open(snap)
	if err != nil {
		t.Fatalf("the snapshot does not open: %v", err)
	}
	if err := copyDB.Integrity(ctx); err != nil {
		t.Errorf("the snapshot is not consistent: %v", err)
	}
	copyDB.Close()

	// After the snapshot: Alice loses her library; Bob keeps working.
	ideas, _ := a.Outline(ctx)
	if _, err := w.store.DB().Exec(`DELETE FROM ideas WHERE user_id = ?`, w.user("alice@example.com").ID); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := a.Counts(ctx, w.now()); n != 0 {
		t.Fatalf("setup: Alice still has %d items", n)
	}
	bob.addSet("the-creeds")
	bobItems, _, _ := bo.Counts(ctx, w.now())

	// One learner is restored from the snapshot; nobody else is touched.
	rows, err := w.store.RestoreUser(ctx, snap, "alice@example.com")
	if err != nil || rows == 0 {
		t.Fatalf("restore: %d rows, %v", rows, err)
	}
	if n, _, _ := a.Counts(ctx, w.now()); n != 26 {
		t.Errorf("Alice has %d items after restore, want 26", n)
	}
	if hist, _ := a.History(ctx, first.ID); len(hist) != 1 {
		t.Errorf("Alice's history was not restored: %d reviews", len(hist))
	}
	if after, _ := a.Outline(ctx); len(after) != len(ideas) {
		t.Errorf("Alice's outline: %d ideas, want %d", len(after), len(ideas))
	}
	if n, _, _ := bo.Counts(ctx, w.now()); n != bobItems {
		t.Errorf("restoring Alice changed Bob's library: %d -> %d", bobItems, n)
	}
	if _, err := w.store.RestoreUser(ctx, snap, "nobody@example.com"); err == nil {
		t.Error("restoring an unknown learner should fail")
	}

	// A learner's own mistakes are recoverable too: a removed idea can be
	// restored from Settings.
	alice.get("/library")
	alice.post(fmt.Sprintf("/library/ideas/%d/remove", ideas[0].ID))
	if n, _, _ := a.Counts(ctx, w.now()); n != 0 {
		t.Fatalf("remove left %d items", n)
	}
	has(t, alice.mustGet("/settings"), "Recently removed", "The NATO alphabet", "26 items")
	alice.post(fmt.Sprintf("/settings/restore/%d", ideas[0].ID))
	if n, _, _ := a.Counts(ctx, w.now()); n != 26 {
		t.Errorf("restore brought back %d items, want 26", n)
	}

	// And everything a learner owns can be taken away as a file.
	_, export := alice.get("/settings/export")
	var doc struct {
		Email  string
		Tables map[string][]map[string]any
	}
	if err := json.Unmarshal([]byte(export), &doc); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	if doc.Email != "alice@example.com" || len(doc.Tables["items"]) != 26 || len(doc.Tables["reviews"]) != 1 {
		t.Errorf("export: %s, %d items, %d reviews", doc.Email, len(doc.Tables["items"]), len(doc.Tables["reviews"]))
	}
	for _, table := range store.UserTables() {
		if _, ok := doc.Tables[table]; !ok {
			t.Errorf("export is missing %s", table)
		}
	}
	// Old snapshots are pruned to the configured number.
	for i := 0; i < 3; i++ {
		w.advance(time.Hour)
		if _, err := w.app.Backup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	w.app.Cfg.BackupKeep = 2
	w.advance(time.Hour)
	_, _ = w.app.Backup(ctx)
	if names, _ := filepath.Glob(filepath.Join(dir, "scry-*.db")); len(names) != 2 {
		t.Errorf("%d snapshots kept, want 2", len(names))
	}
}

// Rule (AGENTS.md): `make run` serves the app in test mode with seeded
// demo data.
func TestRule_SeededDemoData(t *testing.T) {
	w := newWorld(t)
	if err := w.app.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.app.SeedDemo(ctx); err != nil { // seeding twice is harmless
		t.Fatal(err)
	}
	if n, _ := w.store.CountUsers(ctx); n != 1 {
		t.Fatalf("%d users after seeding twice", n)
	}
	b := w.browser()
	b.get("/signin")
	b.post("/signin", "email", app.DemoEmail, "password", app.DemoPassword)
	if b.path != "/study" {
		t.Fatalf("demo sign-in ended at %s", b.path)
	}
	lib := w.lib(app.DemoEmail)
	items, due, _ := lib.Counts(ctx, app.EndOfDay(w.user(app.DemoEmail), w.now()))
	if items < 100 || due < 10 || due == items {
		t.Errorf("demo library: %d items, %d due", items, due)
	}
	has(t, b.mustGet("/inbox"), "7 suggestions", "from reading agent · The Federalist Papers")
	has(t, b.mustGet("/billing"), "You've used 62% of this month's allowance. It resets on", "Top-up", "7-day trial started")
	has(t, b.mustGet("/library"), "The NATO alphabet", "Mental models", "The Creeds")
}
