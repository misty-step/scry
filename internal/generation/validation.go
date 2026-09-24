package generation

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

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
	// RequiredIdeas is empty for exact tasks. A nonempty list is the bounded
	// meaning rubric for a prose recall answer, authored in the same request.
	RequiredIdeas []string `json:"required_ideas"`
}

// maxRequiredIdeas bounds each generated rubric and its Jev battery size.
const maxRequiredIdeas = 4

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
	if issue := validateRequiredIdeas(q.RequiredIdeas, prompt); issue != "" {
		return issue
	}
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
	combined := q.Prompt + "\n" + q.Answer + "\n" + q.Explanation + "\n" + strings.Join(q.Choices, "\n") + "\n" + strings.Join(q.Variants, "\n") + "\n" + strings.Join(q.RequiredIdeas, "\n")
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
	if (q.Basis != "source" && q.Basis != "web") || strings.TrimSpace(q.Evidence) == "" || !strings.Contains(job.SourceText, q.Evidence) || len(strings.Fields(q.Evidence)) < 2 {
		return "unverified_source_quote"
	}
	evidence := normalized(q.Evidence)
	if (q.Basis == "source" && !answerSupported(answer, evidence)) || !numbersSupported(q.Answer, q.Evidence) || !numbersSupported(q.Explanation, q.Evidence) {
		return "unsupported_source_answer"
	}
	for _, variant := range q.Variants {
		if (q.Basis == "source" && !answerSupported(normalized(variant), evidence)) || !numbersSupported(variant, q.Evidence) {
			return "unsupported_source_variant"
		}
	}
	for _, idea := range q.RequiredIdeas {
		if (q.Basis == "source" && !answerSupported(normalized(idea), evidence)) || !numbersSupported(idea, q.Evidence) {
			return "unsupported_source_idea"
		}
	}
	for _, match := range quotedText.FindAllStringSubmatch(combined, -1) {
		for _, quote := range match[1:] {
			if quote != "" && !strings.Contains(job.SourceText, quote) {
				return "invented_quoted_text"
			}
		}
	}
	if qualifications.MatchString(q.Evidence) {
		// The answer check is unchanged. Required ideas are then added to the
		// same claim, so they can only make it stricter, never looser.
		claim := answer + " " + explanation
		if strengthensQualification(claim, evidence) || (len(q.RequiredIdeas) > 0 && strengthensQualification(claim+" "+normalized(strings.Join(q.RequiredIdeas, " ")), evidence)) {
			return "strengthened_source_claim"
		}
	}
	return ""
}

// validateRequiredIdeas checks only the structure of a generated rubric: its
// size, plain text, distinct ideas, and that no idea is printed in the prompt.
// v5 restricts rubrics to explain-level prose recall, not deterministic tasks.
func validateRequiredIdeas(ideas []string, prompt string) string {
	if len(ideas) > maxRequiredIdeas {
		return "quiz_bounds"
	}
	seen := make(map[string]bool, len(ideas))
	for _, idea := range ideas {
		key := normalized(idea)
		if key == "" || strings.TrimSpace(idea) != idea || len(idea) > 1024 || hasUnsafeControl(idea) || markup.MatchString(idea) || seen[key] {
			return "invalid_required_idea"
		}
		if phraseContains(prompt, key) {
			return "answer_leakage_or_vague_prompt"
		}
		seen[key] = true
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
