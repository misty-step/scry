package app

import (
	"reflect"
	"strings"

	"scry/internal/ai"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/speech"
)

// Readiness is what /healthz says about how this process is wired: which
// build it is, which mode, and which real adapters are in place. It holds
// adapter labels only, never a key, secret, DSN or token.
type Readiness struct {
	BuildSHA    string
	Mode        string // live or test
	Grading     bool   // the real Jev grader judges answers
	Drafting    bool   // real Claude drafts study material (via OpenRouter or Anthropic)
	Explainer   bool   // real Claude writes explanations (via OpenRouter or Anthropic)
	Speech      bool   // a real transcriber hears spoken answers (OpenRouter or OpenAI)
	BillingMode string // stripe-live, stripe-test or test
	AuthMode    string // clerk-live, clerk-test or local
	// Providers is the exact active adapter for each capability.
	Providers map[string]string
	// Warnings are deliberate but unusual wirings an operator must see,
	// such as live mode charging with Stripe test keys.
	Warnings []string
}

// Readiness reports the wiring. It makes no network call.
func (a *App) Readiness() Readiness {
	r := Readiness{BuildSHA: a.Cfg.BuildSHA, Mode: "live", Providers: map[string]string{}, Warnings: []string{}}
	if r.BuildSHA == "" {
		r.BuildSHA = "dev"
	}
	if a.Cfg.TestMode {
		r.Mode = "test"
	}

	var judge, draft any = a.Model, a.Model
	if split, ok := a.Model.(ai.Split); ok {
		judge, draft = split.JudgeProvider, split.DraftProvider
	}
	r.Providers["grading"] = judgeLabel(judge)
	r.Providers["drafting"] = label(draft)
	if j, ok := judge.(*ai.Jev); ok && j != nil {
		r.Grading = true
	}
	if c, ok := draft.(*ai.Claude); ok && c != nil {
		r.Drafting = true
	}

	r.Providers["explainer"] = label(a.Explainer)
	if c, ok := a.Explainer.(*ai.Claude); ok && c != nil {
		r.Explainer = true
	}

	r.Providers["speech"] = label(a.Speech)
	switch s := a.Speech.(type) {
	case speech.OpenRouter, speech.OpenAI:
		r.Speech = true
	case *speech.OpenRouter:
		r.Speech = s != nil
	case *speech.OpenAI:
		r.Speech = s != nil
	}

	r.BillingMode = "test"
	if a.Pay != nil {
		r.BillingMode = a.Pay.Name()
	}
	if s, ok := a.Pay.(*billing.Stripe); ok && s != nil {
		r.BillingMode = s.Name()
	}
	r.Providers["billing"] = r.BillingMode

	r.AuthMode = "local"
	if c, ok := a.Auth.(*auth.Clerk); ok && c != nil {
		r.AuthMode = c.Mode()
	}
	r.Providers["auth"] = r.AuthMode

	if !a.Cfg.TestMode && r.BillingMode == "stripe-test" {
		r.Warnings = append(r.Warnings, "billing: live mode is using Stripe test-mode keys (SCRY_STRIPE_MODE=test); no real card is charged")
	}
	if !a.Cfg.TestMode && r.AuthMode == "clerk-test" {
		r.Warnings = append(r.Warnings, "auth: live mode is signing learners in with a Clerk development instance (clerk-test keys); accounts are not the production user pool")
	}
	if a.Cfg.TestMode && r.Grading {
		r.Warnings = append(r.Warnings, "grading: test mode is calling the real Jev grader (SCRY_JUDGE_MODE=live or SCRY_AI_MODE=live); judgments cost real money")
	}
	if a.Cfg.TestMode && (r.Drafting || r.Explainer || r.Speech) {
		var real []string
		for _, c := range []struct {
			on   bool
			name string
		}{{r.Drafting, "drafting"}, {r.Explainer, "explainer"}, {r.Speech, "speech"}} {
			if c.on {
				real = append(real, c.name+" ("+r.Providers[c.name]+")")
			}
		}
		r.Warnings = append(r.Warnings, "ai: test mode is calling real AI providers (SCRY_AI_MODE=live) for "+strings.Join(real, ", ")+"; calls cost real money")
	}
	return r
}

func label(v any) string {
	if v == nil {
		return "none"
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "none" // a typed nil adapter is not wired, and calling it would panic
	}
	if n, ok := v.(interface{ Name() string }); ok {
		return n.Name()
	}
	return "unknown"
}

// judgeLabel names Jev with the route it calls, since the two routes are
// billed and fail separately.
func judgeLabel(v any) string {
	if j, ok := v.(*ai.Jev); ok && j != nil {
		if j.OpenRouter {
			return "jev-openrouter"
		}
		return "jev-typesafe"
	}
	return label(v)
}
