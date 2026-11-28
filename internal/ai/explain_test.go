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

// [S139-2] An Almost that gives the first sentence of a two-sentence answer:
// the test explainer's Rebuild sentence names the sentence the learner left
// out ("Velocity also includes direction"), never the learner's own phrase
// set against itself.
func TestFakeExplanation_AlmostNamesTheMissingSentence(t *testing.T) {
	const expected = "Both describe how fast something moves. Velocity also includes direction."
	for _, c := range []struct {
		v      srs.Verdict
		answer string
		want   string
	}{
		{srs.Close, "Both describe how fast something moves.",
			"Rebuild: the card's point is “Velocity also includes direction”, not “Both describe how fast something moves.”."},
		{srs.Close, "both DESCRIBE how fast something moves", // case and stops do not matter
			"Rebuild: the card's point is “Velocity also includes direction”, not “both DESCRIBE how fast something moves”."},
		{srs.Missed, "Both describe how fast something moves.",
			"Rebuild: the card's point is “Velocity also includes direction”, not “Both describe how fast something moves.”."},
		{srs.Missed, "the price", // nothing covered: the first sentence, as before
			"Rebuild: the card's point is “Both describe how fast something moves”, not “the price”."},
		{srs.Close, "Velocity has direction", // the second sentence is not fully covered either; the first one is named
			"Rebuild: the card's point is “Both describe how fast something moves”, not “Velocity has direction”."},
		{srs.Close, "Both describe how fast something moves.\nVelocity also includes direction.", // everything covered: no self-contrast
			"Rebuild: the card's point is “Both describe how fast something moves. Velocity also includes direction”."},
	} {
		text := FakeExplanation(ExplainInput{Question: "How is velocity different from speed?", Expected: expected, Answer: c.answer, Verdict: c.v})
		_, last, ok := strings.Cut(text, ". Rebuild:")
		last = "Rebuild:" + last
		if !ok || last != c.want || strings.Count(text, "Rebuild:") != 1 || utf8.RuneCountInString(text) > ExplainMaxChars || !strings.HasPrefix(text, "What differed:") {
			t.Errorf("%s %q:\n got %q\nwant %q", c.v, c.answer, text, c.want)
		}
		// Never the same phrase on both sides of ", not".
		if point, learner, two := strings.Cut(strings.TrimPrefix(last, "Rebuild: the card's point is "), ", not "); two && normalized(point) == normalized(learner) {
			t.Errorf("%s %q: contrasts a phrase with itself: %q", c.v, c.answer, last)
		}
	}
	// The exact Cold Critic repro: it names "direction".
	text := FakeExplanation(ExplainInput{Question: "How is velocity different from speed?", Expected: expected, Answer: "Both describe how fast something moves.", Verdict: srs.Close})
	if !strings.Contains(text, "direction") || strings.Contains(text, "point is “Both describe how fast something moves”, not “Both describe how fast something moves.”") {
		t.Fatalf("Almost repro: %q", text)
	}
}
