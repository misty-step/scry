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
	"github.com/misty-step/scry/internal/semantic"
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
	budget, err := integerEnv("SCRY_GENERATION_DAILY_BUDGET_MICROS", 3_500_000)
	if err != nil {
		return err
	}
	reservation, err := integerEnv("SCRY_GENERATION_RESERVATION_MICROS", 500_000)
	if err != nil {
		return err
	}
	semanticReservation, err := integerEnv("SCRY_SEMANTIC_RESERVATION_MICROS", 2_000)
	if err != nil {
		return err
	}
	if semanticReservation < 0 || semanticReservation > budget {
		return errors.New("SCRY_SEMANTIC_RESERVATION_MICROS must be nonnegative and no larger than SCRY_GENERATION_DAILY_BUDGET_MICROS")
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
	semanticModel := env("SCRY_SEMANTIC_MODEL", semantic.DefaultModel)
	semanticKey := os.Getenv("SCRY_SEMANTIC_API_KEY")
	if semanticKey == "" {
		semanticKey = os.Getenv("SCRY_MODEL_API_KEY")
	}
	// The semantic endpoint receives the bearer key and private learner text;
	// refuse to start rather than send either over plaintext.
	if err := semantic.ValidateEndpoint(os.Getenv("SCRY_SEMANTIC_ENDPOINT")); err != nil {
		return fmt.Errorf("SCRY_SEMANTIC_ENDPOINT: %w", err)
	}
	if err := generation.ValidateExaEndpoint(env("SCRY_EXA_ENDPOINT", "https://api.exa.ai")); err != nil {
		return fmt.Errorf("SCRY_EXA_ENDPOINT: %w", err)
	}
	semanticClient := semantic.NewClient(semantic.Config{
		Endpoint: os.Getenv("SCRY_SEMANTIC_ENDPOINT"), APIKey: semanticKey, Model: semanticModel,
		HTTPClient: &http.Client{Timeout: 8 * time.Second, CheckRedirect: noRedirect},
	})
	semanticSpending := semantic.Spending{ReservationMicros: semanticReservation, DailyBudgetMicros: budget}
	var critic semantic.Client
	if strings.TrimSpace(os.Getenv("SCRY_SEMANTIC_ENDPOINT")) != "" {
		if semanticReservation == 0 {
			return errors.New("configured semantic assessments require a positive SCRY_SEMANTIC_RESERVATION_MICROS")
		}
		critic = semanticClient
	}
	if os.Getenv("SCRY_SEMANTIC_ENDPOINT") == "" {
		// No endpoint means no request can leave the process: reserve nothing.
		semanticSpending = semantic.Spending{}
	}
	handler, err := web.New(db, web.Config{
		Mode: mode, OwnerID: os.Getenv("SCRY_OWNER_ID"), Secret: secret,
		BaseURL: baseURL, TrustProxy: mode == "production", TrustedProxyIPs: trustedPeers,
		RedirectHosts: strings.Split(os.Getenv("SCRY_REDIRECT_HOSTS"), ","),
		Semantic:      semantic.NewAssessor(db, semanticClient, semanticModel, semanticSpending),
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
		ExaEndpoint:       env("SCRY_EXA_ENDPOINT", "https://api.exa.ai"),
		ExaAPIKey:         os.Getenv("SCRY_EXA_API_KEY"),
		DailyBudgetMicros: budget,
		ReservationMicros: reservation, PollInterval: time.Second,
		Critic: critic, CriticModel: semanticModel, CriticSpending: semanticSpending,
		HTTPClient: &http.Client{Timeout: 180 * time.Second, CheckRedirect: noRedirect},
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
	// Synthetic content never enters an existing database: refuse the file
	// and any SQLite sidecar before opening anything.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(*dbPath + suffix); err == nil {
			return fmt.Errorf("seed-fixture only creates a new database; %s already exists", *dbPath+suffix)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	src, err := db.Capture(ctx, store.CaptureInput{Text: "Synthetic DNS and TLS review fixture " + time.Now().UTC().Format(time.RFC3339Nano), Mode: "topic"}, "seed-dns-source-"+stamp)
	if err != nil {
		return err
	}
	// Walk the real chain with authored content: research finds nothing (no
	// web call is made), the plan names two concepts, and the questions link
	// to them. The fixture never contacts a provider.
	claim := func(kind string) (*store.Job, error) {
		j, err := db.ClaimJob(ctx, time.Minute, 100, 1_000_000)
		if err != nil || j == nil || j.Kind != kind {
			return nil, fmt.Errorf("claim authored fixture %s: %v %w", kind, j, err)
		}
		return j, nil
	}
	publish := func(j *store.Job, result store.GenerationResult, cost int64) error {
		result.Model, result.PromptVersion = "authored-test-fixture", "fixture-v5"
		return db.CompleteJob(ctx, j.ID, j.LeaseToken, result, &cost)
	}
	research, err := claim("research")
	if err == nil {
		err = publish(research, store.GenerationResult{Note: "Synthetic fixture: no web research."}, 0)
	}
	if err != nil {
		return err
	}
	note := func(title, body string) *store.NoteContent {
		return &store.NoteContent{Title: title, Body: body, Basis: "topic"}
	}
	plan, err := claim("plan")
	if err == nil {
		err = publish(plan, store.GenerationResult{Plan: &store.PlanContent{Goal: "Synthetic DNS and TLS review", Concepts: []store.PlannedConcept{
			{Key: "dns", Name: "DNS address records", Summary: "An A record maps a hostname to an IPv4 address.",
				Note: note("DNS address records", "A DNS A record answers one question: which IPv4 address serves this hostname. AAAA records do the same for IPv6.")},
			{Key: "tls", Name: "TLS certificate trust", Summary: "HTTPS runs over TLS, which checks the server's certificate.",
				Note: note("TLS certificate trust", "HTTPS wraps HTTP in TLS. The client trusts the server only when its certificate names the intended host and chains to a trusted issuer.")},
		}}}, 40)
	}
	if err != nil {
		return err
	}
	questions, err := claim("questions")
	if err != nil {
		return err
	}
	jc, err := db.JobContext(ctx, questions.ID)
	if err != nil || len(jc.Concepts) != 2 {
		return fmt.Errorf("authored fixture concepts: %+v %w", jc.Concepts, err)
	}
	dns, tls := jc.Concepts[0].ID, jc.Concepts[1].ID
	err = publish(questions, store.GenerationResult{
		Quizzes: []store.GeneratedQuiz{
			{
				Kind: "choice", Level: "recognize", Concept: dns, Prompt: "What type of address does a DNS A record map a hostname to?",
				Answer: "IPv4 address", Choices: []string{"Text value", "IPv4 address", "IPv6 address", "Mail server address"},
				Explanation: "A DNS A record maps a hostname to an IPv4 address.", Basis: "topic",
			},
			{
				Kind: "recall", Level: "recall", Concept: tls, Prompt: "What protocol does HTTPS use to encrypt HTTP?",
				Answer: "TLS", Variants: []string{"Transport Layer Security"},
				Explanation: "HTTPS wraps HTTP in TLS.", Basis: "topic",
			},
			{
				Kind: "recall", Level: "explain", Grading: "semantic", Concept: tls, Prompt: "What two checks let a TLS client trust a server certificate?",
				Answer:      "It identifies the intended host and chains to a trusted issuer.",
				Explanation: "Certificate validation checks both hostname identity and a chain to a trusted issuer.", Basis: "topic",
				Rubric: &store.Rubric{
					Required: []store.RubricIdea{
						{Text: "The certificate identifies the intended host", Cue: "Think about the server name the client requested."},
						{Text: "The certificate chain leads to a trusted issuer"},
					},
					Contradictions: []store.RubricClaim{{Text: "Encryption alone proves the server identity", Feedback: "Encryption protects the connection, but identity still depends on hostname and chain validation."}},
				},
			},
		},
	}, 70)
	if err != nil {
		return err
	}
	// The synthetic learner reads each intro the stream offers, so the stream
	// opens on a question awaiting an answer (the release smoke's contract).
	for i := 0; i < 4; i++ {
		state, err := db.Review(ctx)
		if err != nil {
			return err
		}
		if state.Current != nil {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"source": src.ID, "model": "authored-test-fixture"})
		}
		if state.Intro == nil {
			break
		}
		if _, err = db.AcknowledgeIntro(ctx, state.Intro.Concept.ID, "seed-intro-"+strconv.Itoa(i)+"-"+stamp, false); err != nil {
			return err
		}
	}
	return errors.New("authored fixture did not reach a question")
}
