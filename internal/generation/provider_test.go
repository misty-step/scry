package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/store"
)

func topicDraft() store.GeneratedQuiz {
	return store.GeneratedQuiz{
		Key: "quiz_energy", Level: "target", EstimatedSeconds: 30,
		Kind: "recall", Basis: "topic", Evidence: "",
		Prompt:      "Which molecule directly supplies energy for many cellular processes?",
		Answer:      "ATP",
		Explanation: "ATP transfers chemical energy through phosphate-group reactions rather than serving as long-term genetic storage.",
		Choices:     []string{}, Variants: []string{}, Links: []store.GeneratedLink{{UnitKey: "unit_energy", Role: "assesses"}},
	}
}

func outputJSON(t *testing.T, kind string, quizzes ...store.GeneratedQuiz) string {
	t.Helper()
	bundle := store.GenerationResult{
		Coverage: store.CoverageReport{Kind: kind, Complete: true, Missing: []string{}},
		Units:    []store.GeneratedUnit{}, Relations: []store.GeneratedRelation{},
		Materials: []store.GeneratedMaterial{}, Quizzes: []store.GeneratedQuiz{}, Suggestions: []store.GeneratedSuggestion{},
	}
	seen := make(map[string]bool)
	for index, quiz := range quizzes {
		if quiz.Key == "" {
			quiz.Key = fmt.Sprintf("quiz_%d", index)
		}
		if quiz.Level == "" {
			quiz.Level = "target"
		}
		if quiz.EstimatedSeconds == 0 {
			quiz.EstimatedSeconds = 30
		}
		if len(quiz.Links) == 0 {
			quiz.Links = []store.GeneratedLink{{UnitKey: fmt.Sprintf("unit_%d", index), Role: "assesses"}}
		}
		statement := quiz.Explanation
		if quiz.Basis == "source" {
			statement = quiz.Evidence
		}
		for _, link := range quiz.Links {
			if seen[link.UnitKey] {
				continue
			}
			unitKind := "foundation"
			if kind == "exact_text" {
				unitKind = "exact_text"
			}
			bundle.Units = append(bundle.Units, store.GeneratedUnit{Key: link.UnitKey, Statement: statement, Kind: unitKind})
			seen[link.UnitKey] = true
		}
		bundle.Quizzes = append(bundle.Quizzes, quiz)
		if index == 0 && kind != "complete_set" && kind != "exact_text" && len(meaningful(normalized(statement))) >= 6 {
			bundle.Materials = append(bundle.Materials, store.GeneratedMaterial{
				Key: "instruction", Kind: "explanation", Title: "Energy transfer foundation",
				Body: statement, Basis: quiz.Basis, Evidence: quiz.Evidence, EstimatedSeconds: 45,
				Links: []store.GeneratedLink{{UnitKey: quiz.Links[0].UnitKey, Role: "teaches"}},
			})
		}
	}
	return bundleJSON(t, bundle)
}

func bundleJSON(t *testing.T, bundle store.GenerationResult) string {
	t.Helper()
	// The wire has exactly six roots; model provenance belongs to the provider
	// receipt and must never be accepted from the model's generated object.
	body, err := json.Marshal(struct {
		Coverage    store.CoverageReport        `json:"coverage"`
		Units       []store.GeneratedUnit       `json:"units"`
		Relations   []store.GeneratedRelation   `json:"relations"`
		Materials   []store.GeneratedMaterial   `json:"materials"`
		Quizzes     []store.GeneratedQuiz       `json:"quizzes"`
		Suggestions []store.GeneratedSuggestion `json:"suggestions"`
	}{bundle.Coverage, bundle.Units, bundle.Relations, bundle.Materials, bundle.Quizzes, bundle.Suggestions})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func envelopeJSON(t *testing.T, content, finish string, price json.RawMessage) string {
	t.Helper()
	envelope := map[string]any{
		"model": "boundary-model",
		"choices": []any{map[string]any{
			"finish_reason": finish,
			"message":       map[string]any{"role": "assistant", "content": content},
		}},
	}
	if price != nil {
		envelope["usage"] = map[string]any{"cost": price}
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func localConfig(endpoint string) Config {
	return Config{Endpoint: endpoint, Model: "boundary-model", DailyBudgetMicros: 1_000_000, ReservationMicros: 100_000}
}

func responseServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPaidRejectedResponsesRetainReportedCost(t *testing.T) {
	valid := outputJSON(t, "concepts", topicDraft())
	cases := []struct{ name, body string }{
		{"malformed quiz JSON", envelopeJSON(t, `{"coverage":`, "stop", json.RawMessage(`0.012345`))},
		{"malformed choice envelope", `{"choices":"not-an-array","usage":{"cost":0.012345}}`},
		{"truncated completion", envelopeJSON(t, valid, "length", json.RawMessage(`0.012345`))},
		{"choice-level provider error", strings.Replace(envelopeJSON(t, valid, "stop", json.RawMessage(`0.012345`)), `"finish_reason":"stop"`, `"error":{"message":"provider failed"},"finish_reason":"stop"`, 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := responseServer(t, test.body, http.StatusOK)
			worker := New(nil, localConfig(server.URL))
			result, cost, failure := worker.generate(context.Background(), &store.Job{Kind: "capture", Context: store.KnowledgeContext{Version: store.KnowledgeContextVersion}, SourceText: "mitochondria", SourceKind: "topic", Attempts: 1})
			if failure == nil || failure.retry || len(result.Quizzes) != 0 {
				t.Fatalf("malformed paid output must not publish or trigger an unearned retry: result=%+v failure=%+v", result, failure)
			}
			if cost == nil || *cost != 12_345 {
				t.Fatalf("rejected paid response lost its receipt: %v", cost)
			}
		})
	}
}

func TestUnknownPriceDoesNotBecomeFreeRetry(t *testing.T) {
	server := responseServer(t, `{"error":{"message":"provider-private-detail"}}`, http.StatusServiceUnavailable)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || failure.retry || response.cost != nil {
		t.Fatalf("unknown error spend must remain unknown without an automatic paid retry: cost=%v failure=%+v", response.cost, failure)
	}
	if strings.Contains(failure.message, "provider-private-detail") {
		t.Fatal("raw provider details leaked into durable user-visible failure")
	}
}

func TestExplicitFreeTransientRejectionCanRetry(t *testing.T) {
	server := responseServer(t, `{"error":{"message":"busy"},"usage":{"cost":0}}`, http.StatusTooManyRequests)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || !failure.retry || response.cost == nil || *response.cost != 0 {
		t.Fatalf("an explicitly free transient rejection should permit bounded durable backoff: cost=%v failure=%+v", response.cost, failure)
	}
}

func TestRedirectCannotDiscloseSourceOrCredentials(t *testing.T) {
	var reached atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	cfg := localConfig(redirect.URL)
	cfg.APIKey = "private-boundary-test-key"
	worker := New(nil, cfg)
	_, failure := worker.call(context.Background(), []byte(`{"source":"private-source"}`))
	if failure == nil || reached.Load() != 0 {
		t.Fatalf("redirect received private data: requests=%d failure=%+v", reached.Load(), failure)
	}
}

func TestOversizeResponseNeverPublishesOrInventsCost(t *testing.T) {
	server := responseServer(t, strings.Repeat(" ", maxResponseBytes+1), http.StatusOK)
	worker := New(nil, localConfig(server.URL))
	response, failure := worker.call(context.Background(), []byte(`{}`))
	if failure == nil || failure.retry || response.cost != nil || response.content != "" {
		t.Fatalf("oversized response escaped the bounded boundary: %+v %+v", response, failure)
	}
}

func TestQualityRepairRequiresKnownReceiptAndStopsAfterOnePass(t *testing.T) {
	bad := topicDraft()
	bad.Prompt = "Why does ATP directly supply cellular energy?" // Leaks the answer.
	server := responseServer(t, envelopeJSON(t, outputJSON(t, "concepts", bad), "stop", json.RawMessage(`0.001`)), http.StatusOK)
	worker := New(nil, localConfig(server.URL))
	job := &store.Job{Kind: "capture", Context: store.KnowledgeContext{Version: store.KnowledgeContextVersion}, SourceText: "mitochondria", SourceKind: "topic", Attempts: 1}
	_, cost, first := worker.generate(context.Background(), job)
	if first == nil || !first.retry || cost == nil || *cost != 1_000 {
		t.Fatalf("small paid quality failure did not request a separately reserved repair: %v %+v", cost, first)
	}
	job.Attempts, job.Error = 2, first.message
	_, cost, second := worker.generate(context.Background(), job)
	if second == nil || second.retry || cost == nil || *cost != 1_000 {
		t.Fatalf("quality repair did not stop after one additional paid claim: %v %+v", cost, second)
	}
}

func TestOversizeCompleteTaskIsRejectedBeforeAnyPaidTransmission(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	worker := New(nil, localConfig(server.URL))
	job := &store.Job{Kind: "capture", Context: store.KnowledgeContext{Version: store.KnowledgeContextVersion}, SourceKind: "source", SourceText: "Learn all entries:\n" + strings.Repeat("- entry\n", 61), Attempts: 1}
	result, cost, failure := worker.generate(context.Background(), job)
	if calls.Load() != 0 || failure == nil || failure.retry || cost == nil || *cost != 0 || len(result.Quizzes) != 0 {
		t.Fatalf("oversized complete task was billed or silently sampled: calls=%d result=%+v cost=%v failure=%+v", calls.Load(), result, cost, failure)
	}
}

func TestObservedGapRejectsUnusableTriggerOrTargetBeforeTransmission(t *testing.T) {
	cases := []struct {
		name   string
		change func(*store.Job)
	}{
		{"missing observation identity", func(job *store.Job) { job.ObservationID = "" }},
		{"omitted trigger with another failure", func(job *store.Job) { job.Context.Evidence[0].ID = "newer_failure"; job.Context.Omitted.Evidence = 1 }},
		{"duplicate trigger identity", func(job *store.Job) { job.Context.Evidence = append(job.Context.Evidence, job.Context.Evidence[0]) }},
		{"different observed material", func(job *store.Job) { job.Context.Evidence[0].MaterialID = "different_material" }},
		{"different observed version", func(job *store.Job) { job.Context.Evidence[0].MaterialVersion++ }},
		{"practice is not a direct review", func(job *store.Job) { job.Context.Evidence[0].Kind = "practice" }},
		{"revealed answer is not a failure", func(job *store.Job) { job.Context.Evidence[0].Outcome = "revealed" }},
		{"rating does not record failure", func(job *store.Job) { job.Context.Evidence[0].Rating = 3 }},
		{"assisted failure", func(job *store.Job) { job.Context.Evidence[0].Assisted = true }},
		{"disputed failure", func(job *store.Job) { job.Context.Evidence[0].Disputed = true }},
		{"corrected failure", func(job *store.Job) { job.Context.Evidence[0].CorrectionIDs = []string{"material_correction"} }},
		{"different encountered mode", func(job *store.Job) { job.Context.Evidence[0].Mode = "choice" }},
		{"missing chosen goal", func(job *store.Job) { job.Context.GoalID = "" }},
		{"different chosen goal revision", func(job *store.Job) { job.Context.GoalRevision++ }},
		{"missing target", func(job *store.Job) { job.Context.Materials = nil }},
		{"superseded target", func(job *store.Job) { job.Context.Materials[0].Version++ }},
		{"metadata-only target", func(job *store.Job) { job.Context.Materials[0].MetadataOnly = true }},
		{"missing presentation", func(job *store.Job) { job.TargetPresentationID = "" }},
		{"wrong presentation material version", func(job *store.Job) { job.TargetPresentationVersion++ }},
		{"oversized original trigger", func(job *store.Job) {
			job.Context.Evidence[0].AssistanceReasons = []string{strings.Repeat("Original context. ", 1024)}
		}},
		{"oversized original target", func(job *store.Job) {
			job.Context.Materials[0].Quiz.Explanation = strings.Repeat("Original explanation. ", 2048)
		}},
		{"oversized scoped source", func(job *store.Job) { job.SourceText = strings.Repeat("s", maxSourceBytes+1) }},
		{"unsupported scoped provenance", func(job *store.Job) { job.SourceKind = "retrieved" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			job, _ := observedGapFixture(t)
			test.change(&job)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer server.Close()
			worker := New(nil, localConfig(server.URL))
			result, cost, failure := worker.generate(context.Background(), &job)
			if calls.Load() != 0 || failure == nil || failure.retry || cost == nil || *cost != 0 || len(result.Quizzes)+len(result.Materials) != 0 {
				t.Fatalf("invalid observed work reached a paid request or invented support: calls=%d result=%+v cost=%v failure=%+v", calls.Load(), result, cost, failure)
			}
		})
	}
}

func TestObservedGapTransmitsExactTriggerAndTargetWithinBoundedScope(t *testing.T) {
	job, bundle := observedGapFixture(t)
	job.SourceKind = "source"
	job.SourceText = "Learn all cellular entries:\n" + strings.Repeat("- A separate entry in the saved source inventory.\n", 61)
	bundle.Materials[0].Basis, bundle.Quizzes[0].Basis = "background", "background"
	bundle.Materials[0].Body = "Generated background: " + bundle.Materials[0].Body
	bundle.Quizzes[0].Explanation = "Generated background: " + bundle.Quizzes[0].Explanation
	target, trigger := job.Context.Materials[0], job.Context.Evidence[0]
	job.Context.Materials, job.Context.Evidence = nil, nil
	job.Context.Omitted.Materials, job.Context.Omitted.Evidence = 2, 3
	for index := range 24 {
		job.Context.Materials = append(job.Context.Materials, store.Material{
			ID: fmt.Sprintf("unrelated_%02d", index), SourceID: job.SourceID, Version: 1, Kind: "quiz", Unmapped: true,
			Quiz: &store.Quiz{ID: fmt.Sprintf("old_quiz_%02d", index), Version: 1, Kind: "recall", Prompt: fmt.Sprintf("Which saved detail belongs to entry %d?", index), Answer: "an unrelated detail", Explanation: "This previously saved quiz remains unrelated to the observed energy target.", Basis: "topic"},
		})
		recent := trigger
		recent.ID, recent.At = fmt.Sprintf("recent_%02d", index), trigger.At.Add(time.Duration(index+1)*time.Minute)
		recent.Outcome, recent.Rating = "correct", 3
		job.Context.Evidence = append(job.Context.Evidence, recent)
	}
	// Neither history order nor category ceilings can crowd out the original
	// failed target. Other observations retain their limitations as whole rows.
	job.Context.Evidence[0].Assisted, job.Context.Evidence[0].Disputed = true, true
	job.Context.Evidence[0].CorrectionIDs = []string{"review_dispute"}
	job.Context.Evidence[0].Corrections = []learning.Correction{{ID: "review_dispute", Kind: "dispute", Reason: "The saved answer was disputed.", At: trigger.At.Add(2 * time.Minute)}}
	job.Context.Evidence[0].AssistanceReasons = []string{"The explanation was previously revealed."}
	job.Context.Materials = append(job.Context.Materials, target)
	job.Context.Evidence = append(job.Context.Evidence, trigger)
	before, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan []byte, 1)
	response := envelopeJSON(t, bundleJSON(t, bundle), "stop", json.RawMessage(`0.000031`))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, response)
	}))
	defer server.Close()
	worker := New(nil, localConfig(server.URL))
	result, cost, failure := worker.generate(context.Background(), &job)
	if failure != nil || cost == nil || *cost != 31 || !result.Coverage.Complete || len(result.Materials) != 1 || len(result.Quizzes) != 1 || result.Quizzes[0].Level != "foundation" {
		t.Fatalf("useful ambiguous-target support was replaced by full-source mapping or rejected: %+v %v %+v", result, cost, failure)
	}
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(<-received, &request); err != nil {
		t.Fatal(err)
	}
	var input struct {
		ObservationID             string                 `json:"observation_id"`
		TargetMaterialID          string                 `json:"target_material_id"`
		TargetMaterialVersion     int                    `json:"target_material_version"`
		TargetPresentationID      string                 `json:"target_presentation_id"`
		TargetPresentationVersion int                    `json:"target_presentation_version"`
		Context                   store.KnowledgeContext `json:"knowledge_context"`
		Plan                      coveragePlan           `json:"task_contract"`
	}
	for _, message := range request.Messages {
		if message.Role == "user" {
			if err := json.Unmarshal([]byte(message.Content), &input); err != nil {
				t.Fatal(err)
			}
		}
	}
	if input.ObservationID != trigger.ID || input.TargetMaterialID != target.ID || input.TargetMaterialVersion != target.Version || input.TargetPresentationID != job.TargetPresentationID || input.TargetPresentationVersion != target.Version || input.Context.GoalID != job.GoalID || input.Context.GoalRevision != job.GoalRevision {
		t.Fatalf("paid request changed original failure/target identity or chosen goal authority: %+v", input)
	}
	if input.Plan.Task != "infer" || len(input.Plan.Units) != 0 || input.Context.Omitted.Materials <= 2 || input.Context.Omitted.Evidence <= 3 {
		t.Fatalf("bounded support regenerated unrelated inventory or hid context omissions: %+v", input)
	}
	if len(input.Context.Materials) == 0 || !reflect.DeepEqual(input.Context.Materials[0], target) || len(input.Context.Evidence) < 2 || !reflect.DeepEqual(input.Context.Evidence[0], trigger) || !reflect.DeepEqual(input.Context.Evidence[1], job.Context.Evidence[0]) {
		t.Fatalf("exact versioned target, ambiguous trigger or limited history was lost: %+v", input.Context)
	}
	after, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("preparing the paid request mutated saved context")
	}
}
