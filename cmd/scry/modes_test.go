package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/web"
)

// Distinctive stand-ins so a leak into /healthz is easy to spot. None is a
// real credential.
var (
	clerkLivePK = "pk_live_" + base64.RawStdEncoding.EncodeToString([]byte("clerk.scry.example.test$"))
	liveKeys    = map[string]string{
		"CLERK_PUBLISHABLE_KEY":  clerkLivePK,
		"CLERK_SECRET_KEY":       "sk_live_CLERKSECRETstandin0001",
		"STRIPE_SECRET_KEY":      "sk_live_STRIPESECRETstandin0002",
		"STRIPE_PUBLISHABLE_KEY": "pk_live_STRIPEPUBstandin0003",
		"STRIPE_PRICE_ID":        "price_LIVEstandin0004",
		"STRIPE_WEBHOOK_SECRET":  "whsec_LIVEstandin0005",
		"OPENROUTER_API_KEY":     "or-ROUTERKEYstandin0006",
		"ANTHROPIC_API_KEY":      "ant-ANTHROPICstandin0007",
		"OPENAI_API_KEY":         "oai-OPENAIstandin0008",
	}
	stripeTestKeys = map[string]string{
		"STRIPE_SECRET_KEY":      "sk_test_STRIPESECRETstandin0009",
		"STRIPE_PUBLISHABLE_KEY": "pk_test_STRIPEPUBstandin0010",
		"STRIPE_PRICE_ID":        "price_TESTstandin0011",
		"STRIPE_WEBHOOK_SECRET":  "whsec_TESTstandin0012",
	}
)

func modeEnv(t *testing.T, mode string, sets ...map[string]string) {
	t.Helper()
	adapterEnv(t, mode)
	t.Setenv("SCRY_STRIPE_MODE", "")
	t.Setenv("SCRY_JUDGE_MODE", "")
	t.Setenv("SCRY_JUDGE_MODEL", "")
	t.Setenv("SENTRY_DSN", "")
	for _, set := range sets {
		for k, v := range set {
			t.Setenv(k, v)
		}
	}
}

func tryBuild(t *testing.T) (*app.App, error) {
	t.Helper()
	a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if a != nil {
		t.Cleanup(func() { a.Store.Close() })
	}
	return a, err
}

func mustFail(t *testing.T, has string) {
	t.Helper()
	a, err := tryBuild(t)
	if err == nil {
		t.Fatalf("boot succeeded (billing %s, model %s); want a loud failure", a.Pay.Name(), a.Model.Name())
	}
	if !strings.Contains(err.Error(), has) {
		t.Fatalf("error %q does not say %q", err, has)
	}
}

// Slice 12: Stripe's mode follows SCRY_MODE unless SCRY_STRIPE_MODE asks
// otherwise, and every unsafe or ambiguous combination stops the boot.
func TestStripeModeSelection(t *testing.T) {
	for _, tc := range []struct {
		name, mode, stripeMode string
		keys                   map[string]string
		billing                string // "" means boot must fail
		errHas                 string
	}{
		{"test default with test keys", "test", "", stripeTestKeys, "stripe-test", ""},
		{"test explicit test", "test", "test", stripeTestKeys, "stripe-test", ""},
		{"test explicit test without keys", "test", "test", nil, "", "SCRY_STRIPE_MODE=test needs"},
		{"test with live keys", "test", "", map[string]string{"STRIPE_SECRET_KEY": "sk_live_x", "STRIPE_PUBLISHABLE_KEY": "pk_live_x", "STRIPE_PRICE_ID": "p", "STRIPE_WEBHOOK_SECRET": "w"}, "", "live-mode key"},
		{"test refuses SCRY_STRIPE_MODE=live", "test", "live", liveKeys, "", "SCRY_STRIPE_MODE=live is refused"},
		{"test refuses invalid", "test", "prod", nil, "", "SCRY_STRIPE_MODE must be"},
		{"live default is stripe-live", "live", "", nil, "stripe-live", ""},
		{"live explicit live", "live", "live", nil, "stripe-live", ""},
		{"live default refuses test keys", "live", "", stripeTestKeys, "", "test-mode key"},
		{"live staged cutover to test keys", "live", "test", stripeTestKeys, "stripe-test", ""},
		{"live staged cutover refuses live keys", "live", "test", nil, "", "live-mode key"},
		{"live refuses mixed keys", "live", "", map[string]string{"STRIPE_PUBLISHABLE_KEY": "pk_test_x"}, "", "STRIPE_PUBLISHABLE_KEY is a test-mode key"},
		{"live refuses invalid", "live", "sandbox", nil, "", "SCRY_STRIPE_MODE must be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mode == "live" {
				modeEnv(t, "live", liveKeys, tc.keys)
			} else {
				modeEnv(t, "test", tc.keys)
			}
			t.Setenv("SCRY_STRIPE_MODE", tc.stripeMode)
			if tc.billing == "" {
				mustFail(t, tc.errHas)
				return
			}
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			if a.Pay.Name() != tc.billing {
				t.Fatalf("billing %s, want %s", a.Pay.Name(), tc.billing)
			}
			rd := a.Readiness()
			warned := strings.Contains(strings.Join(rd.Warnings, "\n"), "Stripe test-mode keys")
			if want := tc.mode == "live" && tc.billing == "stripe-test"; warned != want {
				t.Fatalf("staged-cutover warning = %v, want %v (%v)", warned, want, rd.Warnings)
			}
		})
	}
}

// SCRY_JUDGE_MODE lets test mode grade with real Jev on purpose, and never
// lets live mode stub grading.
func TestJudgeModeSelection(t *testing.T) {
	t.Run("test unset stays fake even with keys", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": "k", "TYPESAFE_API_KEY": "k"})
		a, err := tryBuild(t)
		if err != nil || a.Model.Name() != "test" || a.Readiness().Grading {
			t.Fatal(err, a.Model.Name())
		}
		if _, fake := a.Model.(*ai.Fake); !fake {
			t.Fatalf("model is %T", a.Model)
		}
	})
	t.Run("test explicit test stays fake", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": "k"})
		t.Setenv("SCRY_JUDGE_MODE", "test")
		a, err := tryBuild(t)
		if err != nil || a.Model.Name() != "test" {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		name, key, label string
	}{{"openrouter", "OPENROUTER_API_KEY", "jev-openrouter"}, {"typesafe", "TYPESAFE_API_KEY", "jev-typesafe"}} {
		t.Run("test live judge via "+tc.name, func(t *testing.T) {
			modeEnv(t, "test", map[string]string{tc.key: "k"})
			t.Setenv("SCRY_JUDGE_MODE", "live")
			t.Setenv("SCRY_JUDGE_MODEL", "custom/decisions-model")
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			split, ok := a.Model.(ai.Split)
			if !ok {
				t.Fatalf("model is %T", a.Model)
			}
			jev, ok := split.JudgeProvider.(*ai.Jev)
			if !ok || jev.Model != "custom/decisions-model" {
				t.Fatalf("judge %T %+v", split.JudgeProvider, jev)
			}
			if _, fake := split.DraftProvider.(*ai.Fake); !fake {
				t.Fatalf("drafts are %T, want the fake", split.DraftProvider)
			}
			rd := a.Readiness()
			if !rd.Grading || rd.Drafting || rd.Providers["grading"] != tc.label || rd.Providers["drafting"] != "test" || a.Model.Name() != "jev+test-drafts" {
				t.Fatalf("readiness %+v name %s", rd, a.Model.Name())
			}
			if a.Pay.Name() != "test" || a.Speech.Name() != "test" {
				t.Fatal("live judge enabled other adapters")
			}
		})
	}
	t.Run("test live judge without a key fails loud", func(t *testing.T) {
		modeEnv(t, "test")
		t.Setenv("SCRY_JUDGE_MODE", "live")
		mustFail(t, "SCRY_JUDGE_MODE=live needs OPENROUTER_API_KEY or TYPESAFE_API_KEY")
	})
	t.Run("live refuses a stubbed judge", func(t *testing.T) {
		modeEnv(t, "live", liveKeys)
		t.Setenv("SCRY_JUDGE_MODE", "test")
		mustFail(t, "SCRY_JUDGE_MODE=test is refused")
	})
	t.Run("live explicit live judge", func(t *testing.T) {
		modeEnv(t, "live", liveKeys)
		t.Setenv("SCRY_JUDGE_MODE", "live")
		a, err := tryBuild(t)
		if err != nil || !a.Readiness().Grading || a.Model.Name() != "jev+anthropic-drafts" {
			t.Fatal(err)
		}
	})
	t.Run("invalid judge mode", func(t *testing.T) {
		modeEnv(t, "test")
		t.Setenv("SCRY_JUDGE_MODE", "real")
		mustFail(t, "SCRY_JUDGE_MODE must be")
	})
}

func TestBuildSHA(t *testing.T) {
	old := buildSHA
	t.Cleanup(func() { buildSHA = old })
	buildSHA = ""
	if got := resolveBuildSHA(); got != "dev" && len(got) != 7 {
		t.Fatalf("fallback sha %q", got)
	}
	buildSHA = "abc1234"
	modeEnv(t, "test")
	a, err := tryBuild(t)
	if err != nil || a.Cfg.BuildSHA != "abc1234" || a.Readiness().BuildSHA != "abc1234" {
		t.Fatal(err, a.Cfg.BuildSHA)
	}
}

func getHealthz(t *testing.T, a *app.App) (int, string, map[string]any) {
	t.Helper()
	srv := httptest.NewServer(web.New(a, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("healthz is not JSON: %s", raw)
	}
	return resp.StatusCode, string(raw), body
}

// /healthz across the supported wirings, built exactly as the binary
// builds them, with no secret anywhere in the body.
func TestHealthzAcrossModes(t *testing.T) {
	type want struct {
		mode                                   string
		grading, drafting, explainer, speech   bool
		billing, auth                          string
		gradingLabel, draftingLabel, speechLbl string
		warning                                string
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
		want  want
	}{
		{"default test", func(t *testing.T) { modeEnv(t, "test") },
			want{"test", false, false, false, false, "test", "local", "test", "test", "test", ""}},
		{"test with live judge", func(t *testing.T) {
			modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": liveKeys["OPENROUTER_API_KEY"]})
			t.Setenv("SCRY_JUDGE_MODE", "live")
		}, want{"test", true, false, false, false, "test", "local", "jev-openrouter", "test", "test", "real Jev grader"}},
		{"test with Clerk and Stripe test", func(t *testing.T) {
			modeEnv(t, "test", stripeTestKeys, map[string]string{"CLERK_PUBLISHABLE_KEY": adapterKeys["CLERK_PUBLISHABLE_KEY"], "CLERK_SECRET_KEY": "sk_test_CLERKstandin"})
		}, want{"test", false, false, false, false, "stripe-test", "clerk-test", "test", "test", "test", ""}},
		{"live with Stripe test cutover", func(t *testing.T) {
			modeEnv(t, "live", liveKeys, stripeTestKeys)
			t.Setenv("SCRY_STRIPE_MODE", "test")
		}, want{"live", true, true, true, true, "stripe-test", "clerk-live", "jev-openrouter", "anthropic", "openai", "Stripe test-mode keys"}},
		{"full live", func(t *testing.T) { modeEnv(t, "live", liveKeys) },
			want{"live", true, true, true, true, "stripe-live", "clerk-live", "jev-openrouter", "anthropic", "openai", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			old := buildSHA
			buildSHA = "feedbee"
			t.Cleanup(func() { buildSHA = old })
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			status, raw, h := getHealthz(t, a)
			w := tc.want
			if status != 200 || h["status"] != "ok" || h["database"] != "ok" || h["db"] != true || !strings.Contains(raw, `"status":"ok"`) {
				t.Fatalf("healthz %d %s", status, raw)
			}
			if h["build_sha"] != "feedbee" || h["mode"] != w.mode || h["grading"] != w.grading || h["drafting"] != w.drafting ||
				h["explainer"] != w.explainer || h["speech"] != w.speech || h["billing_mode"] != w.billing || h["auth_mode"] != w.auth {
				t.Fatalf("healthz %s", raw)
			}
			p, _ := h["providers"].(map[string]any)
			if p["grading"] != w.gradingLabel || p["drafting"] != w.draftingLabel || p["speech"] != w.speechLbl || p["billing"] != w.billing || p["auth"] != w.auth {
				t.Fatalf("providers %v", p)
			}
			if len(p) != 6 {
				t.Fatalf("providers has %d labels, want grading, drafting, explainer, speech, billing, auth: %v", len(p), p)
			}
			warnings, _ := h["warnings"].([]any)
			if (w.warning == "") != (len(warnings) == 0) || (w.warning != "" && !strings.Contains(raw, w.warning)) {
				t.Fatalf("warnings %v, want %q", warnings, w.warning)
			}
			assertNoSecrets(t, raw)
		})
	}
}

func assertNoSecrets(t *testing.T, raw string) {
	t.Helper()
	for _, set := range []map[string]string{liveKeys, stripeTestKeys, adapterKeys} {
		for k, v := range set {
			if k == "STRIPE_PRICE_ID" {
				continue // a price id is not a secret, but it is not reported either
			}
			if strings.Contains(raw, v) {
				t.Fatalf("/healthz leaks %s: %s", k, raw)
			}
		}
	}
	for _, marker := range []string{"sk_", "pk_", "rk_", "whsec_", "standin", "dsn", "token", "Bearer"} {
		if strings.Contains(raw, marker) {
			t.Fatalf("/healthz contains %q: %s", marker, raw)
		}
	}
}

// A database that cannot be written turns /healthz into a 503 that keeps
// the fields scry-lib.sh and scry-watch read.
func TestHealthzDatabaseFailure(t *testing.T) {
	modeEnv(t, "test")
	a, err := tryBuild(t)
	if err != nil {
		t.Fatal(err)
	}
	a.Store.Close()
	status, raw, h := getHealthz(t, a)
	if status != http.StatusServiceUnavailable || h["status"] != "unhealthy" || h["database"] != "failing" || h["db"] != false {
		t.Fatalf("healthz %d %s", status, raw)
	}
	if strings.Contains(raw, `"status":"ok"`) || h["mode"] != "test" || h["build_sha"] == "" {
		t.Fatalf("healthz %s", raw)
	}
	assertNoSecrets(t, raw)
}
