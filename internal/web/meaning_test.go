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
	return store.GeneratedQuiz{Kind: "recall", Grading: "semantic", Prompt: "Why can a browser reuse a cached page after a conditional request?", Answer: "The server confirms the stored copy is still current, so the body is not sent again.", Explanation: "A 304 response confirms the stored representation is current, so the client reuses it.", Basis: "topic", Rubric: &store.Rubric{Required: []store.RubricIdea{{Text: "The server confirms the stored copy is still current"}, {Text: "The body is not sent again"}}}}
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

func TestEditFormCannotChooseGrading(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	exactSource, err := s.Capture(ctx, store.CaptureInput{Text: "HTTP caching", Mode: "topic"}, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "recall", Prompt: "Which header carries an opaque validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"})
	meaningSource, err := s.Capture(ctx, store.CaptureInput{Text: "Conditional requests", Mode: "topic"}, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, meaningQuiz())
	exactDetail, err := s.Source(ctx, exactSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	meaningDetail, err := s.Source(ctx, meaningSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	exact, meaning := exactDetail.Quizzes[0], meaningDetail.Quizzes[0]
	forged := func(q store.Quiz) url.Values {
		return url.Values{"kind": {q.Kind}, "prompt": {q.Prompt}, "answer": {q.Answer}, "explanation": {q.Explanation}, "grading": {"semantic"}, "required_ideas": {"Anything at all"}, "contradictions": {"Nothing"}}
	}
	postEdit(t, app, exact, forged(exact))
	if edited, err := s.Quiz(ctx, exact.ID); err != nil || edited.Grading != "" || edited.Rubric != nil {
		t.Fatalf("forged grading accepted: %+v %v", edited, err)
	}
	form := forged(meaning)
	form.Set("grading", "exact")
	form.Set("explanation", "A 304 lets a stored copy be reused.")
	postEdit(t, app, meaning, form)
	edited, err := s.Quiz(ctx, meaning.ID)
	if err != nil || edited.Grading != "semantic" || edited.Rubric == nil || edited.Rubric.Required[0].Text == "Anything at all" {
		t.Fatalf("forged fields replaced rubric: %+v %v", edited, err)
	}
	form = forged(edited)
	form.Set("prompt", "What does a 304 response let a browser do?")
	postEdit(t, app, edited, form)
	reworded, err := s.Quiz(ctx, meaning.ID)
	if err != nil || reworded.Grading != "" || reworded.Rubric != nil {
		t.Fatalf("stale rubric survived rewording: %+v %v", reworded, err)
	}
}
