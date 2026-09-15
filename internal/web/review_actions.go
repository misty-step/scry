package web

import (
	"net/http"

	"github.com/misty-step/scry/internal/store"
)

func (s *server) startBridge(w http.ResponseWriter, r *http.Request) {
	id, op := r.PostForm.Get("presentation_id"), r.PostForm.Get("operation_id")
	if id == "" || op == "" || len(op) > 128 {
		s.reviewFailure(w, r, store.ErrInvalid, op, "")
		return
	}
	state, err := s.store.StartBridge(r.Context(), id, op)
	if err != nil {
		s.reviewFailure(w, r, err, op, "")
		return
	}
	s.reviewResponse(w, r, state, "Your target is retained. Asking for foundations is not a failed answer.")
}

func (s *server) returnToTarget(w http.ResponseWriter, r *http.Request) {
	id, op := r.PostForm.Get("bridge_id"), r.PostForm.Get("operation_id")
	if id == "" || op == "" || len(op) > 128 {
		s.reviewFailure(w, r, store.ErrInvalid, op, "")
		return
	}
	state, err := s.store.ReturnToTarget(r.Context(), id, op)
	if err != nil {
		s.reviewFailure(w, r, err, op, "")
		return
	}
	s.reviewResponse(w, r, state, "The saved return state is restored. Help and practice remain part of the record.")
}

func (s *server) continueMaterial(w http.ResponseWriter, r *http.Request) {
	id, op := r.PostForm.Get("presentation_id"), r.PostForm.Get("operation_id")
	if id == "" || op == "" || len(op) > 128 {
		s.reviewFailure(w, r, store.ErrInvalid, op, "")
		return
	}
	state, err := s.store.ContinueMaterial(r.Context(), id, op)
	if err != nil {
		s.reviewFailure(w, r, err, op, "")
		return
	}
	s.reviewResponse(w, r, state, "Reading recorded once. It is not an assessment of recall.")
}

func (s *server) markAssisted(w http.ResponseWriter, r *http.Request) {
	id, op, reason := r.PostForm.Get("presentation_id"), r.PostForm.Get("operation_id"), r.PostForm.Get("reason")
	if id == "" || op == "" || len(op) > 128 || reason == "" || len(reason) > 2000 {
		s.reviewFailure(w, r, store.ErrInvalid, op, "")
		return
	}
	if _, err := s.store.MarkAssisted(r.Context(), id, reason, op); err != nil {
		s.reviewFailure(w, r, err, op, "")
		return
	}
	state, err := s.store.Review(r.Context())
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	s.reviewResponse(w, r, state, "Help recorded without grading an answer.")
}
