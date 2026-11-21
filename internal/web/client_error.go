package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// clientErrorPath receives native CSP violation reports (the policy's
// report-uri) and the error telemetry app.js sends. Each accepted report
// becomes one sanitized error-level log line, which live mode forwards to
// Sentry through alert.SentryHandler. A browser failure the learner sees
// therefore reaches the operator instead of failing silently.
const clientErrorPath = "/api/client-error"

const (
	clientErrorMaxBody = 8 << 10 // bytes; larger bodies are refused with 413
	clientErrorPerIP   = 20      // reports per address per window
	clientErrorAll     = 240     // reports from everyone per window
	clientErrorWindow  = time.Minute
	clientErrorMaxIPs  = 4096 // addresses tracked at once; memory stays bounded
	clientErrorRepeat  = 10 * time.Minute
	clientErrorMaxSeen = 1024 // distinct reports remembered for de-duplication
)

// clientErrLimiter bounds what anonymous browsers can make Scry log: a fixed
// window per address and overall, and identical reports at error level at
// most once per clientErrorRepeat. Both maps have a hard size cap.
type clientErrLimiter struct {
	mu   sync.Mutex
	ips  map[string]*clientErrBucket
	all  clientErrBucket
	seen map[string]time.Time
}

type clientErrBucket struct {
	start time.Time
	n     int
}

func (l *clientErrLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ips == nil {
		l.ips = map[string]*clientErrBucket{}
	}
	b := l.ips[ip]
	if b != nil && now.Sub(b.start) >= clientErrorWindow {
		delete(l.ips, ip)
		b = nil
	}
	if b == nil {
		if len(l.ips) >= clientErrorMaxIPs {
			for k, v := range l.ips {
				if now.Sub(v.start) >= clientErrorWindow {
					delete(l.ips, k)
				}
			}
			if len(l.ips) >= clientErrorMaxIPs {
				return false // a flood of fresh addresses: refuse, never grow
			}
		}
		b = &clientErrBucket{start: now}
		l.ips[ip] = b
	}
	b.n++
	if now.Sub(l.all.start) >= clientErrorWindow {
		l.all = clientErrBucket{start: now}
	}
	l.all.n++
	return b.n <= clientErrorPerIP && l.all.n <= clientErrorAll
}

// first reports whether this exact sanitized report is new in the window.
func (l *clientErrLimiter) first(fingerprint string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]time.Time{}
	}
	if t, ok := l.seen[fingerprint]; ok && now.Sub(t) < clientErrorRepeat {
		return false
	}
	if len(l.seen) >= clientErrorMaxSeen {
		for k, t := range l.seen {
			if now.Sub(t) >= clientErrorRepeat {
				delete(l.seen, k)
			}
		}
		if len(l.seen) >= clientErrorMaxSeen {
			l.seen = map[string]time.Time{}
		}
	}
	l.seen[fingerprint] = now
	return true
}

// clientReport is the direct telemetry app.js sends. Every field is
// untrusted and is sanitized before use.
type clientReport struct {
	Kind               string `json:"kind"`
	Route              string `json:"route"`
	ViolatedDirective  string `json:"violatedDirective"`
	EffectiveDirective string `json:"effectiveDirective"`
	BlockedURI         string `json:"blockedURI"`
	SourceFile         string `json:"sourceFile"`
	Line               int    `json:"line"`
	Column             int    `json:"column"`
	Message            string `json:"message"`
	Disposition        string `json:"disposition"`
}

// nativeCSPReport is what browsers POST to report-uri.
type nativeCSPReport struct {
	Report *struct {
		DocumentURI        string `json:"document-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		BlockedURI         string `json:"blocked-uri"`
		SourceFile         string `json:"source-file"`
		Line               int    `json:"line-number"`
		Column             int    `json:"column-number"`
		Disposition        string `json:"disposition"`
		// original-policy, referrer and script-sample are never read: they
		// can carry page content.
	} `json:"csp-report"`
}

var clientErrorKinds = map[string]string{
	"csp_violation": "csp_violation", "securitypolicyviolation": "csp_violation", "csp-violation": "csp_violation",
	"js_error": "js_error", "error": "js_error",
	"unhandled_rejection": "unhandled_rejection", "unhandledrejection": "unhandled_rejection",
	"resource_error":    "resource_error",
	"integration_error": "integration_error",
}

func (s *Server) clientError(w http.ResponseWriter, r *http.Request) {
	now := s.App.Now()
	if !s.clientErrs.allow(clientIP(r), now) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": http.StatusText(http.StatusTooManyRequests)})
		return
	}
	if r.ContentLength > clientErrorMaxBody {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": http.StatusText(http.StatusRequestEntityTooLarge)})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, clientErrorMaxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": http.StatusText(http.StatusRequestEntityTooLarge)})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unreadable body"})
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var rep clientReport
	switch media {
	case "application/csp-report":
		var native nativeCSPReport
		if json.Unmarshal(body, &native) != nil || native.Report == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid CSP report"})
			return
		}
		n := native.Report
		rep = clientReport{Kind: "csp_violation", Route: n.DocumentURI, ViolatedDirective: n.ViolatedDirective,
			EffectiveDirective: n.EffectiveDirective, BlockedURI: n.BlockedURI, SourceFile: n.SourceFile,
			Line: n.Line, Column: n.Column, Disposition: n.Disposition}
	case "application/json":
		if json.Unmarshal(body, &rep) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	default:
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json or application/csp-report"})
		return
	}
	ev, ok := sanitizeClientReport(rep)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing kind, directive or message"})
		return
	}
	msg := ev.summary(s.App.Cfg.BuildSHA)
	level := slog.LevelError
	switch {
	case ev.extension:
		// Browser extensions trip the policy on their own; keep them local.
		level = slog.LevelWarn
	case !s.clientErrs.first(msg, now):
		level = slog.LevelWarn // already sent to Sentry in this window
	}
	attrs := []any{"kind", ev.kind, "route", ev.route, "violated_directive", ev.violated,
		"effective_directive", ev.effective, "blocked_uri", ev.blocked, "source_file", ev.source,
		"line", ev.line, "column", ev.column, "disposition", ev.disposition, "build_sha", s.App.Cfg.BuildSHA}
	if level == slog.LevelError {
		s.Log.ErrorContext(r.Context(), msg, attrs...)
	} else {
		s.Log.WarnContext(r.Context(), msg, append(attrs, "forwarded", false)...)
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientEvent is a report after sanitizing: nothing in it can carry a query
// string, fragment, userinfo, email address or token.
type clientEvent struct {
	kind, route, violated, effective, blocked, source, message, disposition string
	line, column                                                            int
	extension                                                               bool
}

func (e clientEvent) summary(build string) string {
	var b strings.Builder
	b.WriteString("client ")
	b.WriteString(e.kind)
	b.WriteString(" route=")
	b.WriteString(e.route)
	if e.kind == "csp_violation" {
		b.WriteString(" directive=")
		b.WriteString(e.violated)
		if e.effective != "" && e.effective != e.violated {
			b.WriteString(" effective=")
			b.WriteString(e.effective)
		}
		b.WriteString(" blocked=")
		if e.blocked == "" {
			b.WriteString("none")
		} else {
			b.WriteString(e.blocked)
		}
		if e.disposition == "report" {
			b.WriteString(" disposition=report")
		}
	}
	if e.message != "" {
		b.WriteString(" message=")
		b.WriteString(strconv.Quote(e.message))
	}
	if e.source != "" {
		b.WriteString(" source=")
		b.WriteString(e.source)
		if e.line > 0 {
			b.WriteString(":" + strconv.Itoa(e.line))
			if e.column > 0 {
				b.WriteString(":" + strconv.Itoa(e.column))
			}
		}
	}
	b.WriteString(" build=")
	if build == "" {
		build = "dev"
	}
	b.WriteString(build)
	return b.String()
}

func sanitizeClientReport(r clientReport) (clientEvent, bool) {
	kind, ok := clientErrorKinds[strings.ToLower(strings.TrimSpace(r.Kind))]
	if !ok {
		return clientEvent{}, false
	}
	e := clientEvent{kind: kind, route: sanitizeRoute(r.Route)}
	e.violated = sanitizeDirective(r.ViolatedDirective)
	e.effective = sanitizeDirective(r.EffectiveDirective)
	if e.violated == "" {
		e.violated = e.effective
	}
	var extBlocked, extSource bool
	e.blocked, extBlocked = sanitizeURI(r.BlockedURI)
	e.source, extSource = sanitizeURI(r.SourceFile)
	e.extension = extBlocked || extSource
	e.message = sanitizeText(r.Message, 300)
	if r.Line > 0 && r.Line < 1e7 {
		e.line = r.Line
	}
	if r.Column > 0 && r.Column < 1e7 {
		e.column = r.Column
	}
	if d := strings.ToLower(strings.TrimSpace(r.Disposition)); d == "enforce" || d == "report" {
		e.disposition = d
	}
	if kind == "csp_violation" {
		return e, e.violated != ""
	}
	return e, e.message != ""
}

var directiveRe = regexp.MustCompile(`^[a-z][a-z-]{0,39}$`)

// sanitizeDirective keeps the directive name only: older browsers send the
// whole directive with its source list.
func sanitizeDirective(s string) string {
	f := strings.Fields(strings.ToLower(s))
	if len(f) == 0 || !directiveRe.MatchString(f[0]) {
		return ""
	}
	return f[0]
}

// sanitizeRoute is the path of a page, never its query, fragment or host.
func sanitizeRoute(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2048 {
		s = s[:2048]
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	return sanitizePath(u.Path)
}

var (
	tokenChars = regexp.MustCompile(`^[A-Za-z0-9_\-=~+]{20,}$`)
	digits     = regexp.MustCompile(`^[0-9]+$`)
	hostRe     = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]{1,100}$`)
)

// sanitizePath keeps a path's shape and drops what could identify a learner
// or carry a credential: ids become :id, emails :email, and long
// token-shaped segments :token.
func sanitizePath(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		switch {
		case seg == "":
		case strings.Contains(seg, "@"):
			segs[i] = ":email"
		case digits.MatchString(seg):
			segs[i] = ":id"
		case tokenLike(seg) || looksSecret(seg):
			segs[i] = ":token"
		default:
			segs[i] = cleanSegment(seg)
		}
	}
	out := strings.Join(segs, "/")
	if len(out) > 200 {
		out = out[:200]
	}
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}

// tokenLike is a long run of token characters with digits in it (a hash,
// UUID, reset token or JWT part), ignoring a file extension. Plain words
// joined by dashes, such as elements-inner-card.html, are kept.
func tokenLike(seg string) bool {
	if i := strings.LastIndexByte(seg, '.'); i > 0 && len(seg)-i <= 6 {
		seg = seg[:i]
	}
	if !tokenChars.MatchString(seg) {
		return false
	}
	n := 0
	for _, r := range seg {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n >= 4
}

func cleanSegment(seg string) string {
	var b strings.Builder
	for _, r := range seg {
		if b.Len() >= 64 {
			break
		}
		if r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.~%+=,:", r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

var keywordSources = map[string]bool{
	"inline": true, "eval": true, "wasm-eval": true, "self": true, "data": true, "blob": true,
	"trusted-types-policy": true, "trusted-types-sink": true,
}

// sanitizeURI reduces a URI to scheme, host and path, or to the CSP
// keyword or scheme a browser reports (inline, eval, data, blob). It also
// says whether the URI belongs to a browser extension.
func sanitizeURI(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	lower := strings.ToLower(s)
	if keywordSources[lower] {
		return lower, false
	}
	if len(s) > 2048 {
		s = s[:2048]
	}
	u, err := url.Parse(s)
	if err != nil {
		if i := strings.IndexByte(lower, ':'); i > 0 && i < 30 {
			return schemeOnly(lower[:i])
		}
		return "unparsed", false
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "ws", "wss":
		if !hostRe.MatchString(u.Host) {
			return scheme, false
		}
		return scheme + "://" + strings.ToLower(u.Host) + sanitizePath(u.Path), false
	case "":
		if strings.HasPrefix(u.Path, "/") {
			return sanitizePath(u.Path), false
		}
		return "unparsed", false
	default:
		return schemeOnly(scheme)
	}
}

func schemeOnly(scheme string) (string, bool) {
	if strings.HasSuffix(scheme, "-extension") {
		return "extension", true
	}
	if !directiveRe.MatchString(scheme) {
		return "unparsed", false
	}
	return scheme, false
}

var (
	urlInText   = regexp.MustCompile(`(?i)\b(?:https?|wss?|blob|data|chrome-extension|moz-extension|safari-web-extension):[^\s"'<>()]+`)
	emailInText = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	secretText  = regexp.MustCompile(`(?i)\b(?:sk|pk|rk|whsec|sess|tok|key|pm|seti|pi|cus|sub)_[A-Za-z0-9_]{6,}|\beyJ[A-Za-z0-9_\-]{6,}(?:\.[A-Za-z0-9_\-]*){0,2}|\bbearer\s+\S+|\b(?:password|passwd|token|secret|session|csrf|authorization)\s*[=:]\s*\S+`)
	longToken   = regexp.MustCompile(`[A-Za-z0-9_\-+=]{32,}`)
	// card-number-shaped runs of digits, with or without separators
	cardDigits = regexp.MustCompile(`\b(?:\d[ \-]?){12,19}\b`)
)

// looksSecret is true for a value shaped like a key, token or JWT.
func looksSecret(s string) bool { return secretText.MatchString(s) }

// sanitizeText makes a free-text message safe to forward: one line, capped,
// with URLs reduced to scheme+host+path and emails, card numbers and
// token-shaped strings replaced.
func sanitizeText(s string, max int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "?")
	}
	if len(s) > 4096 {
		s = s[:4096]
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = urlInText.ReplaceAllStringFunc(s, func(u string) string {
		clean, _ := sanitizeURI(u)
		return clean
	})
	s = emailInText.ReplaceAllString(s, "[email]")
	s = secretText.ReplaceAllString(s, "[redacted]")
	s = cardDigits.ReplaceAllString(s, "[number]")
	s = longToken.ReplaceAllString(s, "[redacted]")
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max]) + "…"
	}
	return s
}
