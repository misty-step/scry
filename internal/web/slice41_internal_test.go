package web

import (
	"encoding/xml"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"scry/internal/srs"
	"scry/internal/starter"
	"scry/internal/store"
)

// Slice 41, item 1: the guard that keeps answers off the Study rail.
func TestSlice41_Spoiler_SafeHeaderGuard(t *testing.T) {
	nested := func(topic, title string, n int) store.IdeaPlace {
		return store.IdeaPlace{Title: title, Topic: topic, Nested: true, Ordinal: n}
	}
	top := func(title string) store.IdeaPlace { return store.IdeaPlace{Title: title, Topic: title} }
	nato := []string{"Alfa", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India"}
	for _, c := range []struct {
		name    string
		place   store.IdeaPlace
		answers []string
		want    string
	}{
		{"the old leaking section title never shows", nested("NATO phonetic alphabet", "Alfa to India", 1), nato, "NATO phonetic alphabet · Set 1"},
		{"the neutral section title does not show either", nested("NATO phonetic alphabet", "Letters J–R", 2), []string{"Juliett", "Romeo"}, "NATO phonetic alphabet · Set 2"},
		{"a topic naming the current answer is dropped", nested("Alfa and friends", "Section", 3), nato, "Set 3"},
		{"a topic naming a sibling's answer is dropped", nested("From Hotel to home", "Section", 1), nato, "Set 1"},
		{"case and punctuation do not hide a leak", nested("the INDIA-file", "Section", 2), nato, "Set 2"},
		{"a plural still leaks its singular", nested("Capitals", "Section", 1), []string{"The capital"}, "Set 1"},
		{"a number leaks", top("The year 1066"), []string{"1066"}, "Current set"},
		{"a top-level idea is its own topic", top("World capitals"), []string{"Oslo"}, "World capitals"},
		{"a top-level title holding the answer", top("Oslo, capital of Norway"), []string{"Oslo"}, "Current set"},
		{"AI-drafted idea titled with its answer", nested("Meditations", "The dichotomy of control", 1), []string{"Meditations by Marcus Aurelius"}, "Set 1"},
		{"common words are not answers", top("The art of the deal"), []string{"The one and the other"}, "The art of the deal"},
		{"a word inside a longer word is not that word", top("NATO phonetic alphabet"), []string{"Alpha", "Net"}, "NATO phonetic alphabet"},
		{"no topic at all", store.IdeaPlace{}, []string{"x"}, "Current set"},
		{"a set number that is itself an answer word gives way to the topic", nested("Kitchen numbers", "Spoons and cups", 3), []string{"3 teaspoons"}, "Kitchen numbers"},
		{"the word set in an answer gives way to the topic", nested("Core ideas in mathematics", "Functions", 2), []string{"A rule from one set to another"}, "Core ideas in mathematics"},
		{"topic and set number both taken", nested("Teaspoons", "Spoons", 3), []string{"3 teaspoons"}, "Current set"},
		{"every label taken: no place is named", nested("Teaspoons", "Spoons", 3), []string{"3 teaspoons", "The current set"}, ""},
	} {
		if got := spoilerSafeHeader(c.place, c.answers); got != c.want {
			t.Errorf("%s: header = %q, want %q", c.name, got, c.want)
		}
		// Whatever the label, it holds no answer word.
		if got := spoilerSafeHeader(c.place, c.answers); leaksAnswer(got, c.answers) {
			t.Errorf("%s: the safe header %q leaks an answer", c.name, got)
		}
	}
	if got := revealedHeader(nested("NATO phonetic alphabet", "Letters A–I", 1)); got != "NATO phonetic alphabet · Letters A–I" {
		t.Errorf("revealed header = %q", got)
	}
	if got := revealedHeader(top("World capitals")); got != "World capitals" {
		t.Errorf("revealed top-level header = %q", got)
	}
}

// The safe header holds for every starter set as shipped: for each idea,
// the label shown while its cards wait shares no word with their answers.
func TestSlice41_Spoiler_EveryStarterSetHasASafeHeader(t *testing.T) {
	for _, set := range starter.All() {
		for n, idea := range set.Ideas {
			var answers []string
			for _, it := range idea.Items {
				answers = append(answers, it.A)
			}
			if idea.Exact != nil {
				answers = append(answers, idea.Exact.Lines...)
			}
			label := spoilerSafeHeader(store.IdeaPlace{Title: idea.Title, Topic: set.Title, Nested: true, Ordinal: n + 1}, answers)
			if leaksAnswer(label, answers) || strings.Contains(label, idea.Title) {
				t.Errorf("%s / %s: the study label %q can spoil", set.Slug, idea.Title, label)
			}
		}
	}
}

func TestSlice41_Spoiler_UpcomingCueThatQuotesTheAnswerIsHeldBack(t *testing.T) {
	for _, c := range []struct {
		text, answer string
		want         bool
	}{
		{"Apostles' Creed: what follows “I believe in God, the Father almighty,”", "I believe in God, the Father almighty,", true},
		{"What does inversion ask you to list first?", "Inversion", true},
		{"NATO alphabet: B", "Alfa", false},
		{"NATO alphabet: B", "A", false}, // too short to be a quotation
		{"The alphabetical order", "Alpha", false},
	} {
		if got := quotesAnswer(c.text, c.answer); got != c.want {
			t.Errorf("quotesAnswer(%q, %q) = %v, want %v", c.text, c.answer, got, c.want)
		}
	}
}

// Slice 41, item 2: the review's facts are formatted from the stored
// schedule and history, in the learner's own time zone.
func TestSlice41_Telemetry_IsFormattedFromTheStoredSchedule(t *testing.T) {
	u := store.User{TZ: "America/New_York"}
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	it := store.Item{Sched: srs.State{Stability: 3.7145, Difficulty: 5.1618, Reps: 3, Lapses: 1, Last: now, Due: now.Add(4 * 24 * time.Hour)}}
	got := telemetry(u, it, store.ReviewCounts{Total: 3, Right: 2, Close: 0, Missed: 1, Practice: 1}, now)
	want := telemetryView{NextDate: "Nov 26", NextRel: "in 4 days", NextISO: "2026-11-26", Interval: "4 days", Stability: "3.7 days",
		Difficulty: "5.2 / 10", Reviews: "3 · 2 correct · 0 almost · 1 incorrect", Total: 3, Lapses: 1, Practice: 1}
	if got != want {
		t.Errorf("telemetry = %+v\nwant        %+v", got, want)
	}
	// A missed card comes back tomorrow, one day out.
	it.Sched.Due = now.Add(24 * time.Hour)
	if got := telemetry(u, it, store.ReviewCounts{Total: 1, Missed: 1}, now); got.NextRel != "tomorrow" || got.Interval != "1 day" || got.NextDate != "Nov 23" {
		t.Errorf("missed card: %+v", got)
	}
	// Across the new year the date carries its year.
	it.Sched.Due = now.Add(60 * 24 * time.Hour)
	if got := telemetry(u, it, store.ReviewCounts{Total: 1, Right: 1}, now); got.NextDate != "Jan 21, 2027" || got.NextRel != "in 60 days" {
		t.Errorf("next year: %+v", got)
	}
	for v, want := range map[srs.Verdict][2]string{srs.Right: {"Correct", "verdict-right"}, srs.Close: {"Almost Correct", "verdict-close"}, srs.Missed: {"Incorrect", "verdict-missed"}} {
		if label, icon := verdictBadge(v); label != want[0] || icon != want[1] {
			t.Errorf("verdictBadge(%s) = %q, %q", v, label, icon)
		}
	}
}

// Slice 41, item 9: one icon family, drawn as DESIGN.md says.
func TestSlice41_Icons_OneFamilyOfInlineLineIcons(t *testing.T) {
	for _, name := range iconNames() {
		svg := string(icon(name))
		for _, want := range []string{
			`viewBox="0 0 24 24"`, `fill="none"`, `stroke="currentColor"`, `stroke-width="1.75"`,
			`stroke-linecap="round"`, `stroke-linejoin="round"`, `aria-hidden="true"`, `focusable="false"`, `class="icon icon-` + name + `"`,
		} {
			if !strings.Contains(svg, want) {
				t.Errorf("icon %s is missing %s", name, want)
			}
		}
		if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
			t.Errorf("icon %s is not well-formed: %v", name, err)
		}
		inner := iconPaths[name]
		for _, bad := range []string{"fill=", "style=", "<text", "<image", "href", "http", "#", "<script", "gradient", "filter"} {
			if strings.Contains(inner, bad) {
				t.Errorf("icon %s contains %q: line icons are stroked paths only", name, bad)
			}
		}
		// Clean geometry on the 24 px grid: every coordinate within it.
		for _, n := range regexp.MustCompile(`(?:x|y|cx|cy)="([0-9.]+)"`).FindAllStringSubmatch(inner, -1) {
			if len(n[1]) > 5 {
				t.Errorf("icon %s has an untidy coordinate %s", name, n[1])
			}
		}
	}
	if icon("no-such-icon") != "" {
		t.Error("an unknown icon should render nothing")
	}
	for _, name := range starter.Icons {
		if _, ok := iconPaths[name]; !ok {
			t.Errorf("starter icon %q is not drawn", name)
		}
	}
	for _, name := range []string{"cards", "library", "inbox", "create", "billing", "settings", "plus", "check", "arrow", "help", "pencil",
		"calendar", "spinner", "verdict-right", "verdict-close", "verdict-missed"} {
		if _, ok := iconPaths[name]; !ok {
			t.Errorf("interface icon %q is not drawn", name)
		}
	}
	// Study and Create do not share a glyph.
	if iconPaths["cards"] == iconPaths["create"] {
		t.Error("Study and Create use the same icon")
	}
}

// Every icon a template asks for exists, and no template uses emoji or an
// icon font in place of one.
func TestSlice41_Icons_TemplatesUseOnlyDrawnIconsAndNoEmoji(t *testing.T) {
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil || len(names) == 0 {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`\{\{icon "([a-z-]+)"\}\}`)
	used := map[string]bool{}
	for _, name := range names {
		raw, _ := templateFS.ReadFile(name)
		for _, m := range call.FindAllStringSubmatch(string(raw), -1) {
			used[m[1]] = true
			if _, ok := iconPaths[m[1]]; !ok {
				t.Errorf("%s asks for the icon %q, which is not drawn", name, m[1])
			}
		}
		for _, r := range string(raw) {
			// Emoji and pictographs, dingbats, and the private use area icon fonts live in.
			if (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0xE000 && r <= 0xF8FF) || r == 0xFE0F {
				t.Errorf("%s uses %q (U+%04X) as interface", name, string(r), r)
			}
		}
	}
	for _, name := range []string{"cards", "library", "inbox", "create", "billing", "settings", "plus", "check", "arrow", "help", "pencil", "calendar", "spinner"} {
		if !used[name] {
			t.Errorf("no template uses the icon %q", name)
		}
	}
}

// Slice 41, item 9: the anti-slop rules of DESIGN.md hold in the style
// sheet and scripts: Geist only, served from Scry, with no glows,
// gradients, shadows or third-party font hosts.
func TestSlice41_Design_AntiSlopRulesHoldInTheStyleSheet(t *testing.T) {
	raw, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	lower := strings.ToLower(css)
	for _, bad := range []string{"inter\"", "'inter'", "font-family: inter", "fonts.googleapis", "fonts.gstatic", "typekit", "@import",
		"linear-gradient", "radial-gradient", "conic-gradient", "box-shadow", "text-shadow", "drop-shadow", "background-clip: text",
		"-webkit-background-clip", "backdrop-filter", "blur(", "violet", "purple", "#8b5cf6", "#7c3aed", "#a855f7", "#6366f1"} {
		if strings.Contains(lower, bad) {
			t.Errorf("app.css contains %q", bad)
		}
	}
	faces := regexp.MustCompile(`font-family:\s*"([^"]+)"`).FindAllStringSubmatch(css, -1)
	for _, f := range faces {
		if f[1] != "Geist" && f[1] != "Geist Mono" {
			t.Errorf("a third typeface is declared: %q", f[1])
		}
	}
	for _, u := range regexp.MustCompile(`url\(([^)]+)\)`).FindAllStringSubmatch(css, -1) {
		if !strings.HasPrefix(strings.Trim(u[1], `"'`), "/static/") {
			t.Errorf("app.css loads %s from outside Scry", u[1])
		}
	}
	for _, want := range []string{
		`.icon { width: 18px; height: 18px;`, `.nav .icon { width: 16px; height: 16px; }`, `.tile-title .icon { width: 20px; height: 20px;`,
		`.num { font-variant-numeric: tabular-nums;`, `.verdict-right { color: var(--right); }`, `.verdict-missed { color: var(--missed); }`,
		`.shelf-bar {`, `position: sticky; bottom: 0;`, `[hidden] { display: none !important; }`,
		`.btn:active { transform: translateY(1px); }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css is missing %q", want)
		}
	}
	// The only looping marks are the two processing ones, and reduced
	// motion stills everything.
	if n := strings.Count(css, "infinite"); n != 2 {
		t.Errorf("%d looping animations, want the spinner and the progress line only", n)
	}
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce)") || !strings.Contains(css, "animation: none !important;") {
		t.Error("reduced motion no longer stills animations")
	}
	// Scripts: the in-place add, the edit and the processing state are there,
	// and nothing is loaded from elsewhere.
	js, _ := staticFS.ReadFile("static/app.js")
	for _, want := range []string{"data-add", `"Accept": "application/json"`, "form.replaceWith(tag)", "data-edit-form", "window.scryBusy", "busyStart", "busyStop", `e.key === " " && !nativeControl && next`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("app.js is missing %q", want)
		}
	}
	for _, bad := range []string{"location.reload", "scrollTo(0", "window.scroll("} {
		if strings.Contains(string(js), bad) {
			t.Errorf("app.js contains %q", bad)
		}
	}
	stripe, _ := staticFS.ReadFile("static/stripe.js")
	if strings.Count(string(stripe), "giveBack()") < 4 || !strings.Contains(string(stripe), "window.scryBusy.start(form)") {
		t.Error("stripe.js does not start the processing state and give the form back on every error path")
	}
}
