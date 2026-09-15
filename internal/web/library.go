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
	if s.guardInspection(w, r, "library", query) {
		return
	}
	sources, err := s.store.Sources(r.Context(), query)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	goals, err := s.store.Goals(r.Context(), query)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	for i := range sources {
		sources[i].Text = "Original saved input"
		sources[i].Quizzes = nil
		sources[i].Materials = nil
		sources[i].Job = nil
		sources[i].Coverage = store.CoverageReport{}
	}
	materials := make([]store.Material, 0)
	seen := make(map[string]bool)
	for i := range goals {
		item := goals[i]
		for _, material := range item.Materials {
			if seen[material.ID] {
				continue
			}
			seen[material.ID] = true
			materials = append(materials, store.Material{ID: material.ID, SourceID: material.SourceID, Title: material.Title, Version: material.Version, Kind: material.Kind, Level: material.Level, MetadataOnly: true, Archived: material.Archived, Unmapped: material.Unmapped, FirstPresentedAt: material.FirstPresentedAt})
		}
		// Library scope permits titles, not hidden bodies, unit statements,
		// proposed answers, or a generation model's reusable context.
		goals[i] = store.Goal{ID: item.ID, SourceID: item.SourceID, Title: item.Title, Revision: item.Revision, MetadataOnly: true, UnitCount: item.UnitCount, MaterialCount: item.MaterialCount, MissingCoverageCount: item.MissingCoverageCount, Archived: item.Archived, CreatedAt: item.CreatedAt}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"sources": sources, "goals": goals, "materials": materials, "query": query})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "library", Title: "Library", Active: "library", Query: query, Sources: sources, Goals: goals, Materials: materials})
}

func (s *server) source(w http.ResponseWriter, r *http.Request) {
	if s.guardInspection(w, r, "source", r.PathValue("id")) {
		return
	}
	item, err := s.store.Source(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	item = publicSource(item)
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
			p.Error = "This input is not eligible for another retry, or its bounded retry limit has been reached. Open the saved material to inspect existing work or edit as new input. Nothing was duplicated."
		}
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/sources/"+item.ID, map[string]any{"source_id": item.ID, "status": item.Status})
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
	if s.guardInspection(w, r, "quiz", r.PathValue("id")) {
		return
	}
	q, err := s.store.Quiz(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
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
	s.finish(w, r, "/sources/"+updated.SourceID, map[string]any{"quiz_id": updated.ID, "source_id": updated.SourceID, "version": updated.Version})
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
	events, err := s.store.History(r.Context(), store.HistoryPageSize)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	interactions, err := s.store.Interactions(r.Context(), store.HistoryPageSize)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	// The grant covers the rendered mixed timeline, not another independent
	// hundred direct reviews hidden beside it in the JSON response.
	visibleReviews := make(map[string]bool, len(interactions))
	for _, interaction := range interactions {
		if interaction.ReviewID != "" {
			visibleReviews[interaction.ReviewID] = true
		}
	}
	visibleEvents := events[:0]
	for _, event := range events {
		if visibleReviews[event.ID] {
			visibleEvents = append(visibleEvents, event)
		}
	}
	events = visibleEvents
	if r.URL.Query().Get("inspect") == "1" && s.guardInspection(w, r, "history", "") {
		return
	}
	access, err := s.store.InspectionAccess(r.Context(), "history", "")
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	blocked := access.RequiresAssistance || !access.Allowed
	if blocked {
		for i := range interactions {
			item := interactions[i]
			item.Answer, item.Reason = "", ""
			item.Snapshot = store.Material{ID: item.MaterialID, Version: item.MaterialVersion}
			interactions[i] = item
		}
	}
	for i := range events {
		// A prior wording or pinned coverage can differ from today's links.
		// Keep every historical answer-bearing snapshot closed while a cold
		// target remains, rather than matching only today's quiz/source ID.
		if blocked {
			q := events[i].Quiz
			events[i].Quiz = store.Quiz{ID: q.ID, SourceID: q.SourceID, Kind: q.Kind, Version: q.Version, Prompt: "Historical wording closed until inspection"}
			events[i].Answer = ""
			events[i].Material = nil
			events[i].Coverage = nil
		}
	}
	byID := make(map[string]store.ReviewEvent, len(events))
	for _, event := range events {
		byID[event.ID] = event
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"history": events, "interactions": interactions, "answer_content_hidden": blocked, "limit": store.HistoryPageSize})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "history", Title: "History", Active: "history", ReviewByID: byID, Interactions: interactions, HistoryHidden: blocked, HistoryLimit: store.HistoryPageSize})
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
		access, err := s.store.InspectionAccess(r.Context(), "history", "")
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		inTimeline := false
		if event.Rating != 0 && access.Allowed && !access.RequiresAssistance {
			interactions, err := s.store.Interactions(r.Context(), store.HistoryPageSize)
			if err != nil {
				s.fail(w, r, err, page{})
				return
			}
			for _, interaction := range interactions {
				if interaction.ReviewID == event.ID {
					inTimeline = true
					break
				}
			}
		}
		if !inTimeline {
			q := event.Quiz
			event.Quiz = store.Quiz{ID: q.ID, SourceID: q.SourceID, Kind: q.Kind, Version: q.Version}
			event.Answer = ""
			event.Material = nil
			event.Coverage = nil
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
