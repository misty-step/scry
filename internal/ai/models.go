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
	case *Fake:
		if v != nil {
			return "test-judge"
		}
	}
	return named(m)
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
	return "unknown"
}
