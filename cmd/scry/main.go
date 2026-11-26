// Command scry runs Scry: one service, server-rendered, on one SQLite file.
//
//	scry                         serve (the default)
//	scry healthcheck             exit 0 if the running server is healthy
//	scry backup [dir]            write a snapshot of the database
//	scry restore-user SNAP EMAIL restore one learner's data from a snapshot
//
// Grading ([S113-1]) is one ordered chain of models, configured with:
//
//	SCRY_JUDGE_MODEL             the primary decisions model (default typesafe/jev-1.14 on OpenRouter, jev-latest on Typesafe)
//	SCRY_JUDGE_FALLBACK_MODELS   comma-separated fallback decisions models, in order; unset means
//	                             openai/gpt-6-luna-decisions,google/gemini-decision-1; set but empty means none
//	SCRY_JUDGE_ESCALATION_MODEL  the LLM asked once when no decisions model is certain (default google/gemini-4-hydrogen)
//
// On OPENROUTER_API_KEY every one must be a vendor/model slug or the boot
// stops. Without it the chain is Jev on Typesafe alone and the escalation is
// the drafting model on Anthropic (the two OpenRouter-only settings are
// ignored, with a warning). In test mode the chain is a fixture:
//
//	SCRY_TEST_JUDGMENTS    verdict script, one per submission: right, close, missed
//	SCRY_TEST_JUDGE_PATHS  chain-path script, one per submission (see ai.FakeJudgePaths): primary,
//	                       primary-unavailable, primary-error, primary-unparseable, chain-unavailable,
//	                       primary-low-confidence, primary-low-probability, primary-thin-margin,
//	                       primary-none, all-fail
//	SCRY_TEST_JUDGE_DELAY  a Go duration each judgment waits first (e.g. 3500ms), to see "still checking"
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; learners still have time zones

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/safeerr"
	"scry/internal/speech"
	"scry/internal/srs"
	"scry/internal/store"
	"scry/internal/web"
)

// buildSHA is set at link time: -ldflags "-X main.buildSHA=<sha>". When it
// is not, the Go toolchain's VCS stamp is used, else "dev".
var buildSHA = ""

func resolveBuildSHA() string {
	if buildSHA != "" {
		return buildSHA
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range info.Settings {
			if kv.Key == "vcs.revision" && kv.Value != "" {
				if len(kv.Value) > 7 {
					return kv.Value[:7]
				}
				return kv.Value
			}
		}
	}
	return "dev"
}

// modeChoice reads an optional explicit adapter mode: "", "live" or "test".
// Anything else stops the process.
func modeChoice(key string) (string, error) {
	switch v := env(key, ""); v {
	case "", "live", "test":
		return v, nil
	default:
		return "", fmt.Errorf("%s must be live, test or unset, not %q", key, v)
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func main() {
	level := slog.LevelInfo
	if env("LOG_LEVEL", "") == "debug" {
		level = slog.LevelDebug
	}
	test := env("SCRY_MODE", "live") == "test"
	if !test {
		if err := sentry.Init(sentry.ClientOptions{Dsn: env("SENTRY_DSN", ""), SendDefaultPII: false}); err != nil {
			fmt.Fprintln(os.Stderr, "invalid Sentry configuration")
			os.Exit(1)
		}
		defer sentry.Flush(2 * time.Second)
	}
	log := slog.New(alert.Watch(operatorHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}), test), env("SCRY_WATCH_WEBHOOK_URL", ""), env("SCRY_WATCH_WEBHOOK_SECRET", ""), test))
	slog.SetDefault(log)
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(log)
	case "healthcheck":
		err = healthcheck()
	case "backup":
		err = backup(log)
	case "restore-user":
		err = restoreUser(log)
	default:
		err = fmt.Errorf("unknown command %q (serve, healthcheck, backup, restore-user)", cmd)
	}
	if err != nil {
		log.Error("scry stopped", "err", err.Error())
		os.Exit(1)
	}
}

func operatorHandler(base slog.Handler, test bool) slog.Handler {
	if test {
		return base
	}
	return alert.SentryHandler{Handler: base}
}

// build wires auth and billing independently in test mode; model and speech
// stay fake. Live mode uses only real adapters
// with credentials from the environment. Outside test mode a missing
// credential stops the process; nothing falls back to a test adapter.
//
// The four AI capabilities (grading, drafting, explainer, speech) all run
// on OPENROUTER_API_KEY when it is set; only when it is unset does each
// fall back to its direct key (TYPESAFE_API_KEY, ANTHROPIC_API_KEY,
// OPENAI_API_KEY). See realAI.
//
// Explicit, optional overrides exist for a staged cutover, and every
// combination that could silently stub, silently charge or silently sign
// learners into the wrong user pool is refused:
//
//	SCRY_STRIPE_MODE=test  in live mode: Stripe test keys (stripe-test), surfaced in /healthz
//	SCRY_STRIPE_MODE=live  in test mode: refused (a test build never charges real cards)
//	SCRY_JUDGE_MODE=live   in test mode: real Jev grades, drafts stay fake; needs a valid Jev key
//	SCRY_JUDGE_MODE=test   in live mode: refused (live grading is never stubbed)
//	SCRY_AI_MODE=live      in test mode: all four real AI adapters, surfaced in /healthz
//	SCRY_AI_MODE=test      in live mode: refused (live AI is never stubbed)
//	SCRY_AUTH_MODE=test|clerk-test in live mode: Clerk test keys required, surfaced in /healthz
//	SCRY_AUTH_MODE=live    in live mode: live Clerk keys required; in test mode: refused
//	live Clerk keys        in test mode: refused, whatever SCRY_AUTH_MODE says
//
// Live mode with Clerk test keys and SCRY_AUTH_MODE unset still boots, as
// before, but /healthz warns about it.
func build(log *slog.Logger) (*app.App, error) {
	mode := env("SCRY_MODE", "live")
	if mode != "test" && mode != "live" {
		return nil, fmt.Errorf("SCRY_MODE must be test or live, not %q", mode)
	}
	test := mode == "test"
	stripeMode, err := modeChoice("SCRY_STRIPE_MODE")
	if err != nil {
		return nil, err
	}
	judgeMode, err := modeChoice("SCRY_JUDGE_MODE")
	if err != nil {
		return nil, err
	}
	aiMode, err := modeChoice("SCRY_AI_MODE")
	if err != nil {
		return nil, err
	}
	authMode := env("SCRY_AUTH_MODE", "")
	switch authMode {
	case "", "live", "test", "clerk-test":
	default:
		return nil, fmt.Errorf("SCRY_AUTH_MODE must be live, test, clerk-test or unset, not %q", authMode)
	}
	if test && stripeMode == "live" {
		return nil, errors.New("SCRY_STRIPE_MODE=live is refused with SCRY_MODE=test: a test build never charges real cards")
	}
	if !test && judgeMode == "test" {
		return nil, errors.New("SCRY_JUDGE_MODE=test is refused with SCRY_MODE=live: live grading is never stubbed")
	}
	if !test && aiMode == "test" {
		return nil, errors.New("SCRY_AI_MODE=test is refused with SCRY_MODE=live: live AI is never stubbed")
	}
	if aiMode == "live" && judgeMode == "test" {
		return nil, errors.New("SCRY_JUDGE_MODE=test contradicts SCRY_AI_MODE=live: choose one")
	}
	if test && authMode == "live" {
		return nil, errors.New("SCRY_AUTH_MODE=live is refused with SCRY_MODE=test: a test build never signs learners into the production Clerk instance")
	}
	if err := checkClerkKeys(test, authMode, env("CLERK_PUBLISHABLE_KEY", ""), env("CLERK_SECRET_KEY", "")); err != nil {
		return nil, err
	}
	// SCRY_SPEECH_ROUTE is checked in every mode, before anything opens:
	// a typo never boots, even where speech is the fake.
	if _, err := speech.ParseSpeechRoute(env("SCRY_SPEECH_ROUTE", "")); err != nil {
		return nil, err
	}
	st, err := store.Open(env("SCRY_DB", "data/scry.db"))
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			st.Close()
		}
	}()
	cfg := app.DefaultConfig()
	cfg.TestMode = test
	cfg.BuildSHA = resolveBuildSHA()
	cfg.BackupDir = env("BACKUP_DIR", "")
	if cfg.BackupDir != "" {
		cfg.BackupInterval = 6 * time.Hour
		if v := env("BACKUP_INTERVAL", ""); v != "" {
			if cfg.BackupInterval, err = time.ParseDuration(v); err != nil {
				return nil, fmt.Errorf("BACKUP_INTERVAL: %w", err)
			}
		}
		if v := env("BACKUP_KEEP", ""); v != "" {
			if cfg.BackupKeep, err = strconv.Atoi(v); err != nil {
				return nil, fmt.Errorf("BACKUP_KEEP: %w", err)
			}
		}
	}
	a := &app.App{Store: st, Log: log, Now: time.Now, Cfg: cfg}

	// Alerts: a webhook if one is configured, the log otherwise.
	var notifier alert.Notifier = alert.Log{Logger: log}
	if url := env("ALERT_WEBHOOK_URL", ""); url != "" && !test {
		notifier = alert.Webhook{URL: url, Logger: log}
	} else if !test {
		log.Warn("ALERT_WEBHOOK_URL is not set: alerts go to the log only")
	}
	a.Alert = &alert.Quiet{Next: notifier, Window: 5 * time.Minute}

	var missing []string
	need := func(key string) string {
		v := env(key, "")
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}
	pk, sk := env("CLERK_PUBLISHABLE_KEY", ""), env("CLERK_SECRET_KEY", "")
	stripeSK, stripePK, price, wh := env("STRIPE_SECRET_KEY", ""), env("STRIPE_PUBLISHABLE_KEY", ""), env("STRIPE_PRICE_ID", ""), env("STRIPE_WEBHOOK_SECRET", "")
	if test {
		a.Auth = &auth.Local{DB: st.DB()}
		if pk != "" && sk != "" {
			a.Auth = &auth.Clerk{PublishableKey: pk, SecretKey: sk}
		}
		// Test mode only: start the app's clock at a given instant (RFC 3339,
		// its zone kept) and let it run, to show month rollovers on a live
		// binary, e.g. SCRY_TEST_CLOCK=2026-12-01T01:50:00+02:00.
		if v := env("SCRY_TEST_CLOCK", ""); v != "" {
			t0, perr := time.Parse(time.RFC3339Nano, v)
			if perr != nil {
				return nil, fmt.Errorf("SCRY_TEST_CLOCK: %w", perr)
			}
			started := time.Now()
			a.Now = func() time.Time { return t0.Add(time.Since(started)) }
		}
		fake := billing.Fake{}
		// Test mode only: hold each fake top-up charge, to exercise a slow
		// processor (for example SCRY_TEST_CHARGE_DELAY=4s).
		if d := env("SCRY_TEST_CHARGE_DELAY", ""); d != "" {
			if fake.ChargeDelay, err = time.ParseDuration(d); err != nil {
				return nil, fmt.Errorf("SCRY_TEST_CHARGE_DELAY: %w", err)
			}
		}
		a.Pay = fake
		if stripeSK != "" && stripePK != "" && price != "" && wh != "" {
			if a.Pay, err = billing.NewStripeWithMode(false, stripeSK, stripePK, price, wh); err != nil {
				return nil, err
			}
		} else if stripeMode == "test" {
			return nil, errors.New("SCRY_STRIPE_MODE=test needs STRIPE_SECRET_KEY, STRIPE_PUBLISHABLE_KEY, STRIPE_PRICE_ID and STRIPE_WEBHOOK_SECRET")
		}
		model := &ai.Fake{Delay: 350 * time.Millisecond}
		// Test mode only: a scripted verdict sequence, handed out in turn
		// whatever the answer says, e.g. SCRY_TEST_JUDGMENTS=close,missed,right.
		for _, v := range strings.Split(env("SCRY_TEST_JUDGMENTS", ""), ",") {
			if v = strings.TrimSpace(v); v == "" {
				continue
			}
			if !srs.Verdict(v).Valid() {
				return nil, fmt.Errorf("SCRY_TEST_JUDGMENTS: %q is not right, close or missed", v)
			}
			model.Verdicts = append(model.Verdicts, srs.Verdict(v))
		}
		// Test mode only ([S113-1]): every submission walks the real grading
		// chain over fixture steps, with no network call. A scripted path
		// sequence forces it through each step in turn, whatever the answer
		// says, e.g. SCRY_TEST_JUDGE_PATHS=primary-unavailable,primary-none,all-fail.
		for _, p := range strings.Split(env("SCRY_TEST_JUDGE_PATHS", ""), ",") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			if !ai.IsFakeJudgePath(p) {
				return nil, fmt.Errorf("SCRY_TEST_JUDGE_PATHS: %q is not one of %s", p, strings.Join(ai.FakeJudgePaths, ", "))
			}
			model.Paths = append(model.Paths, p)
		}
		// Each judgment waits this long first, so "checking" and "still
		// checking" can be seen (for example SCRY_TEST_JUDGE_DELAY=3500ms).
		if d := env("SCRY_TEST_JUDGE_DELAY", ""); d != "" {
			if model.JudgeDelay, err = time.ParseDuration(d); err != nil || model.JudgeDelay < 0 || model.JudgeDelay > 40*time.Second {
				return nil, fmt.Errorf("SCRY_TEST_JUDGE_DELAY: %q is not a duration between 0 and 40s", d)
			}
		}
		model.Log = log
		a.Model = model
		// SCRY_TEST_EXPLAIN=fail or slow exercises the explanation fallback.
		a.Explainer = &ai.FakeExplainer{Mode: env("SCRY_TEST_EXPLAIN", "")}
		// Bridge cards in test mode are always a deterministic fixture with
		// no network call, whatever SCRY_AI_MODE or SCRY_JUDGE_MODE says
		// (Slice 75); SCRY_TEST_BRIDGES=fail exercises the error state.
		// The live overrides below never touch a.Bridger.
		a.Bridger = &ai.FakeBridger{Mode: env("SCRY_TEST_BRIDGES", "")}
		a.Speech = speech.Fake{Phrase: env("SCRY_TEST_SPEECH", "")}
		if aiMode == "live" {
			// Explicitly asked for: all four real AI adapters on a test build.
			// Without keys this stops; it never falls back to the fakes.
			judge, claude, ear, aiMissing, invalid := realAI(log)
			if len(aiMissing) > 0 || len(invalid) > 0 {
				return nil, bootProblems("SCRY_AI_MODE=live needs", aiMissing, "", invalid)
			}
			a.Model = ai.Split{JudgeProvider: judge, DraftProvider: claude}
			a.Explainer, a.Speech = claude, ear
			log.Warn("test mode is calling real AI providers (SCRY_AI_MODE=live)", "model", a.Model.Name(), "speech", ear.Name())
		} else if judgeMode == "live" {
			// Explicitly asked for: grade with real Jev, keep drafting fake.
			// Without a key this stops; it never falls back to the fake.
			router, typesafe := env("OPENROUTER_API_KEY", ""), env("TYPESAFE_API_KEY", "")
			judge, jerr := ai.NewJev(typesafe, router, log)
			if jerr != nil {
				return nil, errors.New("SCRY_JUDGE_MODE=live needs OPENROUTER_API_KEY or TYPESAFE_API_KEY")
			}
			// The same key check as realAI, on the key Jev will actually use:
			// OPENROUTER_API_KEY whenever it is set, else TYPESAFE_API_KEY.
			name, key := "OPENROUTER_API_KEY", router
			if router == "" {
				name, key = "TYPESAFE_API_KEY", typesafe
			}
			if kerr := safeerr.CheckAPIKey(name, key); kerr != nil {
				return nil, fmt.Errorf("SCRY_JUDGE_MODE=live: %w", kerr)
			}
			if judge.Model, err = judgeModel(router); err != nil {
				return nil, fmt.Errorf("SCRY_JUDGE_MODE=live: %w", err)
			}
			// The escalation LLM needs a generative client: OpenRouter's when
			// it carries grading, else the direct Anthropic key if there is one.
			var llm *ai.Claude
			if router != "" {
				llm, _ = ai.NewClaudeWithRouter("", router, "", "")
			} else if ant := env("ANTHROPIC_API_KEY", ""); ant != "" {
				if kerr := safeerr.CheckAPIKey("ANTHROPIC_API_KEY", ant); kerr != nil {
					return nil, fmt.Errorf("SCRY_JUDGE_MODE=live: %w", kerr)
				}
				llm, _ = ai.NewClaudeWithRouter(ant, "", env("SCRY_DRAFT_MODEL", ""), "")
			} else {
				log.Warn("SCRY_JUDGE_MODE=live on TYPESAFE_API_KEY without ANTHROPIC_API_KEY: the grading chain has no escalation LLM")
			}
			chain, cerr := judgeChain(router, judge, llm, log)
			if cerr != nil {
				return nil, fmt.Errorf("SCRY_JUDGE_MODE=live: %w", cerr)
			}
			a.Model = ai.Split{JudgeProvider: chain, DraftProvider: model}
			log.Warn("test mode is grading with the real Jev grader (SCRY_JUDGE_MODE=live)", "grader", a.Model.Name())
		}
	} else {
		a.Auth = &auth.Clerk{PublishableKey: need("CLERK_PUBLISHABLE_KEY"), SecretKey: need("CLERK_SECRET_KEY")}
		judge, claude, ear, aiMissing, invalid := realAI(log)
		missing = append(missing, aiMissing...)
		a.Model = ai.Split{JudgeProvider: judge, DraftProvider: claude}
		// Bridge cards use the same drafting client (Slice 36's empirical
		// winner on OpenRouter) as Create.
		a.Explainer, a.Bridger, a.Speech = claude, claude, ear
		sk, pk, price, wh := need("STRIPE_SECRET_KEY"), need("STRIPE_PUBLISHABLE_KEY"), need("STRIPE_PRICE_ID"), need("STRIPE_WEBHOOK_SECRET")
		if len(missing) > 0 || len(invalid) > 0 {
			return nil, bootProblems("live mode needs these environment variables:", missing, " (or set SCRY_MODE=test)", invalid)
		}
		liveStripe := stripeMode != "test"
		if a.Pay, err = billing.NewStripeWithMode(liveStripe, sk, pk, price, wh); err != nil {
			return nil, err
		}
		if !liveStripe {
			log.Warn("live mode is billing with Stripe test-mode keys (SCRY_STRIPE_MODE=test): no real card is charged", "billing", a.Pay.Name())
		}
		if c, _ := a.Auth.(*auth.Clerk); c != nil && c.Mode() == "clerk-test" {
			log.Warn("live mode is signing learners in with a Clerk development instance (clerk-test keys)", "auth", c.Mode(), "auth_mode", authMode)
		}
	}
	rd := a.Readiness()
	log.Info("adapters", "mode", mode, "build_sha", rd.BuildSHA, "auth", rd.AuthMode, "billing", rd.BillingMode, "model", a.Model.Name(),
		"grading", rd.Providers["grading"], "drafting", rd.Providers["drafting"], "explainer", a.Explainer.Name(), "speech", a.Speech.Name(),
		"grading_model", rd.Models["grading"], "drafting_model", rd.Models["drafting"], "explainer_model", rd.Models["explainer"],
		"speech_model", rd.Models["speech"], "speech_route", rd.SpeechRoute,
		"grading_fallbacks", strings.Join(rd.GradingChain.Fallbacks, ","), "grading_escalation", rd.GradingChain.Escalation,
		"grading_escalation_provider", rd.GradingChain.EscalationProvider)
	ok = true
	return a, nil
}

// realAI wires the four real AI adapters with one precedence rule, the
// same as ai.NewJev's: when OPENROUTER_API_KEY is set it carries grading,
// drafting, explanations and speech, and no direct key is needed or read;
// only when it is unset does each capability fall back to its direct key
// (TYPESAFE_API_KEY for Jev, ANTHROPIC_API_KEY for drafting and
// explanations, OPENAI_API_KEY for speech). It reports every missing key
// and every invalid one (such as a JWT-shaped key), never a key's value.
//
// Grading is the ordered chain of [S113-1] (judgeChain): Jev first, its
// fallback decisions models, and the escalation LLM on the drafting
// client's route and credential.
func realAI(log *slog.Logger) (judge *ai.Chain, claude *ai.Claude, ear speech.Transcriber, missing, invalid []string) {
	router, typesafe := env("OPENROUTER_API_KEY", ""), env("TYPESAFE_API_KEY", "")
	anthropic, openai := env("ANTHROPIC_API_KEY", ""), env("OPENAI_API_KEY", "")
	check := func(name, key string) {
		if key != "" {
			if err := safeerr.CheckAPIKey(name, key); err != nil {
				invalid = append(invalid, err.Error())
			}
		}
	}
	if router != "" {
		check("OPENROUTER_API_KEY", router)
	} else {
		check("TYPESAFE_API_KEY", typesafe)
		check("ANTHROPIC_API_KEY", anthropic)
	}
	var err error
	jev, err := ai.NewJev(typesafe, router, log)
	if err != nil {
		missing = append(missing, "OPENROUTER_API_KEY or TYPESAFE_API_KEY")
	} else if jev.Model, err = judgeModel(router); err != nil {
		invalid = append(invalid, err.Error())
	}
	if router == "" && anthropic == "" {
		missing = append(missing, "OPENROUTER_API_KEY or ANTHROPIC_API_KEY")
	} else if claude, err = ai.NewClaudeWithRouter(anthropic, router, env("SCRY_DRAFT_MODEL", ""), env("SCRY_EXPLAIN_MODEL", "")); err != nil {
		invalid = append(invalid, err.Error())
	}
	if jev != nil {
		if judge, err = judgeChain(router, jev, claude, log); err != nil {
			invalid = append(invalid, err.Error())
		}
	}
	if router == "" && openai == "" {
		missing = append(missing, "OPENROUTER_API_KEY or OPENAI_API_KEY")
	} else if ear, err = speech.NewTranscriberWithRoute(router, openai, env("SCRY_SPEECH_MODEL", ""), env("SCRY_SPEECH_ROUTE", "")); err != nil {
		if !(router != "" && safeerr.LooksLikeJWT(router)) { // already reported once
			invalid = append(invalid, err.Error())
		}
	}
	return judge, claude, ear, missing, invalid
}

// judgeModel reads SCRY_JUDGE_MODEL, the Decisions model override ("" keeps
// the route's default, typesafe/jev-1.14 on OpenRouter). On OpenRouter it
// must be a vendor/model slug, as SCRY_DRAFT_MODEL, SCRY_EXPLAIN_MODEL and
// SCRY_SPEECH_MODEL must, so a direct-route alias left in the environment
// fails the boot instead of every judgment.
func judgeModel(router string) (string, error) {
	m := env("SCRY_JUDGE_MODEL", "")
	if m != "" && router != "" && !safeerr.IsRouterSlug(m) {
		return "", fmt.Errorf("SCRY_JUDGE_MODEL=%q is not an OpenRouter model slug (vendor/model) and OPENROUTER_API_KEY is set", m)
	}
	return m, nil
}

// judgeChain builds the ordered grading chain of [S113-1] around primary.
// On OpenRouter (router set) the fallback decisions models come from
// SCRY_JUDGE_FALLBACK_MODELS (unset: ai.DefaultJudgeFallbackModels; set
// but empty: none) and the escalation LLM from SCRY_JUDGE_ESCALATION_MODEL
// (unset or empty: ai.DefaultJudgeEscalationModel), each a vendor/model
// slug or the boot stops; they share the primary's key, log and route.
// Without OpenRouter the chain is the primary alone on Typesafe and the
// escalation is llm's drafting model on its own provider; the two
// OpenRouter-only settings are ignored with a warning. A nil llm means no
// escalation (the boot reports the missing drafting key elsewhere).
func judgeChain(router string, primary *ai.Jev, llm *ai.Claude, log *slog.Logger) (*ai.Chain, error) {
	chain := &ai.Chain{Decisions: []ai.GradingStep{primary}, Log: log}
	fallbacks, fallbacksSet := os.LookupEnv("SCRY_JUDGE_FALLBACK_MODELS")
	escalation := env("SCRY_JUDGE_ESCALATION_MODEL", "")
	if router == "" {
		if strings.TrimSpace(fallbacks) != "" || escalation != "" {
			log.Warn("SCRY_JUDGE_FALLBACK_MODELS and SCRY_JUDGE_ESCALATION_MODEL are OpenRouter slugs and are ignored without OPENROUTER_API_KEY: the chain is Jev on Typesafe, escalating to the drafting model")
		}
		if llm != nil {
			chain.Escalation = &ai.LLMJudge{Client: llm}
		}
		return chain, nil
	}
	models := ai.DefaultJudgeFallbackModels()
	if fallbacksSet {
		models = nil
		for _, m := range strings.Split(fallbacks, ",") {
			if m = strings.TrimSpace(m); m == "" {
				continue
			}
			if !safeerr.IsRouterSlug(m) {
				return nil, fmt.Errorf("SCRY_JUDGE_FALLBACK_MODELS has %q, which is not an OpenRouter model slug (vendor/model), and OPENROUTER_API_KEY is set", m)
			}
			models = append(models, m)
		}
	}
	for _, m := range models {
		chain.Decisions = append(chain.Decisions, &ai.Jev{Model: m, APIKey: primary.APIKey, OpenRouter: true, BaseURL: primary.BaseURL, HTTP: primary.HTTP, Log: primary.Log})
	}
	if escalation == "" {
		escalation = ai.DefaultJudgeEscalationModel
	}
	if !safeerr.IsRouterSlug(escalation) {
		return nil, fmt.Errorf("SCRY_JUDGE_ESCALATION_MODEL=%q is not an OpenRouter model slug (vendor/model) and OPENROUTER_API_KEY is set", escalation)
	}
	if llm != nil {
		chain.Escalation = &ai.LLMJudge{Client: llm, Model: escalation}
	}
	return chain, nil
}

// bootProblems is one loud boot error naming every missing variable and
// every invalid one. It never contains a value.
func bootProblems(lead string, missing []string, tail string, invalid []string) error {
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, lead+" "+strings.Join(missing, ", ")+tail)
	}
	parts = append(parts, invalid...)
	return errors.New(strings.Join(parts, "; "))
}

// checkClerkKeys pins Clerk to one instance kind. A publishable and secret
// key from different kinds (one live, one test) never boot. SCRY_MODE=test
// never accepts a live Clerk key (pk_live_/sk_live_), whatever
// SCRY_AUTH_MODE says, just as it never accepts live Stripe keys. With
// SCRY_AUTH_MODE=live both must be live keys; with test or clerk-test,
// each key set must be a test key (pk_test_/sk_test_), and in test mode
// both must be present. Errors name the variable, never its value.
func checkClerkKeys(test bool, authMode, pk, sk string) error {
	kind := func(key, live, testp string) string {
		switch {
		case strings.HasPrefix(key, live):
			return "live"
		case strings.HasPrefix(key, testp):
			return "test"
		}
		return ""
	}
	pkKind, skKind := kind(pk, "pk_live_", "pk_test_"), kind(sk, "sk_live_", "sk_test_")
	keys := []struct{ name, key, kind string }{{"CLERK_PUBLISHABLE_KEY", pk, pkKind}, {"CLERK_SECRET_KEY", sk, skKind}}
	if test {
		for _, k := range keys {
			if k.kind == "live" {
				return fmt.Errorf("SCRY_MODE=test refuses live Clerk keys: %s is a live key (a test build never signs learners into the production Clerk instance)", k.name)
			}
		}
	}
	switch authMode {
	case "live":
		for _, k := range keys {
			if k.key != "" && k.kind != "live" {
				return fmt.Errorf("SCRY_AUTH_MODE=live needs live Clerk keys, but %s is not a live key", k.name)
			}
		}
	case "test", "clerk-test":
		if test && (pk == "" || sk == "") {
			return fmt.Errorf("SCRY_AUTH_MODE=%s needs CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY", authMode)
		}
		for _, k := range keys {
			switch {
			case k.kind == "live":
				return fmt.Errorf("SCRY_AUTH_MODE=%s needs Clerk test keys, but %s is a live key", authMode, k.name)
			case k.key != "" && k.kind != "test":
				return fmt.Errorf("SCRY_AUTH_MODE=%s needs Clerk test keys, but %s is not a test key", authMode, k.name)
			}
		}
	}
	if pkKind != "" && skKind != "" && pkKind != skKind {
		return errors.New("CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY belong to different Clerk instances (one live, one test)")
	}
	return nil
}

// Clerk owns its accounts: skip the entire local demo fixture with Clerk.
func shouldSeedDemo(a *app.App) bool {
	_, local := a.Auth.(*auth.Local)
	return a.Cfg.TestMode && local && env("SCRY_SEED_DEMO", "") == "1"
}

func serve(log *slog.Logger) error {
	a, err := build(log)
	if err != nil {
		return err
	}
	defer a.Store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.Store.Integrity(ctx); err != nil {
		a.Alert.Notify(ctx, "Database failed its integrity check", err.Error())
		return err
	}
	if err := a.Store.FailStaleDrafts(ctx); err != nil {
		return err
	}
	// Nothing is in flight before serving starts, so every reservation left
	// is an orphan from an unclean stop and would hold allowance forever.
	released, err := a.Store.FailStaleReservations(ctx)
	if err != nil {
		return err
	}
	if released > 0 {
		log.Warn("released stale allowance reservations left by an unclean stop", "count", released)
	}
	seeded := false
	if shouldSeedDemo(a) {
		if err := a.SeedDemo(ctx); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
		seeded = true
	}

	srv := web.New(a, log)
	srv.DemoHint = seeded
	srv.BaseURL = env("SCRY_BASE_URL", "")
	srv.Secure = strings.HasPrefix(srv.BaseURL, "https://")
	httpSrv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go a.Run(ctx)
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	log.Info("scry is listening", "addr", httpSrv.Addr, "test_mode", a.Cfg.TestMode, "database", a.Store.Path())

	select {
	case err := <-errc:
		a.Alert.Notify(context.Background(), "Scry stopped serving", err.Error())
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = httpSrv.Shutdown(shutdown)
	a.WaitDrafts()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// healthcheck asks the running server how it is. The container's
// HEALTHCHECK runs it. The deployment supervisor must restart unhealthy
// containers; Docker restart policies alone only restart exited processes.
func healthcheck() error {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + env("PORT", "8080") + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

func backup(log *slog.Logger) error {
	dir := env("BACKUP_DIR", "backups")
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}
	st, err := store.Open(env("SCRY_DB", "data/scry.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	path, err := st.Snapshot(context.Background(), dir, time.Now())
	if err != nil {
		return err
	}
	log.Info("backup written", "path", path)
	return nil
}

func restoreUser(log *slog.Logger) error {
	if len(os.Args) != 4 {
		return errors.New("usage: scry restore-user SNAPSHOT.db EMAIL")
	}
	st, err := store.Open(env("SCRY_DB", "data/scry.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.RestoreUser(context.Background(), os.Args[2], os.Args[3])
	if err != nil {
		return err
	}
	log.Info("learner restored from snapshot", "email", os.Args[3], "rows", n, "snapshot", os.Args[2])
	return nil
}
