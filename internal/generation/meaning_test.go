package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

func conceptualDraft() quizDraft {
	return quizDraft{
		Kind: "recall", Basis: "topic", Evidence: "",
		Prompt:        "Why can a browser reuse a cached page after sending a conditional request?",
		Answer:        "The server confirms that the stored copy is still current, so it does not resend the body.",
		Explanation:   "A conditional request asks the origin to compare validators; a 304 lets the client reuse its stored copy instead of downloading it again.",
		Choices:       []string{},
		Variants:      []string{},
		Covers:        []string{},
		RequiredIdeas: []string{"The server confirms the stored copy is still current", "The body is not sent again"},
	}
}

func choiceDraft() quizDraft {
	return quizDraft{
		Kind: "choice", Basis: "topic", Evidence: "",
		Prompt:      "Which status code tells a client its cached copy is still current?",
		Answer:      "304 Not Modified",
		Explanation: "304 confirms the stored representation is current, unlike 200, which sends a full body again.",
		Choices:     []string{"304 Not Modified", "200 OK", "412 Precondition Failed"},
		Variants:    []string{}, Covers: []string{},
	}
}

func recordingCritic(t *testing.T, calls *atomic.Int32, semanticBatteries *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request semantic.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, ok := request.Questions["rubric_misaligned"]; ok {
			semanticBatteries.Add(1)
		}
		answers := map[string]any{}
		for key := range request.Questions {
			answers[key] = map[string]any{"type": "noul", "noul": .01}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "fixture-critic", "answers": answers, "usage": map[string]any{"cost": .00001, "input_tokens": 100, "output_tokens": 1}})
	}))
	t.Cleanup(server.Close)
	return server
}

// One ordinary capture sends ONE generation request. That request asks for
// required ideas, and its answer alone decides grading: the conceptual recall
// publishes with a rubric, while the crisp fact and the choice stay exact.
// No routing call, classifier, or learner setting is involved.
func TestOrdinaryGenerationAuthorsMeaningRubricInTheSameRequest(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "HTTP caching", 100_000)
	var generationCalls atomic.Int32
	var requestBody []byte
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generationCalls.Add(1)
		requestBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, envelopeJSON(t, outputJSON(t, "concepts", conceptualDraft(), topicDraft(), choiceDraft()), "stop", json.RawMessage(`0.001`)))
	}))
	defer generator.Close()
	var criticCalls, semanticBatteries atomic.Int32
	critic := recordingCritic(t, &criticCalls, &semanticBatteries)
	cfg := localConfig(generator.URL)
	cfg.Critic = semantic.NewClient(semantic.Config{Endpoint: critic.URL})
	cfg.CriticSpending = store.SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 1_000_000}
	if err := New(s, cfg).process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if generationCalls.Load() != 1 {
		t.Fatalf("generation calls=%d want exactly one", generationCalls.Load())
	}
	var sent struct {
		Messages       []map[string]string `json:"messages"`
		ResponseFormat struct {
			JSONSchema struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	if err := json.Unmarshal(requestBody, &sent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sent.ResponseFormat.JSONSchema.Schema), `"required_ideas"`) || !strings.Contains(sent.Messages[0]["content"], "MEANING-CHECKED RECALL") {
		t.Fatal("the generation request does not ask for required ideas")
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil || len(saved.Quizzes) != 3 || saved.Job.Status != "complete" {
		t.Fatalf("publication: %+v %v", saved, err)
	}
	byPrompt := map[string]store.Quiz{}
	for _, quiz := range saved.Quizzes {
		byPrompt[quiz.Prompt] = quiz
	}
	conceptual := byPrompt[conceptualDraft().Prompt]
	if conceptual.Grading != "semantic" || conceptual.Rubric == nil || len(conceptual.Rubric.Required) != 2 ||
		conceptual.Rubric.Required[1].Text != "The body is not sent again" || conceptual.Rubric.Required[0].Cue != "" || len(conceptual.Rubric.Contradictions) != 0 {
		t.Fatalf("conceptual recall did not carry its generated rubric: %+v", conceptual)
	}
	for _, prompt := range []string{topicDraft().Prompt, choiceDraft().Prompt} {
		if quiz := byPrompt[prompt]; quiz.Grading != "" || quiz.Rubric != nil {
			t.Fatalf("deterministic quiz gained meaning grading: %+v", quiz)
		}
	}
	// The existing bounded critic judges every candidate once; only the
	// meaning-checked candidate adds the rubric-alignment judgment.
	if criticCalls.Load() != 3 || semanticBatteries.Load() != 1 {
		t.Fatalf("critic calls=%d rubric batteries=%d", criticCalls.Load(), semanticBatteries.Load())
	}
	if saved.Job.Candidates == nil || saved.Job.Candidates.Result.PromptVersion != "scry-go-quiz-v4" {
		t.Fatalf("prompt version not attributed: %+v", saved.Job.Candidates)
	}
}

func TestRequiredIdeasNeverMakeDeterministicTasksMeaningGraded(t *testing.T) {
	topic := store.Job{SourceKind: "topic", SourceText: "HTTP caching"}
	topicPlan, err := planTask(topic.SourceText, topic.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	withIdeas := func(q quizDraft, ideas ...string) quizDraft {
		q.RequiredIdeas = ideas
		return q
	}
	cases := []struct {
		name  string
		draft quizDraft
		issue string
	}{
		{"choice", withIdeas(choiceDraft(), "The copy is still current"), "rubric_on_exact_task"},
		{"idea printed in prompt", withIdeas(conceptualDraft(), "a cached page"), "answer_leakage_or_vague_prompt"},
		{"duplicate idea", withIdeas(conceptualDraft(), "The body is not sent again", "the body is NOT sent again."), "invalid_required_idea"},
		{"blank idea", withIdeas(conceptualDraft(), " "), "invalid_required_idea"},
		{"markup idea", withIdeas(conceptualDraft(), "<b>The body</b> is not sent"), "invalid_required_idea"},
		{"more than four ideas", withIdeas(conceptualDraft(), "One idea here", "Two idea here", "Three idea here", "Four idea here", "Five idea here"), "quiz_bounds"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, issues, err := validateOutput(outputJSON(t, "concepts", test.draft), &topic, topicPlan)
			if err != nil || len(result.Quizzes) != 0 || len(issues) != 1 || issues[0] != test.issue {
				t.Fatalf("result=%+v issues=%v err=%v", result.Quizzes, issues, err)
			}
		})
	}

	exact := store.Job{SourceKind: "source", SourceText: "Recite verbatim:\nSo much depends\nupon"}
	exactPlan, err := planTask(exact.SourceText, exact.SourceKind)
	if err != nil || exactPlan.Task != "exact_text" {
		t.Fatalf("exact plan: %+v %v", exactPlan, err)
	}
	line := quizDraft{Kind: "recall", Basis: "source", Evidence: "So much depends", Prompt: "Recite the opening line of the poem you saved.", Answer: "So much depends",
		Explanation: "The opening line sets up everything the poem goes on to name.", Choices: []string{}, Variants: []string{}, Covers: []string{"u1"}}
	result, issues, err := validateOutput(outputJSON(t, "exact_text", withIdeas(line, "Something depends on something")), &exact, exactPlan)
	if err != nil || len(result.Quizzes) != 0 || len(issues) != 1 || issues[0] != "rubric_on_exact_task" {
		t.Fatalf("exact text accepted a rubric: %+v %v %v", result.Quizzes, issues, err)
	}
	result, _, err = validateOutput(outputJSON(t, "exact_text", line), &exact, exactPlan)
	if err != nil || len(result.Quizzes) != 1 || result.Quizzes[0].Grading != "" {
		t.Fatalf("exact text without a rubric changed: %+v %v", result.Quizzes, err)
	}

	set := store.Job{SourceKind: "source", SourceText: "Learn all of these:\n- ETag\n- Last-Modified"}
	setPlan, err := planTask(set.SourceText, set.SourceKind)
	if err != nil || setPlan.Task != "complete_set" {
		t.Fatalf("set plan: %+v %v", setPlan, err)
	}
	member := quizDraft{Kind: "recall", Basis: "source", Evidence: "ETag", Prompt: "Name the first validator header in your saved list.", Answer: "ETag",
		Explanation: "ETag is the first header in the list; Last-Modified comes second.", Choices: []string{}, Variants: []string{}, Covers: []string{"u1"}}
	result, issues, err = validateOutput(outputJSON(t, "complete_set", withIdeas(member, "It is an entity tag header")), &set, setPlan)
	if err != nil || len(result.Quizzes) != 0 || len(issues) != 1 || issues[0] != "rubric_on_exact_task" {
		t.Fatalf("complete set accepted a rubric: %+v %v %v", result.Quizzes, issues, err)
	}
}

func TestSourceRubricMustBeSupportedByItsEvidence(t *testing.T) {
	job, draft := sourceDraft()
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	draft.Prompt = "What do mitochondria do in aerobic cells, and by which process?"
	draft.Answer = "They produce ATP by oxidative phosphorylation."
	draft.RequiredIdeas = []string{"Mitochondria produce ATP", "They use oxidative phosphorylation"}
	result, issues, err := validateOutput(outputJSON(t, "concepts", draft), &job, plan)
	if err != nil || len(issues) != 0 || len(result.Quizzes) != 1 || result.Quizzes[0].Grading != "semantic" || len(result.Quizzes[0].Rubric.Required) != 2 {
		t.Fatalf("supported source rubric rejected: %+v %v %v", result.Quizzes, issues, err)
	}
	draft.RequiredIdeas = []string{"Mitochondria produce ATP", "Chloroplast pigments absorb sunlight"}
	result, issues, err = validateOutput(outputJSON(t, "concepts", draft), &job, plan)
	if err != nil || len(result.Quizzes) != 0 || len(issues) != 1 || issues[0] != "unsupported_source_idea" {
		t.Fatalf("an idea outside the quoted evidence published: %+v %v %v", result.Quizzes, issues, err)
	}
}

func TestQualifiedSourceIdeaCannotBecomeACertainty(t *testing.T) {
	evidence := "Regular sleep may improve recall in some adults."
	job := store.Job{SourceKind: "source", SourceText: evidence}
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		t.Fatal(err)
	}
	draft := quizDraft{
		Kind: "recall", Basis: "source", Evidence: evidence,
		Prompt:      "According to your note, what can regular sleep do for recall in some adults?",
		Answer:      "It may improve recall in some adults.",
		Explanation: "The note is hedged: sleep may improve recall, and only in some adults.",
		Choices:     []string{}, Variants: []string{}, Covers: []string{},
		RequiredIdeas: []string{"Regular sleep may improve recall"},
	}
	result, issues, err := validateOutput(outputJSON(t, "concepts", draft), &job, plan)
	if err != nil || len(issues) != 0 || len(result.Quizzes) != 1 {
		t.Fatalf("a hedged rubric was rejected: %v %v", issues, err)
	}
	draft.RequiredIdeas = []string{"Regular sleep always improves recall in adults"}
	result, issues, err = validateOutput(outputJSON(t, "concepts", draft), &job, plan)
	if err != nil || len(result.Quizzes) != 0 || len(issues) != 1 || issues[0] != "strengthened_source_claim" {
		t.Fatalf("a rubric strengthened a qualified claim: %+v %v %v", result.Quizzes, issues, err)
	}
}

func TestMissingRequiredIdeasFieldIsMalformedNotExact(t *testing.T) {
	job := store.Job{SourceKind: "topic", SourceText: "HTTP caching"}
	plan, _ := planTask(job.SourceText, job.SourceKind)
	body := outputJSON(t, "concepts", topicDraft())
	body = strings.Replace(body, `,"required_ideas":[]`, "", 1)
	if _, _, err := validateOutput(body, &job, plan); err == nil {
		t.Fatal("an output without the strict required_ideas field was accepted")
	}
}
