package web_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/srs"
)

// Slice 70: iPhone dogfooding polish on the Study review flow.

var nextRelRE = regexp.MustCompile(`data-next-rel>([^<]+)<`)

// PRODUCT.md (review story): an empty answer is still judged by the grader
// (G10: no local verdict), and the review says "No answer given." and
// "Skipped — review the answer above." in place of the grader's rubric.
func TestSlice70_Review_SkipSaysNoAnswerGivenInPlainWords(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("skip70@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	before := w.model.judgeCalls()
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
	review := b.answer(it, "")
	if w.model.judgeCalls() != before+1 {
		t.Fatalf("the empty answer was judged %d times, want once by the grader", w.model.judgeCalls()-before)
	}
	has(t, review, `data-verdict="missed"`, "<span data-verdict-label>Incorrect</span>",
		`data-skipped>No answer given.</p>`, `<span class="why">`+ai.SkipWhy+`</span>`, "data-canonical>"+it.Answer+"<")
	hasNot(t, review, ai.CriterionSentence(srs.Missed), "grader", "judgment-struck")
	// The stored reason is the grader's own criterion, unchanged.
	hist, _ := w.lib("skip70@example.com").History(ctx, it.ID)
	if len(hist) != 1 || hist[0].Reason != ai.CriterionSentence(srs.Missed) || hist[0].Answer != "" {
		t.Fatalf("stored review = %+v", hist)
	}
}

// Every verdict's why is a short plain sentence: no rubric wording reaches
// the review, whatever the grader's criterion.
func TestSlice70_Review_GradedWhyIsPlainNeverTheRubric(t *testing.T) {
	for _, v := range srs.Verdicts {
		w := newWorld(t)
		b := w.browser()
		email := "plain-" + string(v) + "@example.com"
		b.signUp(email)
		b.addSet("nato-alphabet")
		it, _ := b.current(b.mustGet("/study"))
		w.model.script = []ai.Judgment{{Verdict: v, Reason: ai.CriterionSentence(v)}}
		review := b.answer(it, "an answer")
		has(t, review, `<span class="why">`+ai.PlainWhy(v)+`</span>`)
		hasNot(t, review, ai.CriterionSentence(v), "No answer given.", ai.SkipWhy)
		// The card's history in the library uses the same plain words.
		item := b.mustGet("/library/items/" + itoa(it.ID))
		has(t, item, ai.PlainWhy(v))
		hasNot(t, item, ai.CriterionSentence(v))
	}
}

// DESIGN.md, Post-answer review: the review is the current card only; no
// earlier card above it and no next card ("NATO alphabet: D") below it.
// Progression is unchanged: Next card moves to the next card.
func TestSlice70_Review_ShowsTheCurrentCardOnly(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("only70@example.com")
	b.addSet("nato-alphabet")
	body := b.mustGet("/study")
	var prompts []string
	for i := 0; i < 3; i++ {
		it, ok := b.current(body)
		if !ok {
			t.Fatalf("card %d is not asking", i+1)
		}
		prompts = append(prompts, it.Prompt)
		review := b.answer(it, "x")
		if id, ok := b.reviewed(review); !ok || id != it.ID {
			t.Fatalf("card %d did not stop on its review", i+1)
		}
		hasNot(t, review, `<ol class="scrollback"`, `<ol class="upcoming"`, "Coming up")
		for _, p := range prompts[:i] {
			hasNot(t, review, p) // an earlier card
		}
		has(t, review, "data-prompt>"+it.Prompt+"<")
		body = b.next()
		next, ok := b.current(body)
		if !ok || next.ID == it.ID {
			t.Fatalf("Next card did not move on from %q", it.Prompt)
		}
		hasNot(t, review, next.Prompt) // the next card was not previewed
	}
	// While a card waits, the scrollback and coming-up list are as before.
	has(t, body, `<ol class="scrollback"`, `<ol class="upcoming"`)
}

// The observed "Next review Nov 23 · today" beside "Interval 1 day": the
// event and the interval share one clock. Right after answering the review
// reads "in 1 day" beside "1 day after this answer"; reopened 20 hours later
// it counts down in hours, never "today".
func TestSlice70_Review_EventAndIntervalAgreeOnTheRunningPath(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("clock70@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
	review := b.answer(it, "wrong")
	if m := nextRelRE.FindStringSubmatch(review); m == nil || m[1] != "in 1 day" {
		t.Fatalf("right after answering: %v", m)
	}
	has(t, review, "data-interval>1 day<", "data-interval-from>after this answer<")
	w.advance(20 * time.Hour)
	again := b.mustGet("/study")
	if m := nextRelRE.FindStringSubmatch(again); m == nil || m[1] != "in 4 hours" {
		t.Fatalf("reopened 20 hours later: %v", m)
	}
	has(t, again, "data-interval>1 day<")
	schedule := regexp.MustCompile(`(?s)<dl class="telemetry".*?</dl>`).FindString(again)
	has(t, schedule, "data-next-rel>in 4 hours<")
	hasNot(t, strings.ToLower(schedule), "today")
}

// The schedule carries crisp line icons and a factual trail of verdicts
// (a record, never a streak count).
func TestSlice70_Review_ScheduleIconsAndVerdictTrail(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("trail70@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Close, Reason: ai.CriterionSentence(srs.Close)}}
	review := b.answer(it, "almost")
	for _, name := range []string{"calendar", "interval", "anchor", "gauge", "history"} {
		has(t, review, `class="icon icon-`+name+`"`)
	}
	has(t, review, `data-trail role="img" aria-label="Recent verdicts, oldest first: Almost Correct"`, `data-trail-verdict="close"`)
	hasNot(t, strings.ToLower(visibleText(review)), "streak", "points", "🔥")
}

// Next card is filled in archival ink (not the accent); the focus ring is
// keyboard-only; on phones the six destinations sit in one compact row and
// Study does not repeat its title and purpose under the navigation; the
// review clears the pinned bar.
func TestSlice70_Design_InkNextCardKeyboardRingCompactNav(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("design70@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	review := b.answer(it, "x")
	has(t, review, `<button class="btn btn-ink" data-autofocus-no-scroll data-next>`)
	hasNot(t, review, `btn-accent" data-autofocus-no-scroll`, "autofocus data-next")
	if n := strings.Count(review, "btn-accent"); n != 0 {
		t.Errorf("the review has %d accent-filled controls, want 0 (Next card is ink)", n)
	}
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, css,
		".btn-ink { background: var(--ink); border-color: var(--ink); color: var(--page);",
		".js-focus :focus-visible { outline: none; }",
		".js-focus.kbd-nav :focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }",
		".telemetry {\n  display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));",
		".trail-mark .icon { width: 14px; height: 14px; }")
	if strings.Contains(css, "2563eb") || strings.Contains(strings.ToLower(css), "#2563eb") {
		t.Error("the electric #2563eb is in the style sheet")
	}
	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	phone = phone[:strings.Index(phone, "\n}\n")]
	has(t, phone, "grid-template-columns: repeat(6, minmax(0, 1fr));", ".margin-mode, .margin-purpose { display: none; }",
		".telemetry { grid-template-columns: repeat(2, minmax(0, 1fr)); }")
	hasNot(t, phone, "overflow-x: auto;")
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `root.classList.add("js-focus")`, `root.classList.add("kbd-nav")`, `root.classList.remove("kbd-nav")`,
		"next.focus({ preventScroll: true });", `fetch("/settings/tz"`)
}

// The learner's time zone follows the browser on signed-in pages, so the
// review's dates are on their own clock. CSRF-checked; an unknown zone
// changes nothing.
func TestSlice70_TimeZoneFollowsTheBrowser(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("tz70@example.com")
	page := b.mustGet("/study")
	has(t, page, `data-tz="`, `data-csrf="`)
	if status, _ := b.post("/settings/tz", "tz", "America/Chicago"); status != http.StatusOK {
		t.Fatalf("POST /settings/tz = %d", status)
	}
	if got := w.user("tz70@example.com").TZ; got != "America/Chicago" {
		t.Fatalf("tz = %q", got)
	}
	b.post("/settings/tz", "tz", "Not/AZone")
	if got := w.user("tz70@example.com").TZ; got != "America/Chicago" {
		t.Fatalf("an unknown zone changed tz to %q", got)
	}
	b.csrf = "forged"
	if status, _ := b.post("/settings/tz", "tz", "Europe/Paris"); status == http.StatusOK && w.user("tz70@example.com").TZ == "Europe/Paris" {
		t.Fatal("a forged CSRF token changed the time zone")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
