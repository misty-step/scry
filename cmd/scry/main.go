// Command scry runs Scry: one service, server-rendered, on one SQLite file.
//
//	scry                         serve (the default)
//	scry healthcheck             exit 0 if the running server is healthy
//	scry backup [dir]            write a snapshot of the database
//	scry restore-user SNAP EMAIL restore one learner's data from a snapshot
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
	"scry/internal/speech"
	"scry/internal/srs"
	"scry/internal/store"
	"scry/internal/web"
)

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
func build(log *slog.Logger) (*app.App, error) {
	mode := env("SCRY_MODE", "live")
	if mode != "test" && mode != "live" {
		return nil, fmt.Errorf("SCRY_MODE must be test or live, not %q", mode)
	}
	test := mode == "test"
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
			if a.Pay, err = billing.NewStripe(stripeSK, stripePK, price, wh); err != nil {
				return nil, err
			}
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
		a.Model = model
		// SCRY_TEST_EXPLAIN=fail or slow exercises the explanation fallback.
		a.Explainer = &ai.FakeExplainer{Mode: env("SCRY_TEST_EXPLAIN", "")}
		a.Speech = speech.Fake{Phrase: env("SCRY_TEST_SPEECH", "")}
	} else {
		a.Auth = &auth.Clerk{PublishableKey: need("CLERK_PUBLISHABLE_KEY"), SecretKey: need("CLERK_SECRET_KEY")}
		judge, jerr := ai.NewJev(env("TYPESAFE_API_KEY", ""), env("OPENROUTER_API_KEY", ""), log)
		if jerr != nil {
			missing = append(missing, "OPENROUTER_API_KEY or TYPESAFE_API_KEY")
		}
		if judge != nil {
			judge.Model = env("SCRY_JUDGE_MODEL", "")
		}
		claude := ai.NewClaude(need("ANTHROPIC_API_KEY"), env("SCRY_DRAFT_MODEL", ""))
		claude.ExplainModel = env("SCRY_EXPLAIN_MODEL", "")
		a.Model = ai.Split{JudgeProvider: judge, DraftProvider: claude}
		a.Explainer = claude
		a.Speech = speech.OpenAI{APIKey: need("OPENAI_API_KEY"), Model: env("SCRY_SPEECH_MODEL", "")}
		sk, pk, price, wh := need("STRIPE_SECRET_KEY"), need("STRIPE_PUBLISHABLE_KEY"), need("STRIPE_PRICE_ID"), need("STRIPE_WEBHOOK_SECRET")
		if len(missing) > 0 {
			return nil, fmt.Errorf("live mode needs these environment variables: %s (or set SCRY_MODE=test)", strings.Join(missing, ", "))
		}
		if a.Pay, err = billing.NewStripe(sk, pk, price, wh); err != nil {
			return nil, err
		}
	}
	log.Info("adapters", "mode", mode, "auth", a.Auth.Name(), "billing", a.Pay.Name(), "model", a.Model.Name(), "explainer", a.Explainer.Name(), "speech", a.Speech.Name())
	ok = true
	return a, nil
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
	seeded := false
	if shouldSeedDemo(a) {
		if err := a.SeedDemo(ctx); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
		seeded = true
	}

	srv := web.New(a, log)
	srv.DemoHint = seeded
	srv.Secure = strings.HasPrefix(env("SCRY_BASE_URL", ""), "https://")
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
