package web

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

// add prefills shared material but never chooses a mode: Topic and Link send
// material to web research, so only the learner may select them.
func (s *server) add(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	text := query.Get("url")
	if text == "" {
		text = query.Get("text")
	}
	if text == "" {
		text = query.Get("title")
	}
	if len(text) > store.MaxSourceBytes {
		text = ""
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken(), "max_bytes": store.MaxSourceBytes, "text": text, "mode": ""})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "add", Title: "Add", Active: "add", Text: text})
}

func (s *server) capture(w http.ResponseWriter, r *http.Request) {
	text, mode, op := r.PostForm.Get("text"), r.PostForm.Get("mode"), r.PostForm.Get("operation_id")
	p := page{View: "add", Title: "Add", Active: "add", Text: text, CaptureMode: mode, Operation: op}
	if op == "" || len(op) > 128 || !utf8.ValidString(text) || len(text) > store.MaxSourceBytes || (mode != "topic" && mode != "text" && mode != "link" && mode != "photo") || (mode != "photo" && strings.TrimSpace(text) == "") {
		s.fail(w, r, fmt.Errorf("%w: choose a kind and add up to 32 KiB of text, a link, or a photo", store.ErrInvalid), p)
		return
	}
	in := store.CaptureInput{Text: text, Mode: mode}
	var file multipart.File
	var header *multipart.FileHeader
	var err error
	if r.MultipartForm != nil {
		file, header, err = r.FormFile("photo")
	}
	if err == nil && file != nil {
		defer file.Close()
		if mode != "photo" || header.Size > store.MaxImageBytes {
			s.fail(w, r, fmt.Errorf("%w: choose Photo and an image no larger than 4 MiB", store.ErrInvalid), p)
			return
		}
		in.Image, err = io.ReadAll(io.LimitReader(file, store.MaxImageBytes+1))
		if err != nil || len(in.Image) == 0 || len(in.Image) > store.MaxImageBytes {
			s.fail(w, r, fmt.Errorf("%w: choose an image no larger than 4 MiB", store.ErrInvalid), p)
			return
		}
		// Detect the bytes, not the upload's untrusted Content-Type header.
		in.ImageMIME = http.DetectContentType(in.Image)
		if in.ImageMIME != "image/jpeg" && in.ImageMIME != "image/png" && in.ImageMIME != "image/webp" {
			s.fail(w, r, fmt.Errorf("%w: use JPEG, PNG, or WebP", store.ErrInvalid), p)
			return
		}
	} else if err != nil && err != http.ErrMissingFile {
		s.fail(w, r, fmt.Errorf("%w: photo could not be read", store.ErrInvalid), p)
		return
	}
	if mode == "photo" && len(in.Image) == 0 {
		s.fail(w, r, fmt.Errorf("%w: add a photo", store.ErrInvalid), p)
		return
	}
	source, err := s.store.Capture(r.Context(), in, op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/sources/"+source.ID, source)
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
	p := page{View: "source", Title: "Your material", Active: "map", Source: item, JobPending: item.Job != nil && jobPending(item.Job.Status), PollRemaining: 20}
	if r.URL.Query().Get("status") == "1" && isHTMX(r) {
		s.renderJob(w, r, p)
		return
	}
	s.render(w, r, http.StatusOK, p)
}

func (s *server) sourceImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil && current.Quiz.SourceID == id {
		s.gate(w, r, current)
		return
	}
	image, err := s.store.CaptureImage(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if image == nil {
		s.fail(w, r, store.ErrNotFound, page{})
		return
	}
	if image.MIME != "image/jpeg" && image.MIME != "image/png" && image.MIME != "image/webp" {
		s.fail(w, r, store.ErrInvalid, page{})
		return
	}
	w.Header().Set("Content-Type", image.MIME)
	w.Header().Set("Content-Disposition", "inline")
	w.Write(image.Bytes)
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
	fix, err := s.store.QuizFix(r.Context(), q.ID)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"quiz": q, "fix": fix, "csrf": r.Context().Value(csrfKey{})})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "edit", Title: "Edit question", Active: "map", Quiz: q, Fix: fix, FixOpen: r.URL.Query().Get("fix") == "1"})
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
	// Grading is internal. The form never carries a grading mode or rubric;
	// Store.EditQuiz keeps a generated rubric only for unchanged wording.
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
		s.fail(w, r, err, page{View: "edit", Title: "Edit question", Active: "map", Quiz: original})
		return
	}
	s.finish(w, r, "/sources/"+updated.SourceID, updated)
}

func nonemptyLines(raw string) []string {
	var lines []string
	for _, line := range formLines(raw) {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func formLines(raw string) []string {
	return strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
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
