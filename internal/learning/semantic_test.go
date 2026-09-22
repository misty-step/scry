package learning

import "testing"

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
		rating    int
		missing   int
	}{
		{name: "accept", judgments: base, params: SemanticV1Params(), decision: "correct", rating: 3, missing: -1},
		{name: "incomplete", judgments: SemanticJudgments{
			Ideas: []float64{0.94, 0.20}, Contradictions: []float64{0.05}, Relation: "partial",
			RelationProbabilities: map[string]float64{"partial": 0.88}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "incomplete", missing: 1},
		{name: "contradiction disabled", judgments: SemanticJudgments{
			Ideas: []float64{0.10, 0.10}, Contradictions: []float64{0.96}, Relation: "different",
			RelationProbabilities: map[string]float64{"different": 0.94}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "ungraded", missing: -1},
		{name: "unclear", judgments: SemanticJudgments{
			Ideas: []float64{0.55, 0.50}, Contradictions: []float64{0.10}, Relation: "unclear",
			RelationProbabilities: map[string]float64{"unclear": 0.95}, Injection: 0.01,
		}, params: SemanticV1Params(), decision: "ungraded", missing: -1},
		{name: "injection", judgments: SemanticJudgments{
			Ideas: []float64{0.99, 0.99}, Contradictions: []float64{0.01}, Relation: "equivalent",
			RelationProbabilities: map[string]float64{"equivalent": 0.99}, Injection: 0.95,
		}, params: SemanticV1Params(), decision: "ungraded", missing: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GradeSemantic(tc.judgments, tc.params)
			if got.Decision != tc.decision || got.Rating != tc.rating || got.MissingIdea != tc.missing {
				t.Fatalf("got %+v, want decision=%s rating=%d missing=%d", got, tc.decision, tc.rating, tc.missing)
			}
		})
	}
}

func TestGradeSemanticIncorrectCanBeExplicitlyEnabled(t *testing.T) {
	params := SemanticV1Params()
	params.IncorrectEnabled = true
	got := GradeSemantic(SemanticJudgments{
		Ideas: []float64{0.10}, Contradictions: []float64{0.94}, Relation: "different",
		RelationProbabilities: map[string]float64{"different": 0.90}, Injection: 0.01,
	}, params)
	if got.Decision != "incorrect" || got.Rating != 1 || got.Contradiction != 0 {
		t.Fatalf("got %+v", got)
	}
}
