package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"scry/internal/srs"
)

const routerKey = "sk-or-v1-ROUTERstandin"

func routerClaude(t *testing.T, h http.HandlerFunc) (*Claude, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClaudeWithRouter("", routerKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	return c, srv
}

// chunk is one OpenRouter stream chunk carrying text.
func chunk(text string) string {
	b, _ := json.Marshal(map[string]any{"model": "anthropic/claude-sonnet-5.5", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}}})
	return "data: " + string(b) + "\n\n"
}

func finishChunk(reason string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": reason}}})
	return "data: " + string(b) + "\n\n"
}

const usageChunk = `data: {"model":"anthropic/claude-sonnet-5.5","choices":[],"usage":{"prompt_tokens":812,"completion_tokens":344,"cost":0.0075}}` + "\n\n"

func checkRouterHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
		t.Errorf("wrong endpoint %s %s", r.Method, r.URL.Path)
	}
	for k, want := range map[string]string{"Authorization": "Bearer " + routerKey, "Content-Type": "application/json", "HTTP-Referer": "https://scry.mistystep.io", "X-Title": "Scry"} {
		if got := r.Header.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
	if r.Header.Get("x-api-key") != "" || r.Header.Get("anthropic-version") != "" {
		t.Error("Anthropic headers sent to OpenRouter")
	}
}

// Slice 20: drafting streams over OpenRouter's OpenAI-compatible route
// with the bounded request, survives keepalive comments, CRLF and JSONL
// split across chunks, stops at [DONE], and records the exact usage.cost.
func TestSlice20_OpenRouterDraftStream(t *testing.T) {
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		checkRouterHeaders(t, r)
		var b struct {
			Model         string         `json:"model"`
			MaxTokens     int            `json:"max_tokens"`
			Stream        bool           `json:"stream"`
			StreamOptions map[string]any `json:"stream_options"`
			System        any            `json:"system"`
			Messages      []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		if b.Model != DefaultOpenRouterDraftModel || b.MaxTokens != 16000 || !b.Stream || b.StreamOptions["include_usage"] != true || b.System != nil {
			t.Errorf("request %+v", b)
		}
		if len(b.Messages) != 2 || b.Messages[0].Role != "system" || b.Messages[0].Content != draftSystem || b.Messages[1].Role != "user" || !strings.Contains(b.Messages[1].Content, "<request>physics</request>") {
			t.Errorf("messages %+v", b.Messages)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
		fmt.Fprint(w, chunk(`{"kind":"topic","title":"Phys`))
		fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
		fmt.Fprint(w, chunk("ics\"}\r\n{\"idea\":{\"title\":\"Force\",\"summary\":\"Push\",\"items\":[{\"q\":\"Unit of force?\",\"a\":\"Newton\"}]}}\n{\"idea\":{\"title\":\"Mass\","))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, chunk(`"items":[{"q":"Unit of mass?","a":"Kilogram"}]}}`+"\n"))
		fmt.Fprint(w, chunk(`{"done":true,"note":"Pitched to a beginner."}`))
		fmt.Fprint(w, finishChunk("stop"))
		fmt.Fprint(w, usageChunk)
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	if c.Name() != "openrouter" {
		t.Fatalf("name %s", c.Name())
	}
	var heads []DraftHead
	var ideas []DraftIdea
	res, use, err := c.Draft(context.Background(), DraftInput{Query: "physics"}, DraftSink{
		Head: func(h DraftHead) error { heads = append(heads, h); return nil },
		Idea: func(i DraftIdea) error { ideas = append(ideas, i); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 1 || heads[0] != (DraftHead{Kind: "topic", Title: "Physics"}) {
		t.Fatalf("heads %+v", heads)
	}
	if len(ideas) != 2 || ideas[0].Title != "Force" || ideas[1].Title != "Mass" || ideas[1].Items[0].Answer != "Kilogram" {
		t.Fatalf("ideas %+v", ideas)
	}
	if res.Ideas != 2 || res.Note != "Pitched to a beginner." {
		t.Fatalf("result %+v", res)
	}
	if use.Model != "anthropic/claude-sonnet-5.5" || use.InputTokens != 812 || use.OutputTokens != 344 {
		t.Fatalf("usage %+v", use)
	}
	if use.CostUSD == nil || *use.CostUSD != 0.0075 || use.CostMicros == nil || *use.CostMicros != 7500 {
		t.Fatalf("cost %v %v", use.CostUSD, use.CostMicros)
	}
}

// Missing or nonsensical usage.cost is never inferred.
func TestSlice20_OpenRouterDraftCostNeverInferred(t *testing.T) {
	for name, usage := range map[string]string{
		"missing":  `{"prompt_tokens":5,"completion_tokens":6}`,
		"negative": `{"prompt_tokens":5,"completion_tokens":6,"cost":-1}`,
		"huge":     `{"prompt_tokens":5,"completion_tokens":6,"cost":1000}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, chunk("{\"done\":true}\n"))
				fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":%s}\n\n", usage)
				fmt.Fprint(w, "data: [DONE]\n\n")
			})
			_, use, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{})
			if err != nil || use.InputTokens != 5 || use.OutputTokens != 6 || use.CostUSD != nil || use.CostMicros != nil {
				t.Fatalf("%+v %v", use, err)
			}
		})
	}
}

// Every way a stream can fail, fails loudly; none leaks the key.
func TestSlice20_OpenRouterDraftFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"refusal", chunk(`{"kind":"topic","title":"X"}`+"\n") + finishChunk("content_filter") + "data: [DONE]\n\n", 200, "draft provider declined"},
		{"stream error object", chunk("{\"kind\":\"topic\",\"title\":\"X\"}\n") + `data: {"error":{"code":502,"message":"upstream echoed ` + routerKey + `"},"choices":[{"delta":{"content":""},"finish_reason":"error"}]}` + "\n\n", 200, "draft provider stream failed"},
		{"finish reason error", finishChunk("error") + "data: [DONE]\n\n", 200, "draft provider stream failed"},
		{"truncated without DONE", chunk(`{"kind":"topic","title":"X"}` + "\n"), 200, "draft stream ended before completion"},
		{"invalid chunk", "data: {not json\n\n", 200, "invalid draft stream"},
		{"http error", `{"error":{"message":"bad key ` + routerKey + `"}}`, 401, "draft provider status 401"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, _, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{Head: func(DraftHead) error { return nil }})
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), routerKey) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
		})
	}
}

// Nothing after [DONE] is read as draft text.
func TestSlice20_OpenRouterDraftStopsAtDone(t *testing.T) {
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chunk("{\"nothing\":\"Too vague.\"}\n"))
		fmt.Fprint(w, "data: [DONE]\n\n")
		fmt.Fprint(w, chunk("{\"idea\":{\"title\":\"Late\",\"items\":[{\"q\":\"q?\",\"a\":\"a\"}]}}\n"))
	})
	res, _, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{Idea: func(DraftIdea) error { t.Error("idea after [DONE]"); return nil }})
	if err != nil || res.Ideas != 0 || res.Note != "Too vague." {
		t.Fatalf("%+v %v", res, err)
	}
}

// A redirect is never followed, so the key never reaches another origin,
// and a model name echoing the key is redacted.
func TestSlice20_OpenRouterNoRedirectAndRedaction(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v1/chat/completions", http.StatusTemporaryRedirect)
	})
	if _, _, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{}); err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatal(err)
	}
	if _, _, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Missed}); err == nil {
		t.Fatal("explain followed a redirect")
	}
	if leaked.Load() != 0 {
		t.Fatal("redirect followed with the key")
	}
	c2, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "data: {\"model\":\"m-%s\",\"choices\":[{\"delta\":{\"content\":\"{\\\"done\\\":true}\\n\"}}]}\n\ndata: [DONE]\n\n", routerKey)
	})
	_, use, err := c2.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{})
	if err != nil || strings.Contains(use.Model, routerKey) || use.Model != "m-[redacted]" {
		t.Fatalf("%q %v", use.Model, err)
	}
}

// Slice 20: the on-demand explainer makes one non-streaming OpenRouter
// call with no Anthropic-only field, and records usage.cost.
func TestSlice20_OpenRouterExplain(t *testing.T) {
	var calls atomic.Int32
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		checkRouterHeaders(t, r)
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Error(err)
		}
		if b["model"] != DefaultOpenRouterExplainModel || b["max_tokens"] != float64(ExplainMaxTokens) || b["stream"] != nil || b["output_config"] != nil || b["system"] != nil {
			t.Errorf("request %s", raw)
		}
		msgs, _ := b["messages"].([]any)
		if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[0].(map[string]any)["content"] != explainSystem ||
			!strings.Contains(msgs[1].(map[string]any)["content"].(string), "<learner_answer>Paris</learner_answer>") {
			t.Errorf("messages %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"anthropic/claude-sonnet-5.5","choices":[{"message":{"role":"assistant","content":"  You named the city,\n but the card asks for the country.  "},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":18,"cost":0.00063}}`)
	})
	text, use, err := c.Explain(context.Background(), ExplainInput{Question: "Where is the Louvre?", Expected: "France", Answer: "Paris", Verdict: srs.Close})
	if err != nil || text != "You named the city, but the card asks for the country." || calls.Load() != 1 {
		t.Fatalf("%q %v", text, err)
	}
	if use.Model != "anthropic/claude-sonnet-5.5" || use.InputTokens != 120 || use.OutputTokens != 18 || use.CostUSD == nil || *use.CostUSD != 0.00063 || use.CostMicros == nil || *use.CostMicros != 630 {
		t.Fatalf("usage %+v", use)
	}
}

func TestSlice20_OpenRouterExplainClampsAndFails(t *testing.T) {
	long := strings.Repeat("word ", 200)
	for _, tc := range []struct {
		name, body string
		status     int
		want       string // "" means success
	}{
		{"clamped", `{"choices":[{"message":{"content":"` + long + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`, 200, ""},
		{"refusal", `{"choices":[{"message":{"content":""},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":1,"completion_tokens":0}}`, 200, "explanation provider declined"},
		{"http error echoing key", `{"error":{"code":401,"message":"invalid key ` + routerKey + `"}}`, 401, "explanation provider status 401"},
		{"error object on 200", `{"error":{"code":502,"message":"upstream"}}`, 200, "explanation provider status 200"},
		{"null content", `{"choices":[{"message":{"content":null},"finish_reason":"stop"}]}`, 200, "explanation was empty"},
		{"no choices", `{"choices":[]}`, 200, "explanation was empty"},
		{"not json", `<html>`, 502, "explanation provider status 502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			text, use, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Missed})
			if tc.want == "" {
				if err != nil || len([]rune(text)) > ExplainMaxChars || !strings.HasSuffix(text, "…") || use.CostMicros != nil {
					t.Fatalf("%q %+v %v", text, use, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), routerKey) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
		})
	}
}

// Slice 20: the same precedence as NewJev. OpenRouter wins whenever its key
// is set; the direct Anthropic key is the fallback only without it.
func TestSlice20_NewClaudeWithRouterPrecedence(t *testing.T) {
	c, err := NewClaudeWithRouter("ant-direct", "or-router", "", "")
	if err != nil || !c.OpenRouter || c.secret != "or-router" || c.Name() != "openrouter" || c.DraftModel != DefaultOpenRouterDraftModel || c.ExplainModel != DefaultOpenRouterExplainModel {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = NewClaudeWithRouter("ant-direct", "", "", "")
	if err != nil || c.OpenRouter || c.secret != "ant-direct" || c.Name() != "anthropic" || c.DraftModel != DefaultDraftModel || c.ExplainModel != "" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = NewClaudeWithRouter("", "or-router", "anthropic/claude-opus-5.5", "anthropic/claude-haiku-5.5")
	if err != nil || c.DraftModel != "anthropic/claude-opus-5.5" || c.ExplainModel != "anthropic/claude-haiku-5.5" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = NewClaudeWithRouter("ant-direct", "", "claude-custom", "claude-explain")
	if err != nil || c.DraftModel != "claude-custom" || c.ExplainModel != "claude-explain" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := NewClaudeWithRouter("", "", "", ""); err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY or ANTHROPIC_API_KEY") {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"claude-opus-5-5", ""}, {"", "claude-opus-5-5"}, {"anthropic/", ""}, {"/x", ""}} {
		if _, err := NewClaudeWithRouter("", "or-router-secret", bad[0], bad[1]); err == nil || !strings.Contains(err.Error(), "not an OpenRouter model slug") || strings.Contains(err.Error(), "or-router-secret") {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	// The original constructor is unchanged: direct Anthropic.
	if c := NewClaude("k", ""); c.OpenRouter || c.Name() != "anthropic" || c.DraftModel != DefaultDraftModel {
		t.Fatalf("%+v", c)
	}
}

// The direct route keeps the default explain model and its Anthropic body.
func TestSlice20_DirectExplainUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if r.URL.Path != "/v1/messages" || b["model"] != DefaultExplainModel || b["output_config"] == nil || r.Header.Get("x-api-key") != "ant" {
			t.Errorf("direct explain %s %v", r.URL.Path, b)
		}
		fmt.Fprint(w, `{"stop_reason":"end_turn","content":[{"type":"text","text":"Close."}],"usage":{"input_tokens":3,"output_tokens":1}}`)
	}))
	defer srv.Close()
	c, err := NewClaudeWithRouter("ant", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL = srv.URL
	if text, _, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Close}); err != nil || text != "Close." {
		t.Fatalf("%q %v", text, err)
	}
}

// G2: App.runDraft settles a draft's reservation only when Usage.Model is
// set, and holds it otherwise (the provider may have billed a call whose
// outcome is unknown). So every OpenRouter failure that arrives before a
// usage chunk must return an empty Usage.Model, and every stream that did
// report usage must return it set, even when the stream then fails.
func TestSlice20_OpenRouterDraftUsageModelOnlyWithReportedUsage(t *testing.T) {
	head := chunk(`{"kind":"topic","title":"X"}` + "\n")
	usageWith := func(extra string) string {
		return `data: {"model":"anthropic/claude-sonnet-5.5","usage":{"prompt_tokens":40,"completion_tokens":7,"cost":0.0002}` + extra + "}\n\n"
	}
	sinkErr := fmt.Errorf("sink failed")
	for _, tc := range []struct {
		name, body string
		sink       DraftSink
		want       string // error substring
		usage      bool   // usage was reported, so Model must be set
	}{
		{"mid-stream error object", head + `data: {"error":{"code":502,"message":"upstream"}}` + "\n\n", DraftSink{}, "draft provider stream failed", false},
		{"truncated without DONE", head + chunk(`{"idea":{"title":"Half`), DraftSink{}, "draft stream ended before completion", false},
		{"invalid JSON chunk", head + "data: {not json\n\n", DraftSink{}, "invalid draft stream", false},
		{"finish_reason error", head + finishChunk("error") + "data: [DONE]\n\n", DraftSink{}, "draft provider stream failed", false},
		{"content_filter without usage", head + finishChunk("content_filter") + "data: [DONE]\n\n", DraftSink{}, "draft provider declined", false},
		{"sink error before usage", head + usageChunk + "data: [DONE]\n\n", DraftSink{Head: func(DraftHead) error { return sinkErr }}, "sink failed", false},
		{"usage then truncated", head + usageChunk, DraftSink{}, "draft stream ended before completion", true},
		{"usage then stream error", head + usageChunk + `data: {"error":{"code":502}}` + "\n\n", DraftSink{}, "draft provider stream failed", true},
		{"content_filter with usage", head + usageWith(`,"choices":[{"delta":{},"finish_reason":"content_filter"}]`) + "data: [DONE]\n\n", DraftSink{}, "draft provider declined", true},
		{"error object with usage", head + usageWith(`,"error":{"code":502}`), DraftSink{}, "draft provider stream failed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.body)
			})
			_, use, err := c.Draft(context.Background(), DraftInput{Query: "x"}, tc.sink)
			if err == nil || (tc.want != "" && !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
			if tc.usage {
				if use.Model != "anthropic/claude-sonnet-5.5" || use.InputTokens == 0 {
					t.Fatalf("reported usage lost: %+v", use)
				}
				return
			}
			if use.Model != "" {
				t.Fatalf("Usage.Model = %q with no usage reported: the reservation would be settled at $0 (%+v)", use.Model, use)
			}
		})
	}
	t.Run("clean completion without a usage chunk still names the model", func(t *testing.T) {
		c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, chunk("{\"done\":true}\n")+"data: [DONE]\n\n")
		})
		_, use, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{})
		if err != nil || use.Model != "anthropic/claude-sonnet-5.5" {
			t.Fatalf("%+v %v", use, err)
		}
	})
	t.Run("clean completion without any model falls back to the requested one", func(t *testing.T) {
		c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"done\\\":true}\\n\"}}]}\n\ndata: [DONE]\n\n")
		})
		_, use, err := c.Draft(context.Background(), DraftInput{Query: "x"}, DraftSink{})
		if err != nil || use.Model != DefaultOpenRouterDraftModel {
			t.Fatalf("%+v %v", use, err)
		}
	})
}

// G2 for the explainer: a failure with no usage reported leaves
// Usage.Model empty; reported usage is always kept.
func TestSlice20_OpenRouterExplainUsageModelOnlyWithReportedUsage(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		usage      bool
	}{
		{"http error without usage", `{"error":{"code":401,"message":"no"}}`, 401, false},
		{"error object on 200 without usage", `{"model":"anthropic/claude-sonnet-5.5","error":{"code":502}}`, 200, false},
		{"finish_reason error without usage", `{"model":"anthropic/claude-sonnet-5.5","choices":[{"message":{"content":"partial"},"finish_reason":"error"}]}`, 200, false},
		{"content_filter without usage", `{"model":"anthropic/claude-sonnet-5.5","choices":[{"message":{"content":""},"finish_reason":"content_filter"}]}`, 200, false},
		{"no choices without usage", `{"model":"anthropic/claude-sonnet-5.5","choices":[]}`, 200, false},
		{"not json", `<html>`, 502, false},
		{"finish_reason error with usage", `{"model":"anthropic/claude-sonnet-5.5","choices":[{"message":{"content":"partial"},"finish_reason":"error"}],"usage":{"prompt_tokens":9,"completion_tokens":3}}`, 200, true},
		{"refusal with usage", `{"model":"anthropic/claude-sonnet-5.5","choices":[{"message":{"content":""},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":9,"completion_tokens":1}}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			text, use, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Missed})
			if err == nil || text != "" {
				t.Fatalf("%q %v: want an error and no text", text, err)
			}
			if got := use.Model != ""; got != tc.usage {
				t.Fatalf("Usage.Model = %q, usage reported = %v", use.Model, tc.usage)
			}
		})
	}
	t.Run("success without usage still names the model", func(t *testing.T) {
		c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"It differs."},"finish_reason":"stop"}]}`)
		})
		text, use, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Missed})
		if err != nil || text != "It differs." || use.Model != DefaultOpenRouterExplainModel {
			t.Fatalf("%q %+v %v", text, use, err)
		}
	})
}

// finish_reason "error" on the non-streaming explainer is an explicit
// failure, never partial text.
func TestSlice20_OpenRouterExplainFinishReasonError(t *testing.T) {
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"You named the ci"},"finish_reason":"error"}],"usage":{"prompt_tokens":9,"completion_tokens":3}}`)
	})
	text, _, err := c.Explain(context.Background(), ExplainInput{Question: "q", Expected: "e", Answer: "a", Verdict: srs.Missed})
	if err == nil || !strings.Contains(err.Error(), "explanation provider failed") || text != "" {
		t.Fatalf("%q %v", text, err)
	}
}

// SCRY_DRAFT_MODEL and SCRY_EXPLAIN_MODEL on OpenRouter share speech's
// strict slug check (safeerr.IsRouterSlug).
func TestSlice20_NewClaudeWithRouterRejectsMalformedSlugs(t *testing.T) {
	for _, m := range []string{"anthropic/claude sonnet", "anthropic/\tclaude", "/anthropic/claude-sonnet-5.5", "anthropic/claude-sonnet-5.5/", "anthropic//claude", "claude-opus-5-5"} {
		if c, err := NewClaudeWithRouter("", "or-router", m, ""); err == nil || c != nil || !strings.Contains(err.Error(), "SCRY_DRAFT_MODEL") {
			t.Errorf("draft %q: %v", m, err)
		}
		if c, err := NewClaudeWithRouter("", "or-router", "", m); err == nil || c != nil || !strings.Contains(err.Error(), "SCRY_EXPLAIN_MODEL") {
			t.Errorf("explain %q: %v", m, err)
		}
	}
}
