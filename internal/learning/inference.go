package learning

import (
	"sort"
	"time"
)

// InferencePolicy is a conservative attribution policy, not a calibrated model
// of knowledge or evidence that cross-item selection improves learning.
const InferencePolicy = "knowledge-attribution-v1;exact-versions;recall>=choice;latest-adverse;age=7d;no-graph-grades"

const EvidenceFreshness = 7 * 24 * time.Hour

type UnitVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type RelationVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type CoverageVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type Correction struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

type EvidenceTarget struct {
	UnitID           string            `json:"unit_id"`
	UnitVersion      int               `json:"unit_version"`
	Role             string            `json:"role"`
	CoverageID       string            `json:"coverage_id,omitempty"`
	CoverageVersion  int               `json:"coverage_version,omitempty"`
	RelationIDs      []string          `json:"relation_ids,omitempty"`
	RelationVersions []RelationVersion `json:"relation_versions,omitempty"`
}

// Evidence represents one original observation. Targets are the encountered
// coverage, never today's quiz links. Corrections add context, not new attempts.
type Evidence struct {
	ID                string           `json:"id"`
	MaterialID        string           `json:"material_id"`
	MaterialVersion   int              `json:"material_version"`
	At                time.Time        `json:"at"`
	Mode              string           `json:"mode"`
	Kind              string           `json:"kind"`
	Outcome           string           `json:"outcome"`
	Rating            int              `json:"rating"`
	Assisted          bool             `json:"assisted"`
	Disputed          bool             `json:"disputed"`
	Ambiguous         bool             `json:"ambiguous"`
	Unmapped          bool             `json:"unmapped"`
	Targets           []EvidenceTarget `json:"targets"`
	ScheduleAfter     *Card            `json:"schedule_after,omitempty"`
	DueAt             time.Time        `json:"due_at"`
	Algorithm         string           `json:"algorithm,omitempty"`
	CorrectionIDs     []string         `json:"correction_ids"`
	Corrections       []Correction     `json:"corrections"`
	AssistanceReasons []string         `json:"assistance_reasons,omitempty"`
}

// Prerequisite and composition edges point from the component to the compound
// target. Only established edges carry uncertain support backwards; contrast
// and proposed edges never certify either end. Versions are part of identity.
type Relation struct {
	ID            string      `json:"id"`
	Version       int         `json:"version"`
	From          UnitVersion `json:"from"`
	To            UnitVersion `json:"to"`
	Kind          string      `json:"kind"`
	Proposed      bool        `json:"proposed"`
	CorrectionIDs []string    `json:"correction_ids,omitempty"`
}

type Attribution struct {
	EvidenceID        string            `json:"evidence_id"`
	MaterialID        string            `json:"material_id"`
	MaterialVersion   int               `json:"material_version"`
	At                time.Time         `json:"at"`
	Mode              string            `json:"mode"`
	Kind              string            `json:"kind"`
	Outcome           string            `json:"outcome"`
	Rating            int               `json:"rating"`
	Assisted          bool              `json:"assisted"`
	Disputed          bool              `json:"disputed"`
	Ambiguous         bool              `json:"ambiguous"`
	Role              string            `json:"role"`
	Targets           []EvidenceTarget  `json:"targets"`
	Coverage          []CoverageVersion `json:"coverage"`
	Relations         []RelationVersion `json:"relations"`
	CorrectionIDs     []string          `json:"correction_ids"`
	Corrections       []Correction      `json:"corrections"`
	AssistanceReasons []string          `json:"assistance_reasons,omitempty"`
}

type Estimate struct {
	Unit              UnitVersion       `json:"unit"`
	Mode              string            `json:"mode"`
	AsOf              time.Time         `json:"as_of"`
	TargetAt          time.Time         `json:"target_at"`
	State             string            `json:"state"`
	Reason            string            `json:"reason"`
	Policy            string            `json:"policy"`
	Direct            []Attribution     `json:"direct"`
	Support           []Attribution     `json:"support"`
	Exposure          []Attribution     `json:"exposure"`
	Assumed           []Attribution     `json:"assumed"`
	EvidenceIDs       []string          `json:"evidence_ids"`
	CorrectionIDs     []string          `json:"correction_ids"`
	Relations         []RelationVersion `json:"relations"`
	LatestAt          time.Time         `json:"latest_at"`
	ContextAt         time.Time         `json:"context_at"`
	AgeSeconds        int64             `json:"age_seconds"`
	RecallProbability *float64          `json:"recall_probability,omitempty"`
	RecallCondition   string            `json:"recall_condition,omitempty"`
	RecallEvidenceID  string            `json:"recall_evidence_id,omitempty"`
}

// Infer derives a read-only, time/context-conditioned estimate. One identity is
// attributed at most once, even through diamonds or a direct and indirect link.
// Unknown/exposure/support states intentionally have no invented probability.
func Infer(unit UnitVersion, mode string, asOf, targetAt time.Time, evidence []Evidence, relations []Relation) Estimate {
	asOf, targetAt = asOf.UTC(), targetAt.UTC()
	if targetAt.IsZero() {
		targetAt = asOf
	}
	result := Estimate{Unit: unit, Mode: mode, AsOf: asOf, TargetAt: targetAt, State: "unknown", Reason: "No applicable observation for this exact unit version and task context.", Policy: InferencePolicy}
	if !validUnit(unit) || modeStrength(mode) == 0 || asOf.IsZero() || targetAt.Before(asOf) {
		result.Reason = "A valid exact unit version, choice/recall context, and non-past target time are required."
		return result
	}
	events := uniqueEvidence(evidence, asOf)
	var latest *Evidence
	latestRole := ""
	for i := range events {
		e := &events[i]
		a, ok := attribute(unit, *e, relations)
		if !ok {
			continue
		}
		result.EvidenceIDs = append(result.EvidenceIDs, e.ID)
		result.CorrectionIDs = append(result.CorrectionIDs, a.CorrectionIDs...)
		result.Relations = append(result.Relations, a.Relations...)
		switch a.Role {
		case "direct":
			result.Direct = append(result.Direct, a)
		case "support":
			result.Support = append(result.Support, a)
		case "exposure":
			result.Exposure = append(result.Exposure, a)
		case "assumed":
			result.Assumed = append(result.Assumed, a)
		}
		// An ordinary reading does not erase a demonstration. An explicit help
		// request, dispute, or uncertain assessment does invalidate a colder
		// claim until another actual applicable success occurs.
		relevant := a.Role == "direct" || a.Role == "support" || e.Assisted || e.Disputed || e.Ambiguous
		if relevant && (latest == nil || laterEvidence(*e, *latest)) {
			latest, latestRole = e, a.Role
		}
	}
	result.EvidenceIDs = uniqueStrings(result.EvidenceIDs)
	result.CorrectionIDs = uniqueStrings(result.CorrectionIDs)
	result.Relations = uniqueRelations(result.Relations)
	if latest == nil {
		if len(result.Exposure) > 0 {
			result.State, result.Reason = "exposed", "Instruction or mention was encountered; it does not demonstrate unaided performance."
			result.LatestAt = result.Exposure[len(result.Exposure)-1].At
		} else if len(result.Assumed) > 0 {
			result.State, result.Reason = "assumed", "Encountered material assumes this unit; assumption is not assessment."
			result.LatestAt = result.Assumed[len(result.Assumed)-1].At
		}
		if !result.LatestAt.IsZero() {
			result.ContextAt = result.LatestAt
			result.AgeSeconds = int64(targetAt.Sub(result.LatestAt) / time.Second)
		}
		return result
	}
	result.LatestAt = latest.At.UTC()
	result.ContextAt = evidenceContextAt(*latest).UTC()
	result.AgeSeconds = int64(targetAt.Sub(latest.At) / time.Second)
	switch {
	case latest.Disputed:
		result.State, result.Reason = "disputed", "The latest relevant observation is disputed; its original outcome is retained but supplies no success credit."
	case latest.Assisted:
		result.State, result.Reason = "assisted", "The latest relevant observation was answer-assisted or requested help, not unaided performance."
	case latest.Ambiguous || latest.Unmapped:
		result.State, result.Reason = "ambiguous", "The latest observation has uncertain or corrected attribution; an exact-version probe is needed."
	case latestRole == "support":
		if directSuccess(*latest) && modeStrength(latest.Mode) >= modeStrength(mode) {
			result.State, result.Reason = "supported", "An established component relation supplies uncertain cross-item support, not an independent review or demonstration."
		} else {
			result.State, result.Reason = "ambiguous", "A compound task cannot identify which prerequisite or component caused a miss; no component failure is invented."
		}
	case latestRole != "direct":
		result.State, result.Reason = "exposed", "This interaction supplies exposure only, not an assessment."
	case latest.Rating == 1 || latest.Outcome == "wrong" || latest.Outcome == "revealed":
		result.State, result.Reason = "gap", "The latest direct assessment observed a gap in this task context; older success cannot hide it."
	case !directSuccess(*latest):
		result.State, result.Reason = "ambiguous", "The latest direct response was not an eligible graded success."
	case modeStrength(latest.Mode) < modeStrength(mode):
		result.State, result.Reason = "exposed", "Recognition success does not establish uncued recall."
	default:
		result.State, result.Reason = "demonstrated", "The latest eligible direct observation demonstrated performance in the same or stronger task context."
	}
	if (result.State == "demonstrated" || result.State == "supported") && targetAt.Sub(latest.At) >= EvidenceFreshness {
		result.State, result.Reason = "stale", "Applicable success is at least seven days old at the target time; this policy does not assume permanent knowledge."
	}
	// This is the real card's conditional prediction for that encountered task,
	// not a calibrated probability for an arbitrary unit or another material.
	if latestRole == "direct" && (result.State == "demonstrated" || result.State == "stale") && applicableCard(*latest) && schedulerError == nil {
		if probability, err := scheduler.Retrievability(*latest.ScheduleAfter, targetAt); err == nil {
			result.RecallProbability = &probability
			result.RecallEvidenceID = latest.ID
			result.RecallCondition = "Conditional on the unchanged encountered material, its actual quiz-owned " + Algorithm + " card, and the observed " + latest.Mode + " task context; not a unit-wide calibrated prediction."
		}
	}
	return result
}

func validUnit(unit UnitVersion) bool { return unit.ID != "" && unit.Version > 0 }

func modeStrength(mode string) int {
	switch mode {
	case "choice":
		return 1
	case "recall":
		return 2
	default:
		return 0
	}
}

func directSuccess(e Evidence) bool {
	return e.Kind == "review" && e.Outcome == "correct" && e.Rating == 3 && !e.Assisted && !e.Disputed && !e.Ambiguous && !e.Unmapped && modeStrength(e.Mode) > 0
}

func applicableCard(e Evidence) bool {
	return directSuccess(e) && e.Algorithm == Algorithm && e.ScheduleAfter != nil && e.ScheduleAfter.State != 0 && e.ScheduleAfter.Stability > 0 && e.ScheduleAfter.LastReview.Equal(e.At) && e.ScheduleAfter.Due.Equal(e.DueAt) && e.DueAt.After(e.At)
}

func evidenceContextAt(e Evidence) time.Time {
	at := e.At
	for _, correction := range e.Corrections {
		if correction.At.After(at) {
			at = correction.At
		}
	}
	return at
}

func equalEvidenceCard(a, b *Card) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func laterEvidence(a, b Evidence) bool {
	if aTime, bTime := evidenceContextAt(a), evidenceContextAt(b); !aTime.Equal(bTime) {
		return aTime.After(bTime)
	}
	// A tie has no reliable event ordering: adverse context wins, then stable ID.
	if adverseRank(a) != adverseRank(b) {
		return adverseRank(a) > adverseRank(b)
	}
	return a.ID > b.ID
}

func adverseRank(e Evidence) int {
	switch {
	case e.Disputed:
		return 5
	case e.Assisted:
		return 4
	case e.Ambiguous || e.Unmapped:
		return 3
	case !directSuccess(e):
		return 2
	default:
		return 1
	}
}

func uniqueEvidence(input []Evidence, asOf time.Time) []Evidence {
	byID := make(map[string]Evidence, len(input))
	for _, original := range input {
		if original.ID == "" || original.At.IsZero() || original.At.After(asOf) {
			continue
		}
		e := original
		e.Targets = append([]EvidenceTarget(nil), e.Targets...)
		e.CorrectionIDs = append([]string(nil), e.CorrectionIDs...)
		e.Corrections = nil
		futureIDs := make(map[string]bool)
		for _, correction := range original.Corrections {
			if correction.At.After(asOf) {
				futureIDs[correction.ID] = true
				continue
			}
			e.Corrections = append(e.Corrections, correction)
			e.CorrectionIDs = append(e.CorrectionIDs, correction.ID)
			if correction.Kind == "dispute" {
				e.Disputed = true
			} else {
				e.Ambiguous = true
			}
		}
		ids := e.CorrectionIDs[:0]
		for _, id := range e.CorrectionIDs {
			if !futureIDs[id] {
				ids = append(ids, id)
			}
		}
		e.CorrectionIDs = ids
		if prior, ok := byID[e.ID]; ok {
			// Duplicate observations can contribute additional paths, never
			// additional votes. Contradictory identity payloads fail closed.
			conflict := prior.MaterialID != e.MaterialID || prior.MaterialVersion != e.MaterialVersion || !prior.At.Equal(e.At) || prior.Mode != e.Mode || prior.Kind != e.Kind || prior.Outcome != e.Outcome || prior.Rating != e.Rating || prior.Algorithm != e.Algorithm || !prior.DueAt.Equal(e.DueAt) || !equalEvidenceCard(prior.ScheduleAfter, e.ScheduleAfter)
			if laterEvidence(prior, e) {
				prior, e = e, prior
			}
			e.Ambiguous = e.Ambiguous || prior.Ambiguous || conflict
			e.Assisted = e.Assisted || prior.Assisted
			e.Disputed = e.Disputed || prior.Disputed
			e.Unmapped = e.Unmapped || prior.Unmapped
			e.Targets = append(e.Targets, prior.Targets...)
			e.CorrectionIDs = append(e.CorrectionIDs, prior.CorrectionIDs...)
			e.Corrections = append(e.Corrections, prior.Corrections...)
			e.AssistanceReasons = append(append([]string(nil), e.AssistanceReasons...), prior.AssistanceReasons...)
		}
		e.Targets = uniqueTargets(e.Targets)
		e.Corrections = uniqueCorrections(e.Corrections)
		if e.Rating == 1 || e.Outcome == "wrong" {
			assessed := make(map[UnitVersion]bool)
			for _, target := range e.Targets {
				if target.Role == "assesses" {
					assessed[UnitVersion{ID: target.UnitID, Version: target.UnitVersion}] = true
				}
			}
			// A multi-target miss cannot identify which assessed component
			// caused the failure, even when the coverage itself is explicit.
			e.Ambiguous = e.Ambiguous || len(assessed) > 1
		}
		e.CorrectionIDs = uniqueStrings(e.CorrectionIDs)
		e.AssistanceReasons = uniqueStrings(append([]string(nil), e.AssistanceReasons...))
		byID[e.ID] = e
	}
	result := make([]Evidence, 0, len(byID))
	for _, e := range byID {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return laterEvidence(result[j], result[i]) })
	return result
}

func attribute(unit UnitVersion, e Evidence, relations []Relation) (Attribution, bool) {
	a := Attribution{EvidenceID: e.ID, MaterialID: e.MaterialID, MaterialVersion: e.MaterialVersion, At: e.At.UTC(), Mode: e.Mode, Kind: e.Kind, Outcome: e.Outcome, Rating: e.Rating, Assisted: e.Assisted, Disputed: e.Disputed, Ambiguous: e.Ambiguous, CorrectionIDs: append([]string(nil), e.CorrectionIDs...), Corrections: append([]Correction(nil), e.Corrections...), AssistanceReasons: append([]string(nil), e.AssistanceReasons...)}
	best := 0
	for _, target := range e.Targets {
		origin := UnitVersion{ID: target.UnitID, Version: target.UnitVersion}
		rank := 0
		var path []RelationVersion
		var corrections []string
		if origin == unit {
			switch target.Role {
			case "assesses":
				rank = 4
			case "teaches", "mentions":
				rank = 2
			case "assumes":
				rank = 1
			}
		} else if target.Role == "assesses" {
			if p, c, ok := supportingPath(unit, origin, relations); ok {
				rank, path, corrections = 3, p, c
			}
		}
		if rank == 0 {
			continue
		}
		if rank > best {
			best = rank
		}
		a.Targets = append(a.Targets, target)
		if target.CoverageID != "" && target.CoverageVersion > 0 {
			a.Coverage = append(a.Coverage, CoverageVersion{ID: target.CoverageID, Version: target.CoverageVersion})
		}
		a.Relations = append(a.Relations, target.RelationVersions...)
		a.Relations = append(a.Relations, path...)
		a.CorrectionIDs = append(a.CorrectionIDs, corrections...)
	}
	if best == 0 {
		return a, false
	}
	switch best {
	case 4:
		a.Role = "direct"
	case 3:
		a.Role = "support"
	case 2:
		a.Role = "exposure"
	case 1:
		a.Role = "assumed"
	}
	a.Relations = uniqueRelations(a.Relations)
	a.CorrectionIDs = uniqueStrings(a.CorrectionIDs)
	sort.Slice(a.Coverage, func(i, j int) bool {
		if a.Coverage[i].ID == a.Coverage[j].ID {
			return a.Coverage[i].Version < a.Coverage[j].Version
		}
		return a.Coverage[i].ID < a.Coverage[j].ID
	})
	coverage := a.Coverage[:0]
	for _, c := range a.Coverage {
		if len(coverage) == 0 || coverage[len(coverage)-1] != c {
			coverage = append(coverage, c)
		}
	}
	a.Coverage = coverage
	return a, true
}

// Traverse each exact-version node once. Edges on any successful path remain
// inspectable, but paths never multiply the observation's evidential weight.
func supportingPath(unit, assessed UnitVersion, relations []Relation) ([]RelationVersion, []string, bool) {
	if !validUnit(unit) || !validUnit(assessed) {
		return nil, nil, false
	}
	forward := map[UnitVersion]bool{unit: true}
	queue := []UnitVersion{unit}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, relation := range relations {
			if relation.Proposed || relation.ID == "" || relation.Version < 1 || (relation.Kind != "composition" && relation.Kind != "prerequisite") || relation.From != current || !validUnit(relation.To) {
				continue
			}
			if !forward[relation.To] {
				forward[relation.To] = true
				queue = append(queue, relation.To)
			}
		}
	}
	if !forward[assessed] {
		return nil, nil, false
	}
	backward := map[UnitVersion]bool{assessed: true}
	queue = []UnitVersion{assessed}
	var path []RelationVersion
	var corrections []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, relation := range relations {
			if relation.Proposed || relation.ID == "" || relation.Version < 1 || (relation.Kind != "composition" && relation.Kind != "prerequisite") || relation.To != current || !forward[relation.From] {
				continue
			}
			path = append(path, RelationVersion{ID: relation.ID, Version: relation.Version})
			corrections = append(corrections, relation.CorrectionIDs...)
			if !backward[relation.From] {
				backward[relation.From] = true
				queue = append(queue, relation.From)
			}
		}
	}
	return uniqueRelations(path), uniqueStrings(corrections), true
}

func uniqueStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if value != "" && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}

func uniqueRelations(values []RelationVersion) []RelationVersion {
	sort.Slice(values, func(i, j int) bool {
		if values[i].ID == values[j].ID {
			return values[i].Version < values[j].Version
		}
		return values[i].ID < values[j].ID
	})
	result := values[:0]
	for _, value := range values {
		if value.ID != "" && value.Version > 0 && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}

func uniqueCorrections(values []Correction) []Correction {
	sort.Slice(values, func(i, j int) bool {
		if values[i].ID != values[j].ID {
			return values[i].ID < values[j].ID
		}
		if !values[i].At.Equal(values[j].At) {
			return values[i].At.Before(values[j].At)
		}
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		return values[i].Reason < values[j].Reason
	})
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func uniqueTargets(values []EvidenceTarget) []EvidenceTarget {
	type targetKey struct {
		unit, role, coverage     string
		version, coverageVersion int
	}
	byKey := make(map[targetKey]EvidenceTarget, len(values))
	for _, target := range values {
		key := targetKey{target.UnitID, target.Role, target.CoverageID, target.UnitVersion, target.CoverageVersion}
		previous := byKey[key]
		target.RelationIDs = uniqueStrings(append(append([]string(nil), target.RelationIDs...), previous.RelationIDs...))
		target.RelationVersions = uniqueRelations(append(append([]RelationVersion(nil), target.RelationVersions...), previous.RelationVersions...))
		byKey[key] = target
	}
	result := make([]EvidenceTarget, 0, len(byKey))
	for _, target := range byKey {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.UnitID != b.UnitID {
			return a.UnitID < b.UnitID
		}
		if a.UnitVersion != b.UnitVersion {
			return a.UnitVersion < b.UnitVersion
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		if a.CoverageID != b.CoverageID {
			return a.CoverageID < b.CoverageID
		}
		return a.CoverageVersion < b.CoverageVersion
	})
	return result
}
