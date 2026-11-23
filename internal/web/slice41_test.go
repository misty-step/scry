package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/srs"
	"scry/internal/starter"
	"scry/internal/store"
)

// Slice 41: the ten dogfooding fixes. Each test names the item it pins.

// postJSON posts a form the way the page's own script does: it asks for
// JSON and follows no redirect.
func (b *browser) postJSON(path string, kv ...string) (int, map[string]any, string) {
	b.t.Helper()
	form := url.Values{"csrf": {b.csrf}}
	for i := 0; i+1 < len(kv); i += 2 {
		form.Set(kv[i], kv[i+1])
	}
	req, _ := http.NewRequest(http.MethodPost, b.w.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Jar: b.c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp.Header.Get("Content-Type")
}

var (
	railRE       = regexp.MustCompile(`(?s)<aside class="session-margin.*?</aside>`)
	headerRE     = regexp.MustCompile(`data-study-header>([^<]*)<`)
	upcomingRE   = regexp.MustCompile(`(?s)<ol class="upcoming".*?</ol>`)
	tagRE        = regexp.MustCompile(`<[^>]+>`)
	canonicalRE  = regexp.MustCompile(`(?s)data-canonical>(.*?)<`)
	submittedRE  = regexp.MustCompile(`(?s)data-submitted>(.*?)<`)
	nextReviewRE = regexp.MustCompile(`(?s)data-next-review><time datetime="([0-9-]+)">([^<]+)</time> · ([^<]+)<`)
)

func studyHeader(t *testing.T, body string) string {
	t.Helper()
	m := headerRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the study page has no place line")
	}
	return m[1]
}

// visibleText is the page's main content as a person reads it.
func visibleText(body string) string {
	if i := strings.Index(body, `<main`); i >= 0 {
		body = body[i:]
	}
	if i := strings.Index(body, `</main>`); i >= 0 {
		body = body[:i]
	}
	return tagRE.ReplaceAllString(body, " ")
}

// showsWord reports whether text holds word as a whole word or phrase,
// whatever its case ("alphabet" does not show "Alpha").
func showsWord(text, word string) bool {
	return regexp.MustCompile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(word) + `($|[^\pL\pN])`).MatchString(text)
}

// Item 1: while a card waits for its answer, nothing on the Study screen
// names the answer to it or to the cards beside it.
func TestSlice41_Spoiler_StudyScreenShowsNoAnswerBeforeItIsGiven(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("spoiler@example.com")
	b.addSet("nato-alphabet")
	set, _ := starter.Get("nato-alphabet")
	var answers []string
	for _, idea := range set.Ideas {
		for _, it := range idea.Items {
			answers = append(answers, it.A)
		}
	}
	if len(answers) != 26 {
		t.Fatalf("%d NATO answers", len(answers))
	}

	body := b.mustGet("/study")
	if got := studyHeader(t, body); got != "NATO phonetic alphabet · Set 1" {
		t.Fatalf("place line = %q, want the topic and a neutral set number", got)
	}
	rail := railRE.FindString(body)
	has(t, rail, "Study", "Recall what's due today", "1 / ")
	text := visibleText(body)
	for _, a := range append(answers, "Alpha", "Letters A–I", "Alfa to India") {
		if showsWord(text, a) {
			t.Errorf("the study page shows %q before the card is answered", a)
		}
		if showsWord(tagRE.ReplaceAllString(rail, " "), a) {
			t.Errorf("the study rail shows %q before the card is answered", a)
		}
	}
	hasNot(t, body, "Alfa", "India", "Alpha") // not even in markup or attributes

	// Once the answer is in, the review names the real place.
	first, _ := b.current(body)
	review := b.answer(first, "Alfa")
	if got := studyHeader(t, review); got != "NATO phonetic alphabet · Letters A–I" {
		t.Errorf("place line in the review = %q", got)
	}
	// The next card is spoiler-safe again, and still no sibling answer shows
	// (the card just answered may show: it is in the scrollback by its cue only).
	body = b.next()
	if got := studyHeader(t, body); got != "NATO phonetic alphabet · Set 1" {
		t.Errorf("place line on the second card = %q", got)
	}
	text = visibleText(body)
	for _, a := range answers {
		if showsWord(text, a) {
			t.Errorf("the second card's page shows %q", a)
		}
	}
}

// Item 1: the guard also covers ideas the learner (or a draft) named after
// an answer, and the cue of an exact text's next line.
func TestSlice41_Spoiler_HandMadeTitlesAndExactTextsFallBackToNeutralLabels(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("guard@example.com")
	lib := w.lib("guard@example.com")

	// A top-level idea whose title holds the answer: "Current set".
	top, _ := lib.CreateIdea(ctx, 0, "Paris and other capitals", "", "", w.now())
	if _, err := lib.AddItem(ctx, store.NewItem{IdeaID: top, Prompt: "Capital of France?", Answer: "Paris"}, w.now()); err != nil {
		t.Fatal(err)
	}
	body := b.mustGet("/study")
	if got := studyHeader(t, body); got != "Current set" {
		t.Fatalf("a title holding the answer shows as %q, want \"Current set\"", got)
	}
	hasNot(t, visibleText(body), "Paris")
	first, _ := b.current(body)
	review := b.answer(first, "Paris")
	if got := studyHeader(t, review); got != "Paris and other capitals" {
		t.Errorf("after answering the place line = %q", got)
	}
	b.next()

	// A nested idea: its own title is never shown; a topic that names a
	// sibling's answer is dropped too, leaving the set number.
	parent, _ := lib.CreateIdea(ctx, 0, "Rivers such as the Nile", "", "", w.now().Add(time.Second))
	if _, err := lib.CreateIdea(ctx, parent, "Warm-up", "", "", w.now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	child, _ := lib.CreateIdea(ctx, parent, "Longest rivers: Amazon", "", "", w.now().Add(3*time.Second))
	for _, qa := range [][2]string{{"Largest river by volume?", "The Amazon"}, {"Longest river in Africa?", "The Nile"}} {
		if _, err := lib.AddItem(ctx, store.NewItem{IdeaID: child, Prompt: qa[0], Answer: qa[1]}, w.now().Add(4*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	body = b.mustGet("/study")
	if got := studyHeader(t, body); got != "Set 2" {
		t.Fatalf("a nested idea under a leaking topic shows as %q, want \"Set 2\"", got)
	}
	hasNot(t, visibleText(body), "Amazon", "Nile")
	cur, _ := b.current(body)
	b.answerNext(cur, "x")
	cur, _ = b.current(b.mustGet("/study"))
	b.answerNext(cur, "x")

	// A safe topic is shown with the set number.
	safe, _ := lib.CreateIdea(ctx, 0, "World geography", "", "", w.now().Add(5*time.Second))
	kid, _ := lib.CreateIdea(ctx, safe, "Oslo and friends", "", "", w.now().Add(6*time.Second))
	if _, err := lib.AddItem(ctx, store.NewItem{IdeaID: kid, Prompt: "Capital of Norway?", Answer: "Oslo"}, w.now().Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	body = b.mustGet("/study")
	if got := studyHeader(t, body); got != "World geography · Set 1" {
		t.Fatalf("place line = %q", got)
	}
	hasNot(t, visibleText(body), "Oslo")
	cur, _ = b.current(body)
	b.answerNext(cur, "Oslo")

	// An exact text: the cue of line 2 quotes line 1, so it is held back
	// from "coming up" until line 1 has been answered.
	if _, page := b.post("/library/texts", "title", "Short verse", "text", "Roses are red today\nViolets are blue tonight\nSugar is sweet", "parent", "0"); !strings.Contains(page, "Short verse") {
		t.Fatal("the exact text was not added")
	}
	body = b.mustGet("/study")
	hasNot(t, visibleText(body), "Roses are red today")
	line1, _ := b.current(body)
	if !line1.Exact || line1.Answer != "Roses are red today" {
		t.Fatalf("line 1 = %+v", line1)
	}
	review = b.answer(line1, "Roses are red today")
	has(t, review, "Coming up", "Short verse: what follows “Roses are red today”") // shown once answered
}

// Pass 2 (B1): a short line of an exact text ("Go") is quoted by the cue of
// the line after it. That cue is held back from "coming up" while the short
// line is still owed, whatever its length.
func TestSlice41_Spoiler_ShortExactAnswerIsNotShownInComingUp(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("short@example.com")
	if _, page := b.post("/library/texts", "title", "Studio notes", "text", "Go\nStay the course\nRest", "parent", "0"); !strings.Contains(page, "Studio notes") {
		t.Fatal("the exact text was not added")
	}
	body := b.mustGet("/study")
	line1, _ := b.current(body)
	if !line1.Exact || line1.Answer != "Go" || line1.LineNo != 1 {
		t.Fatalf("line 1 = %+v", line1)
	}
	has(t, body, "Exact text · Studio notes · line 1", "Studio notes: how does it begin?", "Coming up",
		"<li>Studio notes: what follows “Stay the course”</li>") // line 3's cue quotes line 2, not the answer owed
	hasNot(t, body, "what follows “Go”", "“Go”")
	if showsWord(visibleText(body), "Go") {
		t.Errorf("the study page shows the answer \"Go\" before it is given:\n%s", visibleText(body))
	}
	if n := strings.Count(upcomingRE.FindString(body), "<li>"); n != 1 {
		t.Errorf("%d cues are coming up, want only line 3's", n)
	}
	// Once "Go" is in, the cue that quotes it is shown.
	review := b.answer(line1, "Go")
	has(t, review, `data-verdict="right"`, "Coming up", "<li>Studio notes: what follows “Go”</li>")
	// Line 2 waits: its own cue quotes line 1 (that is the cue), and line
	// 3's cue, which quotes line 2, is held back in its turn.
	body = b.next()
	line2, _ := b.current(body)
	if line2.Answer != "Stay the course" {
		t.Fatalf("line 2 = %+v", line2)
	}
	has(t, body, "Studio notes: what follows “Go”")
	hasNot(t, body, "Stay the course", "Coming up")
}

// Pass 2 (B1): an exact text named for one of its own lines ("Daffodils",
// whose first line is "Daffodils"). While that line is owed the title is
// nowhere on the page: not in the rail, the exact-text label, the prompt,
// the page title, the scrollback or "coming up". The review shows it.
func TestSlice41_Spoiler_AnswerBearingExactTitleIsNeutralUntilAnswered(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("daffodils@example.com")
	if _, page := b.post("/library/texts", "title", "Daffodils", "text", "Daffodils\nI wandered lonely as a cloud\nThat floats on high", "parent", "0"); !strings.Contains(page, "Daffodils") {
		t.Fatal("the exact text was not added")
	}
	noTitle := func(when, body string) {
		t.Helper()
		if strings.Contains(strings.ToLower(body), "daffodil") {
			i := strings.Index(strings.ToLower(body), "daffodil")
			t.Errorf("%s: the page holds the answer before it is given: …%s…", when, body[max(0, i-80):min(len(body), i+60)])
		}
	}
	body := b.mustGet("/study")
	line1, _ := b.current(body)
	if !line1.Exact || line1.Answer != "Daffodils" || line1.TextTitle != "Daffodils" || line1.Prompt != "Daffodils: how does it begin?" {
		t.Fatalf("line 1 = %+v", line1)
	}
	noTitle("line 1 waiting", body) // the whole document: title, rail, label, prompt, lists, attributes
	if got := studyHeader(t, body); got != "Current set" {
		t.Errorf("rail = %q, want \"Current set\"", got)
	}
	has(t, body, "<title>Study · scry</title>", `<p class="meta exact-label">Exact text · line 1</p>`,
		`<h1 class="prompt display" id="prompt">Exact text: how does it begin?</h1>`,
		"Coming up", "<li>Exact text: what follows “I wandered lonely as a cloud”</li>")
	if n := strings.Count(upcomingRE.FindString(body), "<li>"); n != 1 {
		t.Errorf("%d cues are coming up, want only line 3's (line 2's quotes the answer)", n)
	}

	// A wrong answer: the review names the text, and the next cue.
	review := b.answer(line1, "Tulips")
	has(t, review, `data-kind="exact"`, `<p class="meta exact-label">Exact text · Daffodils · line 1</p>`, "data-prompt>Daffodils: how does it begin?</h1>",
		"data-canonical>Daffodils<", "<li>Daffodils: what follows “Daffodils”</li>")
	if got := studyHeader(t, review); got != "Daffodils" {
		t.Errorf("rail in the review = %q", got)
	}

	// Again: the line is owed once more, so the title is gone once more,
	// from the card and from its own row in the scrollback.
	_, body = b.post("/study/next", "again", "1")
	again, ok := b.current(body)
	if !ok || again.ID != line1.ID {
		t.Fatalf("Again did not bring line 1 back: %+v", again)
	}
	noTitle("line 1 again", body)
	has(t, body, `<p class="meta exact-label">Exact text · line 1 · again</p>`, `id="prompt">Exact text: how does it begin?</h1>`,
		`<span class="answered-q">Exact text: how does it begin?</span>`)

	// Answered again, the practice review names the text.
	review = b.answer(again, "Daffodils")
	has(t, review, `data-verdict="right"`, "Exact text · Daffodils · line 1")

	// Line 2 waits. "Daffodils" is no longer owed: the text has its name
	// again, and the cue quotes line 1 as every cue quotes the line before.
	body = b.next()
	line2, _ := b.current(body)
	if line2.Answer != "I wandered lonely as a cloud" {
		t.Fatalf("line 2 = %+v", line2)
	}
	has(t, body, `<p class="meta exact-label">Exact text · Daffodils · line 2</p>`, `id="prompt">Daffodils: what follows “Daffodils”</h1>`,
		`<span class="answered-q">Daffodils: how does it begin?</span>`)
	hasNot(t, body, "I wandered lonely as a cloud", "Coming up")
}

func TestSlice41_Spoiler_NeutralExactTitleCanBeTheAnswer(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("neutral-title@example.com")
	b.post("/library/texts", "title", "Exact text", "text", "Exact text\nNow turn the page", "parent", "0")

	body := b.mustGet("/study")
	line, ok := b.current(body)
	if !ok || line.Answer != "Exact text" {
		t.Fatalf("unexpected waiting card: %+v", line)
	}
	hasNot(t, strings.ToLower(body), "exact text")
	has(t, body, `<p class="meta exact-label">Line 1</p>`, `id="prompt">Line 1 — recite from memory</h1>`)
	if got := studyHeader(t, body); got != "Current set" {
		t.Errorf("waiting rail = %q, want Current set", got)
	}

	review := b.answer(line, "Not the line")
	has(t, review, `<p class="meta exact-label">Exact text · Exact text · line 1</p>`,
		"data-prompt>Exact text: how does it begin?</h1>", "data-canonical>Exact text<")
	if got := studyHeader(t, review); got != "Exact text" {
		t.Errorf("review rail = %q, want Exact text", got)
	}
}

func TestSlice41_Spoiler_EditedExactPromptDoesNotRepeatOwedAnswerOnAgain(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("repeated-title@example.com")
	b.post("/library/texts", "title", "Daffodils", "text", "Daffodils\nI wandered lonely as a cloud", "parent", "0")
	line, ok := b.current(b.mustGet("/study"))
	if !ok || line.Answer != "Daffodils" {
		t.Fatalf("unexpected waiting card: %+v", line)
	}
	has(t, b.answer(line, "Tulips"), `data-kind="exact"`)

	const edited = "Daffodils: what is the first line of Daffodils?"
	status, review := b.post("/study/edit", "item_id", strconv.FormatInt(line.ID, 10), "prompt", edited, "answer", line.Answer)
	if status != http.StatusOK {
		t.Fatalf("edit status = %d", status)
	}
	has(t, review, "data-prompt>"+edited+"</h1>")
	_, body := b.post("/study/next", "again", "1")
	again, ok := b.current(body)
	if !ok || again.ID != line.ID {
		t.Fatalf("Again did not bring line 1 back: %+v", again)
	}
	hasNot(t, strings.ToLower(body), "daffodils")
	has(t, body, `id="prompt">Line 1 — recite from memory</h1>`,
		`<p class="meta exact-label">Exact text · line 1 · again</p>`)
	has(t, b.answer(again, "Daffodils"), "data-prompt>"+edited+"</h1>", "data-canonical>Daffodils<")
}

// Pass 2 (B3): a card confirmation that arrives for a superseded attempt
// (the form was given back after the 30 second timeout, or a newer submit
// started) is ignored by stripe.js: its payment method is never written or
// submitted, and it does not unlock the form or print an error over the
// attempt that replaced it. scripts/verify-stripe-retry.mjs runs the same
// race in a real browser against app.js and its real 30 second timer.
func TestSlice41_Processing_SupersededStripeAttemptIsIgnored(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is needed for the stripe.js stand-in")
	}
	cmd := exec.Command("node", "-e", `
const vm = require("node:vm"), fs = require("node:fs"), assert = require("node:assert/strict");
const source = fs.readFileSync("static/stripe.js", "utf8");
// page stands in for the card page. withBusy adds app.js's processing state
// as its contract goes: start and stop each move form.dataset.submitAttempt on.
function page(withBusy) {
  const p = {calls: [], reports: [], submitted: [], stops: 0, error: {textContent: ""}, hidden: {value: ""}, button: {disabled: false}};
  const handlers = {}, submits = [];
  const mount = {dataset: {key: "pk_test_x"}};
  const form = p.form = {dataset: {}, addEventListener(ev, fn) { submits.push(fn); }, querySelector() { return p.button; }, submit() { p.submitted.push(p.hidden.value); }};
  const document = {
    getElementById(id) { return id === "card-element" ? mount : id === "card-error" ? p.error : id === "payment_method" ? p.hidden : null; },
    querySelector() { return form; }
  };
  const bump = () => { form.dataset.submitAttempt = String((Number(form.dataset.submitAttempt) || 0) + 1); };
  const window = {scryReport(kind, code) { p.reports.push(code); },
    Stripe() { return {elements() { return {create() { return {mount() {}, on(ev, fn) { handlers[ev] = fn; }}; }}; },
      createPaymentMethod() { const call = {}; p.calls.push(call); return {then(ok, bad) { call.resolve = ok; call.reject = bad; }}; }}; }};
  if (withBusy) window.scryBusy = {start() { bump(); }, stop() { p.stops++; bump(); p.button.disabled = false; }, attempt() { return Number(form.dataset.submitAttempt) || 0; }};
  vm.runInNewContext(source, {window, document, setTimeout() { return 1; }, clearTimeout() {}});
  handlers.ready();
  // submit presses the button: every submit listener runs, as in the page.
  p.submit = () => { const e = {defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }}; submits.forEach(fn => fn(e)); return e.defaultPrevented; };
  // timeout is app.js giving the form back after 30 seconds.
  p.timeout = () => { if (withBusy) window.scryBusy.stop(form); else { bump(); p.button.disabled = false; } };
  return p;
}
for (const withBusy of [true, false]) {
  const at = withBusy ? "with app.js: " : "stripe.js alone: ";
  // The race: #1 is slow, the form is given back, #2 starts, #1 answers first.
  for (const late of ["a payment method", "a refusal", "a network failure"]) {
    const p = page(withBusy);
    assert.ok(p.submit(), at + "the form must not post before the card is confirmed");
    assert.equal(p.calls.length, 1);
    assert.ok(p.submit(), at + "a second submit while confirming is held");
    assert.equal(p.calls.length, 1, at + "a second submit while confirming asks the processor again");
    p.timeout();
    assert.equal(p.button.disabled, false);
    p.submit();
    assert.equal(p.calls.length, 2, at + "the retry is a new attempt");
    assert.equal(p.button.disabled, true);
    const stops = p.stops;
    if (late === "a payment method") p.calls[0].resolve({paymentMethod: {id: "pm_first_stale"}});
    if (late === "a refusal") p.calls[0].resolve({error: {message: "Your first card was declined."}});
    if (late === "a network failure") p.calls[0].reject(new Error("Failed to fetch"));
    assert.equal(p.hidden.value, "", at + late + " for the superseded attempt was written to the form");
    assert.deepEqual(p.submitted, [], at + late + " for the superseded attempt submitted the form");
    assert.equal(p.error.textContent, "", at + late + " for the superseded attempt printed an error");
    assert.equal(p.button.disabled, true, at + late + " for the superseded attempt unlocked the form");
    assert.equal(p.stops, stops, at + late + " for the superseded attempt ended the processing state");
    assert.deepEqual(p.reports, [], at + late + " for the superseded attempt was reported");
    p.calls[1].resolve({paymentMethod: {id: "pm_second"}});
    assert.deepEqual(p.submitted, ["pm_second"], at + "only the retry's payment method is submitted");
  }
  // Given back and not retried: the late answer still changes nothing.
  {
    const p = page(withBusy);
    p.submit(); p.timeout();
    p.calls[0].resolve({paymentMethod: {id: "pm_first_stale"}});
    assert.equal(p.hidden.value, "");
    assert.deepEqual(p.submitted, []);
    assert.equal(p.button.disabled, false);
  }
  // The post itself stalled: #1 was confirmed and submitted, the form was
  // given back, and the retry confirms the card in the field now.
  {
    const p = page(withBusy);
    p.submit();
    p.calls[0].resolve({paymentMethod: {id: "pm_first"}});
    assert.deepEqual(p.submitted, ["pm_first"]);
    assert.ok(p.submit(), at + "a second submit while the form is posting is held");
    assert.equal(p.calls.length, 1);
    p.timeout();
    assert.ok(p.submit(), at + "the retry must not post the earlier card as it stands");
    assert.equal(p.hidden.value, "", at + "the retry kept the earlier payment method");
    assert.equal(p.calls.length, 2);
    p.calls[1].resolve({paymentMethod: {id: "pm_second"}});
    assert.deepEqual(p.submitted, ["pm_first", "pm_second"]);
  }
  // The attempt in hand still fails the way it did: reason, form given back.
  {
    const p = page(withBusy);
    p.submit();
    p.calls[0].resolve({error: {message: "Your card was declined."}});
    assert.equal(p.error.textContent, "Your card was declined.");
    assert.equal(p.button.disabled, false);
    assert.deepEqual(p.submitted, []);
    p.submit();
    assert.equal(p.calls.length, 2, at + "a refused card can be tried again");
    p.calls[1].reject(new Error("Failed to fetch"));
    assert.match(p.error.textContent, /Couldn't reach the card processor/);
    assert.deepEqual(p.reports, ["stripe_payment_method_failed"]);
    assert.equal(p.button.disabled, false);
  }
}
`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stripe.js: %v\n%s", err, out)
	}
	// app.js numbers the attempts: starting and ending the processing
	// state both move form.dataset.submitAttempt on.
	js := string(mustStatic(t, "static/app.js"))
	start := js[strings.Index(js, "function busyStart(form) {"):strings.Index(js, "function busyStop(form) {")]
	stop := js[strings.Index(js, "function busyStop(form) {"):strings.Index(js, "window.scryBusy = ")]
	for name, body := range map[string]string{"busyStart": start, "busyStop": stop} {
		if !strings.Contains(body, "busySupersede(form);") {
			t.Errorf("app.js: %s does not supersede the attempt before it", name)
		}
	}
	has(t, js, "form.dataset.submitAttempt = String(next);", "window.scryBusy = { start: busyStart, stop: busyStop, attempt: busyAttempt };",
		"form.busyTimer = setTimeout(function () { busyStop(form); }, 30000);")
}

func mustStatic(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Item 2: every verdict stops on the post-answer review, which shows the
// submitted and correct answers, the verdict in words, Ask Why?, the
// card's stored schedule, Edit card and Next card.
func TestSlice41_Review_EveryAnswerStopsOnTheReviewWithItsFacts(t *testing.T) {
	w := newWorld(t)
	w.app.Explainer = &ai.FakeExplainer{}
	b := w.browser()
	b.signUp("review@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("review@example.com")
	user := w.user("review@example.com")

	for i, c := range []struct {
		verdict     srs.Verdict
		kind, badge string
		typed       string
	}{
		{srs.Right, "right", "Correct", "alfa"},
		{srs.Close, "close", "Almost Correct", "Brave"},
		{srs.Missed, "missed", "Incorrect", "no idea"},
	} {
		body := b.mustGet("/study")
		item, ok := b.current(body)
		if !ok {
			t.Fatalf("%s: no card is waiting", c.kind)
		}
		w.model.script = []ai.Judgment{{Verdict: c.verdict, Reason: "Reason for " + c.kind + "."}}
		review := b.answer(item, c.typed)
		if b.path != "/study" {
			t.Fatalf("%s: answering ended at %s", c.kind, b.path)
		}
		// The review, not the next card.
		if id, ok := b.reviewed(review); !ok || id != item.ID {
			t.Fatalf("%s: the answer did not stop on its review (reviewing %d, %v)", c.kind, id, ok)
		}
		if _, asking := b.current(review); asking {
			t.Fatalf("%s: the next card is already on screen", c.kind)
		}
		has(t, review,
			`data-kind="`+c.kind+`"`, `data-verdict="`+string(c.verdict)+`"`, `<span data-verdict-label>`+c.badge+`</span>`,
			`icon-verdict-`+string(c.verdict), "Reason for "+c.kind+".",
			"data-explain", "Ask Why?", "data-telemetry", "Next review", "Interval", "Stability", "Difficulty", "Reviews",
			"data-edit-toggle", "Edit card", "data-next", "Next card", "enter or space for next card")
		if got := submittedRE.FindStringSubmatch(review); got == nil || got[1] != c.typed {
			t.Errorf("%s: submitted answer shown as %v, want %q", c.kind, got, c.typed)
		}
		if got := canonicalRE.FindStringSubmatch(review); got == nil || got[1] != item.Answer {
			t.Errorf("%s: correct answer shown as %v, want %q", c.kind, got, item.Answer)
		}
		if c.kind == "missed" {
			has(t, review, "judgment-struck", "struck-text", "judgment-right") // the loud moment is kept
		} else {
			has(t, review, "Your answer", "Correct answer")
		}

		// The schedule on screen is the stored one.
		stored := mustItem(t, lib, item.ID)
		m := nextReviewRE.FindStringSubmatch(review)
		if m == nil {
			t.Fatalf("%s: no next-review fact", c.kind)
		}
		due := stored.Sched.Due.In(user.Location())
		days := int(stored.Sched.Due.Sub(stored.Sched.Last).Hours()/24 + 0.5)
		wantRel := "tomorrow"
		if days > 1 {
			wantRel = fmt.Sprintf("in %d days", days)
		}
		if m[1] != due.Format("2006-01-02") || m[2] != due.Format("Jan 2") || m[3] != wantRel {
			t.Errorf("%s: next review shown as %q %q %q; stored due %s", c.kind, m[1], m[2], m[3], due)
		}
		wantInterval := "1 day"
		if days > 1 {
			wantInterval = fmt.Sprintf("%d days", days)
		}
		has(t, review,
			`data-interval>`+wantInterval+`<`,
			`data-stability>`+strconv.FormatFloat(stored.Sched.Stability, 'f', 1, 64)+` days<`,
			`data-difficulty>`+strconv.FormatFloat(stored.Sched.Difficulty, 'f', 1, 64)+` / 10<`,
			`data-review-total="1"`)
		counts := map[srs.Verdict]string{srs.Right: "1 · 1 correct · 0 almost · 0 incorrect", srs.Close: "1 · 0 correct · 1 almost · 0 incorrect", srs.Missed: "1 · 0 correct · 0 almost · 1 incorrect"}
		has(t, review, counts[c.verdict])
		// Facts only.
		hasNot(t, strings.ToLower(visibleText(review)), "streak", "points", "confetti", "well done", "great job", "🔥")

		// Reloading Study shows the same review: nothing auto-advances.
		again := b.mustGet("/study")
		if id, ok := b.reviewed(again); !ok || id != item.ID {
			t.Fatalf("%s: reloading Study left the review", c.kind)
		}
		// Next card moves on.
		after := b.next()
		next, ok := b.current(after)
		if !ok || next.ID == item.ID {
			t.Fatalf("%s: Next card did not bring the next card", c.kind)
		}
		if strings.Count(after, `<li class="answered">`) != i+1 {
			t.Errorf("%s: scrollback has %d rows, want %d", c.kind, strings.Count(after, `<li class="answered">`), i+1)
		}
	}

	// A second review of the same card counts both, from the history.
	first, _ := lib.Search(ctx, "NATO alphabet: A")
	if c, err := lib.ReviewCounts(ctx, first[0].ID); err != nil || c.Total != 1 || c.Right != 1 {
		t.Fatalf("counts = %+v, %v", c, err)
	}
}

// Item 2: the last card says "Finish session", and an exact text keeps
// its word-by-word comparison and "Again" inside the same review.
func TestSlice41_Review_LastCardAndExactText(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("exact@example.com")
	lib := w.lib("exact@example.com")
	if _, page := b.post("/library/texts", "title", "Motto", "text", "Know thyself well\nNothing in excess", "parent", "0"); !strings.Contains(page, "Motto") {
		t.Fatal("the exact text was not added")
	}
	line1, _ := b.current(b.mustGet("/study"))
	review := b.answer(line1, "Know yourself well")
	has(t, review, `data-kind="exact"`, `data-verdict="missed"`, `<span data-verdict-label>Incorrect</span>`,
		`<del class="x-wrong"><span class="sr">you wrote: </span>yourself</del>`, "data-canonical>Know thyself well<",
		"data-telemetry", "data-edit-toggle", ">Again</button>", "Next card", "a again · enter or space for next card")
	hasNot(t, review, "data-explain", "Ask Why?") // the diff already says what differed
	// Again is practice: the review of the retry says the schedule stood still.
	due := mustItem(t, lib, line1.ID).Sched.Due
	_, body := b.post("/study/next", "again", "1")
	again, _ := b.current(body)
	practice := b.answer(again, "know thyself well")
	has(t, practice, `data-verdict="right"`, "Practice attempt: the schedule did not move.", `data-review-total="1"`)
	if got := mustItem(t, lib, line1.ID).Sched.Due; !got.Equal(due) {
		t.Fatal("the practice attempt moved the schedule")
	}
	body = b.next()
	line2, ok := b.current(body)
	if !ok || line2.ID == line1.ID {
		t.Fatal("Next card did not bring line 2")
	}
	last := b.answer(line2, "Nothing in excess")
	has(t, last, "Finish session", "enter or space for finish")
	hasNot(t, last, "Next card<")
	b.next()
	if b.path != "/study/done" {
		t.Fatalf("after the last card the learner is at %s", b.path)
	}
}

// Item 2: "Edit card" saves the question and the correct answer from the
// review, in place for the page's script and by a plain form post, behind
// sign-in, CSRF and ownership, and changes nothing but the card's text.
func TestSlice41_Edit_SavesInPlacePersistsAndIsGuarded(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("editor@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("editor@example.com")
	item, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "Not it."}}
	review := b.answer(item, "apple")
	has(t, review, `action="/study/edit"`, `name="prompt"`, `name="answer"`, "Save card",
		"Edits apply from the next review. This grade and its schedule stay as they are.")
	hist, _ := lib.History(ctx, item.ID)
	sched := mustItem(t, lib, item.ID).Sched
	id := strconv.FormatInt(item.ID, 10)

	// In place: JSON in, JSON out, no redirect.
	status, out, ctype := b.postJSON("/study/edit", "item_id", id, "prompt", "  NATO alphabet: the letter A ", "answer", "Alfa (also spelled Alpha)")
	if status != 200 || !strings.Contains(ctype, "application/json") || out["saved"] != true ||
		out["prompt"] != "NATO alphabet: the letter A" || out["answer"] != "Alfa (also spelled Alpha)" {
		t.Fatalf("edit as JSON: %d %s %v", status, ctype, out)
	}
	saved := mustItem(t, lib, item.ID)
	if saved.Prompt != "NATO alphabet: the letter A" || saved.Answer != "Alfa (also spelled Alpha)" || saved.Exact != item.Exact {
		t.Fatalf("stored card = %+v", saved)
	}
	// The grade, its snapshot and the schedule are as they were.
	if after, _ := lib.History(ctx, item.ID); len(after) != 1 || after[0] != hist[0] || after[0].GradedExpected != "Alfa" {
		t.Fatalf("the edit changed the review: %+v", after)
	}
	if saved.Sched != sched {
		t.Fatal("the edit moved the schedule")
	}
	// The learner has not lost their place: the same review, with the new text.
	page := b.mustGet("/study")
	if rid, ok := b.reviewed(page); !ok || rid != item.ID {
		t.Fatal("the edit left the review")
	}
	has(t, page, "data-canonical>Alfa (also spelled Alpha)<", "NATO alphabet: the letter A", `data-verdict="missed"`, "Not it.")

	// Refused in words; nothing changes.
	for name, kv := range map[string][2]string{"an empty answer": {"Still a question", "  "}, "an empty question": {"", "Still an answer"}} {
		status, out, _ := b.postJSON("/study/edit", "item_id", id, "prompt", kv[0], "answer", kv[1])
		if msg, _ := out["error"].(string); status != http.StatusOK || out["saved"] != false || msg == "" {
			t.Errorf("%s: %d %v", name, status, out)
		}
	}
	status, page = b.post("/study/edit", "item_id", id, "prompt", "Kept question", "answer", "")
	if status != http.StatusUnprocessableEntity {
		t.Errorf("empty answer by form post: %d", status)
	}
	has(t, page, "The correct answer can't be empty.", "data-edit open", "Kept question", "data-review")
	if got := mustItem(t, lib, item.ID); got.Answer != "Alfa (also spelled Alpha)" {
		t.Fatalf("a refused edit changed the card: %+v", got)
	}

	// Without JavaScript: the form posts, and the review returns with "Saved".
	status, page = b.post("/study/edit", "item_id", id, "prompt", "NATO alphabet: A", "answer", "Alfa")
	if status != 200 || b.path != "/study" {
		t.Fatalf("edit by form post: %d at %s", status, b.path)
	}
	has(t, page, "data-edit open", "data-edit-status>", "Saved", "data-canonical>Alfa<", "data-review")
	hasNot(t, page, "data-edit-status hidden")
	if got := mustItem(t, lib, item.ID); got.Prompt != "NATO alphabet: A" || got.Answer != "Alfa" {
		t.Fatalf("stored card = %+v", got)
	}

	// CSRF: a post without the token is refused and changes nothing.
	form := url.Values{"item_id": {id}, "prompt": {"hijacked"}, "answer": {"hijacked"}}
	req, _ := http.NewRequest("POST", w.srv.URL+"/study/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if status, _ := b.do(req); status != http.StatusForbidden {
		t.Errorf("edit without a CSRF token: %d, want 403", status)
	}
	// Signed out: sent to sign-in.
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ = http.NewRequest("POST", w.srv.URL+"/study/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := anon.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/signin") {
		t.Errorf("signed-out edit: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// Another learner: not found, as JSON or as a page.
	m := w.browser()
	m.signUp("mallory@example.com")
	m.addSet("kitchen-numbers")
	m.get("/study")
	if status, out, _ := m.postJSON("/study/edit", "item_id", id, "prompt", "mine", "answer", "mine"); status != http.StatusNotFound || out["saved"] == true {
		t.Errorf("another learner's edit as JSON: %d %v", status, out)
	}
	if status, _ := m.post("/study/edit", "item_id", id, "prompt", "mine", "answer", "mine"); status != http.StatusNotFound {
		t.Errorf("another learner's edit by form: %d", status)
	}
	if got := mustItem(t, lib, item.ID); got.Prompt != "NATO alphabet: A" || got.Answer != "Alfa" {
		t.Fatalf("someone else changed the card: %+v", got)
	}
}

// Item 3: Study with an empty library is its own screen.
func TestSlice41_EmptyLibrary_StudySaysSoAndOffersTwoWaysForward(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("empty@example.com")
	client := &http.Client{Jar: b.c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(w.srv.URL + "/study")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /study with an empty library = %d %s, want the page itself", resp.StatusCode, resp.Header.Get("Location"))
	}
	page := b.mustGet("/study")
	has(t, page, "data-study-empty", "No cards to study yet.", "Your library is empty.", "Recall what's due today",
		`href="/shelf" data-cta="starter"`, "Pick a starter set", `href="/learn" data-cta="create"`, "Create cards with AI",
		`<a href="/study" aria-current="page"`)
	hasNot(t, page, "data-answer-form", "Nothing due")
	// With cards, Study is the session again.
	b.addSet("kitchen-numbers")
	has(t, b.mustGet("/study"), "data-answer-form")
}

// Item 4: Study and Create are told apart at a glance.
func TestSlice41_Nav_StudyAndCreateAreUnmistakable(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("nav@example.com")
	b.addSet("kitchen-numbers")
	for _, path := range []string{"/study", "/library", "/inbox", "/learn", "/billing", "/settings", "/shelf"} {
		page := b.mustGet(path)
		nav := page[strings.Index(page, `<nav class="nav"`):]
		nav = nav[:strings.Index(nav, "</nav>")]
		for _, want := range []string{
			`<a href="/study"`, `icon-cards`, `<span class="nav-label">Study</span>`,
			`icon-library`, `<span class="nav-label">Library</span>`, `icon-inbox`, `<span class="nav-label">Inbox</span>`,
			`<a href="/learn"`, `icon-create`, `<span class="nav-label">Create</span>`,
			`icon-billing`, `<span class="nav-label">Billing</span>`, `icon-settings`, `<span class="nav-label">Settings</span>`,
		} {
			if !strings.Contains(nav, want) {
				t.Errorf("%s: the navigation is missing %s", path, want)
			}
		}
		if strings.Contains(nav, "Learn") {
			t.Errorf("%s: the navigation still says Learn", path)
		}
		if strings.Count(nav, "<svg") != 6 || strings.Count(nav, `aria-hidden="true"`) != 6 {
			t.Errorf("%s: want one hidden icon per destination, got %d", path, strings.Count(nav, "<svg"))
		}
		has(t, page, "g n create")
		hasNot(t, page, "g n learn")
	}
	create := b.mustGet("/learn")
	has(t, create, "<title>Create · scry</title>", "Generate cards with AI", "What do you want to know?", `<a href="/learn" aria-current="page"`)
	if n := len(strings.Fields("Generate cards with AI")); n != 4 {
		t.Fatalf("the Create purpose line has %d words", n)
	}
	study := b.mustGet("/study")
	has(t, study, "Recall what's due today", `<a href="/study" aria-current="page"`)
	hasNot(t, study, "Generate cards with AI")
}

// Item 5: adding a set answers the shelf's script with JSON, so the page
// confirms in place; a plain post still works and returns to the tile.
func TestSlice41_Shelf_AddConfirmsInPlaceWithoutAPageLoad(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("shelf@example.com")
	lib := w.lib("shelf@example.com")
	shelf := b.mustGet("/shelf")
	has(t, shelf, "data-add", "data-added-template", "data-shelf-bar hidden", `data-tile data-cards="26"`)

	status, out, ctype := b.postJSON("/shelf/add", "slug", "nato-alphabet")
	if status != 200 || !strings.Contains(ctype, "application/json") || out["ok"] != true || out["slug"] != "nato-alphabet" ||
		out["summary"] != "1 set added · 26 cards" || out["study"] != "/study" {
		t.Fatalf("add as JSON: %d %s %v", status, ctype, out)
	}
	if n, _, _ := lib.Counts(ctx, w.now()); n != 26 {
		t.Fatalf("%d cards after adding", n)
	}
	// Twice is once.
	if status, out, _ = b.postJSON("/shelf/add", "slug", "nato-alphabet"); status != 200 || out["summary"] != "1 set added · 26 cards" {
		t.Fatalf("adding again: %d %v", status, out)
	}
	if status, out, _ = b.postJSON("/shelf/add", "slug", "world-geography"); status != 200 || out["summary"] != "2 sets added · 50 cards" {
		t.Fatalf("second set: %d %v", status, out)
	}
	if n, _, _ := lib.Counts(ctx, w.now()); n != 50 {
		t.Fatalf("%d cards after two sets", n)
	}
	if status, out, _ = b.postJSON("/shelf/add", "slug", "no-such-set"); status != http.StatusNotFound || out["error"] == nil {
		t.Fatalf("unknown set: %d %v", status, out)
	}
	shelf = b.mustGet("/shelf")
	has(t, shelf, `<li class="tile is-added" id="nato-alphabet"`, `<li class="tile is-added" id="world-geography"`,
		"2 sets added · 50 cards", "Start studying", `data-shelf-bar>`)

	// Without JavaScript: the plain post redirects back to the tile.
	client := &http.Client{Jar: b.c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.PostForm(w.srv.URL+"/shelf/add", url.Values{"csrf": {b.csrf}, "slug": {"mental-models"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/shelf?added=mental-models#mental-models" {
		t.Fatalf("plain add: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// Items 6 and 9: the shelf's headline, order, icons and counts.
func TestSlice41_Shelf_HeadlineOrderIconsAndCounts(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("tiles@example.com")
	shelf := b.mustGet("/shelf")
	has(t, shelf, "Pick a deck and start recalling in seconds.", "30 starter sets · add one or more to begin")
	hasNot(t, shelf, "Start with something worth knowing.")
	ids := regexp.MustCompile(`<li class="tile[^"]*" id="([a-z0-9-]+)"`).FindAllStringSubmatch(shelf, -1)
	if len(ids) != 30 {
		t.Fatalf("%d tiles", len(ids))
	}
	var order []string
	for _, m := range ids {
		order = append(order, m[1])
	}
	if got := strings.Join(order[:4], ","); got != "nato-alphabet,mental-models,kitchen-numbers,world-geography" {
		t.Errorf("first row = %s", got)
	}
	for i, slug := range order {
		if slug == "the-creeds" && i < 28 {
			t.Errorf("the Creeds are at place %d", i+1)
		}
	}
	has(t, shelf, "The Creeds", "Passages worth keeping") // moved, not removed
	// Each tile: its subject icon, and a card-stack count in tabular figures.
	for _, set := range starter.All() {
		tile := shelf[strings.Index(shelf, `id="`+set.Slug+`"`):]
		tile = tile[:strings.Index(tile, "</li>")]
		for _, want := range []string{
			`icon-` + set.Icon, `<span>` + set.Title + `</span>`, `icon-cards`,
			fmt.Sprintf(`<span class="num">%d</span> cards`, set.Count()), `icon-plus`, `aria-label="Add ` + set.Title + `"`,
		} {
			if !strings.Contains(tile, want) {
				t.Errorf("%s tile is missing %s", set.Slug, want)
			}
		}
	}
	if !strings.Contains(shelf, `icon-radio`) || !strings.Contains(shelf, `icon-globe`) || !strings.Contains(shelf, `icon-beaker`) ||
		!strings.Contains(shelf, `icon-layers`) || !strings.Contains(shelf, `icon-ruler`) || !strings.Contains(shelf, `icon-scroll`) {
		t.Error("the subject icons named in DESIGN.md are not all on the shelf")
	}
}

// Item 7: the card step and Top up carry the processing state.
func TestSlice41_Processing_CardAndTopUpFormsCarryTheBusyState(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("busy@example.com")
	for _, path := range []string{"/signup/card", "/billing/card"} {
		page := b.mustGet(path)
		has(t, page, `data-card-form data-busy-form`, `data-busy hidden`, `role="status"`, "Confirming your card…", "icon-spinner", "busy-track")
	}
	b.post("/signup/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	billing := b.mustGet("/billing")
	has(t, billing, `action="/billing/topup" class="topup" data-busy-form data-busy-text="Updating billing…"`, "Updating billing…", `data-busy hidden`)
	// A declined card comes back as an interactive form with the reason.
	d := w.browser()
	d.signUpNoCard("declined@example.com")
	d.get("/signup/card")
	status, page := d.post("/signup/card", "number", "4000 0000 0000 0002", "expiry", "12/30", "cvc", "123")
	if status == 200 && d.path == "/shelf" {
		t.Fatal("the declined test card was accepted")
	}
	has(t, page, `data-card-form data-busy-form`, `data-busy hidden`, `id="number"`, `role="alert"`)
	hasNot(t, page, "is-busy", `aria-busy="true"`, "<button disabled")
}

// Item 8: Settings and Inbox say what they are in plain words.
func TestSlice41_PlainCopy_SettingsInboxAndPagePurposes(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("plain@example.com")
	settings := b.mustGet("/settings")
	has(t, settings, `<h2 class="title">API Tokens</h2>`, `id="api-tokens"`,
		"Programmatic access to your account.", "suggest cards through the Scry API", "Token name", "Create token",
		`<h2 class="title">Study profile</h2>`, "Background and goals used to tailor AI-generated cards.",
		"Appearance, your study profile, API tokens and your data.")
	hasNot(t, settings, "can_add", "May add straight to my library", "without the inbox", ">Agents<", "About you",
		"In your own words", "New agent's name", "Scry shapes new material to this")
	// The form has exactly one visible field: the token's name.
	form := settings[strings.Index(settings, `action="/settings/tokens"`):]
	form = form[:strings.Index(form, "</form>")]
	if strings.Count(form, "<input") != 2 || strings.Contains(form, "checkbox") { // csrf + name
		t.Errorf("the Create token form has more than a name: %s", form)
	}
	// A posted can_add is ignored: the token still goes through the inbox.
	_, page := b.post("/settings/tokens", "name", "reading script", "can_add", "1")
	has(t, page, "Token for reading script.", "suggests to inbox")
	tokens, _ := w.lib("plain@example.com").Tokens(ctx)
	if len(tokens) != 1 || tokens[0].CanAdd {
		t.Fatalf("tokens = %+v, want one that may only suggest", tokens)
	}
	_, saved := b.post("/settings/about", "about", "I read history.")
	has(t, saved, "Study profile saved.")

	inbox := b.mustGet("/inbox")
	has(t, inbox, "Cards suggested by external scripts or tools wait here until you approve them.", "Nothing waiting.",
		`href="/settings#api-tokens"`)
	hasNot(t, inbox, "your agents", "Give an agent a token")

	b.addSet("kitchen-numbers")
	for path, purpose := range map[string]string{
		"/study":    "Recall what's due today",
		"/library":  "Every card you have, grouped by idea.",
		"/inbox":    "Cards suggested by external scripts or tools wait here until you approve them.",
		"/learn":    "Generate cards with AI",
		"/billing":  "Your plan, what you have used and what you have been charged.",
		"/settings": "Appearance, your study profile, API tokens and your data.",
	} {
		page := b.mustGet(path)
		has(t, page, purpose)
		lower := strings.ToLower(visibleText(page))
		for _, buzz := range []string{"unlock", "elevate", "seamless", "effortless", "supercharge", "delve", "magic"} {
			if strings.Contains(lower, buzz) {
				t.Errorf("%s says %q", path, buzz)
			}
		}
	}
}
