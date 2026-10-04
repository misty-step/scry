package learning

import (
	"testing"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

func TestNewReviewTransitionRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name        string
		operationID string
		rating      int
		assisted    bool
		policy      string
	}{
		{name: "assisted unaided success", operationID: "assisted", rating: int(fsrs.Good), assisted: true, policy: "exact-v1"},
		{name: "missing operation ID", policy: "exact-v1"},
		{name: "unknown policy", operationID: "unknown-policy", policy: "other-v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewReviewTransition(test.operationID, "correct", test.rating, test.assisted, true, test.policy); err == nil {
				t.Fatal("invalid review transition was accepted")
			}
		})
	}
}

func TestReviewTransitionHeldExactHasNoAuthority(t *testing.T) {
	transition, err := NewReviewTransition("held-exact", "ungraded", 0, false, false, "exact-v1")
	if err != nil {
		t.Fatal(err)
	}
	if transition.Authority() != "" {
		t.Fatalf("held exact attempt has authority %q", transition.Authority())
	}
}
