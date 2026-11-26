package web

import (
	"encoding/xml"
	"io/fs"
	"reflect"
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
		{"NATO alphabet: B", "A", false},
		{"The alphabetical order", "Alpha", false},
		// Pass 2 (B1): an answer is quoted whatever its length.
		{"Studio notes: what follows “Go”", "Go", true},
		{"Studio notes: what follows “Go”", "go!", true},
		{"Scale degrees: what follows “A”", "A", true},
		{"Counting: what follows “7”", "7", true},
		{"Gothic architecture: how does it begin?", "Go", false}, // whole words only
		{"Studio notes: how does it begin?", "Go", false},
		{"Daffodils: how does it begin?", "The daffodils", true}, // a leading article hides nothing
		{"Daffodils: how does it begin?", "A", false},
		{"A pause: what follows “* * *”", "* * *", true}, // no letter or digit: matched as written
		{"A pause: how does it begin?", "* * *", false},
		{"Anything at all", "", false},
	} {
		if got := quotesAnswer(c.text, c.answer); got != c.want {
			t.Errorf("quotesAnswer(%q, %q) = %v, want %v", c.text, c.answer, got, c.want)
		}
	}
}

// Pass 2 (B1): a line of an exact text whose own title gives the line away
// is asked for under a neutral title, in the label and in the prompt, and
// the real title comes back with the review.
func TestSlice41_Spoiler_ExactTitleThatGivesTheLineAwayIsNeutralWhileWaiting(t *testing.T) {
	line := func(title string, n int, prev, answer string) store.Item {
		return store.Item{Exact: true, TextTitle: title, LineNo: n, Prompt: store.ExactPrompt(title, n, prev), Answer: answer}
	}
	editedLine := func(answer string) store.Item {
		return store.Item{Exact: true, TextTitle: "Studio notes", LineNo: 1, Prompt: "Recall " + answer + "?", Answer: answer}
	}
	for _, c := range []struct {
		name                      string
		it                        store.Item
		label, prompt, labelAfter string
	}{
		{"the title is the line", line("Daffodils", 1, "", "Daffodils"),
			"Exact text · line 1", "Exact text: how does it begin?", "Exact text · Daffodils · line 1"},
		{"the title holds the line", line("Go west, young man", 1, "", "Go west"),
			"Exact text · line 1", "Exact text: how does it begin?", "Exact text · Go west, young man · line 1"},
		{"a later line that is the title", line("Daffodils", 3, "And dances with the", "daffodils."),
			"Exact text · line 3", "Exact text: what follows “And dances with the”", "Exact text · Daffodils · line 3"},
		{"case, punctuation and a leading article do not hide it", line("DAFFODILS!", 1, "", "The daffodils"),
			"Exact text · line 1", "Exact text: how does it begin?", "Exact text · DAFFODILS! · line 1"},
		{"a short title that is the line", line("Go", 1, "", "Go"),
			"Exact text · line 1", "Exact text: how does it begin?", "Exact text · Go · line 1"},
		{"a title that only shares words with the line stays: it is the cue", line("Preamble to the United States Constitution", 1, "", "We the People of the United States,"),
			"Exact text · Preamble to the United States Constitution · line 1", "Preamble to the United States Constitution: how does it begin?",
			"Exact text · Preamble to the United States Constitution · line 1"},
		{"a safe title stays", line("Studio notes", 1, "", "Go"),
			"Exact text · Studio notes · line 1", "Studio notes: how does it begin?", "Exact text · Studio notes · line 1"},
		{"a hand-written prompt that names the title gives way to the line number",
			store.Item{Exact: true, TextTitle: "Daffodils, by Wordsworth", LineNo: 2, Prompt: "Second line of Daffodils?", Answer: "Daffodils"},
			"Exact text · line 2", "Line 2 — recite from memory", "Exact text · Daffodils, by Wordsworth · line 2"},
		{"a hand-written prompt that does not name the title stays",
			store.Item{Exact: true, TextTitle: "Oslo", Prompt: "Capital of Norway, spelled exactly", Answer: "Oslo"},
			"Exact text", "Capital of Norway, spelled exactly", "Exact text · Oslo"},
		{"a hand-written prompt naming the title, with no line number",
			store.Item{Exact: true, TextTitle: "Oslo", Prompt: "Spell Oslo", Answer: "Oslo"},
			"Exact text", "Recite this line from memory", "Exact text · Oslo"},
		{"the neutral title is itself the answer", line("Exact text", 1, "", "Exact text"),
			"Line 1", "Line 1 — recite from memory", "Exact text · Exact text · line 1"},
		{"the edited prompt repeats the answer after the title prefix",
			store.Item{Exact: true, TextTitle: "Daffodils", LineNo: 1, Prompt: "Daffodils: what is the first line of Daffodils?", Answer: "Daffodils"},
			"Exact text · line 1", "Line 1 — recite from memory", "Exact text · Daffodils · line 1"},
		{"the edited prompt quotes the answer even though the title is safe",
			store.Item{Exact: true, TextTitle: "Studio notes", LineNo: 1, Prompt: "What's after Go?", Answer: "Go"},
			"Exact text · Studio notes · line 1", "Line 1 — recite from memory", "Exact text · Studio notes · line 1"},
		{"the first prompt fallback quotes the answer", editedLine("From memory"),
			"Exact text · Studio notes · line 1", "Recite line 1", "Exact text · Studio notes · line 1"},
		{"the line number is the answer", editedLine("1"),
			"", "Recite this line from memory", "Exact text · Studio notes · line 1"},
		{"even the numberless recitation instruction quotes the answer", editedLine("Recite"),
			"Exact text · Studio notes · line 1", "Next line", "Exact text · Studio notes · line 1"},
		{"every worded candidate quotes the answer", editedLine("Line"),
			"", "…", "Exact text · Studio notes · line 1"},
	} {
		if got := exactLabel(c.it, false, false); got != c.label {
			t.Errorf("%s: waiting label = %q, want %q", c.name, got, c.label)
		}
		if got := waitingPrompt(c.it); got != c.prompt {
			t.Errorf("%s: waiting prompt = %q, want %q", c.name, got, c.prompt)
		}
		if got := exactLabel(c.it, true, false); got != c.labelAfter {
			t.Errorf("%s: label in the review = %q, want %q", c.name, got, c.labelAfter)
		}
	}
	if got := exactLabel(line("Studio notes", 1, "", "Again"), false, true); got != "" {
		t.Errorf("retry label = %q, want no label when the retry suffix is the answer", got)
	}
	// An ordinary card is never touched, whatever its idea is called.
	plain := store.Item{Prompt: "Which flower did Wordsworth write of?", Answer: "Daffodils", TextTitle: "Daffodils"}
	if exactLabel(plain, false, false) != "" || waitingPrompt(plain) != plain.Prompt {
		t.Error("a card that is not a line of an exact text was changed")
	}
}

// Pass 2 (B1): what the scrollback and the "coming up" list may print
// while an answer is owed.
func TestSlice41_Spoiler_CuesShownWhileAnAnswerIsOwed(t *testing.T) {
	line := func(title string, n int, prev string) store.Item {
		return store.Item{Exact: true, TextTitle: title, LineNo: n, Prompt: store.ExactPrompt(title, n, prev)}
	}
	for _, c := range []struct {
		name   string
		it     store.Item
		answer string
		cue    string
		ok     bool
	}{
		{"the next line quotes a short answer: held back", line("Studio notes", 2, "Go"), "Go", "", false},
		{"a later line does not: shown", line("Studio notes", 3, "Stay the course"), "Go", "Studio notes: what follows “Stay the course”", true},
		{"the next line of a text named for its line: held back", line("Daffodils", 2, "Daffodils"), "Daffodils", "", false},
		{"a later line of it: shown under a neutral title", line("Daffodils", 3, "I wandered lonely as a cloud"), "Daffodils",
			"Exact text: what follows “I wandered lonely as a cloud”", true},
		{"its first line, already answered: neutral title", line("Daffodils", 1, ""), "Daffodils", "Exact text: how does it begin?", true},
		{"an ordinary cue that quotes the answer: held back", store.Item{Prompt: "What does inversion ask you to list first?"}, "Inversion", "", false},
		{"an ordinary cue: shown", store.Item{Prompt: "NATO alphabet: B"}, "Alfa", "NATO alphabet: B", true},
	} {
		if cue, ok := cueWhile(c.it, c.answer); cue != c.cue || ok != c.ok {
			t.Errorf("%s: cueWhile = %q, %v; want %q, %v", c.name, cue, ok, c.cue, c.ok)
		}
		if cue, ok := cueWhile(c.it, c.answer); ok && quotesAnswer(cue, c.answer) {
			t.Errorf("%s: the cue %q quotes the answer %q", c.name, cue, c.answer)
		}
	}
}

// Pass 2 (B1): no starter text loses its title. The title is the cue that
// says which text is asked for; none of them holds a whole line.
func TestSlice41_Spoiler_EveryStarterExactTextKeepsItsTitle(t *testing.T) {
	texts := 0
	for _, set := range starter.All() {
		for _, idea := range set.Ideas {
			if idea.Exact == nil {
				continue
			}
			texts++
			prev := ""
			for n, l := range idea.Exact.Lines {
				it := store.Item{Exact: true, TextTitle: idea.Exact.Title, LineNo: n + 1, Prompt: store.ExactPrompt(idea.Exact.Title, n+1, prev), Answer: l}
				if got := waitingPrompt(it); got != it.Prompt {
					t.Errorf("%s line %d: the waiting prompt is %q, want %q", idea.Exact.Title, n+1, got, it.Prompt)
				}
				if got := exactLabel(it, false, false); !strings.Contains(got, idea.Exact.Title) {
					t.Errorf("%s line %d: the waiting label %q lost the title", idea.Exact.Title, n+1, got)
				}
				prev = l
			}
		}
	}
	if texts == 0 {
		t.Fatal("no starter exact texts were checked")
	}
}

// Slice 41, item 2 (as refined by Slice 70): the review's facts are
// formatted from the stored schedule and history, in the learner's own
// time zone, with the event and the interval on one clock.
func TestSlice41_Telemetry_IsFormattedFromTheStoredSchedule(t *testing.T) {
	u := store.User{TZ: "America/New_York"}
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	it := store.Item{Sched: srs.State{Stability: 3.7145, Difficulty: 5.1618, Reps: 3, Lapses: 1, Last: now, Due: now.Add(4 * 24 * time.Hour)}}
	history := []store.Review{{Verdict: srs.Right}, {Verdict: srs.Missed, Practice: true}, {Verdict: srs.Missed}, {Verdict: srs.Right}}
	got := telemetry(u, it, store.ReviewCounts{Total: 3, Right: 2, Close: 0, Missed: 1, Practice: 1}, history, false, now)
	want := telemetryView{NextDate: "Thu, Nov 26", NextRel: "in 4 days", NextISO: "2026-11-26T10:00:00-05:00", Interval: "4 days", After: "after this answer",
		Stability: "3.7 days", Difficulty: "5.2 / 10", Reviews: "3 · 2 correct · 0 almost · 1 incorrect", Total: 3, Lapses: 1, Practice: 1,
		Trail: []trailMark{{srs.Right, "Correct", "verdict-right"}, {srs.Missed, "Incorrect", "verdict-missed"}, {srs.Right, "Correct", "verdict-right"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("telemetry = %+v\nwant        %+v", got, want)
	}
	// A missed card comes back one day out: "in 1 day" beside "1 day".
	it.Sched.Due = now.Add(24 * time.Hour)
	if got := telemetry(u, it, store.ReviewCounts{Total: 1, Missed: 1}, nil, false, now); got.NextRel != "in 1 day" || got.Interval != "1 day" || got.NextDate != "Mon, Nov 23" {
		t.Errorf("missed card: %+v", got)
	}
	// Across the new year the date carries its year.
	it.Sched.Due = now.Add(60 * 24 * time.Hour)
	if got := telemetry(u, it, store.ReviewCounts{Total: 1, Right: 1}, nil, false, now); got.NextDate != "Thu, Jan 21, 2027" || got.NextRel != "in 60 days" {
		t.Errorf("next year: %+v", got)
	}
	for v, want := range map[srs.Verdict][2]string{srs.Right: {"Correct", "verdict-right"}, srs.Close: {"Almost", "verdict-close"}, srs.Missed: {"Incorrect", "verdict-missed"}} {
		if label, icon := verdictBadge(v); label != want[0] || icon != want[1] {
			t.Errorf("verdictBadge(%s) = %q, %q", v, label, icon)
		}
	}
}

// Slice 70: the observed "Next review Nov 23 · today" beside "Interval 1
// day". The event and the interval share one clock, so a review reopened
// later counts down in hours while the interval stays the gap from the
// graded answer, and "today" never sits beside "1 day".
func TestSlice70_Telemetry_EventAndIntervalShareOneClock(t *testing.T) {
	u := store.User{TZ: "America/Chicago"}
	answered := time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC) // 5:30 PM in Chicago
	it := store.Item{Sched: srs.State{Stability: 0.5, Difficulty: 7.6, Reps: 1, Last: answered, Due: answered.Add(srs.MinInterval)}}
	cases := []struct {
		at       time.Time
		rel, day string
	}{
		{answered.Add(2 * time.Second), "in 1 day", "Mon, Nov 23"},
		{answered.Add(3 * time.Hour), "in 21 hours", "Mon, Nov 23"},
		{answered.Add(20 * time.Hour), "in 4 hours", "Mon, Nov 23"},
		{answered.Add(23*time.Hour + 45*time.Minute), "in 15 minutes", "Mon, Nov 23"},
		{answered.Add(25 * time.Hour), "due now", "Mon, Nov 23"},
	}
	for _, c := range cases {
		got := telemetry(u, it, store.ReviewCounts{Total: 1, Missed: 1}, nil, false, c.at)
		if got.NextRel != c.rel || got.Interval != "1 day" || got.After != "after this answer" || got.NextDate != c.day {
			t.Errorf("at +%s: %q · %q, interval %q %q", c.at.Sub(answered), got.NextDate, got.NextRel, got.Interval, got.After)
		}
		if strings.Contains(got.NextRel, "today") {
			t.Errorf("at +%s: %q says today beside a one-day interval", c.at.Sub(answered), got.NextRel)
		}
	}
	// A practice attempt measures the interval from the last graded answer.
	if got := telemetry(u, it, store.ReviewCounts{Total: 1, Missed: 1, Practice: 1}, nil, true, answered); got.After != "after the last graded answer" {
		t.Errorf("practice: %+v", got)
	}
	// The hours are rounded first: from 21 h 30 min (22 rounded hours) a
	// span reads in days.
	for d, want := range map[time.Duration]string{
		10 * time.Minute: "10 minutes", 29 * time.Minute: "29 minutes", 30 * time.Minute: "1 hour", 40 * time.Minute: "1 hour",
		21*time.Hour + 20*time.Minute: "21 hours", 21*time.Hour + 29*time.Minute: "21 hours", 21*time.Hour + 30*time.Minute: "1 day",
		22 * time.Hour: "1 day", 24*time.Hour - time.Second: "1 day", 36 * time.Hour: "2 days", 4 * 24 * time.Hour: "4 days",
	} {
		if got := spanLabel(d); got != want {
			t.Errorf("spanLabel(%s) = %q, want %q", d, got, want)
		}
	}
}

// Slice 70: the trail holds at most eight graded verdicts, oldest first.
func TestSlice70_Telemetry_TrailIsTheLastEightGradedVerdicts(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	it := store.Item{Sched: srs.State{Stability: 2, Difficulty: 5, Reps: 10, Last: now, Due: now.Add(48 * time.Hour)}}
	var history []store.Review // newest first, as store.History returns it
	for i := 0; i < 10; i++ {
		v := srs.Right
		if i == 0 {
			v = srs.Close // the newest
		}
		if i == 9 {
			v = srs.Missed // the oldest, beyond the eight shown
		}
		history = append(history, store.Review{Verdict: v})
	}
	got := telemetry(store.User{}, it, store.ReviewCounts{Total: 10}, history, false, now).Trail
	if len(got) != trailLen || got[len(got)-1].Verdict != srs.Close || got[0].Verdict != srs.Right {
		t.Errorf("trail = %+v", got)
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
	for _, name := range []string{"cards", "library", "create", "billing", "settings", "plus", "check", "arrow", "help", "pencil",
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
	for _, name := range []string{"cards", "library", "create", "billing", "settings", "plus", "check", "arrow", "help", "pencil", "calendar", "spinner"} {
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
	// Pass 2 (B2): "coming up" is readable text, secondary by its size.
	// Ghost opacity (20% / 22% ink, under 2:1) colors hairlines and the dot
	// grid, never text.
	if !strings.Contains(css, `.upcoming { display: grid; gap: var(--s1); font: 400 var(--meta)/1.4 var(--mono); color: var(--ink-muted); }`) {
		t.Error("app.css: the upcoming items are not Meta type in ink-muted")
	}
	if m := regexp.MustCompile(`(?m)(^|[\s;{])color:\s*var\(--ink-ghost\)`).FindString(css); m != "" {
		t.Errorf("app.css colors text with ink-ghost: %q", m)
	}
	// The only looping marks are the two processing ones and (S113-2) the
	// checking pulse, and reduced motion stills everything.
	if n := strings.Count(css, "infinite"); n != 3 {
		t.Errorf("%d looping animations, want the spinner, the progress line and the checking pulse only", n)
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
	// The page never jumps on its own (the shelf adds in place, Ask Why and
	// Edit card keep the learner's scroll). The one exception is the
	// answer-submit -> review transition, which reframes the review at the
	// question (PRODUCT.md: the review never scrolls the question out of
	// view): exactly one scroll reset, gated on that swap.
	src := string(js)
	const reframe = `if (opts && opts.top && $("[data-review]")) window.scrollTo(0, 0);`
	if n := strings.Count(src, "scrollTo("); n != 1 || strings.Count(src, reframe) != 1 {
		t.Errorf("app.js has %d scrollTo calls; want only the review reframe %q", n, reframe)
	}
	for _, bad := range []string{"location.reload", "window.scroll(", "scrollTop ="} {
		if strings.Contains(src, bad) {
			t.Errorf("app.js contains %q", bad)
		}
	}
	stripe, _ := staticFS.ReadFile("static/stripe.js")
	if strings.Count(string(stripe), "giveBack()") < 4 || !strings.Contains(string(stripe), "window.scryBusy.start(form)") {
		t.Error("stripe.js does not start the processing state and give the form back on every error path")
	}
}
