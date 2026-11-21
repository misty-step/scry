package web

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientErrLimiterStaysBounded(t *testing.T) {
	var l clientErrLimiter
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	for i := 0; i < clientErrorMaxIPs*3; i++ {
		l.allow("10.0."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256), now)
	}
	if len(l.ips) > clientErrorMaxIPs {
		t.Fatalf("tracking %d addresses, cap is %d", len(l.ips), clientErrorMaxIPs)
	}
	// A fresh address is refused while the table is full of live windows...
	if l.allow("192.0.2.1", now.Add(time.Second)) {
		t.Fatal("a full table must refuse, not grow")
	}
	// ...and expired windows are reclaimed once they pass.
	if !l.allow("192.0.2.1", now.Add(2*clientErrorWindow)) {
		t.Fatal("expired windows were not reclaimed")
	}
	if len(l.ips) > clientErrorMaxIPs {
		t.Fatalf("tracking %d addresses", len(l.ips))
	}
	for i := 0; i < clientErrorMaxSeen*3; i++ {
		l.first("fp"+strconv.Itoa(i), now)
	}
	if len(l.seen) > clientErrorMaxSeen {
		t.Fatalf("remembering %d fingerprints, cap is %d", len(l.seen), clientErrorMaxSeen)
	}
}

func TestClientErrLimiterGlobalBudget(t *testing.T) {
	var l clientErrLimiter
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	ok := 0
	for i := 0; i < clientErrorAll*2; i++ {
		if l.allow("10.1."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256), now) {
			ok++
		}
	}
	if ok != clientErrorAll {
		t.Fatalf("accepted %d from many addresses in one window, want the global %d", ok, clientErrorAll)
	}
}

func TestClientErrLimiterPreservesDedupWindowAtGlobalBudget(t *testing.T) {
	var l clientErrLimiter
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	fingerprint := "initial"
	// Start the first admission window before recording the fingerprint, so
	// another fixed window starts before its rolling window expires.
	if !l.allow("seed", now) || !l.allow("seed", now.Add(time.Second)) || !l.first(fingerprint, now.Add(time.Second)) {
		t.Fatal("first report was not accepted")
	}
	for bucket := 1; bucket <= int(clientErrorRepeat/clientErrorWindow); bucket++ {
		tick := now.Add(time.Duration(bucket) * clientErrorWindow)
		for i := 0; i < clientErrorAll; i++ {
			if !l.allow(strconv.Itoa(i), tick) || !l.first(strconv.Itoa(bucket*clientErrorAll+i), tick) {
				t.Fatalf("new report refused at bucket %d report %d", bucket, i)
			}
		}
	}
	if l.first(fingerprint, now.Add(clientErrorRepeat)) {
		t.Fatal("the initial report was forwarded again before expiry")
	}
	if !l.first(fingerprint, now.Add(clientErrorRepeat+time.Second)) {
		t.Fatal("the initial report was not forwarded again after expiry")
	}
}

// testPolicy is a Scry configured with SCRY_BASE_URL=https://scry.example
// and a Clerk instance.
var testPolicy = uriPolicy{base: uriOrigin{"https", "scry.example", "443"}, clerk: "clerk.scry-csp.example"}

func TestSanitizeClientURI(t *testing.T) {
	for in, want := range map[string]string{
		"":                                    "",
		"inline":                              "inline",
		"eval":                                "eval",
		"'self'":                              "self",
		"none":                                "none",
		"data":                                "data",
		"wasm-eval":                           "wasm-eval",
		"trusted-types-sink":                  "trusted-types-sink",
		"data:image/png;base64,AAAA":          "data",
		"blob:https://scry.example/1234-abcd": "blob",
		"about:blank":                         "about",
		"filesystem:https://x.example/temp/a": "filesystem",
		"chrome-extension://abcdef/inject.js": "extension",
		"moz-extension://abcdef/inject.js":    "extension",
		"safari-extension://a/b":              "extension",
		"safari-web-extension://a/b":          "extension",
		"edge-extension://a/b":                "extension",

		// Allowlisted integrations are https://<host> only: no path, query,
		// fragment, userinfo or port.
		"https://js.stripe.com/v3/?x=1#y":                                            "https://js.stripe.com",
		"https://u:p@js.stripe.com/v3/elements-inner-card.html?a=b#c":                "https://js.stripe.com",
		"https://js.stripe.com/cards/4242424242424242":                               "https://js.stripe.com",
		"https://js.stripe.com/u/bob@example.com/42":                                 "https://js.stripe.com",
		"https://js.stripe.com:443/v3/":                                              "https://js.stripe.com",
		"http://js.stripe.com/v3/":                                                   "https://js.stripe.com",
		"http://js.stripe.com:80/v3/":                                                "https://js.stripe.com",
		"https://hooks.stripe.com/3d_secure":                                         "https://hooks.stripe.com",
		"https://api.stripe.com/v1/payment_methods":                                  "https://api.stripe.com",
		"https://r.stripe.com/b":                                                     "https://r.stripe.com",
		"https://m.stripe.network/inner.html":                                        "https://m.stripe.network",
		"https://q.stripe.com/x":                                                     "https://q.stripe.com",
		"https://errors.stripe.com/x":                                                "https://errors.stripe.com",
		"https://challenges.cloudflare.com/turnstile/v0/api.js":                      "https://challenges.cloudflare.com",
		"https://img.clerk.com/avatar/alice-smith.png":                               "https://img.clerk.com",
		"https://clerk-telemetry.com/v1/event":                                       "https://clerk-telemetry.com",
		"https://clerk.scry-csp.example/npm/@clerk/clerk-js@5/dist/clerk.browser.js": "https://clerk.scry-csp.example",
		"https://b.js.stripe.com/v3/fingerprinted/js/x.js":                           "https://*.js.stripe.com",
		"https://alice-smith.js.stripe.com/x":                                        "https://*.js.stripe.com",
		"https://a.b.protect.clerk.com/p":                                            "https://*.protect.clerk.com",
		"https://csp-probe.invalid/probe.png?token=verify-probe-secret#frag":         externalURI,
		"https://csp-probe.invalid/private/notes.png":                                externalURI,

		// Everything else is external: no host, no path.
		"https://js.stripe.com:8443/v3/":                         externalURI,
		"http://js.stripe.com:443/v3/":                           externalURI,
		"https://js.stripe.com:80/v3/":                           externalURI,
		"wss://js.stripe.com/socket":                             externalURI,
		"https://js.stripe.com.evil.example/v3/cards/42?x=1":     externalURI,
		"https://evil-js.stripe.com/x":                           externalURI,
		"https://.js.stripe.com/x":                               externalURI,
		"https://protect.clerk.com/x":                            externalURI,
		"https://clerk.accounts.dev/x":                           externalURI,
		"https://u:p@Evil.Example:8443/a/b?c":                    externalURI,
		"wss://ws.example/socket?token=abc":                      externalURI,
		"https://cdn.example/lib.js?key=pk_live_abcdefgh":        externalURI,
		"https://x.example/private-notes/my-diagnosis":           externalURI,
		"//x.example/protocol-relative/secret":                   externalURI,
		"//scry.example/static/app.js":                           externalURI,
		"//js.stripe.com/v3/":                                    externalURI,
		"/\\evil.example/static/app.js":                          externalURI,
		"http://203.0.113.9/a?b":                                 externalURI,
		"http://[2001:db8::1]:8080/a":                            externalURI,
		"https://bob@example.com@evil.example/x":                 externalURI,
		"https://sk_live_abcdef123456.evil.example/x":            externalURI,
		"https://" + strings.Repeat("a", 64) + ".evil.example/x": externalURI,
		"https://scry.example.evil.example/static/app.js?x=1":    externalURI,
		"javascript:alert(document.cookie)":                      externalURI,
		"not a url %%%":                                          externalURI,
		"relative/path?x=1":                                      externalURI,
		"bob@example.com":                                        externalURI,
		"trusted-types-policy":                                   externalURI,
		"https://" + strings.Repeat("x", 3000):                   externalURI,

		// First-party: exactly the embedded static files, as a bare path,
		// on a relative URL, SCRY_BASE_URL's origin, the production host or
		// loopback. Any other first-party path is external.
		"/static/app.js?v=2":                                 "/static/app.js",
		"/static/stripe.js#x":                                "/static/stripe.js",
		"/static/clerk.js":                                   "/static/clerk.js",
		"/static/app.css":                                    "/static/app.css",
		"https://scry.example/static/stripe.js#x":            "/static/stripe.js",
		"https://scry.example:443/static/app.js":             "/static/app.js",
		"http://scry.example/static/app.js":                  externalURI,
		"https://scry.mistystep.io/static/app.js?token=x":    "/static/app.js",
		"https://scry.mistystep.io:443/static/app.js":        "/static/app.js",
		"http://scry.mistystep.io:80/static/app.js":          "/static/app.js",
		"http://scry.mistystep.io:443/static/app.js":         externalURI,
		"http://127.0.0.1:18080/static/app.css":              "/static/app.css",
		"http://localhost:8080/static/clerk.js":              "/static/clerk.js",
		"https://scry.mistystep.io:8443/static/app.js":       externalURI,
		"https://scry.example:8443/static/app.js":            externalURI,
		"/static/private-notes.js":                           externalURI,
		"/static/app.js/extra":                               externalURI,
		"/static/../static/app.js":                           externalURI,
		"https://scry.example/library/items/42?q=private":    externalURI,
		"https://scry.example/private/bob/4242424242424242":  externalURI,
		"https://scry.mistystep.io/signin?token=x":           externalURI,
		"/my-secret-page":                                    externalURI,
		"/api/v1/items?token=x":                              externalURI,
		"https://evil.example/static/app.js":                 externalURI,
		"https://scry.mistystep.io.evil.example/static/a.js": externalURI,
	} {
		if got, _ := testPolicy.uri(in); got != want {
			t.Errorf("uri(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ext := testPolicy.uri("moz-extension://x/y"); !ext {
		t.Error("moz-extension is an extension")
	}
	// Without SCRY_BASE_URL, an arbitrary host is never first-party, and
	// without Clerk configured no Clerk host is an integration.
	for in, want := range map[string]string{
		"https://scry.example/static/app.js":           externalURI,
		"https://clerk.scry-csp.example/npm/x.js":      externalURI,
		"https://scry.mistystep.io/static/stripe.js":   "/static/stripe.js",
		"https://js.stripe.com/v3/elements-inner.html": "https://js.stripe.com",
	} {
		if got, _ := (uriPolicy{}).uri(in); got != want {
			t.Errorf("unconfigured uri(%q) = %q, want %q", in, got, want)
		}
	}
}

// The first-party asset allowlist is exactly the embedded static files.
func TestClientStaticAssetsAreEmbeddedFiles(t *testing.T) {
	assets := staticAssets()
	for _, want := range []string{"/static/app.js", "/static/stripe.js", "/static/clerk.js", "/static/app.css"} {
		if !assets[want] {
			t.Errorf("missing %s from %v", want, assets)
		}
	}
	for path := range assets {
		if _, err := staticFS.ReadFile(strings.TrimPrefix(path, "/")); err != nil {
			t.Errorf("%s is not embedded: %v", path, err)
		}
	}
}

func TestSanitizeClientDirective(t *testing.T) {
	for _, d := range []string{"default-src", "script-src", "script-src-elem", "script-src-attr", "style-src",
		"style-src-elem", "style-src-attr", "img-src", "font-src", "connect-src", "frame-src", "worker-src",
		"child-src", "object-src", "media-src", "manifest-src", "form-action", "frame-ancestors", "base-uri",
		"navigate-to", "upgrade-insecure-requests"} {
		if got := sanitizeDirective(d); got != d {
			t.Errorf("sanitizeDirective(%q) = %q", d, got)
		}
	}
	for in, want := range map[string]string{
		"":                                  "",
		"   ":                               "",
		"FRAME-SRC":                         "frame-src",
		"frame-src https://js.stripe.com":   "frame-src",
		"alice-smith-diagnosis":             otherDirective,
		"script-src-private":                otherDirective,
		"require-trusted-types-for":         otherDirective,
		"bob@example.com":                   otherDirective,
		"https://evil.example/frame-src":    otherDirective,
		strings.Repeat("img-src", 10):       otherDirective,
		"img-src\n4242424242424242 private": "img-src",
	} {
		if got := sanitizeDirective(in); got != want {
			t.Errorf("sanitizeDirective(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every route is a known Scry route template or /other: arbitrary path text
// a caller sends never reaches logs or Sentry.
func TestSanitizeClientRoute(t *testing.T) {
	for in, want := range map[string]string{
		"/":                              "/",
		"/signin":                        "/signin",
		"/signup":                        "/signup",
		"/signup/card?next=%2Fbilling#x": "/signup/card",
		"/shelf":                         "/shelf",
		"/study":                         "/study",
		"/study/":                        "/study",
		"/study/done":                    "/study/done",
		"/library":                       "/library",
		"/library/ideas/7":               "/library/ideas/:id",
		"https://scry.mistystep.io/library/items/42?q=x": "/library/items/:id",
		"/library/texts/new":                             "/library/texts/new",
		"/library/texts/abc":                             "/library/texts/:id",
		"/inbox":                                         "/inbox",
		"/learn":                                         "/learn",
		"/learn/bob@example.com":                         "/learn/:id",
		"/billing":                                       "/billing",
		"/billing/card":                                  "/billing/card",
		"/settings":                                      "/settings",
		"/healthz":                                       "/healthz",
		"https://scry.mistystep.io":                      "/",

		"":                                   "/other",
		"/api/v1/items":                      "/other",
		"/api/client-error?x=1":              "/other",
		"javascript:alert(1)":                "/other",
		"relative/path":                      "/other",
		"//evil.example/study":               "/other",
		"/settings/tokens/sk_live_abcdef123": "/other",
		"/private-notes/my-diagnosis":        "/other",
		"/cards/4242424242424242":            "/other",
		"/library/items/42/secret-extra":     "/other",
		"/library/items/":                    "/other",
		"/learn/a%2Fb":                       "/other",
		"/static/app.js":                     "/other",
		"/STUDY":                             "/other",
		"/" + strings.Repeat("x", 3000):      "/other",
	} {
		if got := sanitizeRoute(in); got != want {
			t.Errorf("sanitizeRoute(%q) = %q, want %q", in, got, want)
		}
	}
}

// Diagnostic codes are server-defined: integration codes and error classes
// come from closed allowlists, and no message text is ever read.
func TestSanitizeClientCodes(t *testing.T) {
	for in, want := range map[string]string{
		"stripe_script_missing":        "stripe_script_missing",
		"stripe_mount_failed":          "stripe_mount_failed",
		"stripe_load_error":            "stripe_load_error",
		"stripe_ready_timeout":         "stripe_ready_timeout",
		"stripe_payment_method_failed": "stripe_payment_method_failed",
		"integration_unknown":          integrationUnknown,
		"":                             integrationUnknown,
		"stripe_script_missing: alice": integrationUnknown,
		"STRIPE_MOUNT_FAILED":          integrationUnknown,
		"stripe.js unavailable":        integrationUnknown,
		"my diagnosis bob@example.com": integrationUnknown,
	} {
		ev, ok := sanitizeClientReport(clientReport{Kind: "integration_error", Code: in}, testPolicy)
		if !ok || ev.code != want {
			t.Errorf("integration_error code %q = %q (ok %v), want %q", in, ev.code, ok, want)
		}
	}
	for _, c := range []struct{ kind, class, want string }{
		{"js_error", "TypeError", "TypeError"},
		{"js_error", "ReferenceError", "ReferenceError"},
		{"js_error", "SyntaxError", "SyntaxError"},
		{"js_error", "RangeError", "RangeError"},
		{"js_error", "URIError", "URIError"},
		{"js_error", "SecurityError", "SecurityError"},
		{"js_error", "NetworkError", "NetworkError"},
		{"js_error", "AbortError", "AbortError"},
		{"js_error", "DOMException", "DOMException"},
		{"js_error", "Error", "Error"},
		{"js_error", "", "UnknownError"},
		{"js_error", "AliceSmithError", "UnknownError"},
		{"js_error", "TypeError: card 4242424242424242", "UnknownError"},
		{"unhandled_rejection", "TypeError", "TypeError"},
		{"unhandled_rejection", "", "UnhandledRejection"},
		{"unhandled_rejection", "private text", "UnhandledRejection"},
	} {
		ev, ok := sanitizeClientReport(clientReport{Kind: c.kind, ErrorClass: c.class}, testPolicy)
		if !ok || ev.code != c.want {
			t.Errorf("%s class %q = %q (ok %v), want %q", c.kind, c.class, ev.code, ok, c.want)
		}
	}
	ev, ok := sanitizeClientReport(clientReport{Kind: "resource_error", Code: "alice", SourceFile: "https://x.example/a.js"}, testPolicy)
	if !ok || ev.code != resourceLoadFailed || ev.source != externalURI {
		t.Errorf("resource_error = %+v", ev)
	}
	// Line and column are kept only for an identified source.
	ev, _ = sanitizeClientReport(clientReport{Kind: "js_error", SourceFile: "/static/app.js", Line: 3, Column: 9}, testPolicy)
	if ev.summary("b") != "client js_error route=/other code=UnknownError source=/static/app.js:3:9 build=b" {
		t.Errorf("summary %q", ev.summary("b"))
	}
	ev, _ = sanitizeClientReport(clientReport{Kind: "js_error", SourceFile: "https://x.example/a.js", Line: 3, Column: 9}, testPolicy)
	if ev.line != 0 || ev.column != 0 || ev.summary("b") != "client js_error route=/other code=UnknownError source=external build=b" {
		t.Errorf("external source keeps position: %q", ev.summary("b"))
	}
	ev, _ = sanitizeClientReport(clientReport{Kind: "js_error", SourceFile: "https://js.stripe.com/v3/x.js", Line: 3, Column: 9}, testPolicy)
	if ev.summary("b") != "client js_error route=/other code=UnknownError source=https://js.stripe.com:3:9 build=b" {
		t.Errorf("allowlisted origin loses position: %q", ev.summary("b"))
	}
	// A CSP keyword is neither a first-party asset nor an allowlisted origin.
	for _, src := range []string{"inline", "eval", "blob:https://scry.example/uuid", "data:text/javascript,x"} {
		ev, _ = sanitizeClientReport(clientReport{Kind: "js_error", SourceFile: src, Line: 3, Column: 9}, testPolicy)
		if ev.line != 0 || ev.column != 0 {
			t.Errorf("keyword source %q keeps position: %q", src, ev.summary("b"))
		}
	}
}

// A report's summary and attributes are built only from closed sets, so no
// caller-supplied value can reach the log or Sentry, whatever is sent.
func TestSanitizeClientReportClosedSets(t *testing.T) {
	hostile := "alice-smith bob@example.com 4242424242424242 https://evil.example/private?token=sk_live_x#frag //proto.example/p"
	for _, kind := range []string{"csp_violation", "js_error", "unhandled_rejection", "resource_error", "integration_error"} {
		ev, ok := sanitizeClientReport(clientReport{Kind: kind, Route: hostile, ViolatedDirective: hostile,
			EffectiveDirective: hostile, BlockedURI: hostile, SourceFile: hostile, Code: hostile,
			ErrorClass: hostile, Disposition: hostile, Line: 5, Column: 6}, testPolicy)
		if !ok {
			t.Fatalf("%s refused", kind)
		}
		for _, v := range []string{ev.summary("b"), ev.route, ev.violated, ev.effective, ev.blocked, ev.source, ev.code, ev.disposition} {
			for _, leak := range []string{"alice", "bob", "4242", "evil", "private", "token", "sk_live", "frag", "proto"} {
				if strings.Contains(v, leak) {
					t.Errorf("%s: %q carries %q", kind, v, leak)
				}
			}
		}
	}
}
