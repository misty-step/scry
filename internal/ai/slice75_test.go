package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
		if _, err := ParseBridges(reply, BridgeInput{}); err != ErrNoBridges {
			t.Errorf("%q: err = %v", reply, err)
		}
	}
	cards, err := ParseBridges("Here you go:\n```json\n{\"cards\":[{\"q\":\"a?\",\"a\":\"1\"},{\"q\":\"b?\",\"a\":\"2\"}]}\n```", BridgeInput{})
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
		if strings.Contains(a[i].Question+a[i].Answer, "secret answer") || Spoils(a[i], BridgeInput{Question: "Q?", Answer: "secret answer"}) {
			t.Fatalf("fixture quotes the parent's answer: %+v", a[i])
		}
	}
	f.Mode = "fail"
	if _, _, err := f.Bridge(context.Background(), BridgeInput{}); err == nil {
		t.Fatal("fail mode should fail")
	}
}

// slice75Parent is a parent card for the spoiler tests.
var slice75Parent = BridgeInput{Question: "What does NATO stand for?", Answer: "North Atlantic Treaty Organization", Misses: []string{"North American Trade Org"}}

// bridgeReply is a provider reply carrying the given cards, as JSON.
func bridgeReply(cards ...[2]string) string {
	var cs []map[string]string
	for _, c := range cards {
		cs = append(cs, map[string]string{"q": c[0], "a": c[1]})
	}
	b, _ := json.Marshal(map[string]any{"cards": cs})
	return string(b)
}

// Slice 75 (pass 2): a bridge card that gives the parent away is dropped
// before the preview, by the production adapter itself, on both drafting
// routes: two safe cards and one quoting the parent's answer verbatim make
// a two-card preview; one safe card left is a safe failure, never a
// spoiler and never a half-empty preview.
func TestSlice75_Bridge_RealAdapterDropsSpoilers(t *testing.T) {
	safe1 := [2]string{"What is a treaty?", "A formal written agreement between states"}
	safe2 := [2]string{"Which ocean lies between Europe and North America?", "The Atlantic"}
	leakQ := [2]string{"Is NATO short for the North Atlantic Treaty Organization?", "Yes"}
	leakA := [2]string{"What alliance was founded in 1949 in Washington?", "the north-atlantic treaty ORGANIZATION."}
	repeat := [2]string{"what does  NATO stand for", "An alliance"}
	cases := []struct {
		name  string
		reply string
		want  []string // kept questions; nil means ErrNoBridges
	}{
		{"two safe plus a verbatim answer in the question", bridgeReply(safe1, leakQ, safe2), []string{safe1[0], safe2[0]}},
		{"two safe plus the answer as the answer", bridgeReply(leakA, safe1, safe2), []string{safe1[0], safe2[0]}},
		{"two safe plus the parent's own question", bridgeReply(safe1, repeat, safe2), []string{safe1[0], safe2[0]}},
		{"spoilers do not take a safe card's place", bridgeReply(leakQ, safe1, leakA, safe2, repeat, [2]string{"What does a military alliance promise its members?", "Mutual defence"}),
			[]string{safe1[0], safe2[0], "What does a military alliance promise its members?"}},
		{"one safe card left is a safe failure", bridgeReply(safe1, leakQ, leakA), nil},
		{"all spoilers is a safe failure", bridgeReply(leakQ, leakA, repeat), nil},
	}
	routes := map[string]func(t *testing.T, reply string) *Claude{
		"openrouter": func(t *testing.T, reply string) *Claude {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"model": DefaultOpenRouterDraftModel,
					"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90},
					"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": "stop"}}})
			})
			return c
		},
		"anthropic": func(t *testing.T, reply string) *Claude {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/messages" {
					t.Errorf("path %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(map[string]any{"model": DefaultDraftModel, "stop_reason": "end_turn",
					"usage":   map[string]any{"input_tokens": 300, "output_tokens": 90},
					"content": []map[string]any{{"type": "text", "text": reply}}})
			}))
			t.Cleanup(srv.Close)
			c := NewClaude("ant-standin", "")
			c.BaseURL, c.HTTP = srv.URL, srv.Client()
			return c
		},
	}
	for route, mk := range routes {
		for _, tc := range cases {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				cards, use, err := mk(t, tc.reply).Bridge(context.Background(), slice75Parent)
				if use.Model == "" {
					t.Errorf("usage not reported: %+v", use)
				}
				if tc.want == nil {
					if err != ErrNoBridges || cards != nil {
						t.Fatalf("cards %+v, err %v; want ErrNoBridges", cards, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var got []string
				for _, c := range cards {
					got = append(got, c.Question)
					if strings.Contains(strings.ToLower(c.Question+" "+c.Answer), "north atlantic treaty organization") {
						t.Fatalf("spoiler kept: %+v", c)
					}
				}
				if strings.Join(got, "|") != strings.Join(tc.want, "|") {
					t.Fatalf("kept %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// Slice 75 (pass 2): what counts as an obvious spoiler. Whole words in
// order, case and punctuation ignored; a shared word is not a spoiler.
func TestSlice75_Spoils(t *testing.T) {
	b := BridgeInput{Question: "NATO alphabet: B", Answer: "Bravo"}
	for _, tc := range []struct {
		q, a  string
		spoil bool
	}{
		{"NATO alphabet: B", "x", true},             // the parent's question
		{"nato alphabet b?", "x", true},             // normalized repeat
		{"Which word follows Alfa?", "Bravo", true}, // the answer as the answer
		{"Is B spelled BRAVO?", "Yes", true},        // the answer in the question
		{"What is the NATO alphabet for?", "Spelling letters clearly by voice", false},
		{"Name a word for B", "Bravos", false},                       // not the whole word
		{"NATO alphabet: B, in the Navy's old one?", "Baker", false}, // a longer, different question
	} {
		if got := Spoils(BridgeCard{Question: tc.q, Answer: tc.a}, b); got != tc.spoil {
			t.Errorf("Spoils(%q, %q) = %v, want %v", tc.q, tc.a, got, tc.spoil)
		}
	}
	// A multi-word answer must appear whole: one of its words is fine.
	if Spoils(BridgeCard{Question: "What is a treaty?", Answer: "An agreement"}, slice75Parent) {
		t.Fatal("a shared word is not a spoiler")
	}
	// An empty parent spoils nothing.
	if Spoils(BridgeCard{Question: "q", Answer: "a"}, BridgeInput{}) {
		t.Fatal("empty parent")
	}
}
