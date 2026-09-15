package generation

import (
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func sourceDraft() (store.Job, quizDraft) {
	evidence := "In aerobic cells, mitochondria use oxidative phosphorylation to produce ATP, a carrier of chemical energy."
	job := store.Job{SourceKind: "source", SourceText: evidence + "\nChloroplasts capture light energy in photosynthetic cells."}
	quiz := quizDraft{
		Kind: "recall", Basis: "source", Evidence: evidence,
		Prompt:      "Which molecule is produced by mitochondrial oxidative phosphorylation?",
		Answer:      "ATP",
		Explanation: "ATP carries chemical energy in aerobic cells; the excerpt specifically links its production to mitochondrial oxidative phosphorylation.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{},
	}
	return job, quiz
}

func TestSourceQuotationCannotLeakAnswerOrLaunderUnseenCitations(t *testing.T) {
	job, valid := sourceDraft()
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*quizDraft)
	}{
		{"answer-bearing source in prompt", func(q *quizDraft) { q.Prompt = q.Evidence + " Which energy carrier is produced?" }},
		{"fabricated evidence", func(q *quizDraft) { q.Evidence = "Mitochondria produce ATP by nuclear fission." }},
		{"real quote but unrelated answer", func(q *quizDraft) { q.Answer = "DNA" }},
		{"invented quotation in explanation", func(q *quizDraft) { q.Explanation += ` The author calls this "the universal proof of energy".` }},
		{"unseen external citation", func(q *quizDraft) { q.Explanation += " See https://unseen.invalid/proof for confirmation." }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			quiz := valid
			test.mutate(&quiz)
			result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Quizzes) != 0 {
				t.Fatalf("unsafe source attribution published: %+v", result.Quizzes)
			}
		})
	}
	result, _, err := validateOutput(outputJSON(t, "concepts", valid), &job, plan)
	if err != nil || len(result.Quizzes) != 1 || result.Quizzes[0].Evidence != valid.Evidence {
		t.Fatalf("exact supported source material did not survive the gate: %+v %v", result, err)
	}
}

func TestSingleLetterIdentifierDoesNotLeakThroughAnArticle(t *testing.T) {
	job := store.Job{SourceKind: "source", SourceText: "A: maps a hostname to an IPv4 address."}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := quizDraft{
		Kind: "choice", Basis: "source", Evidence: job.SourceText,
		Prompt:      "Which DNS record type maps a hostname to an IPv4 address?",
		Answer:      "A",
		Explanation: "An A record maps a hostname to an IPv4 address, as the supplied mapping states.",
		Choices:     []string{"A", "AAAA", "CNAME"}, Variants: []string{}, Covers: []string{},
	}
	result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 1 || result.Quizzes[0].Answer != "A" {
		t.Fatalf("an ordinary article suppressed a supported identifier quiz: %+v %v", result, err)
	}
	quiz.Prompt = "Which DNS record called A maps a hostname to an IPv4 address?"
	result, _, err = validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("an explicitly revealed identifier was published: %+v %v", result, err)
	}
}

func TestTopicExpansionCannotPretendItsSeedIsEvidence(t *testing.T) {
	job := store.Job{SourceText: "mitochondria", SourceKind: "topic"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := topicDraft()
	quiz.Basis, quiz.Evidence = "source", "mitochondria"
	result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("topic seed became false proof: %+v %v", result, err)
	}
}

func TestCompleteSetCoverageMustBeActuallyTestedInSourceOrder(t *testing.T) {
	job := store.Job{SourceKind: "source", SourceText: "Learn all mappings:\n- Red: Crimson\n- Blue: Azure\n- Green: Viridian"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	keys, values := []string{"Red", "Blue", "Green"}, []string{"Crimson", "Azure", "Viridian"}
	drafts := make([]quizDraft, 0, 3)
	for index, unit := range plan.Units {
		drafts = append(drafts, quizDraft{
			Kind: "recall", Basis: "source", Evidence: unit.Text,
			Prompt:      "Which value is paired with " + keys[index] + " in the supplied mapping?",
			Answer:      values[index],
			Explanation: "The supplied mapping explicitly pairs " + keys[index] + " with " + values[index] + "; each key identifies its own separate value.",
			Choices:     []string{}, Variants: []string{}, Covers: []string{unit.ID},
		})
	}
	complete, _, err := validateOutput(outputJSON(t, "complete_set", drafts...), &job, plan)
	if err != nil || complete.Partial || len(complete.Quizzes) != 3 {
		t.Fatalf("complete ordered set was not accepted: %+v %v", complete, err)
	}
	partial, _, err := validateOutput(outputJSON(t, "complete_set", drafts[0], drafts[2]), &job, plan)
	if err != nil || !partial.Partial || len(partial.Quizzes) != 2 {
		t.Fatalf("sampled set claimed completion: %+v %v", partial, err)
	}
	reordered, _, err := validateOutput(outputJSON(t, "complete_set", drafts[1], drafts[0], drafts[2]), &job, plan)
	if err != nil || !reordered.Partial || len(reordered.Quizzes) != 2 || reordered.Quizzes[0].Answer != "Azure" || reordered.Quizzes[1].Answer != "Viridian" {
		t.Fatalf("reordered set escaped coverage fencing: %+v %v", reordered, err)
	}
	laundered := drafts[0]
	laundered.Evidence = job.SourceText
	laundered.Covers = []string{"u2"} // Entire source is quoted, but Blue is not tested.
	partial, _, err = validateOutput(outputJSON(t, "complete_set", laundered), &job, plan)
	if err != nil || len(partial.Quizzes) != 0 {
		t.Fatalf("coverage IDs were trusted without a tested unit: %+v %v", partial, err)
	}
}

func TestExactTextCannotParaphraseOrClaimPartialInventoryComplete(t *testing.T) {
	job := store.Job{SourceKind: "source", SourceText: "Memorize this text verbatim:\nHope is the thing with feathers\nThat perches in the soul"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := quizDraft{
		Kind: "recall", Basis: "source", Evidence: plan.Units[0].Text,
		Prompt:      "Recite the first line of the supplied poem.",
		Answer:      plan.Units[0].Text,
		Explanation: "The opening line introduces hope through a feathered creature, retaining the supplied wording rather than replacing it with a paraphrase.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{"u1"},
	}
	result, _, err := validateOutput(outputJSON(t, "exact_text", quiz), &job, plan)
	if err != nil || !result.Partial || len(result.Quizzes) != 1 || result.Quizzes[0].Answer != "Hope is the thing with feathers" {
		t.Fatalf("partial recitation inventory was misrepresented: %+v %v", result, err)
	}
	quiz.Answer += "." // Even plausible punctuation is not the supplied exact text.
	result, _, err = validateOutput(outputJSON(t, "exact_text", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("rewritten exact text published: %+v %v", result, err)
	}
}

func TestModelInventedFiniteInventoryCannotProveCompleteness(t *testing.T) {
	job := store.Job{SourceKind: "topic", SourceText: "Learn all cellular energy carriers"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := validateOutput(outputJSON(t, "complete_set", topicDraft()), &job, plan)
	if err != nil || !result.Partial || len(result.Quizzes) != 1 {
		t.Fatalf("model self-reported coverage became a completeness claim: %+v %v", result, err)
	}
}

func TestDuplicateJSONMembersAndFencedObjectsFailClosed(t *testing.T) {
	job := store.Job{SourceKind: "topic", SourceText: "mitochondria"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	valid := outputJSON(t, "concepts", topicDraft())
	inputs := []string{
		"```json\n" + valid + "\n```",
		strings.Replace(valid, `"answer":"ATP"`, `"answer":"DNA","answer":"ATP"`, 1),
		valid + valid,
	}
	for _, content := range inputs {
		result, _, err := validateOutput(content, &job, plan)
		if err == nil || len(result.Quizzes) != 0 {
			t.Fatalf("ambiguous or wrapped object was accepted: %+v %v", result, err)
		}
	}
}

func TestQualifiedSourceCannotBeStrengthenedIntoCausation(t *testing.T) {
	job := store.Job{SourceKind: "source", SourceText: "The observational study found that exercise was associated with lower blood pressure."}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := quizDraft{
		Kind: "recall", Basis: "source", Evidence: job.SourceText,
		Prompt:      "What outcome was associated with exercise in the observational study?",
		Answer:      "lower blood pressure",
		Explanation: "Exercise causes lower blood pressure because the observational study proves a causal relationship.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{},
	}
	result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("qualified source was strengthened: %+v %v", result, err)
	}
	quiz.Explanation = "The study found an association with lower blood pressure; an observational association does not prove causation."
	result, _, err = validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 1 {
		t.Fatalf("faithful qualification was rejected: %+v %v", result, err)
	}
}

func TestOverlappingNumericDistractorsCannotHaveTwoCorrectAnswers(t *testing.T) {
	job := store.Job{SourceKind: "topic", SourceText: "inclusive numeric intervals"}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := quizDraft{
		Kind: "choice", Basis: "topic", Evidence: "",
		Prompt:      "Which inclusive interval contains the integer five?",
		Answer:      "0–5",
		Explanation: "The first interval includes five at its upper endpoint; inclusion of both endpoints determines membership.",
		Choices:     []string{"0–5", "5–10", "11–15"}, Variants: []string{}, Covers: []string{},
	}
	result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("overlapping intervals published: %+v %v", result, err)
	}
	quiz.Choices[1] = "6–10"
	result, _, err = validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 1 {
		t.Fatalf("disjoint intervals rejected: %+v %v", result, err)
	}
}

func TestPossibleSourceClaimDoesNotBecomeCertain(t *testing.T) {
	evidence := "Certain medicines may cause drowsiness in susceptible adults."
	job := store.Job{SourceKind: "source", SourceText: evidence + "\nThe supplied note does not make a universal claim."}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	quiz := quizDraft{
		Kind: "recall", Basis: "source", Evidence: evidence,
		Prompt:      "What effect may these medicines have in susceptible adults?",
		Answer:      "drowsiness",
		Explanation: "These medicines cause drowsiness in susceptible adults, so the symptom follows treatment.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{},
	}
	result, _, err := validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 0 {
		t.Fatalf("possibility became certainty: %+v %v", result, err)
	}
	quiz.Explanation = "These medicines may cause drowsiness in susceptible adults; the source describes a possibility rather than a guaranteed outcome."
	result, _, err = validateOutput(outputJSON(t, "concepts", quiz), &job, plan)
	if err != nil || len(result.Quizzes) != 1 {
		t.Fatalf("preserved uncertainty was rejected: %+v %v", result, err)
	}
}
