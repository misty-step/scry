package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/misty-step/scry/internal/store"
)

func validateV5Question(raw json.RawMessage, job *store.Job, input store.JobContext, targets map[string]bool, contract coveragePlan, index int) (store.GeneratedQuiz, error) {
	var quiz struct {
		store.GeneratedQuiz
		RequiredIdeas []string `json:"required_ideas"`
		Covers        []string `json:"covers"`
	}
	if strictObject(raw, &quiz, "concept", "level", "answer_form", "kind", "prompt", "answer", "explanation", "basis", "evidence", "choices", "variants", "citations", "required_ideas", "covers") != nil {
		return store.GeneratedQuiz{}, errors.New("invalid question fields")
	}
	q := quiz.GeneratedQuiz
	if !targets[q.Concept] || (q.Level != "recognize" && q.Level != "recall" && q.Level != "explain" && q.Level != "apply") || (q.Kind == "choice" && q.AnswerForm != "") || (q.Kind == "recall" && q.AnswerForm != "exact" && q.AnswerForm != "flexible") {
		return q, errors.New("invalid question target or level")
	}
	if len(quiz.RequiredIdeas) > 0 && (q.Level != "explain" || q.Kind != "recall") {
		return q, errors.New("invalid choices or grading")
	}
	if job.Kind == "fix" && q.Concept != input.Quiz.ConceptID {
		return q, errors.New("correction changed concept")
	}
	if len(quiz.RequiredIdeas) > 0 {
		if issue := validateRequiredIdeas(quiz.RequiredIdeas, q.Prompt); issue != "" {
			return q, errors.New(issue)
		}
		rubric := &store.Rubric{Required: make([]store.RubricIdea, len(quiz.RequiredIdeas))}
		for j, idea := range quiz.RequiredIdeas {
			rubric.Required[j] = store.RubricIdea{Text: idea}
		}
		q.Grading, q.Rubric = "semantic", rubric
	}
	basis, quotes, citations, err := snapV5Evidence(q.Basis, []string{q.Evidence}, q.Citations, job, input)
	if err != nil {
		return q, err
	}
	q.Basis, q.Citations = basis, citations
	if len(quotes) > 0 {
		q.Evidence = quotes[0]
	} else {
		q.Evidence = ""
	}
	if err = validateV5Basis(q.Basis, []string{q.Evidence}, q.Citations, job, input); err != nil {
		return q, err
	}
	if job.Kind != "questions" || len(contract.Units) == 0 {
		quiz.Covers = nil
	}
	if contract.Task == "exact_text" && len(q.Variants) != 0 {
		return q, errors.New("exact wording changed")
	}
	q.Variants = safeV5Variants(q)
	draft := quizDraft{Evidence: q.Evidence, Basis: q.Basis, Kind: q.Kind, Prompt: q.Prompt, Answer: q.Answer, Explanation: q.Explanation, Choices: q.Choices, Variants: q.Variants, Covers: quiz.Covers, RequiredIdeas: quiz.RequiredIdeas}
	legacy := *job
	if q.Basis != "topic" {
		legacy.SourceKind = "source"
		legacy.SourceText = v5EvidenceText(q.Basis, job, input)
	} else {
		legacy.SourceKind = "topic"
	}
	if issue := validateQuiz(draft, &legacy); issue != "" {
		return q, fmt.Errorf("question failed %s", issue)
	}
	if len(contract.Units) > 0 && job.Kind == "questions" {
		if index >= len(contract.Units) || len(quiz.Covers) != 1 || quiz.Covers[0] != contract.Units[index].ID || !strings.Contains(q.Evidence, contract.Units[index].Text) || !unitIsTested(contract.Units[index].Text, draft, contract.Task) {
			return q, errors.New("required units changed or reordered")
		}
		if contract.Task == "exact_text" && (q.Kind != "recall" || q.Answer != contract.Units[index].Text || q.AnswerForm != "exact" || len(q.Variants) != 0) {
			return q, errors.New("exact wording changed")
		}
	}
	return q, nil
}

// A removed variant cannot make a typed answer easier to accept. Keep only
// variants that pass the same lexical/provenance fences as the shared validator.
func safeV5Variants(q store.GeneratedQuiz) []string {
	if q.Kind != "recall" || len(q.Variants) == 0 {
		return nil
	}
	kept := make([]string, 0, min(len(q.Variants), 8))
	prompt, answer, evidence := normalized(q.Prompt), normalized(q.Answer), normalized(q.Evidence)
	seen := map[string]bool{answer: true}
	for _, variant := range q.Variants {
		key := normalized(variant)
		if key == "" || strings.TrimSpace(variant) != variant || len(variant) > 1024 || hasUnsafeControl(variant) || markup.MatchString(variant) || seen[key] ||
			phraseContains(prompt, key) || phraseContains(answer, key) || phraseContains(key, answer) || strings.ContainsAny(variant, "*|") {
			continue
		}
		if q.Basis == "source" && !answerSupported(key, evidence) ||
			q.Basis != "topic" && !numbersSupported(variant, q.Evidence) {
			continue
		}
		kept = append(kept, variant)
		seen[key] = true
		if len(kept) == 8 {
			break
		}
	}
	return kept
}
