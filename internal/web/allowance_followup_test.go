package web_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/store"
	"strings"
	"testing"
	"time"
)

func TestF1_TrialDraftRefusalAgreesWithBilling(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("draftcap@example.com")
	for i := 0; i < 40; i++ {
		b.get("/learn")
		b.post("/learn", "q", "Meditations")
		path := b.path
		w.app.WaitDrafts()
		body := b.mustGet(path)
		if strings.Contains(body, "can't draft") || strings.Contains(body, "Not enough allowance") {
			has(t, body, "Not enough allowance left for a draft")
			has(t, b.mustGet("/learn"), "Not enough allowance left for a draft")
			has(t, b.mustGet("/billing"), "Not enough allowance left for a draft", "0 drafts")
			return
		}
	}
	t.Fatal("never refused")
}

func TestF2_RepeatedTopupIsTruthful(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("topuprepeat@example.com")
	b.get("/billing")
	form := b.topup // the same rendered form, submitted twice
	_, body := b.post("/billing/topup", "topup_token", form)
	has(t, body, "Topped up.")
	before, _ := w.lib("topuprepeat@example.com").Subscription(ctx)
	_, body = b.post("/billing/topup", "topup_token", form)
	has(t, body, "already processed", "No additional credit")
	hasNot(t, body, "Topped up.")
	after, _ := w.lib("topuprepeat@example.com").Subscription(ctx)
	if before.TopupMicros != after.TopupMicros {
		t.Fatal("duplicate credit")
	}
}

func TestF2_ConcurrentTopupsReportOneCredit(t *testing.T) {
	w := newWorld(t)
	w.browser().signUp("concurrentcredit@example.com")
	u := w.user("concurrentcredit@example.com")
	results := make(chan bool, 8)
	errs := make(chan error, 8)
	token := app.NewTopupToken() // one form, submitted eight times at once
	for i := 0; i < 8; i++ {
		go func() { added, err := w.app.TopUpResult(ctx, u.ID, token); results <- added; errs <- err }()
	}
	count := 0
	for i := 0; i < 8; i++ {
		if <-results {
			count++
		}
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	if count != 1 {
		t.Fatalf("reported %d credits, want 1", count)
	}
}

func TestAcceptedTopupMonthCap(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("cap@example.com")
	u := w.user("cap@example.com")
	for i := 0; i < 4; i++ {
		if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
			t.Fatal(err)
		}
		w.advance(time.Minute)
	}
	if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); !errors.Is(err, store.ErrTopupCap) {
		t.Fatal(err)
	}
	has(t, b.mustGet("/billing"), "Monthly $20 top-up limit reached", "disabled", "December 1")
	w.advance(31 * 24 * time.Hour)
	if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
		t.Fatal(err)
	}
}

func TestClerkUnconfiguredAndVerificationGate(t *testing.T) {
	w := newWorld(t)
	w.app.Auth = &auth.Clerk{}
	b := w.browser()
	has(t, b.mustGet("/signin"), "Sign-in not configured")
	has(t, b.mustGet("/signup"), "Sign-up not configured")
	b.get("/billing")
	if b.path != "/signin?next=%2Fbilling" && !strings.HasPrefix(b.path, "/signin") {
		t.Fatal(b.path)
	}
}

func TestOperatorFaultDrill(t *testing.T) {
	t.Setenv("SCRY_FAULT_DRILL", "1")
	t.Setenv("SCRY_FAULT_DRILL_TOKEN", "local-drill")
	w := newWorld(t)
	req, _ := http.NewRequest("POST", w.srv.URL+"/ops/fault-drill", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
	req.Header.Set("Authorization", "Bearer local-drill")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 || !strings.Contains(w.logs.String(), `"level":"ERROR"`) {
		t.Fatal(resp.StatusCode, w.logs.String())
	}
}

func TestClerkSessionStandIn(t *testing.T) {
	w := newWorld(t)
	w.app.Auth = &auth.Clerk{SecretKey: "stand-in-only", Resolve: func(_ context.Context, token, origin string) (string, string, error) {
		if token != "verified" {
			return "", "", errors.New("verify email")
		}
		return "clerk-stand-in", "verified@example.com", nil
	}}
	b := w.browser()
	target, _ := url.Parse(w.srv.URL)
	b.c.Jar.SetCookies(target, []*http.Cookie{{Name: "__session", Value: "unverified"}})
	b.get("/billing")
	if !strings.HasPrefix(b.path, "/signin") {
		t.Fatal(b.path)
	}
	b.c.Jar.SetCookies(target, []*http.Cookie{{Name: "__session", Value: "verified"}})
	b.get("/billing")
	if b.path != "/signup/card" {
		t.Fatal(b.path)
	}
}

func TestAcceptedTopupConcurrentCap(t *testing.T) {
	w := newWorld(t)
	w.browser().signUp("caprace@example.com")
	u := w.user("caprace@example.com")
	for i := 0; i < 3; i++ {
		if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
			t.Fatal(err)
		}
		w.advance(time.Minute)
	}
	results := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		go func() {
			added, err := w.app.TopUpResult(ctx, u.ID, app.NewTopupToken())
			if err != nil && !errors.Is(err, store.ErrTopupCap) {
				t.Error(err)
			}
			results <- added
		}()
	}
	n := 0
	for i := 0; i < 12; i++ {
		if <-results {
			n++
		}
	}
	if n != 1 {
		t.Fatal(n)
	}
	sub, _ := w.store.For(u.ID).Subscription(ctx)
	if sub.TopupMicros != 4*w.app.Cfg.TopupMicros {
		t.Fatal(sub)
	}
	w.advance(time.Minute)
	if _, err := w.app.TopUpResult(ctx, u.ID, app.NewTopupToken()); !errors.Is(err, store.ErrTopupCap) {
		t.Fatal(err)
	}
}

func TestClerkConfiguredFormsUsePublishableDomain(t *testing.T) {
	w := newWorld(t)
	w.app.Auth = &auth.Clerk{
		PublishableKey: "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk",
		SecretKey:      "stand-in-only",
	}
	b := w.browser()
	for _, path := range []string{"/signup", "/signin"} {
		body := b.mustGet(path)
		has(t, body, "https://example.clerk.accounts.dev/npm/@clerk/clerk-js@5/dist/clerk.browser.js")
		has(t, body, "/static/clerk.js")
		if path == "/signup" {
			has(t, body, "Create your account", "Verify your email")
		} else {
			has(t, body, "Use your email and password", "Forgot password")
		}
		has(t, body, fmt.Sprintf("data-signup=%q", fmt.Sprint(path == "/signup")))
		if strings.Contains(body, "Sign-in not configured") {
			t.Fatal("configured Clerk did not render")
		}
	}
}
