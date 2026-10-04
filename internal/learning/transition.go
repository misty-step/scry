package learning

import (
	"fmt"
	"strings"
)

// ReviewTransition is the validated grading state a store may persist for one
// review operation. Its fields are private so callers must use
// NewReviewTransition.
type ReviewTransition struct {
	outcome   string
	rating    int
	authority string
	algorithm string
}

// NewReviewTransition validates the cross-field invariants of one review
// transition and derives its durable provenance.
func NewReviewTransition(operationID, outcome string, rating int, assisted bool, policy string) (ReviewTransition, error) {
	if strings.TrimSpace(operationID) == "" {
		return ReviewTransition{}, fmt.Errorf("missing operation ID")
	}
	switch policy {
	case "exact-v1", ShortPolicyVersion, SemanticPolicyVersion, LearnerPolicyVersion:
	default:
		return ReviewTransition{}, fmt.Errorf("unsupported review policy %q", policy)
	}
	if assisted && rating == 3 {
		return ReviewTransition{}, fmt.Errorf("assisted review cannot be an unaided success")
	}

	authority := ""
	switch {
	case outcome == "revealed":
		authority = "reveal"
	case strings.HasPrefix(outcome, "warm_"):
	case policy == "exact-v1":
		authority = "exact"
	case policy == ShortPolicyVersion || policy == SemanticPolicyVersion:
		authority = "jev"
	case policy == LearnerPolicyVersion:
		authority = "learner"
	}

	algorithm := Algorithm
	if policy != "exact-v1" {
		algorithm = EventAlgorithm(policy)
	}
	return ReviewTransition{outcome: outcome, rating: rating, authority: authority, algorithm: algorithm}, nil
}

func (t ReviewTransition) Outcome() string   { return t.outcome }
func (t ReviewTransition) Rating() int       { return t.rating }
func (t ReviewTransition) Authority() string { return t.authority }
func (t ReviewTransition) Algorithm() string { return t.algorithm }
