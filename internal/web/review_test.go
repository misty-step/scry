package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func completeSyntheticQuiz(t *testing.T, s *store.Store, quiz store.GeneratedQuiz) {
	t.Helper()
	ctx := context.Background()
	cost := int64(0)
	claim := func(kind string) *store.Job {
		t.Helper()
		job, err := s.ClaimJob(ctx, time.Minute, 1, 1000000)
		if err != nil || job == nil || job.Kind != kind {
			t.Fatalf("claim %s: %+v %v", kind, job, err)
		}
		return job
	}
	research := claim("research")
	if err := s.CompleteJob(ctx, research.ID, research.LeaseToken, store.GenerationResult{Model: "authored-test-fixture", PromptVersion: "fixture-v5"}, &cost); err != nil {
		t.Fatal(err)
	}
	plan := claim("plan")
	note := &store.NoteContent{Title: "Synthetic concept", Body: "This synthetic concept gives the question a place in the map and a short explanation of its meaning.", Basis: "topic"}
	result := store.GenerationResult{Plan: &store.PlanContent{Goal: "Synthetic learning goal", Concepts: []store.PlannedConcept{{Key: "c1", Name: "Synthetic concept", Summary: "A synthetic concept for a controlled browser test.", Note: note}}}, Model: "authored-test-fixture", PromptVersion: "fixture-v5"}
	if err := s.CompleteJob(ctx, plan.ID, plan.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	questions := claim("questions")
	context, err := s.JobContext(ctx, questions.ID)
	if err != nil || len(context.Concepts) != 1 {
		t.Fatalf("question concept: %+v %v", context, err)
	}
	quiz.Concept = context.Concepts[0].ID
	quiz.Level = "recall"
	if quiz.Kind == "recall" {
		quiz.AnswerForm = "exact"
	}
	if err := s.CompleteJob(ctx, questions.ID, questions.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{quiz}, Model: "authored-test-fixture", PromptVersion: "fixture-v5"}, &cost); err != nil {
		t.Fatal(err)
	}
}

func openReviewState(t *testing.T, s *store.Store) *store.Presentation {
	t.Helper()
	state, err := s.Review(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Intro != nil {
		state, err = s.AcknowledgeIntro(context.Background(), state.Intro.Concept.ID, randomToken(), false)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Current == nil {
		t.Fatalf("no review occurrence: %+v", state)
	}
	return state.Current
}

func reviewPage(t *testing.T, app http.Handler, cookie *http.Cookie) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := ownerRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	app.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("review page: %d %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func requirePresent(t *testing.T, page, label string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Errorf("%s: missing %q", label, want)
		}
	}
}

func TestPrivateReviewAnswerBoundary(t *testing.T) {
	secret := store.Quiz{Answer: "private-answer", Explanation: "private-explanation", Evidence: "private-evidence", Variants: []string{"private-variant"}, Citations: []store.Citation{{Title: "Published source", URL: "https://example.org"}}}
	for _, tc := range []struct {
		name         string
		graded, self bool
		visible      bool
	}{
		{"question", false, false, false}, {"self-check", false, true, true}, {"graded", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := privateReview(store.ReviewState{Current: &store.Presentation{Quiz: secret, Graded: tc.graded, SelfCheck: tc.self}, Preview: &store.Presentation{ID: "next-id", Quiz: secret, Answer: "private-answer"}})
			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			current := state.Current.Quiz
			if (current.Answer != "") != tc.visible || (current.Explanation != "") != tc.visible || (len(current.Variants) > 0) != tc.visible || (len(current.Citations) > 0) != tc.visible {
				t.Fatalf("answer boundary failed: %s", encoded)
			}
			if state.Preview.ID != "" || state.Preview.Quiz.Answer != "" || state.Preview.Answer != "" {
				t.Fatalf("preview disclosed answer or occurrence: %s", encoded)
			}
		})
	}
}

// Release-smoke contract: with a question awaiting an answer, the first GET /
// renders the answer form and the JSON keeps {review, csrf, operation_id};
// neither carries the answer or explanation.
func TestStreamJSONShapeAndForm(t *testing.T) {
	s, app := privateApp(t)
	if _, err := s.Capture(context.Background(), store.CaptureInput{Text: "Synthetic protocols", Mode: "topic"}, randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "recall", Prompt: "Which protocol secures HTTPS?", Answer: "ANSWER-TLS", Explanation: "EXPLANATION-HTTPS runs over TLS.", Basis: "topic"})
	openReviewState(t, s)
	r := ownerRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	var response struct {
		Review    *store.ReviewState `json:"review"`
		CSRF      string             `json:"csrf"`
		Operation string             `json:"operation_id"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Review == nil || response.Review.Current == nil || response.Review.Current.Graded || response.CSRF == "" || response.Operation == "" {
		t.Fatalf("stream JSON: %d %s", w.Code, w.Body.String())
	}
	cookie, _, _ := bootstrapForm(t, app)
	html := reviewPage(t, app, cookie)
	if !strings.Contains(html, `action="/review/answer"`) {
		t.Fatal("stream with a current question did not render the answer form")
	}
	for _, body := range []string{w.Body.String(), html} {
		if strings.Contains(body, "ANSWER-TLS") || strings.Contains(body, "EXPLANATION-HTTPS") {
			t.Fatalf("ungraded stream exposed the answer: %s", body)
		}
	}
}

func TestRetiredRoutesAreGone(t *testing.T) {
	_, app := privateApp(t)
	for _, path := range []string{"/foundations", "/foundations/old", "/library"} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, ownerRequest("GET", path, nil))
		if w.Code != 404 {
			t.Errorf("%s returned %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, ownerRequest("POST", "/review/foundation", url.Values{}))
	if w.Code != 403 {
		t.Fatalf("retired mutation bypassed CSRF: %d", w.Code)
	}
}
