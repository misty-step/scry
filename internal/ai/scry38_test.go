package ai

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"scry/internal/srs"
)

// Phaedrus's production repro for SCRY-38 (scry.mistystep.io item 27,
// 2026-11-25 22:34:04Z and 22:34:08Z, release 8a14e49): this exact question
// and answer drew "Jev decision unavailable or ambiguous" and an HTTP 503
// twice.
const (
	scry38Question = "What is the testing effect (retrieval practice) in learning science, and why does it outperform rereading?"
	scry38Answer   = "the testing effect states that it is better for recall to quiz a student and have them actually try to answer a question rather than just reread the material because it is challenging, it forces you to dig through your mind and find the material and the understanding yourself rather than just let the words on the page pass over you again"
	scry38Expected = "Recalling information from memory strengthens long-term retention more than rereading, because effortful retrieval reinforces and reorganizes the memory trace and reveals gaps, while rereading builds only familiarity."
	// The body typesafe/jev-1.14 returned live on OpenRouter for this
	// state on 2026-11-25 (id redacted): a valid, metered decision split
	// between the two passing grades. Release 8a14e49 had nowhere to send
	// it, so the learner got a 503, deterministically, on every retry.
	scry38LiveResponse = `{"model":"typesafe/jev-1.14-20261014","answers":{"judgment":{"type":"choice","choice":"correct","probabilities":{"almost correct":0.36,"correct":0.64,"incorrect":0,"none_of_these":0},"confidence":0.52}},"usage":{"input_tokens":520,"output_tokens":51,"cost":0.00002184,"is_byok":false,"cost_details":{"upstream_inference_cost":0.00002184,"upstream_inference_prompt_cost":0.00002184,"upstream_inference_completions_cost":0}},"id":"gen-dec-redacted","provider":"TypeSafe"}`
)

var scry38Input = JudgeInput{Question: scry38Question, Expected: scry38Expected, Answer: scry38Answer}

// The recorded decision, through the real Jev adapter and the real chain
// ([S113-1]): Jev classes the split uncertain (readable, metered, below its
// probability bar), never unavailable, so it is not shopped to another
// decisions model; the escalation LLM is asked once with the same question
// and answer, and its verdict is the grade. The answer is never a dead end.
func TestSCRY38_PhaedrusSplitDecisionEscalatesAndIsGraded(t *testing.T) {
	for _, tc := range []struct {
		llm  string
		want srs.Verdict
	}{{"correct", srs.Right}, {"almost correct", srs.Close}, {"incorrect", srs.Missed}} {
		t.Run("the LLM says "+tc.llm, func(t *testing.T) {
			w := newS113World(t, true)
			w.decide[DefaultOpenRouterJudgeModel] = func(rw http.ResponseWriter) { rw.Write([]byte(scry38LiveResponse)) }
			w.escalate = func(rw http.ResponseWriter) {
				json.NewEncoder(rw).Encode(map[string]any{"model": DefaultJudgeEscalationModel, "choices": []map[string]any{{"message": map[string]any{"content": `{"verdict":"` + tc.llm + `"}`}, "finish_reason": "stop"}},
					"usage": map[string]any{"prompt_tokens": 610, "completion_tokens": 9, "cost": 0.00065}})
			}
			// Jev alone: uncertain, and still metered (the try is paid for).
			_, use, err := w.chain.Decisions[0].Judge(context.Background(), scry38Input)
			if !errors.Is(err, ErrDecisionUncertain) || errors.Is(err, ErrDecisionUnavailable) || !strings.Contains(err.Error(), "probability below its bar") {
				t.Fatalf("Jev classed the recorded split as %v, want uncertain (probability below its bar)", err)
			}
			if use.CostMicros == nil || *use.CostMicros != 22 || use.Model != "typesafe/jev-1.14-20261014" {
				t.Fatalf("usage %+v", use)
			}
			// The whole chain: one verdict, from the LLM.
			j, use, err := w.chain.Judge(context.Background(), scry38Input)
			if err != nil {
				t.Fatalf("the long conceptual answer got no verdict: %v", err)
			}
			if j.Verdict != tc.want || j.Reason != CriterionSentence(tc.want) {
				t.Fatalf("judgment %+v, want %s", j, tc.want)
			}
			w.wantCalls(2, 0, 0, 1) // Jev twice (once alone above), no fallback, the LLM once
			att := attempts(use)
			if len(att) != 2 || att[0].Step != "primary" || att[0].Outcome != OutcomeUncertain || att[1].Step != "escalation" || att[1].Outcome != OutcomeAnswered || use.Model != DefaultJudgeEscalationModel {
				t.Fatalf("attempts %+v, usage %+v", att, use)
			}
			// The LLM got the exact production question and answer.
			msgs, _ := w.llmBody["messages"].([]any)
			if len(msgs) != 2 {
				t.Fatalf("escalation request %+v", w.llmBody)
			}
			var state struct {
				Prompt   string   `json:"card_prompt"`
				Accepted []string `json:"accepted_answers"`
				Answer   string   `json:"learner_answer"`
			}
			user, _ := msgs[1].(map[string]any)["content"].(string)
			if err := json.Unmarshal([]byte(user), &state); err != nil || state.Prompt != scry38Question || state.Answer != scry38Answer || len(state.Accepted) != 1 || state.Accepted[0] != scry38Expected {
				t.Fatalf("escalation state %q (%v)", user, err)
			}
			// The decision line names the fixed choice; nothing logs the answer.
			logs := w.logs.String()
			if !strings.Contains(logs, `"choice":"correct"`) || !strings.Contains(logs, `"outcome":"uncertain"`) || !strings.Contains(logs, `"escalated_because":"uncertain"`) {
				t.Fatalf("logs do not say what happened: %s", logs)
			}
			if strings.Contains(logs, "dig through your mind") || strings.Contains(logs, s113Key) {
				t.Fatalf("logs leak the answer or the key: %s", logs)
			}
		})
	}
}

func TestDecide_GradesAsChosenOrSaysWhichKindOfNoVerdict(t *testing.T) {
	f := func(x float64) *float64 { return &x }
	p := func(c, a, i, n float64) map[string]float64 {
		return map[string]float64{"correct": c, "almost correct": a, "incorrect": i, "none_of_these": n}
	}
	uncertain, unreadable := ErrDecisionUncertain, ErrDecisionUnavailable
	for _, tc := range []struct {
		name   string
		choice string
		conf   *float64
		probs  map[string]float64
		want   srs.Verdict // "" = no verdict
		class  error
		gate   string
	}{
		{"SCRY-38 live split", "correct", f(0.52), p(0.64, 0.36, 0, 0), "", uncertain, "probability below its bar"},
		{"SCRY-38 retry split", "correct", f(0.51), p(0.63, 0.37, 0, 0), "", uncertain, "probability below its bar"},
		{"right-versus-wrong split", "correct", nil, p(0.5, 0, 0.5, 0), "", uncertain, "probability below its bar"},
		{"almost-versus-incorrect split", "almost correct", nil, p(0, 0.6, 0.4, 0), "", uncertain, "probability below its bar"},
		{"a split is never graded at another grade", "almost correct", nil, p(0.79, 0.21, 0, 0), "", uncertain, "probability below its bar"},
		{"unclassifiable mass", "correct", nil, p(0.5, 0.2, 0.05, 0.25), "", uncertain, "probability below its bar"},
		{"confident correct", "correct", f(0.77), p(0.83, 0.17, 0, 0), srs.Right, nil, ""},
		{"exactly at the bar", "correct", f(0.73), p(0.8, 0.2, 0, 0), srs.Right, nil, ""},
		{"confident almost correct", "almost correct", nil, p(0.05, 0.9, 0.05, 0), srs.Close, nil, ""},
		{"confident incorrect", "incorrect", nil, p(0.1, 0.1, 0.8, 0), srs.Missed, nil, ""},
		{"a strong distribution with low confidence", "correct", f(0.3), p(0.9, 0.1, 0, 0), "", uncertain, "confidence below its bar"},
		{"no distribution, confident", "correct", f(0.9), nil, srs.Right, nil, ""},
		{"no distribution, no confidence", "incorrect", nil, nil, srs.Missed, nil, ""},
		{"no distribution, low confidence", "correct", f(0.4), nil, "", uncertain, "confidence below its bar"},
		{"none_of_these chosen", "none_of_these", nil, p(0, 0, 0, 1), "", uncertain, "chose none of these"},
		{"none_of_these chosen against the mass", "none_of_these", nil, p(0.6, 0, 0, 0.4), "", uncertain, "chose none of these"},
		{"unknown choice", "invented", nil, p(1, 0, 0, 0), "", unreadable, "unreadable"},
		{"missing choice", "", nil, nil, "", unreadable, "unreadable"},
		{"partial distribution", "correct", nil, map[string]float64{"correct": 1}, "", unreadable, "unreadable"},
		{"distribution does not sum to one", "correct", nil, p(0.8, 0, 0, 0.1), "", unreadable, "unreadable"},
		{"negative probability", "correct", nil, p(1.2, -0.2, 0, 0), "", unreadable, "unreadable"},
		{"confidence above one", "correct", f(1.2), p(1, 0, 0, 0), "", unreadable, "unreadable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decide(tc.choice, tc.conf, tc.probs)
			if got != tc.want || (err == nil) != (tc.want != "") {
				t.Fatalf("Decide = %q, %v; want %q", got, err, tc.want)
			}
			if err == nil {
				return
			}
			other := ErrDecisionUnavailable
			if tc.class == ErrDecisionUnavailable {
				other = ErrDecisionUncertain
			}
			if !errors.Is(err, tc.class) || errors.Is(err, other) || !strings.Contains(err.Error(), tc.gate) {
				t.Fatalf("Decide error %q, want class %q naming %q", err, tc.class, tc.gate)
			}
		})
	}
}

// The invariant SCRY-38 lacked, over 20,000 random decisions: every
// well-formed decision (a known choice, a complete distribution summing to
// one, a confidence in range or none) is either graded exactly as the model
// chose, and only when the choice clears its bars, or is uncertain, which
// the chain escalates to the LLM. It is never "unavailable" (a dead end, or
// a second decisions model), and Scry never awards a grade the model did
// not choose with certainty.
func TestDecide_EveryWellFormedDecisionIsGradedAsChosenOrEscalates(t *testing.T) {
	rng := rand.New(rand.NewSource(38))
	graded, escalated := 0, 0
	for n := 0; n < 20000; n++ {
		// A random distribution on a 0.01 grid, like Jev's.
		cuts := []int{rng.Intn(101), rng.Intn(101), rng.Intn(101)}
		for i := range cuts {
			for k := i + 1; k < len(cuts); k++ {
				if cuts[k] < cuts[i] {
					cuts[i], cuts[k] = cuts[k], cuts[i]
				}
			}
		}
		if rng.Intn(3) == 0 { // often no unclassifiable mass at all
			cuts[2] = 100
		}
		if rng.Intn(4) == 0 { // often a confident winner
			cuts[0], cuts[1] = cuts[0]/10, 80+rng.Intn(21)
			if cuts[2] < cuts[1] {
				cuts[2] = cuts[1]
			}
		}
		ps := []float64{float64(cuts[0]) / 100, float64(cuts[1]-cuts[0]) / 100, float64(cuts[2]-cuts[1]) / 100, float64(100-cuts[2]) / 100}
		probs := map[string]float64{}
		for i, o := range judgeOptions {
			probs[o.ID] = ps[i]
		}
		ci := rng.Intn(len(judgeOptions))
		if rng.Intn(2) == 0 { // usually the model chooses its own top label
			for i := range ps {
				if ps[i] > ps[ci] {
					ci = i
				}
			}
		}
		var conf *float64
		if rng.Intn(2) == 0 {
			c := float64(rng.Intn(101)) / 100
			conf = &c
		}
		got, err := Decide(judgeOptions[ci].ID, conf, probs)
		clears := judgeOptions[ci].Verdict != "" && ps[ci] >= minJevProbability && (conf == nil || *conf >= minJevConfidence)
		switch {
		case err == nil:
			graded++
			if !clears || got != judgeOptions[ci].Verdict {
				t.Fatalf("graded %q for choice %q %v conf %v", got, judgeOptions[ci].ID, ps, conf)
			}
		case errors.Is(err, ErrDecisionUncertain) && !errors.Is(err, ErrDecisionUnavailable):
			escalated++
			if clears || got != "" {
				t.Fatalf("a decision that clears its bars was not graded: %q %v conf %v: %v", judgeOptions[ci].ID, ps, conf, err)
			}
		default:
			t.Fatalf("a well-formed decision was a dead end: %q %v conf %v: %v", judgeOptions[ci].ID, ps, conf, err)
		}
	}
	// The generator must exercise both outcomes, or the property is vacuous.
	if graded < 1000 || escalated < 1000 {
		t.Fatalf("graded %d, escalated %d of 20000", graded, escalated)
	}
}
