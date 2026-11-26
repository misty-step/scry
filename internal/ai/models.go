package ai

import "reflect"

// The model each AI surface is configured to call, by name. Reservations
// price their worst case with it (app.BudgetMicros) and /healthz reports
// it. An adapter that is not wired (nil, or a typed nil) names no model.

// JudgeModel names the model a grader asks for: the Decisions model for
// Jev, "test-judge" for the test adapter.
func JudgeModel(m any) string {
	switch v := m.(type) {
	case Split:
		return JudgeModel(v.JudgeProvider)
	case *Jev:
		if v != nil {
			return v.RequestedModel()
		}
	case *Chain:
		if v != nil && v.Primary() != nil {
			return JudgeModel(v.Primary())
		}
		return ""
	case *Fake:
		if v != nil {
			return FakeJudgeModel
		}
	case *LLMJudge:
		if v != nil {
			return v.RequestedModel()
		}
	}
	return named(m)
}

// ChainInfo is the grading chain a grader walks, by model ([S113-1]).
type ChainInfo struct {
	Primary            string   `json:"primary"`
	Fallbacks          []string `json:"fallbacks"`
	Escalation         string   `json:"escalation"`          // "" when there is none
	EscalationProvider string   `json:"escalation_provider"` // openrouter, anthropic, test or none
}

// JudgeChain describes the chain a grader walks. A bare decisions model
// is a chain of one with no escalation; the test adapter is its fixture
// chain.
func JudgeChain(m any) ChainInfo {
	switch v := m.(type) {
	case Split:
		return JudgeChain(v.JudgeProvider)
	case *Fake:
		if v != nil {
			return JudgeChain(v.Chain())
		}
	case *Chain:
		if v == nil {
			break
		}
		info := ChainInfo{Fallbacks: []string{}, EscalationProvider: "none"}
		for i, d := range v.Decisions {
			if i == 0 {
				info.Primary = JudgeModel(d)
			} else {
				info.Fallbacks = append(info.Fallbacks, JudgeModel(d))
			}
		}
		if v.Escalation != nil && named(v.Escalation) != "" {
			info.Escalation = JudgeModel(v.Escalation)
			info.EscalationProvider = providerName(v.Escalation)
		}
		return info
	}
	return ChainInfo{Primary: JudgeModel(m), Fallbacks: []string{}, EscalationProvider: "none"}
}

// JudgeModels is every model one submission may be charged for, in the
// order the chain asks them: what a judgment reservation is sized for.
func JudgeModels(m any) []string {
	c := JudgeChain(m)
	out := append([]string{c.Primary}, c.Fallbacks...)
	if c.Escalation != "" {
		out = append(out, c.Escalation)
	}
	return out
}

// DraftModel names the model that drafts study material.
func DraftModel(m any) string {
	switch v := m.(type) {
	case Split:
		return DraftModel(v.DraftProvider)
	case *Claude:
		if v != nil {
			return v.DraftModel
		}
	case *Fake:
		if v != nil {
			return "test-draft"
		}
	}
	return named(m)
}

// ExplainModel names the model that writes on-demand explanations.
func ExplainModel(e any) string {
	switch v := e.(type) {
	case *Claude:
		if v != nil {
			return v.explainModel()
		}
	case *FakeExplainer:
		if v != nil {
			return "test-explain"
		}
	}
	return named(e)
}

// named is the fallback for adapters this file does not know: one that
// reports its own model (interface{ Model() string }) is believed, any
// other wired adapter is "unknown" (priced at the ceiling), and an
// unwired one is "".
func named(v any) string {
	if v == nil {
		return ""
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return ""
	}
	if m, ok := v.(interface{ ModelName() string }); ok {
		return m.ModelName()
	}
	if m, ok := v.(interface{ RequestedModel() string }); ok {
		return m.RequestedModel()
	}
	return "unknown"
}
