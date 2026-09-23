package learning

// ShortPolicyVersion identifies the short-answer meaning check for recall
// questions whose answer form is flexible: different wording, articles,
// abbreviations, or small slips may count when a careful teacher would accept
// them, while a changed exact value never does.
const ShortPolicyVersion = "short-v1"

// LearnerPolicyVersion names grades the learner gives when comparing their saved
// answer with the key (self-check) or when overriding an automatic grade.
const LearnerPolicyVersion = "learner-v1"

// ShortJudgments is the provider-independent evidence consumed by short-v1:
// one Choice (accept, reject, unsure) and two Nouls.
type ShortJudgments struct {
	Verdict       string
	Probabilities map[string]float64
	Identity      float64
	Injection     float64
}

// ShortParams freezes the code-owned thresholds for short-v1.
type ShortParams struct {
	PolicyVersion   string
	AcceptThreshold float64
	RejectThreshold float64
	IdentityMax     float64
	InjectionMax    float64
}

// ShortV1Params is the single source of the short-v1 thresholds. Rejection
// needs more confidence than acceptance because a learner can contest either,
// but a false miss also pulls the schedule forward.
func ShortV1Params() ShortParams {
	return ShortParams{PolicyVersion: ShortPolicyVersion, AcceptThreshold: 0.85, RejectThreshold: 0.90, IdentityMax: 0.35, InjectionMax: 0.20}
}

// GradeShort applies short-v1 without side effects. Anything uncertain,
// malformed, or suspicious is ungraded; the store then asks the learner to
// compare their saved answer with the key instead of inventing a result.
func GradeShort(j ShortJudgments, p ShortParams) SemanticDecision {
	ungraded := SemanticDecision{Decision: "ungraded", Outcome: "ungraded", MissingIdea: -1, Contradiction: -1}
	if p.PolicyVersion != ShortPolicyVersion || !validShortParams(p) || !validProbability(j.Identity) || !validProbability(j.Injection) {
		return ungraded
	}
	for _, probability := range j.Probabilities {
		if !validProbability(probability) {
			return ungraded
		}
	}
	probability, ok := j.Probabilities[j.Verdict]
	if !ok || j.Injection > p.InjectionMax {
		return ungraded
	}
	switch j.Verdict {
	case "accept":
		if probability >= p.AcceptThreshold && j.Identity <= p.IdentityMax {
			return SemanticDecision{Decision: "correct", Applied: true, Outcome: "correct", Rating: 3, MissingIdea: -1, Contradiction: -1}
		}
	case "reject":
		if probability >= p.RejectThreshold {
			return SemanticDecision{Decision: "incorrect", Applied: true, Outcome: "wrong", Rating: 1, MissingIdea: -1, Contradiction: -1}
		}
	}
	return ungraded
}

func validShortParams(p ShortParams) bool {
	return validProbability(p.AcceptThreshold) && validProbability(p.RejectThreshold) &&
		validProbability(p.IdentityMax) && validProbability(p.InjectionMax)
}
