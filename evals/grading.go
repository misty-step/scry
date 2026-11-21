package evals

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"scry/internal/ai"
	"scry/internal/srs"
)

// GradingRecord is one raw grading call.
type GradingRecord struct {
	Surface          string             `json:"surface"`
	Track            string             `json:"track,omitempty"`
	CaseID           string             `json:"case_id"`
	Category         string             `json:"category,omitempty"`
	Model            string             `json:"model"`
	Label            string             `json:"label"`
	Predicted        string             `json:"predicted"` // correct, close, incorrect, abstain, error, skipped
	Match            bool               `json:"match"`
	Transcript       string             `json:"transcript,omitempty"`
	LatencyMs        float64            `json:"latency_ms"`
	CostUSD          *float64           `json:"cost_usd"`
	PromptTokens     int64              `json:"prompt_tokens"`
	CompletionTokens int64              `json:"completion_tokens"`
	ResolvedModel    string             `json:"resolved_model"`
	Provider         string             `json:"provider,omitempty"`
	RawChoice        string             `json:"raw_choice,omitempty"`
	Confidence       *float64           `json:"confidence,omitempty"`
	Probabilities    map[string]float64 `json:"probabilities,omitempty"`
	HTTPStatus       int                `json:"http_status,omitempty"`
	Error            string             `json:"error,omitempty"`
}

// verdictLabel maps Scry's verdicts to the dataset's labels.
func verdictLabel(v srs.Verdict) string {
	switch v {
	case srs.Right:
		return "correct"
	case srs.Close:
		return "close"
	case srs.Missed:
		return "incorrect"
	}
	return "error"
}

// errJevAbstain is the exact error Scry's Jev adapter returns when its
// acceptance gates reject a decision (or the model chose none_of_these).
const errJevAbstain = "Jev decision unavailable or ambiguous"

// decisionAnswer is the raw judgment answer, read from the response the
// adapter received so abstentions can be explained.
func decisionAnswer(raw []byte) (choice string, conf *float64, probs map[string]float64) {
	var out struct {
		Answers map[string]struct {
			Choice        string             `json:"choice"`
			Confidence    *float64           `json:"confidence"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return "", nil, nil
	}
	a := out.Answers["judgment"]
	return a.Choice, a.Confidence, a.Probabilities
}

// judgeOnce runs Scry's production Jev adapter once (model "" is the
// production default) and fills rec.
func (r *Runner) judgeOnce(ctx context.Context, surface, model string, in ai.JudgeInput, rec *GradingRecord) {
	t, err := r.reserve(ctx, r.estimate(surface, model, 0))
	if err != nil {
		rec.Predicted = "skipped"
		rec.Error = err.Error()
		return
	}
	rc := newRecorder()
	j := &ai.Jev{APIKey: r.cfg.Key, OpenRouter: true, Model: model, BaseURL: r.cfg.Base, HTTP: rc.client(20 * time.Second), Log: quiet}
	jctx, cancel := context.WithTimeout(ctx, 25*time.Second) // as App.Judge
	start := time.Now()
	dec, use, jerr := j.Judge(jctx, in)
	rec.LatencyMs = ms(time.Since(start))
	cancel()
	raw := rc.buf.Bytes()
	u := parseUsage(raw)
	rec.CostUSD = use.CostUSD
	if rec.CostUSD == nil {
		rec.CostUSD = u.Cost
	}
	t.Settle(rec.CostUSD)
	r.observe(surface, model, rec.CostUSD)
	rec.PromptTokens, rec.CompletionTokens = u.Prompt, u.Completion
	rec.ResolvedModel, rec.Provider, rec.HTTPStatus = redact(u.Model, r.cfg.Key), u.Provider, rc.status
	rec.RawChoice, rec.Confidence, rec.Probabilities = decisionAnswer(raw)
	switch {
	case jerr == nil:
		rec.Predicted = verdictLabel(dec.Verdict)
	case jerr.Error() == errJevAbstain:
		rec.Predicted = "abstain"
		rec.Error = jerr.Error()
	default:
		rec.Predicted = "error"
		rec.Error = errText(jerr, r.cfg.Key)
		if rc.status != 0 && rc.status != 200 {
			rec.Error += " — " + errorSnippet(rc.status, raw, r.cfg.Key)
		}
	}
	rec.Match = rec.Predicted == rec.Label
}

// GradingRow is one model's grading metrics.
type GradingRow struct {
	Model       string                    `json:"model"`
	Cases       int                       `json:"cases"`
	AccuracyPct float64                   `json:"accuracy_pct"`
	FPRPct      float64                   `json:"fpr_pct"`
	FNRPct      float64                   `json:"fnr_pct"`
	Abstentions int                       `json:"abstentions"`
	Errors      int                       `json:"errors"`
	USDPer1000  float64                   `json:"usd_per_1000"`
	Confusion   map[string]map[string]int `json:"confusion"`
	Stats
}

// scoreGrades aggregates records for one model. Only dispatched calls count.
func scoreGrades(model string, recs []GradingRecord, acc *statAcc) GradingRow {
	row := GradingRow{Model: model, Confusion: map[string]map[string]int{}}
	var right, incorrect, fp, correct, fn int
	for _, rec := range recs {
		if rec.Predicted == "skipped" {
			continue
		}
		row.Cases++
		if row.Confusion[rec.Label] == nil {
			row.Confusion[rec.Label] = map[string]int{}
		}
		row.Confusion[rec.Label][rec.Predicted]++
		if rec.Match {
			right++
		}
		switch rec.Predicted {
		case "abstain":
			row.Abstentions++
		case "error":
			row.Errors++
		}
		if rec.Label == "incorrect" {
			incorrect++
			if rec.Predicted == "correct" {
				fp++
			}
		}
		if rec.Label == "correct" {
			correct++
			if rec.Predicted == "incorrect" {
				fn++
			}
		}
	}
	row.AccuracyPct, row.FPRPct, row.FNRPct = pct(right, row.Cases), pct(fp, incorrect), pct(fn, correct)
	row.Stats = acc.stats()
	row.USDPer1000 = math.Round(row.MeanCostUSD*1000*1e6) / 1e6
	return row
}

func (r *Runner) runGrading(ctx context.Context) map[string]any {
	cases := limit(r.data.Grading, r.cfg.Limit)
	models := r.models("grading")
	recs := map[string][]GradingRecord{}
	accs := map[string]*statAcc{}
	for _, m := range models {
		accs[m] = &statAcc{}
	}
	matrix(len(cases), models, func(i int, m string) {
		c := cases[i]
		rec := GradingRecord{Surface: "grading", CaseID: c.ID, Category: c.Category, Model: m, Label: c.Label}
		r.judgeOnce(ctx, "grading", m, ai.JudgeInput{Question: c.Prompt, AcceptedAnswers: c.Accepted, Answer: c.Answer}, &rec)
		r.write("grading", rec)
		r.mu.Lock()
		recs[m] = append(recs[m], rec)
		if rec.Predicted == "skipped" {
			accs[m].skipped++
		} else {
			accs[m].add(rec.LatencyMs, rec.CostUSD)
		}
		r.mu.Unlock()
		r.logf("grading %-30s %s label=%-9s got=%-9s %7.0fms cost=%s", m, c.ID, c.Label, rec.Predicted, rec.LatencyMs, costStr(rec.CostUSD))
	})
	var rows []GradingRow
	for _, m := range models {
		rows = append(rows, scoreGrades(m, recs[m], accs[m]))
	}
	counts := map[string]int{}
	for _, c := range cases {
		counts[c.Label]++
	}
	return map[string]any{"dataset": map[string]any{"cases": len(cases), "labels": counts, "file": "evals/datasets/grading.jsonl"},
		"adapter": "scry/internal/ai.Jev.Judge (OpenRouter /api/alpha/decisions, production prompt, criteria and acceptance gates)", "models": rows}
}

func costStr(c *float64) string {
	if c == nil {
		return "unknown"
	}
	return "$" + trimFloat(*c)
}

func trimFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
