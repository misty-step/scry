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
		if copy.Kind != "reference" {
			copy.Material = materialIdentity(copy.Material)
			if !copy.Graded {
				copy.Quiz = withoutAnswer(copy.Quiz)
				copy.PlanningReason = ""
			}
		} else if copy.Material != nil {
			material := *copy.Material
			material.Links = nil
			material.Provenance = store.Provenance{}
			material.Quiz = nil
			material.PlanningReason = ""
			material.SelectionPolicy = ""
			copy.Material = &material
		}
		state.Current = &copy
	}
	if state.Preview != nil {
		// A reference's title, diagram, citation and coverage can all carry
		// instruction. None belongs in a speculative response or hidden markup.
		state.Preview = &store.Presentation{Kind: state.Preview.Kind}
	}
	if state.Suspended != nil {
		suspended := make([]*store.Presentation, 0, len(state.Suspended))
		for _, item := range state.Suspended {
			if item != nil {
				suspended = append(suspended, &store.Presentation{ID: item.ID, Kind: item.Kind, GoalID: item.GoalID, GoalRevision: item.GoalRevision, GoalPinOrigin: item.GoalPinOrigin, BridgeID: item.BridgeID, Material: materialIdentity(item.Material), Assisted: item.Assisted, Graded: item.Graded, Practice: item.Practice})
			}
		}
		state.Suspended = suspended
	}
	if state.Bridge != nil {
		copy := *state.Bridge
		copy.Reason = ""
		if copy.Job != nil {
			job := *copy.Job
			job.Context = store.KnowledgeContext{}
			job.SourceText = ""
			job.Coverage.Missing = nil
			if job.Error != "" {
				job.Error = "Preparation did not finish. Open the saved job details for its recorded error."
			}
			copy.Job = &job
		}
		state.Bridge = &copy
	}
	return state
}

func materialIdentity(material *store.Material) *store.Material {
	if material == nil {
		return nil
	}
	return &store.Material{ID: material.ID, SourceID: material.SourceID, GoalID: material.GoalID, GoalRevision: material.GoalRevision, Version: material.Version, Kind: material.Kind, Level: material.Level, MetadataOnly: true, Archived: material.Archived, Unmapped: material.Unmapped, DueAt: material.DueAt, EstimatedSeconds: material.EstimatedSeconds}
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
	if state.Current != nil && state.Current.Kind == "reference" {
		if state.Current.Material == nil {
			s.fail(w, r, fmt.Errorf("reference presentation has no material"), page{})
			return
		}
		if s.guardInspection(w, r, "presentation", state.Current.ID) {
			return
		}
	}
	if state.Current != nil && state.Current.Graded && s.guardInspection(w, r, "presentation", state.Current.ID) {
		return
	}
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
	p := page{View: "review", Title: "Review", Active: "review", Review: state, Notice: notice}
	if state.Current != nil {
		p.ReviewGoalID = state.Current.GoalID
	} else if state.Bridge != nil {
		p.ReviewGoalID = state.Bridge.GoalID
	}
	s.render(w, r, http.StatusOK, p)
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
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(raw, "\\\r\n\t") {
		return ""
	}
	if u.RawQuery != "" {
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil || u.Path != "/library" || len(query) != 1 || len(query["q"]) != 1 || len(query.Get("q")) > 1024 {
			return ""
		}
		return u.String()
	}
	switch raw {
	case "/", "/history", "/library", "/export":
		return raw
	}
	parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	if !strings.HasPrefix(raw, "/") || len(parts) < 2 || len(parts) > 3 || parts[1] == "" {
		return ""
	}
	for _, c := range parts[1] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return ""
		}
	}
	switch parts[0] {
	case "sources", "units", "materials":
		if len(parts) == 2 {
			return raw
		}
	case "goals":
		if len(parts) == 2 || len(parts) == 3 && parts[2] == "plan" {
			return raw
		}
	case "quizzes":
		if len(parts) == 3 && parts[2] == "edit" {
			return raw
		}
	case "reviews":
		if len(parts) == 3 && parts[2] == "dispute" {
			return raw
		}
	}
	return ""
}

// Inspection is read-only until the owner explicitly accepts this fence. The
// store checks stable source identity and shared versioned coverage, including
// suspended bridge targets; a browser route is not an assistance loophole.
func (s *server) guardInspection(w http.ResponseWriter, r *http.Request, kind, id string) bool {
	access, err := s.store.InspectionAccess(r.Context(), kind, id)
	if err != nil {
		s.fail(w, r, err, page{})
		return true
	}
	if access.Allowed && !access.RequiresAssistance {
		return false
	}
	destination := safeReturn(inspectionDestination(kind, id))
	if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/review/") {
		destination = "/"
	}
	if destination == "" {
		destination = "/library"
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusConflict, map[string]any{
			"error":  "Open this content explicitly before reading it. Related unanswered targets are marked helped, never automatically graded.",
			"access": access, "kind": kind, "id": id, "csrf": r.Context().Value(csrfKey{}),
			"operation_id": randomToken(), "return_to": destination, "action": "/inspection/assist",
		})
		return true
	}
	s.render(w, r, http.StatusOK, page{View: "gate", Title: "Open saved learning content", Active: "library", Gate: access.RequiresAssistance, GateKind: kind, GateID: id, ReturnTo: destination})
	return true
}

func (s *server) assistInspection(w http.ResponseWriter, r *http.Request) {
	kind, id, op := r.PostForm.Get("kind"), r.PostForm.Get("id"), r.PostForm.Get("operation_id")
	destination := safeReturn(r.PostForm.Get("return_to"))
	if kind == "library" {
		id = strings.TrimSpace(id)
		if len(id) > 1024 {
			s.fail(w, r, store.ErrInvalid, page{Operation: op})
			return
		}
		destination = inspectionDestination(kind, id)
	}
	if op == "" || len(op) > 128 || destination == "" {
		s.fail(w, r, store.ErrInvalid, page{Operation: op})
		return
	}
	_, err := s.store.AssistInspection(r.Context(), kind, id, op)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	// The destination GET rechecks its own guard. No answered quiz, reference
	// body, knowledge statement, or bridge target is returned as a side channel.
	s.finish(w, r, destination, map[string]any{"inspection_recorded": true, "location": destination})
}
