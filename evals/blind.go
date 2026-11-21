package evals

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// BlindLabels is "Model A", "Model B", ... for n models.
func BlindLabels(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Model %c", 'A'+i)
	}
	return out
}

// BlindAssign deterministically shuffles models for one case and returns
// label -> model. The permutation is a Fisher-Yates shuffle driven by
// SHA-256(seed, surface, case, step), so it is stable across runs, Go
// versions and machines, differs between cases, and can be re-derived by
// anyone holding the seed.
func BlindAssign(seed, surface, caseID string, models []string) map[string]string {
	perm := append([]string(nil), models...)
	sort.Strings(perm) // the order models were listed in must not leak
	for i := len(perm) - 1; i > 0; i-- {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", seed, surface, caseID, i)))
		j := int(binary.BigEndian.Uint64(h[:8]) % uint64(i+1))
		perm[i], perm[j] = perm[j], perm[i]
	}
	out := map[string]string{}
	for i, label := range BlindLabels(len(perm)) {
		out[label] = perm[i]
	}
	return out
}

// BlindOutput is one anonymized output.
type BlindOutput struct {
	ModelLabel string `json:"model_label"`
	Status     string `json:"status"` // ok, or why there is no output
	Output     any    `json:"output"`
}

// BlindSet is one case's side-by-side outputs under blind labels.
type BlindSet struct {
	Surface string         `json:"surface"`
	CaseID  string         `json:"case_id"`
	Context map[string]any `json:"context"`
	Outputs []BlindOutput  `json:"outputs"`
}

// BlindKey is the sealed provenance: surface/case -> label -> model slug.
type BlindKey struct {
	Seed string                       `json:"seed"`
	Note string                       `json:"note"`
	Sets map[string]map[string]string `json:"sets"`
}

// BuildBlind lays out one case. outputs maps model -> (status, output).
func BuildBlind(seed, surface, caseID string, context map[string]any, models []string, outputs map[string]BlindOutput) (BlindSet, map[string]string) {
	assign := BlindAssign(seed, surface, caseID, models)
	set := BlindSet{Surface: surface, CaseID: caseID, Context: context}
	for _, label := range BlindLabels(len(models)) {
		o, ok := outputs[assign[label]]
		if !ok {
			o = BlindOutput{Status: "not run"}
		}
		o.ModelLabel = label
		set.Outputs = append(set.Outputs, o)
	}
	return set, assign
}
