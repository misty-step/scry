package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/misty-step/scry/internal/generation"
	"github.com/misty-step/scry/internal/recovery"
	"github.com/misty-step/scry/internal/store"
	"github.com/misty-step/scry/internal/web"
)

var revision = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scry:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("choose serve, check, backup, restore, export, seed-fixture, or version; use --help for a command")
	}
	switch args[0] {
	case "version":
		fmt.Println("scry", revision)
		return nil
	case "serve":
		return serve(args[1:])
	case "check":
		return check(args[1:])
	case "backup":
		return backup(args[1:])
	case "restore":
		return restore(args[1:])
	case "export":
		return export(args[1:])
	case "seed-fixture":
		return seedFixture(args[1:])
	case "help", "--help", "-h":
		fmt.Println("Scry: a private learning application\n\nCommands:\n  serve    Serve the application and durable background work\n  check    Check database integrity and schema without changing it\n  backup   Create a consistent snapshot and verify configured remote storage\n  restore  Restore into a new database with uncertain jobs paused\n  export   Export personal learning data without credentials\n  seed-fixture  Publish labeled authored quizzes for local review proof\n  version  Print the source revision\n\nLocal use: scry serve --dev --db ./data/scry.sqlite\nProduction configuration: deploy/scry.env.example")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func integerEnv(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("%s must be a nonnegative integer", name)
	}
	return number, nil
}

func databaseFlag(fs *flag.FlagSet) *string {
	return fs.String("db", env("SCRY_DB", "data/scry.sqlite"), "SQLite database path")
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

func recoveryConfig(dbPath string) (recovery.Config, error) {
	interval, err := durationEnv("SCRY_BACKUP_INTERVAL", 24*time.Hour)
	if err != nil {
		return recovery.Config{}, err
	}
	keep, err := integerEnv("SCRY_BACKUP_KEEP", 30)
	if err != nil || keep < 1 || keep > 3650 {
		return recovery.Config{}, errors.New("SCRY_BACKUP_KEEP must be between 1 and 3650")
	}
	return recovery.Config{
		Dir:         env("SCRY_BACKUP_DIR", filepath.Join(filepath.Dir(dbPath), "backups")),
		RemoteURL:   os.Getenv("SCRY_BACKUP_REMOTE_URL"),
		RemoteToken: os.Getenv("SCRY_BACKUP_REMOTE_TOKEN"),
		Interval:    interval,
		Keep:        int(keep),
		HTTPClient:  &http.Client{Timeout: 90 * time.Second, CheckRedirect: noRedirect},
	}, nil
}

func noRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("redirects are not permitted for authenticated external requests")
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := databaseFlag(fs)
	address := fs.String("addr", env("SCRY_ADDR", "127.0.0.1:8080"), "HTTP listener")
	development := fs.Bool("dev", false, "enable explicit loopback-only development identity")
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	mode := env("SCRY_MODE", "production")
	if *development {
		mode = "development"
	}
	if mode != "development" && mode != "production" {
		return errors.New("SCRY_MODE must be development or production")
	}
	host, _, err := net.SplitHostPort(*address)
	if err != nil {
		return fmt.Errorf("invalid listener: %w", err)
	}
	if mode == "development" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("development identity requires an explicit loopback IP listener")
		}
	}
	secret := os.Getenv("SCRY_SECRET")
	if mode == "development" && secret == "" {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return err
		}
		secret = hex.EncodeToString(bytes[:])
	}
	baseURL := os.Getenv("SCRY_BASE_URL")
	if mode == "development" && baseURL == "" {
		baseURL = "http://" + *address
	}
	var trustedPeers []string
	for _, value := range strings.Split(os.Getenv("SCRY_TRUSTED_PROXY_IPS"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			if net.ParseIP(value) == nil {
				return errors.New("SCRY_TRUSTED_PROXY_IPS must contain exact IP addresses")
			}
			trustedPeers = append(trustedPeers, value)
		}
	}
	budget, err := integerEnv("SCRY_GENERATION_DAILY_BUDGET_MICROS", 1_000_000)
	if err != nil {
		return err
	}
	reservation, err := integerEnv("SCRY_GENERATION_RESERVATION_MICROS", 200_000)
	if err != nil {
		return err
	}
	backupConfig, err := recoveryConfig(*dbPath)
	if err != nil {
		return err
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	handler, err := web.New(db, web.Config{
		Mode: mode, OwnerID: os.Getenv("SCRY_OWNER_ID"), Secret: secret,
		BaseURL: baseURL, TrustProxy: mode == "production", TrustedProxyIPs: trustedPeers,
		RedirectHosts: strings.Split(os.Getenv("SCRY_REDIRECT_HOSTS"), ","),
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10,
	}
	worker := generation.New(db, generation.Config{
		Endpoint: os.Getenv("SCRY_MODEL_ENDPOINT"), APIKey: os.Getenv("SCRY_MODEL_API_KEY"),
		Model: os.Getenv("SCRY_MODEL"), Provider: os.Getenv("SCRY_MODEL_PROVIDER"),
		DailyBudgetMicros: budget,
		ReservationMicros: reservation, PollInterval: time.Second,
		HTTPClient: &http.Client{Timeout: 60 * time.Second, CheckRedirect: noRedirect},
	})
	backups := recovery.New(db, backupConfig)
	var background sync.WaitGroup
	for _, task := range []struct {
		name string
		run  func(context.Context) error
	}{{"generation", worker.Run}, {"recovery", backups.Run}} {
		background.Add(1)
		go func() {
			defer background.Done()
			if err := task.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("background work stopped", "component", task.name, "error", err)
			}
		}()
	}
	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Serve(listener) }()
	slog.Info("Scry ready", "address", listener.Addr().String(), "mode", mode, "revision", revision)
	var serveError error
	select {
	case <-ctx.Done():
	case serveError = <-serverResult:
		cancel()
	}
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	shutdownError := server.Shutdown(shutdownContext)
	if shutdownError != nil {
		_ = server.Close()
	}
	cancel()
	background.Wait()
	if serveError != nil && !errors.Is(serveError, http.ErrServerClosed) {
		return serveError
	}
	return shutdownError
}

func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	dbPath := databaseFlag(fs)
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := recovery.Check(ctx, *dbPath); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"integrity": "ok", "compatible": true, "revision": revision})
}

func backup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dbPath := databaseFlag(fs)
	requireRemote := fs.Bool("require-remote", false, "fail unless independent remote checksum readback succeeds")
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	cfg, err := recoveryConfig(*dbPath)
	if err != nil {
		return err
	}
	if *requireRemote && cfg.RemoteURL == "" {
		return errors.New("SCRY_BACKUP_REMOTE_URL is required for an independent backup")
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	record, err := recovery.New(db, cfg).Backup(ctx)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		return err
	}
	if *requireRemote && !record.Remote {
		return errors.New("backup was not independently verified remotely")
	}
	return nil
}

func restore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	snapshot := fs.String("snapshot", "", "completed recovery snapshot")
	destination := fs.String("destination", "", "new unused SQLite path")
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *snapshot == "" || *destination == "" {
		return errors.New("restore requires --snapshot and --destination; an existing destination is never overwritten")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := recovery.Restore(ctx, *snapshot, *destination); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"restored": true, "jobs": "paused", "activated": false})
}

func export(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	dbPath := databaseFlag(fs)
	output := fs.String("output", "", "new export file (default stdout)")
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	data, err := db.Export(ctx)
	if err != nil {
		return err
	}
	if *output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func seedFixture(args []string) error {
	fs := flag.NewFlagSet("seed-fixture", flag.ContinueOnError)
	dbPath := databaseFlag(fs)
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	src, err := db.Capture(ctx, "Synthetic DNS and TLS review fixture "+time.Now().UTC().Format(time.RFC3339Nano), "seed-dns-source-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		return err
	}
	claim, err := db.ClaimJob(ctx, time.Minute, 100, 1_000_000)
	if err != nil || claim == nil {
		return fmt.Errorf("claim authored fixture: %v %w", claim, err)
	}
	cost := int64(70)
	err = db.CompleteJob(ctx, claim.ID, claim.LeaseToken, store.GenerationResult{
		Quizzes: []store.GeneratedQuiz{
			{
				Kind: "choice", Prompt: "What type of address does a DNS A record map a hostname to?",
				Answer: "IPv4 address", Choices: []string{"Text value", "IPv4 address", "IPv6 address", "Mail server address"},
				Explanation: "A DNS A record maps a hostname to an IPv4 address.", Basis: "topic",
			},
			{
				Kind: "recall", Prompt: "What protocol does HTTPS use to encrypt HTTP?",
				Answer: "TLS", Variants: []string{"Transport Layer Security"},
				Explanation: "HTTPS wraps HTTP in TLS.", Basis: "topic",
			},
		},
		Model: "authored-test-fixture", PromptVersion: "fixture-v1",
	}, &cost)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"source": src.ID, "model": "authored-test-fixture"})
}
