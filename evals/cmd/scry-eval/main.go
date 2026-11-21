// scry-eval runs Scry's 4-surface micro-eval bench live on OpenRouter.
// Only an explicit invocation sends data; it reads OPENROUTER_API_KEY from
// the environment and never prints or writes it.
//
//	go run ./evals/cmd/scry-eval --surface all --out evals/results/latest --max-spend 1.35
//	go run ./evals/cmd/scry-eval --surface grading --limit 3
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"scry/evals"
)

func run(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("scry-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	surface := fs.String("surface", "all", "all, or a comma list of: "+strings.Join(evals.Surfaces, ", "))
	out := fs.String("out", "", "output directory (default evals/results/<UTC timestamp>)")
	evidence := fs.String("evidence", "", "optional flat directory for collector-friendly copies (.jsonl -> .jsonl.txt)")
	maxSpend := fs.Float64("max-spend", 1.35, "hard spend cap in USD for this run, from exact usage.cost")
	lim := fs.Int("limit", 0, "cases (clips) per surface; 0 means all")
	seed := fs.String("seed", "scry-eval-v1", "blind-label shuffle seed")
	appendRun := fs.Bool("append", false, "continue a run in --out: append raw logs, merge the summary and blind sets, and count the earlier spend against --max-spend")
	caseParallel := fs.Int("case-parallel", 1, "drafting cases in flight at once (all models of each)")
	var models modelFlags
	fs.Var(&models, "models", "override one surface's models: key=slug,slug (keys: grading, explainer, drafting, drafting-optional, speech-chat, speech-stt, speech-stt-probe, speech-audio-grade); repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var surfaces []string
	if *surface == "all" {
		surfaces = evals.Surfaces
	} else {
		for _, s := range strings.Split(*surface, ",") {
			if s = strings.TrimSpace(s); s != "" {
				surfaces = append(surfaces, s)
			}
		}
	}
	if *out == "" {
		*out = filepath.Join("evals", "results", time.Now().UTC().Format("20060102T150405Z"))
	}
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	r, err := evals.NewRunner(evals.Config{Surfaces: surfaces, OutDir: *out, EvidenceDir: *evidence, MaxSpend: *maxSpend, Limit: *lim, Models: models.m, Key: key, Seed: *seed, Args: args, Log: stderr, Append: *appendRun, CaseParallel: *caseParallel})
	if err != nil {
		return err
	}
	// SIGINT/SIGTERM stop dispatching; the summary is still written.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return r.Run(ctx)
}

type modelFlags struct{ m map[string][]string }

func (f *modelFlags) String() string { return fmt.Sprint(f.m) }

func (f *modelFlags) Set(v string) error {
	k, list, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return errors.New("want key=slug,slug")
	}
	if f.m == nil {
		f.m = map[string][]string{}
	}
	var slugs []string
	for _, s := range strings.Split(list, ",") {
		if s = strings.TrimSpace(s); s != "" {
			slugs = append(slugs, s)
		}
	}
	f.m[k] = slugs
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "scry-eval:", err)
		os.Exit(1)
	}
}
