package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken(), "max_bytes": 32768})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "add", Title: "Add something", Active: "add"})
}

func (s *server) capture(w http.ResponseWriter, r *http.Request) {
	text, op := r.PostForm.Get("text"), r.PostForm.Get("operation_id")
	p := page{View: "add", Title: "Add something", Active: "add", Text: text, Operation: op}
	if len(text) > 32768 || !utf8.ValidString(text) || strings.TrimSpace(text) == "" || op == "" || len(op) > 128 {
		p.Error = "Add a word, goal, or passage of at most 32 KiB. Split longer material into self-contained sections. Nothing has been truncated or saved."
		if wantsJSON(r) {
			jsonResponse(w, http.StatusUnprocessableEntity, map[string]any{"error": p.Error, "operation_id": op})
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	source, err := s.store.Capture(r.Context(), text, op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/sources/"+source.ID, source)
}

func (s *server) library(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 1024 {
		s.fail(w, r, store.ErrInvalid, page{View: "library", Title: "Library", Active: "library", Query: query})
		return
	}
	sources, err := s.store.Sources(r.Context(), query)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	for i := range sources {
		// Search can find the current material, but its snippet must not reveal
		// the answer. Opening it requires explicit assistance first.
		if current != nil && sources[i].ID == current.Quiz.SourceID {
			sources[i].Text = "Material in your current review"
			sources[i].Quizzes = nil
			sources[i].Job = nil
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"sources": sources, "query": query})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "library", Title: "Library", Active: "library", Query: query, Sources: sources})
}

func (s *server) source(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Source(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil && current.Quiz.SourceID == item.ID {
		s.gate(w, r, current)
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"source": item, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	p := page{View: "source", Title: "Your material", Active: "library", Source: item, JobPending: item.Job != nil && jobPending(item.Job.Status), PollRemaining: 20}
	if r.URL.Query().Get("status") == "1" && isHTMX(r) {
		s.renderJob(w, r, p)
		return
	}
	s.render(w, r, http.StatusOK, p)
}

func (s *server) retrySource(w http.ResponseWriter, r *http.Request) {
	op := r.PostForm.Get("operation_id")
	if op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, page{})
		return
	}
	item, err := s.store.RetrySource(r.Context(), r.PathValue("id"), op)
	if err != nil {
		p := page{Operation: op}
		if status, _ := publicError(err); status == http.StatusConflict {
			p.Error = "This input is not eligible for another retry, or its bounded retry limit has been reached. Open the saved material to inspect existing questions or edit as new input. Nothing was duplicated."
		}
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/sources/"+item.ID, item)
}

func (s *server) archiveSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.ArchiveSource(r.Context(), id); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/sources/"+id, map[string]any{"archived": true, "source_id": id})
}

func (s *server) editQuiz(w http.ResponseWriter, r *http.Request) {
	q, err := s.store.Quiz(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil && current.Quiz.ID == q.ID {
		s.gate(w, r, current)
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"quiz": q, "csrf": r.Context().Value(csrfKey{})})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "edit", Title: "Edit question", Active: "library", Quiz: q})
}

func (s *server) saveQuiz(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	original, err := s.store.Quiz(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	version, err := strconv.Atoi(r.PostForm.Get("version"))
	if err != nil {
		s.fail(w, r, store.ErrInvalid, page{})
		return
	}
	q := store.GeneratedQuiz{
		Kind: r.PostForm.Get("kind"), Prompt: r.PostForm.Get("prompt"), Answer: r.PostForm.Get("answer"),
		Explanation: r.PostForm.Get("explanation"), Evidence: r.PostForm.Get("evidence"), Basis: original.Basis,
		Choices: nonemptyLines(r.PostForm.Get("choices")), Variants: nonemptyLines(r.PostForm.Get("variants")),
	}
	if q.Kind == "recall" {
		q.Choices = nil
	}
	if q.Kind == "choice" {
		q.Variants = nil
	}
	// Attribution is inherited from the saved source, not a browser-selectable
	// claim. A source-backed edit must still cite an exact saved quotation.
	updated, err := s.store.EditQuiz(r.Context(), id, version, q)
	if err != nil {
		original.Kind, original.Prompt, original.Answer = q.Kind, q.Prompt, q.Answer
		original.Explanation, original.Evidence = q.Explanation, q.Evidence
		original.Choices, original.Variants, original.Version = q.Choices, q.Variants, version
		s.fail(w, r, err, page{View: "edit", Title: "Edit question", Active: "library", Quiz: original})
		return
	}
	s.finish(w, r, "/sources/"+updated.SourceID, updated)
}

func nonemptyLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (s *server) archiveQuiz(w http.ResponseWriter, r *http.Request) {
	q, err := s.store.Quiz(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if err := s.store.ArchiveQuiz(r.Context(), q.ID); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	where := "/sources/" + q.SourceID
	if r.PostForm.Get("return_to") == "/" {
		where = "/"
	}
	s.finish(w, r, where, map[string]any{"archived": true, "quiz_id": q.ID})
}

func (s *server) history(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.History(r.Context(), 100)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil {
		for i := range events {
			if events[i].Quiz.ID == current.Quiz.ID {
				events[i].Quiz = withoutAnswer(events[i].Quiz)
				events[i].Answer = ""
			}
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"history": events, "limit": 100})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "history", Title: "History", Active: "history", History: events})
}

func (s *server) disputePage(w http.ResponseWriter, r *http.Request) {
	// The link is issued for the recent-history window. Older records remain
	// exportable; no new event lookup/storage contract is invented here.
	events, err := s.store.History(r.Context(), 1000)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	for _, event := range events {
		if event.ID != r.PathValue("id") {
			continue
		}
		current, err := s.coldReview(r)
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		if current != nil && current.Quiz.ID == event.Quiz.ID {
			event.Quiz = withoutAnswer(event.Quiz)
			event.Answer = ""
		}
		if wantsJSON(r) {
			jsonResponse(w, http.StatusOK, map[string]any{"review": event, "csrf": r.Context().Value(csrfKey{})})
			return
		}
		s.render(w, r, http.StatusOK, page{View: "dispute", Title: "Flag this review", Active: "history", ReviewID: event.ID, Quiz: event.Quiz})
		return
	}
	s.fail(w, r, store.ErrNotFound, page{})
}

func (s *server) dispute(w http.ResponseWriter, r *http.Request) {
	id, note := r.PathValue("id"), strings.TrimSpace(r.PostForm.Get("note"))
	reset := r.PostForm.Get("reset") == "yes"
	if note == "" || len(note) > 2000 {
		s.fail(w, r, fmt.Errorf("%w: write a problem note of 1–2000 bytes", store.ErrInvalid), page{View: "dispute", Title: "Flag this review", Active: "history", ReviewID: id, Note: note, Reset: reset})
		return
	}
	if err := s.store.Dispute(r.Context(), id, note, reset); err != nil {
		s.fail(w, r, err, page{View: "dispute", Title: "Flag this review", Active: "history", ReviewID: id, Note: note, Reset: reset})
		return
	}
	s.finish(w, r, "/history", map[string]any{"disputed": true, "review_id": id, "schedule_reset": reset})
}
