package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"scry/internal/ai"
	"scry/internal/auth"
	"scry/internal/srs"
	"scry/internal/starter"
)

// Slice 54 (Active Folio 8bf4475e): Scry's brand mark, the landing page's
// Study Specimen, and the craft pass over Study, Review, Create, Library,
// Inbox, Settings and phones.

// PRODUCT.md: "As a newcomer, I can understand at a glance that Scry is an
// AI-judged active-recall instrument, see one real example of a card being
// judged on the landing page, sign up ... I can also add Scry to my phone's
// home screen to launch directly into daily recall."
func TestStory_Newcomer_SeesAJudgedCardAtAGlanceAndCanAddScryToTheHomeScreen(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	home := b.mustGet("/")

	// At a glance: the headline, a lede naming active recall and spaced
	// repetition, one filled accent action beside a quiet secondary Sign in,
	// and the trial terms with "$5 a month" kept unbroken.
	has(t, home, `<h1 class="display">Keep what's worth knowing.</h1>`, "active recall", "spaced repetition",
		`<a class="btn btn-accent" href="/signup">Start the 7-day trial</a>`, `<a class="btn" href="/signin">Sign in</a>`,
		`then <span class="nowrap">$5 a month</span> if you keep it`)
	if n := strings.Count(home, "btn-accent"); n != 1 {
		t.Errorf("the landing page has %d filled accent controls, want 1", n)
	}

	// The Study Specimen: one real review moment, a real starter card judged
	// with the plain sentence for the grader's criterion (Slice 70: never
	// the rubric wording) and its schedule.
	spec := home[strings.Index(home, `<figure class="specimen"`):]
	spec = spec[:strings.Index(spec, "</figure>")]
	q := "Which razor says never to attribute to malice what is adequately explained by carelessness or stupidity?"
	has(t, spec, q, "Your answer", "hanlons razor", "Correct answer", "Hanlon's razor",
		`verdict-badge verdict-right`, ">Correct</span>", ai.PlainWhy(srs.Right), "Next review", "Interval", "Stability", "Difficulty")
	real := false
	for _, set := range starter.All() {
		for _, idea := range set.Ideas {
			for _, it := range idea.Items {
				if it.Q == q && it.A == "Hanlon's razor" && idea.Title == "The razors" {
					real = true
				}
			}
		}
	}
	if !real {
		t.Error("the specimen's card is not a real starter card")
	}

	// Four ruled rows; private ownership names scripts or tools, not agents.
	rows := home[strings.Index(home, `<section class="plain-list"`):]
	if n := strings.Count(rows, `class="plain-row"`); n != 4 {
		t.Errorf("%d ruled rows below the hero, want 4", n)
	}
	has(t, rows, "AI grader", "word for word", "Thirty starter sets", "book", "scripts or tools")
	hasNot(t, rows, "agents")
	// Public pages end with a quiet hairline footer naming Misty Step and the
	// privacy commitment.
	for _, path := range []string{"/", "/signin", "/signup"} {
		has(t, b.mustGet(path), `foot-public`, "Made by Misty Step", "Your library and history are yours alone")
	}

	// Home screen: a manifest that launches into Study, with the dark-ground
	// icon, and the same icon for Apple and link previews.
	has(t, home, `<link rel="manifest" href="/static/manifest.webmanifest">`, `<link rel="apple-touch-icon" href="/static/icon-180.png">`,
		`<link rel="icon" href="/static/icon.svg" type="image/svg+xml">`, `<meta property="og:image" content="`+w.srv.URL+`/static/icon-512.png">`)
	res, err := http.Get(w.srv.URL + "/static/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("manifest Content-Type = %q", ct)
	}
	var m struct {
		Start string `json:"start_url"`
		Disp  string `json:"display"`
		Bg    string `json:"background_color"`
		Icons []struct{ Src, Sizes, Type string }
	}
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Start != "/study" || m.Disp != "standalone" || m.Bg != "#12101A" || len(m.Icons) < 2 {
		t.Errorf("manifest = %+v", m)
	}
	for _, icon := range append(m.Icons, struct{ Src, Sizes, Type string }{Src: "/static/icon-180.png", Type: "image/png"}) {
		r, err := http.Get(w.srv.URL + icon.Src)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 || len(body) == 0 || !strings.HasPrefix(r.Header.Get("Content-Type"), icon.Type) {
			t.Errorf("%s: %d %q", icon.Src, r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	// Signed in, the start URL lands on daily recall.
	b.signUp("homescreen@example.com")
	b.addSet("kitchen-numbers")
	if status, _ := b.get(m.Start); status != 200 || !strings.HasSuffix(b.path, "/study") {
		t.Errorf("start_url ended at %s (%d)", b.path, status)
	}
}

// DESIGN.md, Brand mark and wordmark: the mark sits 20 px square beside
// "scry" in the top bar, on every page, signed in or out.
func TestDesign_BrandMarkBesideTheWordmark(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	pages := []string{b.mustGet("/"), b.mustGet("/signin")}
	b.signUp("mark@example.com")
	b.addSet("kitchen-numbers")
	pages = append(pages, b.mustGet("/study"), b.mustGet("/library"))
	for _, page := range pages {
		has(t, page, `<a class="wordmark" href="/" aria-label="scry, home"><svg class="mark" viewBox="0 0 24 24" width="20" height="20"`,
			`stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"`, `<circle class="mark-focal" cx="12" cy="12" r="2.25"/>`,
			`<span class="wordmark-text">scry</span>`)
	}
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Get(w.srv.URL + "/favicon.ico")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/static/icon.svg" {
		t.Errorf("/favicon.ico = %d → %q", res.StatusCode, res.Header.Get("Location"))
	}
}

// PRODUCT.md: "As a returning learner who forgot my password, I enter my
// email on Sign in, choose "Forgot password" on the password step to reset
// it by email, and sign in with my new password." DESIGN.md: both sign-up
// pages keep "7-day trial · no card needed" above the heading.
func TestStory_ForgotPassword_EnterEmailThenChooseForgotPasswordOnThePasswordStep(t *testing.T) {
	w := newWorld(t)
	local := w.browser().mustGet("/signup")
	if i, j := strings.Index(local, "7-day trial · no card needed"), strings.Index(local, `<h1 class="display">Sign up</h1>`); i < 0 || j < 0 || i > j {
		t.Error("local sign-up: the trial line is not above the heading")
	}
	w.app.Auth = &auth.Clerk{PublishableKey: "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk", SecretKey: "stand-in-only"}
	b := w.browser()
	signin := b.mustGet("/signin")
	has(t, signin, "enter your email first, then your password", "After you enter your email, choose “Forgot password” on the password step to reset it by email.")
	hasNot(t, signin, "7-day trial · no card needed")
	signup := b.mustGet("/signup")
	if i, j := strings.Index(signup, "7-day trial · no card needed"), strings.Index(signup, `id="auth-heading"`); i < 0 || j < 0 || i > j {
		t.Error("Clerk sign-up: the trial line is not above the heading")
	}
}

// PRODUCT.md: "In Create I can ask Scry for material on any topic and get
// it ... or tap a concrete starter topic or book example right under the
// field to begin without a blank-page pause." DESIGN.md: the draft outline
// does not repeat the idea heading on every row.
func TestStory_Create_TapAStarterExampleUnderTheField(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("create@example.com")
	page := b.mustGet("/learn")
	field := strings.Index(page, `id="ask-hint"`)
	ex := strings.Index(page, `<ul class="examples"`)
	if field < 0 || ex < field {
		t.Fatal("the examples are not right under the field")
	}
	links := regexp.MustCompile(`<a class="example" href="/learn\?q=([^"]+)" data-example="([^"]+)">`).FindAllStringSubmatch(page, -1)
	if len(links) != 3 {
		t.Fatalf("%d examples, want a book, a mental model and a technical topic", len(links))
	}
	has(t, page, `data-example="Meditations by Marcus Aurelius"`, `data-example="Second-order thinking"`)
	// Without script an example opens Create with the field filled.
	filled := b.mustGet("/learn?q=" + links[0][1])
	has(t, filled, `value="Meditations by Marcus Aurelius"`)
	// The book example drafts its core ideas; rows do not repeat the idea.
	b.post("/learn", "q", "Meditations by Marcus Aurelius")
	w.app.WaitDrafts()
	draft := b.mustGet(b.path)
	has(t, draft, "The dichotomy of control", "8 items in 4 ideas")
	hasNot(t, draft, "sug-idea", "new idea")
	// The inbox, where suggestions come from outside, still names the idea.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "initExamples", `input.value = a.getAttribute("data-example")`)
}

// PRODUCT.md: "... Scry never skips ahead on its own, and the review always
// stays framed at the question and judgment—never scrolling the question
// out of view." DESIGN.md: focusing "Next card" with preventScroll.
func TestStory_Review_StaysFramedAtTheQuestionAndJudgment(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "function frameReview()", "next.focus({ preventScroll: true });", "if (frameReview()) return;",
		`$("[data-next][data-autofocus-no-scroll]")`)
	// Without script nothing may scroll the review: the template carries no
	// HTML autofocus on Next card (checked on a rendered review in
	// TestSlice41_Review_EveryAnswerStopsOnTheReviewWithItsFacts).
	tpl, err := os.ReadFile("templates/study.html")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`<button[^>]*\sautofocus[\s>][^>]*data-next`).Match(tpl) || regexp.MustCompile(`<button[^>]*data-next[^>]*\sautofocus[\s>]`).Match(tpl) {
		t.Error("study.html: Next card still carries HTML autofocus")
	}
	if strings.Contains(js, "next.focus();") || strings.Contains(js, "if (next) next.focus()\n") {
		t.Error("app.js still focuses Next card with a scroll")
	}
	css, _ := staticAsset(t, w, "/static/app.css")
	// The explanation is set off from the criterion line by a 2 px rule.
	has(t, css, "border-left: 2px solid var(--line); padding-left: var(--s3); }")
}

// PRODUCT.md: "On phones, opening an idea shows that idea and its cards
// immediately above a compact collapsible outline." DESIGN.md: on phones
// the active idea renders first beneath a compact collapsible idea picker.
func TestStory_Library_OnPhonesTheIdeaComesFirstWithACollapsibleOutline(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("phone@example.com")
	b.addSet("nato-alphabet")
	lib := b.mustGet("/library")
	ideaURL := regexp.MustCompile(`href="(/library/ideas/\d+)"`).FindAllStringSubmatch(lib, -1)
	if len(ideaURL) < 2 {
		t.Fatal("no ideas")
	}
	page := b.mustGet(ideaURL[1][1])
	picker := strings.Index(page, `<details class="idea-picker" data-idea-picker>`)
	idea := strings.Index(page, `<article class="pane pane-idea">`)
	if picker < 0 || idea < picker {
		t.Fatal("the idea picker is missing or below the idea")
	}
	body := page[picker : strings.Index(page[picker:], "</details>")+picker]
	// Closed by default (compact), naming the open idea; the outline is inside.
	has(t, body, `<span class="picker-current"><span class="outline-num">1.1</span>`, `<ol class="outline">`, `aria-current="true"`)
	css, _ := staticAsset(t, w, "/static/app.css")
	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	has(t, css, ".idea-picker { display: none; }", "font-variant-numeric: tabular-nums; color: var(--ink-muted); }")
	has(t, phone, ".idea-picker { display: block;", ".pane-outline { display: none; }")
	// "/" on a phone opens the picker and focuses its search, since the
	// outline's own search is hidden there.
	has(t, body, `id="q-picker" name="q" type="search"`, "data-picker-search>")
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `search.offsetParent === null && picker && picker.offsetParent !== null`, `search = $("[data-picker-search]", picker);`, "picker.open = true;")
}

// DESIGN.md, Inbox: the purpose line is always on its own row beneath the
// title, including when the inbox is empty.
func TestDesign_InboxPurposeLineOnItsOwnRowEvenWhenEmpty(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("empty@example.com")
	page := b.mustGet("/inbox")
	has(t, page, "Nothing waiting.", `<p class="purpose list-purpose">Cards suggested by external scripts or tools wait here until you approve them.</p>`)
	css, _ := staticAsset(t, w, "/static/app.css")
	// The 100% basis must not be capped by .purpose's max-width (the measure),
	// or the line fits beside the title at 1440 px (seen on the real path).
	has(t, css, ".list-purpose { order: 1; flex-basis: 100%; max-width: none; }")
}

// DESIGN.md, Settings: choosing a mode applies at once as well as saving;
// section buttons are content-sized on desktop.
func TestDesign_SettingsAppearanceAppliesAtOnceAndButtonsAreContentSized(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("settings@example.com")
	page := b.mustGet("/settings")
	has(t, page, `<form method="post" action="/settings/theme" class="form" data-theme-form>`)
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "function initTheme()", `doc.documentElement.setAttribute("data-theme", radio.value)`, `doc.documentElement.removeAttribute("data-theme")`)
	// Saves are serialized: one POST in flight, later changes coalesce into a
	// single follow-up that reads the form (the latest choice) when it is sent.
	has(t, js, "if (saving) { pending = true; return; }", "if (pending) saveTheme();")
	// A follow-up still waiting when the page is left or hidden is sent at
	// once as a keepalive request, exactly once.
	has(t, js, "keepalive: true, credentials: \"same-origin\"", "pending = false; // sent once",
		`window.addEventListener("pagehide", function () { if (themeFlush) themeFlush(); });`,
		`if (doc.visibilityState === "hidden" && themeFlush) themeFlush();`)
	// The keepalive body is the form, url-encoded, as the handler reads it.
	if st, _ := b.post("/settings/theme", "theme", "light"); st != http.StatusOK {
		t.Fatalf("url-encoded theme POST = %d", st)
	}
	if page := b.mustGet("/settings"); !strings.Contains(page, `value="light" checked`) {
		t.Error("an url-encoded theme POST did not save")
	}
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, css, ".setting .form > .btn { justify-self: start; }")
}

// DESIGN.md: inputs keep a line hairline at rest; hover fills only on
// hover-capable pointers; the active nav underlines its label only; the
// study margin is 196 px and top-aligned; phones get a 56 px brand row, one
// compact navigation row (Slice 70: all six destinations, no scrolling) and
// safe-area clearance.
func TestDesign_Slice54_CraftRulesHoldInTheStyleSheet(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	// A long typed answer never widens the page: the caret's measuring ruler
	// is clipped inside the answer line.
	has(t, css, "overflow: hidden;\n  border-bottom: 1px solid var(--ink-ghost);")
	has(t, css,
		"border: 1px solid var(--line); border-radius: var(--radius-control);",
		".input:focus { border-color: var(--ink);",
		"@media (hover: hover) {",
		".nav a[aria-current] .nav-label { text-decoration: underline;",
		"grid-template-columns: 196px minmax(0, 1fr); align-items: start;",
		".session-margin { display: flex; flex-direction: column; gap: var(--s1); align-self: start;",
		"grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);",
		".specimen-frame::before", `mask: url("/static/grid.svg")`,
		".mark { width: 20px; height: 20px;", ".mark-focal { fill: var(--accent); stroke: none; }",
		".wordmark { gap: var(--s2); }",
		".example {", "border: 1px solid var(--line); border-radius: var(--radius-control);",
	)
	hover := css[strings.Index(css, "@media (hover: hover) {"):]
	hover = hover[:strings.Index(hover, "\n}\n")]
	outside := strings.Replace(css, hover, "", 1)
	if m := regexp.MustCompile(`[^\n]*:hover[^\n]*`).FindString(outside); m != "" {
		t.Errorf("a hover rule applies on touch screens: %q", m)
	}
	for _, sel := range []string{".btn:hover", ".tile:hover", ".outline-row:hover", ".sug:hover", ".tile-add:hover"} {
		if !strings.Contains(hover, sel) {
			t.Errorf("hover block is missing %s", sel)
		}
	}
	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	phone = phone[:strings.Index(phone, "\n}\n")]
	// Slice 70: signed in, one compact 52 px row holds all six destinations
	// (no sideways scroll), and the session clears the pinned bar.
	has(t, phone, "min-height: 56px;", ".top-app .nav {", "grid-template-columns: repeat(6, minmax(0, 1fr));", "height: 52px;",
		"calc(var(--s3) + env(safe-area-inset-bottom, 0px))", "body.in-session { padding-bottom: calc(var(--touch) + 2 * var(--s3) + 1px + var(--s4) + env(safe-area-inset-bottom, 0px)); }",
		".ask-input { font-size: min(var(--display), 6.4vw); }")
	page := w.browser().mustGet("/")
	has(t, page, `content="width=device-width, initial-scale=1, viewport-fit=cover"`)
}

// DESIGN.md, Sign-up and trial: embedded Clerk panels inherit Scry's
// tokens with no shadows or gradients.
func TestDesign_ClerkPanelsInheritScrysTokens(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/clerk.js")
	has(t, js, "appearance:appearance()", `colorBackground: raised`, `colorInputBackground: page`, `borderRadius: "4px"`,
		`fontSize: "16px"`, `boxShadow: "none"`, `backgroundImage: "none"`, `border: "1px solid " + line, borderRadius: "0"`, `token("raised"`)
	hasNot(t, js, `colorPrimary:"#0562EF"`)
}

func staticAsset(t *testing.T, w *world, path string) (string, string) {
	t.Helper()
	res, err := http.Get(w.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("GET %s = %d", path, res.StatusCode)
	}
	return string(b), res.Header.Get("Content-Type")
}
