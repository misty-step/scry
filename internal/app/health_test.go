package app_test

import (
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/speech"
)

// Slice 20: readiness counts the OpenRouter and direct adapters alike as
// real, labels each route, and warns about every deliberate unusual wiring.
func TestSlice20_ReadinessLabelsBothRoutes(t *testing.T) {
	router, _ := ai.NewClaudeWithRouter("", "or-standin", "", "")
	direct, _ := ai.NewClaudeWithRouter("ant-standin", "", "", "")
	for _, tc := range []struct {
		name                      string
		judge                     *ai.Jev
		claude                    *ai.Claude
		ear                       speech.Transcriber
		grading, drafting, speech string
	}{
		{"openrouter", &ai.Jev{APIKey: "or-standin", OpenRouter: true}, router, speech.OpenRouter{APIKey: "or-standin"}, "jev-openrouter", "openrouter", "openrouter"},
		{"openrouter pointer speech", &ai.Jev{APIKey: "or-standin", OpenRouter: true}, router, &speech.OpenRouter{APIKey: "or-standin"}, "jev-openrouter", "openrouter", "openrouter"},
		{"direct", &ai.Jev{APIKey: "ts-standin"}, direct, speech.OpenAI{APIKey: "oai-standin"}, "jev-typesafe", "anthropic", "openai"},
		{"direct pointer speech", &ai.Jev{APIKey: "ts-standin"}, direct, &speech.OpenAI{APIKey: "oai-standin"}, "jev-typesafe", "anthropic", "openai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app.App{Model: ai.Split{JudgeProvider: tc.judge, DraftProvider: tc.claude}, Explainer: tc.claude, Speech: tc.ear}
			r := a.Readiness()
			if !r.Grading || !r.Drafting || !r.Explainer || !r.Speech {
				t.Fatalf("%+v", r)
			}
			if r.Providers["grading"] != tc.grading || r.Providers["drafting"] != tc.drafting || r.Providers["explainer"] != tc.drafting || r.Providers["speech"] != tc.speech {
				t.Fatalf("%v", r.Providers)
			}
			if len(r.Warnings) != 0 {
				t.Fatalf("live wiring warned: %v", r.Warnings)
			}
			a.Cfg.TestMode = true
			ws := strings.Join(a.Readiness().Warnings, "\n")
			if !strings.Contains(ws, "real Jev grader") || !strings.Contains(ws, "SCRY_AI_MODE=live") || !strings.Contains(ws, "speech ("+tc.speech+")") {
				t.Fatalf("test-mode warnings %q", ws)
			}
		})
	}
	t.Run("nil pointer speech is not real", func(t *testing.T) {
		var o *speech.OpenRouter
		if (&app.App{Speech: o}).Readiness().Speech {
			t.Fatal("nil adapter counted as real")
		}
	})
	t.Run("fakes are not real and do not warn", func(t *testing.T) {
		a := &app.App{Model: &ai.Fake{}, Explainer: &ai.FakeExplainer{}, Speech: speech.Fake{}}
		a.Cfg.TestMode = true
		if r := a.Readiness(); r.Grading || r.Drafting || r.Explainer || r.Speech || len(r.Warnings) != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("live mode on a Clerk development instance warns", func(t *testing.T) {
		a := &app.App{Auth: &auth.Clerk{PublishableKey: "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk", SecretKey: "sk_test_standin"}}
		r := a.Readiness()
		if r.AuthMode != "clerk-test" || !strings.Contains(strings.Join(r.Warnings, "\n"), "Clerk development instance") {
			t.Fatalf("%+v", r)
		}
		for _, w := range r.Warnings {
			if strings.Contains(w, "pk_") || strings.Contains(w, "sk_") || strings.Contains(w, "standin") {
				t.Fatalf("warning leaks a key: %q", w)
			}
		}
	})
}
