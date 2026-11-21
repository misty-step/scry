package evals

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"scry/internal/ai"
	"scry/internal/srs"
)

// ExplainerRecord is one raw explainer call.
type ExplainerRecord struct {
	Surface          string         `json:"surface"`
	CaseID           string         `json:"case_id"`
	Domain           string         `json:"domain"`
	Verdict          string         `json:"verdict"`
	Model            string         `json:"model"`
	Status           string         `json:"status"`           // ok, error, skipped
	OutputText       string         `json:"output_text"`      // what Scry shows (ClampExplanation)
	RawText          string         `json:"raw_text"`         // the model's unclamped text, which the rubric scores
	Rubric           *ExplainRubric `json:"rubric,omitempty"` // nil when there was no output
	LatencyMs        float64        `json:"latency_ms"`
	CostUSD          *float64       `json:"cost_usd"`
	PromptTokens     int64          `json:"prompt_tokens"`
	CompletionTokens int64          `json:"completion_tokens"`
	ReasoningTokens  int64          `json:"reasoning_tokens,omitempty"`
	ResolvedModel    string         `json:"resolved_model"`
	Provider         string         `json:"provider,omitempty"`
	HTTPStatus       int            `json:"http_status,omitempty"`
	Error            string         `json:"error,omitempty"`
}

func chatText(raw []byte) (text string, reasoning int64) {
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			Details *struct {
				Reasoning int64 `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return "", 0
	}
	if len(out.Choices) > 0 && out.Choices[0].Message.Content != nil {
		text = *out.Choices[0].Message.Content
	}
	if out.Usage != nil && out.Usage.Details != nil {
		reasoning = out.Usage.Details.Reasoning
	}
	return text, reasoning
}

// ExplainerRow is one model's explainer metrics.
type ExplainerRow struct {
	Model            string  `json:"model"`
	Cases            int     `json:"cases"`
	OK               int     `json:"ok"`
	Errors           int     `json:"errors"`
	RubricMeanOK     float64 `json:"rubric_mean_ok"`
	RubricMeanAll    float64 `json:"rubric_mean_all"`
	VerdictRespected float64 `json:"verdict_respected_pct"`
	Within40Pct      float64 `json:"within_40_words_pct"`
	Within80Pct      float64 `json:"within_80_words_pct"`
	Within280Pct     float64 `json:"within_280_chars_unclamped_pct"`
	CleanFormatPct   float64 `json:"clean_format_pct"`
	ConceptCoverage  float64 `json:"concept_coverage_pct"`
	AddressesLearner float64 `json:"addresses_learner_pct"`
	MeanWords        float64 `json:"mean_words"`
	USDPer1000       float64 `json:"usd_per_1000"`
	Stats
}

func (r *Runner) runExplainer(ctx context.Context) map[string]any {
	cases := limit(r.data.Explainer, r.cfg.Limit)
	models := r.models("explainer")
	recs := map[string]map[string]ExplainerRecord{}
	accs := map[string]*statAcc{}
	for _, m := range models {
		accs[m], recs[m] = &statAcc{}, map[string]ExplainerRecord{}
	}
	matrix(len(cases), models, func(i int, m string) {
		c := cases[i]
		rec := ExplainerRecord{Surface: "explainer", CaseID: c.ID, Domain: c.Domain, Verdict: c.Verdict, Model: m}
		t, err := r.reserve(ctx, r.estimate("explainer", m, 0))
		if err != nil {
			rec.Status, rec.Error = "skipped", err.Error()
		} else {
			rc := newRecorder()
			cl, err := ai.NewClaudeWithRouter("", r.cfg.Key, m, m)
			if err != nil {
				rec.Status, rec.Error = "error", errText(err, r.cfg.Key)
				t.Settle(nil)
			} else {
				cl.BaseURL = r.cfg.Base
				cl.HTTP = rc.client(ai.ExplainTimeout)
				ectx, cancel := context.WithTimeout(ctx, ai.ExplainTimeout) // as App.Explain
				start := time.Now()
				text, use, xerr := cl.Explain(ectx, ai.ExplainInput{Question: c.Question, Expected: c.Expected, Answer: c.LearnerAnswer, Verdict: srs.Verdict(c.Verdict)})
				rec.LatencyMs = ms(time.Since(start))
				cancel()
				raw := rc.buf.Bytes()
				u := parseUsage(raw)
				rec.CostUSD = use.CostUSD
				if rec.CostUSD == nil {
					rec.CostUSD = u.Cost
				}
				t.Settle(rec.CostUSD)
				r.observe("explainer", m, rec.CostUSD)
				rec.PromptTokens, rec.CompletionTokens, rec.ResolvedModel, rec.Provider, rec.HTTPStatus = u.Prompt, u.Completion, redact(u.Model, r.cfg.Key), u.Provider, rc.status
				rec.RawText, rec.ReasoningTokens = chatText(raw)
				rec.OutputText = text
				if xerr != nil {
					rec.Status, rec.Error = "error", errText(xerr, r.cfg.Key)
					if rc.status != 0 && rc.status != 200 {
						rec.Error += " — " + errorSnippet(rc.status, raw, r.cfg.Key)
					}
				} else {
					rec.Status = "ok"
					rub := ScoreExplanation(c, rec.RawText)
					rec.Rubric = &rub
				}
			}
		}
		r.write("explainer", rec)
		r.mu.Lock()
		recs[m][c.ID] = rec
		if rec.Status == "skipped" {
			accs[m].skipped++
		} else {
			accs[m].add(rec.LatencyMs, rec.CostUSD)
		}
		r.mu.Unlock()
		score := -1
		if rec.Rubric != nil {
			score = rec.Rubric.Score
		}
		r.logf("explainer %-34s %s %-7s score=%3d %7.0fms cost=%s %s", m, c.ID, rec.Status, score, rec.LatencyMs, costStr(rec.CostUSD), rec.Error)
	})
	var rows []ExplainerRow
	for _, m := range models {
		row := ExplainerRow{Model: m}
		var sum, words float64
		var resp, w40, w80, w280, clean, cov, covOf, addr int
		for _, c := range cases {
			rec, ok := recs[m][c.ID]
			if !ok || rec.Status == "skipped" {
				continue
			}
			row.Cases++
			if rec.Rubric == nil {
				row.Errors++
				continue
			}
			row.OK++
			rb := rec.Rubric
			sum += float64(rb.Score)
			words += float64(rb.Words)
			for _, b := range []struct {
				ok bool
				n  *int
			}{{rb.VerdictRespected, &resp}, {rb.Within40Words, &w40}, {rb.Within80Words, &w80}, {rb.Within280Chars, &w280}, {rb.NoMarkdown && rb.NoBullets && rb.NoPreamble, &clean}, {rb.AddressesLearner, &addr}} {
				if b.ok {
					*b.n++
				}
			}
			cov += rb.ConceptsCovered
			covOf += rb.ConceptsTotal
		}
		if row.OK > 0 {
			row.RubricMeanOK = math.Round(10*sum/float64(row.OK)) / 10
			row.MeanWords = math.Round(10*words/float64(row.OK)) / 10
		}
		if row.Cases > 0 {
			row.RubricMeanAll = math.Round(10*sum/float64(row.Cases)) / 10
		}
		row.VerdictRespected, row.Within40Pct, row.Within80Pct, row.Within280Pct = pct(resp, row.OK), pct(w40, row.OK), pct(w80, row.OK), pct(w280, row.OK)
		row.CleanFormatPct, row.ConceptCoverage, row.AddressesLearner = pct(clean, row.OK), pct(cov, covOf), pct(addr, row.OK)
		row.Stats = accs[m].stats()
		row.USDPer1000 = math.Round(row.MeanCostUSD*1000*1e6) / 1e6
		rows = append(rows, row)
	}
	// Blind side-by-side: every case, labels shuffled per case.
	for _, c := range cases {
		outs := map[string]BlindOutput{}
		for _, m := range models {
			rec, ok := recs[m][c.ID]
			switch {
			case !ok:
			case rec.Status == "ok":
				outs[m] = BlindOutput{Status: "ok", Output: rec.OutputText}
			default:
				outs[m] = BlindOutput{Status: rec.Status + ": no explanation (Scry would show the fixed criterion sentence)"}
			}
		}
		set, assign := BuildBlind(r.cfg.Seed, "explainer", c.ID, map[string]any{
			"question": c.Question, "expected": c.Expected, "learner_answer": c.LearnerAnswer, "verdict": c.Verdict, "key_misconception": c.KeyMisconception,
		}, models, outs)
		r.mu.Lock()
		r.blind = append(r.blind, set)
		r.key.Sets["explainer/"+c.ID] = assign
		r.mu.Unlock()
	}
	return map[string]any{"dataset": map[string]any{"cases": len(cases), "file": "evals/datasets/explainer.jsonl"},
		"adapter": "scry/internal/ai.NewClaudeWithRouter -> Claude.Explain (OpenRouter chat completions, production system prompt, XML card framing, 1024 max tokens, 8s timeout, ClampExplanation)",
		"models":  rows}
}
