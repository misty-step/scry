package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/misty-step/scry/internal/store"
)

// Constellation layout: up to four stars per row with odd rows staggered. A
// short row is centered, and the drawing is only as tall as its rows, so a
// goal with two concepts does not sit in an empty panel.
func starX(i, n int) int {
	row := i / 4
	inRow := min(4, n-row*4)
	return 47 + (4-inRow)*38 + (i%4)*76 + row%2*17
}
func starY(i int) int                        { return 25 + (i/4)*49 + (i%3)*5 }
func constellationHeight(n int) int          { return 60 + (max(n, 1)-1)/4*49 }
func constellation(view store.GoalView) bool { return len(view.Concepts) > 0 }

func (s *server) mapPage(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 1024 {
		s.fail(w, r, fmt.Errorf("%w: use a shorter search", store.ErrInvalid), page{View: "map", Title: "Map", Active: "map"})
		return
	}
	view, err := s.store.Map(r.Context(), query)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	// Material from the current question's own capture, and search excerpts
	// for any concept its answer could be read from, stay hidden until
	// assistance has been recorded.
	if current != nil {
		linked, err := s.linkedConcepts(r, current)
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		hits := view.Hits[:0]
		for _, hit := range view.Hits {
			if !linked[hit.ConceptID] && hit.ID != current.Quiz.ID && (hit.Kind != "source" || hit.ID != current.Quiz.SourceID) {
				hits = append(hits, hit)
			}
		}
		view.Hits = hits
		for i := range view.Unmapped {
			// Replace the whole value so fields added later stay hidden too.
			if u := view.Unmapped[i]; u.ID == current.Quiz.SourceID {
				view.Unmapped[i] = store.Source{ID: u.ID, Kind: u.Kind, Mode: u.Mode, CreatedAt: u.CreatedAt, Text: currentMaterial}
			}
		}
		hideCurrentMaterial(current, view.Preparing)
		for i := range view.Goals {
			g := &view.Goals[i]
			if g.Goal.SourceID != current.Quiz.SourceID {
				continue
			}
			g.Goal.Title = currentMaterial
			if g.Preparing != nil {
				receipt := *g.Preparing
				receipt.Title = currentMaterial
				g.Preparing = &receipt
			}
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"map": view, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "map", Title: "Map", Active: "map", Map: view, Query: query})
}

func (s *server) conceptPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil {
		linked, err := s.linkedConcepts(r, current)
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		if linked[id] {
			s.gate(w, r, current)
			return
		}
	}
	view, err := s.store.ConceptPage(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	// A concept page links to its capture; the capture's text is served only
	// by the gated Source page.
	if view.Source != nil {
		view.Source = &store.Source{ID: view.Source.ID}
	}
	if current != nil {
		for i := range view.Goals {
			if view.Goals[i].SourceID == current.Quiz.SourceID {
				view.Goals[i].Title = currentMaterial
			}
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"concept": view, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "concept", Title: view.Concept.Name, Active: "map", Concept: view})
}

func conceptOperation(r *http.Request) (string, error) {
	op := r.PostForm.Get("operation_id")
	if op == "" || len(op) > 128 {
		return "", fmt.Errorf("%w: reload and try again", store.ErrInvalid)
	}
	return op, nil
}
func (s *server) practiceConcept(w http.ResponseWriter, r *http.Request) {
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.PracticeConcept(r.Context(), r.PathValue("id"), op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/", map[string]any{"practicing": r.PathValue("id")})
}
func (s *server) requestNote(w http.ResponseWriter, r *http.Request) {
	level := r.PostForm.Get("level")
	if level != "simpler" && level != "deeper" {
		s.fail(w, r, fmt.Errorf("%w: choose a note level", store.ErrInvalid), page{})
		return
	}
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.RequestNote(r.Context(), r.PathValue("id"), level, op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/concepts/"+r.PathValue("id"), map[string]any{"level": level})
}
func (s *server) requestQuestions(w http.ResponseWriter, r *http.Request) {
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.RequestQuestions(r.Context(), r.PathValue("id"), op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/concepts/"+r.PathValue("id"), map[string]any{"requested": true})
}
func (s *server) archiveConcept(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ArchiveConcept(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/map", map[string]any{"archived": true})
}
func (s *server) updateGoal(w http.ResponseWriter, r *http.Request) {
	action := r.PostForm.Get("action")
	switch action {
	case "pause", "resume", "focus", "unfocus":
	default:
		s.fail(w, r, fmt.Errorf("%w: choose an available action", store.ErrInvalid), page{})
		return
	}
	if err := s.store.UpdateGoal(r.Context(), r.PathValue("id"), action); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/map", map[string]any{"action": action})
}
func (s *server) fixQuiz(w http.ResponseWriter, r *http.Request) {
	instruction := strings.TrimSpace(r.PostForm.Get("instruction"))
	if instruction == "" || len(instruction) > 1000 {
		s.fail(w, r, fmt.Errorf("%w: describe the correction in 1–1000 bytes", store.ErrInvalid), page{})
		return
	}
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.RequestFix(r.Context(), r.PathValue("id"), instruction, op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/quizzes/"+r.PathValue("id")+"/edit", map[string]any{"requested": true})
}
