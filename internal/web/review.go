package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/misty-step/scry/internal/store"
)

// Public review transport defensively removes answer-bearing fields even if a
// future store implementation returns a richer internal presentation.
func privateReview(state store.ReviewState) store.ReviewState {
	if state.Current != nil {
		copy := *state.Current
		if !copy.Graded {
			copy.Quiz = withoutAnswer(copy.Quiz)
		}
		state.Current = &copy
	}
	if state.Preview != nil {
		copy := *state.Preview
		copy.ID = ""
		copy.Answer = ""
		copy.Quiz = withoutAnswer(copy.Quiz)
		state.Preview = &copy
	}
	return state
}

func withoutAnswer(q store.Quiz) store.Quiz {
	q.Answer = ""
	q.Explanation = ""
	q.Evidence = ""
	q.Variants = nil
	return q
}

func (s *server) review(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Review(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.reviewResponse(w, r, state, "")
}

func (s *server) reviewResponse(w http.ResponseWriter, r *http.Request, state store.ReviewState, notice string) {
	state = privateReview(state)
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"review": state, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	if r.Method == http.MethodPost && !isHTMX(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		w.Header().Set("HX-Replace-Url", "/")
	} else {
		w.Header().Set("HX-Push-Url", "/")
	}
	s.render(w, r, http.StatusOK, page{View: "review", Title: "Review", Active: "review", Review: state, Notice: notice})
}

func (s *server) preview(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Review(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	state = privateReview(state)
	current := ""
	if state.Current != nil {
		current = state.Current.ID
	}
	jsonResponse(w, http.StatusOK, map[string]any{"after": current, "preview": state.Preview})
}

func (s *server) answer(w http.ResponseWriter, r *http.Request) {
	s.submit(w, r, false)
}

func (s *server) reveal(w http.ResponseWriter, r *http.Request) {
	s.submit(w, r, true)
}

func (s *server) submit(w http.ResponseWriter, r *http.Request, reveal bool) {
	id, op := r.PostForm.Get("presentation_id"), r.PostForm.Get("operation_id")
	answer := r.PostForm.Get("answer")
	if reveal {
		answer = ""
	}
	if len(answer) > 8192 || id == "" || op == "" || len(op) > 128 {
		s.reviewFailure(w, r, fmt.Errorf("%w: missing occurrence or oversized answer", store.ErrInvalid), op, answer)
		return
	}
	p, err := s.store.Submit(r.Context(), id, op, answer, reveal)
	if err != nil {
		s.reviewFailure(w, r, err, op, answer)
		return
	}
	if reveal {
		if destination := safeReturn(r.PostForm.Get("return_to")); destination != "" {
			s.finish(w, r, destination, map[string]any{"presentation": p, "location": destination})
			return
		}
	}
	state, err := s.store.Review(r.Context())
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	notice := ""
	if state.Current == nil || state.Current.ID != p.ID {
		notice = "That attempt was already recorded. This is your current review."
	}
	s.reviewResponse(w, r, state, notice)
}

func (s *server) reviewFailure(w http.ResponseWriter, r *http.Request, err error, op, answer string) {
	state, readErr := s.store.Review(r.Context())
	if readErr != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	state = privateReview(state)
	// Preserve the entered text on a definite rejection; never attach it to a
	// different occurrence after a competing tab moved the session forward.
	if state.Current != nil && state.Current.ID == r.PostForm.Get("presentation_id") && !state.Current.Graded {
		state.Current.Answer = answer
		state.Current.Draft = answer
	}
	if state.Current == nil || state.Current.ID != r.PostForm.Get("presentation_id") {
		op = ""
	}
	s.fail(w, r, err, page{View: "review", Title: "Review", Active: "review", Review: state, Operation: op})
}

func (s *server) next(w http.ResponseWriter, r *http.Request) {
	id := r.PostForm.Get("presentation_id")
	if id == "" {
		s.reviewFailure(w, r, store.ErrInvalid, "", "")
		return
	}
	state, err := s.store.Next(r.Context(), id)
	if err != nil {
		s.reviewFailure(w, r, err, "", "")
		return
	}
	s.reviewResponse(w, r, state, "")
}

func safeReturn(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "\\") {
		return ""
	}
	if raw == "/history" || raw == "/library" || strings.HasPrefix(raw, "/sources/") || strings.HasPrefix(raw, "/quizzes/") || strings.HasPrefix(raw, "/reviews/") {
		return raw
	}
	return ""
}

func (s *server) coldReview(r *http.Request) (*store.Presentation, error) {
	current, err := s.store.Current(r.Context())
	if err != nil {
		return nil, err
	}
	if current != nil && !current.Graded {
		return current, nil
	}
	return nil, nil
}

func (s *server) gate(w http.ResponseWriter, r *http.Request, current *store.Presentation) {
	if wantsJSON(r) {
		jsonResponse(w, http.StatusConflict, map[string]any{
			"error":           "This material contains the answer to your current question. Reveal it first, or finish that review.",
			"presentation_id": current.ID, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken(), "return_to": r.URL.Path,
		})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "gate", Title: "Inspect this material", Active: "library", Gate: current, ReturnTo: r.URL.Path})
}
