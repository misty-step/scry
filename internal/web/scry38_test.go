package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/srs"
)

// SCRY-38 (Sentry 7867844443, release 8a14e49): Phaedrus's long, right,
// conceptual answer was refused with "Couldn't check that just now" (HTTP
// 503) twice, because the real Jev grader split its decision between
// correct (0.64) and almost correct (0.36) and the grader had nowhere to
// send a split. Through the real Study handler and the real grading chain
// ([S113-1]: the Jev adapter, its fallback decisions models and the
// escalation LLM adapter), with a stand-in replaying that recorded
// response, the split goes straight to the LLM and the answer stops on its
// review with the LLM's verdict; no alert is raised, no fallback decisions
// model is asked, and one review is recorded.
func TestStory_SCRY38_LongConceptualAnswerIsJudgedNot503(t *testing.T) {
	const (
		question = "What is the testing effect (retrieval practice) in learning science, and why does it outperform rereading?"
		expected = "Recalling information from memory strengthens long-term retention more than rereading, because effortful retrieval reinforces and reorganizes the memory trace and reveals gaps, while rereading builds only familiarity."
		answer   = "the testing effect states that it is better for recall to quiz a student and have them actually try to answer a question rather than just reread the material because it is challenging, it forces you to dig through your mind and find the material and the understanding yourself rather than just let the words on the page pass over you again"
		// typesafe/jev-1.14 on OpenRouter, live, for this state (2026-11-25).
		live = `{"model":"typesafe/jev-1.14-20261014","answers":{"judgment":{"type":"choice","choice":"correct","probabilities":{"almost correct":0.36,"correct":0.64,"incorrect":0,"none_of_these":0},"confidence":0.52}},"usage":{"input_tokens":520,"output_tokens":51,"cost":0.00002184,"is_byok":false},"id":"gen-dec-redacted","provider":"TypeSafe"}`
	)
	for _, tc := range []struct {
		llm     string
		verdict srs.Verdict
	}{
		{"almost correct", srs.Close},
		{"correct", srs.Right},
	} {
		t.Run("the LLM says "+tc.llm, func(t *testing.T) {
			w := newWorld(t)
			b := w.browser()
			email := "phaedrus38-" + string(tc.verdict) + "@example.com"
			b.signUp(email)
			calls := map[string]int{}
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				var body struct {
					Model    string
					State    map[string]any
					Messages []struct{ Role, Content string }
				}
				json.NewDecoder(r.Body).Decode(&body)
				calls[body.Model]++
				switch {
				case r.URL.Path == "/api/alpha/decisions" && body.Model == ai.DefaultOpenRouterJudgeModel:
					if body.State["learner_answer"] != answer || body.State["card_prompt"] != question {
						t.Errorf("Jev did not get the exact question and answer: %v", body.State)
					}
					rw.Write([]byte(live))
				case r.URL.Path == "/api/v1/chat/completions":
					if len(body.Messages) != 2 || !strings.Contains(body.Messages[1].Content, "dig through your mind") {
						t.Errorf("the LLM did not get the answer: %+v", body.Messages)
					}
					json.NewEncoder(rw).Encode(map[string]any{"model": body.Model, "choices": []map[string]any{{"message": map[string]any{"content": `{"verdict":"` + tc.llm + `"}`}, "finish_reason": "stop"}},
						"usage": map[string]any{"prompt_tokens": 610, "completion_tokens": 9, "cost": 0.00065}})
				default:
					t.Errorf("an uncertain decision was shopped to %s %s", r.URL.Path, body.Model)
					rw.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer srv.Close()
			jev := func(m string) *ai.Jev {
				return &ai.Jev{Model: m, APIKey: "local-secret", OpenRouter: true, BaseURL: srv.URL, Log: w.app.Log}
			}
			llm, err := ai.NewClaudeWithRouter("", "local-secret", "", "")
			if err != nil {
				t.Fatal(err)
			}
			llm.BaseURL = srv.URL
			fallbacks := ai.DefaultJudgeFallbackModels()
			chain := &ai.Chain{Decisions: []ai.GradingStep{jev(""), jev(fallbacks[0]), jev(fallbacks[1])},
				Escalation: &ai.LLMJudge{Client: llm, Model: ai.DefaultJudgeEscalationModel}, Log: w.app.Log}
			w.app.Model = ai.Split{JudgeProvider: chain, DraftProvider: w.model}
			csv := "question,answer\n\"" + question + "\",\"" + expected + "\"\n"
			if _, err := w.app.Import(ctx, w.user(email), "testing-effect.csv", []byte(csv), "Learning science"); err != nil {
				t.Fatal(err)
			}
			it, ok := b.current(b.mustGet("/study"))
			if !ok || it.Prompt != question {
				t.Fatalf("current card %+v", it)
			}
			before := len(w.alerts.Titles())
			status, page := b.post("/study/answer", "item_id", strconv.FormatInt(it.ID, 10), "answer", answer)
			if status != http.StatusOK || len(w.alerts.Titles()) != before {
				t.Fatalf("status %d, alerts %v", status, w.alerts.Titles())
			}
			if calls[ai.DefaultOpenRouterJudgeModel] != 1 || calls[ai.DefaultJudgeEscalationModel] != 1 || len(calls) != 2 {
				t.Fatalf("calls %v: want Jev once, then the LLM once", calls)
			}
			if strings.Contains(page, "Couldn't check that just now") || !strings.Contains(page, "data-review") {
				t.Fatal("the answer did not stop on its review")
			}
			// The verdict by its data attribute: its wording is [S130-1]'s.
			has(t, page, `data-verdict="`+string(tc.verdict)+`"`, `class="why"`)
			hist, err := w.lib(email).History(ctx, it.ID)
			if err != nil || len(hist) != 1 {
				t.Fatalf("history %v %v", hist, err)
			}
			if r := hist[0]; r.Verdict != tc.verdict || r.Answer != answer || r.Judge != ai.DefaultJudgeEscalationModel {
				t.Fatalf("review %+v", r)
			}
		})
	}
}
