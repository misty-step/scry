package app

import (
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
	Drafting    bool   // real Claude drafts study material
	Explainer   bool   // real Claude writes explanations
	Speech      bool   // real OpenAI transcribes spoken answers
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
	switch a.Speech.(type) {
	case speech.OpenAI, *speech.OpenAI:
		r.Speech = true
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
	if a.Cfg.TestMode && r.Grading {
		r.Warnings = append(r.Warnings, "grading: test mode is calling the real Jev grader (SCRY_JUDGE_MODE=live); judgments cost real money")
	}
	return r
}

func label(v any) string {
	if v == nil {
		return "none"
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
