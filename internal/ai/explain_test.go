package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"scry/internal/srs"
)

// The on-demand explanation is one bounded, non-streaming request after the
// verdict: low effort, capped output tokens, the verdict given as data, the
// key sent only as a header, and a refusal or error is an error (the app
// then shows the criterion sentence).
func TestExplain_ClaudeRequestIsBoundedAndParsed(t *testing.T) {
	var got map[string]any
	reply := `{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":""},{"type":"text","text":"You named the letter, but the card asks for its NATO word."}],"usage":{"input_tokens":120,"output_tokens":30}}`
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		if r.Header.Get("x-api-key") != "stand-in" || strings.Contains(string(raw), "stand-in") {
			t.Error("key must travel only in the header")
		}
		w.Write([]byte(reply))
	}))
	defer srv.Close()
	c := NewClaude("stand-in", "")
	c.BaseURL = srv.URL
	text, use, err := c.Explain(context.Background(), ExplainInput{Question: "NATO word for A?", Expected: "Alfa", Answer: "A", Verdict: srs.Missed})
	if err != nil || text != "You named the letter, but the card asks for its NATO word." || use.Model != DefaultExplainModel || use.OutputTokens != 30 {
		t.Fatalf("%q %+v %v", text, use, err)
	}
	if got["model"] != DefaultExplainModel || got["max_tokens"] != float64(ExplainMaxTokens) || got["stream"] != nil {
		t.Fatalf("request: %v", got)
	}
	if oc, _ := got["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Fatalf("effort: %v", got["output_config"])
	}
	msg := got["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(msg, "<grade>incorrect</grade>") || !strings.Contains(msg, "<learner_answer>A</learner_answer>") {
		t.Fatalf("content: %s", msg)
	}

	reply = `{"stop_reason":"refusal","content":[],"usage":{"input_tokens":10,"output_tokens":0}}`
	if _, _, err := c.Explain(context.Background(), ExplainInput{Verdict: srs.Close}); err == nil {
		t.Fatal("a refusal must fall back")
	}
	reply = `{"stop_reason":"end_turn","content":[{"type":"text","text":"` + strings.Repeat("word ", 200) + `"}]}`
	text, _, err = c.Explain(context.Background(), ExplainInput{Verdict: srs.Close})
	if err != nil || utf8.RuneCountInString(text) > ExplainMaxChars || !strings.HasSuffix(text, "…") {
		t.Fatalf("unbounded output: %d %v", utf8.RuneCountInString(text), err)
	}
	if _, _, err := c.Explain(context.Background(), ExplainInput{Answer: strings.Repeat("x", ExplainInputChars)}); err == nil {
		t.Fatal("oversized input must not be sent")
	}
	if calls != 3 {
		t.Fatalf("%d provider calls, want 3 (no retries, oversized input never sent)", calls)
	}
}

func TestExplain_CriterionSentencesAreTheJudgeCriteria(t *testing.T) {
	for _, o := range judgeOptions {
		if o.Verdict != "" && CriterionSentence(o.Verdict) != o.Reason {
			t.Errorf("%s: %q", o.Verdict, CriterionSentence(o.Verdict))
		}
	}
	f := &Fake{Verdicts: []srs.Verdict{srs.Close, srs.Missed}}
	for _, want := range []srs.Verdict{srs.Close, srs.Missed, srs.Close} {
		j, _, _ := f.Judge(context.Background(), JudgeInput{Answer: "anything"})
		if j.Verdict != want || j.Reason != CriterionSentence(want) {
			t.Fatalf("scripted %s: %+v", want, j)
		}
	}
}
