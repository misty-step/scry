package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"scry/internal/alert"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRule_TestModeNeverCallsAlertWebhook(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	t.Setenv("SCRY_MODE", "test")
	t.Setenv("SCRY_DB", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("ALERT_WEBHOOK_URL", srv.URL)
	a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Store.Close()
	a.Alert.Notify(context.Background(), "test", "test")
	if calls.Load() != 0 {
		t.Fatal("test mode sent an external alert")
	}
}

// All values here are stand-ins; tests never require provider credentials.
var adapterKeys = map[string]string{
	"CLERK_PUBLISHABLE_KEY":  "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk",
	"CLERK_SECRET_KEY":       "sk_test_standin",
	"STRIPE_SECRET_KEY":      "sk_test_standin",
	"STRIPE_PUBLISHABLE_KEY": "pk_test_standin",
	"STRIPE_PRICE_ID":        "price_standin",
	"STRIPE_WEBHOOK_SECRET":  "whsec_standin",
	"OPENROUTER_API_KEY":     "standin",
	"ANTHROPIC_API_KEY":      "standin",
	"OPENAI_API_KEY":         "standin",
}

func adapterEnv(t *testing.T, mode string) {
	t.Helper()
	for key := range adapterKeys {
		t.Setenv(key, "")
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("SCRY_MODE", mode)
	t.Setenv("SCRY_DB", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("BACKUP_DIR", "")
	t.Setenv("ALERT_WEBHOOK_URL", "")
}

func TestAdapterSelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		keys          []string
		auth, billing string
	}{
		{"no keys", nil, "test", "test"},
		{"Clerk", []string{"CLERK_PUBLISHABLE_KEY", "CLERK_SECRET_KEY"}, "clerk", "test"},
		{"partial Clerk", []string{"CLERK_PUBLISHABLE_KEY"}, "test", "test"},
		{"Stripe", []string{"STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY", "STRIPE_PRICE_ID", "STRIPE_WEBHOOK_SECRET"}, "test", "stripe-test"},
		{"partial Stripe", []string{"STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY", "STRIPE_PRICE_ID"}, "test", "test"},
		{"both", []string{"CLERK_PUBLISHABLE_KEY", "CLERK_SECRET_KEY", "STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY", "STRIPE_PRICE_ID", "STRIPE_WEBHOOK_SECRET"}, "clerk", "stripe-test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapterEnv(t, "test")
			for _, key := range tc.keys {
				t.Setenv(key, adapterKeys[key])
			}
			// Model credentials must not enable model or speech in test mode.
			for _, key := range []string{"OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
				t.Setenv(key, adapterKeys[key])
			}
			a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Store.Close()
			if a.Auth.Name() != tc.auth || a.Pay.Name() != tc.billing || a.Model.Name() != "test" || a.Speech.Name() != "test" {
				t.Fatalf("adapters: %s %s %s %s", a.Auth.Name(), a.Pay.Name(), a.Model.Name(), a.Speech.Name())
			}
			t.Setenv("SCRY_SEED_DEMO", "1")
			if shouldSeedDemo(a) != (tc.auth == "test") {
				t.Fatal("demo must only seed with local auth")
			}
			t.Setenv("SCRY_SEED_DEMO", "")
			if shouldSeedDemo(a) {
				t.Fatal("demo seeded without opt-in")
			}
		})
	}
}

func TestLiveRequiresEveryCredential(t *testing.T) {
	for missing := range adapterKeys {
		t.Run(missing, func(t *testing.T) {
			adapterEnv(t, "live")
			for key, value := range adapterKeys {
				t.Setenv(key, value)
			}
			t.Setenv(missing, "")
			a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if a != nil {
				a.Store.Close()
			}
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing %s: %v", missing, err)
			}
		})
	}
}

func TestTestModeRefusesLiveStripe(t *testing.T) {
	for _, key := range []string{"STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY"} {
		t.Run(key, func(t *testing.T) {
			adapterEnv(t, "test")
			for k, v := range adapterKeys {
				t.Setenv(k, v)
			}
			t.Setenv(key, strings.Replace(adapterKeys[key], "_test_", "_live_", 1))
			a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if a != nil {
				a.Store.Close()
			}
			if err == nil {
				t.Fatal("accepted live Stripe key")
			}
		})
	}
}

func TestTestModeDisablesOperatorErrorReporting(t *testing.T) {
	base := slog.NewTextHandler(io.Discard, nil)
	if operatorHandler(base, true) != base {
		t.Fatal("test mode installed external error reporting")
	}
	if _, ok := operatorHandler(base, false).(alert.SentryHandler); !ok {
		t.Fatal("live mode lost Sentry reporting")
	}
}
