package learning

// SemanticPolicyVersion identifies the frozen semantic grading policy without
// changing Algorithm, which continues to identify the scheduler contract.
const SemanticPolicyVersion = "semantic-v1"

// Rubric is immutable quiz content. Its version is the containing quiz content
// version; it is not a separately mutable grading object.
type Rubric struct {
	Required       []RubricIdea  `json:"required"`
	Contradictions []RubricClaim `json:"contradictions,omitempty"`
}

type RubricIdea struct {
	Text string `json:"text"`
	Cue  string `json:"cue,omitempty"`
}

type RubricClaim struct {
	Text     string `json:"text"`
	Feedback string `json:"feedback,omitempty"`
}

// Params freezes the code-owned thresholds used to turn Jev judgments into a
// scheduling decision. Incomplete and incorrect are shadow classes until holdout
// evidence supports granting Jev authority over cues or misses: the decision is
// recorded, but the learner sees plain ungraded and no assistance is charged.
type Params struct {
	PolicyVersion            string
	IdeaThreshold            float64
	IdeaLowThreshold         float64
	ContradictionLow         float64
	ContradictionHigh        float64
	RelationThreshold        float64
	PartialRelationThreshold float64
	InjectionThreshold       float64
	IncompleteEnabled        bool
	IncorrectEnabled         bool
}

// SemanticV1Params is the single source of the semantic-v1 policy parameters
// authorized for the bounded rollout. RelationThreshold was raised from the
// initial 0.75 to the tune-split frozen 0.85 on 2026-09-22; no threshold was
// ever lowered to widen acceptance.
func SemanticV1Params() Params {
	return Params{
		PolicyVersion:            SemanticPolicyVersion,
		IdeaThreshold:            0.80,
		IdeaLowThreshold:         0.35,
		ContradictionLow:         0.20,
		ContradictionHigh:        0.90,
		RelationThreshold:        0.85,
		PartialRelationThreshold: 0.60,
		InjectionThreshold:       0.20,
		IncompleteEnabled:        false,
		IncorrectEnabled:         false,
	}
}

// SemanticJudgments is the minimal provider-independent evidence consumed by
// semantic-v1. Slice order matches the authored rubric.
type SemanticJudgments struct {
	Ideas                 []float64
	Contradictions        []float64
	Relation              string
	RelationProbabilities map[string]float64
	Injection             float64
}

// SemanticDecision is policy output, not provider output. Decision names the
// policy class the evidence supports. Applied reports whether these Params grant
// that class authority over the occurrence; a shadow class is recorded for
// evaluation but the learner outcome stays ungraded with no assistance charged.
// MissingIdea and Contradiction are rubric indexes; -1 means no authored
// cue/feedback applies.
type SemanticDecision struct {
	Decision      string
	Applied       bool
	Outcome       string
	Rating        int
	MissingIdea   int
	Contradiction int
}

func shadow(d SemanticDecision) SemanticDecision {
	d.Applied, d.Outcome, d.Rating = false, "ungraded", 0
	return d
}

// GradeSemantic applies semantic-v1 without HTTP, storage, logging, or other
// side effects. Invalid, incomplete, or uncertain judgment sets remain
// ungraded rather than manufacturing success or failure.
func GradeSemantic(j SemanticJudgments, p Params) SemanticDecision {
	ungraded := SemanticDecision{Decision: "ungraded", Outcome: "ungraded", MissingIdea: -1, Contradiction: -1}
	if p.PolicyVersion != SemanticPolicyVersion || len(j.Ideas) == 0 || !validParams(p) || !validProbability(j.Injection) {
		return ungraded
	}
	for _, probability := range j.Ideas {
		if !validProbability(probability) {
			return ungraded
		}
	}
	for _, probability := range j.Contradictions {
		if !validProbability(probability) {
			return ungraded
		}
	}
	for _, probability := range j.RelationProbabilities {
		if !validProbability(probability) {
			return ungraded
		}
	}
	relationProbability, ok := j.RelationProbabilities[j.Relation]
	if !ok {
		return ungraded
	}

	ideasComplete := true
	for _, probability := range j.Ideas {
		ideasComplete = ideasComplete && probability >= p.IdeaThreshold
	}
	contradictionsLow := true
	for _, probability := range j.Contradictions {
		contradictionsLow = contradictionsLow && probability <= p.ContradictionLow
	}
	if ideasComplete && contradictionsLow && j.Relation == "equivalent" && relationProbability >= p.RelationThreshold && j.Injection <= p.InjectionThreshold {
		return SemanticDecision{Decision: "correct", Applied: true, Outcome: "correct", Rating: 3, MissingIdea: -1, Contradiction: -1}
	}

	// Shadow classes are always evaluated so the recorded decision is comparable
	// with holdout evidence; only enabled classes gain authority.
	if j.Relation == "different" && relationProbability >= p.RelationThreshold {
		for index, probability := range j.Contradictions {
			if probability >= p.ContradictionHigh {
				decision := SemanticDecision{Decision: "incorrect", Applied: true, Outcome: "wrong", Rating: 1, MissingIdea: -1, Contradiction: index}
				if !p.IncorrectEnabled {
					return shadow(decision)
				}
				return decision
			}
		}
	}

	if contradictionsLow && j.Relation == "partial" && relationProbability >= p.PartialRelationThreshold {
		missing := -1
		for index, probability := range j.Ideas {
			switch {
			case probability <= p.IdeaLowThreshold && missing == -1:
				missing = index
			case probability < p.IdeaThreshold:
				missing = -1
				return ungraded
			}
		}
		if missing >= 0 {
			decision := SemanticDecision{Decision: "incomplete", Applied: true, Outcome: "incomplete", MissingIdea: missing, Contradiction: -1}
			if !p.IncompleteEnabled {
				return shadow(decision)
			}
			return decision
		}
	}
	return ungraded
}

func validParams(p Params) bool {
	return validProbability(p.IdeaThreshold) && validProbability(p.IdeaLowThreshold) &&
		validProbability(p.ContradictionLow) && validProbability(p.ContradictionHigh) &&
		validProbability(p.RelationThreshold) && validProbability(p.PartialRelationThreshold) &&
		validProbability(p.InjectionThreshold) && p.IdeaLowThreshold <= p.IdeaThreshold &&
		p.ContradictionLow <= p.ContradictionHigh
}

func validProbability(value float64) bool { return value >= 0 && value <= 1 }
