// Package web serves the private, progressively enhanced Scry application.
package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/store"
)

// Config grants authority only to the explicitly configured ingress peers.
// TrustProxy alone never trusts an identity header. Development is loopback-only.
type Config struct {
	Mode            string
	OwnerID         string
	Secret          string
	BaseURL         string
	RedirectHosts   []string
	TrustProxy      bool
	TrustedProxyIPs []string
}

//go:embed templates/*.html assets/*
var files embed.FS

type server struct {
	store     *store.Store
	cfg       Config
	origin    *url.URL
	peers     map[netip.Addr]bool
	templates *template.Template
}

type page struct {
	Title          string
	View           string
	Active         string
	CSRF           string
	Operation      string
	Error          string
	Notice         string
	Query          string
	Text           string
	Review         store.ReviewState
	Sources        []store.Source
	Source         store.Source
	Quiz           store.Quiz
	History        []store.ReviewEvent
	Summary        store.Summary
	Gate           *store.Presentation
	ReturnTo       string
	ReviewID       string
	Note           string
	Reset          bool
	JobPending     bool
	BackupStale    bool
	PollRemaining  int
	RecordedModels []string
	Status         int
}

// New validates the private boundary before exposing any application route.
func New(s *store.Store, cfg Config) (http.Handler, error) {
	if s == nil {
		return nil, errors.New("web: a store is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("web: BaseURL must be an absolute origin without credentials, path, query, or fragment")
	}
	u.Path = ""
	if strings.ContainsAny(cfg.OwnerID, "\r\n\t ,") {
		return nil, errors.New("web: OwnerID must be one exact identity value")
	}
	peers := make(map[netip.Addr]bool, len(cfg.TrustedProxyIPs))
	for _, raw := range cfg.TrustedProxyIPs {
		ip, err := netip.ParseAddr(raw)
		if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return nil, fmt.Errorf("web: invalid exact trusted proxy IP %q", raw)
		}
		peers[ip.Unmap()] = true
	}
	switch cfg.Mode {
	case "production":
		if cfg.OwnerID == "" || len(cfg.Secret) < 32 || u.Scheme != "https" || !cfg.TrustProxy || len(peers) == 0 {
			return nil, errors.New("web: production requires OwnerID, a secret of at least 32 bytes, HTTPS BaseURL, TrustProxy and exact TrustedProxyIPs")
		}
	case "development":
		if u.Scheme != "http" || !localHostname(u.Hostname()) {
			return nil, errors.New("web: development requires an explicit HTTP localhost BaseURL")
		}
		if cfg.OwnerID == "" {
			cfg.OwnerID = "local-development"
		}
		if cfg.Secret == "" {
			cfg.Secret = randomToken()
		}
		if len(cfg.Secret) < 32 {
			return nil, errors.New("web: development secret must contain at least 32 bytes when supplied")
		}
	default:
		return nil, errors.New("web: Mode must explicitly be production or development")
	}
	var redirectHosts []string
	for _, host := range cfg.RedirectHosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		alias, err := url.Parse("https://" + host)
		if err != nil || alias.Host != host || alias.Hostname() == "" || alias.User != nil || strings.EqualFold(host, u.Host) {
			return nil, fmt.Errorf("web: invalid alternate host %q", host)
		}
		redirectHosts = append(redirectHosts, host)
	}
	funcs := template.FuncMap{
		"timeText":   timeText,
		"timeISO":    timeISO,
		"money":      money,
		"excerpt":    excerpt,
		"joinLines":  func(v []string) string { return strings.Join(v, "\n") },
		"outcome":    outcomeText,
		"kind":       kindText,
		"jobLabel":   jobLabel,
		"jobPending": jobPending,
		"retryable":  retryable,
	}
	t, err := template.New("scry").Funcs(funcs).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web templates: %w", err)
	}
	app := &server{store: s, cfg: cfg, origin: u, peers: peers, templates: t}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", app.review)
	mux.HandleFunc("GET /review/preview", app.preview)
	mux.HandleFunc("POST /review/answer", app.answer)
	mux.HandleFunc("POST /review/reveal", app.reveal)
	mux.HandleFunc("POST /review/next", app.next)
	mux.HandleFunc("GET /add", app.add)
	mux.HandleFunc("POST /add", app.capture)
	mux.HandleFunc("GET /library", app.library)
	mux.HandleFunc("GET /sources/{id}", app.source)
	mux.HandleFunc("POST /sources/{id}/retry", app.retrySource)
	mux.HandleFunc("POST /sources/{id}/archive", app.archiveSource)
	mux.HandleFunc("GET /quizzes/{id}/edit", app.editQuiz)
	mux.HandleFunc("POST /quizzes/{id}/edit", app.saveQuiz)
	mux.HandleFunc("POST /quizzes/{id}/archive", app.archiveQuiz)
	mux.HandleFunc("GET /history", app.history)
	mux.HandleFunc("GET /reviews/{id}/dispute", app.disputePage)
	mux.HandleFunc("POST /reviews/{id}/dispute", app.dispute)
	mux.HandleFunc("GET /settings", app.settings)
	mux.HandleFunc("GET /export", app.export)
	mux.HandleFunc("GET /session", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]bool{"authenticated": true})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		app.render(w, r, http.StatusNotFound, page{View: "error", Title: "Page not found", Error: "That page is not available. Return to review or find your material in the library."})
	})
	private := app.authenticate(mux)
	assets, err := fs.Sub(files, "assets")
	if err != nil {
		return nil, err
	}
	static := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w, cfg.Mode == "production")
		for _, host := range redirectHosts {
			if !strings.EqualFold(r.Host, host) {
				continue
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "This address has changed. Reload Scry before submitting; nothing was saved.", http.StatusConflict)
				return
			}
			location := *app.origin
			location.Path, location.RawPath = r.URL.Path, r.URL.RawPath
			location.RawQuery, location.ForceQuery = r.URL.RawQuery, r.URL.ForceQuery
			http.Redirect(w, r, location.String(), http.StatusPermanentRedirect)
			return
		}
		if r.URL.Path == "/healthz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("ok\n"))
			return
		}
		if r.URL.Path == "/readyz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if _, err := s.Summary(ctx); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("ready\n"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			// Embedded public code only: no directory listing or private content.
			name := strings.TrimPrefix(r.URL.Path, "/assets/")
			switch name {
			case "app.css", "app.js", "htmx-2.0.10.min.js", "htmx-LICENSE":
				static.ServeHTTP(w, r)
			default:
				http.NotFound(w, r)
			}
			return
		}
		private.ServeHTTP(w, r)
	}), nil
}

func (s *server) render(w http.ResponseWriter, r *http.Request, status int, p page) {
	p.CSRF, _ = r.Context().Value(csrfKey{}).(string)
	p.Status = status
	if p.Operation == "" {
		p.Operation = randomToken()
	}
	if p.Title == "" {
		p.Title = "Scry"
	}
	var buf bytes.Buffer
	name := "document"
	if isHTMX(r) {
		name = "main"
	}
	if err := s.templates.ExecuteTemplate(&buf, name, p); err != nil {
		slog.Error("web rendering failed", "view", p.View)
		http.Error(w, "The page could not be displayed. Reload to recover your saved state.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "HX-Request, Accept, Cookie")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error, p page) {
	status, message := publicError(err)
	if p.Error != "" && status < 500 {
		message = p.Error
	}
	if status >= 500 {
		// Errors can contain source/provider data; do not log their raw text.
		slog.Error("web operation failed", "method", r.Method, "status", status)
	}
	if wantsJSON(r) {
		jsonResponse(w, status, map[string]any{"error": message, "status": status, "operation_id": p.Operation})
		return
	}
	p.Error = message
	if p.View == "" {
		p.View = "error"
		p.Title = "Something needs attention"
	}
	s.render(w, r, status, p)
}

func (s *server) finish(w http.ResponseWriter, r *http.Request, location string, value any) {
	if wantsJSON(r) {
		w.Header().Set("Location", location)
		jsonResponse(w, http.StatusOK, value)
		return
	}
	if isHTMX(r) {
		destination, _ := json.Marshal(map[string]string{"path": location, "target": "#main", "select": "#main", "swap": "outerHTML"})
		w.Header().Set("HX-Location", string(destination))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func publicError(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, "This action conflicts with the current saved state. Your recorded work is safe. Reload before making another change."
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "This material is no longer available. Return to review or the library."
	case errors.Is(err, store.ErrBudget):
		return http.StatusTooManyRequests, "Generation is paused at the spending limit. Your input is saved; review existing material or retry when the allowance is available."
	case errors.Is(err, store.ErrInvalid):
		return http.StatusUnprocessableEntity, "Check your input: " + strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": ")
	default:
		return http.StatusServiceUnavailable, "The request could not be confirmed. Keep this page open and retry the same request, or reload to recover what was saved."
	}
}

func (s *server) parseForm(w http.ResponseWriter, r *http.Request) bool {
	// URL encoding can triple the byte count of a valid 32 KiB source.
	r.Body = http.MaxBytesReader(w, r.Body, 160<<10)
	if err := r.ParseForm(); err != nil {
		status := http.StatusBadRequest
		message := "This form could not be read. Nothing was changed by this request. Reload the form before trying again."
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			status = http.StatusRequestEntityTooLarge
			message = "Use at most 32 KiB of input. Split a long passage into smaller self-contained sections; nothing has been truncated or saved."
		}
		if wantsJSON(r) {
			jsonResponse(w, status, map[string]string{"error": message})
		} else {
			s.render(w, r, status, page{View: "error", Title: "Check this form", Error: message})
		}
		return false
	}
	return true
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Vary", "Accept, HX-Request, Cookie")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("cryptographic randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func timeText(ms int64) string {
	if ms <= 0 {
		return "Not scheduled"
	}
	return time.UnixMilli(ms).UTC().Format("2 Jan 2006, 15:04 UTC")
}

func timeISO(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func money(micros int64) string {
	if micros > 0 && micros < 1000 {
		return "less than $0.001"
	}
	return "$" + strconv.FormatFloat(float64(micros)/1e6, 'f', 3, 64)
}

func excerpt(text string) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) <= 140 {
		return string(r)
	}
	return string(r[:140]) + "…"
}

func kindText(kind string) string {
	if kind == "choice" {
		return "Recognition"
	}
	return "Cued recall"
}

func outcomeText(p string) string {
	switch p {
	case "correct":
		return "Correct"
	case "close":
		return "Not quite matched"
	case "wrong":
		return "Not yet"
	case "revealed":
		return "Answer revealed"
	case "ungraded":
		return "Not graded"
	default:
		return "Unanswered"
	}
}

func jobLabel(status string) string {
	switch status {
	case "queued":
		return "Waiting to generate"
	case "running":
		return "Generating questions"
	case "retry":
		return "Waiting to retry"
	case "ready", "complete":
		return "Questions ready"
	case "partial":
		return "Partially ready"
	case "failed":
		return "Generation stopped"
	case "paused":
		return "Generation paused"
	case "canceled":
		return "Generation canceled"
	case "archived":
		return "Archived"
	default:
		return "Saved; inspect generation below"
	}
}

func jobPending(status string) bool {
	switch status {
	case "queued", "running", "retry":
		return true
	default:
		return false
	}
}

func retryable(status string) bool {
	switch status {
	case "failed", "paused", "canceled":
		return true
	default:
		return false
	}
}
