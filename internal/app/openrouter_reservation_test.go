package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"scry/internal/ai"
)

// openRouterDrafter is the real OpenRouter drafting adapter pointed at a
// stand-in server that writes body as its SSE stream.
func openRouterDrafter(t *testing.T, body string) *ai.Claude {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	c, err := ai.NewClaudeWithRouter("", "sk-or-v1-RESERVATIONstandin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	if !c.OpenRouter {
		t.Fatal("not on OpenRouter")
	}
	return c
}

func routerChunk(text string) string {
	return fmt.Sprintf("data: {\"model\":\"anthropic/claude-sonnet-5.5\",\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", text)
}

const routerUsage = `data: {"model":"anthropic/claude-sonnet-5.5","choices":[],"usage":{"prompt_tokens":900,"completion_tokens":400,"cost":0.0123}}` + "\n\n"

// G2 end to end on the real OpenRouter drafting adapter: a stream that
// sends partial material and then truncates with no usage chunk and no
// [DONE] leaves the outcome unknown (OpenRouter may have billed it), so
// the draft's reservation stays held instead of being settled at $0 and
// handing the learner back the full allowance for an unmetered retry.
// The same stream with a usage chunk settles at exactly the reported cost.
// draftHold is the worst case of one draft on the default OpenRouter
// drafting model, openai/gpt-6.2-sol at $2/$10 per million tokens:
// (32,000 + 1,024) input and 16,000 output tokens.
const draftHold = (32000+1024)*2 + 16000*10

func TestG2_OpenRouterDraftTruncatedWithoutUsageKeepsReservation(t *testing.T) {
	partial := routerChunk(`{"kind":"topic","title":"Physics"}`+"\n") +
		routerChunk(`{"idea":{"title":"Force","summary":"Push","items":[{"q":"Unit of force?","a":"Newton"}]}}`+"\n") +
		routerChunk(`{"idea":{"title":"Ma`)
	for _, tc := range []struct {
		name, body string
		held       int64 // micros left reserved
		used       int64 // micros spent
		status     string
	}{
		{"truncated, no usage, no DONE", partial, draftHold, 0, "failed"},
		{"stream error, no usage", partial + `data: {"error":{"code":502,"message":"upstream"}}` + "\n\n", draftHold, 0, "failed"},
		{"finish_reason error, no usage", partial + `data: {"choices":[{"delta":{},"finish_reason":"error"}]}` + "\n\n" + "data: [DONE]\n\n", draftHold, 0, "failed"},
		{"usage reported, then truncated", partial + routerUsage, 0, 12300, "failed"},
		{"complete with usage", routerChunk(`{"kind":"topic","title":"Physics"}`+"\n"+`{"idea":{"title":"Force","summary":"Push","items":[{"q":"Unit of force?","a":"Newton"}]}}`+"\n") + routerUsage + "data: [DONE]\n\n", 0, 12300, "ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newBillingWorld(t, filepath.Join(t.TempDir(), "g2-openrouter.db"))
			w.app.Model = ai.Split{JudgeProvider: w.app.Model, DraftProvider: openRouterDrafter(t, tc.body)}
			id, err := w.app.Learn(w.ctx, w.user, "physics")
			if err != nil {
				t.Fatal(err)
			}
			w.app.WaitDrafts()
			d, err := w.us.Draft(w.ctx, id)
			if err != nil || d.Status != tc.status {
				t.Fatalf("draft %+v %v, want %s", d, err, tc.status)
			}
			if held, _ := w.us.ReservedMicros(w.ctx); held != tc.held {
				t.Fatalf("held %d micros, want %d", held, tc.held)
			}
			sub, _ := w.us.Subscription(w.ctx)
			if sub.UsedMicros != tc.used {
				t.Fatalf("used %d micros, want %d", sub.UsedMicros, tc.used)
			}
		})
	}
}
