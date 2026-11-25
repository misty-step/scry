package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"scry/internal/app"
	"scry/internal/store"
)

func (s *Server) settingsData(r *http.Request, c *rc) (map[string]any, error) {
	tokens, err := c.us.Tokens(r.Context())
	if err != nil {
		return nil, err
	}
	removed, err := recentlyRemoved(r.Context(), c)
	if err != nil {
		return nil, err
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return map[string]any{
		"Title": "Settings", "Nav": "settings", "Tokens": tokens, "Removed": removed, "Origin": scheme + "://" + r.Host,
		"Flash": map[string]string{"theme": "Appearance saved.", "about": "Study profile saved.",
			"revoked": "That token no longer works.", "restored": "Restored to your library."}[r.URL.Query().Get("ok")],
	}, nil
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request, c *rc) error {
	data, err := s.settingsData(r, c)
	if err != nil {
		return err
	}
	return s.render(w, r, c, http.StatusOK, "settings.html", data)
}

func (s *Server) settingsTheme(w http.ResponseWriter, r *http.Request, c *rc) error {
	theme := r.FormValue("theme")
	if theme != "light" && theme != "dark" {
		theme = "system"
	}
	// Scripted saves carry the page's tab token (theme_tab) and its choice
	// number on that page (theme_seq); within one tab an older save arriving
	// late is ignored (and answered as usual), while saves from different
	// tabs or devices are last-writer-wins. A plain form submit carries
	// neither and always applies. An invalid token or number is dropped, so
	// that save takes the plain path.
	tab := themeTab(r.FormValue("theme_tab"))
	seq, err := strconv.ParseInt(r.FormValue("theme_seq"), 10, 64)
	if err != nil || seq < 0 {
		seq = 0
	}
	if _, err := c.us.SetUserTheme(r.Context(), theme, tab, seq); err != nil {
		return err
	}
	return back(w, r, "/settings?ok=theme")
}

// themeTab is a posted tab token, trimmed: at most 64 printable ASCII
// characters, or "" when it is missing or invalid.
func themeTab(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 64 {
		return ""
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return ""
		}
	}
	return v
}

func (s *Server) settingsAbout(w http.ResponseWriter, r *http.Request, c *rc) error {
	about := strings.TrimSpace(r.FormValue("about"))
	if len(about) > 1000 {
		about = about[:1000]
	}
	if err := c.us.SetAbout(r.Context(), about); err != nil {
		return err
	}
	if tz := r.FormValue("tz"); tz != "" {
		_ = c.us.SetTZ(r.Context(), tz)
	}
	return back(w, r, "/settings?ok=about")
}

func (s *Server) settingsToken(w http.ResponseWriter, r *http.Request, c *rc) error {
	name := r.FormValue("name")
	// A token has no options (S106-4): every token adds straight to the
	// library, and a posted can_add (from an older form) means nothing.
	token, err := s.App.CreateAgentToken(r.Context(), c.user.ID, name)
	var fe *app.FieldError
	if err != nil && !errors.As(err, &fe) {
		return err
	}
	data, derr := s.settingsData(r, c)
	if derr != nil {
		return derr
	}
	status := http.StatusOK
	if fe != nil {
		data["TokenError"], status = fe.Msg, http.StatusUnprocessableEntity
	} else {
		// Shown once, on this response only. Scry keeps only its hash.
		data["NewToken"], data["NewTokenName"] = token, strings.TrimSpace(name)
	}
	return s.render(w, r, c, status, "settings.html", data)
}

func (s *Server) settingsRevoke(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := c.us.RevokeToken(r.Context(), id, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/settings?ok=revoked")
}

func (s *Server) settingsRestore(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	// Recently removed lists ideas and cards (S106-5); a card's row says so.
	restore := c.us.RestoreIdea
	if r.FormValue("kind") == "card" {
		restore = c.us.RestoreItem
	}
	if err := restore(r.Context(), id); err != nil {
		return err
	}
	return back(w, r, "/settings?ok=restored")
}

// settingsExport hands the learner everything Scry holds for them. It
// works whatever the state of their plan: the data is theirs.
func (s *Server) settingsExport(w http.ResponseWriter, r *http.Request, c *rc) error {
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

// The API for the learner's own scripts and tools. A script authenticates
// with a token its learner made in Settings; everything it does lands in
// that learner's account only.

type agentRequest struct {
	Source string          `json:"source"`
	Items  []app.AgentItem `json:"items"`
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) (store.AgentToken, bool) {
	bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "send your API token as: Authorization: Bearer scry_…"})
		return store.AgentToken{}, false
	}
	t, err := s.App.AgentFromToken(r.Context(), strings.TrimSpace(bearer))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "that token is not valid or has been revoked"})
		return t, false
	}
	if info, ok := r.Context().Value(logKey{}).(*logInfo); ok {
		info.user = t.UserID
	}
	// No free tier applies to agents too.
	if _, err := s.App.Plan(r.Context(), t.UserID); err != nil {
		if errors.Is(err, app.ErrNoPlan) || errors.Is(err, app.ErrPlanEnded) {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "this learner has no active plan"})
		} else {
			s.Log.ErrorContext(r.Context(), "api plan check failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "try again"})
		}
		return t, false
	}
	return t, true
}

func (s *Server) agentBody(w http.ResponseWriter, r *http.Request) (agentRequest, bool) {
	var req agentRequest
	raw, err := readLimited(r, 1<<20)
	if err == nil {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		err = dec.Decode(&req)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `send JSON like {"source": "…", "items": [{"question": "…", "answer": "…", "idea": "…"}]}`})
		return req, false
	}
	return req, true
}

// apiSuggest is POST /api/v1/suggestions, kept for scripts written when
// cards waited in an Inbox. There is no Inbox now (S106-4): it is an alias
// of POST /api/v1/items and adds straight to the library, for every token.
func (s *Server) apiSuggest(w http.ResponseWriter, r *http.Request) { s.apiAdd(w, r) }

// apiAdd is POST /api/v1/items: 1 to 50 cards (question, answer, idea,
// summary, exact) go straight into the token owner's library, all or none.
func (s *Server) apiAdd(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agent(w, r)
	if !ok {
		return
	}
	req, ok := s.agentBody(w, r)
	if !ok {
		return
	}
	ids, err := s.App.AgentAdd(r.Context(), t, req.Source, req.Items)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"added": len(ids), "ids": ids, "status": "in the library"})
}

func (s *Server) apiIdeas(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agent(w, r)
	if !ok {
		return
	}
	outline, err := s.App.Store.For(t.UserID).Outline(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "try again"})
		return
	}
	type idea struct {
		Number string `json:"number"`
		Title  string `json:"title"`
		Items  int    `json:"items"`
	}
	out := []idea{}
	for _, o := range outline {
		out = append(out, idea{o.Number, o.Title, o.Items})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ideas": out})
}
