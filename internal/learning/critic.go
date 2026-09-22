package learning

// CriticPolicyVersion identifies the initial, code-owned publication policy.
// These thresholds are frozen policy, not a claim of calibrated model accuracy.
const CriticPolicyVersion = "critic-v1"
const CriticVetoThreshold = 0.80
const CriticSoftKey = "explanation_restates_without_teaching"

// CriticParams selects applicability, never provider-supplied thresholds.
type CriticParams struct {
	Source   bool
	Choice   bool
	Semantic bool
}

type CriticDecision struct {
	Decision      string   `json:"decision"`
	Reasons       []string `json:"reasons"`
	TeachingScore float64  `json:"teaching_score"`
}

// CriticHardKeys returns an ordered battery of independent defect judgments.
func CriticHardKeys(params CriticParams) []string {
	keys := []string{"evidence_contradicts", "qualification_changed", "not_answerable_from_context", "multiple_defensible_answers", "no_defensible_answer", "prompt_leaks_answer", "adversarial_content"}
	if params.Source {
		keys = append(keys, "unsupported_by_evidence")
	}
	if params.Choice {
		keys = append(keys, "mcq_options_overlap")
	}
	if params.Semantic {
		keys = append(keys, "rubric_misaligned")
	}
	return keys
}

// JudgeCandidate requires the entire applicable battery, including the soft
// dimension. Invalid or missing data cannot authorize publication. A soft
// judgment only ranks teaching value; it never vetoes a candidate.
func JudgeCandidate(judgments map[string]float64, params CriticParams) CriticDecision {
	result := CriticDecision{Decision: "ungraded", Reasons: []string{}}
	keys := CriticHardKeys(params)
	if len(judgments) != len(keys)+1 {
		return result
	}
	for _, key := range append(append([]string(nil), keys...), CriticSoftKey) {
		value, ok := judgments[key]
		if !ok || !(value >= 0 && value <= 1) {
			return result
		}
	}
	result.Decision = "accept"
	result.TeachingScore = 1 - judgments[CriticSoftKey]
	for _, key := range keys {
		if judgments[key] >= CriticVetoThreshold {
			result.Decision = "reject"
			result.Reasons = append(result.Reasons, key)
		}
	}
	return result
}
