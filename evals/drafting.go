package evals

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"scry/internal/ai"
)

// DraftingRecord is one raw drafting call.
type DraftingRecord struct {
	Surface          string        `json:"surface"`
	CaseID           string        `json:"case_id"`
	Category         string        `json:"category"`
	Model            string        `json:"model"`
	Status           string        `json:"status"`                  // ok, error, skipped
	ProdHead         *ai.DraftHead `json:"prod_head,omitempty"`     // what Scry's parser handed the sink
	ProdIdeas        int           `json:"prod_ideas"`              // ideas Scry's parser accepted
	ProdNote         string        `json:"prod_note,omitempty"`     // DraftResult.Note
	RawText          string        `json:"raw_text"`                // the model's whole streamed text
	Check            *DraftCheck   `json:"check,omitempty"`         // strict schema + quality rubric on RawText
	SchemaValid      bool          `json:"schema_valid"`            // copy of Check.SchemaValid
	FirstIdeaMs      float64       `json:"first_idea_ms,omitempty"` // when the first idea reached the sink
	LatencyMs        float64       `json:"latency_ms"`
	CostUSD          *float64      `json:"cost_usd"`
	PromptTokens     int64         `json:"prompt_tokens"`
	CompletionTokens int64         `json:"completion_tokens"`
	ReasoningTokens  int64         `json:"reasoning_tokens,omitempty"`
	ResolvedModel    string        `json:"resolved_model"`
	Provider         string        `json:"provider,omitempty"`
	HTTPStatus       int           `json:"http_status,omitempty"`
	Error            string        `json:"error,omitempty"`
}

// sseContent rebuilds the streamed text from an OpenRouter chat SSE body.
func sseContent(raw []byte) (text, provider string, reasoning int64) {
	var b strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			continue
		}
		var ev struct {
			Provider string `json:"provider"`
			Choices  []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				Details *struct {
					Reasoning int64 `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		if ev.Provider != "" {
			provider = ev.Provider
		}
		if len(ev.Choices) > 0 {
			b.WriteString(ev.Choices[0].Delta.Content)
		}
		if ev.Usage != nil && ev.Usage.Details != nil {
			reasoning = ev.Usage.Details.Reasoning
		}
	}
	return b.String(), provider, reasoning
}

// DraftingRow is one model's drafting metrics.
type DraftingRow struct {
	Model           string  `json:"model"`
	Cases           int     `json:"cases"`
	OK              int     `json:"ok"`
	Errors          int     `json:"errors"`
	SchemaValidPct  float64 `json:"schema_valid_pct"`
	KindMatchPct    float64 `json:"head_kind_matches_pct"`
	QualityMean     float64 `json:"quality_mean"`
	StandalonePct   float64 `json:"standalone_questions_pct"`
	NonYesNoPct     float64 `json:"non_yes_no_pct"`
	ShortAnswerPct  float64 `json:"answers_le_25_words_pct"`
	NonDuplicatePct float64 `json:"ideas_not_duplicating_knows_pct"`
	LinkExactPct    float64 `json:"links_exact_pct"`
	LibraryLinks    int     `json:"library_links_exact"`
	MalformedLines  int     `json:"malformed_lines"`
	MeanIdeas       float64 `json:"mean_ideas"`
	FirstIdeaP50Ms  float64 `json:"first_idea_p50_ms"`
	USDPer100       float64 `json:"usd_per_100_drafts"`
	Stats
}

// interleave orders cases round-robin by category (first-appearance
// order), so a spend stop or --limit still covers notes, topics and books.
func interleave(cases []DraftingCase) []DraftingCase {
	var order []string
	groups := map[string][]DraftingCase{}
	for _, c := range cases {
		if groups[c.Category] == nil {
			order = append(order, c.Category)
		}
		groups[c.Category] = append(groups[c.Category], c)
	}
	var out []DraftingCase
	for len(out) < len(cases) {
		for _, k := range order {
			if len(groups[k]) > 0 {
				out = append(out, groups[k][0])
				groups[k] = groups[k][1:]
			}
		}
	}
	return out
}

func (r *Runner) draftOnce(ctx context.Context, m string, c DraftingCase) DraftingRecord {
	rec := DraftingRecord{Surface: "drafting", CaseID: c.ID, Category: c.Category, Model: m}
	t, err := r.budget.Reserve(r.estimate("drafting", m, 0))
	if err != nil {
		rec.Status, rec.Error = "skipped", err.Error()
		return rec
	}
	rc := newRecorder()
	cl, err := ai.NewClaudeWithRouter("", r.cfg.Key, m, m)
	if err != nil {
		rec.Status, rec.Error = "error", errText(err, r.cfg.Key)
		t.Settle(nil)
		return rec
	}
	cl.BaseURL = r.cfg.Base
	cl.HTTP = rc.client(5 * time.Minute)
	var once sync.Once
	start := time.Now()
	sink := ai.DraftSink{
		Head: func(h ai.DraftHead) error { rec.ProdHead = &h; return nil },
		Idea: func(ai.DraftIdea) error {
			once.Do(func() { rec.FirstIdeaMs = ms(time.Since(start)) })
			return nil
		},
	}
	res, use, derr := cl.Draft(ctx, ai.DraftInput{Query: c.Query, Learner: ai.Learner{About: c.Learner.About, Knows: c.Learner.Knows, Level: c.Learner.Level}}, sink)
	rec.LatencyMs = ms(time.Since(start))
	raw := rc.buf.Bytes()
	rec.CostUSD = use.CostUSD
	if rec.CostUSD == nil && rc.status != 200 {
		rec.CostUSD = parseUsage(raw).Cost
	}
	t.Settle(rec.CostUSD)
	r.observe("drafting", m, rec.CostUSD)
	rec.PromptTokens, rec.CompletionTokens, rec.ResolvedModel, rec.HTTPStatus = use.InputTokens, use.OutputTokens, use.Model, rc.status
	rec.ProdIdeas, rec.ProdNote = res.Ideas, res.Note
	if rc.status == 200 {
		rec.RawText, rec.Provider, rec.ReasoningTokens = sseContent(raw)
	}
	if derr != nil {
		rec.Status, rec.Error = "error", errText(derr, r.cfg.Key)
		if rc.status != 0 && rc.status != 200 {
			rec.Error += " — " + errorSnippet(rc.status, raw, r.cfg.Key)
		}
	} else {
		rec.Status = "ok"
	}
	if rec.RawText != "" {
		chk := CheckDraft(c, rec.RawText)
		rec.Check, rec.SchemaValid = &chk, chk.SchemaValid && derr == nil
	}
	return rec
}

func (r *Runner) runDrafting(ctx context.Context) map[string]any {
	cases := limit(interleave(r.data.Drafting), r.cfg.Limit)
	models := r.models("drafting")
	recs := map[string]map[string]DraftingRecord{}
	accs := map[string]*statAcc{}
	pass := func(models []string) {
		for _, m := range models {
			accs[m], recs[m] = &statAcc{}, map[string]DraftingRecord{}
		}
		matrix(len(cases), models, func(i int, m string) {
			c := cases[i]
			rec := r.draftOnce(ctx, m, c)
			r.write("drafting", rec)
			r.mu.Lock()
			recs[m][c.ID] = rec
			if rec.Status == "skipped" {
				accs[m].skipped++
			} else {
				accs[m].add(rec.LatencyMs, rec.CostUSD)
			}
			r.mu.Unlock()
			q := -1
			if rec.Check != nil {
				q = rec.Check.QualityScore
			}
			r.logf("drafting %-28s %s %-7s valid=%-5v quality=%3d ideas=%d %8.0fms cost=%s %s", m, c.ID, rec.Status, rec.SchemaValid, q, rec.ProdIdeas, rec.LatencyMs, costStr(rec.CostUSD), rec.Error)
		})
	}
	pass(models)
	// Optional models: only when the remaining budget covers a full pass
	// 1.5x, estimated from the main pass's mean tokens at catalog prices.
	var inTok, outTok, n float64
	for _, m := range models {
		for _, rec := range recs[m] {
			if rec.Status == "ok" {
				inTok, outTok, n = inTok+float64(rec.PromptTokens), outTok+float64(rec.CompletionTokens), n+1
			}
		}
	}
	var optional []string
	for _, m := range r.models("drafting-optional") {
		p := r.cat.Verify(m).Price
		est := 0.0
		if n > 0 {
			est = float64(len(cases)) * (p.Prompt*inTok/n + p.Completion*outTok/n)
		}
		st := r.budget.State()
		remaining := st.MaxUSD - st.ChargedUSD
		if n == 0 || est <= 0 || remaining < 1.5*est {
			r.note("drafting optional %s not run: remaining budget $%.4f < 1.5 x estimated full pass $%.4f (%d cases at mean %.0f in / %.0f out tokens from the main pass, catalog prices)", m, remaining, est, len(cases), inTok/max(n, 1), outTok/max(n, 1))
			continue
		}
		r.note("drafting optional %s run: remaining budget $%.4f >= 1.5 x estimated full pass $%.4f", m, remaining, est)
		optional = append(optional, m)
	}
	if len(optional) > 0 {
		pass(optional)
		models = append(models, optional...)
	}
	var rows []DraftingRow
	for _, m := range models {
		row := DraftingRow{Model: m}
		var valid, kind, ok int
		var q, stand, yn, short, nodup, link, ideas float64
		var first []float64
		for _, c := range cases {
			rec, has := recs[m][c.ID]
			if !has || rec.Status == "skipped" {
				continue
			}
			row.Cases++
			if rec.Status != "ok" || rec.Check == nil {
				row.Errors++
				continue
			}
			ok++
			ch := rec.Check
			if rec.SchemaValid {
				valid++
			}
			if ch.KindMatches {
				kind++
			}
			q += float64(ch.QualityScore)
			stand, yn, short, nodup, link = stand+ch.StandaloneRate, yn+ch.NonYesNoRate, short+ch.ShortAnswerRate, nodup+ch.NonDuplicateRate, link+ch.LinkValidRate
			ideas += float64(ch.IdeaCount)
			row.LibraryLinks += ch.LibraryLinks
			row.MalformedLines += ch.MalformedLines
			if rec.FirstIdeaMs > 0 {
				first = append(first, rec.FirstIdeaMs)
			}
		}
		row.OK = ok
		row.SchemaValidPct, row.KindMatchPct = pct(valid, row.Cases), pct(kind, row.Cases)
		if ok > 0 {
			f := float64(ok)
			r1 := func(x float64) float64 { return math.Round(1000*x/f) / 10 }
			row.QualityMean = math.Round(10*q/f) / 10
			row.StandalonePct, row.NonYesNoPct, row.ShortAnswerPct, row.NonDuplicatePct, row.LinkExactPct = r1(stand), r1(yn), r1(short), r1(nodup), r1(link)
			row.MeanIdeas = math.Round(10*ideas/f) / 10
		}
		row.FirstIdeaP50Ms = Percentile(first, .5)
		row.Stats = accs[m].stats()
		row.USDPer100 = math.Round(row.MeanCostUSD*100*1e6) / 1e6
		rows = append(rows, row)
	}
	for _, c := range cases {
		outs := map[string]BlindOutput{}
		for _, m := range models {
			rec, has := recs[m][c.ID]
			switch {
			case !has:
			case rec.Check != nil && rec.Status == "ok":
				outs[m] = BlindOutput{Status: "ok", Output: map[string]any{"head": map[string]string{"kind": rec.Check.HeadKind, "title": rec.Check.HeadTitle}, "ideas": blindIdeas(rec.Check.Ideas), "note": rec.ProdNote}}
			default:
				outs[m] = BlindOutput{Status: rec.Status + ": no outline"}
			}
		}
		set, assign := BuildBlind(r.cfg.Seed, "drafting", c.ID, map[string]any{"query": c.Query, "learner": c.Learner, "category": c.Category}, models, outs)
		r.mu.Lock()
		r.blind = append(r.blind, set)
		r.key.Sets["drafting/"+c.ID] = assign
		r.mu.Unlock()
	}
	return map[string]any{"dataset": map[string]any{"cases": len(cases), "file": "evals/datasets/drafting.jsonl"},
		"adapter": "scry/internal/ai.NewClaudeWithRouter -> Claude.Draft (OpenRouter streamed chat completions, production system prompt, learner framing, 16000 max tokens, draftLines NDJSON parser)",
		"models":  rows}
}

func blindIdeas(ideas []OutlineIdea) []OutlineIdea {
	out := make([]OutlineIdea, len(ideas))
	for i, idea := range ideas {
		idea.Issues = nil
		out[i] = idea
	}
	return out
}
