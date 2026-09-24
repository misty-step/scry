package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func (s *server) renderJob(w http.ResponseWriter, r *http.Request, p page) {
	remaining, err := strconv.Atoi(r.URL.Query().Get("remaining"))
	if err != nil || remaining < 0 {
		remaining = 0
	}
	if remaining > 20 {
		remaining = 20
	}
	p.PollRemaining = remaining
	if remaining > 0 {
		p.PollRemaining--
	}
	p.CSRF, _ = r.Context().Value(csrfKey{}).(string)
	p.Operation = randomToken()
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, "job", p); err != nil {
		http.Error(w, "Status could not be displayed. Refresh this page to check saved work.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Summary(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	preferences, err := s.store.Preferences(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	stale := summary.LastBackup == nil || !summary.LastBackup.Remote || summary.LastBackup.Error != "" || summary.LastBackup.CreatedAt < time.Now().Add(-24*time.Hour).UnixMilli()
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"summary": summary, "preferences": preferences, "backup_needs_attention": stale, "csrf": r.Context().Value(csrfKey{})})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "settings", Title: "Settings", Active: "settings", Summary: summary, Preferences: preferences, BackupStale: stale})
}

func (s *server) setPace(w http.ResponseWriter, r *http.Request) {
	pace := r.PostForm.Get("pace")
	if pace != "light" && pace != "steady" && pace != "intense" {
		s.fail(w, r, fmt.Errorf("%w: choose a pace", store.ErrInvalid), page{})
		return
	}
	if err := s.store.SetPace(r.Context(), pace); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/settings", map[string]any{"pace": pace})
}

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.Export(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="scry-export-`+time.Now().UTC().Format("2006-01-02")+`.json"`)
	w.Header().Set("X-Download-Options", "noopen")
	w.Write(data)
}
