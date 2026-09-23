package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

func validateV5Output(job *store.Job, input store.JobContext, content string) (store.GenerationResult, error) {
	result := store.GenerationResult{}
	data := []byte(content)
	if len(data) > maxContentBytes || !utf8.Valid(data) || checkJSON(data, 16) != nil {
		return result, errors.New("invalid JSON")
	}
	switch job.Kind {
	case "transcribe":
		var transcript struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		}
		if strictObject(data, &transcript, "title", "text") != nil || !plainV5(transcript.Title, 240) ||
			strings.TrimSpace(transcript.Text) == "" || len(transcript.Text) > 128<<10 || hasUnsafeControl(transcript.Text) {
			return result, errors.New("invalid transcript")
		}
		result.Documents = []store.DocumentContent{{Kind: "transcript", Title: transcript.Title, Text: transcript.Text, Provider: "model"}}
	case "plan":
		var envelope struct {
			Goal     string            `json:"goal"`
			Concepts []json.RawMessage `json:"concepts"`
		}
		if strictObject(data, &envelope, "goal", "concepts") != nil || len(envelope.Concepts) < 1 {
			return result, errors.New("invalid plan fields or concept count")
		}
		plan := store.PlanContent{Goal: shortGoal(envelope.Goal)}
		if !plainV5(plan.Goal, 480) {
			return result, errors.New("invalid study goal")
		}
		taskText, taskKind := v5TaskMaterial(job, input)
		task, taskErr := planTask(taskText, taskKind)
		if taskErr != nil {
			return result, taskErr
		}
		exact := task.Task == "exact_text" || task.Task == "complete_set"
		if exact && (len(envelope.Concepts) != 1 || task.Unverified || len(task.Units) == 0) {
			return result, errors.New("exact task requires one concept and authoritative units")
		}
		existing := make(map[string]bool, len(input.ExistingConcepts))
		for _, c := range input.ExistingConcepts {
			existing[c.ID] = true
		}
		keys := make(map[string]bool, len(envelope.Concepts))
		var kept []store.PlannedConcept
		var rejected []string
		lastInvalidName := ""
		for _, raw := range envelope.Concepts {
			if len(kept) >= store.MaxPlanConcepts {
				rejected = append(rejected, "plan limit exceeded")
				continue
			}
			var c store.PlannedConcept
			var reason error
			if err := json.Unmarshal(raw, &c); err != nil {
				reason = errors.New("invalid concept fields")
			}
			switch {
			case reason != nil:
			case !plainV5(c.Key, 40) || strings.ContainsAny(c.Key, " \t\n") || keys[c.Key]:
				reason = errors.New("duplicate or invalid plan key")
			case len(strings.Fields(c.Name)) < 1 || len(strings.Fields(c.Name)) > 7 || !plainV5(c.Name, 120) || !plainV5(c.Summary, 500):
				reason = errors.New("invalid concept name or summary")
			case c.ExistingID != "" && !existing[c.ExistingID]:
				reason = errors.New("unknown existing concept")
			case c.ExistingID == "" && c.Note == nil:
				reason = errors.New("missing concept note")
			case c.Note != nil:
				reason = validateV5Note(c.Note, job, input)
			}
			if reason == nil && exact {
				if c.Note == nil {
					reason = errors.New("exact task requires a concept note")
				} else {
					for _, unit := range task.Units {
						if !strings.Contains(c.Note.Body, unit.Text) {
							reason = errors.New("required unit missing from plan")
							break
						}
					}
				}
			}
			if reason != nil {
				if exact {
					return result, fmt.Errorf("concept %q: %w", c.Name, reason)
				}
				lastInvalidName = c.Name
				rejected = append(rejected, reason.Error())
				continue
			}
			keys[c.Key] = true
			kept = append(kept, c)
		}
		if len(kept) == 0 {
			if lastInvalidName != "" {
				return result, fmt.Errorf("concept %q: %s", lastInvalidName, rejected[len(rejected)-1])
			}
			return result, fmt.Errorf("no valid concepts: %s", rejectionNote("concepts", rejected))
		}
		for i := range kept {
			c := &kept[i]
			c.Requires = validRelations(c.Requires, c.Key, c.ExistingID, keys, existing)
			c.PartOf = validRelations(c.PartOf, c.Key, c.ExistingID, keys, existing)
			c.ConfusedWith = validRelations(c.ConfusedWith, c.Key, c.ExistingID, keys, existing)
		}
		plan.Concepts = kept
		result.Plan = &plan
		if len(rejected) > 0 {
			result.Partial = true
			result.Note = rejectionNote("concepts", rejected)
		}
	case "questions", "fix":
		var envelope struct {
			Quizzes []json.RawMessage `json:"quizzes"`
		}
		if strictObject(data, &envelope, "quizzes") != nil || len(envelope.Quizzes) < 1 ||
			job.Kind == "fix" && len(envelope.Quizzes) != 1 {
			return result, errors.New("invalid question count")
		}
		targets := make(map[string]bool, len(input.Concepts))
		for _, c := range input.Concepts {
			targets[c.ID] = true
		}
		if len(targets) == 0 {
			return result, errors.New("missing target concepts")
		}
		taskText, taskKind := v5TaskMaterial(job, input)
		contract, err := planTask(taskText, taskKind)
		if err != nil && job.Kind == "questions" {
			return result, err
		}
		if job.Kind == "fix" && (input.Quiz == nil || len(envelope.Quizzes) != 1) {
			return result, errors.New("invalid correction")
		}
		seen := make(map[string]bool, len(envelope.Quizzes))
		exact := job.Kind == "questions" && len(contract.Units) > 0
		var rejected []string
		limit := store.MaxCriticCandidates
		for i, raw := range envelope.Quizzes {
			if len(result.Quizzes) >= limit {
				if exact {
					return result, errors.New("required unit count exceeds the question limit")
				}
				rejected = append(rejected, "question limit exceeded")
				continue
			}
			q, err := validateV5Question(raw, job, input, targets, contract, i)
			if err == nil && seen[normalized(q.Prompt)] {
				err = errors.New("duplicate question")
			}
			if err != nil {
				if exact || job.Kind == "fix" {
					return result, fmt.Errorf("question %d: %w", i+1, err)
				}
				rejected = append(rejected, err.Error())
				continue
			}
			seen[normalized(q.Prompt)] = true
			result.Quizzes = append(result.Quizzes, q)
		}
		if exact && len(result.Quizzes) != len(contract.Units) {
			return result, errors.New("required unit missing")
		}
		if len(result.Quizzes) == 0 {
			return result, fmt.Errorf("no valid questions: %s", rejectionNote("questions", rejected))
		}
		if !exact && job.Kind == "questions" {
			rank := map[string]int{"recognize": 1, "recall": 2, "explain": 3, "apply": 3}
			order := make(map[string]int, len(input.Concepts))
			for i, concept := range input.Concepts {
				order[concept.ID] = i
			}
			sort.SliceStable(result.Quizzes, func(i, j int) bool {
				if result.Quizzes[i].Concept == result.Quizzes[j].Concept {
					return rank[result.Quizzes[i].Level] < rank[result.Quizzes[j].Level]
				}
				return order[result.Quizzes[i].Concept] < order[result.Quizzes[j].Concept]
			})
		}
		if len(rejected) > 0 {
			result.Partial = true
			result.Note = rejectionNote("questions", rejected)
		}
	default:
		return result, errors.New("unsupported output kind")
	}
	if result.Note == "" {
		result.Note = "Study material is ready."
	}
	return result, nil
}

func shortGoal(goal string) string {
	goal = strings.Join(strings.Fields(goal), " ")
	runes := []rune(goal)
	if len(runes) <= 120 {
		return goal
	}
	end := 119
	for i := end - 1; i > 0; i-- {
		if unicode.IsSpace(runes[i]) {
			end = i
			break
		}
	}
	return string(runes[:end]) + "…"
}

func validRelations(refs []string, selfKey, selfID string, keys, existing map[string]bool) []string {
	var valid []string
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref != selfKey && ref != selfID && !seen[ref] && (keys[ref] || existing[ref]) {
			valid = append(valid, ref)
			seen[ref] = true
		}
	}
	return valid
}

func rejectionNote(kind string, rejected []string) string {
	counts := make(map[string]int)
	var reasons []string
	for _, reason := range rejected {
		if counts[reason] == 0 {
			reasons = append(reasons, reason)
		}
		counts[reason]++
	}
	sort.Strings(reasons)
	for i, reason := range reasons {
		if counts[reason] > 1 {
			reasons[i] = fmt.Sprintf("%s (%d)", reason, counts[reason])
		}
	}
	if len(rejected) == 1 {
		return fmt.Sprintf("1 %s was not published: %s", strings.TrimSuffix(kind, "s"), strings.Join(reasons, ", "))
	}
	return fmt.Sprintf("%d %s were not published: %s", len(rejected), kind, strings.Join(reasons, ", "))
}

func plainV5(text string, limit int) bool {
	return strings.TrimSpace(text) != "" && len(text) <= limit && utf8.ValidString(text) && !hasUnsafeControl(text) && !markup.MatchString(text)
}

func validateV5Note(note *store.NoteContent, job *store.Job, input store.JobContext) error {
	if !plainV5(note.Title, 200) || !plainV5(note.Body, 2400) || len(note.Body) < 40 || len(note.Evidence) > 6 {
		return errors.New("invalid note content or length")
	}
	basis, quotes, citations, err := snapV5Evidence(note.Basis, note.Evidence, note.Citations, job, input)
	if err != nil {
		return err
	}
	note.Basis, note.Evidence, note.Citations = basis, quotes, citations
	return validateV5Basis(note.Basis, note.Evidence, note.Citations, job, input)
}

func v5EvidenceText(basis string, job *store.Job, input store.JobContext) string {
	var parts []string
	if basis == "source" && (job.SourceMode == "text" || job.SourceMode == "") {
		parts = append(parts, job.SourceText)
	}
	for _, doc := range input.Documents {
		if basis == "source" && (doc.Kind == "page" || doc.Kind == "transcript") || basis == "web" && doc.Kind == "search_result" {
			parts = append(parts, doc.Text, doc.URL)
		}
	}
	return strings.Join(parts, "\n")
}

func validateV5Basis(basis string, evidence []string, citations []store.Citation, job *store.Job, input store.JobContext) error {
	if basis == "topic" && job.SourceKind != "topic" || basis == "web" && job.SourceKind != "topic" || basis == "source" && job.SourceKind != "source" {
		return errors.New("basis does not match the saved material")
	}
	if basis == "topic" {
		if len(citations) != 0 {
			return errors.New("topic cannot cite documents")
		}
		for _, quote := range evidence {
			if quote != "" {
				return errors.New("topic cannot quote evidence")
			}
		}
		return nil
	}
	if basis != "source" && basis != "web" || len(evidence) == 0 || len(evidence) > 6 {
		return errors.New("missing or invalid basis")
	}
	for _, quote := range evidence {
		if !plainV5(quote, 8192) {
			return errors.New("invalid evidence text")
		}
		if basis == "source" {
			found := (job.SourceMode == "text" || job.SourceMode == "") && strings.Contains(job.SourceText, quote)
			for _, doc := range input.Documents {
				if (doc.Kind == "page" || doc.Kind == "transcript") && strings.Contains(doc.Text, quote) {
					found = true
				}
			}
			if !found {
				return errors.New("source evidence is not an exact quotation")
			}
		}
	}
	if basis == "source" {
		if len(citations) != 0 {
			return errors.New("source cannot cite web results")
		}
		return nil
	}
	if len(citations) == 0 {
		return errors.New("web excerpt needs citation")
	}
	for _, citation := range citations {
		found := false
		for _, doc := range input.Documents {
			if doc.ID == citation.DocumentID && doc.Kind == "search_result" && doc.URL == citation.URL && doc.Title == citation.Title {
				for _, quote := range evidence {
					if strings.Contains(doc.Text, quote) {
						found = true
						break
					}
				}
			}
		}
		if !found {
			return errors.New("citation does not support the quoted excerpt")
		}
	}
	for _, quote := range evidence {
		found := false
		for _, citation := range citations {
			for _, doc := range input.Documents {
				if doc.Kind == "search_result" && doc.ID == citation.DocumentID && doc.URL == citation.URL && doc.Title == citation.Title && strings.Contains(doc.Text, quote) {
					found = true
				}
			}
		}
		if !found {
			return errors.New("web evidence lacks its document citation")
		}
	}
	return nil
}
