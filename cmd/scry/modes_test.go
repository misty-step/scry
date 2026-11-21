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
	"scry/internal/speech"
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
	t.Setenv("SCRY_AI_MODE", "")
	t.Setenv("SCRY_AUTH_MODE", "")
	t.Setenv("SCRY_DRAFT_MODEL", "")
	t.Setenv("SCRY_EXPLAIN_MODEL", "")
	t.Setenv("SCRY_SPEECH_MODEL", "")
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
		// liveKeys includes OPENROUTER_API_KEY, which carries drafting too.
		if err != nil || !a.Readiness().Grading || a.Model.Name() != "jev+openrouter-drafts" {
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
		}, want{"live", true, true, true, true, "stripe-test", "clerk-live", "jev-openrouter", "openrouter", "openrouter", "Stripe test-mode keys"}},
		// OPENROUTER_API_KEY carries all four AI capabilities whenever it is
		// set, even with the direct keys also present.
		{"full live", func(t *testing.T) { modeEnv(t, "live", liveKeys) },
			want{"live", true, true, true, true, "stripe-live", "clerk-live", "jev-openrouter", "openrouter", "openrouter", ""}},
		{"full live on direct fallbacks", func(t *testing.T) {
			modeEnv(t, "live", liveKeys, map[string]string{"OPENROUTER_API_KEY": "", "TYPESAFE_API_KEY": "ts-TYPESAFEstandin0013"})
		}, want{"live", true, true, true, true, "stripe-live", "clerk-live", "jev-typesafe", "anthropic", "openai", ""}},
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
			if p["grading"] != w.gradingLabel || p["drafting"] != w.draftingLabel || p["explainer"] != w.draftingLabel || p["speech"] != w.speechLbl || p["billing"] != w.billing || p["auth"] != w.auth {
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
	for _, set := range []map[string]string{liveKeys, stripeTestKeys, adapterKeys, slice20Keys} {
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

// Slice 20 stand-ins. jwtKey has the shape of a JSON Web Token (three
// base64url segments, header {"alg":"HS256"}); it is not a real token.
var (
	jwtKey      = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzdGFuZGluIn0.c3RhbmRpbnNpZ25hdHVyZQ"
	slice20Keys = map[string]string{
		"JWT_STANDIN":        jwtKey,
		"CLERK_TEST_PK":      adapterKeys["CLERK_PUBLISHABLE_KEY"],
		"CLERK_TEST_SK":      "sk_test_CLERKSECRETstandin0014",
		"TYPESAFE_API_KEY":   "ts-TYPESAFEstandin0015",
		"OPENROUTER_ONLY":    "or-ROUTERONLYstandin0016",
		"ANTHROPIC_FALLBACK": "ant-FALLBACKstandin0017",
		"OPENAI_FALLBACK":    "oai-FALLBACKstandin0018",
	}
	// Live mode with every non-AI credential and no AI key at all.
	liveNoAI = map[string]string{"OPENROUTER_API_KEY": "", "ANTHROPIC_API_KEY": "", "OPENAI_API_KEY": "", "TYPESAFE_API_KEY": ""}
	// Clerk's development instance.
	clerkTestKeys = map[string]string{"CLERK_PUBLISHABLE_KEY": slice20Keys["CLERK_TEST_PK"], "CLERK_SECRET_KEY": slice20Keys["CLERK_TEST_SK"]}
)

func healthzWarnings(t *testing.T, h map[string]any) string {
	t.Helper()
	ws, _ := h["warnings"].([]any)
	var out []string
	for _, w := range ws {
		s, _ := w.(string)
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

func failBoot(t *testing.T, has string) error {
	t.Helper()
	a, err := tryBuild(t)
	if err == nil {
		t.Fatalf("boot succeeded (model %s, speech %s); want a loud failure saying %q", a.Model.Name(), a.Speech.Name(), has)
	}
	if !strings.Contains(err.Error(), has) {
		t.Fatalf("error %q does not say %q", err, has)
	}
	for _, set := range []map[string]string{liveKeys, stripeTestKeys, adapterKeys, slice20Keys} {
		for k, v := range set {
			if len(v) > 8 && strings.Contains(err.Error(), v) {
				t.Fatalf("boot error leaks %s: %s", k, err)
			}
		}
	}
	return err
}

func assertAllRouter(t *testing.T, a *app.App) {
	t.Helper()
	split, ok := a.Model.(ai.Split)
	if !ok {
		t.Fatalf("model is %T", a.Model)
	}
	if j, ok := split.JudgeProvider.(*ai.Jev); !ok || !j.OpenRouter {
		t.Fatalf("judge %T %+v", split.JudgeProvider, split.JudgeProvider)
	}
	if c, ok := split.DraftProvider.(*ai.Claude); !ok || !c.OpenRouter || c.DraftModel != ai.DefaultOpenRouterDraftModel || c.ExplainModel != ai.DefaultOpenRouterExplainModel {
		t.Fatalf("drafter %T %+v", split.DraftProvider, split.DraftProvider)
	}
	if c, ok := a.Explainer.(*ai.Claude); !ok || !c.OpenRouter {
		t.Fatalf("explainer %T", a.Explainer)
	}
	if _, ok := a.Speech.(speech.OpenRouter); !ok {
		t.Fatalf("speech %T", a.Speech)
	}
}

// Slice 20: live mode with ONLY OPENROUTER_API_KEY wires all four AI
// capabilities, here during a staged cutover on Stripe test keys and a
// Clerk development instance, and /healthz says so loudly with no secret.
func TestSlice20_LiveOnOpenRouterAloneWithStagedCutover(t *testing.T) {
	for _, authMode := range []string{"", "test", "clerk-test"} {
		t.Run("SCRY_AUTH_MODE="+authMode, func(t *testing.T) {
			modeEnv(t, "live", liveKeys, liveNoAI, stripeTestKeys, clerkTestKeys,
				map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_STRIPE_MODE": "test", "SCRY_AUTH_MODE": authMode})
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			assertAllRouter(t, a)
			status, raw, h := getHealthz(t, a)
			if status != 200 || h["mode"] != "live" || h["grading"] != true || h["drafting"] != true || h["explainer"] != true || h["speech"] != true ||
				h["billing_mode"] != "stripe-test" || h["auth_mode"] != "clerk-test" {
				t.Fatalf("healthz %d %s", status, raw)
			}
			p, _ := h["providers"].(map[string]any)
			if p["grading"] != "jev-openrouter" || p["drafting"] != "openrouter" || p["explainer"] != "openrouter" || p["speech"] != "openrouter" {
				t.Fatalf("providers %v", p)
			}
			ws := healthzWarnings(t, h)
			if !strings.Contains(ws, "Stripe test-mode keys") || !strings.Contains(ws, "Clerk development instance") {
				t.Fatalf("warnings %q", ws)
			}
			assertNoSecrets(t, raw)
		})
	}
}

// Live mode on live Clerk keys carries no Clerk warning; on Clerk test
// keys it always does, whether or not SCRY_AUTH_MODE asked for them.
func TestSlice20_ClerkTestWarningInLiveMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys map[string]string
		warn bool
	}{{"live keys", nil, false}, {"test keys", clerkTestKeys, true}} {
		t.Run(tc.name, func(t *testing.T) {
			modeEnv(t, "live", liveKeys, tc.keys)
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			_, raw, h := getHealthz(t, a)
			if got := strings.Contains(healthzWarnings(t, h), "Clerk development instance"); got != tc.warn {
				t.Fatalf("Clerk warning %v, want %v: %s", got, tc.warn, raw)
			}
			assertNoSecrets(t, raw)
		})
	}
	t.Run("test mode with Clerk test keys does not warn", func(t *testing.T) {
		modeEnv(t, "test", clerkTestKeys)
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		if rd := a.Readiness(); rd.AuthMode != "clerk-test" || len(rd.Warnings) != 0 {
			t.Fatalf("readiness %+v", rd)
		}
	})
}

// SCRY_AUTH_MODE pins Clerk to one instance kind and fails closed.
func TestSlice20_AuthModeSelection(t *testing.T) {
	t.Run("live with live keys", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AUTH_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil || a.Readiness().AuthMode != "clerk-live" {
			t.Fatal(err)
		}
	})
	t.Run("live refuses pk_test under SCRY_AUTH_MODE=live", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AUTH_MODE": "live", "CLERK_PUBLISHABLE_KEY": slice20Keys["CLERK_TEST_PK"]})
		failBoot(t, "SCRY_AUTH_MODE=live needs live Clerk keys")
	})
	t.Run("live refuses all test keys under SCRY_AUTH_MODE=live", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, clerkTestKeys, map[string]string{"SCRY_AUTH_MODE": "live"})
		failBoot(t, "CLERK_PUBLISHABLE_KEY is not a live key")
	})
	for _, m := range []string{"test", "clerk-test"} {
		t.Run("live refuses live keys under SCRY_AUTH_MODE="+m, func(t *testing.T) {
			modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AUTH_MODE": m})
			failBoot(t, "SCRY_AUTH_MODE="+m+" needs Clerk test keys")
		})
		t.Run("test mode explicit "+m+" needs Clerk keys", func(t *testing.T) {
			modeEnv(t, "test", map[string]string{"SCRY_AUTH_MODE": m})
			failBoot(t, "SCRY_AUTH_MODE="+m+" needs CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY")
		})
		t.Run("test mode explicit "+m+" with test keys", func(t *testing.T) {
			modeEnv(t, "test", clerkTestKeys, map[string]string{"SCRY_AUTH_MODE": m})
			a, err := tryBuild(t)
			if err != nil || a.Readiness().AuthMode != "clerk-test" {
				t.Fatal(err)
			}
		})
	}
	t.Run("test mode refuses SCRY_AUTH_MODE=live", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"SCRY_AUTH_MODE": "live"})
		failBoot(t, "SCRY_AUTH_MODE=live is refused with SCRY_MODE=test")
	})
	t.Run("mixed Clerk instances are refused", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"CLERK_SECRET_KEY": slice20Keys["CLERK_TEST_SK"]})
		failBoot(t, "different Clerk instances")
	})
	t.Run("invalid value", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AUTH_MODE": "prod"})
		failBoot(t, "SCRY_AUTH_MODE must be")
	})
}

// SCRY_AI_MODE=live puts all four real AI adapters on a test build, on
// purpose and loudly; live mode never stubs AI.
func TestSlice20_AIModeSelection(t *testing.T) {
	t.Run("test mode, live AI via OpenRouter", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_AI_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		assertAllRouter(t, a)
		_, raw, h := getHealthz(t, a)
		if h["mode"] != "test" || h["grading"] != true || h["drafting"] != true || h["explainer"] != true || h["speech"] != true || h["billing_mode"] != "test" || h["auth_mode"] != "local" {
			t.Fatalf("healthz %s", raw)
		}
		ws := healthzWarnings(t, h)
		if !strings.Contains(ws, "SCRY_AI_MODE=live") || !strings.Contains(ws, "speech (openrouter)") || !strings.Contains(ws, "real Jev grader") {
			t.Fatalf("warnings %q", ws)
		}
		assertNoSecrets(t, raw)
	})
	t.Run("test mode, live AI via direct fallbacks", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"TYPESAFE_API_KEY": slice20Keys["TYPESAFE_API_KEY"], "ANTHROPIC_API_KEY": slice20Keys["ANTHROPIC_FALLBACK"], "OPENAI_API_KEY": slice20Keys["OPENAI_FALLBACK"], "SCRY_AI_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		rd := a.Readiness()
		if rd.Providers["grading"] != "jev-typesafe" || rd.Providers["drafting"] != "anthropic" || rd.Providers["explainer"] != "anthropic" || rd.Providers["speech"] != "openai" || !rd.Speech {
			t.Fatalf("readiness %+v", rd)
		}
		_, raw, _ := getHealthz(t, a)
		assertNoSecrets(t, raw)
	})
	t.Run("test mode, live AI without keys fails loud", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"SCRY_AI_MODE": "live"})
		failBoot(t, "SCRY_AI_MODE=live needs OPENROUTER_API_KEY or TYPESAFE_API_KEY, OPENROUTER_API_KEY or ANTHROPIC_API_KEY, OPENROUTER_API_KEY or OPENAI_API_KEY")
	})
	t.Run("test mode, live AI contradicts a stubbed judge", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_AI_MODE": "live", "SCRY_JUDGE_MODE": "test"})
		failBoot(t, "contradicts SCRY_AI_MODE=live")
	})
	t.Run("test mode, explicit test AI stays fake", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_AI_MODE": "test"})
		a, err := tryBuild(t)
		if err != nil || a.Model.Name() != "test" || a.Speech.Name() != "test" || a.Explainer.Name() != "test" {
			t.Fatal(err)
		}
	})
	t.Run("live mode refuses stubbed AI", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AI_MODE": "test"})
		failBoot(t, "SCRY_AI_MODE=test is refused with SCRY_MODE=live")
	})
	t.Run("live mode explicit live AI", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_AI_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		assertAllRouter(t, a)
	})
	t.Run("invalid value", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"SCRY_AI_MODE": "real"})
		failBoot(t, "SCRY_AI_MODE must be")
	})
	t.Run("SCRY_JUDGE_MODE=live still grades alone", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_JUDGE_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		rd := a.Readiness()
		if !rd.Grading || rd.Drafting || rd.Explainer || rd.Speech || strings.Contains(strings.Join(rd.Warnings, "\n"), "SCRY_AI_MODE=live) for") {
			t.Fatalf("readiness %+v", rd)
		}
	})
}

// Live mode on direct keys, with OPENROUTER_API_KEY unset, still works;
// every missing direct key and every JWT-shaped key is named loudly, and
// no error carries a key's value.
func TestSlice20_LiveDirectFallbacksAndKeyChecks(t *testing.T) {
	direct := map[string]string{"TYPESAFE_API_KEY": slice20Keys["TYPESAFE_API_KEY"], "ANTHROPIC_API_KEY": slice20Keys["ANTHROPIC_FALLBACK"], "OPENAI_API_KEY": slice20Keys["OPENAI_FALLBACK"]}
	t.Run("direct Typesafe, Anthropic and OpenAI", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, liveNoAI, direct)
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		split := a.Model.(ai.Split)
		if c, ok := split.DraftProvider.(*ai.Claude); !ok || c.OpenRouter || c.DraftModel != ai.DefaultDraftModel {
			t.Fatalf("drafter %+v", split.DraftProvider)
		}
		if _, ok := a.Speech.(speech.OpenAI); !ok {
			t.Fatalf("speech %T", a.Speech)
		}
		_, raw, h := getHealthz(t, a)
		p, _ := h["providers"].(map[string]any)
		if p["grading"] != "jev-typesafe" || p["drafting"] != "anthropic" || p["explainer"] != "anthropic" || p["speech"] != "openai" || h["speech"] != true {
			t.Fatalf("healthz %s", raw)
		}
		assertNoSecrets(t, raw)
	})
	t.Run("no AI key at all names every alternative", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, liveNoAI)
		failBoot(t, "OPENROUTER_API_KEY or TYPESAFE_API_KEY, OPENROUTER_API_KEY or ANTHROPIC_API_KEY, OPENROUTER_API_KEY or OPENAI_API_KEY")
	})
	t.Run("JWT-shaped OPENAI_API_KEY on the fallback is refused", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, liveNoAI, direct, map[string]string{"OPENAI_API_KEY": jwtKey})
		err := failBoot(t, "OPENAI_API_KEY is JWT-shaped")
		if strings.Contains(err.Error(), "eyJ") {
			t.Fatalf("boot error echoes the key: %s", err)
		}
	})
	t.Run("JWT-shaped ANTHROPIC_API_KEY on the fallback is refused", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, liveNoAI, direct, map[string]string{"ANTHROPIC_API_KEY": jwtKey})
		failBoot(t, "ANTHROPIC_API_KEY is JWT-shaped")
	})
	t.Run("JWT-shaped OPENROUTER_API_KEY is refused once", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"OPENROUTER_API_KEY": jwtKey})
		err := failBoot(t, "OPENROUTER_API_KEY is JWT-shaped")
		if n := strings.Count(err.Error(), "JWT-shaped"); n != 1 {
			t.Fatalf("reported %d times: %s", n, err)
		}
	})
	t.Run("JWT-shaped OPENAI_API_KEY is ignored when OpenRouter carries speech", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"OPENAI_API_KEY": jwtKey})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		assertAllRouter(t, a)
	})
	t.Run("a direct Anthropic model name on OpenRouter is refused", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_DRAFT_MODEL": "claude-opus-5-5"})
		failBoot(t, `SCRY_DRAFT_MODEL="claude-opus-5-5" is not an OpenRouter model slug`)
	})
	t.Run("model overrides reach every OpenRouter adapter", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_DRAFT_MODEL": "anthropic/claude-opus-5.5", "SCRY_EXPLAIN_MODEL": "anthropic/claude-haiku-5.5", "SCRY_SPEECH_MODEL": "google/gemini-2.5-pro"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		c := a.Explainer.(*ai.Claude)
		if c.DraftModel != "anthropic/claude-opus-5.5" || c.ExplainModel != "anthropic/claude-haiku-5.5" || a.Speech.(speech.OpenRouter).Model != "google/gemini-2.5-pro" {
			t.Fatalf("models %+v %+v", c, a.Speech)
		}
	})
}

// SCRY_JUDGE_MODE=live in test mode runs the same key check as realAI on
// the key Jev will use: a JWT-shaped key stops the boot loudly, naming the
// variable and never echoing the value.
func TestSlice20_JudgeModeLiveRefusesJWTShapedKeys(t *testing.T) {
	t.Run("JWT-shaped OPENROUTER_API_KEY", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": jwtKey, "SCRY_JUDGE_MODE": "live"})
		err := failBoot(t, "SCRY_JUDGE_MODE=live: OPENROUTER_API_KEY is JWT-shaped")
		if strings.Contains(err.Error(), "eyJ") {
			t.Fatalf("boot error echoes the key: %s", err)
		}
	})
	t.Run("JWT-shaped TYPESAFE_API_KEY on the fallback", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"TYPESAFE_API_KEY": jwtKey, "SCRY_JUDGE_MODE": "live"})
		failBoot(t, "SCRY_JUDGE_MODE=live: TYPESAFE_API_KEY is JWT-shaped")
	})
	t.Run("an unused JWT-shaped TYPESAFE_API_KEY does not block OpenRouter", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "TYPESAFE_API_KEY": jwtKey, "SCRY_JUDGE_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil || a.Readiness().Providers["grading"] != "jev-openrouter" {
			t.Fatal(err)
		}
	})
}

// A test build never accepts a live Clerk key, whatever SCRY_AUTH_MODE
// says (as with Stripe); under SCRY_AUTH_MODE=test or clerk-test every
// Clerk key set must be a test key.
func TestSlice20_ClerkKeysInTestMode(t *testing.T) {
	for _, authMode := range []string{"", "test", "clerk-test"} {
		for _, tc := range []struct {
			name, pk, sk, want string
		}{
			{"live pk and sk", clerkLivePK, liveKeys["CLERK_SECRET_KEY"], "CLERK_PUBLISHABLE_KEY is a live key"},
			{"live pk only", clerkLivePK, "", "CLERK_PUBLISHABLE_KEY is a live key"},
			{"live sk only", "", liveKeys["CLERK_SECRET_KEY"], "CLERK_SECRET_KEY is a live key"},
			{"live sk with test pk", slice20Keys["CLERK_TEST_PK"], liveKeys["CLERK_SECRET_KEY"], "CLERK_SECRET_KEY is a live key"},
		} {
			t.Run("SCRY_AUTH_MODE="+authMode+"/"+tc.name, func(t *testing.T) {
				modeEnv(t, "test", map[string]string{"CLERK_PUBLISHABLE_KEY": tc.pk, "CLERK_SECRET_KEY": tc.sk, "SCRY_AUTH_MODE": authMode})
				failBoot(t, tc.want)
			})
		}
	}
	for _, m := range []string{"test", "clerk-test"} {
		for _, tc := range []struct{ name, mode string }{{"test mode", "test"}, {"live mode", "live"}} {
			t.Run("SCRY_AUTH_MODE="+m+" refuses a key of no known kind in "+tc.name, func(t *testing.T) {
				sets := []map[string]string{clerkTestKeys, {"SCRY_AUTH_MODE": m, "CLERK_SECRET_KEY": "CLERKSECRETnoprefix0019"}}
				if tc.mode == "live" {
					sets = append([]map[string]string{liveKeys}, sets...)
				}
				modeEnv(t, tc.mode, sets...)
				failBoot(t, "SCRY_AUTH_MODE="+m+" needs Clerk test keys, but CLERK_SECRET_KEY is not a test key")
			})
		}
	}
	t.Run("test keys still boot with SCRY_AUTH_MODE unset", func(t *testing.T) {
		modeEnv(t, "test", clerkTestKeys)
		a, err := tryBuild(t)
		if err != nil || a.Readiness().AuthMode != "clerk-test" {
			t.Fatal(err)
		}
	})
}
