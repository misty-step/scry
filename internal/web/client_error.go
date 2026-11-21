package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"scry/internal/auth"
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
	ev, ok := sanitizeClientReport(rep, s.uriPolicy(r))
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

// uriPolicy says which hosts a sanitized URI may name with a path: the
// page's own host, where only /static/ assets keep their path and pages
// become route templates, and the integrations Scry loads (Stripe, Clerk,
// Cloudflare Turnstile). Every other host keeps only scheme://host.
type uriPolicy struct {
	self  string // the page's own host[:port], lowercase
	clerk string // the configured Clerk Frontend API host, if any
}

func (s *Server) uriPolicy(r *http.Request) uriPolicy {
	p := uriPolicy{self: strings.ToLower(r.Host)}
	if provider, ok := s.App.Auth.(*auth.Clerk); ok && provider.Configured() {
		p.clerk = strings.ToLower(provider.Domain())
	}
	return p
}

func sanitizeClientReport(r clientReport, p uriPolicy) (clientEvent, bool) {
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
	e.blocked, extBlocked = p.uri(r.BlockedURI)
	e.source, extSource = p.uri(r.SourceFile)
	e.extension = extBlocked || extSource
	if kind == "integration_error" {
		e.message = integrationMessage(r.Message, p)
	} else {
		e.message = p.text(r.Message, clientMessageMax)
	}
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

// otherRoute stands for every page path Scry does not serve: what a caller
// put in the path never reaches logs or Sentry.
const otherRoute = "/other"

// routeExact and routeWithID are the page routes Scry serves (server.go);
// a reported route is one of these templates or otherRoute.
var (
	routeExact = map[string]bool{
		"/": true, "/signin": true, "/signup": true, "/signup/card": true, "/shelf": true,
		"/study": true, "/study/done": true, "/library": true, "/library/texts/new": true,
		"/inbox": true, "/learn": true, "/billing": true, "/billing/card": true,
		"/settings": true, "/healthz": true,
	}
	routeWithID = map[string]string{
		"/library/ideas": "/library/ideas/:id",
		"/library/items": "/library/items/:id",
		"/library/texts": "/library/texts/:id",
		"/learn":         "/learn/:id",
	}
)

// routeTemplate maps a decoded path to the Scry route template it matches,
// or to otherRoute.
func routeTemplate(path string) string {
	if path == "" {
		return "/"
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	if routeExact[path] {
		return path
	}
	if path == "/api" || strings.HasPrefix(path, "/api/") {
		return "/api/:path"
	}
	if i := strings.LastIndexByte(path, '/'); i > 0 && i < len(path)-1 {
		if t, ok := routeWithID[path[:i]]; ok {
			return t
		}
	}
	return otherRoute
}

// sanitizeRoute is the route template of a page, never its query,
// fragment, host or any path text a caller chose.
func sanitizeRoute(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 2048 {
		return otherRoute
	}
	u, err := url.Parse(s)
	if err != nil {
		return otherRoute
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return routeTemplate(u.Path)
	case "":
		if u.Host == "" && strings.HasPrefix(u.Path, "/") {
			return routeTemplate(u.Path)
		}
	}
	return otherRoute
}

var (
	tokenChars = regexp.MustCompile(`^[A-Za-z0-9_\-=~+]{20,}$`)
	digits     = regexp.MustCompile(`^[0-9]+$`)
	digitRun   = regexp.MustCompile(`[0-9]{6,}`)
	hostRe     = regexp.MustCompile(`^[a-z0-9.\-\[\]:]{1,100}$`)
)

// sanitizePath keeps a path's shape and drops what could identify a learner
// or carry a credential: card-like numbers become :number, ids :id, emails
// :email, and long token-shaped segments :token. It is used only for paths
// Scry trusts the shape of (its own /static/ assets and the integrations'
// scripts and frames).
func sanitizePath(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		switch {
		case seg == "":
		case emailSegment(seg):
			segs[i] = ":email"
		case cardDigits.MatchString(seg):
			segs[i] = ":number"
		case digits.MatchString(seg):
			segs[i] = ":id"
		case tokenLike(seg) || looksSecret(seg):
			segs[i] = ":token"
		case digitRun.MatchString(seg):
			segs[i] = ":id"
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

// emailSegment is a path segment holding an email address, plain or
// percent-encoded. Scoped package paths such as @clerk/clerk-js@5 are kept.
func emailSegment(seg string) bool {
	if dec, err := url.PathUnescape(seg); err == nil {
		seg = dec
	}
	return emailInText.MatchString(seg)
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
		if r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.~%+=,:@", r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

var keywordSources = map[string]bool{
	"inline": true, "eval": true, "wasm-eval": true, "self": true, "none": true, "data": true, "blob": true,
	"trusted-types-policy": true, "trusted-types-sink": true,
}

// integrationHosts are the third parties Scry's policy admits whose script
// and frame paths are worth keeping (sanitized). integrationSuffixes cover
// their per-instance subdomains.
var (
	integrationHosts = map[string]bool{
		"js.stripe.com": true, "hooks.stripe.com": true, "api.stripe.com": true, "m.stripe.network": true,
		"r.stripe.com": true, "q.stripe.com": true, "errors.stripe.com": true,
		"challenges.cloudflare.com": true, "clerk.accounts.dev": true, "clerk.scry.mistystep.io": true,
		"img.clerk.com": true, "protect.clerk.com": true, "clerk-telemetry.com": true,
		"csp-probe.invalid": true, // scripts/verify-live.mjs --simulate-violation
	}
	integrationSuffixes = []string{".js.stripe.com", ".protect.clerk.com", ".clerk.accounts.dev"}
)

func (p uriPolicy) integration(hostname string) bool {
	if integrationHosts[hostname] || (p.clerk != "" && hostname == p.clerk) {
		return true
	}
	for _, s := range integrationSuffixes {
		if strings.HasSuffix(hostname, s) {
			return true
		}
	}
	return false
}

// redactedHost replaces a host name that could itself carry data: an IP
// address, an over-long name or label, or a label shaped like a token,
// key, email or number.
const redactedHost = "[redacted-host]"

func suspiciousHost(hostname string) bool {
	if hostname == "" || len(hostname) > 253 || net.ParseIP(hostname) != nil || strings.ContainsAny(hostname, "@%[]:") {
		return true
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) > 63 || len(label) >= 40 || tokenLike(label) || looksSecret(label) || digitRun.MatchString(label) {
			return true
		}
	}
	return false
}

// uri reduces a URI to what is safe to forward, never with userinfo, query
// or fragment:
//   - a CSP keyword or bare scheme (inline, eval, data, blob, about)
//   - Scry's own /static/ assets with their sanitized path, and its own
//     pages as route templates
//   - an integration's script or frame with its sanitized path
//   - any other host as scheme://host only, or scheme://[redacted-host]
//     when the host name itself looks like data
//
// It also says whether the URI belongs to a browser extension.
func (p uriPolicy) uri(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	lower := strings.ToLower(s)
	if keywordSources[strings.Trim(lower, "'")] {
		return strings.Trim(lower, "'"), false
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
		return scheme + "://" + p.hostPath(u), false
	case "":
		if u.Host != "" { // protocol-relative //host/path
			return "//" + p.hostPath(u), false
		}
		if strings.HasPrefix(u.Path, "/") {
			return p.selfPath(u.Path), false
		}
		return "unparsed", false
	default:
		return schemeOnly(scheme)
	}
}

func (p uriPolicy) hostPath(u *url.URL) string {
	host := strings.ToLower(u.Host)
	if p.self != "" && host == p.self {
		return host + p.selfPath(u.Path)
	}
	hostname := strings.ToLower(u.Hostname())
	if !hostRe.MatchString(host) || suspiciousHost(hostname) {
		return redactedHost
	}
	if port := u.Port(); port != "" && !digits.MatchString(port) {
		return redactedHost
	}
	if p.integration(hostname) {
		return host + sanitizePath(u.Path)
	}
	return host
}

// selfPath is a path on Scry itself: a static asset keeps its sanitized
// path, anything else becomes its route template.
func (p uriPolicy) selfPath(path string) string {
	if strings.HasPrefix(path, "/static/") {
		return sanitizePath(path)
	}
	return routeTemplate(path)
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
	// card-number-shaped runs: 13 or more digits, each optionally
	// separated by one space or dash, wherever they appear (inside a word
	// or a path too)
	cardDigits = regexp.MustCompile(`[0-9](?:[ \-]?[0-9]){12,}`)
	// a word in a message carrying a query string or fragment, relative
	// (/signin?token=x, path?email=y, ?secret=1, #frag) or left over
	queryInText = regexp.MustCompile(`[^\s"'<>()?#]*[?#][^\s"'<>()]+`)
)

// looksSecret is true for a value shaped like a key, token or JWT.
func looksSecret(s string) bool { return secretText.MatchString(s) }

// clientMessageMax caps a forwarded free-text message (runes).
const clientMessageMax = 160

// text makes a free-text message safe to forward: one line, capped at max
// runes, every absolute URL reduced as uri does, every relative URL's query
// and fragment dropped, and emails, card-like numbers, long digit runs and
// token-shaped strings replaced.
func (p uriPolicy) text(s string, max int) string {
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
		clean, _ := p.uri(u)
		return clean
	})
	s = queryInText.ReplaceAllStringFunc(s, func(w string) string {
		i := strings.IndexAny(w, "?#")
		switch prefix := w[:i]; {
		case prefix == "":
			return w[:1] + "[redacted]"
		case strings.HasPrefix(prefix, "/"):
			return sanitizePath(prefix)
		default:
			return prefix
		}
	})
	s = emailInText.ReplaceAllString(s, "[email]")
	s = secretText.ReplaceAllString(s, "[redacted]")
	s = cardDigits.ReplaceAllString(s, "[number]")
	s = digitRun.ReplaceAllStringFunc(s, func(d string) string {
		if len(d) >= 7 {
			return "[number]"
		}
		return d
	})
	s = longToken.ReplaceAllString(s, "[redacted]")
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = strings.TrimRight(string(r[:max]), " ") + "…"
	}
	return s
}

// integrationMessages are the failures Scry's own scripts (stripe.js)
// report. An integration_error is forwarded as one of these fixed codes,
// with at most a short scrubbed detail; any other text is not forwarded.
var integrationMessages = []string{
	"stripe.js unavailable",
	"stripe card element failed to mount",
	"stripe card element not ready after 15s",
	"stripe card element loaderror",
	"stripe createPaymentMethod rejected",
}

const (
	integrationDetailMax = 80
	unknownIntegration   = "unrecognized integration error"
)

func integrationMessage(raw string, p uriPolicy) string {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return ""
	}
	for _, code := range integrationMessages {
		if !strings.HasPrefix(msg, code) {
			continue
		}
		rest := msg[len(code):]
		if rest != "" && !strings.HasPrefix(rest, ":") {
			continue // "stripe.js unavailableXYZ" is not the code
		}
		if d := p.text(strings.TrimPrefix(rest, ":"), integrationDetailMax); d != "" {
			return code + ": " + d
		}
		return code
	}
	return unknownIntegration
}
