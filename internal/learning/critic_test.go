package learning

import (
	"math"
	"testing"
)

func criticValues(params CriticParams) map[string]float64 {
	values := map[string]float64{CriticSoftKey: 0.1}
	for _, key := range CriticHardKeys(params) {
		values[key] = 0.1
	}
	return values
}

func TestCriticEveryHardVetoAndFrozenBoundary(t *testing.T) {
	params := CriticParams{Source: true, Choice: true, Semantic: true}
	for _, key := range CriticHardKeys(params) {
		t.Run(key, func(t *testing.T) {
			values := criticValues(params)
			values[key] = math.Nextafter(CriticVetoThreshold, 0)
			if got := JudgeCandidate(values, params); got.Decision != "accept" {
				t.Fatalf("below threshold: %+v", got)
			}
			values[key] = CriticVetoThreshold
			got := JudgeCandidate(values, params)
			if got.Decision != "reject" || len(got.Reasons) != 1 || got.Reasons[0] != key {
				t.Fatalf("hard veto: %+v", got)
			}
		})
	}
}

func TestCriticSoftRanksButNeverRejects(t *testing.T) {
	for _, params := range []CriticParams{{}, {Source: true}, {Choice: true}, {Semantic: true}} {
		values := criticValues(params)
		values[CriticSoftKey] = 1
		got := JudgeCandidate(values, params)
		if got.Decision != "accept" || got.TeachingScore != 0 || len(got.Reasons) != 0 {
			t.Fatal(got)
		}
	}
}

func TestCriticMalformedIsUngraded(t *testing.T) {
	params := CriticParams{Source: true}
	cases := []map[string]float64{nil, {}, criticValues(params)}
	cases[2]["extra"] = 0
	for _, invalid := range []float64{math.NaN(), math.Inf(1), -.01, 1.01} {
		values := criticValues(params)
		values[CriticSoftKey] = invalid
		cases = append(cases, values)
	}
	for _, key := range append(CriticHardKeys(params), CriticSoftKey) {
		values := criticValues(params)
		delete(values, key)
		cases = append(cases, values)
	}
	for _, values := range cases {
		if got := JudgeCandidate(values, params); got.Decision != "ungraded" {
			t.Fatalf("malformed published: %+v", got)
		}
	}
	values := criticValues(params)
	values["no_defensible_answer"], values["evidence_contradicts"] = 1, 1
	if got := JudgeCandidate(values, params); len(got.Reasons) != 2 {
		t.Fatal(got)
	}
}
