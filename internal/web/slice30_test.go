package web_test

import (
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
	"scry/internal/web"
)

// Slice 30: the Stripe card field must never be blocked by Scry's own
// Content-Security-Policy, and a policy violation or script failure in a
// learner's browser must reach Sentry instead of failing silently.

const clerkTestDomain = "clerk.scry-csp.example"

func clerkTestKey() string {
	return "pk_test_" + base64.RawStdEncoding.EncodeToString([]byte(clerkTestDomain+"$"))
}

// cspRig is one Scry whose operator log runs through alert.SentryHandler,
// exactly as cmd/scry wires it in live mode, with Sentry's capture recorded.
type cspRig struct {
	t     *testing.T
	srv   *httptest.Server
	logs  *syncBuffer
	mu    sync.Mutex
	sent  []string
	clock time.Time
}

func newCSPRig(t *testing.T, stripe, clerk bool) *cspRig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "scry.db"))
	if err != nil {
		t.Fatal(err)
	}
	r := &cspRig{t: t, logs: &syncBuffer{}, clock: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)}
	log := slog.New(alert.SentryHandler{
		Handler: slog.NewJSONHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
		Capture: func(m string) { r.mu.Lock(); r.sent = append(r.sent, m); r.mu.Unlock() },
	})
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	cfg.BuildSHA = "abc1234"
	a := &app.App{Store: st, Auth: &auth.Local{DB: st.DB(), Cost: bcrypt.MinCost}, Pay: billing.Fake{},
		Model: &ai.Fake{}, Speech: speech.Fake{}, Alert: &alert.Recorder{}, Log: log, Cfg: cfg, Now: r.now}
	if stripe {
		p, err := billing.NewStripe("sk_test_standin", "pk_test_standin", "price_standin", "whsec_standin")
		if err != nil {
			t.Fatal(err)
		}
		a.Pay = p
	}
	if clerk {
		c := &auth.Clerk{PublishableKey: clerkTestKey(), SecretKey: "sk_test_standin"}
		if !c.Configured() || c.Domain() != clerkTestDomain {
			t.Fatalf("clerk stand-in not configured: %q", c.Domain())
		}
		a.Auth = c
	}
	r.srv = httptest.NewServer(web.New(a, log))
	t.Cleanup(func() { r.srv.Close(); st.Close() })
	return r
}

func (r *cspRig) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clock
}

func (r *cspRig) advance(d time.Duration) {
	r.mu.Lock()
	r.clock = r.clock.Add(d)
	r.mu.Unlock()
}

func (r *cspRig) captured() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, m := range r.sent {
		if strings.HasPrefix(m, "client ") {
			out = append(out, m)
		}
	}
	return out
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (r *cspRig) policy(path string) (int, string, http.Header) {
	r.t.Helper()
	resp, err := noRedirect().Get(r.srv.URL + path)
	if err != nil {
		r.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Content-Security-Policy"), resp.Header
}

func (r *cspRig) report(contentType, body string) int {
	r.t.Helper()
	resp, err := http.Post(r.srv.URL+"/api/client-error", contentType, strings.NewReader(body))
	if err != nil {
		r.t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// parseCSP splits a policy into directive -> sources, failing on a
// directive that appears twice or a source repeated inside one directive.
func parseCSP(t *testing.T, policy string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, part := range strings.Split(policy, ";") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		if _, dup := out[f[0]]; dup {
			t.Fatalf("directive %s appears twice: %s", f[0], policy)
		}
		seen := map[string]bool{}
		for _, s := range f[1:] {
			if seen[s] {
				t.Fatalf("%s repeats %s: %s", f[0], s, policy)
			}
			seen[s] = true
		}
		out[f[0]] = f[1:]
	}
	return out
}

func includes(t *testing.T, csp map[string][]string, directive string, want ...string) {
	t.Helper()
	for _, w := range want {
		found := false
		for _, s := range csp[directive] {
			if s == w {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lacks %s; has %v", directive, w, csp[directive])
		}
	}
}

func exactly(t *testing.T, csp map[string][]string, directive string, want ...string) {
	t.Helper()
	if strings.Join(csp[directive], " ") != strings.Join(want, " ") {
		t.Errorf("%s = %v, want %v", directive, csp[directive], want)
	}
}

func TestSlice30CSPComposesStripeAndClerk(t *testing.T) {
	clerkOrigin := "https://" + clerkTestDomain
	for _, tc := range []struct {
		name          string
		stripe, clerk bool
	}{
		{"local auth, fake billing", false, false},
		{"stripe only", true, false},
		{"clerk only", false, true},
		{"clerk and stripe together", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newCSPRig(t, tc.stripe, tc.clerk)
			status, policy, h := r.policy("/signup/card")
			if status != http.StatusSeeOther {
				t.Fatalf("GET /signup/card signed out = %d, want 303", status)
			}
			if loc := h.Get("Location"); loc != "/signin?next=%2Fsignup%2Fcard" {
				t.Fatalf("redirect to %q", loc)
			}
			// Every route gets the same, deterministic policy.
			for _, path := range []string{"/", "/signin", "/signup", "/healthz", "/signup/card"} {
				if _, again, _ := r.policy(path); again != policy {
					t.Fatalf("policy on %s differs:\n%s\n%s", path, again, policy)
				}
			}
			csp := parseCSP(t, policy)
			exactly(t, csp, "default-src", "'self'")
			exactly(t, csp, "font-src", "'self'")
			exactly(t, csp, "base-uri", "'none'")
			exactly(t, csp, "form-action", "'self'")
			exactly(t, csp, "frame-ancestors", "'none'")
			exactly(t, csp, "report-uri", "/api/client-error")
			includes(t, csp, "script-src", "'self'")
			includes(t, csp, "connect-src", "'self'")
			includes(t, csp, "img-src", "'self'", "data:")
			for _, d := range []string{"script-src", "style-src", "connect-src"} {
				for _, s := range csp[d] {
					if s == "'unsafe-eval'" || s == "*" || s == "https:" {
						t.Errorf("%s is too broad: %s", d, s)
					}
				}
			}
			if !tc.stripe && !tc.clerk {
				exactly(t, csp, "frame-src", "'none'")
				exactly(t, csp, "script-src", "'self'")
				exactly(t, csp, "style-src", "'self'")
				exactly(t, csp, "connect-src", "'self'")
				if _, ok := csp["worker-src"]; ok {
					t.Error("worker-src without Clerk")
				}
				return
			}
			for _, s := range csp["frame-src"] {
				if s == "'none'" {
					t.Errorf("frame-src keeps 'none' beside real origins: %v", csp["frame-src"])
				}
			}
			includes(t, csp, "style-src", "'self'", "'unsafe-inline'")
			if tc.stripe {
				includes(t, csp, "script-src", "https://js.stripe.com", "https://m.stripe.network")
				includes(t, csp, "frame-src", "https://js.stripe.com", "https://hooks.stripe.com", "https://m.stripe.network")
				includes(t, csp, "connect-src", "https://api.stripe.com", "https://r.stripe.com", "https://m.stripe.network",
					"https://q.stripe.com", "https://errors.stripe.com")
				includes(t, csp, "img-src", "https://*.stripe.com")
			}
			if tc.clerk {
				includes(t, csp, "script-src", clerkOrigin, "https://challenges.cloudflare.com", "https://*.protect.clerk.com")
				includes(t, csp, "frame-src", "https://challenges.cloudflare.com", "https://*.protect.clerk.com")
				includes(t, csp, "connect-src", clerkOrigin, "https://challenges.cloudflare.com", "https://*.protect.clerk.com:*", "https://clerk-telemetry.com")
				includes(t, csp, "img-src", clerkOrigin, "https://img.clerk.com")
				exactly(t, csp, "worker-src", "'self'", "blob:")
			}
			if tc.stripe && tc.clerk {
				// The live bug: Clerk's frame-src replaced Stripe's, so the
				// card field rendered as a broken-document icon.
				includes(t, csp, "frame-src", "https://js.stripe.com", "https://hooks.stripe.com", "https://challenges.cloudflare.com")
				includes(t, csp, "script-src", "https://js.stripe.com", clerkOrigin, "https://challenges.cloudflare.com")
				includes(t, csp, "connect-src", "https://api.stripe.com", clerkOrigin)
			}
		})
	}
}

func TestSlice30ClientErrorsReachSentry(t *testing.T) {
	r := newCSPRig(t, true, true)
	for _, c := range []struct {
		name, contentType, body string
		want                    []string
	}{
		{"securitypolicyviolation", "application/json",
			`{"kind":"securitypolicyviolation","route":"/signup/card","violatedDirective":"frame-src","effectiveDirective":"frame-src","blockedURI":"https://js.stripe.com/v3/elements-inner-card.html"}`,
			[]string{"client csp_violation", "route=/signup/card", "directive=frame-src", "blocked=https://js.stripe.com/v3/elements-inner-card.html", "build=abc1234"}},
		{"error", "application/json",
			`{"kind":"error","route":"/study","message":"TypeError: card is undefined","sourceFile":"https://scry.example/static/stripe.js","line":12,"column":7}`,
			[]string{"client js_error", "route=/study", `message="TypeError: card is undefined"`, "source=https://scry.example/static/stripe.js:12:7", "build=abc1234"}},
		{"unhandledrejection", "application/json",
			`{"kind":"unhandledrejection","route":"/billing/card","message":"Failed to fetch"}`,
			[]string{"client unhandled_rejection", "route=/billing/card", `message="Failed to fetch"`, "build=abc1234"}},
		{"native csp-report", "application/csp-report",
			`{"csp-report":{"document-uri":"https://scry.mistystep.io/signup/card","referrer":"","violated-directive":"frame-src","effective-directive":"frame-src","original-policy":"default-src 'self'","disposition":"enforce","blocked-uri":"https://hooks.stripe.com/3d_secure","status-code":200,"script-sample":"secret page text"}}`,
			[]string{"client csp_violation", "route=/signup/card", "directive=frame-src", "blocked=https://hooks.stripe.com/3d_secure", "build=abc1234"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := len(r.captured())
			if status := r.report(c.contentType, c.body); status != http.StatusNoContent {
				t.Fatalf("status %d, want 204", status)
			}
			got := r.captured()
			if len(got) != before+1 {
				t.Fatalf("Sentry captured %d new client events, want 1: %v", len(got)-before, got)
			}
			has(t, got[len(got)-1], c.want...)
		})
	}
	// The local structured log keeps the same fields as attributes.
	has(t, r.logs.String(), `"build_sha":"abc1234"`, `"violated_directive":"frame-src"`, `"route":"/signup/card"`, `"level":"ERROR"`)
	hasNot(t, r.logs.String(), "secret page text", "default-src 'self'")
}

func TestSlice30ClientErrorStripsSecretsBeforeSentry(t *testing.T) {
	r := newCSPRig(t, true, false)
	body := `{"kind":"csp_violation","route":"https://scry.mistystep.io/signup/card?next=%2Fbilling&token=tok_routeSecret123#frag",` +
		`"violatedDirective":"img-src","blockedURI":"https://learner:hunter2@evil.example/pixel/a.png?session=sess_abcdef123456&sk=sk_live_abcdef123456#frag"}`
	if status := r.report("application/json", body); status != 204 {
		t.Fatalf("status %d", status)
	}
	native := `{"csp-report":{"document-uri":"https://scry.mistystep.io/library/items/42?q=private+notes#x","violated-directive":"script-src-elem",` +
		`"blocked-uri":"https://cdn.example/lib.js?key=pk_live_abcdefgh&email=bob@example.com"}}`
	if status := r.report("application/csp-report", native); status != 204 {
		t.Fatalf("status %d", status)
	}
	msgBody := `{"kind":"js_error","route":"/settings/tokens/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln","message":"failed for bob@example.com with token=abcdef123 at https://api.example/v1/x?secret=1 using sk_live_abcdef123456 and eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln card 4242 4242 4242 4242"}`
	if status := r.report("application/json", msgBody); status != 204 {
		t.Fatalf("status %d", status)
	}
	got := r.captured()
	if len(got) != 3 {
		t.Fatalf("captured %v", got)
	}
	has(t, got[0], "route=/signup/card ", "blocked=https://evil.example/pixel/a.png ", "directive=img-src")
	has(t, got[1], "route=/library/items/:id ", "blocked=https://cdn.example/lib.js ", "directive=script-src-elem")
	has(t, got[2], "route=/settings/tokens/:token ", "[email]", "https://api.example/v1/x", "[redacted]", "[number]")
	for _, secret := range []string{"tok_routeSecret123", "next=", "#frag", "learner", "hunter2", "sess_abcdef", "sk_live", "pk_live",
		"bob@example.com", "private", "secret=1", "abcdef123", "eyJ", "4242 4242", "?"} {
		if strings.Contains(strings.Join(got, "\n"), secret) {
			t.Errorf("Sentry message carries %q:\n%s", secret, strings.Join(got, "\n"))
		}
		if secret != "?" && strings.Contains(r.logs.String(), secret) {
			t.Errorf("log carries %q", secret)
		}
	}
}

func TestSlice30ClientErrorLimits(t *testing.T) {
	r := newCSPRig(t, false, false)
	big := `{"kind":"js_error","message":"` + strings.Repeat("x", 9000) + `"}`
	if status := r.report("application/json", big); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413", status)
	}
	// Without a Content-Length the limit still holds while reading.
	req, _ := http.NewRequest(http.MethodPost, r.srv.URL+"/api/client-error", io.MultiReader(strings.NewReader(big)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked oversized body = %d, want 413", resp.StatusCode)
	}
	for _, c := range []struct {
		contentType, body string
		want              int
	}{
		{"application/json", `{not json`, 400},
		{"application/json", `{"route":"/"}`, 400},
		{"application/json", `{"kind":"telepathy","message":"x"}`, 400},
		{"application/json", `{"kind":"csp_violation","route":"/"}`, 400},
		{"application/json", `{"kind":"js_error","route":"/"}`, 400},
		{"application/csp-report", `{"other":{}}`, 400},
		{"text/plain", `{"kind":"js_error","message":"x"}`, 415},
	} {
		if status := r.report(c.contentType, c.body); status != c.want {
			t.Errorf("%s %s = %d, want %d", c.contentType, c.body, status, c.want)
		}
	}
	if len(r.captured()) != 0 {
		t.Fatalf("refused reports reached Sentry: %v", r.captured())
	}
	if resp, err := http.Get(r.srv.URL + "/api/client-error"); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET = %v %v", resp.StatusCode, err)
	}

	// The window: 20 reports per address, then 429 until it passes.
	r2 := newCSPRig(t, false, false)
	accepted := 0
	for i := 0; i < 25; i++ {
		body := `{"kind":"js_error","route":"/study","message":"boom ` + strings.Repeat("i", i+1) + `"}`
		switch status := r2.report("application/json", body); status {
		case 204:
			accepted++
		case 429:
		default:
			t.Fatalf("report %d = %d", i, status)
		}
	}
	if accepted != 20 {
		t.Fatalf("accepted %d reports in one window, want 20", accepted)
	}
	if status := r2.report("application/json", `{"kind":"js_error","message":"late"}`); status != http.StatusTooManyRequests {
		t.Fatalf("over the limit = %d, want 429", status)
	}
	if n := len(r2.captured()); n != 20 {
		t.Fatalf("Sentry saw %d, want 20: rate-limited reports must not be forwarded", n)
	}
	r2.advance(61 * time.Second)
	if status := r2.report("application/json", `{"kind":"js_error","message":"next window"}`); status != 204 {
		t.Fatalf("after the window = %d, want 204", status)
	}
}

func TestSlice30ClientErrorDedupesAndKeepsExtensionsLocal(t *testing.T) {
	r := newCSPRig(t, true, false)
	same := `{"kind":"csp_violation","route":"/signup/card","violatedDirective":"frame-src","blockedURI":"https://js.stripe.com/v3/x.html?a=1"}`
	nativeSame := `{"csp-report":{"document-uri":"http://127.0.0.1/signup/card","violated-directive":"frame-src","blocked-uri":"https://js.stripe.com/v3/x.html?b=2"}}`
	for _, b := range []struct{ ct, body string }{{"application/json", same}, {"application/json", same}, {"application/csp-report", nativeSame}} {
		if status := r.report(b.ct, b.body); status != 204 {
			t.Fatalf("status %d", status)
		}
	}
	if got := r.captured(); len(got) != 1 {
		t.Fatalf("one violation reported natively and by app.js must page once, got %v", got)
	}
	ext := `{"kind":"csp_violation","route":"/","violatedDirective":"script-src","blockedURI":"chrome-extension://abcdefghijklmnop/inject.js"}`
	if status := r.report("application/json", ext); status != 204 {
		t.Fatalf("status %d", status)
	}
	if got := r.captured(); len(got) != 1 {
		t.Fatalf("extension noise reached Sentry: %v", got)
	}
	has(t, r.logs.String(), "blocked=extension", `"forwarded":false`)
	r.advance(11 * time.Minute)
	if status := r.report("application/json", same); status != 204 || len(r.captured()) != 2 {
		t.Fatalf("a repeat after the window must page again: %d %v", status, r.captured())
	}
}

// A signed-in browser posts telemetry without a CSRF token: the endpoint
// is exempt, and a CSP report (which browsers send without one) works too.
func TestSlice30ClientErrorIsCSRFExempt(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("telemetry@example.com")
	req, _ := http.NewRequest(http.MethodPost, w.srv.URL+"/api/client-error",
		strings.NewReader(`{"kind":"js_error","route":"/study","message":"boom"}`))
	req.Header.Set("Content-Type", "application/json")
	if status, _ := b.do(req); status != http.StatusNoContent {
		t.Fatalf("signed-in report without CSRF = %d, want 204", status)
	}
	has(t, w.logs.String(), "client js_error route=/study")
}

// stripe.js must say so, and report it, when Stripe.js is missing or the
// card field fails to mount, instead of leaving an empty box.
func TestSlice30StripeJSFailsLoudly(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is needed for the stripe.js stand-in")
	}
	cmd := exec.Command("node", "-e", `
const vm = require("node:vm"), fs = require("node:fs"), assert = require("node:assert/strict");
const source = fs.readFileSync("static/stripe.js", "utf8");
function run(stripe) {
  const reports = [], handlers = {}, submits = [], timers = [];
  const error = {textContent: ""}, hidden = {value: ""};
  const mount = {dataset: {key: "pk_test_x"}, mounted: false};
  const form = {addEventListener(ev, fn) { submits.push(fn); }, querySelector() { return {disabled: false}; }, submit() {}};
  const document = {
    getElementById(id) { return id === "card-element" ? mount : id === "card-error" ? error : id === "payment_method" ? hidden : null; },
    querySelector() { return form; }
  };
  const window = {scryReport(kind, message) { reports.push([kind, message]); }};
  if (stripe) window.Stripe = stripe(handlers, mount);
  vm.runInNewContext(source, {window, document, setTimeout(fn) { timers.push(fn); return 1; }, clearTimeout() {}});
  const submit = () => { let prevented = false; submits.forEach(fn => fn({preventDefault() { prevented = true; }})); return prevented; };
  return {reports, error, mount, handlers, submit, timers};
}
// No window.Stripe: blocked or failed to load.
let r = run(null);
assert.match(r.error.textContent, /card form couldn't load/);
assert.deepEqual(r.reports, [["integration_error", "stripe.js unavailable: window.Stripe is missing"]]);
assert.ok(r.submit(), "the form must not post without a card");
// mount throws.
r = run(() => () => ({elements() { return {create() { return {mount() { throw new Error("frame blocked"); }, on() {}}; }}; }}));
assert.match(r.error.textContent, /card form couldn't load/);
assert.equal(r.reports[0][1], "stripe card element failed to mount: frame blocked");
// Mounted, but the iframe never becomes ready.
r = run((handlers, mount) => () => ({elements() { return {create() { return {mount() { mount.mounted = true; }, on(ev, fn) { handlers[ev] = fn; }}; }}; }}));
assert.ok(r.mount.mounted);
assert.equal(r.error.textContent, "");
r.timers[0]();
assert.equal(r.reports[0][1], "stripe card element not ready after 15s");
// Healthy: ready fires, nothing is reported, the field accepts input.
r = run((handlers, mount) => () => ({elements() { return {create() { return {mount() { mount.mounted = true; }, on(ev, fn) { handlers[ev] = fn; }}; }}; }}));
r.handlers.ready();
r.handlers.change({error: {message: "Your card number is incomplete."}});
assert.equal(r.error.textContent, "Your card number is incomplete.");
assert.deepEqual(r.reports, []);
`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stripe.js: %v\n%s", err, out)
	}
}

// app.js reports policy violations, errors and rejections to
// /api/client-error: sanitized, de-duplicated and capped per page.
func TestSlice30AppJSReportsClientErrors(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is needed for the app.js stand-in")
	}
	cmd := exec.Command("node", "-e", `
const vm = require("node:vm"), fs = require("node:fs"), assert = require("node:assert/strict");
const source = fs.readFileSync("static/app.js", "utf8");
function run(beaconWorks) {
  const docHandlers = {}, winHandlers = {}, beacons = [], fetches = [];
  const document = {
    addEventListener(ev, fn) { docHandlers[ev] = fn; },
    querySelector() { return null; }, querySelectorAll() { return []; },
    documentElement: {classList: {add() {}}}
  };
  const window = {addEventListener(ev, fn) { winHandlers[ev] = fn; }};
  const navigator = {sendBeacon(url, blob) { beacons.push([url, blob]); return beaconWorks; }};
  const location = {pathname: "/signup/card", href: "https://scry.example/signup/card?next=%2Fbilling#x"};
  function fetch(url, opts) { fetches.push([url, opts]); return Promise.resolve(); }
  vm.runInNewContext(source, {window, document, navigator, location, fetch, URL, Blob, Intl, setTimeout, clearTimeout, history: {}});
  return {docHandlers, winHandlers, beacons, fetches, window};
}
(async () => {
  let r = run(true);
  r.docHandlers.securitypolicyviolation({violatedDirective: "frame-src", effectiveDirective: "frame-src",
    blockedURI: "https://user:pw@js.stripe.com/v3/elements-inner-card.html?key=pk_live_x#frag", sourceFile: "https://js.stripe.com/v3/?a=1", lineNumber: 1, columnNumber: 2, disposition: "enforce"});
  assert.equal(r.beacons.length, 1);
  assert.equal(r.beacons[0][0], "/api/client-error");
  assert.equal(r.beacons[0][1].type, "application/json");
  let sent = JSON.parse(await r.beacons[0][1].text());
  assert.equal(sent.kind, "csp_violation");
  assert.equal(sent.route, "/signup/card");
  assert.equal(sent.violatedDirective, "frame-src");
  assert.equal(sent.blockedURI, "https://js.stripe.com/v3/elements-inner-card.html");
  assert.equal(sent.sourceFile, "https://js.stripe.com/v3/");
  // The same violation again is not re-sent.
  r.docHandlers.securitypolicyviolation({violatedDirective: "frame-src", effectiveDirective: "frame-src",
    blockedURI: "https://js.stripe.com/v3/elements-inner-card.html?other=1", sourceFile: "https://js.stripe.com/v3/", lineNumber: 1, columnNumber: 2, disposition: "enforce"});
  assert.equal(r.beacons.length, 1);
  r.winHandlers.error({message: "Uncaught TypeError: x", filename: "https://scry.example/static/app.js?v=1", lineno: 3, colno: 4,
    error: {message: "x is not a function at https://scry.example/static/app.js?v=1"}});
  sent = JSON.parse(await r.beacons[1][1].text());
  assert.equal(sent.kind, "js_error");
  assert.equal(sent.sourceFile, "https://scry.example/static/app.js");
  assert.equal(sent.message, "x is not a function at https://scry.example/static/app.js");
  r.winHandlers.unhandledrejection({reason: new Error("Failed to fetch")});
  sent = JSON.parse(await r.beacons[2][1].text());
  assert.deepEqual([sent.kind, sent.message], ["unhandled_rejection", "Failed to fetch"]);
  // A script that failed to load (capture phase, target is the element).
  r.winHandlers.error({target: {tagName: "SCRIPT", src: "https://js.stripe.com/v3/?x=1"}});
  // A load error for something the policy already reported is not sent twice.
  r.winHandlers.error({target: {tagName: "IFRAME", src: "https://js.stripe.com/v3/elements-inner-card.html"}});
  await new Promise(done => setTimeout(done, 150));
  assert.equal(r.beacons.length, 4);
  sent = JSON.parse(await r.beacons[3][1].text());
  assert.deepEqual([sent.kind, sent.message], ["resource_error", "failed to load script https://js.stripe.com/v3/"]);
  // Cross-origin "Script error." carries nothing useful.
  r.winHandlers.error({message: "Script error.", filename: ""});
  assert.equal(r.beacons.length, 4);
  r.window.scryReport("integration_error", "stripe.js unavailable");
  assert.equal(r.beacons.length, 5);
  // Capped at five per page load.
  for (let i = 0; i < 10; i++) r.winHandlers.unhandledrejection({reason: "again " + i});
  assert.equal(r.beacons.length, 5);
  // Without sendBeacon, fetch with keepalive.
  r = run(false);
  r.winHandlers.unhandledrejection({reason: "nope"});
  assert.equal(r.fetches.length, 1);
  assert.equal(r.fetches[0][0], "/api/client-error");
  assert.equal(r.fetches[0][1].keepalive, true);
  assert.equal(JSON.parse(r.fetches[0][1].body).message, "nope");
})().catch(err => { console.error(err); process.exit(1); });
`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("app.js: %v\n%s", err, out)
	}
}
