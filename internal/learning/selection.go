package learning

import (
	"sort"
	"time"
)

// SelectionPolicy is an inspectable heuristic, separate from quiz-owned FSRS.
// It makes no calibrated effectiveness claim and never changes a card.
const SelectionPolicy = "knowledge-selection-v1;direct-exact-all-units;other-material;same-or-stronger;fixed-min-due-7d;paced-mixed"

const (
	DeferralHorizon       = 7 * 24 * time.Hour
	DefaultPlanSeconds    = 300
	DefaultNewAssessments = 5
)

type DeferralInput struct {
	MaterialID      string        `json:"material_id"`
	MaterialVersion int           `json:"material_version"`
	Mode            string        `json:"mode"`
	Units           []UnitVersion `json:"units"`
	AsOf            time.Time     `json:"as_of"`
	Ambiguous       bool          `json:"ambiguous"`
	Unmapped        bool          `json:"unmapped"`
	Evidence        []Evidence    `json:"evidence"`
}

type Deferral struct {
	Deferred        bool          `json:"deferred"`
	MaterialID      string        `json:"material_id"`
	MaterialVersion int           `json:"material_version"`
	Policy          string        `json:"policy"`
	Reason          string        `json:"reason"`
	EvidenceIDs     []string      `json:"evidence_ids"`
	Units           []UnitVersion `json:"units"`
	ReconsiderAt    time.Time     `json:"reconsider_at"`
}

// DeferPractice requires a recent actual direct success for every exact target,
// from other material. Graph support and ungraded exposure cannot defer a probe.
// ReconsiderAt comes from original event/card timestamps, never the read time.
func DeferPractice(input DeferralInput) Deferral {
	result := Deferral{MaterialID: input.MaterialID, MaterialVersion: input.MaterialVersion, Policy: SelectionPolicy, Reason: "No eligible direct evidence covers every exact assessed unit version."}
	if input.MaterialID == "" || input.MaterialVersion < 1 || modeStrength(input.Mode) == 0 || input.AsOf.IsZero() || input.Ambiguous || input.Unmapped || len(input.Units) == 0 {
		result.Reason = "Unmapped, ambiguous, or invalid targets cannot be deferred."
		return result
	}
	units := make(map[UnitVersion]bool, len(input.Units))
	for _, unit := range input.Units {
		if !validUnit(unit) {
			result.Reason = "Every assessed target requires an exact unit version."
			return result
		}
		units[unit] = true
	}
	for unit := range units {
		result.Units = append(result.Units, unit)
	}
	sort.Slice(result.Units, func(i, j int) bool {
		if result.Units[i].ID == result.Units[j].ID {
			return result.Units[i].Version < result.Units[j].Version
		}
		return result.Units[i].ID < result.Units[j].ID
	})
	events := uniqueEvidence(input.Evidence, input.AsOf)
	for _, unit := range result.Units {
		var latest *Evidence
		var success *Evidence
		for i := range events {
			e := &events[i]
			assesses, touches := false, false
			for _, target := range e.Targets {
				if target.UnitID == unit.ID && target.UnitVersion == unit.Version {
					touches = true
					assesses = assesses || target.Role == "assesses"
				}
			}
			if !touches {
				continue
			}
			if assesses || e.Assisted || e.Disputed || e.Ambiguous {
				if latest == nil || laterEvidence(*e, *latest) {
					latest = e
				}
			}
			if !assesses || e.MaterialID == input.MaterialID || !applicableCard(*e) || modeStrength(e.Mode) < modeStrength(input.Mode) {
				continue
			}
			bound := e.At.Add(DeferralHorizon)
			if e.DueAt.Before(bound) {
				bound = e.DueAt
			}
			if !bound.After(input.AsOf) {
				continue
			}
			if success == nil || laterEvidence(*e, *success) {
				success = e
			}
		}
		if success == nil {
			return result
		}
		if latest != nil && (laterEvidence(*latest, *success) || latest.ID == success.ID) && (!directSuccess(*latest) || modeStrength(latest.Mode) < modeStrength(input.Mode)) {
			result.Reason = "A later relevant failure, dispute, assistance, or weaker/ambiguous response requires reconsidering this exact target."
			return result
		}
		bound := success.At.Add(DeferralHorizon)
		if success.DueAt.Before(bound) {
			bound = success.DueAt
		}
		if result.ReconsiderAt.IsZero() || bound.Before(result.ReconsiderAt) {
			result.ReconsiderAt = bound
		}
		result.EvidenceIDs = append(result.EvidenceIDs, success.ID)
	}
	result.EvidenceIDs = uniqueStrings(result.EvidenceIDs)
	result.Deferred = true
	result.Reason = "Other material directly assessed every exact target successfully in the same or stronger context. Reconsider by the earliest supporting actual due date or seven days after its observation; no FSRS state changed."
	return result
}

type Candidate struct {
	ID               string        `json:"id"`
	Version          int           `json:"version"`
	Kind             string        `json:"kind"`
	Mode             string        `json:"mode"`
	Level            string        `json:"level"`
	EstimatedSeconds int           `json:"estimated_seconds"`
	AvailableAt      time.Time     `json:"available_at"`
	FirstPresentedAt time.Time     `json:"first_presented_at"`
	Assessed         bool          `json:"assessed"`
	Continued        bool          `json:"continued"`
	ReadyProbe       bool          `json:"ready_probe"`
	PacingOverride   bool          `json:"pacing_override"`
	PacingOverrideID string        `json:"pacing_override_id,omitempty"`
	DueAt            time.Time     `json:"due_at"`
	Selected         bool          `json:"selected"`
	Priority         int           `json:"priority"`
	Units            []UnitVersion `json:"units"`
	Gap              bool          `json:"gap"`
	Deferred         bool          `json:"deferred"`
}

type Pacing struct {
	AsOf                time.Time `json:"as_of"`
	AvailableSeconds    int       `json:"available_seconds"`
	SpentSeconds        int       `json:"spent_seconds"`
	NewAssessments      int       `json:"new_assessments"`
	NewAssessmentsToday int       `json:"new_assessments_today"`
}

type Selection struct {
	MaterialID string `json:"material_id"`
	Policy     string `json:"policy"`
	Reason     string `json:"reason"`
}

// SelectMaterial separates library availability, actual introduction, and due
// practice. New instruction precedes new assessment; due retrieval and observed
// gaps remain useful alternatives. Stable identity breaks ties without randomness.
func SelectMaterial(candidates []Candidate, pacing Pacing) Selection {
	result := Selection{Policy: SelectionPolicy, Reason: "No selected material fits current availability, due state, and explicit pacing."}
	remaining := pacing.AvailableSeconds - pacing.SpentSeconds
	var best *Candidate
	for i := range candidates {
		candidate := &candidates[i]
		if !candidate.Selected || candidate.Deferred || candidate.ID == "" || candidate.Version < 1 || candidate.EstimatedSeconds < 1 || (!candidate.PacingOverride && candidate.EstimatedSeconds > remaining) || candidate.AvailableAt.After(pacing.AsOf) {
			continue
		}
		if candidate.Kind == "quiz" {
			if candidate.DueAt.IsZero() || candidate.DueAt.After(pacing.AsOf) {
				continue
			}
			if !candidate.Assessed {
				if !candidate.PacingOverride && pacing.NewAssessmentsToday >= pacing.NewAssessments {
					continue
				}
			}
		} else if candidate.Continued && !candidate.Gap {
			continue
		}
		if best == nil || preferCandidate(*candidate, *best) {
			best = candidate
		}
	}
	if best == nil {
		return result
	}
	result.MaterialID = best.ID
	switch {
	case best.Gap && best.Kind != "quiz":
		result.Reason = "Selected instruction addresses an observed gap within the chosen goal and available time."
	case best.Kind == "quiz" && best.Assessed:
		result.Reason = "An actually attempted assessment is due on its unchanged direct FSRS schedule."
	case best.ReadyProbe:
		result.Reason = "A probe of the exact foundation just taught can distinguish exposure from demonstrated performance."
	case best.Kind != "quiz":
		result.Reason = "Introduce selected instruction before additional new assessment, within the chosen time budget."
	case best.Level == "foundation":
		result.Reason = "A foundation probe can resolve unknown or uncertain knowledge without assuming a gap."
	default:
		result.Reason = "Selected target or composition practice fits the goal, time budget, and new-assessment allowance."
	}
	return result
}

func preferCandidate(a, b Candidate) bool {
	rank := func(c Candidate) int {
		if c.Gap && c.Kind != "quiz" {
			return 0
		}
		if c.Kind == "quiz" && c.Assessed {
			return 1
		}
		if c.ReadyProbe {
			return 2
		}
		if c.Kind != "quiz" {
			return 3
		}
		if c.Level == "foundation" {
			return 4
		}
		return 5
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if rank(a) != rank(b) {
		return rank(a) < rank(b)
	}
	if !a.DueAt.Equal(b.DueAt) {
		return a.DueAt.Before(b.DueAt)
	}
	return a.ID < b.ID
}
