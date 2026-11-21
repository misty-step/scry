package web

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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

// clientReport is the direct telemetry app.js and stripe.js send. Every
// field is untrusted. None is forwarded as text: each is mapped onto a
// closed, server-defined set of values (or dropped) before it is logged.
type clientReport struct {
	Kind               string `json:"kind"`
	Route              string `json:"route"`
	ViolatedDirective  string `json:"violatedDirective"`
	EffectiveDirective string `json:"effectiveDirective"`
	BlockedURI         string `json:"blockedURI"`
	SourceFile         string `json:"sourceFile"`
	Line               int    `json:"line"`
	Column             int    `json:"column"`
	Code               string `json:"code"`       // integration_error, resource_error
	ErrorClass         string `json:"errorClass"` // js_error, unhandled_rejection
	Disposition        string `json:"disposition"`
	// A "message" field, if sent, is never read: free text can carry
	// anything a learner typed or a page rendered.
}

// nativeCSPReport is what browsers POST to report-uri.
type nativeCSPReport struct {
	Report *struct {
		DocumentURI        string `json:"document-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		BlockedURI         string `json:"blocked-uri"`
		Disposition        string `json:"disposition"`
		// source-file, line-number, column-number, original-policy,
		// referrer and script-sample are never read.
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
			EffectiveDirective: n.EffectiveDirective, BlockedURI: n.BlockedURI, Disposition: n.Disposition}
	case "application/json":
		if json.Unmarshal(body, &rep) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	default:
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json or application/csp-report"})
		return
	}
	// The policy comes from server configuration only; nothing in the
	// request (its Host header included) decides what is first-party.
	ev, ok := sanitizeClientReport(rep, s.uriPolicy())
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing kind or directive"})
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
	attrs := []any{"kind", ev.kind, "route", ev.route, "code", ev.code, "violated_directive", ev.violated,
		"effective_directive", ev.effective, "blocked_uri", ev.blocked, "source_file", ev.source,
		"line", ev.line, "column", ev.column, "disposition", ev.disposition, "build_sha", s.App.Cfg.BuildSHA}
	if level == slog.LevelError {
		s.Log.ErrorContext(r.Context(), msg, attrs...)
	} else {
		s.Log.WarnContext(r.Context(), msg, append(attrs, "forwarded", false)...)
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientEvent is a report after sanitizing. Every string in it is a member
// of a closed, server-defined set: a route template, a CSP directive name,
// a diagnostic code, a CSP keyword, an allowlisted origin or first-party
// asset path, or "external". No caller text survives.
type clientEvent struct {
	kind, route, violated, effective, blocked, source, code, disposition string
	line, column                                                         int
	extension                                                            bool
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
	} else {
		b.WriteString(" code=")
		b.WriteString(e.code)
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
	}
	b.WriteString(" build=")
	if build == "" {
		build = "dev"
	}
	b.WriteString(build)
	return b.String()
}

// uriPolicy holds the server-configured origins a URI may be attributed
// to. It is built from configuration only, never from the request.
type uriPolicy struct {
	base  string // host[:port] of SCRY_BASE_URL, lowercase, if configured
	clerk string // the configured Clerk Frontend API host, if any
}

func (s *Server) uriPolicy() uriPolicy {
	var p uriPolicy
	if u, err := url.Parse(strings.TrimSpace(s.BaseURL)); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		p.base = strings.ToLower(u.Host)
	}
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
	switch kind {
	case "csp_violation":
		e.violated = sanitizeDirective(r.ViolatedDirective)
		e.effective = sanitizeDirective(r.EffectiveDirective)
		if e.violated == "" {
			e.violated = e.effective
		}
		e.blocked, e.extension = p.uri(r.BlockedURI)
		if d := strings.ToLower(strings.TrimSpace(r.Disposition)); d == "enforce" || d == "report" {
			e.disposition = d
		}
		return e, e.violated != ""
	case "integration_error":
		e.code = integrationCode(r.Code)
	case "resource_error":
		e.code = resourceLoadFailed
		e.source, e.extension = p.uri(r.SourceFile)
	case "js_error", "unhandled_rejection":
		e.code = errorClass(r.ErrorClass, kind)
		e.source, e.extension = p.uri(r.SourceFile)
		if identifiedSource(e.source) {
			if r.Line > 0 && r.Line < 1e7 {
				e.line = r.Line
			}
			if r.Column > 0 && r.Column < 1e7 {
				e.column = r.Column
			}
		}
	}
	return e, true
}

// identifiedSource is true for a first-party asset path or an allowlisted
// https origin (or wildcard): the only sources whose line and column are
// kept. A CSP keyword, "extension" or externalURI never carries a position.
func identifiedSource(source string) bool {
	return strings.HasPrefix(source, "/") || strings.HasPrefix(source, "https://")
}

// reportDirectives are the standard directive names a report may name. Any
// other non-empty directive is otherDirective.
var reportDirectives = map[string]bool{
	"default-src": true, "script-src": true, "script-src-elem": true, "script-src-attr": true,
	"style-src": true, "style-src-elem": true, "style-src-attr": true, "img-src": true, "font-src": true,
	"connect-src": true, "frame-src": true, "worker-src": true, "child-src": true, "object-src": true,
	"media-src": true, "manifest-src": true, "form-action": true, "frame-ancestors": true, "base-uri": true,
	"navigate-to": true, "upgrade-insecure-requests": true,
}

const otherDirective = "other-directive"

// sanitizeDirective is the directive's name if it is a standard one (older
// browsers send the whole directive with its source list), otherDirective
// for any other value, and "" for none.
func sanitizeDirective(s string) string {
	f := strings.Fields(strings.ToLower(s))
	if len(f) == 0 {
		return ""
	}
	if reportDirectives[f[0]] {
		return f[0]
	}
	return otherDirective
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
		if u.Host == "" && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(s, "//") {
			return routeTemplate(u.Path)
		}
	}
	return otherRoute
}

// externalURI stands for every URI that is not a CSP keyword, a
// first-party asset or an allowlisted integration origin: its host and
// path never reach logs or Sentry.
const externalURI = "external"

// uriKeywords are the values browsers report for non-URL sources; the
// schemes in uriKeywordSchemes are reported by scheme alone.
var (
	uriKeywords = map[string]bool{
		"inline": true, "eval": true, "data": true, "blob": true, "wasm-eval": true,
		"trusted-types-sink": true, "self": true, "none": true, "about": true, "filesystem": true,
	}
	uriKeywordSchemes = map[string]bool{"data": true, "blob": true, "about": true, "filesystem": true}
	extensionSchemes  = map[string]bool{
		"chrome-extension": true, "moz-extension": true, "safari-extension": true,
		"safari-web-extension": true, "edge-extension": true,
	}
)

// firstPartyHosts are the hosts, besides SCRY_BASE_URL's, whose /static/
// assets are Scry's own. Loopback is any port (local runs and the
// verifier); the production host is the default port only.
var firstPartyHosts = map[string]bool{"scry.mistystep.io": false, "127.0.0.1": true, "localhost": true}

// staticAssets is the closed set of first-party asset paths: exactly the
// files embedded under static/.
var staticAssets = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out["/"+path] = true
		}
		return nil
	})
	return out
})

// integrationHosts are the third-party hosts Scry's pages load; each is
// reported as https://<host> only. integrationWildcards are their
// per-instance subdomains, reported as the wildcard itself.
var (
	integrationHosts = map[string]bool{
		"js.stripe.com": true, "hooks.stripe.com": true, "api.stripe.com": true, "r.stripe.com": true,
		"m.stripe.network": true, "q.stripe.com": true, "errors.stripe.com": true,
		"challenges.cloudflare.com": true, "img.clerk.com": true, "clerk-telemetry.com": true,
	}
	integrationWildcards = []string{".js.stripe.com", ".protect.clerk.com"}
	hostLabels           = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)
)

// uri maps a reported URI onto a closed set of values: "" for none, a CSP
// keyword, "extension" (and true) for a browser extension, a first-party
// asset path such as /static/stripe.js, an allowlisted integration origin
// such as https://js.stripe.com, or externalURI for anything else. No
// query, fragment, userinfo, caller-chosen host or caller-chosen path is
// ever returned.
func (p uriPolicy) uri(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if len(s) > 2048 {
		return externalURI, false
	}
	lower := strings.ToLower(s)
	if k := strings.Trim(lower, "'"); uriKeywords[k] {
		return k, false
	}
	if i := strings.IndexByte(lower, ':'); i > 0 {
		scheme := lower[:i]
		if extensionSchemes[scheme] {
			return "extension", true
		}
		if uriKeywordSchemes[scheme] {
			return scheme, false
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return externalURI, false
	}
	switch strings.ToLower(u.Scheme) {
	case "":
		// Only a same-origin absolute path: never //host/... (protocol-
		// relative) or a relative path.
		if u.Host == "" && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(s, "//") && !strings.HasPrefix(s, "/\\") {
			return asset(u.Path), false
		}
	case "http", "https":
		if p.firstParty(u) {
			return asset(u.Path), false
		}
		return p.integration(u), false
	}
	return externalURI, false
}

// firstParty is true when u's host is Scry's configured SCRY_BASE_URL
// origin, the production host or loopback.
func (p uriPolicy) firstParty(u *url.URL) bool {
	if p.base != "" && strings.ToLower(u.Host) == p.base {
		return true
	}
	anyPort, ok := firstPartyHosts[strings.ToLower(u.Hostname())]
	if !ok {
		return false
	}
	if port := u.Port(); port != "" && port != "443" {
		return anyPort && isPort(port)
	}
	return true
}

func isPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536 && strconv.Itoa(n) == s
}

// asset is a first-party path if it is exactly one of Scry's embedded
// static files, externalURI otherwise.
func asset(path string) string {
	if staticAssets()[path] {
		return path
	}
	return externalURI
}

// integration is https://<host> for an allowlisted third-party host on its
// default port, the wildcard for an allowlisted subdomain, and externalURI
// for anything else.
func (p uriPolicy) integration(u *url.URL) string {
	if port := u.Port(); port != "" && port != "443" {
		return externalURI
	}
	host := strings.ToLower(u.Hostname())
	if integrationHosts[host] || (p.clerk != "" && host == p.clerk) {
		return "https://" + host
	}
	for _, suffix := range integrationWildcards {
		if prefix, ok := strings.CutSuffix(host, suffix); ok && prefix != "" && hostLabels.MatchString(prefix) {
			return "https://*" + suffix
		}
	}
	return externalURI
}

// integrationCodes are the failures stripe.js reports. Any other code, or
// none, is integrationUnknown.
var integrationCodes = map[string]bool{
	"stripe_script_missing": true, "stripe_mount_failed": true, "stripe_load_error": true,
	"stripe_ready_timeout": true, "stripe_payment_method_failed": true,
}

const (
	integrationUnknown = "integration_unknown"
	resourceLoadFailed = "resource_load_failed"
)

func integrationCode(code string) string {
	if c := strings.TrimSpace(code); integrationCodes[c] {
		return c
	}
	return integrationUnknown
}

// errorClasses are the error names a js_error or unhandled_rejection may
// report. The exception's message is never read.
var errorClasses = map[string]bool{
	"TypeError": true, "ReferenceError": true, "SyntaxError": true, "RangeError": true, "URIError": true,
	"SecurityError": true, "NetworkError": true, "AbortError": true, "DOMException": true, "Error": true,
	"UnhandledRejection": true, "UnknownError": true,
}

func errorClass(name, kind string) string {
	if n := strings.TrimSpace(name); errorClasses[n] {
		return n
	}
	if kind == "unhandled_rejection" {
		return "UnhandledRejection"
	}
	return "UnknownError"
}
