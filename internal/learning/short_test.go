package learning

import "testing"

// US-008: short-v1 grants success only for a confident accept that changes no
// exact value, grants a miss only for a more confident reject, and leaves
// everything uncertain or suspicious ungraded (the learner then self-checks).
func TestShortV1PolicyUS008(t *testing.T) {
	p := ShortV1Params()
	judge := func(verdict string, probability, identity, injection float64) SemanticDecision {
		return GradeShort(ShortJudgments{Verdict: verdict, Probabilities: map[string]float64{verdict: probability}, Identity: identity, Injection: injection}, p)
	}
	cases := []struct {
		name     string
		decision SemanticDecision
		want     string
		rating   int
	}{
		{"confident accept", judge("accept", 0.93, 0.05, 0.01), "correct", 3},
		{"accept at threshold", judge("accept", 0.85, 0.35, 0.20), "correct", 3},
		{"accept below threshold", judge("accept", 0.84, 0.05, 0.01), "ungraded", 0},
		{"accept that changes an exact value", judge("accept", 0.97, 0.60, 0.01), "ungraded", 0},
		{"confident reject", judge("reject", 0.95, 0.10, 0.01), "incorrect", 1},
		{"reject below its higher threshold", judge("reject", 0.88, 0.10, 0.01), "ungraded", 0},
		{"unsure", judge("unsure", 0.99, 0.10, 0.01), "ungraded", 0},
		{"injection blocks acceptance", judge("accept", 0.99, 0.01, 0.40), "ungraded", 0},
		{"injection blocks rejection", judge("reject", 0.99, 0.01, 0.40), "ungraded", 0},
		{"invalid probability", judge("accept", 1.2, 0.01, 0.01), "ungraded", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.decision.Decision != tc.want || tc.decision.Rating != tc.rating {
				t.Fatalf("got %s/%d, want %s/%d", tc.decision.Decision, tc.decision.Rating, tc.want, tc.rating)
			}
			if applied := tc.want != "ungraded"; tc.decision.Applied != applied {
				t.Fatalf("applied=%v, want %v", tc.decision.Applied, applied)
			}
		})
	}
	missing := GradeShort(ShortJudgments{Verdict: "accept", Probabilities: map[string]float64{"reject": 0.9}}, p)
	if missing.Decision != "ungraded" {
		t.Fatalf("a verdict without its probability graded as %s", missing.Decision)
	}
	wrongPolicy := p
	wrongPolicy.PolicyVersion = "short-v0"
	if GradeShort(ShortJudgments{Verdict: "accept", Probabilities: map[string]float64{"accept": 0.99}}, wrongPolicy).Applied {
		t.Fatal("an unknown policy version granted authority")
	}
}
