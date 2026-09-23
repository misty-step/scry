package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func meaningQuiz() store.GeneratedQuiz {
	return store.GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Prompt: "Why can a browser reuse a cached page after a conditional request?",
		Answer:      "The server confirms the stored copy is still current, so the body is not sent again.",
		Explanation: "A 304 response confirms the stored representation is current, so the client reuses it.",
		Basis:       "topic",
		Rubric:      &store.Rubric{Required: []store.RubricIdea{{Text: "The server confirms the stored copy is still current"}, {Text: "The body is not sent again"}}},
	}
}

func getPage(t *testing.T, app http.Handler, cookie *http.Cookie, path string) string {
	t.Helper()
	r := ownerRequest(http.MethodGet, path, nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d", path, w.Code)
	}
	return w.Body.String()
}

func postEdit(t *testing.T, app http.Handler, quiz store.Quiz, form url.Values) {
	t.Helper()
	cookie, csrf, operation := bootstrapForm(t, app)
	form.Set("csrf", csrf)
	form.Set("operation_id", operation)
	form.Set("version", strconv.Itoa(quiz.Version))
	r := ownerRequest(http.MethodPost, "/quizzes/"+quiz.ID+"/edit", form)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("save edit: %d %s", w.Code, w.Body.String())
	}
}

// The learner never picks grading: the editor has no mode selector and no
// rubric fields, for meaning-checked and exact questions alike.
func TestEditorHasNoGradingControls(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "HTTP caching", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, meaningQuiz())
	if _, err := s.Capture(ctx, "Validators", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "recall", Prompt: "Which header carries an opaque validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"})
	sources, err := s.Sources(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, _ := bootstrapForm(t, app)
	seen := 0
	for _, source := range sources {
		detail, err := s.Source(ctx, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, quiz := range detail.Quizzes {
			seen++
			page := getPage(t, app, cookie, "/quizzes/"+quiz.ID+"/edit")
			requireAbsent(t, page, "editor "+quiz.Grading,
				"Grading mode", `name="grading"`, "Semantic rubric", `name="required_ideas"`, `name="idea_cues"`,
				`name="contradictions"`, `name="contradiction_feedback"`, "Jev", "rubric")
			const help = "Answers in your own words count for this question."
			if quiz.Grading == "semantic" {
				requirePresent(t, page, "meaning editor", help, `aria-describedby="answer-help"`)
			} else {
				requireAbsent(t, page, "exact editor", help)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("expected two quizzes, saw %d", seen)
	}
}

// Forged form fields cannot author grading. An exact question stays exact, a
// meaning-checked question with unchanged wording keeps its rubric, and a
// reworded one becomes exact on its new version.
func TestEditFormCannotChooseGrading(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "HTTP caching", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "recall", Prompt: "Which header carries an opaque validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"})
	if _, err := s.Capture(ctx, "Conditional requests", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, meaningQuiz())
	sources, err := s.Sources(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	byGrading := map[string]store.Quiz{}
	for _, source := range sources {
		detail, err := s.Source(ctx, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		byGrading[detail.Quizzes[0].Grading] = detail.Quizzes[0]
	}
	exact, meaning := byGrading[""], byGrading["semantic"]
	forged := func(q store.Quiz) url.Values {
		return url.Values{
			"kind": {q.Kind}, "prompt": {q.Prompt}, "answer": {q.Answer}, "explanation": {q.Explanation},
			"grading": {"semantic"}, "required_ideas": {"Anything at all"}, "contradictions": {"Nothing"},
		}
	}

	postEdit(t, app, exact, forged(exact))
	if edited, err := s.Quiz(ctx, exact.ID); err != nil || edited.Version != 2 || edited.Grading != "" || edited.Rubric != nil {
		t.Fatalf("forged fields made an exact quiz meaning-graded: %+v %v", edited, err)
	}

	form := forged(meaning)
	form.Set("grading", "exact")
	form.Set("explanation", "A 304 means the stored copy can be reused without downloading it again.")
	postEdit(t, app, meaning, form)
	edited, err := s.Quiz(ctx, meaning.ID)
	if err != nil || edited.Version != 2 || edited.Grading != "semantic" || edited.Rubric == nil || len(edited.Rubric.Required) != 2 || edited.Rubric.Required[0].Text == "Anything at all" {
		t.Fatalf("unchanged wording lost or replaced its generated rubric: %+v %v", edited, err)
	}

	form = forged(edited)
	form.Set("prompt", "What does a 304 response let a browser do?")
	postEdit(t, app, edited, form)
	reworded, err := s.Quiz(ctx, meaning.ID)
	if err != nil || reworded.Version != 3 || reworded.Grading != "" || reworded.Rubric != nil {
		t.Fatalf("reworded question kept a stale rubric: %+v %v", reworded, err)
	}
}

func TestReviewTellsLearnerOwnWordsCountOnlyForMeaningChecks(t *testing.T) {
	s, app := privateApp(t)
	if _, err := s.Capture(context.Background(), "HTTP caching", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, meaningQuiz())
	cookie, _, _ := bootstrapForm(t, app)
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "meaning review", "Answer in your own words.", `data-pending="Checking meaning…"`)
	requireAbsent(t, page, "meaning review", "A short answer is enough.", "Grading mode", "rubric")

	s2, app2 := privateApp(t)
	if _, err := s2.Capture(context.Background(), "Validators", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s2, store.GeneratedQuiz{Kind: "recall", Prompt: "Which header carries an opaque validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"})
	cookie2, _, _ := bootstrapForm(t, app2)
	page = reviewPage(t, app2, cookie2)
	requirePresent(t, page, "exact review", "A short answer is enough.")
	requireAbsent(t, page, "exact review", "Answer in your own words.")
}
