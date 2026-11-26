package web_test

import (
	"html"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Slice 138 (Active Folio b8876196): the landing headline "Remember
// everything."

// PRODUCT.md [S138-1]: "As a newcomer opening https://scry.mistystep.io, the
// first thing I read is the headline "Remember everything." (sentence case,
// ending in a full stop), and the browser tab and link previews read
// "Remember everything · scry". That is the only wording that changes: "Keep
// what's worth knowing." no longer appears anywhere a visitor can see it ...
// while the lede, the "Start the 7-day trial" action, the trial terms line,
// the Study Specimen, the four rows below and the site's description ...
// stay as they are."
func TestStory_S138_1_TheLandingPageSaysRememberEverything(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	home := b.mustGet("/")

	// The page's one h1 is the new headline, in Display type.
	if n := strings.Count(home, "<h1"); n != 1 {
		t.Errorf("the landing page has %d h1 elements, want exactly 1", n)
	}
	has(t, home, `<h1 class="display">Remember everything.</h1>`)

	// The browser tab and link previews.
	has(t, home, "<title>Remember everything · scry</title>",
		`<meta property="og:title" content="Remember everything · scry">`)

	// The old headline is gone from everything a visitor can see, in any
	// escaping.
	for _, form := range []string{"Keep what's worth knowing", "Keep what&#39;s worth knowing", "Keep what&rsquo;s worth knowing"} {
		hasNot(t, home, form)
	}
	if strings.Contains(html.UnescapeString(home), "Keep what's worth knowing") {
		t.Error("the landing page still contains \"Keep what's worth knowing\"")
	}

	// Everything else in the hero and the head stays.
	has(t, home,
		"Scry is active recall with spaced repetition",
		`<a class="btn btn-accent" href="/signup">Start the 7-day trial</a>`,
		`<figure class="specimen"`,
		`<meta name="description" content="Scry gets what's worth knowing into your head and keeps it there, for the fewest minutes a day.">`,
		`<meta property="og:description" content="Scry gets what's worth knowing into your head and keeps it there, for the fewest minutes a day.">`)
}

// DESIGN.md, Landing [S138-1]: the headline is Display, sentence case with
// its full stop, "in the hero's existing 16ch headline measure, so it sets
// as two lines". The measure and the Display face are unchanged; the
// headline itself is what the template renders.
func TestDesign_S138_1_HeadlineKeepsDisplayAndItsMeasure(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`\.hero \.display \{ max-width: 16ch; \}`).Match(css) {
		t.Error("the hero headline lost its 16ch measure")
	}
	if !regexp.MustCompile(`\.display \{ font: 500 var\(--display\)/1\.1 var\(--text\);`).Match(css) {
		t.Error("the Display type role changed")
	}
	tpl, err := os.ReadFile("templates/landing.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tpl), `<h1 class="display">Remember everything.</h1>`) {
		t.Error("landing.html does not render the S138-1 headline")
	}
}
