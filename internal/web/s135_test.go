package web_test

import (
	"regexp"
	"strings"
	"testing"
)

// [S135-1] The landing page is clean on a phone. PRODUCT.md: "the hero
// reads as one clean column: no ruler ticks pinching the sides and no card
// boxed inside another box ... I see one clear action, "Start the 7-day
// trial", across the column; "Sign in" stays where I expect it, once, in
// the header at the top right, and is not repeated beside the trial button
// (on desktop too). The trial terms read "7 days free · no card needed ·
// $5/month after" on one line, and on a narrower screen they break only
// between phrases ... every stage fills the card the way the phone's own
// Study screen does (the prompt, a real-looking answer field, "Return to
// check" with the mic and Check, and the next card coming up)."
//
// The headed proof (390x844 and 1440x900, light and dark: no side ticks or
// side frame lines, 16 px edges, one 48 px action, terms on one line, one
// Specimen height and no blank run over 24 px in any stage) is
// scripts/verify-slice135.mjs.

// tagless strips markup, leaving the text a reader sees.
func tagless(s string) string { return regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, "") }

func TestStory_S135_1_TheLandingPageIsCleanOnAPhone(t *testing.T) {
	w := newWorld(t)
	home := w.browser().mustGet("/")
	hero := home[strings.Index(home, `<section class="hero"`):]
	hero = hero[:strings.Index(hero, "</section>")]
	copyCol := hero[:strings.Index(hero, `<figure class="specimen"`)]

	// One hero action; Sign in only once, in the header.
	if n := strings.Count(copyCol, `class="btn`); n != 1 {
		t.Errorf("the hero copy has %d buttons, want 1 (Start the 7-day trial)", n)
	}
	has(t, copyCol, `<a class="btn btn-accent" href="/signup">Start the 7-day trial</a>`)
	hasNot(t, hero, "Sign in", `href="/signin"`)
	if n := strings.Count(home, ">Sign in<"); n != 1 {
		t.Errorf(`"Sign in" appears %d times on the landing page, want once (the header)`, n)
	}
	header := home[strings.Index(home, `<header class="top`):strings.Index(home, "</header>")]
	has(t, header, `<nav class="nav" aria-label="Account"><a href="/signin">Sign in</a></nav>`)

	// The terms read exactly so, each phrase kept whole, the separator glued
	// to the phrase before it so the line can break only after a " · ".
	m := regexp.MustCompile(`<p class="meta trial-terms-line" data-trial-terms>(.*?)</p>`).FindStringSubmatch(copyCol)
	if m == nil {
		t.Fatal("the hero has no trial terms line")
	}
	if got := tagless(m[1]); got != "7 days free · no card needed · $5/month after" {
		t.Errorf("trial terms = %q", got)
	}
	phrases := regexp.MustCompile(`<span class="nowrap">([^<]*)</span>`).FindAllStringSubmatch(m[1], -1)
	want := []string{"7 days free", " ·", "no card needed", " ·", "$5/month after"}
	if len(phrases) != len(want) {
		t.Fatalf("terms phrases = %q", phrases)
	}
	for i, p := range phrases {
		if p[1] != want[i] {
			t.Errorf("phrase %d = %q, want %q", i, p[1], want[i])
		}
	}
	hasNot(t, home, "if you keep it", "$5 a month")

	// Prompt and Answer carry the phone's picture of Study: the field (the
	// placeholder after a caret, or the typed answer), Return to check, the
	// mic and Check, and one card coming up. It is a picture: hidden from
	// assistive tech and never focusable.
	spec := specimenHTML(t, home)
	prompt := spec[strings.Index(spec, `data-specimen-stage="prompt"`):strings.Index(spec, `data-specimen-stage="answer"`)]
	answer := spec[strings.Index(spec, `data-specimen-stage="answer"`):strings.Index(spec, `data-specimen-stage="judged"`)]
	judged := spec[strings.Index(spec, `data-specimen-stage="judged"`):]
	for name, stage := range map[string]string{"prompt": prompt, "answer": answer} {
		study := stage[strings.Index(stage, `<div class="specimen-study"`):]
		has(t, study, `<div class="specimen-controls" aria-hidden="true">`, `<span class="micro specimen-return">Return to check</span>`,
			`<span class="specimen-mic"><svg`, `<span class="specimen-check">Check</span>`,
			`<p class="specimen-next" aria-hidden="true">NATO alphabet: K</p>`)
		hasNot(t, study, "<button", "<input", "<textarea", "<a ", "tabindex", "btn-accent")
		if !strings.Contains(stage, `class="specimen-line specimen-desk"`) {
			t.Errorf("%s: the desktop answer line is gone; desktop is unchanged by S135-1", name)
		}
	}
	has(t, prompt, `<div class="specimen-field" aria-hidden="true"><span class="specimen-field-caret"></span><span class="specimen-placeholder">Type your answer</span></div>`)
	has(t, answer, `<div class="specimen-field"><span class="specimen-typed">don't assume malice when carelessness or stupidity explains it</span></div>`)
	hasNot(t, judged, "specimen-study")
	// Still one filled accent control on the screen: the trial action.
	if n := strings.Count(home, "btn-accent"); n != 1 {
		t.Errorf("%d filled accent controls, want 1", n)
	}
}

func TestDesign_S135_1_LandingOnPhones(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, cssRule(t, css, ".trial-terms-line"), "text-wrap: balance;")
	has(t, cssRule(t, css, ".specimen-study"), "display: none;")
	// Desktop keeps its ruler ticks and its 1 px frame.
	has(t, cssRule(t, css, ".hero"), "border: 1px solid var(--line);")
	has(t, cssRule(t, css, ".hero::after"), `url("/static/tick.svg") left top`)

	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	phone = phone[:strings.Index(phone, "\n}\n")]
	// The band runs to both screen edges with only top and bottom rules and
	// no ruler ticks; content 24 px top, 32 px bottom, 16 px at the sides.
	has(t, phone, "margin-inline: calc(-1 * var(--s3)); padding: var(--s4) var(--s3) var(--s5);", "border-inline: 0;",
		".hero::after { display: none; }")
	// One full-width action at least 48 px tall, the terms 16 px under it.
	has(t, phone, ".hero-act { gap: var(--s3); justify-items: stretch; }", ".hero-act .actions .btn { flex: 1 1 100%; min-height: var(--s6); }")
	// The Specimen: 16 px inside, 4 px above the steps, parts 12 px apart.
	has(t, phone, ".specimen { padding: var(--s1) var(--s3) var(--s3); gap: 12px; }", ".specimen-stage { gap: 12px; }", ".specimen-desk { display: none; }")
	// The field drawn as the real one, filling what the stage has left.
	field := regexp.MustCompile(`\.specimen-field \{([^}]*)\}`).FindStringSubmatch(phone)
	if field == nil {
		t.Fatal("no phone .specimen-field rule")
	}
	has(t, field[1], "flex: 1 1 auto;", "min-height: 134px;", "padding: 12px var(--s3);", "background: var(--page);",
		"border: 1px solid var(--line);", "border-radius: var(--radius-control);", "font: 400 18px/1.5 var(--text);")
	has(t, phone, ".specimen-field-caret {", "width: 2px;", "background: var(--accent);", ".specimen-placeholder { color: var(--ink-muted); }")
	// Check is bordered, not filled; the mic is the round 44 px control.
	check := regexp.MustCompile(`\.specimen-check \{([^}]*)\}`).FindStringSubmatch(phone)
	if check == nil {
		t.Fatal("no phone .specimen-check rule")
	}
	has(t, check[1], "min-height: var(--touch);", "border: 1px solid var(--ink-ghost);")
	hasNot(t, check[1], "accent")
	has(t, phone, "width: var(--touch); height: var(--touch);", "border-radius: 50%;")
	// One upcoming card under a 1 px rule, one line with an ellipsis.
	has(t, phone, "border-top: 1px solid var(--line);", "overflow: hidden; text-overflow: ellipsis; white-space: nowrap;")
	// Judged: the question recedes to Title in ink-body; answers 8 px apart
	// at Body size inside 12 px; the badge 6 px over a 15 px why.
	has(t, phone, `.specimen [data-specimen-stage="judged"] .specimen-prompt { font: 500 var(--title)/1.25 var(--text); color: var(--ink-body); }`,
		".specimen .compare { gap: var(--s2); padding: 12px 0; }", ".specimen .compare-answer { font-size: var(--body); line-height: 1.35; }",
		".specimen .verdict { flex-direction: column; align-items: flex-start; gap: 6px; }", ".specimen .why { font-size: 15px; line-height: 1.35; }")
}

// [S131-1] on the landing Specimen: the subject header names the deck
// ("Mental models" with its layers icon and "· Set 1") and the count at
// the right, the same in every stage, never with a "Study ·" prefix.
func TestDesign_S131_1_SpecimenSubjectHeader(t *testing.T) {
	w := newWorld(t)
	spec := specimenHTML(t, w.browser().mustGet("/"))
	m := regexp.MustCompile(`<p class="specimen-place" data-subject-header><span class="specimen-deck"><svg class="icon icon-layers"[^>]*aria-hidden="true"[^>]*>.*?</svg><span class="specimen-deck-text"><span class="specimen-deck-name">Mental models</span> <span class="specimen-section">· Set 1</span></span></span> <span class="specimen-count"><span aria-hidden="true">4 / 11</span><span class="sr">Card 4 of 11</span></span></p>`).FindString(spec)
	if m == "" {
		t.Fatal("the Specimen's subject header is not icon + name + section + count")
	}
	if strings.Count(spec, "data-subject-header") != 1 || strings.Index(spec, "data-subject-header") > strings.Index(spec, `class="specimen-stages"`) {
		t.Error("the subject header must be one row above the stages, shared by all three")
	}
	hasNot(t, spec, "Study ·", "Deck:", "This card is from")

	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, cssRule(t, css, ".specimen-place"), "font: 500 var(--reading)/1.4 var(--text); color: var(--ink);", "min-height: var(--s4);")
	has(t, cssRule(t, css, ".specimen-deck .icon"), "width: 16px; height: 16px;", "color: var(--ink);")
	has(t, cssRule(t, css, ".specimen-section"), "var(--mono)", "color: var(--ink-muted);", "white-space: nowrap;")
	has(t, cssRule(t, css, ".specimen-count"), "font-variant-numeric: tabular-nums;", "white-space: nowrap;", "color: var(--ink);")
}
