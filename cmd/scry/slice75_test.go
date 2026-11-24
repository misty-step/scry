package main

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"scry/internal/ai"
)

// trapTransport fails and counts every outbound request, so a test can
// prove an adapter never reached the network.
type trapTransport struct{ calls atomic.Int64 }

func (t *trapTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("network trapped in test")
}

func trapNetwork(t *testing.T) *trapTransport {
	t.Helper()
	trap := &trapTransport{}
	orig := http.DefaultTransport
	http.DefaultTransport = trap
	t.Cleanup(func() { http.DefaultTransport = orig })
	return trap
}

// Slice 75 (pass 2): in SCRY_MODE=test, bridge cards are always the
// deterministic fixture with no network call, even when SCRY_AI_MODE=live
// or SCRY_JUDGE_MODE=live put real adapters on the other AI clients. Those
// other live overrides are kept as they were.
func TestSlice75_TestModeBridgerIsAlwaysTheFixture(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		live bool // the explainer is the real drafting client
	}{
		{"default", nil, false},
		{"SCRY_AI_MODE=live", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_AI_MODE": "live"}, true},
		{"SCRY_AI_MODE=live direct", map[string]string{"TYPESAFE_API_KEY": slice20Keys["TYPESAFE_API_KEY"], "ANTHROPIC_API_KEY": slice20Keys["ANTHROPIC_FALLBACK"], "OPENAI_API_KEY": slice20Keys["OPENAI_FALLBACK"], "SCRY_AI_MODE": "live"}, true},
		{"SCRY_JUDGE_MODE=live", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_JUDGE_MODE": "live"}, false},
		{"SCRY_TEST_BRIDGES=fail under SCRY_AI_MODE=live", map[string]string{"OPENROUTER_API_KEY": slice20Keys["OPENROUTER_ONLY"], "SCRY_AI_MODE": "live", "SCRY_TEST_BRIDGES": "fail"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modeEnv(t, "test", tc.env)
			t.Setenv("SCRY_TEST_BRIDGES", tc.env["SCRY_TEST_BRIDGES"])
			a, err := tryBuild(t)
			if err != nil {
				t.Fatal(err)
			}
			fake, ok := a.Bridger.(*ai.FakeBridger)
			if !ok {
				t.Fatalf("bridger is %T, want the test fixture", a.Bridger)
			}
			if got := ai.BridgeModel(a.Bridger); got != "test-draft" {
				t.Fatalf("bridge model %q", got)
			}
			// The other live overrides are preserved.
			if _, real := a.Explainer.(*ai.Claude); real != tc.live {
				t.Fatalf("explainer is %T, want live=%v", a.Explainer, tc.live)
			}
			if tc.live {
				if _, ok := a.Model.(ai.Split); !ok {
					t.Fatalf("model is %T, want the live split", a.Model)
				}
			}

			trap := trapNetwork(t)
			in := ai.BridgeInput{Question: "NATO alphabet: B", Answer: "Bravo", Misses: []string{"Beta"}}
			cards, _, err := a.Bridger.Bridge(context.Background(), in)
			if tc.env["SCRY_TEST_BRIDGES"] == "fail" {
				if err == nil {
					t.Fatal("SCRY_TEST_BRIDGES=fail should fail")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := ai.FakeBridgeCards(in.Question)
				if len(cards) != len(want) {
					t.Fatalf("cards %+v", cards)
				}
				for i := range want {
					if cards[i] != want[i] {
						t.Fatalf("card %d = %+v, want fixture %+v", i, cards[i], want[i])
					}
				}
			}
			if fake.Calls() != 1 || trap.calls.Load() != 0 {
				t.Fatalf("fixture calls %d, network calls %d", fake.Calls(), trap.calls.Load())
			}
			// Control: the trap does catch a real client's request.
			if tc.live {
				a.Explainer.Explain(context.Background(), ai.ExplainInput{Question: "q", Expected: "a", Answer: "b", Verdict: "missed"})
				if trap.calls.Load() == 0 {
					t.Fatal("the network trap missed the live explainer's request")
				}
			}
		})
	}
}

// Slice 75: live mode drafts bridges with the real drafting client.
func TestSlice75_LiveModeBridgerIsTheDraftingClient(t *testing.T) {
	modeEnv(t, "live", liveKeys)
	a, err := tryBuild(t)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := a.Bridger.(*ai.Claude)
	if !ok || !c.OpenRouter || c.DraftModel != ai.DefaultOpenRouterDraftModel {
		t.Fatalf("bridger %T %+v", a.Bridger, a.Bridger)
	}
}
