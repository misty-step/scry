package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultModels are the live candidates per surface and speech track.
var DefaultModels = map[string][]string{
	"grading":   {"openai/gpt-6-luna-decisions", "google/gemini-decision-1", "typesafe/jev-1.14", "liquid/d1.2", "liquid/d1.2-ultra"},
	"explainer": {"anthropic/claude-sonnet-5.5", "anthropic/claude-haiku-5.5", "openai/gpt-6-luna", "google/gemini-4-hydrogen", "deepseek/deepseek-v4.1-flash-1017"},
	"drafting":  {"openai/gpt-6.2-sol", "google/gemini-4-hydrogen", "google/gemini-4-neon", "anthropic/claude-opus-5.5"},
	// Run after the main drafting matrix only if the remaining budget
	// covers a full pass 1.5 times over (estimated from the tokens the main
	// models actually used, at this model's catalog prices).
	"drafting-optional": {"anthropic/claude-sonnet-5.5"},
	// Track 4A: Scry's speech adapter (chat completions + input_audio).
	"speech-chat": {"google/gemini-2.5-flash", "google/gemini-4-hydrogen", "google/gemini-3.8-flash", "google/gemini-3.1-flash-lite"},
	// Track 4B: OpenRouter's dedicated STT route. The probe model runs on one clip first.
	"speech-stt":       {"x-ai/grok-stt-2.0", "deepgram/nova-3", "openai/whisper-large-v3-turbo", "openai/gpt-4o-mini-transcribe"},
	"speech-stt-probe": {"microsoft/mai-transcribe-2"},
	// Track 4C: audio straight into a Decisions model.
	"speech-audio-grade": {"google/gemini-decision-1"},
}

// Surfaces in the order "all" runs them: cheapest first, drafting last, so
// the spend cap can only cut the most expensive surface short.
var Surfaces = []string{"grading", "explainer", "speech", "drafting"}

// Config is one bench run.
type Config struct {
	Surfaces    []string
	OutDir      string
	EvidenceDir string // optional: flat copies with collector-friendly names
	MaxSpend    float64
	Limit       int                 // cases per surface; 0 means all
	Models      map[string][]string // overrides DefaultModels per key
	Key         string              // OPENROUTER_API_KEY; never written anywhere
	Base        string              // provider origin; https://openrouter.ai unless a test stand-in
	Seed        string              // blind-label seed
	Args        []string
	Log         io.Writer
	// STTProbeMaxUSD is the most the probe clip may cost before the probe
	// model runs on the remaining clips.
	STTProbeMaxUSD float64
	// SkipAccount skips the before/after account usage cross-check.
	SkipAccount bool
	// Append continues a run in OutDir: raw logs are appended, the summary,
	// blind sets and key are merged, and the spend cap covers the earlier
	// invocations' spend too.
	Append bool
	// CaseParallel runs this many cases at once (all models of each);
	// default 1. Only drafting uses it.
	CaseParallel int
}

// Runner runs the bench.
type Runner struct {
	cfg    Config
	data   *Datasets
	budget *Budget
	cat    Catalog

	mu       sync.Mutex
	files    map[string]*os.File
	observed map[string]float64 // surface/model -> highest exact cost seen
	notes    []string
	blind    []BlindSet
	key      BlindKey
	summary  map[string]any
	prior    *priorRun
	// outErr is the first raw-log open, encode, write or close failure.
	// Once set, no further paid call is dispatched and Run returns it.
	outErr error
	// produced marks the surfaces that dispatched at least one call in
	// this invocation; only those replace an earlier invocation's
	// results and blind artifacts under --append.
	produced map[string]bool
	// dispatched is surface -> model -> true for every model that made at
	// least one call in this invocation (the same models whose summary row
	// replaces an earlier one under --append, since their calls > 0).
	dispatched map[string]map[string]bool
}

// priorRun is what an earlier invocation in the same OutDir left.
type priorRun struct {
	Summary map[string]any
	Blind   []BlindSet
	Key     BlindKey
	Spend   BudgetState
}

func loadPrior(dir string) (*priorRun, error) {
	b, err := os.ReadFile(filepath.Join(dir, "eval-summary.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p := &priorRun{}
	if err := json.Unmarshal(b, &p.Summary); err != nil {
		return nil, fmt.Errorf("earlier summary unreadable: %w", err)
	}
	var sp struct{ Spend BudgetState }
	if err := json.Unmarshal(b, &sp); err != nil {
		return nil, err
	}
	p.Spend = sp.Spend
	var blind struct{ Sets []BlindSet }
	if b, err := os.ReadFile(filepath.Join(dir, "blind-samples.json")); err == nil {
		if err := json.Unmarshal(b, &blind); err != nil {
			return nil, err
		}
	}
	p.Blind = blind.Sets
	if b, err := os.ReadFile(filepath.Join(dir, "blind-key.json")); err == nil {
		if err := json.Unmarshal(b, &p.Key); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// checkPriorBlindKey refuses an --append whose earlier blind outputs could
// no longer be de-anonymized: if blind-samples.json in dir holds any blind
// output, blind-key.json must exist, parse, and map every (surface/case_id,
// model_label) of those outputs to a model slug. Merging over such a trail
// would keep or reshuffle outputs nobody can attribute, so it fails before
// anything is dispatched.
func checkPriorBlindKey(dir string) error {
	const fix = "restore blind-key.json from the earlier run, or use a fresh --out directory"
	samples := filepath.Join(dir, "blind-samples.json")
	b, err := os.ReadFile(samples)
	if errors.Is(err, os.ErrNotExist) {
		// A dangling symlink is an existing but unreadable artifact, not
		// an output directory that has no prior blind samples.
		if _, statErr := os.Lstat(samples); errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("--append: earlier blind-samples.json is unreadable (%v); fix it, or use a fresh --out directory; nothing was dispatched", err)
	}
	var blind struct{ Sets []BlindSet }
	if err := json.Unmarshal(b, &blind); err != nil {
		return fmt.Errorf("--append: earlier blind-samples.json is malformed (%v); fix it, or use a fresh --out directory; nothing was dispatched", err)
	}
	need := 0
	for _, s := range blind.Sets {
		need += len(s.Outputs)
	}
	if need == 0 {
		return nil
	}
	kb, err := os.ReadFile(filepath.Join(dir, "blind-key.json"))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("--append: earlier blind-samples.json has %d blind set(s) but blind-key.json is missing; %s; nothing was dispatched", len(blind.Sets), fix)
	}
	if err != nil {
		return fmt.Errorf("--append: earlier blind-key.json is unreadable (%v); %s; nothing was dispatched", err, fix)
	}
	var key BlindKey
	if err := json.Unmarshal(kb, &key); err != nil {
		return fmt.Errorf("--append: earlier blind-key.json is malformed (%v); %s; nothing was dispatched", err, fix)
	}
	var missing []string
	for _, s := range blind.Sets {
		id := s.Surface + "/" + s.CaseID
		for _, o := range s.Outputs {
			if key.Sets[id][o.ModelLabel] == "" {
				missing = append(missing, id+" "+o.ModelLabel)
			}
		}
	}
	if len(missing) > 0 {
		shown := missing
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return fmt.Errorf("--append: earlier blind-key.json lacks %d of %d blind label(s) in blind-samples.json (e.g. %s); %s; nothing was dispatched", len(missing), need, strings.Join(shown, ", "), fix)
	}
	return nil
}

const defaultBase = "https://openrouter.ai"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// NewRunner validates the config and loads the datasets.
func NewRunner(cfg Config) (*Runner, error) {
	if cfg.Key == "" {
		return nil, errors.New("OPENROUTER_API_KEY is not set; live runs need it (offline tests do not)")
	}
	if cfg.MaxSpend <= 0 || math.IsNaN(cfg.MaxSpend) {
		return nil, errors.New("--max-spend must be positive")
	}
	if cfg.Base == "" {
		cfg.Base = defaultBase
	}
	if cfg.Seed == "" {
		cfg.Seed = "scry-eval-v1"
	}
	if cfg.Log == nil {
		cfg.Log = os.Stderr
	}
	if cfg.STTProbeMaxUSD == 0 {
		cfg.STTProbeMaxUSD = 0.01
	}
	for _, s := range cfg.Surfaces {
		if !oneOf(s, Surfaces...) {
			return nil, fmt.Errorf("unknown surface %q", s)
		}
	}
	data, err := LoadDatasets()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}
	if cfg.CaseParallel < 1 {
		cfg.CaseParallel = 1
	}
	r := &Runner{cfg: cfg, data: data, budget: &Budget{Max: cfg.MaxSpend}, files: map[string]*os.File{}, observed: map[string]float64{}, produced: map[string]bool{}, dispatched: map[string]map[string]bool{},
		key:     BlindKey{Seed: cfg.Seed, Note: "Sealed provenance for blind-samples.json: surface/case -> blind label -> model slug. Do not open before rating.", Sets: map[string]map[string]string{}},
		summary: map[string]any{}}
	if cfg.Append {
		if err := checkPriorBlindKey(cfg.OutDir); err != nil {
			return nil, err
		}
		if r.prior, err = loadPrior(cfg.OutDir); err != nil {
			return nil, err
		}
		if r.prior != nil {
			if seed, _ := r.prior.Summary["blind_seed"].(string); seed != cfg.Seed {
				return nil, fmt.Errorf("--append: earlier run used blind seed %q, not %q", seed, cfg.Seed)
			}
			// The cap covers the earlier invocations too.
			ps := r.prior.Spend
			r.budget.spent, r.budget.exact, r.budget.calls, r.budget.unknown, r.budget.refused = ps.ChargedUSD, ps.ExactUSD, ps.Calls, ps.UnknownCostCalls, ps.RefusedForBudget
		}
	}
	return r, nil
}

func (r *Runner) models(key string) []string {
	if m, ok := r.cfg.Models[key]; ok {
		return m
	}
	return DefaultModels[key]
}

func limit[T any](rows []T, n int) []T {
	if n > 0 && n < len(rows) {
		return rows[:n]
	}
	return rows
}

func (r *Runner) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.cfg.Log, redact(fmt.Sprintf(format, args...), r.cfg.Key))
}

func (r *Runner) note(format string, args ...any) {
	s := redact(fmt.Sprintf(format, args...), r.cfg.Key)
	r.mu.Lock()
	r.notes = append(r.notes, s)
	r.mu.Unlock()
	r.logf("NOTE %s", s)
}

// ErrRecording is returned for calls not dispatched because the raw log
// can no longer be written: a paid call whose record would be lost is
// never started.
var ErrRecording = errors.New("raw log recording failed: call not dispatched")

// write appends one raw record to <surface>-raw.jsonl. The first failure
// (encode, open or write) is kept in outErr, which stops dispatch.
func (r *Runner) write(surface string, rec any) {
	b, err := json.Marshal(rec)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.failOutputLocked(fmt.Errorf("encode %s record: %w", surface, err))
		return
	}
	b = []byte(redact(string(b), r.cfg.Key))
	f := r.files[surface]
	if f == nil {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if r.cfg.Append {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err = os.OpenFile(filepath.Join(r.cfg.OutDir, surface+"-raw.jsonl"), flags, 0o644)
		if err != nil {
			r.failOutputLocked(fmt.Errorf("open %s raw log: %w", surface, err))
			return
		}
		r.files[surface] = f
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		r.failOutputLocked(fmt.Errorf("write %s raw log: %w", surface, err))
	}
}

// failOutputLocked keeps the first output error. r.mu must be held.
func (r *Runner) failOutputLocked(err error) {
	err = errors.New(redact(err.Error(), r.cfg.Key))
	if r.outErr == nil {
		r.outErr = err
	}
	fmt.Fprintln(r.cfg.Log, "OUTPUT ERROR "+err.Error())
}

// closeFiles closes every raw log, keeping the first close error.
func (r *Runner) closeFiles() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for surface, f := range r.files {
		if err := f.Close(); err != nil {
			r.failOutputLocked(fmt.Errorf("close %s raw log: %w", surface, err))
		}
		delete(r.files, surface)
	}
}

func (r *Runner) outputErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.outErr
}

// matrix runs fn for every case and model: cases in order, the models of
// one case concurrently, so a spend stop leaves every model with the same
// cases and latencies are not inflated by our own queueing.
func matrix(n int, models []string, fn func(i int, model string)) { matrixK(n, 1, models, fn) }

// matrixK is matrix with k cases in flight at once.
func matrixK(n, k int, models []string, fn func(i int, model string)) {
	for i := 0; i < n; i += k {
		var wg sync.WaitGroup
		for c := i; c < min(i+k, n); c++ {
			for _, m := range models {
				wg.Add(1)
				go func() { defer wg.Done(); fn(c, m) }()
			}
		}
		wg.Wait()
	}
}

// estimate is the pre-dispatch reservation for one call: a catalog-priced
// prior until the model has an exact cost, then twice its highest exact
// cost so far.
func (r *Runner) estimate(surface, model string, audioSeconds float64) float64 {
	p := r.cat.Verify(model).Price
	var prior, floor float64
	switch surface {
	case "grading", "speech-audio-grade", "speech-today":
		prior, floor = p.Prompt*1500+p.Completion*100+max(p.Audio, p.Prompt)*audioSeconds*40, 0.0005
	case "explainer":
		prior, floor = p.Prompt*600+p.Completion*1024, 0.001
	case "drafting":
		prior, floor = p.Prompt*2000+p.Completion*6000, 0.01
	case "speech-chat":
		prior, floor = max(p.Audio, p.Prompt)*audioSeconds*40+p.Prompt*300+p.Completion*512, 0.001
	case "speech-stt":
		if p.Completion > 0 { // token-priced
			prior = p.Prompt*audioSeconds*50 + p.Completion*200
		} else { // priced per second of audio
			prior = p.Prompt * audioSeconds
		}
		floor = 0.001
	}
	r.mu.Lock()
	seen, ok := r.observed[surface+"/"+model]
	r.mu.Unlock()
	est := prior
	if ok {
		est = 2 * seen
	}
	return max(est, floor)
}

func (r *Runner) observe(surface, model string, cost *float64) {
	if cost == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if *cost > r.observed[surface+"/"+model] {
		r.observed[surface+"/"+model] = *cost
	}
}

// Run executes the configured surfaces and writes every output file.
func (r *Runner) Run(ctx context.Context) error {
	started := time.Now().UTC()
	if r.cfg.Append {
		// NewRunner checked this too; re-checked here, before the catalog
		// fetch or any provider call, in case OutDir changed in between.
		if err := checkPriorBlindKey(r.cfg.OutDir); err != nil {
			return err
		}
	}
	cat, err := FetchCatalog(ctx, r.cfg.Base)
	if err != nil {
		return err
	}
	r.cat = cat
	var before *float64
	if !r.cfg.SkipAccount {
		before = accountUsage(ctx, r.cfg.Base, r.cfg.Key)
	}
	for _, s := range r.cfg.Surfaces {
		if ctx.Err() != nil {
			r.note("run interrupted before surface %s; it was not run", s)
			continue
		}
		r.logf("== surface %s (budget so far: $%.6f of $%.6f)", s, r.budget.State().ChargedUSD, r.cfg.MaxSpend)
		switch s {
		case "grading":
			r.summary["grading"] = r.runGrading(ctx)
		case "explainer":
			r.summary["explainer"] = r.runExplainer(ctx)
		case "drafting":
			r.summary["drafting"] = r.runDrafting(ctx)
		case "speech":
			r.summary["speech"] = r.runSpeech(ctx)
		}
	}
	r.closeFiles()
	outErr := r.outputErr()
	if outErr != nil {
		r.note("raw log recording failed (%v): no further calls were dispatched after it; the run is incomplete", outErr)
	}
	if ctx.Err() != nil {
		r.note("run interrupted (%v): calls not yet dispatched were recorded as skipped; in-flight calls were cancelled", ctx.Err())
	}
	var after *float64
	if !r.cfg.SkipAccount {
		time.Sleep(5 * time.Second) // let the account counter catch up
		after = accountUsage(context.Background(), r.cfg.Base, r.cfg.Key)
	}
	// Every call has settled by now. The admission breaker reserves
	// estimates, so a call that settles above its reservation can push the
	// realized spend past the cap; record it and fail the run.
	var capErr error
	if st := r.budget.State(); st.CapExceeded {
		capErr = fmt.Errorf("%w: exact usage.cost $%.6f > cap $%.6f (overshoot $%.6f); --max-spend is a client-side pre-dispatch breaker, and no call was dispatched after the overshoot settled", ErrSpendCapExceeded, st.ExactUSD, st.MaxUSD, st.OvershootUSD)
		r.note("realized spend exceeded the cap: exact usage.cost $%.6f > max_spend $%.6f by $%.6f (a call settled above its pre-dispatch reservation, or --append under a cap below the earlier invocations' spend); summary and raw logs are complete for what ran, and the command exits non-zero", st.ExactUSD, st.MaxUSD, st.OvershootUSD)
	}
	ferr := r.finish(started, before, after)
	if outErr != nil {
		outErr = fmt.Errorf("raw log recording failed: %w", outErr)
	}
	return errors.Join(outErr, capErr, ferr)
}

// ErrInterrupted is returned for calls not dispatched after an interrupt.
var ErrInterrupted = errors.New("run interrupted: call not dispatched")

// reserve is the one gate before every call: an interrupted run, or one
// whose raw log can no longer be written, dispatches nothing more, and the
// spend cap is checked against the call's estimate.
func (r *Runner) reserve(ctx context.Context, surface, model string, audioSeconds float64) (*Ticket, error) {
	if ctx.Err() != nil {
		return nil, ErrInterrupted
	}
	if r.outputErr() != nil {
		return nil, ErrRecording
	}
	t, err := r.budget.Reserve(r.estimate(surface, model, audioSeconds))
	if err == nil {
		top := topSurface(surface)
		r.mu.Lock()
		r.produced[top] = true
		if r.dispatched[top] == nil {
			r.dispatched[top] = map[string]bool{}
		}
		r.dispatched[top][model] = true
		r.mu.Unlock()
	}
	return t, err
}

// topSurface maps an estimate key (speech-chat, speech-today, ...) to its
// surface.
func topSurface(s string) string {
	if strings.HasPrefix(s, "speech") {
		return "speech"
	}
	return s
}

// accountUsage is OpenRouter's lifetime usage for this key (GET
// /api/v1/key), an independent cross-check of the summed usage.cost.
func accountUsage(ctx context.Context, base, key string) *float64 {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/key", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Usage *float64 `json:"usage"`
		} `json:"data"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return nil
	}
	return out.Data.Usage
}

func gitInfo() (sha string, dirty bool) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", false
	}
	st, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return strings.TrimSpace(string(out)), len(bytes.TrimSpace(st)) > 0
}

func (r *Runner) finish(started time.Time, before, after *float64) error {
	sha, dirty := gitInfo()
	state := r.budget.State()
	verified := map[string]CatalogEntry{}
	for k, ms := range DefaultModels {
		if o, ok := r.cfg.Models[k]; ok {
			ms = o
		}
		for _, m := range ms {
			verified[m] = r.cat.Verify(m)
		}
	}
	account := map[string]any{"note": "OpenRouter GET /api/v1/key lifetime usage before and after this run (with --append: before the first invocation); the delta independently cross-checks the summed usage.cost (it can include other traffic on the same key, and calls that failed without reporting a cost, and can lag by seconds)."}
	commands := []string{fmt.Sprintf("[git %s] go run ./evals/cmd/scry-eval %s", sha, strings.Join(r.cfg.Args, " "))}
	surfaces := append([]string(nil), r.cfg.Surfaces...)
	notes := r.notes
	results := r.summary
	if p := r.prior; p != nil {
		if pa, ok := p.Summary["account_cross_check"].(map[string]any); ok {
			if b, ok := pa["usage_before_usd"].(float64); ok {
				before = &b
			}
		}
		if pc, ok := p.Summary["commands"].([]any); ok {
			var prev []string
			for _, c := range pc {
				prev = append(prev, fmt.Sprint(c))
			}
			commands = append(prev, commands...)
		}
		pr, _ := p.Summary["results"].(map[string]any)
		results = mergeResults(pr, results, r.produced)
		var all []string
		if ps, ok := p.Summary["surfaces"].([]any); ok {
			for _, s := range ps {
				all = append(all, fmt.Sprint(s))
			}
		}
		for _, s := range surfaces {
			if !oneOf(s, all...) {
				all = append(all, s)
			}
		}
		surfaces = all
		if pn, ok := p.Summary["notes"].([]any); ok {
			for _, n := range pn {
				notes = append(notes, fmt.Sprint(n))
			}
		}
		var dropped []string
		r.blind, r.key.Sets, dropped = mergeBlind(r.cfg.Seed, p.Blind, p.Key.Sets, r.blind, r.key.Sets, r.produced, r.dispatched)
		notes = append(notes, dropped...)
	}
	if before != nil && after != nil {
		account["usage_before_usd"], account["usage_after_usd"], account["delta_usd"] = *before, *after, *after-*before
	}
	sort.Strings(notes)
	sort.SliceStable(r.blind, func(i, j int) bool {
		if r.blind[i].Surface != r.blind[j].Surface {
			return r.blind[i].Surface > r.blind[j].Surface // explainer before drafting
		}
		return r.blind[i].CaseID < r.blind[j].CaseID
	})
	summary := map[string]any{
		"generated_at":        time.Now().UTC().Format(time.RFC3339),
		"started_at":          started.Format(time.RFC3339),
		"git_sha":             sha,
		"git_dirty_tracked":   dirty,
		"go_version":          runtime.Version(),
		"commands":            commands,
		"surfaces":            surfaces,
		"limit":               r.cfg.Limit,
		"blind_seed":          r.cfg.Seed,
		"total_spend_usd":     state.ExactUSD,
		"spend":               state,
		"account_cross_check": account,
		"catalog_verification": map[string]any{
			"source": "live GET " + r.cfg.Base + "/api/v1/models, ?output_modalities=decisions and ?output_modalities=transcription at run start",
			"models": verified,
		},
		"notes":              notes,
		"results":            results,
		"metric_definitions": metricDefinitions,
	}
	if err := writeJSON(filepath.Join(r.cfg.OutDir, "eval-summary.json"), summary, r.cfg.Key); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(r.cfg.OutDir, "blind-samples.json"), map[string]any{
		"note": "Blind side-by-side outputs. Labels are shuffled per case (see blind-key.json, sealed). Rate prose and outline quality before opening the key.",
		"sets": r.blind,
	}, r.cfg.Key); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(r.cfg.OutDir, "blind-key.json"), r.key, r.cfg.Key); err != nil {
		return err
	}
	if err := assertNoSecret(r.cfg.OutDir, r.cfg.Key); err != nil {
		return err
	}
	if r.cfg.EvidenceDir != "" {
		if err := copyEvidence(r.cfg.OutDir, r.cfg.EvidenceDir); err != nil {
			return err
		}
		if err := assertNoSecret(r.cfg.EvidenceDir, r.cfg.Key); err != nil {
			return err
		}
	}
	r.logf("== done: exact spend $%.6f over %d calls (%d unknown-cost, %d refused for budget); outputs in %s", state.ExactUSD, state.Calls, state.UnknownCostCalls, state.RefusedForBudget, r.cfg.OutDir)
	return nil
}

// mergeResults folds this invocation's results into an earlier one's
// (--append). A surface that dispatched nothing here (interrupted, or every
// call refused for budget) keeps its earlier result untouched. A surface
// that did run is merged per model row: a model that made calls here
// replaces its earlier row, and earlier rows for models not run here are
// kept and listed under rows_from_earlier_invocations.
func mergeResults(prior, cur map[string]any, produced map[string]bool) map[string]any {
	out := map[string]any{}
	for k, v := range prior {
		out[k] = v
	}
	for k, v := range cur {
		pv, has := prior[k]
		switch {
		case !has:
			out[k] = v
		case !produced[k]:
			// keep the earlier result
		default:
			pm, ok1 := pv.(map[string]any)
			cm, ok2 := asJSONMap(v)
			if !ok1 || !ok2 {
				out[k] = v
				continue
			}
			merged, kept := mergeLevel(pm, cm)
			if ed, ok := pm["earlier_dataset"]; ok && kept {
				merged["earlier_dataset"] = ed
			} else if kept && pm["dataset"] != nil && !jsonEqual(pm["dataset"], cm["dataset"]) {
				merged["earlier_dataset"] = pm["dataset"]
			}
			out[k] = merged
		}
	}
	return out
}

// mergeLevel merges one map level: "models" row lists per model, nested
// maps (speech tracks) recursively, anything else from cur. Keys only the
// earlier invocation wrote survive. kept reports whether any earlier row
// was kept at this level or below.
func mergeLevel(prior, cur map[string]any) (map[string]any, bool) {
	out := map[string]any{}
	for k, v := range prior {
		if k != "rows_from_earlier_invocations" && k != "earlier_dataset" {
			out[k] = v
		}
	}
	kept := false
	for k, v := range cur {
		pv, has := prior[k]
		if !has {
			out[k] = v
			continue
		}
		if k == "models" {
			rows, from := mergeRows(pv, v)
			out[k] = rows
			if len(from) > 0 {
				out["rows_from_earlier_invocations"] = from
				kept = true
			}
			continue
		}
		pm, ok1 := pv.(map[string]any)
		cm, ok2 := v.(map[string]any)
		if ok1 && ok2 && k != "dataset" {
			m, kk := mergeLevel(pm, cm)
			out[k], kept = m, kept || kk
			continue
		}
		out[k] = v
	}
	return out, kept
}

// mergeRows merges two model-row lists by "model": a current row that made
// calls wins; otherwise the earlier row is kept. Earlier order first, then
// models new in this invocation.
func mergeRows(pv, cv any) ([]any, []string) {
	prows, _ := pv.([]any)
	crows, _ := cv.([]any)
	model := func(row any) string {
		m, _ := row.(map[string]any)
		s, _ := m["model"].(string)
		return s
	}
	calls := func(row any) float64 {
		m, _ := row.(map[string]any)
		n, _ := m["calls"].(float64)
		return n
	}
	byModel := map[string]any{}
	for _, row := range crows {
		byModel[model(row)] = row
	}
	var out []any
	var kept []string
	seen := map[string]bool{}
	for _, row := range prows {
		m := model(row)
		seen[m] = true
		if c, ok := byModel[m]; ok && (calls(c) > 0 || calls(row) == 0) {
			out = append(out, c)
			continue
		}
		out = append(out, row)
		kept = append(kept, m)
	}
	for _, row := range crows {
		if !seen[model(row)] {
			out = append(out, row)
		}
	}
	return out, kept
}

// mergeBlind folds this invocation's blind sets and key into an earlier
// one's (--append). Sets for surfaces that dispatched nothing here are kept
// verbatim, with their key entries. For a surface that ran, the blind
// outputs follow the same rule as the summary rows (mergeRows), so the two
// never mix invocations for one model:
//
//   - a model that dispatched at least one call here (rerun[surface]) has
//     its summary row replaced, so every earlier blind output of that model
//     on that surface is dropped and only this invocation's outputs (ok,
//     error or skipped) stand, including on cases not re-run here;
//   - any other model keeps its earlier outputs; a current output fills in
//     only where the model had none before.
//
// A case whose model set changed is re-laid out with BuildBlind over its
// remaining models, so the key stays the only provenance; a case left with
// no model is dropped. notes describes every earlier output dropped.
func mergeBlind(seed string, pSets []BlindSet, pKey map[string]map[string]string, cSets []BlindSet, cKey map[string]map[string]string, produced map[string]bool, rerun map[string]map[string]bool) ([]BlindSet, map[string]map[string]string, []string) {
	id := func(b BlindSet) string { return b.Surface + "/" + b.CaseID }
	cur := map[string]BlindSet{}
	for _, b := range cSets {
		cur[id(b)] = b
	}
	var sets []BlindSet
	key := map[string]map[string]string{}
	used := map[string]bool{}
	droppedBy := map[string]int{} // surface\x00model -> earlier outputs dropped on cases not re-run
	for _, ps := range pSets {
		k := id(ps)
		cs, has := cur[k]
		if !produced[ps.Surface] {
			used[k] = true // an unrun surface never replaces earlier artifacts
		}
		if !produced[ps.Surface] || pKey[k] == nil {
			if has && produced[ps.Surface] { // earlier set without a key: cannot de-anonymize, replace
				continue
			}
			sets = append(sets, ps)
			if pKey[k] != nil {
				key[k] = pKey[k]
			}
			continue
		}
		used[k] = true
		outs := map[string]BlindOutput{}
		var models []string
		changed := has
		for _, o := range ps.Outputs {
			m := pKey[k][o.ModelLabel]
			if m == "" {
				continue
			}
			if rerun[ps.Surface][m] {
				changed = true
				if !has {
					droppedBy[ps.Surface+"\x00"+m]++
				}
				continue // replaced by this invocation, like its summary row
			}
			outs[m] = o
			models = append(models, m)
		}
		if has {
			for _, o := range cs.Outputs {
				m := cKey[k][o.ModelLabel]
				if m == "" {
					continue
				}
				if _, had := outs[m]; had && !rerun[cs.Surface][m] {
					continue // not re-run here: the earlier output stands
				}
				if _, had := outs[m]; !had {
					models = append(models, m)
				}
				outs[m] = o
			}
		}
		if !changed {
			sets = append(sets, ps)
			key[k] = pKey[k]
			continue
		}
		if len(models) == 0 {
			continue
		}
		ctx := ps.Context
		if has {
			ctx = cs.Context
		}
		set, assign := BuildBlind(seed, ps.Surface, ps.CaseID, ctx, models, outs)
		sets = append(sets, set)
		key[k] = assign
	}
	for _, cs := range cSets {
		k := id(cs)
		if used[k] {
			continue
		}
		sets = append(sets, cs)
		if cKey[k] != nil {
			key[k] = cKey[k]
		}
	}
	var notes []string
	for sm, n := range droppedBy {
		surface, model, _ := strings.Cut(sm, "\x00")
		notes = append(notes, fmt.Sprintf("--append: %s model %s was re-run on fewer cases than before; its %d earlier blind output(s) on cases not re-run were dropped so blind-samples.json matches its replaced summary row", surface, model, n))
	}
	sort.Strings(notes)
	return sets, key, notes
}

// blindRan reports whether a blind output came from a dispatched call.
func blindRan(status string) bool {
	return status != "not run" && !strings.HasPrefix(status, "skipped")
}

func asJSONMap(v any) (map[string]any, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m) == nil
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

var metricDefinitions = map[string]string{
	"grading.accuracy_pct":         "3-way exact match of predicted (correct/close/incorrect) to label over dispatched calls; abstentions and errors count as wrong",
	"grading.fpr_pct":              "share of label=incorrect cases predicted correct",
	"grading.fnr_pct":              "share of label=correct cases predicted incorrect",
	"grading.abstentions":          "calls Scry's Jev adapter rejected through its confidence (>=0.5), probability (>=0.8) or margin (>=0.2) gates, or none_of_these",
	"usd_per_1000":                 "mean exact usage.cost per call with a reported cost x 1000",
	"explainer.rubric":             "deterministic 0-100: verdict respected 30, concision 25 (<=40 words 10, <=80 words 5, <=280 chars unclamped 10), format 20 (no markdown 7, no bullets 6, no preamble 7), contrast 25 (required concepts 15 pro rata, addresses learner answer 10); failed calls score 0 in rubric_mean_all",
	"drafting.schema_valid_pct":    "share of drafts whose raw JSON Lines meet Scry's contract: head line, 4-7 ideas (title <=48 chars no colon, summary <=140, 1-3 links, 2-4 items), done line last, zero malformed lines",
	"drafting.quality":             "deterministic 0-100, 20 each: standalone questions, non-yes/no questions, answers <=25 words, ideas not duplicating learner.knows, links copied exactly",
	"usd_per_100_drafts":           "mean exact usage.cost per draft x 100",
	"speech.wer_pct":               "word error rate vs the spoken text after lowercase, diacritic fold, punctuation strip and number-word->digit normalization; mean over clips",
	"speech.term_match_pct":        "share of all critical terms found in the normalized transcript",
	"speech.usd_per_1000_5s_clips": "sum of exact usage.cost / sum of clip seconds x 5 x 1000",
}

func writeJSON(path string, v any, key string) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(redact(string(b), key)+"\n"), 0o644)
}

// EvidenceNames maps run outputs to the flat names the evidence collector
// accepts (it does not collect .jsonl).
var EvidenceNames = map[string]string{
	"eval-summary.json":   "eval-summary.json",
	"grading-raw.jsonl":   "grading-raw.jsonl.txt",
	"explainer-raw.jsonl": "explainer-raw.jsonl.txt",
	"drafting-raw.jsonl":  "drafting-raw.jsonl.txt",
	"speech-raw.jsonl":    "speech-raw.jsonl.txt",
	"blind-samples.json":  "blind-samples.json",
	"blind-key.json":      "blind-key.json",
}

func copyEvidence(from, to string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for src, dst := range EvidenceNames {
		b, err := os.ReadFile(filepath.Join(from, src))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(to, dst), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// assertNoSecret fails if the key appears in any file under dir.
func assertNoSecret(dir, key string) error {
	if key == "" {
		return nil
	}
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(key)) {
			return fmt.Errorf("secret found in %s; refusing to finish", p)
		}
		return nil
	})
}

// Stats summarizes latencies and costs of one model's dispatched calls.
type Stats struct {
	Calls        int     `json:"calls"`
	Skipped      int     `json:"skipped_for_budget"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	UnknownCost  int     `json:"unknown_cost_calls"`
	MeanCostUSD  float64 `json:"mean_cost_usd"`
}

type statAcc struct {
	lat     []float64
	cost    float64
	known   int
	unknown int
	skipped int
}

func (a *statAcc) add(latMs float64, cost *float64) {
	a.lat = append(a.lat, latMs)
	if cost == nil {
		a.unknown++
		return
	}
	a.cost += *cost
	a.known++
}

func (a *statAcc) stats() Stats {
	s := Stats{Calls: len(a.lat), Skipped: a.skipped, P50Ms: Percentile(a.lat, .5), P95Ms: Percentile(a.lat, .95), TotalCostUSD: a.cost, UnknownCost: a.unknown}
	if a.known > 0 {
		s.MeanCostUSD = a.cost / float64(a.known)
	}
	return s
}

func pct(n, of int) float64 {
	if of == 0 {
		return 0
	}
	return math.Round(1000*float64(n)/float64(of)) / 10
}

func ms(d time.Duration) float64 { return math.Round(float64(d)/float64(time.Millisecond)*10) / 10 }

func errText(err error, key string) string {
	if err == nil {
		return ""
	}
	return redact(err.Error(), key)
}
