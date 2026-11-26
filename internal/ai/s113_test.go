package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"scry/internal/srs"
)

// The [S113-1] grading chain, against local stand-ins for OpenRouter's
// Decisions API (Jev and the fallback decisions models) and its chat
// completions (the escalation LLM). No real provider is called.

const (
	s113Key      = "local-s113-secret"
	s113Answer   = "Photosynthesis turns light, water and carbon dioxide into sugar.\nOxygen is released as a by-product."
	s113Fallback = "openai/gpt-6-luna-decisions"
	s113Second   = "google/gemini-decision-1"
)

var s113Input = JudgeInput{Question: "What does photosynthesis do?", Expected: "It makes sugar from light, water and CO2, releasing oxygen", Answer: s113Answer}

// s113World is a Decisions stand-in (one behaviour per model) and a chat
// completions stand-in for the escalation, counting calls by model.
type s113World struct {
	t        *testing.T
	mu       sync.Mutex
	calls    map[string]int
	decide   map[string]func(http.ResponseWriter) // by requested model; default answers "correct"
	escalate func(http.ResponseWriter)            // default answers "almost correct"
	llmBody  map[string]any
	logs     bytes.Buffer
	chain    *Chain
}

func decisionsReply(w http.ResponseWriter, b map[string]any) { json.NewEncoder(w).Encode(b) }

func withProbabilities(choice string, p map[string]float64, confidence float64) map[string]any {
	b := jevResponse(choice, confidence)
	b["answers"].(map[string]any)["judgment"].(map[string]any)["probabilities"] = p
	return b
}

// The decisions replies a test can force, one per kind of failure.
var s113Decisions = map[string]func(http.ResponseWriter){
	"unavailable": func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
	"errored": func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":500}}`))
	},
	"unparseable":    func(w http.ResponseWriter) { w.Write([]byte(`{"model":"x","answers":`)) },
	"unknown-choice": func(w http.ResponseWriter) { decisionsReply(w, jevResponse("invented", 1)) },
	"low-confidence": func(w http.ResponseWriter) { decisionsReply(w, jevResponse("correct", 0.3)) },
	"low-probability": func(w http.ResponseWriter) {
		decisionsReply(w, withProbabilities("correct", map[string]float64{"correct": 0.7, "almost correct": 0.1, "incorrect": 0.1, "none_of_these": 0.1}, 0.9))
	},
	// With the 0.8 probability bar and probabilities summing to 1, a thin
	// margin always comes with a low winner; either way it is uncertain.
	"thin-margin": func(w http.ResponseWriter) {
		decisionsReply(w, withProbabilities("correct", map[string]float64{"correct": 0.45, "almost correct": 0.4, "incorrect": 0.1, "none_of_these": 0.05}, 0.9))
	},
	"none": func(w http.ResponseWriter) { decisionsReply(w, jevResponse("none_of_these", 1)) },
}

func newS113World(t *testing.T, escalation bool) *s113World {
	t.Helper()
	w := &s113World{t: t, calls: map[string]int{}, decide: map[string]func(http.ResponseWriter){}}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s113Key {
			t.Errorf("wrong auth on %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bad body: %v", err)
		}
		model, _ := body["model"].(string)
		w.mu.Lock()
		w.calls[model]++
		w.mu.Unlock()
		switch r.URL.Path {
		case "/api/alpha/decisions":
			if f := w.decide[model]; f != nil {
				f(rw)
				return
			}
			b := jevResponse("correct", 1)
			b["model"] = model + "-20261101"
			decisionsReply(rw, b)
		case "/api/v1/chat/completions":
			w.mu.Lock()
			w.llmBody = body
			w.mu.Unlock()
			if w.escalate != nil {
				w.escalate(rw)
				return
			}
			json.NewEncoder(rw).Encode(map[string]any{"model": model, "choices": []map[string]any{{"message": map[string]any{"content": `{"verdict":"almost correct"}`}, "finish_reason": "stop"}},
				"usage": map[string]any{"prompt_tokens": 400, "completion_tokens": 8, "cost": 0.0004}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	log := slog.New(slog.NewJSONHandler(&w.logs, nil))
	step := func(model string) *Jev {
		return &Jev{Model: model, APIKey: s113Key, OpenRouter: true, BaseURL: srv.URL, Log: log}
	}
	w.chain = &Chain{Decisions: []GradingStep{step(""), step(s113Fallback), step(s113Second)}, Log: log}
	if escalation {
		llm, err := NewClaudeWithRouter("", s113Key, "", "")
		if err != nil {
			t.Fatal(err)
		}
		llm.BaseURL = srv.URL
		w.chain.Escalation = &LLMJudge{Client: llm, Model: DefaultJudgeEscalationModel}
	}
	return w
}

func (w *s113World) count(model string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[model]
}

func (w *s113World) wantCalls(primary, fallback, second, escalation int) {
	w.t.Helper()
	got := []int{w.count(DefaultOpenRouterJudgeModel), w.count(s113Fallback), w.count(s113Second), w.count(DefaultJudgeEscalationModel)}
	if want := []int{primary, fallback, second, escalation}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		w.t.Fatalf("calls (primary, fallback 1, fallback 2, escalation) = %v, want %v", got, want)
	}
}

// noLeaks: each attempt is logged with its requested and resolved model,
// latency, cost and outcome, and never the answer, the question or a key.
func (w *s113World) noLeaks() {
	w.t.Helper()
	logs := w.logs.String()
	for _, s := range []string{"photosynthesis", "Photosynthesis", "Oxygen", "sugar", s113Key} {
		if strings.Contains(logs, s) {
			w.t.Fatalf("logs contain %q:\n%s", s, logs)
		}
	}
	for _, s := range []string{`"msg":"judge attempt"`, `"requested_model"`, `"resolved_model"`, `"latency_ms"`, `"cost_micros"`, `"outcome"`, `"msg":"judge chain"`} {
		if !strings.Contains(logs, s) {
			w.t.Fatalf("logs lack %s:\n%s", s, logs)
		}
	}
}

func attempts(u Usage) []Attempt {
	if u.Attempts == nil {
		return nil
	}
	return *u.Attempts
}

func TestStory_S113_1_PrimaryDecisionsModelAnswers(t *testing.T) {
	w := newS113World(t, true)
	j, use, err := w.chain.Judge(context.Background(), s113Input)
	if err != nil || j.Verdict != srs.Right || j.Reason != CriterionSentence(srs.Right) {
		t.Fatalf("%+v %v", j, err)
	}
	w.wantCalls(1, 0, 0, 0)
	if use.Model != "typesafe/jev-1.14-20261101" || use.Attempts != nil || use.CostMicros == nil {
		t.Fatalf("one attempt is reported as it came: %+v", use)
	}
	w.noLeaks()
}

func TestStory_S113_1_PrimaryUnavailableErroredOrUnparseable_NextDecisionsModelAnswers(t *testing.T) {
	for _, kind := range []string{"unavailable", "errored", "unparseable", "unknown-choice"} {
		t.Run(kind, func(t *testing.T) {
			w := newS113World(t, true)
			w.decide[DefaultOpenRouterJudgeModel] = s113Decisions[kind]
			j, use, err := w.chain.Judge(context.Background(), s113Input)
			if err != nil || j.Verdict != srs.Right || j.Reason != CriterionSentence(srs.Right) {
				t.Fatalf("%+v %v", j, err)
			}
			w.wantCalls(1, 1, 0, 0)
			at := attempts(use)
			if len(at) != 2 || at[0].Step != "primary" || at[0].Outcome != OutcomeUnavailable || at[1].Step != "fallback" || at[1].Outcome != OutcomeAnswered ||
				use.Model != s113Fallback+"-20261101" {
				t.Fatalf("usage %+v attempts %+v", use, at)
			}
			w.noLeaks()
		})
	}
	t.Run("the first fallback down too: the second answers", func(t *testing.T) {
		w := newS113World(t, true)
		w.decide[DefaultOpenRouterJudgeModel] = s113Decisions["unavailable"]
		w.decide[s113Fallback] = s113Decisions["unparseable"]
		if j, _, err := w.chain.Judge(context.Background(), s113Input); err != nil || j.Verdict != srs.Right {
			t.Fatalf("%+v %v", j, err)
		}
		w.wantCalls(1, 1, 1, 0)
	})
}

func TestStory_S113_1_EveryDecisionsModelUnavailable_EscalationLLMAnswers(t *testing.T) {
	w := newS113World(t, true)
	for _, m := range []string{DefaultOpenRouterJudgeModel, s113Fallback, s113Second} {
		w.decide[m] = s113Decisions["unavailable"]
	}
	j, use, err := w.chain.Judge(context.Background(), s113Input)
	if err != nil || j.Verdict != srs.Close || j.Reason != CriterionSentence(srs.Close) {
		t.Fatalf("%+v %v", j, err)
	}
	w.wantCalls(1, 1, 1, 1)
	if at := attempts(use); len(at) != 4 || at[3].Step != "escalation" || use.Model != DefaultJudgeEscalationModel || use.InputTokens != 400 {
		t.Fatalf("usage %+v", use)
	}
	// The same question, accepted answers and rubric, with a structured
	// three-way output; the answer stays untrusted data.
	b := w.llmBody
	rf, _ := b["response_format"].(map[string]any)
	schema, _ := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || schema["strict"] != true || b["max_tokens"] != float64(EscalationMaxTokens) {
		t.Fatalf("no structured output: %+v", b)
	}
	enum := schema["schema"].(map[string]any)["properties"].(map[string]any)["verdict"].(map[string]any)["enum"].([]any)
	if len(enum) != 3 || enum[0] != "correct" || enum[1] != "almost correct" || enum[2] != "incorrect" {
		t.Fatalf("not three-way: %v", enum)
	}
	msgs := b["messages"].([]any)
	system, user := msgs[0].(map[string]any)["content"].(string), msgs[1].(map[string]any)["content"].(string)
	for _, o := range judgeOptions {
		if o.Verdict != "" && !strings.Contains(system, o.Reason) {
			t.Fatalf("rubric lacks %q", o.Reason)
		}
	}
	var state struct {
		CardPrompt      string   `json:"card_prompt"`
		AcceptedAnswers []string `json:"accepted_answers"`
		LearnerAnswer   string   `json:"learner_answer"`
	}
	if err := json.Unmarshal([]byte(user), &state); err != nil || state.CardPrompt != s113Input.Question || len(state.AcceptedAnswers) != 1 ||
		state.AcceptedAnswers[0] != s113Input.Expected || state.LearnerAnswer != s113Answer || strings.Contains(system, s113Answer) {
		t.Fatalf("state %+v %v", state, err)
	}
	w.noLeaks()
}

func TestStory_S113_1_LowCertaintyGoesStraightToTheLLMAndIsNeverShopped(t *testing.T) {
	for _, kind := range []string{"low-confidence", "low-probability", "thin-margin", "none"} {
		t.Run(kind, func(t *testing.T) {
			w := newS113World(t, true)
			w.decide[DefaultOpenRouterJudgeModel] = s113Decisions[kind]
			j, use, err := w.chain.Judge(context.Background(), s113Input)
			if err != nil || j.Verdict != srs.Close || j.Reason != CriterionSentence(srs.Close) {
				t.Fatalf("%+v %v", j, err)
			}
			w.wantCalls(1, 0, 0, 1) // no secondary decisions model is asked
			if at := attempts(use); len(at) != 2 || at[0].Outcome != OutcomeUncertain || at[1].Step != "escalation" {
				t.Fatalf("attempts %+v", at)
			}
			if !strings.Contains(w.logs.String(), `"escalated_because":"uncertain"`) {
				t.Fatalf("logs %s", w.logs.String())
			}
			w.noLeaks()
		})
	}
	t.Run("a fallback that is uncertain escalates too", func(t *testing.T) {
		w := newS113World(t, true)
		w.decide[DefaultOpenRouterJudgeModel] = s113Decisions["unavailable"]
		w.decide[s113Fallback] = s113Decisions["none"]
		if j, _, err := w.chain.Judge(context.Background(), s113Input); err != nil || j.Verdict != srs.Close {
			t.Fatalf("%+v %v", j, err)
		}
		w.wantCalls(1, 1, 0, 1)
	})
}

func TestStory_S113_1_EscalationFailingTooIsNoVerdict(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision string
		llm      func(http.ResponseWriter)
		calls    []int
	}{
		{"uncertain then the LLM errors", "none", func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) }, []int{1, 0, 0, 1}},
		{"uncertain then the LLM is unreadable", "low-confidence", func(w http.ResponseWriter) {
			json.NewEncoder(w).Encode(map[string]any{"model": "m", "choices": []map[string]any{{"message": map[string]any{"content": "Correct!"}, "finish_reason": "stop"}},
				"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "cost": 0.000001}})
		}, []int{1, 0, 0, 1}},
		{"all unavailable then the LLM refuses", "unavailable", func(w http.ResponseWriter) {
			json.NewEncoder(w).Encode(map[string]any{"model": "m", "choices": []map[string]any{{"message": map[string]any{"content": ""}, "finish_reason": "content_filter"}},
				"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 0, "cost": 0}})
		}, []int{1, 1, 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newS113World(t, true)
			for _, m := range []string{DefaultOpenRouterJudgeModel, s113Fallback, s113Second} {
				w.decide[m] = s113Decisions[tc.decision]
			}
			w.escalate = tc.llm
			j, _, err := w.chain.Judge(context.Background(), s113Input)
			if !errors.Is(err, ErrNoVerdict) || j.Verdict != "" {
				t.Fatalf("%+v %v", j, err)
			}
			w.wantCalls(tc.calls[0], tc.calls[1], tc.calls[2], tc.calls[3])
			w.noLeaks()
		})
	}
	t.Run("no escalation configured", func(t *testing.T) {
		w := newS113World(t, false)
		w.decide[DefaultOpenRouterJudgeModel] = s113Decisions["none"]
		if _, _, err := w.chain.Judge(context.Background(), s113Input); !errors.Is(err, ErrNoVerdict) {
			t.Fatal(err)
		}
		w.wantCalls(1, 0, 0, 0)
	})
}

// The outcome of a lost reply is unknown: the chain's usage then has no
// Model, so the app keeps the hold; a not-sent step costs nothing.
func TestStory_S113_1_UnknownOutcomesAreReportedForTheHold(t *testing.T) {
	w := newS113World(t, false)
	for _, m := range []string{DefaultOpenRouterJudgeModel, s113Fallback, s113Second} {
		w.decide[m] = s113Decisions["unavailable"] // a 503 with no usage: unknown
	}
	_, use, err := w.chain.Judge(context.Background(), s113Input)
	if !errors.Is(err, ErrNoVerdict) || use.Model != "" || len(attempts(use)) != 3 || attempts(use)[0].Known() {
		t.Fatalf("%+v %v", use, err)
	}
	keyless := &Chain{Decisions: []GradingStep{&Jev{OpenRouter: true}, &Jev{OpenRouter: true, Model: s113Fallback}}}
	_, use, err = keyless.Judge(context.Background(), s113Input)
	if !errors.Is(err, ErrNoVerdict) || use.Model == "" || attempts(use)[0].Sent || attempts(use)[0].Outcome != OutcomeNotSent {
		t.Fatalf("not sent: %+v %v", use, err)
	}
}

// Jev tells the chain which kind of "no verdict" it gave.
func TestStory_S113_1_JevSeparatesUnavailableFromUncertain(t *testing.T) {
	for kind, want := range map[string]error{"unavailable": ErrDecisionUnavailable, "errored": ErrDecisionUnavailable, "unparseable": ErrDecisionUnavailable,
		"unknown-choice": ErrDecisionUnavailable, "low-confidence": ErrDecisionUncertain, "low-probability": ErrDecisionUncertain,
		"thin-margin": ErrDecisionUncertain, "none": ErrDecisionUncertain} {
		t.Run(kind, func(t *testing.T) {
			f := s113Decisions[kind]
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f(w) }))
			defer srv.Close()
			_, _, err := (&Jev{APIKey: s113Key, OpenRouter: true, BaseURL: srv.URL}).Judge(context.Background(), s113Input)
			other := ErrDecisionUncertain
			if want == ErrDecisionUncertain {
				other = ErrDecisionUnavailable
			}
			if !errors.Is(err, want) || errors.Is(err, other) {
				t.Fatalf("%s: %v, want %v", kind, err, want)
			}
		})
	}
}

// Without OpenRouter the escalation is the drafting model on Anthropic,
// with the same rubric and a structured output.
func TestStory_S113_1_DirectEscalationUsesTheDraftingModel(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "ant-local" {
			t.Errorf("route %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]any{"model": "claude-opus-5-5", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": `{"verdict":"incorrect"}`}}, "usage": map[string]any{"input_tokens": 300, "output_tokens": 12}})
	}))
	defer srv.Close()
	c := NewClaude("ant-local", "")
	c.BaseURL = srv.URL
	l := &LLMJudge{Client: c}
	j, use, err := l.Judge(context.Background(), s113Input)
	if err != nil || j.Verdict != srs.Missed || j.Reason != CriterionSentence(srs.Missed) || use.Model != "claude-opus-5-5" || use.InputTokens != 300 {
		t.Fatalf("%+v %+v %v", j, use, err)
	}
	oc, _ := body["output_config"].(map[string]any)
	format, _ := oc["format"].(map[string]any)
	if body["model"] != DefaultDraftModel || format["type"] != "json_schema" || !strings.Contains(body["system"].(string), CriterionSentence(srs.Close)) {
		t.Fatalf("request %+v", body)
	}
	if l.Name() != "anthropic" || l.RequestedModel() != DefaultDraftModel {
		t.Fatal(l.Name(), l.RequestedModel())
	}
}

// In test mode every step of the chain and the escalation is a fixed
// fixture: each path reaches the step it names, and the verdict comes from
// the script, never from the answer.
func TestStory_S113_1_TestFixtureChainPaths(t *testing.T) {
	type calls struct{ primary, fb1, fb2, esc int }
	for _, tc := range []struct {
		path   string
		by     string
		calls  calls
		failed bool
	}{
		{"primary", FakeJudgeModel, calls{1, 0, 0, 0}, false},
		{"primary-unavailable", FakeFallbackModel1, calls{1, 1, 0, 0}, false},
		{"primary-error", FakeFallbackModel1, calls{1, 1, 0, 0}, false},
		{"primary-unparseable", FakeFallbackModel1, calls{1, 1, 0, 0}, false},
		{"chain-unavailable", FakeEscalationModel, calls{1, 1, 1, 1}, false},
		{"primary-low-confidence", FakeEscalationModel, calls{1, 0, 0, 1}, false},
		{"primary-low-probability", FakeEscalationModel, calls{1, 0, 0, 1}, false},
		{"primary-thin-margin", FakeEscalationModel, calls{1, 0, 0, 1}, false},
		{"primary-none", FakeEscalationModel, calls{1, 0, 0, 1}, false},
		{"all-fail", "", calls{1, 1, 1, 1}, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var logs bytes.Buffer
			f := &Fake{Paths: []string{tc.path}, Verdicts: []srs.Verdict{srs.Close, srs.Missed}, Log: slog.New(slog.NewJSONHandler(&logs, nil))}
			for i, want := range []srs.Verdict{srs.Close, srs.Missed} {
				// The answer is the expected answer word for word once and
				// nonsense once: the fixture never looks.
				in := JudgeInput{Question: "q", Expected: "the expected answer", Answer: []string{"the expected answer", "zzz"}[i]}
				j, use, err := f.Judge(context.Background(), in)
				if tc.failed {
					if !errors.Is(err, ErrNoVerdict) || use.Model == "" {
						t.Fatalf("%+v %+v %v", j, use, err)
					}
					continue
				}
				if err != nil || j.Verdict != want || j.Reason != CriterionSentence(want) || use.Model != tc.by {
					t.Fatalf("%+v %+v %v", j, use, err)
				}
			}
			c := f.Calls()
			if got := (calls{c[FakeJudgeModel], c[FakeFallbackModel1], c[FakeFallbackModel2], c[FakeEscalationModel]}); got != (calls{2 * tc.calls.primary, 2 * tc.calls.fb1, 2 * tc.calls.fb2, 2 * tc.calls.esc}) {
				t.Fatalf("calls %+v, want twice %+v", got, tc.calls)
			}
			if strings.Contains(logs.String(), "the expected answer") || strings.Contains(logs.String(), "zzz") {
				t.Fatalf("logs leak the answer: %s", logs.String())
			}
		})
	}
	if len(FakeJudgePaths) != 10 || !IsFakeJudgePath("primary-none") || IsFakeJudgePath("sometimes") {
		t.Fatal(FakeJudgePaths)
	}
}
