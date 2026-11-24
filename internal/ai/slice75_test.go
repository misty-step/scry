package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
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

// Slice 75 (pass 3): a symbol-only parent answer, such as a code or math
// operator, has no words to compare, so it is matched literally. The
// production adapter, on both drafting routes, drops a bridge whose
// question or answer contains it, keeps the safe ones, and fails safely
// when fewer than two are left; word answers still match as before.
func TestSlice75_Bridge_RealAdapterDropsSymbolOnlyAnswerLeaks(t *testing.T) {
	parent := BridgeInput{Question: "In C, which operator tests that two values are not equal?", Answer: " != ", Misses: []string{"=/="}}
	safe1 := [2]string{"In C, what does the == operator test?", "That two values are equal"}
	safe2 := [2]string{"In C, what value does a false comparison produce?", "0"}
	safe3 := [2]string{"In C, which operator negates a boolean value?", "!"}
	leakQ := [2]string{"Which programming languages use != to mean not equal?", "C, Java, Python and many others"}
	leakA := [2]string{"In C, what is the inequality operator written as?", "It is written !=."}
	cases := []struct {
		name  string
		reply string
		want  []string // kept questions; nil means ErrNoBridges
	}{
		{"two safe plus the operator in the question", bridgeReply(safe1, leakQ, safe2), []string{safe1[0], safe2[0]}},
		{"two safe plus the operator in the answer", bridgeReply(leakA, safe1, safe2), []string{safe1[0], safe2[0]}},
		{"three safe cards are all kept", bridgeReply(safe1, safe2, safe3), []string{safe1[0], safe2[0], safe3[0]}},
		{"one safe card left is a safe failure", bridgeReply(safe1, leakQ, leakA), nil},
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
				cards, _, err := mk(t, tc.reply).Bridge(context.Background(), parent)
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
					if strings.Contains(c.Question+" "+c.Answer, "!=") {
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

// Slice 75 (pass 3): symbol-only answers are matched literally, after
// trimming; answers with any letter or digit keep whole-word matching.
func TestSlice75_Spoils_SymbolOnlyAnswers(t *testing.T) {
	for _, tc := range []struct {
		answer, q, a string
		spoil        bool
	}{
		{"!=", "Which languages use != for not equal?", "C and Java", true},
		{"  !=\n", "What is the inequality operator in C?", "!=", true},
		{"!=", "What does the == operator test in C?", "Equality", false},
		{"!=", "Which operator negates a boolean in C?", "!", false},
		{"->", "In C, what does p->x read?", "A struct field via a pointer", true},
		{"->", "In C, what does p.x read?", "A field of a struct value", false},
		{"&&", "What does && do in C?", "Logical and", true},
		{"≠", "Which symbol means not equal in maths?", "≠", true},
		{"≠", "Which symbol means equal in maths?", "=", false},
		{"%", "Which operator gives a remainder in C?", "% (modulo)", true},
		{"< >", "Which SQL pair of brackets means not equal?", "< >", true},
	} {
		parent := BridgeInput{Question: "Which operator?", Answer: tc.answer}
		if got := Spoils(BridgeCard{Question: tc.q, Answer: tc.a}, parent); got != tc.spoil {
			t.Errorf("answer %q: Spoils(%q, %q) = %v, want %v", tc.answer, tc.q, tc.a, got, tc.spoil)
		}
	}
	// Word matching is unchanged: a letter-bearing answer is whole words.
	word := BridgeInput{Question: "In C, how is not-equal spelled in prose?", Answer: "not equal"}
	if !Spoils(BridgeCard{Question: "What does != mean? Not equal?", Answer: "x"}, word) {
		t.Fatal("word answer should still match as whole words")
	}
	if Spoils(BridgeCard{Question: "What does nothing equalize?", Answer: "x"}, word) {
		t.Fatal("partial words are still not spoilers")
	}
	// A whitespace-only answer spoils nothing.
	if Spoils(BridgeCard{Question: "q", Answer: "a"}, BridgeInput{Question: "p", Answer: "   "}) {
		t.Fatal("blank answer")
	}
}

// Slice 75 (pass 4): the reply is decoded one JSON object at a time, so a
// valid cards object followed by another object parses, prose and objects
// before it are skipped, and single objects (bare or fenced) still parse,
// through the production parser and the real OpenRouter adapter alike.
func TestSlice75_ParseBridges_FirstCardsObjectOfSeveral(t *testing.T) {
	two := `{"cards":[{"q":"What is a treaty?","a":"A formal agreement"},{"q":"Which ocean is west of Europe?","a":"The Atlantic"}]}`
	for name, reply := range map[string]string{
		"single":                 two,
		"fenced single":          "```json\n" + two + "\n```",
		"cards then a trailer":   two + "\n" + `{"note":"two cards above"}`,
		"cards then a cards obj": two + ` {"cards":[{"q":"other?","a":"x"},{"q":"other 2?","a":"y"}]}`,
		"an object before it":    `{"thinking":"short"} ` + two,
		"broken brace before it": `Notes {like this} then: ` + two + ` {"tail": true}`,
		"braces inside strings":  `{"cards":[{"q":"In set notation, what does {} mean?","a":"The empty set"},{"q":"Which ocean is west of Europe?","a":"The Atlantic"}]} {"x":1}`,
	} {
		cards, err := ParseBridges(reply, slice75Parent)
		if err != nil || len(cards) != 2 {
			t.Errorf("%s: %+v, %v", name, cards, err)
			continue
		}
		if name != "braces inside strings" && cards[0].Question != "What is a treaty?" {
			t.Errorf("%s: first card %+v", name, cards[0])
		}
	}
	for name, reply := range map[string]string{
		"no cards key":  `{"note":"x"} {"other":[]}`,
		"cards is null": `{"cards":null}`,
		"unclosed":      `{"cards":[{"q":"a?","a":"1"},{"q":"b?","a":"2"}]`,
	} {
		if _, err := ParseBridges(reply, slice75Parent); err != ErrNoBridges {
			t.Errorf("%s: err = %v, want ErrNoBridges", name, err)
		}
	}
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"model": DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90},
			"choices": []map[string]any{{"message": map[string]any{"content": two + "\n{\"note\":\"done\"}"}, "finish_reason": "stop"}}})
	})
	if cards, _, err := c.Bridge(context.Background(), slice75Parent); err != nil || len(cards) != 2 {
		t.Fatalf("real adapter, cards plus a trailing object = %+v, %v", cards, err)
	}
}

// Slice 75 (pass 4): a draft that fails to parse settles only usage the
// provider reported. With no usage in the reply, the adapter reports none
// (the app keeps the reservation pending, G2); with usage reported it is
// kept even though the draft failed; a clean success without usage still
// names the drafting model. Both drafting routes.
func TestSlice75_Bridge_FallbackModelOnlyOnSuccess(t *testing.T) {
	good := bridgeReply([2]string{"What is a treaty?", "A formal agreement"}, [2]string{"Which ocean is west of Europe?", "The Atlantic"})
	routes := map[string]func(t *testing.T, reply string, usage bool) *Claude{
		"openrouter": func(t *testing.T, reply string, usage bool) *Claude {
			c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{"model": DefaultOpenRouterDraftModel,
					"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": "stop"}}}
				if usage {
					body["usage"] = map[string]any{"prompt_tokens": 300, "completion_tokens": 90}
				}
				json.NewEncoder(w).Encode(body)
			})
			return c
		},
		"anthropic": func(t *testing.T, reply string, usage bool) *Claude {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{"model": DefaultDraftModel, "stop_reason": "end_turn",
					"content": []map[string]any{{"type": "text", "text": reply}}}
				if usage {
					body["usage"] = map[string]any{"input_tokens": 300, "output_tokens": 90}
				}
				json.NewEncoder(w).Encode(body)
			}))
			t.Cleanup(srv.Close)
			c := NewClaude("ant-standin", "")
			c.BaseURL, c.HTTP = srv.URL, srv.Client()
			return c
		},
	}
	for route, mk := range routes {
		t.Run(route, func(t *testing.T) {
			for _, bad := range []string{`{"cards":[]}`, `not json`, `{"cards":[{"q":"only","a":"one"}]}`} {
				cards, use, err := mk(t, bad, false).Bridge(context.Background(), slice75Parent)
				if err != ErrNoBridges || cards != nil || use != (Usage{}) {
					t.Fatalf("%q without usage: cards %+v, usage %+v, err %v; want no usage reported", bad, cards, use, err)
				}
				_, use, err = mk(t, bad, true).Bridge(context.Background(), slice75Parent)
				if err != ErrNoBridges || use.Model == "" || use.InputTokens != 300 || use.OutputTokens != 90 {
					t.Fatalf("%q with usage: usage %+v, err %v; want the reported usage kept", bad, use, err)
				}
			}
			cards, use, err := mk(t, good, false).Bridge(context.Background(), slice75Parent)
			if err != nil || len(cards) != 2 || use.Model == "" || use.InputTokens != 0 {
				t.Fatalf("clean success without usage: %+v, %+v, %v", cards, use, err)
			}
		})
	}
}

// Slice 75 (pass 4): the request is fitted to its cap before anything is
// sent. A 900-byte question, a 1,900-byte answer and three recent misses
// of 300 four-byte characters do not fit as they are: the oldest misses
// are dropped (never the question or answer) and the request goes out
// within the cap. Input that cannot fit at all is ErrBridgeNotSent and
// the provider is never called.
func TestSlice75_Bridge_InputIsFittedBeforeDispatch(t *testing.T) {
	q := strings.Repeat("q", 899) + "?"
	a := strings.Repeat("a", 1900)
	miss := strings.Repeat("𝔸", 300) // 4 bytes each
	in := BridgeInput{Question: q, Answer: a, Misses: []string{miss + "1", miss + "2", miss + "3", "fourth"}}
	if _, err := bridgeUser(BridgeInput{Question: q, Answer: a, Misses: in.Misses[:3]}); err == nil {
		t.Fatal("precondition: the unfitted input should exceed the cap")
	}
	fit, err := FitBridgeInput(in)
	if err != nil || fit.Question != q || fit.Answer != a || len(fit.Misses) == 0 || len(fit.Misses) == 3 {
		t.Fatalf("fit = %d misses, %v", len(fit.Misses), err)
	}
	if fit.Misses[0] != in.Misses[0] {
		t.Fatal("the newest miss must be kept first")
	}
	var calls int
	var sent string
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var req struct{ Messages []struct{ Content string } }
		json.Unmarshal(raw, &req)
		sent = req.Messages[1].Content
		json.NewEncoder(w).Encode(map[string]any{"model": DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90},
			"choices": []map[string]any{{"message": map[string]any{"content": bridgeReply([2]string{"What is x?", "x"}, [2]string{"What is y?", "y"})}, "finish_reason": "stop"}}})
	})
	if cards, _, err := c.Bridge(context.Background(), in); err != nil || len(cards) != 2 {
		t.Fatalf("fitted bridge = %+v, %v", cards, err)
	}
	if calls != 1 || len(sent) > BridgeInputChars || !strings.Contains(sent, q) || !strings.Contains(sent, a) || !strings.Contains(sent, "𝔸") {
		t.Fatalf("calls %d, sent %d bytes", calls, len(sent))
	}
	huge := BridgeInput{Question: strings.Repeat("q", BridgeInputChars), Answer: "a", Misses: []string{"m"}}
	if _, err := FitBridgeInput(huge); err != ErrBridgeNotSent {
		t.Fatalf("unfittable fit = %v", err)
	}
	_, use, err := c.Bridge(context.Background(), huge)
	if !errors.Is(err, ErrBridgeNotSent) || use != (Usage{}) || calls != 1 {
		t.Fatalf("unfittable bridge: err %v, usage %+v, calls %d; want ErrBridgeNotSent and no call", err, use, calls)
	}
}

// Slice 75 (pass 4): a drafted card is kept only if it can be accepted
// unchanged: at most 400 characters and within the card limits in bytes.
// 400 ASCII characters and 400 two-byte characters fit; 350 Han characters
// (1,050 bytes) are over the question limit and are dropped.
func TestSlice75_ParseBridges_ByteBoundsMatchAcceptance(t *testing.T) {
	ascii := "Q" + strings.Repeat("abcdefghij", 39) + "klmnopqr?"
	accented := strings.Repeat("é", 399) + "?"
	han := strings.Repeat("桥", 349) + "?"
	if len(ascii) != 400 || utf8.RuneCountInString(accented) != 400 || len(han) <= BridgeQuestionMaxBytes || utf8.RuneCountInString(han) != 350 {
		t.Fatal("bad fixtures")
	}
	cards, err := ParseBridges(bridgeReply([2]string{ascii, "x"}, [2]string{accented, "y"}, [2]string{han, "z"}), slice75Parent)
	if err != nil || len(cards) != 2 || cards[0].Question != ascii || cards[1].Question != accented {
		t.Fatalf("cards %+v, %v; want the ASCII and accented ones only", cards, err)
	}
	for _, c := range cards {
		if len(c.Question) > BridgeQuestionMaxBytes || len(c.Answer) > BridgeAnswerMaxBytes {
			t.Fatalf("over the byte limits: %d/%d", len(c.Question), len(c.Answer))
		}
	}
	if _, err := ParseBridges(bridgeReply([2]string{han, "x"}, [2]string{"What is y?", "y"}), slice75Parent); err != ErrNoBridges {
		t.Fatalf("one acceptable card left = %v, want ErrNoBridges", err)
	}
}

// A symbol-only parent question has no normalized words, but an exact
// literal repeat is still the same question. Using the operator in a
// different prerequisite question is fine.
func TestSlice75_Bridge_RealAdapterDropsSymbolOnlyQuestionRepeat(t *testing.T) {
	parent := BridgeInput{Question: " -> ", Answer: "pointer member access"}
	safe1 := [2]string{"What does -> require on its left in C?", "A pointer to a struct or union"}
	safe2 := [2]string{"Which operator accesses a member of a struct value in C?", "."}
	repeat := [2]string{"->", "Access a member through a pointer"}
	c, _ := routerClaude(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": bridgeReply(safe1, repeat, safe2)}, "finish_reason": "stop"}},
		})
	})
	got, _, err := c.Bridge(context.Background(), parent)
	if err != nil || len(got) != 2 || got[0].Question != safe1[0] || got[1].Question != safe2[0] {
		t.Fatalf("kept %+v, err %v; want only the two different questions", got, err)
	}
}
