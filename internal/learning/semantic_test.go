package learning

import (
	"strings"
	"testing"
)

func TestGradeSemanticPolicy(t *testing.T) {
	base := SemanticJudgments{
		Ideas:                 []float64{0.95, 0.91},
		Contradictions:        []float64{0.05},
		Relation:              "equivalent",
		RelationProbabilities: map[string]float64{"equivalent": 0.92, "partial": 0.04, "different": 0.02, "unclear": 0.02},
		Injection:             0.01,
	}
	cases := []struct {
		name      string
		judgments SemanticJudgments
		params    Params
		decision  string
		applied   bool
		outcome   string
		rating    int
		missing   int
	}{
		{name: "accept", judgments: base, params: SemanticV1Params(), decision: "correct", applied: true, outcome: "correct", rating: 3, missing: -1},
		{name: "relation below frozen threshold", judgments: SemanticJudgments{
			Ideas: []float64{0.95, 0.91}, Contradictions: []float64{0.05}, Relation: "equivalent",
			RelationProbabilities: map[string]float64{"equivalent": 0.80}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "ungraded", outcome: "ungraded", missing: -1},
		{name: "incomplete is shadow by default", judgments: SemanticJudgments{
			Ideas: []float64{0.94, 0.20}, Contradictions: []float64{0.05}, Relation: "partial",
			RelationProbabilities: map[string]float64{"partial": 0.88}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "incomplete", applied: false, outcome: "ungraded", missing: 1},
		{name: "contradiction is shadow by default", judgments: SemanticJudgments{
			Ideas: []float64{0.10, 0.10}, Contradictions: []float64{0.96}, Relation: "different",
			RelationProbabilities: map[string]float64{"different": 0.94}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "incorrect", applied: false, outcome: "ungraded", missing: -1},
		{name: "unclear", judgments: SemanticJudgments{
			Ideas: []float64{0.55, 0.50}, Contradictions: []float64{0.10}, Relation: "unclear",
			RelationProbabilities: map[string]float64{"unclear": 0.95}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "ungraded", outcome: "ungraded", missing: -1},
		{name: "injection", judgments: SemanticJudgments{
			Ideas: []float64{0.99, 0.99}, Contradictions: []float64{0.01}, Relation: "equivalent",
			RelationProbabilities: map[string]float64{"equivalent": 0.99}, Injection: 0.95,
		}, params: SemanticV1Params(), decision: "ungraded", outcome: "ungraded", missing: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GradeSemantic(tc.judgments, tc.params)
			if got.Decision != tc.decision || got.Applied != tc.applied || got.Outcome != tc.outcome || got.Rating != tc.rating || got.MissingIdea != tc.missing {
				t.Fatalf("got %+v, want decision=%s applied=%v outcome=%s rating=%d missing=%d", got, tc.decision, tc.applied, tc.outcome, tc.rating, tc.missing)
			}
		})
	}
}

func TestGradeSemanticShadowClassesCanBeExplicitlyEnabled(t *testing.T) {
	params := SemanticV1Params()
	params.IncorrectEnabled, params.IncompleteEnabled = true, true
	incorrect := GradeSemantic(SemanticJudgments{
		Ideas: []float64{0.10}, Contradictions: []float64{0.94}, Relation: "different",
		RelationProbabilities: map[string]float64{"different": 0.90}, Injection: 0.01,
	}, params)
	if incorrect.Decision != "incorrect" || !incorrect.Applied || incorrect.Outcome != "wrong" || incorrect.Rating != 1 || incorrect.Contradiction != 0 {
		t.Fatalf("enabled incorrect: %+v", incorrect)
	}
	incomplete := GradeSemantic(SemanticJudgments{
		Ideas: []float64{0.94, 0.20}, Contradictions: []float64{0.05}, Relation: "partial",
		RelationProbabilities: map[string]float64{"partial": 0.88}, Injection: 0.01,
	}, params)
	if incomplete.Decision != "incomplete" || !incomplete.Applied || incomplete.Outcome != "incomplete" || incomplete.Rating != 0 || incomplete.MissingIdea != 1 {
		t.Fatalf("enabled incomplete: %+v", incomplete)
	}
}

func TestSemanticV1ParamsAreFrozenAndNeverLowered(t *testing.T) {
	p := SemanticV1Params()
	if p.RelationThreshold != 0.85 || p.IdeaThreshold != 0.80 || p.ContradictionHigh != 0.90 || p.IncompleteEnabled || p.IncorrectEnabled {
		t.Fatalf("semantic-v1 parameters drifted: %+v", p)
	}
}

func TestEventAlgorithmSeparatesSchedulerFromGradingPolicy(t *testing.T) {
	if Algorithm != Scheduler+";grading=exact-v1" {
		t.Fatalf("historical Algorithm identity changed: %q", Algorithm)
	}
	if Algorithm != "go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m;grading=exact-v1" {
		t.Fatalf("frozen Algorithm value changed: %q", Algorithm)
	}
	if EventAlgorithm("exact-v1") != Algorithm {
		t.Fatalf("exact events must keep the historical identity: %q", EventAlgorithm("exact-v1"))
	}
	semantic := EventAlgorithm(SemanticPolicyVersion)
	if !strings.HasPrefix(semantic, Scheduler+";grading=") || !strings.HasSuffix(semantic, ";grading=semantic-v1") || semantic == Algorithm {
		t.Fatalf("semantic event identity is not accurate: %q", semantic)
	}
}
