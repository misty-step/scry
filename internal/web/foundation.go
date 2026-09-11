package web

import (
	"net/http"
	"strconv"

	"github.com/misty-step/scry/internal/store"
)

func (s *server) requestFoundation(w http.ResponseWriter, r *http.Request) {
	revision, parseErr := strconv.Atoi(r.PostForm.Get("bridge_revision"))
	if parseErr != nil {
		s.reviewFailure(w, r, store.ErrInvalid, r.PostForm.Get("operation_id"), r.PostForm.Get("answer"))
		return
	}
	id, err := s.store.RequestFoundation(r.Context(), r.PostForm.Get("presentation_id"), r.PostForm.Get("operation_id"), r.PostForm.Get("answer"), revision)
	if err != nil {
		s.reviewFailure(w, r, err, r.PostForm.Get("operation_id"), r.PostForm.Get("answer"))
		return
	}
	s.finish(w, r, "/foundations/"+id, map[string]string{"id": id})
}

func (s *server) foundation(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.FoundationBridge(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"bridge": b, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "foundation", Title: "Foundations", Active: "review", Bridge: b})
}

func (s *server) advanceFoundation(w http.ResponseWriter, r *http.Request) {
	revision, _ := strconv.Atoi(r.PostForm.Get("revision"))
	err := s.store.AdvanceFoundation(r.Context(), r.PathValue("id"), revision, r.PostForm.Get("operation_id"), r.PostForm.Get("action"), r.PostForm.Get("answer"))
	if err != nil {
		b, readErr := s.store.FoundationBridge(r.Context(), r.PathValue("id"))
		if readErr != nil {
			s.fail(w, r, err, page{})
			return
		}
		p := page{View: "foundation", Title: "Foundations", Active: "review", Bridge: b}
		if b.Revision == revision && b.Phase == "practice" {
			p.Text, p.Operation = r.PostForm.Get("answer"), r.PostForm.Get("operation_id")
		}
		s.fail(w, r, err, p)
		return
	}
	location := "/foundations/" + r.PathValue("id")
	if r.PostForm.Get("action") == "return" {
		location = "/"
	}
	s.finish(w, r, location, map[string]string{"location": location})
}

func (s *server) foundations(w http.ResponseWriter, r *http.Request) {
	bridges, err := s.store.FoundationBridges(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, bridges)
		return
	}
	s.render(w, r, http.StatusOK, page{View: "foundations", Title: "Saved foundations", Active: "library", Bridges: bridges})
}
