package generation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

type outputCoverage struct {
	Kind     string   `json:"kind"`
	Complete bool     `json:"complete"`
	Missing  []string `json:"missing"`
}

type quizDraft struct {
	Evidence    string   `json:"evidence"`
	Basis       string   `json:"basis"`
	Kind        string   `json:"kind"`
	Prompt      string   `json:"prompt"`
	Answer      string   `json:"answer"`
	Explanation string   `json:"explanation"`
	Choices     []string `json:"choices"`
	Variants    []string `json:"variants"`
	Covers      []string `json:"covers"`
}

var (
	markup         = regexp.MustCompile(`(?i)<[/!]?[a-z][^>]*>|\[[^\]]+\]\([^\)]+\)|` + "```")
	quotedText     = regexp.MustCompile(`["“]([^"”\n]{2,})["”]|‘([^’\n]{2,})’|(?:^|[\s(])'([^'\n]{3,})'(?:$|[\s.,!?:;)])`)
	citations      = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>()]+|\bdoi:[^\s]+|\[[0-9]+\]`)
	falseSource    = regexp.MustCompile(`(?i)\b(according to (the |this )?(source|passage|excerpt|text)|the (source|passage|excerpt|text) (says|states|shows|proves))\b`)
	catchAll       = regexp.MustCompile(`(?i)\b(all|none|both|any) of (the |these )?(above|below|options|choices)|\b(not enough information|cannot be determined)\b`)
	keyedOption    = regexp.MustCompile(`^[A-Ea-e][.)]\s`)
	numbers        = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?%?`)
	qualifications = regexp.MustCompile(`(?i)\b(may|might|could|sometimes|often|usually|typically|generally|suggests?|associated|association|correlat\w*|observational|not|no|never|cannot)\b`)
	strongClaims   = regexp.MustCompile(`\b(always|never|everyone|proves?|guarantees?|causes?|causal|causation|certainly|definitely)\b`)
	numericRange   = regexp.MustCompile(`^\s*(-?[0-9]+(?:\.[0-9]+)?)\s*(?:-|–|—|to)\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([^0-9]*)$`)
	numericPoint   = regexp.MustCompile(`^\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([^0-9]*)$`)
)

func strictObject(data []byte, target any, fields ...string) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil || len(object) != len(fields) {
		return errors.New("missing or unknown JSON fields")
	}
	for _, field := range fields {
		value, found := object[field]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("missing or null JSON field")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func validateOutput(content string, job *store.Job, plan coveragePlan) (store.GenerationResult, []string, error) {
	result := store.GenerationResult{}
	data := []byte(content)
	if len(data) > maxContentBytes || !utf8.Valid(data) || checkJSON(data, 12) != nil {
		return result, nil, errors.New("invalid quiz JSON")
	}
	var raw struct {
		Coverage json.RawMessage   `json:"coverage"`
		Quizzes  []json.RawMessage `json:"quizzes"`
	}
	if err := strictObject(data, &raw, "coverage", "quizzes"); err != nil {
		return result, nil, err
	}
	var coverage outputCoverage
	if err := strictObject(raw.Coverage, &coverage, "kind", "complete", "missing"); err != nil {
		return result, nil, err
	}
	switch coverage.Kind {
	case "concepts", "vocabulary", "procedure", "complete_set", "exact_text":
	default:
		return result, nil, errors.New("unsupported task classification")
	}
	if len(raw.Quizzes) > maxQuizzes || len(coverage.Missing) > maxQuizzes {
		return result, nil, errors.New("quiz or coverage count exceeds limit")
	}
	for _, missing := range coverage.Missing {
		if len(missing) > 512 || hasUnsafeControl(missing) {
			return result, nil, errors.New("invalid coverage detail")
		}
	}
	drafts := make([]quizDraft, len(raw.Quizzes))
	for index, rawQuiz := range raw.Quizzes {
		if err := strictObject(rawQuiz, &drafts[index], "evidence", "basis", "kind", "prompt", "answer", "explanation", "choices", "variants", "covers"); err != nil {
			return result, nil, err
		}
	}
	if plan.Task != "infer" && coverage.Kind != plan.Task {
		return result, []string{"task_mismatch"}, nil
	}
	finite := coverage.Kind == "complete_set" || coverage.Kind == "exact_text"
	unitIndex := make(map[string]int, len(plan.Units))
	for index, unit := range plan.Units {
		unitIndex[unit.ID] = index
	}
	covered := make(map[string]bool, len(plan.Units))
	seenPrompts := make(map[string]bool, len(drafts))
	issues := make([]string, 0)
	lastUnit, lastExactOffset := -1, -1
	for _, draft := range drafts {
		if issue := validateQuiz(draft, job); issue != "" {
			issues = append(issues, issue)
			continue
		}
		promptKey := normalized(draft.Prompt)
		if seenPrompts[promptKey] {
			issues = append(issues, "duplicate_question")
			continue
		}
		if len(draft.Covers) > 1 || (len(plan.Units) == 0 && len(draft.Covers) != 0) {
			issues = append(issues, "invented_coverage")
			continue
		}
		unit := -1
		if len(plan.Units) > 0 {
			if len(draft.Covers) != 1 {
				issues = append(issues, "missing_coverage_unit")
				continue
			}
			var found bool
			unit, found = unitIndex[draft.Covers[0]]
			if !found || covered[draft.Covers[0]] || (plan.Ordered && unit <= lastUnit) {
				issues = append(issues, "repeated_or_reordered_unit")
				continue
			}
			if !strings.Contains(draft.Evidence, plan.Units[unit].Text) || !unitIsTested(plan.Units[unit].Text, draft, coverage.Kind) {
				issues = append(issues, "unit_not_actually_tested")
				continue
			}
		}
		if coverage.Kind == "exact_text" {
			if draft.Kind != "recall" || len(draft.Variants) != 0 || job.SourceKind != "source" || !strings.Contains(job.SourceText, draft.Answer) {
				issues = append(issues, "exact_text_changed")
				continue
			}
			if unit < 0 {
				offset := strings.Index(job.SourceText, draft.Answer)
				if offset <= lastExactOffset {
					issues = append(issues, "exact_text_reordered")
					continue
				}
				lastExactOffset = offset
			}
		}
		seenPrompts[promptKey] = true
		if unit >= 0 {
			covered[draft.Covers[0]], lastUnit = true, unit
		}
		result.Quizzes = append(result.Quizzes, store.GeneratedQuiz{
			Kind: draft.Kind, Prompt: draft.Prompt, Answer: draft.Answer,
			Explanation: draft.Explanation, Evidence: draft.Evidence, Basis: draft.Basis,
			Choices: draft.Choices, Variants: draft.Variants,
		})
	}
	unverifiable := finite && (plan.Unverified || len(plan.Units) == 0)
	missingUnits := len(plan.Units) - len(covered)
	result.Partial = len(issues) > 0 || !coverage.Complete || len(coverage.Missing) > 0 || unverifiable || missingUnits > 0
	result.Note = fmt.Sprintf("%d quiz(es) passed structural and provenance checks; this is not independent fact-checking.", len(result.Quizzes))
	if len(issues) > 0 {
		result.Note += fmt.Sprintf(" %d candidate(s) rejected; their paid usage is included. Checks: %s.", len(issues), strings.Join(issues[:min(4, len(issues))], ", "))
	}
	if len(plan.Units) > 0 {
		result.Note += fmt.Sprintf(" Source-unit coverage: %d/%d, in the supplied order.", len(covered), len(plan.Units))
	}
	if unverifiable {
		result.Note += " Partial: complete-set or exact-text coverage cannot be established without a complete authoritative input; no completeness claim is made."
	} else if missingUnits > 0 || !coverage.Complete || len(coverage.Missing) > 0 {
		result.Note += " Partial: some requested material is missing or uncertain. Inspect the source and clarify or split the task before retrying."
	}
	return result, issues, nil
}

func validateQuiz(q quizDraft, job *store.Job) string {
	for _, field := range []struct {
		value string
		limit int
	}{{q.Prompt, 4096}, {q.Answer, 1024}, {q.Explanation, 8192}} {
		if strings.TrimSpace(field.value) == "" || len(field.value) > field.limit || hasUnsafeControl(field.value) || markup.MatchString(field.value) {
			return "invalid_quiz_text"
		}
	}
	if len(q.Evidence) > 8192 || hasUnsafeControl(q.Evidence) || len(q.Variants) > 8 || len(q.Choices) > 5 {
		return "quiz_bounds"
	}
	prompt, answer, explanation := normalized(q.Prompt), normalized(q.Answer), normalized(q.Explanation)
	leaksAnswer := phraseContains(prompt, answer)
	// Preserve case for single-letter identifiers: an ordinary article does
	// not reveal an uppercase label, but naming that label explicitly does.
	if letter, size := utf8.DecodeRuneInString(q.Answer); leaksAnswer && size == len(q.Answer) && unicode.IsUpper(letter) {
		leaksAnswer = false
		for word := range strings.FieldsFuncSeq(q.Prompt, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if word == q.Answer {
				leaksAnswer = true
				break
			}
		}
	}
	if len(strings.Fields(prompt)) < 4 || answer == "" || leaksAnswer {
		return "answer_leakage_or_vague_prompt"
	}
	if len(strings.Fields(explanation)) < 6 || len(meaningful(explanation)) < 3 || explanation == prompt || explanation == answer {
		return "uninformative_explanation"
	}
	if q.Kind == "choice" {
		if len(q.Choices) < 3 || len(q.Variants) != 0 {
			return "invalid_choice_shape"
		}
		found := false
		keys := make([]string, 0, len(q.Choices))
		for _, choice := range q.Choices {
			key := normalized(choice)
			if strings.TrimSpace(choice) != choice || key == "" || len(choice) > 1024 || hasUnsafeControl(choice) || markup.MatchString(choice) || catchAll.MatchString(choice) || keyedOption.MatchString(choice) {
				return "invalid_distractor"
			}
			for index, previous := range keys {
				if previous == key || overlappingOptions(q.Choices[index], choice, previous, key) {
					return "overlapping_distractors"
				}
			}
			keys = append(keys, key)
			found = found || choice == q.Answer
		}
		if !found {
			return "answer_not_displayed_choice"
		}
	} else if q.Kind == "recall" {
		if len(q.Choices) != 0 {
			return "invalid_recall_shape"
		}
		seen := map[string]bool{answer: true}
		for _, variant := range q.Variants {
			key := normalized(variant)
			if key == "" || strings.TrimSpace(variant) != variant || len(variant) > 1024 || hasUnsafeControl(variant) || markup.MatchString(variant) || seen[key] || phraseContains(prompt, key) || phraseContains(answer, key) || phraseContains(key, answer) || strings.ContainsAny(variant, "*|") {
				return "invalid_recall_variant"
			}
			seen[key] = true
		}
	} else {
		return "unsupported_quiz_kind"
	}
	combined := q.Prompt + "\n" + q.Answer + "\n" + q.Explanation + "\n" + strings.Join(q.Choices, "\n") + "\n" + strings.Join(q.Variants, "\n")
	for _, citation := range citations.FindAllString(combined, -1) {
		if job.SourceKind != "source" || !strings.Contains(job.SourceText, citation) {
			return "invented_citation"
		}
	}
	if job.SourceKind == "topic" {
		if q.Basis != "topic" || q.Evidence != "" || falseSource.MatchString(combined) {
			return "false_topic_evidence"
		}
		return ""
	}
	if q.Basis != "source" || strings.TrimSpace(q.Evidence) == "" || !strings.Contains(job.SourceText, q.Evidence) || len(strings.Fields(q.Evidence)) < 2 {
		return "unverified_source_quote"
	}
	evidence := normalized(q.Evidence)
	if !answerSupported(answer, evidence) || !numbersSupported(q.Answer, q.Evidence) || !numbersSupported(q.Explanation, q.Evidence) {
		return "unsupported_source_answer"
	}
	for _, variant := range q.Variants {
		if !answerSupported(normalized(variant), evidence) || !numbersSupported(variant, q.Evidence) {
			return "unsupported_source_variant"
		}
	}
	for _, match := range quotedText.FindAllStringSubmatch(combined, -1) {
		for _, quote := range match[1:] {
			if quote != "" && !strings.Contains(job.SourceText, quote) {
				return "invented_quoted_text"
			}
		}
	}
	if qualifications.MatchString(q.Evidence) && strengthensQualification(answer+" "+explanation, evidence) {
		return "strengthened_source_claim"
	}
	return ""
}

func normalized(text string) string {
	var value strings.Builder
	value.Grow(len(text))
	space := true
	for _, char := range text {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			value.WriteRune(unicode.ToLower(char))
			space = false
		} else if !space {
			value.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(value.String())
}

func phraseContains(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(phrase); {
		index := strings.Index(text[offset:], phrase)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(phrase)
		if (index == 0 || text[index-1] == ' ') && (end == len(text) || text[end] == ' ') {
			return true
		}
		offset = index + 1
	}
	return false
}

func meaningful(text string) []string {
	words := strings.Fields(text)
	out := words[:0]
	for _, word := range words {
		switch word {
		case "a", "an", "the", "of", "to", "in", "on", "at", "by", "and", "or", "for", "is", "are", "was", "were", "be", "it", "its", "this", "that", "with", "from", "as", "has", "have", "had", "which", "what", "does", "do", "can":
			continue
		}
		out = append(out, word)
	}
	return out
}

func answerSupported(answer, evidence string) bool {
	if phraseContains(evidence, answer) {
		return true
	}
	words := meaningful(answer)
	if len(words) == 0 {
		return false
	}
	matched := 0
	for _, word := range words {
		if phraseContains(evidence, word) {
			matched++
		}
	}
	return matched*3 >= len(words)*2
}

func numbersSupported(text, evidence string) bool {
	available := numbers.FindAllString(evidence, -1)
	for _, number := range numbers.FindAllString(text, -1) {
		found := false
		for _, permitted := range available {
			found = found || number == permitted
		}
		if !found {
			return false
		}
	}
	return true
}

func unitIsTested(unit string, q quizDraft, task string) bool {
	if task == "exact_text" {
		return q.Answer == unit
	}
	questionAndAnswer := normalized(q.Prompt + " " + q.Answer)
	words := meaningful(normalized(unit))
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		if !phraseContains(questionAndAnswer, word) {
			return false
		}
	}
	return true
}

func strengthensQualification(claim, evidence string) bool {
	if !qualifications.MatchString(claim) {
		return true
	}
	for _, location := range strongClaims.FindAllStringIndex(claim, -1) {
		word := claim[location[0]:location[1]]
		if phraseContains(evidence, word) && (word == "never" || word == "always" || word == "everyone") {
			continue
		}
		previous := strings.Fields(claim[:location[0]])
		if len(previous) > 3 {
			previous = previous[len(previous)-3:]
		}
		negated := false
		for _, token := range previous {
			negated = negated || token == "not" || token == "no" || token == "cannot" || token == "t" || token == "may" || token == "might" || token == "could" || token == "sometimes"
		}
		if !negated {
			return true
		}
	}
	return false
}

func interval(option string) (float64, float64, string, bool) {
	if match := numericRange.FindStringSubmatch(option); match != nil {
		start, err1 := strconv.ParseFloat(match[1], 64)
		end, err2 := strconv.ParseFloat(match[2], 64)
		return start, end, normalized(match[3]), err1 == nil && err2 == nil && start <= end
	}
	if match := numericPoint.FindStringSubmatch(option); match != nil {
		point, err := strconv.ParseFloat(match[1], 64)
		return point, point, normalized(match[2]), err == nil
	}
	return 0, 0, "", false
}

func overlappingOptions(left, right, leftKey, rightKey string) bool {
	leftMin, leftMax, leftUnit, leftNumeric := interval(left)
	rightMin, rightMax, rightUnit, rightNumeric := interval(right)
	if leftNumeric && rightNumeric {
		return leftUnit == rightUnit && leftMin <= rightMax && rightMin <= leftMax
	}
	return phraseContains(leftKey, rightKey) || phraseContains(rightKey, leftKey)
}
