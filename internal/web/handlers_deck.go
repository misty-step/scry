package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"scry/internal/app"
	"scry/internal/deck"
)

// importLimit is the largest request body the import accepts: the 20 MiB
// file plus room for the form's other fields and multipart framing.
const importLimit = deck.MaxBytes + 1<<20

// importGuard refuses an upload that is plainly too large before anything
// reads it (the CSRF check parses the form), by sending the learner back
// to Create with the refusal in words. A redirect has no side effects, so
// it needs no CSRF token. Bodies of unknown length are capped as they are
// read.
func importGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > importLimit {
			http.Redirect(w, r, "/learn?import=toolarge#import", http.StatusSeeOther)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, importLimit)
		next.ServeHTTP(w, r)
	})
}

var importRefusals = map[string]string{
	"toolarge": "That file is larger than 20 MiB. Split it and import the parts.",
}

// learnImport is Create's "Import a deck" (S106-3, DESIGN.md Create).
func (s *Server) learnImport(w http.ResponseWriter, r *http.Request, c *rc) error {
	ideaTitle := r.FormValue("idea")
	// A refusal is an expected outcome shown in words on Create, so it is
	// a normal page (a 4xx document would log an error in the console).
	refuse := func(msg string) error {
		return s.render(w, r, c, http.StatusOK, "learn.html", map[string]any{
			"Title": "Create", "Nav": "learn", "Examples": learnExamples, "ImportError": msg, "ImportIdea": ideaTitle,
		})
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return refuse(importRefusals["toolarge"])
		}
		return refuse("Choose a file to import: an Anki .apkg, a .csv or .tsv, or a .md file.")
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(f, deck.MaxBytes+1)); err != nil {
		return refuse("Scry couldn't read that file. Try again.")
	}
	res, err := s.App.Import(r.Context(), c.user, hdr.Filename, buf.Bytes(), ideaTitle)
	var fe *app.FieldError
	if errors.As(err, &fe) {
		return refuse(fe.Msg)
	}
	if err != nil {
		return err
	}
	if res.DraftID != 0 {
		return back(w, r, "/learn/"+strconv.FormatInt(res.DraftID, 10)+"?from=import")
	}
	return back(w, r, "/library/ideas/"+strconv.FormatInt(res.IdeaID, 10)+"?imported="+strconv.Itoa(res.Cards))
}

// settingsExport hands the learner everything Scry holds for them, as
// Markdown or CSV (every card, by idea: these import back intact) or JSON
// (every row Scry holds). It works whatever the state of their plan: the
// data is theirs.
func (s *Server) settingsExport(w http.ResponseWriter, r *http.Request, c *rc) error {
	switch r.URL.Query().Get("format") {
	case "md", "markdown":
		cards, err := s.App.DeckExport(r.Context(), c.user)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := deck.WriteMarkdown(&buf, cards); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="scry-export.md"`)
		_, err = w.Write(buf.Bytes())
		return err
	case "csv":
		cards, err := s.App.DeckExport(r.Context(), c.user)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := deck.WriteCSV(&buf, cards); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="scry-export.csv"`)
		_, err = w.Write(buf.Bytes())
		return err
	}
	exp, err := c.us.Export(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="scry-export.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(exp)
}
