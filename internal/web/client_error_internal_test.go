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

// testPolicy is a Scry served at scry.example with a Clerk instance.
var testPolicy = uriPolicy{self: "scry.example", clerk: "clerk.scry-csp.example"}

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
		"chrome-extension://abcdef/inject.js": "extension",
		"javascript:alert(document.cookie)":   "javascript",
		"not a url %%%":                       "unparsed",
		"relative/path?x=1":                   "unparsed",

		// Integrations keep their (sanitized) path; query, fragment and
		// userinfo never survive.
		"https://js.stripe.com/v3/?x=1#y":                                            "https://js.stripe.com/v3/",
		"https://u:p@js.stripe.com/v3/elements-inner-card.html?a=b#c":                "https://js.stripe.com/v3/elements-inner-card.html",
		"https://hooks.stripe.com/3d_secure":                                         "https://hooks.stripe.com/3d_secure",
		"https://b.js.stripe.com/v3/fingerprinted/js/x.js":                           "https://b.js.stripe.com/v3/fingerprinted/js/x.js",
		"https://happy-cat-12.clerk.accounts.dev/npm/x.js?k=pk_test_1":               "https://happy-cat-12.clerk.accounts.dev/npm/x.js",
		"https://clerk.scry-csp.example/npm/@clerk/clerk-js@5/dist/clerk.browser.js": "https://clerk.scry-csp.example/npm/@clerk/clerk-js@5/dist/clerk.browser.js",
		"https://js.stripe.com/reset/" + "abcdefghijklmnopqrstuvwxyz012345":          "https://js.stripe.com/reset/:token",
		"https://js.stripe.com/u/bob@example.com/42":                                 "https://js.stripe.com/u/:email/:id",
		"https://js.stripe.com/u/bob%40example.com":                                  "https://js.stripe.com/u/:email",
		"https://js.stripe.com/cards/4242424242424242":                               "https://js.stripe.com/cards/:number",
		"https://js.stripe.com/4242-4242-4242-4242":                                  "https://js.stripe.com/:number",
		"https://js.stripe.com/c/card4242424242424242.png":                           "https://js.stripe.com/c/:number",
		"https://csp-probe.invalid/probe.png?token=verify-probe-secret#frag":         "https://csp-probe.invalid/probe.png",

		// Unknown hosts keep scheme://host only: no path at all.
		"https://u:p@Evil.Example:8443/a/b?c":                    "https://evil.example:8443",
		"wss://ws.example/socket?token=abc":                      "wss://ws.example",
		"https://cdn.example/lib.js?key=pk_live_abcdefgh":        "https://cdn.example",
		"https://x.example/u/bob@example.com/42":                 "https://x.example",
		"https://x.example/cards/4242424242424242":               "https://x.example",
		"https://x.example/private-notes/my-diagnosis":           "https://x.example",
		"//x.example/protocol-relative/secret":                   "//x.example",
		"http://203.0.113.9/a?b":                                 "http://[redacted-host]",
		"http://[2001:db8::1]:8080/a":                            "http://[redacted-host]",
		"https://bob@example.com@evil.example/x":                 "https://evil.example",
		"https://sk_live_abcdef123456.evil.example/x":            "https://[redacted-host]",
		"https://a1b2c3d4e5f6a7b8c9d0e1f2.evil.example/":         "https://[redacted-host]",
		"https://4242424242424242.evil.example/":                 "https://[redacted-host]",
		"https://" + strings.Repeat("a", 64) + ".evil.example/x": "https://[redacted-host]",
		"https://scry.example.evil.example/static/app.js?x=1":    "https://scry.example.evil.example",
		"https://js.stripe.com.evil.example/v3/cards/42?x=1":     "https://js.stripe.com.evil.example",

		// Scry itself: /static/ keeps its path, pages are route templates.
		"/static/app.js?v=2":                                "/static/app.js",
		"https://scry.example/static/stripe.js#x":           "https://scry.example/static/stripe.js",
		"https://scry.example/library/items/42?q=private":   "https://scry.example/library/items/:id",
		"https://scry.example/private/bob/4242424242424242": "https://scry.example/other",
		"/my-secret-page":                                   "/other",
		"/api/v1/items?token=x":                             "/api/:path",
	} {
		if got, _ := testPolicy.uri(in); got != want {
			t.Errorf("uri(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ext := testPolicy.uri("moz-extension://x/y"); !ext {
		t.Error("moz-extension is an extension")
	}
	// Without a known own host (no Host header), a scheme://host URL is an
	// unknown host and keeps no path.
	if got, _ := (uriPolicy{}).uri("https://scry.example/static/app.js"); got != "https://scry.example" {
		t.Errorf("no self host: %q", got)
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
		"/api/v1/items":                                  "/api/:path",
		"/api/client-error?x=1":                          "/api/:path",
		"https://scry.mistystep.io":                      "/",

		"":                                   "/other",
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

func TestSanitizeClientText(t *testing.T) {
	got := testPolicy.text("line one\nline two with bob@example.com Bearer abc.def token=xyz https://h.example/p?q=1 ", 300)
	want := "line one line two with [email] [redacted] [redacted] https://h.example"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	for in, want := range map[string]string{
		// Relative URLs and bare queries lose their query and fragment.
		"redirect to /signup?token=abc123 failed":      "redirect to /signup failed",
		"bad /signin?token=secret123#frag here":        "bad /signin here",
		"/foo/bar?token=abc#frag":                      "/foo/bar",
		"got ?secret=1 back":                           "got ?[redacted] back",
		"jump #access_token=xyz":                       "jump #[redacted]",
		"path?email=a@b.com":                           "path",
		"fetch(\"/api/v1/items?key=k\") failed":        "fetch(\"/api/v1/items\") failed",
		"at https://js.stripe.com/v3/x.js?y=1:3:4":     "at https://js.stripe.com/v3/x.js",
		"at https://scry.example/library/items/42?q=1": "at https://scry.example/library/items/:id",
		"at https://evil.example/private/notes?q=1":    "at https://evil.example",
		// Card-like numbers anywhere: in text, in paths, inside words.
		"card 4242 4242 4242 4242 declined":  "card [number] declined",
		"card 4242-4242-4242-4242":           "card [number]",
		"GET /cards/4242424242424242 failed": "GET /cards/[number] failed",
		"GET /4242-4242-4242-4242/x":         "GET /[number]/x",
		"id=card4242424242424242":            "id=card[number]",
		"phone 5551234567":                   "phone [number]",
		"line 12 column 345678 ok":           "line 12 column 345678 ok",
		"TypeError: card is undefined":       "TypeError: card is undefined",
	} {
		if got := testPolicy.text(in, 300); got != want {
			t.Errorf("text(%q) = %q, want %q", in, got, want)
		}
	}
	long := testPolicy.text(strings.Repeat("a ", 400), clientMessageMax)
	if n := len([]rune(long)); n > clientMessageMax+1 {
		t.Fatalf("not capped: %d runes", n)
	}
}

func TestSanitizeClientMessagesByKind(t *testing.T) {
	ev, ok := sanitizeClientReport(clientReport{Kind: "js_error", Route: "/study", Message: strings.Repeat("boom ", 100)}, testPolicy)
	if !ok || len([]rune(ev.message)) > clientMessageMax+1 || strings.ContainsAny(ev.message, "\n\r") {
		t.Fatalf("js_error message not capped to one %d-rune line: %q", clientMessageMax, ev.message)
	}
	for in, want := range map[string]string{
		"stripe.js unavailable: window.Stripe is missing":                              "stripe.js unavailable: window.Stripe is missing",
		"stripe.js unavailable":                                                        "stripe.js unavailable",
		"stripe card element not ready after 15s":                                      "stripe card element not ready after 15s",
		"stripe card element loaderror: invalid_request_error":                         "stripe card element loaderror: invalid_request_error",
		"stripe createPaymentMethod rejected: Failed to fetch https://x.example/a?b=c": "stripe createPaymentMethod rejected: Failed to fetch https://x.example",
		"stripe card element failed to mount: " + strings.Repeat("x ", 200):            "stripe card element failed to mount: " + strings.TrimSpace(strings.Repeat("x ", 40)) + "…",
		"my diagnosis is private, bob@example.com":                                     unknownIntegration,
		"stripe.js unavailableANDSECRET":                                               unknownIntegration,
	} {
		ev, ok := sanitizeClientReport(clientReport{Kind: "integration_error", Message: in}, testPolicy)
		if !ok || ev.message != want {
			t.Errorf("integration_error %q = %q (ok %v), want %q", in, ev.message, ok, want)
		}
	}
}
