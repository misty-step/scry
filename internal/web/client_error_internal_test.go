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

func TestSanitizeClientURI(t *testing.T) {
	for in, want := range map[string]string{
		"":                                    "",
		"inline":                              "inline",
		"eval":                                "eval",
		"data":                                "data",
		"data:image/png;base64,AAAA":          "data",
		"blob:https://scry.example/1234-abcd": "blob",
		"https://js.stripe.com/v3/?x=1#y":     "https://js.stripe.com/v3/",
		"https://u:p@Evil.Example:8443/a/b?c": "https://evil.example:8443/a/b",
		"wss://ws.example/socket?token=abc":   "wss://ws.example/socket",
		"https://x.example/reset/" + "abcdefghijklmnopqrstuvwxyz012345":     "https://x.example/reset/:token",
		"https://x.example/u/bob@example.com/42":                            "https://x.example/u/:email/:id",
		"https://x.example/u/bob%40example.com":                             "https://x.example/u/:email",
		"https://clerk.example/npm/@clerk/clerk-js@5/dist/clerk.browser.js": "https://clerk.example/npm/@clerk/clerk-js@5/dist/clerk.browser.js",
		"/static/app.js?v=2":                  "/static/app.js",
		"about:blank":                         "about",
		"chrome-extension://abcdef/inject.js": "extension",
		"javascript:alert(document.cookie)":   "javascript",
		"not a url %%%":                       "unparsed",
	} {
		if got, _ := sanitizeURI(in); got != want {
			t.Errorf("sanitizeURI(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ext := sanitizeURI("moz-extension://x/y"); !ext {
		t.Error("moz-extension is an extension")
	}
}

func TestSanitizeClientRoute(t *testing.T) {
	for in, want := range map[string]string{
		"":                               "/",
		"/signup/card?next=%2Fbilling#x": "/signup/card",
		"https://scry.mistystep.io/library/items/42?q=x": "/library/items/:id",
		"javascript:alert(1)":                            "/",
		"relative/path":                                  "/",
		"/settings/tokens/sk_live_abcdef123":             "/settings/tokens/:token",
	} {
		if got := sanitizeRoute(in); got != want {
			t.Errorf("sanitizeRoute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeClientText(t *testing.T) {
	got := sanitizeText("line one\nline two with bob@example.com Bearer abc.def token=xyz https://h.example/p?q=1 ", 300)
	want := "line one line two with [email] [redacted] [redacted] https://h.example/p"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	long := sanitizeText(strings.Repeat("a ", 400), 300)
	if n := len([]rune(long)); n > 301 {
		t.Fatalf("not capped: %d runes", n)
	}
}
