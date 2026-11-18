// judge-eval compares Decisions models. Only an explicit invocation sends data.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"scry/internal/ai"
	"sort"
	"strings"
	"time"
)

type Sample struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Prompt   string   `json:"prompt"`
	Accepted []string `json:"accepted_answers"`
	Answer   string   `json:"answer"`
	Label    string   `json:"label"`
	Category string   `json:"category"`
}
type Report struct {
	Model        string                    `json:"model"`
	Count        int                       `json:"count"`
	Accuracy     float64                   `json:"accuracy"`
	Confusion    map[string]map[string]int `json:"confusion"`
	TotalCost    *float64                  `json:"total_usage_cost"`
	KnownCost    float64                   `json:"known_usage_cost"`
	UnknownCosts int                       `json:"unknown_cost_calls"`
	P50          float64                   `json:"p50_ms"`
	P95          float64                   `json:"p95_ms"`
}

func readSamples(path string) ([]Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 1<<20)
	var rows []Sample
	ids := map[string]bool{}
	for scan.Scan() {
		var r Sample
		if err := json.Unmarshal(scan.Bytes(), &r); err != nil {
			return nil, err
		}
		if r.ID == "" || ids[r.ID] || r.Prompt == "" || len(r.Accepted) == 0 || !(r.Label == "correct" || r.Label == "close" || r.Label == "incorrect") {
			return nil, fmt.Errorf("invalid sample at line %d", len(rows)+1)
		}
		ids[r.ID] = true
		rows = append(rows, r)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("empty sample")
	}
	return rows, nil
}
func evaluate(ctx context.Context, rows []Sample, models []string, base, key string, out io.Writer) error {
	if len(rows) == 0 || len(models) == 0 || key == "" {
		return errors.New("samples, model list and OPENROUTER_API_KEY are required")
	}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			return errors.New("empty model slug")
		}
		j := &ai.Jev{APIKey: key, OpenRouter: true, Model: model, BaseURL: base}
		report := Report{Model: model, Count: len(rows), Confusion: map[string]map[string]int{}}
		correct := 0
		latencies := make([]float64, 0, len(rows))
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			start := time.Now()
			decision, use, err := j.Judge(ctx, ai.JudgeInput{Question: row.Prompt, AcceptedAnswers: row.Accepted, Answer: row.Answer})
			latencies = append(latencies, float64(time.Since(start))/float64(time.Millisecond))
			predicted := "not_answered"
			if err == nil {
				switch decision.Verdict {
				case "right":
					predicted = "correct"
				case "close":
					predicted = "close"
				case "missed":
					predicted = "incorrect"
				}
			}
			if predicted == row.Label {
				correct++
			}
			if report.Confusion[row.Label] == nil {
				report.Confusion[row.Label] = map[string]int{}
			}
			report.Confusion[row.Label][predicted]++
			if use.CostUSD == nil {
				report.UnknownCosts++
			} else {
				report.KnownCost += *use.CostUSD
			}
		}
		report.Accuracy = float64(correct) / float64(len(rows))
		if report.UnknownCosts == 0 {
			cost := report.KnownCost
			report.TotalCost = &cost
		}
		sort.Float64s(latencies)
		report.P50 = latencies[int(math.Ceil(.50*float64(len(latencies))))-1]
		report.P95 = latencies[int(math.Ceil(.95*float64(len(latencies))))-1]
		if err := json.NewEncoder(out).Encode(report); err != nil {
			return err
		}
	}
	return nil
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("judge-eval", flag.ContinueOnError)
	models := flags.String("models", "", "comma-separated Decisions model slugs")
	data := flags.String("data", "testdata/judge-eval.jsonl", "labeled JSONL sample")
	if err := flags.Parse(args); err != nil {
		return err
	}
	list := flags.Args()
	if *models != "" {
		list = append(strings.Split(*models, ","), list...)
	}
	rows, err := readSamples(*data)
	if err != nil {
		return err
	}
	return evaluate(context.Background(), rows, list, "", os.Getenv("OPENROUTER_API_KEY"), out)
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
