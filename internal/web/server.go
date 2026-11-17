// Package web is Scry's HTTP surface: server-rendered pages, a small JSON
// API for the learner's agents, the processor's webhook and /healthz.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"scry/internal/app"
	"scry/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server serves Scry.
type Server struct {
	App      *app.App
	Log      *slog.Logger
	DemoHint bool // show the demo account on the sign-in page (seeded test mode)
	Secure   bool // set cookies Secure (behind TLS)

	pages   map[string]*template.Template
	handler http.Handler

	mu       sync.Mutex
	attempts map[string][]time.Time
}

// rc is what a handler knows about the request's learner.
type rc struct {
	user     store.User
	sess     store.Session
	token    string
	signedIn bool
	sub      store.Subscription
	us       *store.UserStore
}

type ctxKey struct{}

type handler func(w http.ResponseWriter, r *http.Request, c *rc) error

// New builds the server and its routes.
func New(a *app.App, log *slog.Logger) *Server {
	s := &Server{App: a, Log: log, attempts: map[string][]time.Time{}}
	s.pages = map[string]*template.Template{}
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		panic(err)
	}
	for _, name := range names {
		base := strings.TrimPrefix(name, "templates/")
		if base == "layout.html" || base == "partials.html" {
			continue
		}
		s.pages[base] = template.Must(template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/partials.html", name))
	}

	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(static))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		fileServer.ServeHTTP(w, r)
	})))
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nAllow: /$\nAllow: /signup\nDisallow: /\n")
	})
	// The wordmark has no glyph for now, so there is no icon to serve.
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /webhooks/stripe", s.webhook)
	mux.HandleFunc("POST /api/v1/suggestions", s.apiSuggest)
	mux.HandleFunc("POST /api/v1/items", s.apiAdd)
	mux.HandleFunc("GET /api/v1/ideas", s.apiIdeas)

	mux.Handle("GET /{$}", s.public(s.home))
	mux.Handle("GET /signup", s.public(s.signupForm))
	mux.Handle("POST /signup", s.public(s.signup))
	mux.Handle("GET /signin", s.public(s.signinForm))
	mux.Handle("POST /signin", s.public(s.signin))
	mux.Handle("POST /signout", s.authed(s.signout))
	mux.Handle("GET /signup/card", s.authed(s.cardForm))
	mux.Handle("POST /signup/card", s.authed(s.cardSubmit))

	mux.Handle("GET /shelf", s.member(s.shelf))
	mux.Handle("POST /shelf/add", s.member(s.shelfAdd))

	mux.Handle("GET /study", s.member(s.study))
	mux.Handle("POST /study/answer", s.member(s.studyAnswer))
	mux.Handle("POST /study/next", s.member(s.studyNext))
	mux.Handle("GET /study/done", s.member(s.studyDone))
	mux.Handle("POST /study/speech", s.member(s.studySpeech))

	mux.Handle("GET /library", s.member(s.library))
	mux.Handle("GET /library/ideas/{id}", s.member(s.library))
	mux.Handle("POST /library/ideas/{id}/remove", s.member(s.ideaRemove))
	mux.Handle("POST /library/ideas/{id}/items", s.member(s.itemAdd))
	mux.Handle("GET /library/items/{id}", s.member(s.itemPage))
	mux.Handle("POST /library/items/{id}", s.member(s.itemSave))
	mux.Handle("POST /library/items/{id}/remove", s.member(s.itemRemove))
	mux.Handle("GET /library/texts/new", s.member(s.textForm))
	mux.Handle("POST /library/texts", s.member(s.textSave))

	mux.Handle("GET /inbox", s.member(s.inbox))
	mux.Handle("POST /inbox/approve-all", s.member(s.inboxApproveAll))
	mux.Handle("POST /suggestions/{id}/approve", s.member(s.suggestionApprove))
	mux.Handle("POST /suggestions/{id}/dismiss", s.member(s.suggestionDismiss))

	mux.Handle("GET /learn", s.member(s.learn))
	mux.Handle("POST /learn", s.member(s.learnAsk))
	mux.Handle("GET /learn/{id}", s.member(s.draft))
	mux.Handle("POST /learn/{id}/approve-all", s.member(s.draftApproveAll))

	mux.Handle("GET /billing", s.authed(s.billing))
	mux.Handle("POST /billing/topup", s.authed(s.billingTopup))
	mux.Handle("POST /billing/cancel", s.authed(s.billingCancel))
	mux.Handle("POST /billing/resume", s.authed(s.billingResume))
	mux.Handle("GET /billing/card", s.authed(s.billingCardForm))
	mux.Handle("POST /billing/card", s.authed(s.billingCard))

	mux.Handle("GET /settings", s.authed(s.settings))
	mux.Handle("POST /settings/theme", s.authed(s.settingsTheme))
	mux.Handle("POST /settings/about", s.authed(s.settingsAbout))
	mux.Handle("POST /settings/tokens", s.authed(s.settingsToken))
	mux.Handle("POST /settings/tokens/{id}/revoke", s.authed(s.settingsRevoke))
	mux.Handle("POST /settings/restore/{id}", s.authed(s.settingsRestore))
	mux.Handle("GET /settings/export", s.authed(s.settingsExport))

	mux.Handle("/", s.public(func(w http.ResponseWriter, r *http.Request, c *rc) error { return store.ErrNotFound }))

	s.handler = s.observe(s.secure(mux))
	return s
}

// ServeHTTP makes Server an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

type logInfo struct {
	user int64
	id   string
}

type logKey struct{}

// observe logs every request as one structured line, turns panics into a
// 500 and an alert, and alerts on any other 5xx.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		raw := make([]byte, 6)
		_, _ = rand.Read(raw)
		info := &logInfo{id: hex.EncodeToString(raw)}
		sw := &statusWriter{ResponseWriter: w}
		sw.Header().Set("X-Request-Id", info.id)
		r = r.WithContext(context.WithValue(r.Context(), logKey{}, info))
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.ErrorContext(r.Context(), "panic", "request_id", info.id, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				s.App.Alert.Notify(r.Context(), "A request panicked", fmt.Sprintf("%s %s: %v", r.Method, r.URL.Path, rec))
				if sw.status == 0 {
					s.errorPage(sw, r, http.StatusInternalServerError)
				}
			}
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			}
			if r.URL.Path == "/healthz" && sw.status < 500 {
				level = slog.LevelDebug
			}
			s.Log.Log(r.Context(), level, "request", "request_id", info.id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "duration_ms", time.Since(start).Milliseconds(), "user", info.user)
		}()
		next.ServeHTTP(sw, r)
	})
}

// secure sets the headers that keep the page to its own origin.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		script, frame, connect := "'self'", "'none'", "'self'"
		if s.App.Pay.PublishableKey() != "" {
			script, frame, connect = "'self' https://js.stripe.com", "https://js.stripe.com", "'self' https://api.stripe.com"
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; font-src 'self'; script-src "+script+
			"; frame-src "+frame+"; connect-src "+connect+"; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "microphone=(self), camera=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

const cookieName = "scry_session"

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	c := &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: s.Secure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: int(ttl.Seconds())}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// load resolves the session cookie and, for anything that changes state,
// checks the CSRF token.
func (s *Server) load(w http.ResponseWriter, r *http.Request) (*rc, bool) {
	c := &rc{}
	if ck, err := r.Cookie(cookieName); err == nil {
		if u, se, err := s.App.SessionUser(r.Context(), ck.Value); err == nil {
			c.user, c.sess, c.token, c.signedIn = u, se, ck.Value, true
			c.us = s.App.Store.For(u.ID)
			if s.App.KeepSession(r.Context(), ck.Value, se) {
				s.setCookie(w, r, ck.Value, app.SessionTTL)
			}
			if info, ok := r.Context().Value(logKey{}).(*logInfo); ok {
				info.user = u.ID
			}
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && c.signedIn {
		got := r.Header.Get("X-CSRF-Token")
		if got == "" {
			got = r.FormValue("csrf")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(c.sess.CSRF)) != 1 {
			s.errorPage(w, r, http.StatusForbidden)
			return nil, false
		}
	}
	return c, true
}

func (s *Server) run(w http.ResponseWriter, r *http.Request, c *rc, h handler) {
	err := h(w, r, c)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound):
		s.errorPage(w, r, http.StatusNotFound)
	case errors.Is(err, app.ErrNoPlan):
		http.Redirect(w, r, "/signup/card", http.StatusSeeOther)
	case errors.Is(err, app.ErrPlanEnded):
		http.Redirect(w, r, "/billing", http.StatusSeeOther)
	default:
		s.Log.ErrorContext(r.Context(), "handler failed", "err", err, "path", r.URL.Path)
		s.App.Alert.Notify(r.Context(), "A request failed", r.Method+" "+r.URL.Path+": "+err.Error())
		s.errorPage(w, r, http.StatusInternalServerError)
	}
}

// public serves anyone.
func (s *Server) public(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, ok := s.load(w, r); ok {
			s.run(w, r, c, h)
		}
	})
}

// authed serves signed-in learners, whatever the state of their plan.
func (s *Server) authed(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := s.load(w, r)
		if !ok {
			return
		}
		if !c.signedIn {
			http.Redirect(w, r, "/signin?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		s.run(w, r, c, h)
	})
}

// member serves learners with a live plan. There is no free tier: with
// no plan the learner is sent to the card step, and with an ended plan
// to Billing.
func (s *Server) member(h handler) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, c *rc) error {
		sub, err := s.App.Plan(r.Context(), c.user.ID)
		if err != nil {
			return err
		}
		c.sub = sub
		return h(w, r, c)
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, c *rc, status int, page string, data map[string]any) error {
	t, ok := s.pages[page]
	if !ok {
		return fmt.Errorf("no template %q", page)
	}
	if data == nil {
		data = map[string]any{}
	}
	data["TestMode"] = s.App.Cfg.TestMode
	data["Path"] = r.URL.Path
	data["Now"] = s.App.Now()
	data["Theme"] = "system"
	if c != nil && c.signedIn {
		data["User"] = c.user
		data["CSRF"] = c.sess.CSRF
		data["Theme"] = c.user.Theme
		if n, err := c.us.InboxCount(r.Context()); err == nil {
			data["InboxCount"] = n
		}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = ""
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, err := w.Write([]byte(buf.String()))
	return err
}

func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
		return
	}
	head, line := "Something broke.", "Scry hit an error and has told its keepers. Try again in a moment."
	switch status {
	case http.StatusNotFound:
		head, line = "Not here.", "That page doesn't exist, or it isn't yours to see."
	case http.StatusForbidden:
		head, line = "That didn't go through.", "The form had expired. Go back, reload the page and try again."
	}
	data := map[string]any{"Title": head, "Head": head, "Line": line, "TestMode": s.App.Cfg.TestMode, "Theme": "system", "Path": r.URL.Path, "Bare": true}
	var buf strings.Builder
	if err := s.pages["error.html"].Execute(&buf, data); err != nil {
		http.Error(w, head, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(buf.String()))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.App.Health(r.Context()); err != nil {
		s.Log.ErrorContext(r.Context(), "health check failed", "err", err)
		s.App.Alert.Notify(r.Context(), "Scry is unhealthy", "The database cannot be written: "+err.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unhealthy", "database": "failing"})
		return
	}
	mode := "live"
	if s.App.Cfg.TestMode {
		mode = "test"
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "database": "ok", "mode": mode})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, store.ErrNotFound
	}
	return id, nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tooMany limits password guesses: ten attempts per address and email in
// ten minutes.
func (s *Server) tooMany(key string) bool {
	now := s.App.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []time.Time
	for _, t := range s.attempts[key] {
		if now.Sub(t) < 10*time.Minute {
			keep = append(keep, t)
		}
	}
	keep = append(keep, now)
	s.attempts[key] = keep
	if len(s.attempts) > 10000 {
		s.attempts = map[string][]time.Time{}
	}
	return len(keep) > 10
}

func back(w http.ResponseWriter, r *http.Request, to string) error {
	http.Redirect(w, r, to, http.StatusSeeOther)
	return nil
}
