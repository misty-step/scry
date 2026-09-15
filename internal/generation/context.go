package generation

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/misty-step/scry/internal/store"
)

// prepareJob selects whole versioned objects, never string prefixes. Validation
// receives this exact context, so an ID omitted from the paid request cannot be
// smuggled back as an authorized reuse. The durable job itself is not mutated.
func prepareJob(job *store.Job) (store.Job, error) {
	prepared := *job
	if !slices.Contains([]string{"capture", "enrich", "bridge", "expand"}, job.Kind) {
		return prepared, errors.New("Saved work has an unsupported job kind; no request was transmitted.")
	}
	if job.Context.Version != store.KnowledgeContextVersion || (job.Context.GoalID != "" && (job.Context.GoalID != job.GoalID || job.Context.GoalRevision != job.GoalRevision)) {
		return prepared, errors.New("Saved knowledge context is missing or belongs to a different goal revision; refresh this work before retrying.")
	}
	if job.Kind == "bridge" && (job.TargetMaterialID == "" || job.TargetMaterialVersion < 1 || job.TargetPresentationID == "") {
		return prepared, errors.New("Bridge work is missing its retained versioned target; no request was transmitted.")
	}
	if job.Kind == "expand" && strings.TrimSpace(job.Context.Request) == "" {
		return prepared, errors.New("Expansion work has no explicit accepted scope; no new goal was inferred.")
	}
	original := job.Context
	selected := original
	selected.Units, selected.Relations, selected.Materials, selected.References, selected.Evidence = nil, nil, nil, nil, nil
	if original.Omitted.Units < 0 || original.Omitted.Relations < 0 || original.Omitted.Materials < 0 || original.Omitted.References < 0 || original.Omitted.Evidence < 0 {
		return prepared, errors.New("Saved knowledge context has invalid omission accounting.")
	}
	if len(original.Units) > maxUnits || len(original.Relations) > maxRelations || len(original.Materials) > maxQuizzes+maxMaterials || len(original.References) > 64 || len(original.Evidence) > 128 {
		return prepared, errors.New("Saved knowledge context exceeds bounded selection limits; no request was transmitted.")
	}
	trigger := -1
	if job.Kind == "enrich" && job.ObservationID == "" && (job.TargetMaterialID != "" || job.TargetMaterialVersion != 0 || job.TargetPresentationID != "" || job.TargetPresentationVersion != 0) {
		return prepared, errors.New("Targeted enrichment is missing its original triggering observation; no migration work was substituted.")
	}
	if job.ObservationID != "" {
		if job.Kind != "enrich" || job.GoalID == "" || job.GoalRevision < 1 || original.GoalID != job.GoalID || original.GoalRevision != job.GoalRevision {
			return prepared, errors.New("Observed-gap work is missing its chosen goal revision; no request was transmitted.")
		}
		if job.TargetMaterialID == "" || job.TargetMaterialVersion < 1 || job.TargetPresentationID == "" || job.TargetPresentationVersion != job.TargetMaterialVersion {
			return prepared, errors.New("Observed-gap work is missing its exact versioned presentation target; no request was transmitted.")
		}
		var target *store.Material
		for index := range original.Materials {
			material := &original.Materials[index]
			if material.ID != job.TargetMaterialID {
				continue
			}
			if target != nil {
				return prepared, errors.New("Observed-gap work has conflicting target identities; no request was transmitted.")
			}
			target = material
		}
		if target == nil || target.Version != job.TargetMaterialVersion || target.Archived || target.MetadataOnly || target.Kind != "quiz" || target.Quiz == nil || target.Quiz.Archived || target.Quiz.ID == "" || target.Quiz.Version < 1 {
			return prepared, errors.New("The original observed quiz target is unavailable; no request was transmitted.")
		}
		for index, evidence := range original.Evidence {
			if evidence.ID != job.ObservationID {
				continue
			}
			if trigger >= 0 {
				return prepared, errors.New("Observed-gap work has conflicting trigger identities; no request was transmitted.")
			}
			trigger = index
			if evidence.Kind != "review" || evidence.Outcome != "wrong" || evidence.Rating != 1 || evidence.Assisted || evidence.Disputed || len(evidence.CorrectionIDs) != 0 || len(evidence.Corrections) != 0 || evidence.MaterialID != target.ID || evidence.MaterialVersion != target.Version || !slices.Contains([]string{"choice", "recall"}, evidence.Mode) || evidence.Mode != target.Quiz.Kind {
				return prepared, errors.New("The pinned observation is not the original eligible failure of this quiz target; no request was transmitted.")
			}
		}
		if trigger < 0 {
			return prepared, errors.New("The original triggering observation is unavailable; no other failure was substituted.")
		}
	}
	// Fixed per-category allowances prevent long resources from crowding out
	// foundations or evidence. The remaining envelope space covers goal metadata.
	fits := func(value any, remaining *int) bool {
		data, err := json.Marshal(value)
		if err != nil || len(data)+1 > *remaining {
			return false
		}
		*remaining -= len(data) + 1
		return true
	}
	materialBudget, unitBudget, relationBudget, referenceBudget, evidenceBudget := 28<<10, 14<<10, 6<<10, 2<<10, 8<<10
	if trigger >= 0 {
		if !fits(original.Evidence[trigger], &evidenceBudget) {
			return prepared, errors.New("The original triggering observation cannot fit the bounded request; no request was transmitted.")
		}
		selected.Evidence = append(selected.Evidence, original.Evidence[trigger])
	}
	ordered := slices.Clone(original.Materials)
	priority := func(material store.Material) int {
		if material.ID == job.TargetMaterialID {
			return 0
		}
		if job.Kind == "enrich" && job.ObservationID == "" && material.SourceID == job.SourceID && material.Unmapped {
			return 1
		}
		if material.SourceID == job.SourceID {
			return 2
		}
		return 3
	}
	slices.SortStableFunc(ordered, func(left, right store.Material) int { return priority(left) - priority(right) })
	wanted := make(map[string]bool)
	targetIncluded := job.TargetMaterialID == ""
	for _, material := range ordered {
		if !material.Archived && material.Version > 0 && len(selected.Materials) < 24 && fits(material, &materialBudget) {
			selected.Materials = append(selected.Materials, material)
			targetIncluded = targetIncluded || material.ID == job.TargetMaterialID && material.Version == job.TargetMaterialVersion
			for _, link := range material.Links {
				wanted[link.UnitID] = true
			}
		} else {
			selected.Omitted.Materials++
		}
	}
	if (job.Kind == "bridge" || job.Kind == "expand" || job.ObservationID != "") && !targetIncluded {
		return prepared, errors.New("The versioned target cannot fit the bounded request or is unavailable; no replacement target was invented.")
	}
	units := slices.Clone(original.Units)
	unitPriority := func(unit store.KnowledgeUnit) int {
		if wanted[unit.ID] {
			return 0
		}
		if unit.Kind == "foundation" {
			return 1
		}
		return 2
	}
	slices.SortStableFunc(units, func(left, right store.KnowledgeUnit) int { return unitPriority(left) - unitPriority(right) })
	versions := make(map[string]int)
	for _, unit := range units {
		if !unit.Archived && unit.Version > 0 && len(selected.Units) < 48 && fits(unit, &unitBudget) {
			selected.Units = append(selected.Units, unit)
			versions[unit.ID] = unit.Version
		} else {
			selected.Omitted.Units++
		}
	}
	for _, relation := range original.Relations {
		if !relation.Archived && versions[relation.FromID] == relation.FromVersion && versions[relation.ToID] == relation.ToVersion && len(selected.Relations) < 64 && fits(relation, &relationBudget) {
			selected.Relations = append(selected.Relations, relation)
		} else {
			selected.Omitted.Relations++
		}
	}
	seenReference := make(map[string]bool)
	for _, reference := range original.References {
		if safeReference(reference) && !seenReference[reference] && len(selected.References) < 16 && fits(reference, &referenceBudget) {
			selected.References = append(selected.References, reference)
			seenReference[reference] = true
		} else {
			selected.Omitted.References++
		}
	}
	for index, evidence := range original.Evidence {
		if index == trigger {
			continue
		}
		if len(selected.Evidence) < 24 && fits(evidence, &evidenceBudget) {
			selected.Evidence = append(selected.Evidence, evidence)
		} else {
			selected.Omitted.Evidence++
		}
	}
	data, err := json.Marshal(selected)
	if err != nil || len(data) > maxContextBytes {
		return prepared, errors.New("Goal metadata exceeds the bounded knowledge context; no source or context text was silently truncated.")
	}
	prepared.Context = selected
	return prepared, nil
}

func contextMaterial(job *store.Job, id string) *store.Material {
	for index := range job.Context.Materials {
		material := &job.Context.Materials[index]
		if material.ID == id && !material.Archived && material.Version > 0 {
			return material
		}
	}
	return nil
}

func validateUnitReuse(unit store.GeneratedUnit, job *store.Job) string {
	for _, existing := range job.Context.Units {
		if existing.Archived || existing.Version <= 0 {
			continue
		}
		if unit.ReuseID == existing.ID {
			if unit.Statement != existing.Statement || unit.Kind != existing.Kind {
				return "reused_unit_changed"
			}
			return ""
		}
		if unit.ReuseID == "" && unit.Statement == existing.Statement && unit.Kind == existing.Kind {
			return "duplicate_reusable_unit"
		}
	}
	if unit.ReuseID != "" {
		return "unknown_reused_unit"
	}
	return ""
}

func validateQuizReuse(quiz store.GeneratedQuiz, job *store.Job) string {
	if quiz.ReuseID == "" {
		for _, existing := range job.Context.Materials {
			if !existing.Archived && existing.Quiz != nil && normalized(existing.Quiz.Prompt) == normalized(quiz.Prompt) {
				return "duplicate_reusable_quiz"
			}
		}
		return ""
	}
	material := contextMaterial(job, quiz.ReuseID)
	if material == nil || material.Kind != "quiz" || material.Quiz == nil || material.Quiz.Archived {
		return "unknown_reused_quiz"
	}
	existing := material.Quiz
	if quiz.Kind != existing.Kind || quiz.Prompt != existing.Prompt || quiz.Answer != existing.Answer || quiz.Explanation != existing.Explanation || quiz.Basis != existing.Basis || quiz.Evidence != existing.Evidence || !slices.Equal(quiz.Choices, existing.Choices) || !slices.Equal(quiz.Variants, existing.Variants) || quiz.Level != material.Level || quiz.EstimatedSeconds != material.EstimatedSeconds {
		return "reused_quiz_changed"
	}
	if !plainText(quiz.Prompt, 4096, true) || !plainText(quiz.Answer, 1024, true) || !plainText(quiz.Explanation, 8192, true) || len(quiz.Evidence) > 8192 || hasUnsafeControl(quiz.Evidence) || len(quiz.Choices) > 5 || len(quiz.Variants) > 8 {
		return "unsafe_reused_quiz"
	}
	for _, choice := range quiz.Choices {
		if !plainText(choice, 1024, true) {
			return "unsafe_reused_quiz"
		}
	}
	for _, variant := range quiz.Variants {
		if !plainText(variant, 1024, true) {
			return "unsafe_reused_quiz"
		}
	}
	return ""
}

func validateMaterialReuse(material store.GeneratedMaterial, job *store.Job) string {
	if material.ReuseID == "" {
		for _, existing := range job.Context.Materials {
			if !existing.Archived && existing.Kind == material.Kind && existing.Body == material.Body && existing.ReferenceURL == material.ReferenceURL && existing.StartSeconds == material.StartSeconds && existing.EndSeconds == material.EndSeconds {
				return "duplicate_reusable_material"
			}
		}
		return ""
	}
	existing := contextMaterial(job, material.ReuseID)
	if existing == nil || existing.Kind == "quiz" {
		return "unknown_reused_material"
	}
	if material.Kind != existing.Kind || material.Title != existing.Title || material.Body != existing.Body || material.Basis != existing.Basis || material.Evidence != existing.Evidence || material.ReferenceURL != existing.ReferenceURL || material.StartSeconds != existing.StartSeconds || material.EndSeconds != existing.EndSeconds || material.EstimatedSeconds != existing.EstimatedSeconds || !reflect.DeepEqual(material.Diagram, existing.Diagram) {
		return "reused_material_changed"
	}
	return ""
}

func suppliedReference(material store.GeneratedMaterial, job *store.Job) bool {
	for _, existing := range job.Context.Materials {
		if !existing.Archived && existing.ReferenceURL == material.ReferenceURL && existing.Kind == material.Kind && existing.StartSeconds == material.StartSeconds && existing.EndSeconds == material.EndSeconds {
			return true
		}
	}
	// Only explicit numeric start/end query fields or a Media Fragments range
	// supply a new segment. A bare URL cannot establish an unseen transcript.
	if !suppliedSegment(material.ReferenceURL, material.StartSeconds, material.EndSeconds) {
		return false
	}
	if slices.Contains(job.Context.References, material.ReferenceURL) {
		return true
	}
	for _, reference := range citations.FindAllString(job.SourceText, -1) {
		if strings.TrimRight(reference, ".,;!") == material.ReferenceURL {
			return true
		}
	}
	return false
}

func suppliedSegment(reference string, start, end int) bool {
	if start == 0 && end == 0 {
		return true
	}
	u, err := url.Parse(reference)
	if err != nil {
		return false
	}
	startText, endText := u.Query().Get("start"), u.Query().Get("end")
	if startText == "" {
		startText = u.Query().Get("t")
	}
	if fragment, found := strings.CutPrefix(u.Fragment, "t="); found {
		var both bool
		startText, endText, both = strings.Cut(fragment, ",")
		if !both {
			return false
		}
	}
	from, firstErr := strconv.Atoi(startText)
	to, secondErr := strconv.Atoi(endText)
	return firstErr == nil && secondErr == nil && from == start && to == end && start >= 0 && end > start && end <= 86400
}

func validateJobBundle(result *store.GenerationResult, job *store.Job, units map[string]store.GeneratedUnit) string {
	if job.Kind == "bridge" || job.Kind == "enrich" && job.ObservationID != "" {
		target := contextMaterial(job, job.TargetMaterialID)
		if target == nil || target.Version != job.TargetMaterialVersion {
			return "bridge_target_unavailable"
		}
		targets := make(map[string]bool)
		versions := make(map[string]int, len(job.Context.Units))
		for _, unit := range job.Context.Units {
			versions[unit.ID] = unit.Version
		}
		for key, unit := range units {
			for _, link := range target.Links {
				if link.Role == "assesses" && unit.ReuseID == link.UnitID && versions[unit.ReuseID] == link.UnitVersion {
					targets[key] = true
				}
			}
		}
		for _, quiz := range result.Quizzes {
			if quiz.ReuseID == target.ID {
				for _, key := range assessedKeys(quiz.Links) {
					targets[key] = true
				}
			}
		}
		taught, practiced := make(map[string]bool), make(map[string]bool)
		for _, material := range result.Materials {
			if strings.TrimSpace(material.Body) == "" {
				continue
			}
			for _, link := range material.Links {
				if link.Role == "teaches" && units[link.UnitKey].Kind == "foundation" {
					taught[link.UnitKey] = true
				}
			}
		}
		for _, quiz := range result.Quizzes {
			if quiz.Level != "foundation" {
				continue
			}
			for _, key := range assessedKeys(quiz.Links) {
				if units[key].Kind == "foundation" {
					practiced[key] = true
				}
			}
		}
		dependencies := make(map[string][]string)
		for _, relation := range result.Relations {
			if relation.Kind == "prerequisite" || relation.Kind == "composition" {
				dependencies[relation.From] = append(dependencies[relation.From], relation.To)
			}
		}
		bridge := false
		for key := range taught {
			if !practiced[key] {
				continue
			}
			seen := make(map[string]bool)
			queue := []string{key}
			for len(queue) > 0 {
				current := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				if seen[current] {
					continue
				}
				seen[current] = true
				bridge = bridge || targets[current]
				queue = append(queue, dependencies[current]...)
			}
		}
		if !bridge {
			return "bridge_requires_target_linked_instruction_and_foundation_practice"
		}
	}
	if job.Kind == "enrich" && job.ObservationID == "" {
		mapped := make(map[string]bool)
		for _, quiz := range result.Quizzes {
			mapped[quiz.ReuseID] = quiz.ReuseID != ""
		}
		for _, existing := range job.Context.Materials {
			if existing.Kind == "quiz" && existing.SourceID == job.SourceID && existing.Unmapped && !mapped[existing.ID] {
				result.Coverage.Complete = false
				result.Coverage.Missing = append(result.Coverage.Missing, "Preserved quiz material "+existing.ID+" remains unmapped; earlier reviews were not reinterpreted.")
			}
		}
		if job.Context.Omitted.Materials > 0 {
			result.Coverage.Complete = false
			result.Coverage.Missing = append(result.Coverage.Missing, "Some preserved material was omitted from bounded context; enrichment does not claim to map unseen material.")
		}
	}
	return ""
}
