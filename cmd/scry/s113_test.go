package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
)

// unsetChainEnv leaves the [S113-1] chain settings unset (not empty: an
// empty SCRY_JUDGE_FALLBACK_MODELS means no fallbacks) and restores them
// after the test.
func unsetChainEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SCRY_JUDGE_FALLBACK_MODELS", "SCRY_JUDGE_ESCALATION_MODEL", "SCRY_TEST_JUDGE_PATHS", "SCRY_TEST_JUDGE_DELAY", "SCRY_TEST_JUDGMENTS"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// primaryJev is the primary decisions model of a grading chain.
func primaryJev(p ai.JudgeProvider) (*ai.Jev, bool) {
	c, ok := p.(*ai.Chain)
	if !ok {
		return nil, false
	}
	j, ok := c.Primary().(*ai.Jev)
	return j, ok
}

func chainOf(t *testing.T, p ai.JudgeProvider) *ai.Chain {
	t.Helper()
	c, ok := p.(*ai.Chain)
	if !ok || c == nil {
		t.Fatalf("judge is %T, want the grading chain", p)
	}
	return c
}

// [S113-1] Every model in the chain is a real OpenRouter slug on
// OPENROUTER_API_KEY: SCRY_JUDGE_MODEL overrides the primary,
// SCRY_JUDGE_FALLBACK_MODELS (comma-separated, may be empty) the fallback
// chain, SCRY_JUDGE_ESCALATION_MODEL the LLM; a value that is not a
// vendor/model slug stops the boot. Without OpenRouter the chain is Jev on
// Typesafe and the LLM is the drafting model on its own provider.
func TestStory_S113_1_ChainConfigurationAndSlugChecksAtBoot(t *testing.T) {
	t.Run("defaults on OpenRouter", func(t *testing.T) {
		modeEnv(t, "live", liveKeys)
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		c := chainOf(t, a.Model.(ai.Split).JudgeProvider)
		var models []string
		for _, d := range c.Decisions {
			j, ok := d.(*ai.Jev)
			if !ok || !j.OpenRouter || j.APIKey != liveKeys["OPENROUTER_API_KEY"] {
				t.Fatalf("decisions step %T %+v", d, d)
			}
			models = append(models, j.RequestedModel())
		}
		if want := append([]string{"typesafe/jev-1.14"}, ai.DefaultJudgeFallbackModels()...); !reflect.DeepEqual(models, want) {
			t.Fatalf("decisions models %v, want %v", models, want)
		}
		llm, ok := c.Escalation.(*ai.LLMJudge)
		if !ok || llm.RequestedModel() != "google/gemini-4-hydrogen" || !llm.Client.OpenRouter {
			t.Fatalf("escalation %T %+v", c.Escalation, c.Escalation)
		}
		_, raw, h := getHealthz(t, a)
		gc, _ := h["grading_chain"].(map[string]any)
		if gc["primary"] != "typesafe/jev-1.14" || gc["escalation"] != "google/gemini-4-hydrogen" || gc["escalation_provider"] != "openrouter" ||
			!reflect.DeepEqual(gc["fallbacks"], []any{"openai/gpt-6-luna-decisions", "google/gemini-decision-1"}) {
			t.Fatalf("healthz grading_chain %s", raw)
		}
		if m, _ := h["models"].(map[string]any); m["grading"] != "typesafe/jev-1.14" {
			t.Fatalf("models.grading lost: %s", raw)
		}
		assertNoSecrets(t, raw)
	})
	t.Run("overrides on OpenRouter", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_JUDGE_MODEL": "cloudflare/clef", "SCRY_JUDGE_FALLBACK_MODELS": " liquid/d1.2 , ,upstage/solar-decide",
			"SCRY_JUDGE_ESCALATION_MODEL": "anthropic/claude-haiku-5.5"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		info := ai.JudgeChain(a.Model)
		if info.Primary != "cloudflare/clef" || !reflect.DeepEqual(info.Fallbacks, []string{"liquid/d1.2", "upstage/solar-decide"}) || info.Escalation != "anthropic/claude-haiku-5.5" {
			t.Fatalf("chain %+v", info)
		}
	})
	t.Run("empty SCRY_JUDGE_FALLBACK_MODELS means no fallback decisions models", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, map[string]string{"SCRY_JUDGE_FALLBACK_MODELS": ""})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		info := ai.JudgeChain(a.Model)
		if info.Primary != "typesafe/jev-1.14" || len(info.Fallbacks) != 0 || info.Escalation != ai.DefaultJudgeEscalationModel {
			t.Fatalf("chain %+v", info)
		}
	})
	for _, tc := range []struct{ name, key, value, says string }{
		{"non-slug fallback stops the boot", "SCRY_JUDGE_FALLBACK_MODELS", "openai/gpt-6-luna-decisions,gemini-decision-1", `SCRY_JUDGE_FALLBACK_MODELS has "gemini-decision-1", which is not an OpenRouter model slug`},
		{"non-slug escalation stops the boot", "SCRY_JUDGE_ESCALATION_MODEL", "gemini-4-hydrogen", `SCRY_JUDGE_ESCALATION_MODEL="gemini-4-hydrogen" is not an OpenRouter model slug`},
		{"non-slug primary stops the boot", "SCRY_JUDGE_MODEL", "jev-latest", `SCRY_JUDGE_MODEL="jev-latest" is not an OpenRouter model slug`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modeEnv(t, "live", liveKeys, map[string]string{tc.key: tc.value})
			failBoot(t, tc.says)
		})
		t.Run(tc.name+" with SCRY_JUDGE_MODE=live in test mode", func(t *testing.T) {
			modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": liveKeys["OPENROUTER_API_KEY"], "SCRY_JUDGE_MODE": "live", tc.key: tc.value})
			failBoot(t, tc.says)
		})
	}
	t.Run("without OpenRouter: Jev on Typesafe, escalating to the drafting model on Anthropic", func(t *testing.T) {
		modeEnv(t, "live", liveKeys, liveNoAI, map[string]string{"TYPESAFE_API_KEY": slice20Keys["TYPESAFE_API_KEY"], "ANTHROPIC_API_KEY": slice20Keys["ANTHROPIC_FALLBACK"],
			"OPENAI_API_KEY": slice20Keys["OPENAI_FALLBACK"], "SCRY_JUDGE_FALLBACK_MODELS": "not a slug", "SCRY_JUDGE_ESCALATION_MODEL": "also-not-a-slug"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		c := chainOf(t, a.Model.(ai.Split).JudgeProvider)
		if j, ok := c.Primary().(*ai.Jev); len(c.Decisions) != 1 || !ok || j.OpenRouter {
			t.Fatalf("decisions %+v", c.Decisions)
		}
		llm, ok := c.Escalation.(*ai.LLMJudge)
		if !ok || llm.Client.OpenRouter || llm.RequestedModel() != ai.DefaultDraftModel || llm.Client != a.Explainer.(*ai.Claude) {
			t.Fatalf("escalation %T %+v", c.Escalation, c.Escalation)
		}
		_, raw, h := getHealthz(t, a)
		gc, _ := h["grading_chain"].(map[string]any)
		if gc["primary"] != "jev-latest" || gc["escalation"] != ai.DefaultDraftModel || gc["escalation_provider"] != "anthropic" || len(gc["fallbacks"].([]any)) != 0 {
			t.Fatalf("healthz %s", raw)
		}
		assertNoSecrets(t, raw)
	})
	t.Run("SCRY_JUDGE_MODE=live on OpenRouter carries the whole chain", func(t *testing.T) {
		modeEnv(t, "test", map[string]string{"OPENROUTER_API_KEY": liveKeys["OPENROUTER_API_KEY"], "SCRY_JUDGE_MODE": "live"})
		a, err := tryBuild(t)
		if err != nil {
			t.Fatal(err)
		}
		if info := ai.JudgeChain(a.Model); len(info.Fallbacks) != 2 || info.Escalation != ai.DefaultJudgeEscalationModel || info.EscalationProvider != "openrouter" {
			t.Fatalf("chain %+v", info)
		}
	})
}

// [S113-1] In test mode every step of the chain and the escalation is a
// fixed fixture with no network call. SCRY_TEST_JUDGE_PATHS forces each
// submission down a path in turn; SCRY_TEST_JUDGE_DELAY holds each
// judgment so "checking" and "still checking" can be seen. A bad value
// stops the boot.
func TestStory_S113_1_TestModeChainFixtureSettings(t *testing.T) {
	modeEnv(t, "test", map[string]string{"SCRY_TEST_JUDGE_PATHS": "primary-none, chain-unavailable,all-fail", "SCRY_TEST_JUDGE_DELAY": "30ms", "SCRY_TEST_JUDGMENTS": "close"})
	a, err := tryBuild(t)
	if err != nil {
		t.Fatal(err)
	}
	fake, ok := a.Model.(*ai.Fake)
	if !ok || !reflect.DeepEqual(fake.Paths, []string{"primary-none", "chain-unavailable", "all-fail"}) || fake.JudgeDelay != 30*time.Millisecond {
		t.Fatalf("model %T %+v", a.Model, a.Model)
	}
	start := time.Now()
	j, use, err := a.Model.Judge(context.Background(), ai.JudgeInput{Question: "q", Expected: "e", Answer: "e"})
	if err != nil || j.Verdict != "close" || use.Model != ai.FakeEscalationModel || time.Since(start) < 30*time.Millisecond {
		t.Fatalf("primary-none: %+v %+v %v", j, use, err)
	}
	_, raw, h := getHealthz(t, a)
	gc, _ := h["grading_chain"].(map[string]any)
	if gc["primary"] != ai.FakeJudgeModel || gc["escalation"] != ai.FakeEscalationModel || gc["escalation_provider"] != "test" {
		t.Fatalf("healthz %s", raw)
	}
	for _, bad := range []struct{ key, value, says string }{
		{"SCRY_TEST_JUDGE_PATHS", "primary,sometimes", `SCRY_TEST_JUDGE_PATHS: "sometimes" is not one of`},
		{"SCRY_TEST_JUDGE_DELAY", "soon", `SCRY_TEST_JUDGE_DELAY: "soon" is not a duration`},
		{"SCRY_TEST_JUDGE_DELAY", "2m", `SCRY_TEST_JUDGE_DELAY: "2m" is not a duration between 0 and 40s`},
	} {
		modeEnv(t, "test", map[string]string{bad.key: bad.value})
		err := failBoot(t, bad.says)
		if !strings.Contains(err.Error(), bad.key) {
			t.Fatal(err)
		}
	}
}
