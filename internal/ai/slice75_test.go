package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Slice 75: in production, bridge cards are drafted by Scry's existing
// drafting client: the Slice 36 empirical winner on OpenRouter, sent the
// parent's question and answer and the recent misses as untrusted data,
// and its reply parsed into two or three cards.
func TestSlice75_Bridge_UsesTheDraftingClientOnOpenRouter(t *testing.T) {
	var got map[string]any
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+routerKey {
			t.Errorf("request %s, auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &got)
		reply := `{"cards":[{"q":"What does NATO stand for?","a":"North Atlantic Treaty Organization"},{"q":"What is a phonetic alphabet for?","a":"Spelling letters clearly by voice"},{"q":"What does NATO stand for?","a":"dup"},{"q":"Third?","a":"Yes"},{"q":"Fourth?","a":"Too many"}]}`
		json.NewEncoder(w).Encode(map[string]any{"model": DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90, "cost": 0.0012},
			"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": "stop"}}})
	})
	cards, use, err := c.Bridge(context.Background(), BridgeInput{Question: "NATO alphabet: B", Answer: "Bravo", Misses: []string{"Beta", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != DefaultOpenRouterDraftModel {
		t.Fatalf("model = %v, want the drafting winner %s", got["model"], DefaultOpenRouterDraftModel)
	}
	var msgs strings.Builder
	for _, m := range got["messages"].([]any) {
		msgs.WriteString(m.(map[string]any)["content"].(string))
	}
	for _, want := range []string{"<parent_question>NATO alphabet: B</parent_question>", "<parent_answer>Bravo</parent_answer>",
		"<recent_incorrect_answer>Beta</recent_incorrect_answer>", "(no answer given)", "untrusted data"} {
		if !strings.Contains(msgs.String(), want) {
			t.Errorf("request is missing %q", want)
		}
	}
	if len(cards) != MaxBridges || cards[0].Answer != "North Atlantic Treaty Organization" || cards[2].Question != "Third?" {
		t.Fatalf("cards = %+v (duplicates dropped, at most three kept)", cards)
	}
	if use.Model != DefaultOpenRouterDraftModel || use.CostMicros == nil || *use.CostMicros != 1200 {
		t.Fatalf("usage = %+v", use)
	}
}

// Slice 75: a reply with fewer than two usable cards, or none at all, is
// an error, never a half-empty preview.
func TestSlice75_Bridge_TooFewCardsIsAnError(t *testing.T) {
	for _, reply := range []string{``, `not json`, `{"cards":[]}`, `{"cards":[{"q":"only","a":"one"}]}`, `{"cards":[{"q":"","a":"x"},{"q":"y","a":""}]}`} {
		if _, err := ParseBridges(reply); err != ErrNoBridges {
			t.Errorf("%q: err = %v", reply, err)
		}
	}
	cards, err := ParseBridges("Here you go:\n```json\n{\"cards\":[{\"q\":\"a?\",\"a\":\"1\"},{\"q\":\"b?\",\"a\":\"2\"}]}\n```")
	if err != nil || len(cards) != 2 {
		t.Fatalf("fenced reply = %+v, %v", cards, err)
	}
}

// Slice 75: test mode's bridger is a deterministic fixture with no
// network call that never quotes the parent's answer.
func TestSlice75_FakeBridger_IsDeterministic(t *testing.T) {
	f := &FakeBridger{}
	a, _, err := f.Bridge(context.Background(), BridgeInput{Question: "Q?", Answer: "secret answer"})
	b, _, _ := f.Bridge(context.Background(), BridgeInput{Question: "Q?", Answer: "secret answer"})
	if err != nil || len(a) != 3 || len(b) != 3 {
		t.Fatal(a, err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic: %+v vs %+v", a[i], b[i])
		}
		if strings.Contains(a[i].Question+a[i].Answer, "secret answer") {
			t.Fatalf("fixture quotes the parent's answer: %+v", a[i])
		}
	}
	f.Mode = "fail"
	if _, _, err := f.Bridge(context.Background(), BridgeInput{}); err == nil {
		t.Fatal("fail mode should fail")
	}
}
