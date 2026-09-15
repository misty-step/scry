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
		fmt.Println("Scry: a private learning application\n\nCommands:\n  serve    Serve the application and durable background work\n  check    Check integrity read-only; --allow-migration accepts supported upgrade sources without migrating\n  backup   Create a consistent snapshot and verify configured remote storage\n  restore  Restore into a new database with uncertain jobs paused\n  export   Export personal learning data without credentials\n  seed-fixture  Create an authored synthetic knowledge bundle in an unused database only\n  version  Print the source revision\n\nLocal use: scry serve --dev --db ./data/scry.sqlite\nProduction configuration: deploy/scry.env.example")
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
	allowMigration := fs.Bool("allow-migration", false, "accept a supported source schema read-only; does not migrate (candidate activation only)")
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result := recovery.SchemaCompatibility{
		Integrity: "ok", Accepted: true, Compatible: true,
		SourceSchema: store.SchemaVersion, TargetSchema: store.SchemaVersion, ReadOnly: true,
	}
	if *allowMigration {
		var err error
		result, err = recovery.CheckForMigration(ctx, *dbPath)
		if err != nil {
			return err
		}
	} else if err := recovery.Check(ctx, *dbPath); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		recovery.SchemaCompatibility
		Revision string `json:"revision"`
	}{result, revision})
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
	destination, err := filepath.Abs(*dbPath)
	if err != nil {
		return err
	}
	if err = unusedFixturePath(destination); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	// Work in a new private directory, not in the requested database. Even a
	// concurrent creator cannot make this command claim their queued work.
	work, err := os.MkdirTemp(filepath.Dir(destination), ".scry-fixture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	db, err := store.Open(filepath.Join(work, "authored.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	src, err := db.Capture(ctx, "Synthetic authored DNS and TLS learning fixture; link-only public reference https://www.rfc-editor.org/rfc/rfc1035.html (not retrieved)", "authored-fixture-capture-v2")
	if err != nil {
		return err
	}
	claim, err := db.ClaimJob(ctx, time.Minute, 0, 0)
	if err != nil {
		return fmt.Errorf("claim authored fixture: %w", err)
	}
	if claim == nil || claim.SourceID != src.ID {
		return errors.New("authored fixture did not claim its own isolated capture")
	}
	// A local authored bundle has exactly zero provider spend. Nil would mean
	// unknown external usage, and a positive value would fabricate paid work.
	cost := int64(0)
	if err = db.CompleteJob(ctx, claim.ID, claim.LeaseToken, authoredFixture(), &cost); err != nil {
		return err
	}
	goal, err := db.Goal(ctx, src.GoalID)
	if err != nil {
		return err
	}
	// Ordinary goal reads intentionally return material metadata. This isolated
	// authored CLI fixture is a deliberate full-content synthetic smoke oracle.
	for i := range goal.Materials {
		goal.Materials[i], err = db.Material(ctx, goal.Materials[i].ID)
		if err != nil {
			return fmt.Errorf("load authored fixture material: %w", err)
		}
	}
	prepared := filepath.Join(work, "complete.sqlite")
	if err = db.Backup(ctx, prepared); err != nil {
		return err
	}
	if err = db.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = unusedFixturePath(destination); err != nil {
		return err
	}
	if err = os.Link(prepared, destination); err != nil {
		return fmt.Errorf("publish synthetic fixture without replacing existing data: %w", err)
	}
	parent, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return err
	}
	if err = errors.Join(parent.Sync(), parent.Close()); err != nil {
		return fmt.Errorf("fixture is present but directory durability could not be confirmed: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"source": src.ID, "goal": goal, "model": "authored-test-fixture",
		"synthetic": true, "provider_cost_micros": cost,
	})
}

func unusedFixturePath(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return errors.New("seed-fixture requires an unused database and all SQLite sidecars; existing data is never modified")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func authoredFixture() store.GenerationResult {
	return store.GenerationResult{
		Model: "authored-test-fixture", PromptVersion: "fixture-v2",
		Coverage: store.CoverageReport{Kind: "concepts", Missing: []string{"Linked RFC content was not retrieved; the authored DNS/TLS examples are not quotations or complete protocol coverage"}},
		Units: []store.GeneratedUnit{
			{Key: "dns", Statement: "A DNS A record maps a hostname to an IPv4 address.", Kind: "foundation"},
			{Key: "tls", Statement: "TLS encrypts HTTP traffic when using HTTPS.", Kind: "foundation"},
			{Key: "connection", Statement: "An HTTPS connection combines hostname resolution with TLS-protected HTTP.", Kind: "composition"},
		},
		Relations: []store.GeneratedRelation{
			{From: "dns", To: "connection", Kind: "prerequisite", Evidence: "Proposed relationship: understanding hostname resolution helps explain a hostname-based HTTPS connection."},
			{From: "dns", To: "connection", Kind: "composition", Evidence: "Proposed relationship: hostname resolution is one component of this simplified connection model."},
			{From: "tls", To: "connection", Kind: "composition", Evidence: "Proposed relationship: TLS protection is another component of this simplified connection model."},
			{From: "dns", To: "tls", Kind: "contrast", Evidence: "Proposed relationship: naming and transport encryption solve different problems."},
		},
		Materials: []store.GeneratedMaterial{
			{
				Key: "instruction", Kind: "explanation", Title: "Two different jobs: naming and encryption",
				Body:  "Authored synthetic instruction: DNS A maps a hostname to an IPv4 address. TLS protects HTTP traffic. Knowing the destination address does not itself encrypt a request.",
				Basis: "topic", EstimatedSeconds: 30,
				Links: []store.GeneratedLink{{UnitKey: "dns", Role: "teaches"}, {UnitKey: "tls", Role: "teaches"}, {UnitKey: "connection", Role: "mentions"}},
			},
			{
				Key: "worked", Kind: "worked_example", Title: "Resolve a name, then protect the request",
				Body:  "Authored synthetic worked example: a browser resolves a hostname through a DNS A record to obtain an IPv4 address, then negotiates TLS before sending protected HTTP. If resolution succeeds but TLS fails, the name lookup did not establish a secure connection.",
				Basis: "topic", EstimatedSeconds: 30,
				Links: []store.GeneratedLink{{UnitKey: "connection", Role: "teaches"}, {UnitKey: "dns", Role: "assumes"}, {UnitKey: "tls", Role: "teaches"}},
			},
			{
				Key: "diagram", Kind: "diagram", Title: "A simplified HTTPS connection",
				Body:  "Authored synthetic diagram: Hostname leads through DNS A lookup to an IPv4 address. Connect and negotiate TLS then leads to TLS-protected HTTP. Naming and transport protection are distinct steps.",
				Basis: "topic", EstimatedSeconds: 30,
				Diagram: &store.Diagram{
					Nodes:   []store.DiagramNode{{ID: "name", Label: "Hostname"}, {ID: "address", Label: "IPv4 address"}, {ID: "secure", Label: "TLS-protected HTTP"}},
					Edges:   []store.DiagramEdge{{From: "name", To: "address", Label: "DNS A lookup"}, {From: "address", To: "secure", Label: "Connect and negotiate TLS"}},
					Caption: "Authored conceptual model, not a captured network trace.",
				},
				Links: []store.GeneratedLink{{UnitKey: "dns", Role: "teaches"}, {UnitKey: "tls", Role: "teaches"}, {UnitKey: "connection", Role: "teaches"}},
			},
			{
				Key: "reference", Kind: "article", Title: "RFC 1035: DNS protocol reference (link only)",
				Basis: "reference", ReferenceURL: "https://www.rfc-editor.org/rfc/rfc1035.html", EstimatedSeconds: 30,
				Links: []store.GeneratedLink{{UnitKey: "dns", Role: "teaches"}, {UnitKey: "connection", Role: "mentions"}},
			},
		},
		Quizzes: []store.GeneratedQuiz{
			{
				Key: "foundation-one", Level: "foundation", Kind: "recall",
				Prompt: "Name the DNS record for an IPv4 address, then the protocol that protects HTTP.",
				Answer: "A; TLS", Variants: []string{"A and TLS"},
				Explanation: "A records return IPv4 addresses; TLS protects HTTP in HTTPS.", Basis: "topic", EstimatedSeconds: 20,
				Links: []store.GeneratedLink{{UnitKey: "dns", Role: "assesses"}, {UnitKey: "tls", Role: "assesses"}, {UnitKey: "connection", Role: "mentions"}},
			},
			{
				Key: "foundation-two", Level: "foundation", Kind: "recall",
				Prompt: "Complete both gaps: DNS A returns an ___ address, while HTTPS protects HTTP with ___.",
				Answer: "IPv4; TLS", Variants: []string{"IPv4 and TLS"},
				Explanation: "The two separate facts are IPv4 address mapping and TLS encryption.", Basis: "topic", EstimatedSeconds: 20,
				Links: []store.GeneratedLink{{UnitKey: "dns", Role: "assesses"}, {UnitKey: "tls", Role: "assesses"}, {UnitKey: "connection", Role: "assumes"}},
			},
			{
				Key: "target", Level: "target", Kind: "choice",
				Prompt:      "Which sequence separates naming from transport protection in a simplified HTTPS connection?",
				Answer:      "Resolve the hostname, then negotiate TLS for HTTP",
				Choices:     []string{"Resolve the hostname, then negotiate TLS for HTTP", "Use DNS A to encrypt HTTP", "Use TLS to allocate an IPv4 address", "Send protected HTTP before choosing a destination"},
				Explanation: "DNS resolves the destination; TLS protects transport. Neither substitutes for the other.", Basis: "topic", EstimatedSeconds: 20,
				Links: []store.GeneratedLink{{UnitKey: "connection", Role: "assesses"}, {UnitKey: "dns", Role: "assumes"}, {UnitKey: "tls", Role: "assumes"}},
			},
		},
		Suggestions: []store.GeneratedSuggestion{
			{Key: "advance", Kind: "advance", Title: "Combine the foundations", Reason: "Opt into the authored integrated connection question after checking the two separate facts.", UnitKeys: []string{"connection"}, MaterialKeys: []string{"target"}},
			{Key: "lateral", Kind: "lateral", Title: "Consult the DNS protocol reference", Reason: "Optionally open the supplied public RFC link; its content has not been fetched or assessed.", UnitKeys: []string{"dns"}, MaterialKeys: []string{"reference"}},
		},
	}
}
