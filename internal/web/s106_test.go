package web_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"scry/internal/app"
)

// noFollow is a client that reports redirects instead of following them.
var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// api calls Scry's JSON API with a bearer token.
func (w *world) api(method, path, bearer, body string) (int, map[string]any) {
	w.t.Helper()
	req, _ := http.NewRequest(method, w.srv.URL+path, bytes.NewBufferString(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

var tokenRE = regexp.MustCompile(`scry_[A-Za-z0-9_-]{20,}`)

// addExactText puts a text learned word for word, one card per line, in a
// learner's library. There is no hand-written form for it any more
// (S106-2); exact texts come from starter sets, imports and the API, so
// tests that need one make it directly.
func (w *world) addExactText(email, title, text string) int64 {
	w.t.Helper()
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	id, err := w.lib(email).AddExactText(ctx, 0, title, lines, w.now())
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

// S106-4: /inbox answers 303 to Library, for any method and below it, and
// so does every other Inbox address.
func TestS106_4_InboxRedirects303ToLibrary(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("inbox@example.com")
	b.addSet("kitchen-numbers")
	for _, c := range []struct{ method, path string }{
		{"GET", "/inbox"}, {"HEAD", "/inbox"}, {"POST", "/inbox/approve-all"}, {"GET", "/inbox/anything"},
	} {
		req, _ := http.NewRequest(c.method, w.srv.URL+c.path, nil)
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/library" {
			t.Errorf("%s %s = %d to %q, want 303 to /library", c.method, c.path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// Followed by a signed-in browser, it lands on Library.
	page := b.mustGet("/inbox")
	if b.path != "/library/ideas/1" && b.path != "/library" && !strings.HasPrefix(b.path, "/library") {
		t.Errorf("/inbox ended at %s", b.path)
	}
	has(t, page, "Every card you have, grouped by idea.")
}

// S106-4 and the phone story: the navigation holds five destinations —
// Study, Library, Create, Billing, Settings — with no Inbox in it, in the
// headers, the key hints or the keys themselves, and on phones it is five
// equal columns.
func TestS106_4_NavHasFiveDestinations(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("nav@example.com")
	b.addSet("kitchen-numbers")
	navRE := regexp.MustCompile(`(?s)<nav class="nav" aria-label="Main">(.*?)</nav>`)
	labelRE := regexp.MustCompile(`<span class="nav-label">([^<]+)</span>`)
	for _, path := range []string{"/study", "/library", "/learn", "/billing", "/settings", "/shelf"} {
		page := b.mustGet(path)
		nav := navRE.FindStringSubmatch(page)
		if nav == nil {
			t.Fatalf("%s has no main navigation", path)
		}
		var labels []string
		for _, m := range labelRE.FindAllStringSubmatch(nav[1], -1) {
			labels = append(labels, m[1])
		}
		if got := strings.Join(labels, ","); got != "Study,Library,Create,Billing,Settings" {
			t.Errorf("%s navigation = %s", path, got)
		}
		if strings.Count(nav[1], "<a ") != 5 {
			t.Errorf("%s navigation has %d links, want 5", path, strings.Count(nav[1], "<a "))
		}
		hasNot(t, page, `href="/inbox"`, "icon-inbox", `class="count"`, "g i inbox")
		has(t, page, "g s study · g l library · g n create · g b billing")
		if strings.Contains(strings.ToLower(visibleText(page)), "inbox") {
			t.Errorf("%s still says Inbox", path)
		}
	}
	css, _ := os.ReadFile("static/app.css")
	if !strings.Contains(string(css), "grid-template-columns: repeat(5, minmax(0, 1fr))") || strings.Contains(string(css), "repeat(6, minmax(0, 1fr))") {
		t.Error("the phone navigation is not five equal columns")
	}
	js, _ := os.ReadFile("static/app.js")
	if strings.Contains(string(js), `"/inbox"`) || !strings.Contains(string(js), `var go = { s: "/study", l: "/library", n: "/learn", b: "/billing" };`) {
		t.Error("the g-keys still reach an Inbox")
	}
}

// S106-4: every token may add. A token made in Settings, and one made
// earlier without that permission (can_add = 0), both put cards straight
// into the library through POST /api/v1/items and its alias POST
// /api/v1/suggestions; GET /api/v1/ideas lists the ideas. Settings shows
// a curl example for POST /api/v1/items with the new token once, and only
// a placeholder otherwise.
func TestS106_4_EveryTokenAddsToLibrary(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("owner@example.com")
	lib := w.lib("owner@example.com")

	settings := b.mustGet("/settings")
	has(t, settings, `<h2 class="title">API Tokens</h2>`,
		"Programmatic access to your account. A token lets your own scripts or tools add cards to your library through the Scry API.",
		"Token name", "Create token", "curl -X POST "+w.srv.URL+"/api/v1/items", `Authorization: Bearer scry_…"`)
	hasNot(t, settings, "can_add", "nbox", "suggest", "May add straight")

	_, page := b.post("/settings/tokens", "name", "reading agent", "can_add", "0")
	token := tokenRE.FindString(page)
	if token == "" {
		t.Fatal("no token on the page")
	}
	has(t, page, "Token for reading agent.", "curl -X POST "+w.srv.URL+"/api/v1/items", `-H "Authorization: Bearer `+token+`"`)
	if strings.Count(page, token) != 2 { // the token line and its curl example
		t.Errorf("the new token appears %d times", strings.Count(page, token))
	}
	// Never shown again: Settings prints the placeholder, not the token.
	again := b.mustGet("/settings")
	hasNot(t, again, token)
	has(t, again, `Bearer scry_…"`, "reading agent", "never used")

	// A token made before every token could add: can_add = 0.
	_, page = b.post("/settings/tokens", "name", "old script")
	old := tokenRE.FindString(page)
	if _, err := w.store.DB().Exec(`UPDATE agent_tokens SET can_add = 0 WHERE name = 'old script'`); err != nil {
		t.Fatal(err)
	}

	payload := `{"source": "The Federalist Papers", "items": [
		{"question": "Which Federalist paper is about faction?", "answer": "Federalist No. 10", "idea": "Faction", "summary": "Groups against the common good."},
		{"question": "Who wrote Federalist No. 10?", "answer": "James Madison", "idea": "Faction"}]}`
	total := 0
	for _, c := range []struct{ name, bearer, path string }{
		{"new token, items", token, "/api/v1/items"},
		{"new token, suggestions alias", token, "/api/v1/suggestions"},
		{"old can_add=0 token, items", old, "/api/v1/items"},
		{"old can_add=0 token, suggestions alias", old, "/api/v1/suggestions"},
	} {
		status, out := w.api("POST", c.path, c.bearer, payload)
		if status != http.StatusCreated || out["added"] != float64(2) || out["status"] != "in the library" || len(out["ids"].([]any)) != 2 {
			t.Fatalf("%s: %d %v", c.name, status, out)
		}
		total += 2
		if n, _, _ := lib.Counts(ctx, w.now()); n != total {
			t.Fatalf("%s: library has %d items, want %d", c.name, n, total)
		}
	}
	// Nothing waits anywhere; the cards carry their source.
	if left, _ := lib.Suggestions(ctx, 0); len(left) != 0 {
		t.Errorf("%d cards wait for approval", len(left))
	}
	items, _ := lib.Search(ctx, "Federalist")
	if len(items) != 8 || items[0].IdeaTitle != "Faction" || items[0].Source != "from reading agent · The Federalist Papers" {
		t.Fatalf("added items = %+v", items)
	}
	has(t, b.mustGet("/library"), "Faction", "Which Federalist paper is about faction?")

	// exact and summary are honoured.
	status, out := w.api("POST", "/api/v1/items", old, `{"items": [{"question": "Recite the motto", "answer": "Know thyself", "idea": "Mottos", "summary": "Short lines.", "exact": true}]}`)
	if status != http.StatusCreated {
		t.Fatalf("exact item: %d %v", status, out)
	}
	if motto, _ := lib.Search(ctx, "motto"); len(motto) != 1 || !motto[0].Exact {
		t.Errorf("exact item = %+v", motto)
	}

	// The ideas, to file things well.
	status, out = w.api("GET", "/api/v1/ideas", old, "")
	if status != 200 {
		t.Fatalf("ideas: %d %v", status, out)
	}
	var titles []string
	for _, i := range out["ideas"].([]any) {
		titles = append(titles, i.(map[string]any)["title"].(string))
	}
	if got := strings.Join(titles, ","); got != "Faction,Mottos" {
		t.Errorf("ideas = %s", got)
	}

	// No token, a wrong token, or a revoked token gets nothing.
	for _, bearer := range []string{"", "scry_not_a_real_token_at_all_0000"} {
		if status, _ := w.api("POST", "/api/v1/items", bearer, payload); status != http.StatusUnauthorized {
			t.Errorf("token %q: %d", bearer, status)
		}
	}
	tokens, _ := lib.Tokens(ctx)
	b.get("/settings")
	b.post(fmt.Sprintf("/settings/tokens/%d/revoke", tokens[0].ID))
	for _, path := range []string{"/api/v1/items", "/api/v1/suggestions", "/api/v1/ideas"} {
		method := "POST"
		if path == "/api/v1/ideas" {
			method = "GET"
		}
		if status, _ := w.api(method, path, token, payload); status != http.StatusUnauthorized {
			t.Errorf("revoked token on %s: %d", path, status)
		}
	}
	// Malformed material is refused with a reason, and adds nothing.
	n0, _, _ := lib.Counts(ctx, w.now())
	if status, out := w.api("POST", "/api/v1/suggestions", old, `{"items": [{"question": "No answer?", "idea": "X"}]}`); status != http.StatusUnprocessableEntity || out["error"] != "item 1 needs an answer" {
		t.Errorf("bad item: %d %v", status, out)
	}
	if n1, _, _ := lib.Counts(ctx, w.now()); n1 != n0 {
		t.Errorf("a refused request added %d items", n1-n0)
	}
}

// S106-4: one request carries 1 to 50 cards; none or more than 50 is
// refused (422) in words, and nothing is added.
func TestS106_4_ItemsBatchLimits1To50(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("batch@example.com")
	lib := w.lib("batch@example.com")
	token, err := w.app.CreateAgentToken(ctx, w.user("batch@example.com").ID, "batch script")
	if err != nil {
		t.Fatal(err)
	}
	batch := func(n int) string {
		var items []string
		for i := 1; i <= n; i++ {
			items = append(items, fmt.Sprintf(`{"question": "What is card %d?", "answer": "Card %d", "idea": "Batch"}`, i, i))
		}
		return `{"source": "batch", "items": [` + strings.Join(items, ",") + `]}`
	}
	for _, path := range []string{"/api/v1/items", "/api/v1/suggestions"} {
		for _, c := range []struct {
			n      int
			status int
			err    string
		}{
			{0, http.StatusUnprocessableEntity, "send at least one item: 1 to 50 per request"},
			{51, http.StatusUnprocessableEntity, "send at most 50 items per request; this one had 51"},
			{1, http.StatusCreated, ""},
			{50, http.StatusCreated, ""},
		} {
			before, _, _ := lib.Counts(ctx, w.now())
			status, out := w.api("POST", path, token, batch(c.n))
			after, _, _ := lib.Counts(ctx, w.now())
			if status != c.status {
				t.Errorf("%s with %d items: %d %v", path, c.n, status, out)
				continue
			}
			if c.err != "" {
				if out["error"] != c.err || after != before {
					t.Errorf("%s with %d items: error %v, added %d", path, c.n, out["error"], after-before)
				}
				continue
			}
			if out["added"] != float64(c.n) || after != before+c.n {
				t.Errorf("%s with %d items: added %v, library %d -> %d", path, c.n, out["added"], before, after)
			}
		}
	}
	// Not JSON at all is a 400 with the shape to send.
	if status, out := w.api("POST", "/api/v1/items", token, `items: 3`); status != http.StatusBadRequest || !strings.Contains(fmt.Sprint(out["error"]), `"items"`) {
		t.Errorf("not JSON: %d %v", status, out)
	}
}

// S106-2: Create is one multi-line field in Body type that takes up to
// 16,000 characters, with the hint, the example prompts, "Draft cards" and
// Cmd/Ctrl+Enter; over-long text is refused in words, and pasted material
// keeps its line breaks.
func TestS106_2_CreateTextareaUpTo16000Chars(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("create@example.com")
	page := b.mustGet("/learn")
	has(t, page, `<textarea class="ask-input" id="q" name="q" rows="6" placeholder="What do you want to know?"`, `maxlength="16000"`,
		"A topic, a question, a book's title, or pasted Markdown or notes", `data-example="Meditations by Marcus Aurelius"`,
		`<button class="btn btn-accent">Draft cards</button>`, "⌘/ctrl + enter to draft")
	hasNot(t, page, `<input class="ask-input`, `maxlength="300"`, "Draft it", "enter to draft</p>\n</section>\n<p")
	// The examples come before the one accent action.
	if strings.Index(page, "data-examples") > strings.Index(page, "Draft cards") {
		t.Error("the example prompts should sit between the field and Draft cards")
	}
	js, _ := os.ReadFile("static/app.js")
	for _, want := range []string{`e.key === "Enter" && (e.metaKey || e.ctrlKey)`, "requestSubmit()", "scrollHeight"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("app.js lacks %q (Cmd/Ctrl+Enter and auto-grow)", want)
		}
	}
	css, _ := os.ReadFile("static/app.css")
	if !strings.Contains(string(css), "font: 400 var(--body)/1.55 var(--text)") || !strings.Contains(string(css), "min-height: calc(6 * 1.55em") {
		t.Error("the Create field is not about six lines of Body type")
	}

	// Over 16,000 characters is refused in words, and keeps what was typed.
	status, refused := b.post("/learn", "q", strings.Repeat("a", 16001))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("16,001 characters: %d", status)
	}
	has(t, refused, "Keep it under 16,000 characters.")
	if len(w.model.drafts) != 0 {
		t.Fatal("an over-long request reached the model")
	}

	// Exactly 16,000 characters (CRLF line breaks, as a browser sends) is
	// drafted with its lines intact and a short title.
	line := "# Field notes\r\n" + strings.Repeat("The cache keeps recent answers close at hand. ", 10) + "\r\n"
	text := strings.Repeat(line, 16000/len(line)+1)
	text = text[:16000]
	b.post("/learn", "q", text)
	if !strings.HasPrefix(b.path, "/learn/") {
		t.Fatalf("16,000 characters ended at %s", b.path)
	}
	w.app.WaitDrafts()
	if len(w.model.drafts) != 1 {
		t.Fatalf("%d drafts", len(w.model.drafts))
	}
	got := w.model.drafts[0].Query
	if strings.Contains(got, "\r") || !strings.Contains(got, "\n# Field notes\n") || got != strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n")) {
		t.Errorf("the material lost its line breaks: %q", got[:120])
	}
	draft := b.mustGet(b.path)
	title := regexp.MustCompile(`<h1 class="display">([^<]*)</h1>`).FindStringSubmatch(draft)
	if title == nil || title[1] != "Field notes" {
		t.Errorf("draft title = %v, want the first heading", title)
	}
	has(t, draft, "from your notes")

	// A long, unheaded paste is named by its first line, cut to 80.
	if got := app.DraftTitle(strings.Repeat("word ", 40) + "\nmore"); len([]rune(got)) > 80 || !strings.HasSuffix(got, "…") {
		t.Errorf("DraftTitle = %q", got)
	}
	if got := app.NormalizeLearn("  Second-order   thinking \r\n"); got != "Second-order thinking" {
		t.Errorf("NormalizeLearn(topic) = %q", got)
	}
}

// pastedNote is a typical Markdown note a learner pastes into Create.
const pastedNote = `# Caching in web services

Notes from the performance reading group.

## Cache-aside
- The application reads the cache first and falls back to the database.
- Cache-aside: the application, not the cache, loads missing entries.
- Stale entries are expired with a time to live.

## Write-through
Write-through: every write goes to the cache and the database together.
It keeps reads fast at the price of slower writes.

## Eviction
- LRU evicts the least recently used entry first.
- An eviction policy decides which entry leaves a full cache.

## Thundering herd
When a popular entry expires, many requests miss at once and all hit the database.
A lock or request coalescing lets one request refill the entry.
`

// S106-2: pasted Markdown goes to the drafting agent (in test mode, its
// fixed fixture with no network call), comes back as a draft outline of
// atomic questions grouped by idea, and "Approve all" puts every card in
// the library. It is drawn from the allowance like any draft.
func TestS106_2_MarkdownDraftsFromFixtureToApproval(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("notes@example.com")
	lib := w.lib("notes@example.com")
	before, _ := lib.Subscription(ctx)

	b.get("/learn")
	b.post("/learn", "q", pastedNote)
	if !strings.HasPrefix(b.path, "/learn/") {
		t.Fatalf("Draft cards ended at %s", b.path)
	}
	w.app.WaitDrafts()
	draftPath := b.path
	page := b.mustGet(draftPath)
	has(t, page, `<h1 class="display">Caching in web services</h1>`, "from your notes", "Approve all",
		"Cache-aside", "Write-through", "Eviction", "Thundering herd",
		"In your notes on “Cache-aside”, what is Cache-aside?",
		"the application, not the cache, loads missing entries")
	hasNot(t, page, "Notes from the performance reading group.</h1>")
	if n := strings.Count(page, `class="draft-idea"`); n != 4 {
		t.Errorf("%d ideas, want 4 (one per heading)", n)
	}
	sugs, _ := lib.Suggestions(ctx, mustDraftID(t, draftPath))
	if len(sugs) < 4 {
		t.Fatalf("%d cards drafted", len(sugs))
	}
	// Nothing is in the library until approved.
	if n, _, _ := lib.Counts(ctx, w.now()); n != 0 {
		t.Fatalf("%d items before approval", n)
	}
	// Drawn from the allowance like any draft.
	after, _ := lib.Subscription(ctx)
	if after.UsedMicros <= before.UsedMicros {
		t.Errorf("the draft was not metered: %d -> %d", before.UsedMicros, after.UsedMicros)
	}
	// The same paste always drafts the same cards (a fixed fixture).
	if w.model.drafts[0].Query != strings.TrimSpace(pastedNote) {
		t.Error("the material reached the model changed")
	}

	_, done := b.post(draftPath + "/approve-all")
	has(t, done, fmt.Sprintf("All approved. %d items joined your library.", len(sugs)))
	if n, _, _ := lib.Counts(ctx, w.now()); n != len(sugs) {
		t.Errorf("library has %d items, want %d", n, len(sugs))
	}
	libPage := b.mustGet("/library")
	has(t, libPage, "Caching in web services", "Cache-aside", "Thundering herd")
	outline, _ := lib.Outline(ctx)
	if len(outline) != 5 || outline[0].Title != "Caching in web services" || outline[1].Depth != 1 {
		t.Errorf("outline = %+v", outline)
	}

	// "Ask again" on a draft that failed reopens Create with the material
	// in the field, by the draft's id rather than a 16,000-character URL.
	w.model.mu.Lock()
	w.model.err = fmt.Errorf("provider down")
	w.model.mu.Unlock()
	b.get("/learn")
	b.post("/learn", "q", pastedNote)
	w.app.WaitDrafts()
	failed := b.mustGet(b.path)
	retry := regexp.MustCompile(`href="(/learn\?draft=\d+)">Ask again`).FindStringSubmatch(failed)
	if retry == nil || strings.Contains(failed, "/learn?q=") {
		t.Fatalf("the failed draft's retry link: %v", retry)
	}
	has(t, b.mustGet(retry[1]), "## Thundering herd\nWhen a popular entry expires")
}

func mustDraftID(t *testing.T, path string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(path, "/learn/%d", &id); err != nil {
		t.Fatalf("draft path %q: %v", path, err)
	}
	return id
}

// S106-2: Library has no hand-written card forms. "Add an item" and "Add an
// exact text" are gone, their addresses send the learner to Create (303)
// and add nothing, and the outline ends with "Starter shelf · Create or
// import cards". An existing card is still edited on its own page.
func TestS106_2_NoHandWrittenCardForms(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("forms@example.com")
	b.addSet("kitchen-numbers")
	lib := w.lib("forms@example.com")
	outline, _ := lib.Outline(ctx)
	idea := outline[len(outline)-1]
	page := b.mustGet(fmt.Sprintf("/library/ideas/%d", idea.ID))
	hasNot(t, page, "Add an item", "Add an exact text", "/library/texts", "/items\" class=\"form\"", `name="prompt"`, `name="answer"`)
	end := `<p class="pane-links meta" data-outline-end><a class="link quiet" href="/shelf">Starter shelf</a> · <a class="link quiet" href="/learn#import">Create or import cards</a></p>`
	if strings.Count(page, end) != 2 {
		t.Errorf("the outline (desktop pane and phone picker) should both end with Starter shelf · Create or import cards: %d", strings.Count(page, end))
	}
	n0, _, _ := lib.Counts(ctx, w.now())
	for _, c := range []struct{ method, path string }{
		{"GET", "/library/texts/new"},
		{"POST", "/library/texts"},
		{"POST", fmt.Sprintf("/library/ideas/%d/items", idea.ID)},
	} {
		req, _ := http.NewRequest(c.method, w.srv.URL+c.path, strings.NewReader("csrf="+b.csrf+"&prompt=Q&answer=A&title=T&text=Line"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", b.cookie())
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/learn" {
			t.Errorf("%s %s = %d to %q, want 303 to /learn", c.method, c.path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	if n1, _, _ := lib.Counts(ctx, w.now()); n1 != n0 {
		t.Errorf("a retired form added %d items", n1-n0)
	}
	// An existing card is still edited on its own page.
	items, _ := lib.Items(ctx, idea.ID)
	item := b.mustGet(fmt.Sprintf("/library/items/%d", items[0].ID))
	has(t, item, `name="prompt"`, `name="answer"`)
	b.post(fmt.Sprintf("/library/items/%d", items[0].ID), "prompt", "Edited question?", "answer", "Edited answer")
	if it, _ := lib.Item(ctx, items[0].ID); it.Prompt != "Edited question?" {
		t.Errorf("the card's own page no longer edits it: %+v", it)
	}
	// Create never asks for a front and back.
	create := b.mustGet("/learn")
	hasNot(t, create, `name="prompt"`, `name="answer"`, "Question</label>", "Answer</label>")
}
