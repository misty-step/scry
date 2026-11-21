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
	return &Runner{cfg: cfg, data: data, budget: &Budget{Max: cfg.MaxSpend}, files: map[string]*os.File{}, observed: map[string]float64{},
		key:     BlindKey{Seed: cfg.Seed, Note: "Sealed provenance for blind-samples.json: surface/case -> blind label -> model slug. Do not open before rating.", Sets: map[string]map[string]string{}},
		summary: map[string]any{}}, nil
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

// write appends one raw record to <surface>-raw.jsonl.
func (r *Runner) write(surface string, rec any) {
	b, err := json.Marshal(rec)
	if err != nil {
		r.logf("record encode failed: %v", err)
		return
	}
	b = []byte(redact(string(b), r.cfg.Key))
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.files[surface]
	if f == nil {
		f, err = os.Create(filepath.Join(r.cfg.OutDir, surface+"-raw.jsonl"))
		if err != nil {
			fmt.Fprintf(r.cfg.Log, "cannot create raw log: %v\n", err)
			return
		}
		r.files[surface] = f
	}
	f.Write(append(b, '\n'))
}

// matrix runs fn for every case and model: cases in order, the models of
// one case concurrently, so a spend stop leaves every model with the same
// cases and latencies are not inflated by our own queueing.
func matrix(n int, models []string, fn func(i int, model string)) {
	for i := 0; i < n; i++ {
		var wg sync.WaitGroup
		for _, m := range models {
			wg.Add(1)
			go func() { defer wg.Done(); fn(i, m) }()
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
		if err := ctx.Err(); err != nil {
			return err
		}
		r.logf("== surface %s (budget so far: $%.6f of $%.2f)", s, r.budget.State().ChargedUSD, r.cfg.MaxSpend)
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
	for _, f := range r.files {
		f.Close()
	}
	var after *float64
	if !r.cfg.SkipAccount {
		time.Sleep(5 * time.Second) // let the account counter catch up
		after = accountUsage(ctx, r.cfg.Base, r.cfg.Key)
	}
	return r.finish(started, before, after)
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
	account := map[string]any{"note": "OpenRouter GET /api/v1/key lifetime usage before and after this run; the delta independently cross-checks the summed usage.cost (it can include other traffic on the same key and can lag by seconds)."}
	if before != nil && after != nil {
		account["usage_before_usd"], account["usage_after_usd"], account["delta_usd"] = *before, *after, *after-*before
	}
	sort.Strings(r.notes)
	summary := map[string]any{
		"generated_at":        time.Now().UTC().Format(time.RFC3339),
		"started_at":          started.Format(time.RFC3339),
		"git_sha":             sha,
		"git_dirty_tracked":   dirty,
		"go_version":          runtime.Version(),
		"command":             "go run ./evals/cmd/scry-eval " + strings.Join(r.cfg.Args, " "),
		"surfaces":            r.cfg.Surfaces,
		"limit":               r.cfg.Limit,
		"blind_seed":          r.cfg.Seed,
		"total_spend_usd":     state.ExactUSD,
		"spend":               state,
		"account_cross_check": account,
		"catalog_verification": map[string]any{
			"source": "live GET " + r.cfg.Base + "/api/v1/models, ?output_modalities=decisions and ?output_modalities=transcription at run start",
			"models": verified,
		},
		"notes":              r.notes,
		"results":            r.summary,
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
