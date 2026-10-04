package learning

import "testing"

func TestPolicyLockRecallNeedsAssessmentUnlessExact(t *testing.T) {
	outcome, rating := Grade("recall", "Water", nil, "water", false)
	if outcome != "ungraded" || outcome == "correct" || outcome == "wrong" || rating != 0 {
		t.Fatalf("non-exact recall resolved locally: outcome=%q rating=%d", outcome, rating)
	}
}

func TestPolicyLockRevealNeverBecomesCorrect(t *testing.T) {
	outcome, _ := Grade("recall", "Water", nil, "water", true)
	if outcome != "revealed" || outcome == "correct" {
		t.Fatalf("reveal was not preserved: outcome=%q", outcome)
	}
}
