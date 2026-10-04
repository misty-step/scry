package learning

import (
	"fmt"
	"strings"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// ReviewTransition is the validated grading state a store may persist for one
// review operation. Its fields are private so callers must use
// NewReviewTransition.
type ReviewTransition struct {
	authority string
	algorithm string
}

// NewReviewTransition validates the cross-field invariants of one review
// transition and derives its durable provenance.
func NewReviewTransition(operationID, outcome string, rating int, assisted, graded bool, policy string) (ReviewTransition, error) {
	if strings.TrimSpace(operationID) == "" {
		return ReviewTransition{}, fmt.Errorf("missing operation ID")
	}
	switch policy {
	case "exact-v1", ShortPolicyVersion, SemanticPolicyVersion, LearnerPolicyVersion:
	default:
		return ReviewTransition{}, fmt.Errorf("unsupported review policy %q", policy)
	}
	if assisted && rating == int(fsrs.Good) {
		return ReviewTransition{}, fmt.Errorf("assisted review cannot be an unaided success")
	}

	algorithm := Algorithm
	if policy != "exact-v1" {
		algorithm = EventAlgorithm(policy)
	}
	return ReviewTransition{authority: ReviewAuthority(policy, outcome, graded), algorithm: algorithm}, nil
}

// ReviewAuthority maps persisted review state to its learner-visible authority.
func ReviewAuthority(policy, outcome string, graded bool) string {
	if !graded {
		return ""
	}
	if outcome == "revealed" {
		return "reveal"
	}
	if strings.HasPrefix(outcome, "warm_") {
		return ""
	}
	switch policy {
	case "exact-v1":
		return "exact"
	case ShortPolicyVersion, SemanticPolicyVersion:
		return "jev"
	case LearnerPolicyVersion:
		return "learner"
	default:
		return ""
	}
}

func (t ReviewTransition) Authority() string { return t.authority }
func (t ReviewTransition) Algorithm() string { return t.algorithm }
