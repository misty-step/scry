package web_test

import (
	"regexp"
	"strings"
	"testing"
)

// Slice 106, [S106-1] Landing hero and Study Specimen. PRODUCT.md: "the
// landing hero's Study Specimen steps one card from prompt to typed answer
// to judgment, on its own or when I pick a step, holds still while I hover
// or focus it, and shows the judged step still without script or under
// reduced motion. Its four schedule facts each sit beside an icon on one
// line, never wrapped, clipped or pressed against a border. Four ruled rows
// below, each with its icon, say how Scry works."

// specimenHTML is the landing page's Specimen figure.
func specimenHTML(t *testing.T, home string) string {
	t.Helper()
	i := strings.Index(home, `<figure class="specimen"`)
	if i < 0 {
		t.Fatal("the landing page has no Study Specimen")
	}
	spec := home[i:]
	return spec[:strings.Index(spec, "</figure>")+len("</figure>")]
}

// cssRule returns the declarations of the first rule whose selector is
// exactly sel, outside any media query's indentation.
func cssRule(t *testing.T, css, sel string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(sel) + ` \{([^}]*)\}`).FindStringSubmatch(css)
	if m == nil {
		t.Fatalf("the style sheet has no %s rule", sel)
	}
	return m[1]
}

func TestS106_1_LandingSpecimenStepsAndRestsOnJudged(t *testing.T) {
	w := newWorld(t)
	home := w.browser().mustGet("/")
	spec := specimenHTML(t, home)

	// Three mono step controls, real buttons, in order, with Judged the
	// current one. They are inert until the script wires them (no dead
	// controls without script).
	steps := regexp.MustCompile(`<button type="button" class="specimen-step" data-specimen-step="([a-z]+)" aria-pressed="(true|false)" disabled>([^<]+)</button>`).FindAllStringSubmatch(spec, -1)
	if len(steps) != 3 {
		t.Fatalf("%d step controls, want 3", len(steps))
	}
	for i, want := range [][3]string{{"prompt", "false", "1 · Prompt"}, {"answer", "false", "2 · Answer"}, {"judged", "true", "3 · Judged"}} {
		if steps[i][1] != want[0] || steps[i][2] != want[1] || steps[i][3] != want[2] {
			t.Errorf("step %d = %q, want %q", i+1, steps[i][1:], want)
		}
	}

	// Three stages in one cell; the server shows Judged, so without script
	// (and under reduced motion, which never advances) it is complete.
	stages := regexp.MustCompile(`<div class="specimen-stage( is-shown)?" data-specimen-stage="([a-z]+)">`).FindAllStringSubmatch(spec, -1)
	if len(stages) != 3 || stages[0][2] != "prompt" || stages[1][2] != "answer" || stages[2][2] != "judged" {
		t.Fatalf("stages = %q", stages)
	}
	for _, s := range stages {
		if (s[1] != "") != (s[2] == "judged") {
			t.Errorf("stage %s shown = %v; only Judged is shown by the server", s[2], s[1] != "")
		}
	}
	prompt := spec[strings.Index(spec, `data-specimen-stage="prompt"`):strings.Index(spec, `data-specimen-stage="answer"`)]
	answer := spec[strings.Index(spec, `data-specimen-stage="answer"`):strings.Index(spec, `data-specimen-stage="judged"`)]
	judged := spec[strings.Index(spec, `data-specimen-stage="judged"`):]
	// [S131-1] / [S135-1]: the subject header, never a "Study ·" prefix.
	has(t, spec, `<span class="specimen-deck-name">Mental models</span> <span class="specimen-section">· Set 1</span>`,
		`<span class="specimen-count"><span aria-hidden="true">4 / 11</span><span class="sr">Card 4 of 11</span></span>`)
	hasNot(t, spec, "Study · ")
	has(t, prompt, `<p class="specimen-prompt">Hanlon's razor</p>`, `class="specimen-line specimen-desk"`, `class="specimen-caret"`)
	hasNot(t, prompt, "assume malice")
	has(t, answer, `<span class="specimen-typed">don't assume malice when carelessness or stupidity explains it</span>`)
	has(t, judged, "Your answer", "Correct answer", `verdict-badge verdict-right`, `class="icon icon-verdict-right"`, ">Correct</span>",
		"Same meaning as the correct answer.", `class="specimen-schedule"`)

	// The script wires the steps and the loop, and respects reduced motion.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `document.querySelector("[data-specimen]")`, `"(prefers-reduced-motion: reduce)"`, "every = 4000",
		`"mouseenter"`, `"focusin"`, "chosen = true", "b.disabled = false", `setAttribute("aria-pressed"`)

	// One fixed-height cell; a 200 ms ease-out cross-fade; 44 px steps with
	// the current one underlined; the panel spec.
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, cssRule(t, css, ".specimen-stage"), "grid-area: 1 / 1;", "visibility: hidden; opacity: 0;", "transition: opacity 200ms var(--ease)")
	has(t, cssRule(t, css, ".specimen-stage.is-shown"), "visibility: visible; opacity: 1;")
	has(t, cssRule(t, css, ".specimen-step"), "min-height: var(--touch);", "var(--mono)")
	has(t, cssRule(t, css, `.specimen-step[aria-pressed="true"]`), "text-decoration: underline;")
	has(t, cssRule(t, css, ".specimen"), "padding: var(--s4) 28px 28px;", "background: var(--raised);", "border: 1px solid var(--line);", "border-radius: var(--radius-surface);")
	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	phone = phone[:strings.Index(phone, "\n}\n")]
	has(t, phone, ".specimen { padding: var(--s1) var(--s3) var(--s3); gap: 12px; }")

	// The hero grid and its drafting band: sunken wash, 1 px line frame,
	// the dot grid and ruler ticks; one column at 760 px and below.
	hero := cssRule(t, css, ".hero")
	has(t, hero, "grid-template-columns: minmax(0, 1fr) minmax(0, 1.08fr);", "gap: var(--s6);", "background: var(--sunken);", "border: 1px solid var(--line);")
	hasNot(t, hero, "gradient", "shadow")
	has(t, cssRule(t, css, ".hero::before"), `mask: url("/static/grid.svg")`, "opacity: var(--grid-opacity);")
	has(t, cssRule(t, css, ".hero::after"), `url("/static/tick.svg") left top`, `url("/static/tick.svg") right top`, `url("/static/tick-major.svg")`)
	has(t, phone, "grid-template-columns: minmax(0, 1fr); gap: var(--s5);")
	if svg, _ := staticAsset(t, w, "/static/tick-major.svg"); !strings.Contains(svg, `<svg`) {
		t.Error("the long ruler tick is missing")
	}
}

func TestS106_1_SpecimenScheduleGridCSS(t *testing.T) {
	w := newWorld(t)
	spec := specimenHTML(t, w.browser().mustGet("/"))

	// Four facts, each with its 14 px icon and label over the value.
	facts := regexp.MustCompile(`<div class="specimen-fact"><dt><svg class="icon icon-([a-z]+)"[^>]*>.*?</svg>([^<]+)</dt><dd>([^<]+)</dd></div>`).FindAllStringSubmatch(spec, -1)
	want := [][3]string{{"calendar", "Next review", "in 3 days"}, {"interval", "Interval", "3 days"}, {"anchor", "Stability", "3.4 days"}, {"gauge", "Difficulty", "4.9 / 10"}}
	if len(facts) != len(want) {
		t.Fatalf("%d schedule facts, want 4: %q", len(facts), facts)
	}
	for i, f := range facts {
		if f[1] != want[i][0] || f[2] != want[i][1] || f[3] != want[i][2] {
			t.Errorf("fact %d = %q, want %q", i, f[1:], want[i])
		}
	}
	// Scoped: the post-answer review's table is not reused or restyled.
	hasNot(t, spec, `class="telemetry`, `class="fact`)

	css, _ := staticAsset(t, w, "/static/app.css")
	grid := cssRule(t, css, ".specimen-schedule")
	has(t, grid, "grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0;", "border-top: 1px solid var(--line); border-left: 1px solid var(--line);")
	cell := cssRule(t, css, ".specimen-fact")
	has(t, cell, "padding: 14px var(--s3);", "border-right: 1px solid var(--line); border-bottom: 1px solid var(--line);", "min-width: 0;")
	has(t, cssRule(t, css, ".specimen-fact dt"), "white-space: nowrap;", "gap: 6px;")
	has(t, cssRule(t, css, ".specimen-fact dt .icon"), "width: 14px; height: 14px;")
	has(t, cssRule(t, css, ".specimen-fact dd"), "color: var(--ink);", "font-variant-numeric: tabular-nums;", "white-space: nowrap;")
	// Phones keep the 2x2 grid (no override) with 12 px cell padding.
	phone := css[strings.Index(css, "@media (max-width: 760px)"):]
	phone = phone[:strings.Index(phone, "\n}\n")]
	has(t, phone, ".specimen-fact { padding: 12px; }")
	hasNot(t, phone, ".specimen-schedule {")
	// The review's own table is untouched: four across on desktop.
	has(t, cssRule(t, css, ".telemetry"), "grid-template-columns: repeat(4, minmax(0, 1fr));")
}

func TestS106_1_HowScryWorksRowsHaveIcons(t *testing.T) {
	w := newWorld(t)
	home := w.browser().mustGet("/")
	rows := home[strings.Index(home, `<section class="plain-list" aria-label="How Scry works">`):]
	rows = rows[:strings.Index(rows, "</section>")]
	got := regexp.MustCompile(`<div class="plain-row"><h2 class="title plain-title"><svg class="icon icon-([a-z-]+)"[^>]*aria-hidden="true"[^>]*>.*?</svg>([^<]+)</h2><p>([^<]+)</p></div>`).FindAllStringSubmatch(rows, -1)
	if len(got) != 4 {
		t.Fatalf("%d How Scry works rows with an icon, want 4", len(got))
	}
	for i, icon := range []string{"clock", "verdict-right", "books", "lock"} {
		if got[i][1] != icon {
			t.Errorf("row %d (%s) has the %s icon, want %s", i+1, got[i][2], got[i][1], icon)
		}
	}
	has(t, got[0][3], "typing or speaking")
	has(t, got[1][3], "AI grader", "understanding", "word for word")
	has(t, got[2][3], "Thirty starter sets", "Markdown", "import a deck from Anki, CSV or Markdown")
	has(t, got[3][3], "private", "export", "Markdown, CSV or JSON", "scripts or tools", "API token", "straight into your library")
	// No Inbox and no approval queue any more (S106-4).
	hasNot(t, rows, "Inbox", "approve", "suggest", "agents")

	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, cssRule(t, css, ".plain-title .icon"), "width: 20px; height: 20px;")
}
