package web

import (
	"bytes"
	"net/http"
	"sort"
	"strconv"
	"time"
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
	sources, err := s.store.Sources(r.Context(), "")
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	models := map[string]bool{}
	for _, source := range sources {
		if source.Job != nil && source.Job.Model != "" {
			models[source.Job.Model] = true
		}
	}
	var recordedModels []string
	for model := range models {
		recordedModels = append(recordedModels, model)
	}
	sort.Strings(recordedModels)
	stale := summary.LastBackup != nil && summary.LastBackup.CreatedAt < time.Now().Add(-24*time.Hour).UnixMilli()
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"summary": summary, "recorded_models": recordedModels, "backup_older_than_24h": stale})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "settings", Title: "Settings", Active: "settings", Summary: summary, RecordedModels: recordedModels, BackupStale: stale})
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
