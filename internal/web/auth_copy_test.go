package web_test

import (
	"net/http"
	"regexp"
	"testing"

	"scry/internal/auth"
)

// Newcomer sign-up/sign-in and required verification: each auth page names
// its own task. Hash-routed verification/reset are covered by the JS stand-in.
func TestStory_Newcomer_AuthPageHeadingsAndTitles(t *testing.T) {
	check := func(body, title string) {
		t.Helper()
		has(t, body, "<title>"+title+" · scry</title>")
		headings := regexp.MustCompile(`<h1[^>]*>([^<]*)</h1>`).FindAllStringSubmatch(body, -1)
		if len(headings) != 1 || headings[0][1] != title {
			t.Fatalf("want one H1 %q, got %v", title, headings)
		}
	}
	for _, mode := range []string{"local", "clerk", "unconfigured"} {
		t.Run(mode, func(t *testing.T) {
			w := newWorld(t)
			switch mode {
			case "clerk":
				w.app.Auth = &auth.Clerk{
					PublishableKey: "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk",
					SecretKey:      "stand-in-only",
				}
			case "unconfigured":
				w.app.Auth = &auth.Clerk{}
			}
			b := w.browser()
			for _, page := range []struct{ path, title string }{
				{"/signup", "Sign up"}, {"/signin", "Sign in"},
			} {
				title := page.title
				if mode == "unconfigured" {
					if page.path == "/signup" {
						title = "Sign-up not configured"
					} else {
						title = "Sign-in not configured"
					}
				}
				check(b.mustGet(page.path), title)
				// Error re-renders and Clerk POST fallbacks keep the same identity.
				_, body := b.post(page.path, "email", "invalid", "password", "short")
				check(body, title)
			}
			if mode == "local" {
				b.post("/signup", "email", "copy@example.com", "password", "correct horse")
				if b.path != "/shelf" {
					t.Fatalf("sign-up landed on %s, want /shelf", b.path)
				}
				body := b.mustGet("/signup/card")
				check(body, "Add a card")
				has(t, body, "Nothing is charged today: your trial still ends on", "Cancel any time before then in Billing.")
				status, body := b.post("/signup/card", "number", "invalid", "expiry", "12/30", "cvc", "123")
				if status != http.StatusPaymentRequired {
					t.Fatalf("card error status %d", status)
				}
				check(body, "Add a card")
			}
		})
	}
}
