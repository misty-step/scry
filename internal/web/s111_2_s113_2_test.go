package web_test

// [S111-2] Write a full answer: a multiline answer field that grows.
// [S113-2] Every wait and every change is shown, calmly.
//
// These hold the contract in the markup, the style sheet and the page
// script; the headed walkthrough (1440×900 and 390×844, light and dark)
// proves the same in a browser.

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// funcBody is the source of the named function in app.js, up to the next
// function at the same indentation.
func funcBody(t *testing.T, js, name string) string {
	t.Helper()
	start := strings.Index(js, "  function "+name+"(")
	if start < 0 {
		t.Fatalf("app.js has no function %s", name)
	}
	rest := js[start+1:]
	end := regexp.MustCompile(`\n  (function |var |// ----)`).FindStringIndex(rest)
	if end == nil {
		return rest
	}
	return rest[:end[0]]
}

func TestStory_S111_2_AnswerFieldIsAGrowingTextarea(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("textarea@example.com")
	b.addSet("nato-alphabet")
	page := b.mustGet("/study")
	field := between(t, page, "<textarea class=\"answer-input\"", "</textarea>")
	// A labelled multiline field: three rows without script, the key hint
	// exposed to assistive technology, Return sends on a touch keyboard.
	has(t, field, `id="answer"`, `name="answer"`, `rows="3"`, `placeholder="Type your answer"`, `enterkeyhint="send"`,
		`aria-describedby="answer-hint status"`, "autofocus")
	has(t, page, `<label class="sr" for="answer">Your answer</label>`,
		`id="answer-hint" data-answer-hint>Enter to check · Shift+Enter for new line · Space to start/stop speaking (empty answer)</p>`,
		`id="answer-hint-phone" data-answer-hint-phone>Return to check</p>`)
	hasNot(t, page, `<input class="answer-input`, "data-caret")

	css, _ := staticAsset(t, w, "/static/app.css")
	// Answer type (22 px, 18 px on phones), 1.5 line height, a 64ch measure,
	// a 1 px line frame at the control radius, padded 12 px 16 px, ink on
	// focus with no ring, the native caret in accent; it wraps, never
	// scrolls sideways or resizes, and scrolls inside itself past 45vh.
	has(t, css, "--answer: 22px;", "--answer: 18px;")
	has(t, cssRule(t, css, ".answer-input"), "font: 400 var(--answer)/1.5 var(--text);", "max-width: 64ch;",
		"border: 1px solid var(--line); border-radius: var(--radius-control);", "background: var(--page);", "padding: 12px var(--s3);",
		"caret-color: var(--accent);", "white-space: pre-wrap; overflow-wrap: anywhere;", "overflow-x: hidden; overflow-y: auto; resize: none;",
		"max-height: 45vh;")
	has(t, css, ".js-focus.kbd-nav .answer-input:focus-visible {\n  border-color: var(--ink); outline: none;\n}",
		".answer-input::placeholder { color: var(--ink-muted); opacity: 1; }",
		".answer-input[data-grown] {\n  transition: height var(--motion-quick) var(--ease),",
		".answer-hint-phone { display: block; }", ".answer-input { padding: 12px; }")

	// Script: one row, then the height from the content, measured on a
	// hidden twin so the visible field never collapses; re-measured on
	// input, paste, dictation, resize, rotation and page restore.
	js, _ := staticAsset(t, w, "/static/app.js")
	grow := funcBody(t, js, "answerField")
	has(t, grow, `twin.className = input.className + " answer-measure";`, "input.rows = 1;", "var h = twin.scrollHeight + border;",
		`input.style.height = h + "px";`, `input.setAttribute("data-grown", "");`, `input.addEventListener("input", grow);`, `"paste"`, "doc.fonts.ready.then(grow)")
	hasNot(t, grow, `input.style.height = "auto"`, `input.style.height = ""`)
	has(t, js, `window.addEventListener("resize", function () { if (growAnswer) growAnswer(); });`,
		`window.addEventListener("orientationchange"`, "growAnswer = answerField(input);")
	has(t, funcBody(t, js, "pickHint"), `"answer-hint-phone" : "answer-hint"`)
	has(t, css, ".answer-measure {", "height: 0 !important;", "visibility: hidden;")
}

func TestStory_S111_2_EnterChecksShiftEnterNewLineSpaceSpeaks(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	study := funcBody(t, js, "initStudy")
	// Enter (Return) checks; Shift+Enter is left to the field (a new line);
	// never while an input method composes.
	has(t, study, `if (e.key === "Enter" && !e.shiftKey && !e.altKey && !e.isComposing && e.keyCode !== 229) {`, "form.requestSubmit();")
	// Space in an empty field (or while listening) starts or stops listening.
	has(t, study, `e.key === " " && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !input.readOnly && (voice || input.value === "")`, "toggleVoice();")
	// A second Enter while it checks does nothing.
	has(t, study, "if (held(check)) return;")
}

func TestStory_S111_2_DictationIsAppendedAndTheFieldReMeasures(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	// After typed text with one space between, or on the current line when
	// it ends in a line break; the caret after the words.
	has(t, funcBody(t, js, "appendSpoken"), `input.value = had + (had === "" || /\s$/.test(had) ? "" : " ") + text;`, "input.setSelectionRange(end, end);")
	has(t, js, "appendSpoken(input, j.text); via.value = \"spoken\";\n          if (growAnswer) growAnswer();")
	hasNot(t, js, "input.value = j.text")
}

func TestStory_S111_2_ReviewShowsTheAnswersLineBreaks(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, css, ".compare-answer[data-submitted] { white-space: pre-wrap; line-height: 1.55; }")
	has(t, cssRule(t, css, ".struck-text"), "white-space: pre-wrap;")
}

func TestStory_S111_2_CouldntCheckKeepsAMultilineDraft(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("draft@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	w.model.err = fmt.Errorf("provider is down")
	// A browser posts line breaks as CRLF; a leading line break is the
	// learner's too.
	draft := "\r\nFirst line of my answer.\r\nSecond line\r\n\r\nafter a blank line"
	status, page := b.post("/study/answer", "item_id", strconv.FormatInt(it.ID, 10), "answer", draft)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", status)
	}
	// The same card, the line under the field, and the draft exactly as
	// written: the newline after <textarea> is the one the parser drops.
	if got, _ := b.current(page); got.ID != it.ID {
		t.Fatalf("a different card came back: %d, want %d", got.ID, it.ID)
	}
	has(t, page, `data-status data-failed>Couldn't check that just now. Your answer is still here; press Enter to try again.`,
		"data-answer-input>\n"+draft+"</textarea>")

	// With script the page is not swapped: the field keeps its text, the
	// line says so (the server's own words), the field nudges and has focus.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `var COULDNT_CHECK = "Couldn't check that just now. Your answer is still here; press Enter to try again.";`)
	study := funcBody(t, js, "initStudy")
	has(t, study, "if (!r.ok) { fail(statusFrom(r.html)); return; }", "}, function () { fail(); });",
		"input.readOnly = false;", "tell(status, nodes || COULDNT_CHECK);", "nudge(input);", "input.focus({ preventScroll: true });")
}

// motionCSS resolves the style sheet's custom properties well enough to
// read every transition and animation: durations in ms and timing curves.
type motionCSS struct {
	vars map[string]string
}

func newMotionCSS(css string) motionCSS {
	m := motionCSS{vars: map[string]string{}}
	root := css[strings.Index(css, ":root {"):]
	root = root[:strings.Index(root, "\n}")]
	for _, d := range regexp.MustCompile(`(--[a-z0-9-]+):\s*([^;]+);`).FindAllStringSubmatch(root, -1) {
		m.vars[d[1]] = strings.TrimSpace(d[2])
	}
	return m
}

func (m motionCSS) resolve(v string) string {
	for i := 0; i < 4; i++ {
		v = regexp.MustCompile(`var\((--[a-z0-9-]+)\)`).ReplaceAllStringFunc(v, func(s string) string {
			if r, ok := m.vars[s[4:len(s)-1]]; ok {
				return r
			}
			return s
		})
	}
	return v
}

// splitTop splits on commas outside parentheses.
func splitTop(s string, sep rune) []string {
	var out []string
	depth, last := 0, 0
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == sep && depth == 0:
			out = append(out, strings.TrimSpace(s[last:i]))
			last = i + 1
		}
	}
	return append(out, strings.TrimSpace(s[last:]))
}

var timeRE = regexp.MustCompile(`^(\d*\.?\d+)(ms|s)$`)

func ms(tok string) (float64, bool) {
	m := timeRE.FindStringSubmatch(tok)
	if m == nil {
		return 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	if m[2] == "s" {
		v *= 1000
	}
	return v, true
}

// curveOK reports whether a timing function is an ease-out with no
// overshoot: a cubic-bezier whose y values stay within [0, 1], that comes
// to rest gently (its closing slope is at most 1) and is at least halfway
// there at its midpoint.
func curveOK(fn string) bool {
	m := regexp.MustCompile(`^cubic-bezier\(([^)]+)\)$`).FindStringSubmatch(fn)
	if m == nil {
		return false
	}
	var p [4]float64
	for i, s := range strings.Split(m[1], ",") {
		p[i], _ = strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	at := func(a, b float64) float64 { return 0.375*a + 0.375*b + 0.125 } // B(0.5) for control values a, b
	return p[1] >= 0 && p[1] <= 1 && p[3] >= 0 && p[3] <= 1 && 1-p[3] <= 1-p[2] && at(p[1], p[3]) >= at(p[0], p[2])
}

func TestStory_S113_2_MotionTokensAreEaseOutWithin120To240ms(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	mc := newMotionCSS(css)
	if got := mc.vars["--ease"]; got != "cubic-bezier(0.2, 0, 0, 1)" || !curveOK(got) {
		t.Fatalf("--ease = %q, want DESIGN's crisp ease-out cubic-bezier(0.2, 0, 0, 1)", got)
	}
	for name, want := range map[string]string{"--motion-quick": "120ms", "--motion-status": "160ms", "--motion-leave": "180ms",
		"--motion-arrive": "200ms", "--motion-draw": "240ms", "--motion-pulse": "1.6s", "--rise": "4px", "--rise-card": "8px", "--nudge": "4px"} {
		if mc.vars[name] != want {
			t.Errorf("%s = %q, want %q", name, mc.vars[name], want)
		}
	}
	// The repeating marks DESIGN allows: the processing spinner and
	// progress line, and the checking pulse. Nothing else loops.
	loops := map[string]bool{"spin": true, "slide": true, "breathe": true}
	src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	decls := regexp.MustCompile(`(?m)(?:^|[\s;{])((?:transition|animation)(?:-duration|-timing-function)?)\s*:\s*([^;}]+)`).FindAllStringSubmatch(src, -1)
	if len(decls) < 40 {
		t.Fatalf("found only %d motion declarations; the parser is lost", len(decls))
	}
	for _, d := range decls {
		prop, value := d[1], strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d[2]), "!important"))
		value = strings.TrimSpace(value)
		if value == "none" {
			continue
		}
		for _, item := range splitTop(mc.resolve(value), ',') {
			toks := splitTop(item, ' ')
			loop := false
			for _, tok := range toks {
				if loops[tok] {
					loop = true
				}
				if tok == "infinite" && !loop {
					t.Errorf("%s: %q loops, and only the processing marks and the checking pulse may", prop, item)
				}
			}
			var times []float64
			discrete := false
			var curves []string
			for _, tok := range toks {
				if v, ok := ms(tok); ok {
					times = append(times, v)
				}
				switch {
				case tok == "visibility" || tok == "allow-discrete":
					discrete = true
				case tok == "linear" || tok == "ease" || tok == "ease-in" || tok == "ease-out" || tok == "ease-in-out" ||
					strings.HasPrefix(tok, "cubic-bezier(") || strings.HasPrefix(tok, "steps("):
					curves = append(curves, tok)
				}
			}
			if strings.HasSuffix(prop, "-timing-function") {
				if !curveOK(item) {
					t.Errorf("%s: %q is not the ease-out", prop, item)
				}
				continue
			}
			if len(times) == 0 {
				t.Errorf("%s: %q has no duration", prop, item)
				continue
			}
			if loop {
				continue
			}
			if discrete && (times[0] == 0 || len(curves) == 0) {
				continue // a discrete visibility flip, timed with the fade it ends
			}
			if times[0] < 120 || times[0] > 240 {
				t.Errorf("%s: %q lasts %v ms, want 120-240 ms", prop, item, times[0])
			}
			if strings.HasSuffix(prop, "-duration") {
				continue
			}
			if len(curves) != 1 || !curveOK(curves[0]) {
				t.Errorf("%s: %q is not on the one ease-out (no overshoot)", prop, item)
			}
		}
	}
	// Nothing scales, and nothing moves more than 8 px (the processing
	// line's slide is a loop of its own track).
	if regexp.MustCompile(`scale\w*\(`).MatchString(src) {
		t.Error("app.css scales something; text, controls and cards never scale")
	}
	for _, m := range regexp.MustCompile(`translate[XY]?\(([^()]*(?:\([^()]*\)[^()]*)*)\)`).FindAllStringSubmatch(src, -1) {
		v := mc.resolve(m[1])
		if strings.HasSuffix(v, "%") {
			if v != "-100%" && v != "250%" {
				t.Errorf("translate(%s) moves by a share of the element", m[1])
			}
			continue
		}
		px := regexp.MustCompile(`(\d+(?:\.\d+)?)px`).FindStringSubmatch(v)
		if px == nil {
			t.Errorf("translate(%s): no px distance", m[1])
			continue
		}
		if f, _ := strconv.ParseFloat(px[1], 64); f > 8 {
			t.Errorf("translate(%s) moves %v px, more than 8", m[1], f)
		}
	}
	// Script never sets its own durations or curves.
	js, _ := staticAsset(t, w, "/static/app.js")
	hasNot(t, js, "style.transition", "style.animation", ".animate(")
}

func TestStory_S113_2_EveryMotionMomentIsDrawn(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	has(t, css,
		// Hover, press and keyboard focus: 120 ms; the ring fades in.
		".btn:active { transform: translateY(1px); }", "transform var(--motion-quick) var(--ease);",
		":focus-visible { animation: ring-in var(--motion-quick) var(--ease); }", "@keyframes ring-in { from { outline-color: transparent; } }",
		// Checking: fades in (160 ms) and breathes 60%-100% on 1.6 s.
		".wait-word.is-pulsing { animation: fade-in var(--motion-status) var(--ease) both, breathe var(--motion-pulse) ease-in-out var(--motion-status) infinite; }",
		"@keyframes breathe { 0%, 100% { opacity: 1; } 50% { opacity: 0.6; } }",
		// Verdict: the strike draws by clip once; the rest rises 4 px.
		"@keyframes strike { from { clip-path: inset(0 100% 0 0); }", ".struck-text { animation: strike 200ms var(--ease) both; }",
		".judged .judgment-right, .judged .compare, .judged .expected, .judged .verdict { animation: rise-in var(--motion-arrive) var(--ease) both; }",
		// Card advance, Snooze and Delete: view transitions on the card.
		".current, .done, .empty-state { view-transition-name: card; }",
		"::view-transition-old(card) { animation: card-out var(--motion-quick) var(--ease) both; }",
		"::view-transition-new(card) { animation: card-in var(--motion-arrive) var(--ease) both; }",
		`html[data-vt="leave"]::view-transition-old(card) { animation: card-leave var(--motion-leave) var(--ease) both; }`,
		"transform: translateX(calc(-1 * var(--rise-card)))", "::view-transition-group(top-bar), ::view-transition-group(card) { animation: none; }",
		// Destination switch: a cross-fade under a still header, and the
		// active underline drawn by clip.
		"@view-transition { navigation: auto; }", ".top { view-transition-name: top-bar; }",
		"::view-transition-old(*), ::view-transition-new(*) { animation-duration: var(--motion-status);",
		"animation: underline-in var(--motion-status) var(--ease) both;",
		// Edit card: height and opacity (200 ms); "Saved" fades in (160 ms).
		".edit-card { interpolate-size: allow-keywords; }", "transition: block-size var(--motion-arrive) var(--ease), opacity var(--motion-arrive) var(--ease),",
		".edit-status:not([hidden]) { animation: status-in var(--motion-status) var(--ease) both; }",
		// Status lines: in with a 4 px rise (160 ms), out (120 ms).
		".status-line.is-in, .study-note, .flash, .edit-status:not([hidden]) { animation: status-in var(--motion-status) var(--ease) both; }",
		".status-line.is-out { animation: status-out var(--motion-quick) var(--ease) both; }",
		// Error: a nudge 4 px to the side and back, once (180 ms).
		"@keyframes nudge { 0%, 100% { transform: none; } 50% { transform: translateX(var(--nudge)); } }",
		".is-nudged { animation: nudge var(--motion-leave) var(--ease); }",
		// Idea links draw in (240 ms); the Specimen cross-fades (200 ms).
		"stroke-dasharray: 1; animation: draw var(--motion-draw) var(--ease) both;",
		"transition: opacity 200ms var(--ease), visibility 0s linear 200ms;")
	// The script swaps cards inside a view transition named by kind.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, funcBody(t, js, "move"), "doc.startViewTransition(apply)", `root.setAttribute("data-vt", kind);`, `card.classList.add("is-arriving");`)
	has(t, js, `swap(r.html, r.url, { vt: "card" });`, `swap(r.html, r.url, { vt: kind === "restore" ? "card" : "leave" });`,
		`swap(r.html, r.url, { top: true, vt: "verdict" });`,
		// The underline only draws again when the destination changes.
		"if (nav && here && nav.innerHTML !== here.innerHTML) here.innerHTML = nav.innerHTML;",
		// A destination switch the browser skips is not an error.
		`window.addEventListener("pagereveal", settled);`, `window.addEventListener("pageswap", settled);`)
	layout := w.browser().mustGet("/")
	has(t, layout, `<script src="/static/app.js" defer blocking="render"></script>`)
}

func TestStory_S113_2_ReducedMotionStopsTranslateScalePulse(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	reduced := css[strings.Index(css, "/* ---- Reduced motion"):]
	has(t, reduced, "*, *::before, *::after {\n    animation: none !important;\n    transition: none !important;",
		".btn:active, .tile-add:active, .example:active, .mic:active { transform: none; }",
		"@view-transition { navigation: none; }",
		"::view-transition-group(*), ::view-transition-old(*), ::view-transition-new(*) { animation: none !important; }",
		".edit-card::details-content { transition: none !important; }")
	// No transform is left standing outside a motion: under reduced motion
	// nothing is translated at rest.
	src := regexp.MustCompile(`(?s)@keyframes[^{]*\{(?:[^{}]*\{[^{}]*\})*[^{}]*\}`).ReplaceAllString(css, "")
	for _, m := range regexp.MustCompile(`([^{}\n]+)\{[^{}]*transform:\s*(translate[^;]+);`).FindAllStringSubmatch(src, -1) {
		if !strings.Contains(m[1], ":active") {
			t.Errorf("%s is translated at rest (%s)", strings.TrimSpace(m[1]), m[2])
		}
	}
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `var still = window.matchMedia ? window.matchMedia("(prefers-reduced-motion: reduce)") : { matches: false };`,
		"if (!kind || still.matches) { apply(); return; }")
}

func TestStory_S113_2_WaitsSayTheirWordWithTheControlDisabled(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	// The control is disabled and busy at once; its word comes after 400 ms
	// (in a polite live region), so a quick action never flashes.
	h := funcBody(t, js, "hold")
	has(t, h, `control.setAttribute("aria-busy", "true");`, `control.setAttribute("data-waiting", word || "");`, "control.disabled = true;",
		`control.setAttribute("aria-disabled", "true");`, "h.timer = setTimeout(show, WAIT_DELAY);")
	has(t, js, "var WAIT_DELAY = 400;")
	// Each flow's word.
	has(t, js, `waitWord(status, "checking", true); }, WAIT_DELAY);`, `waitWord(status, "still checking", true); }, 3000);`,
		`waitWord(status, "listening");`, `waitWord(status, "transcribing");`, `hold(save, "saving", { live: failed });`,
		`snooze: ["snoozing",`, `delete: ["removing",`, `hold(button, "importing", { now: true, live: status });`,
		`hold(a, "preparing", { live: status });`)

	// Every word has a polite status line beside its control.
	b := w.browser()
	b.signUp("waits@example.com")
	b.addSet("nato-alphabet")
	page := b.mustGet("/study")
	has(t, page, `<p class="status meta status-line" id="status" role="status" aria-live="polite" data-status>`,
		`<p class="meta status-line manage-status" role="status" aria-live="polite" data-manage-status></p>`)
	it, _ := b.current(page)
	review := b.answer(it, "Alfa")
	has(t, review, `<span class="meta status-line edit-fail" role="status" aria-live="polite" data-edit-fail></span>`,
		`<p class="meta status-line manage-status" role="status" aria-live="polite" data-manage-status></p>`)
	learn := b.mustGet("/learn")
	has(t, learn, `role="status" aria-live="polite" data-import-status`,
		`<div class="busy-track import-progress" aria-hidden="true" data-import-progress hidden><span class="busy-bar"></span></div>`)
	// Downloads stay plain links without script.
	settings := b.mustGet("/settings")
	for _, f := range []string{"md", "csv", "json"} {
		has(t, settings, `href="/settings/export?format=`+f+`" download data-download>`)
	}
	has(t, settings, `<p class="meta status-line download-status" role="status" aria-live="polite" data-download-status></p>`)
}

func TestStory_S113_2_FailuresSayWhatHappenedKeepInputAndRetryInPlace(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js,
		`tell(failed, text || "Couldn't save that just now. Your edits are still here.");`,
		`"Couldn't snooze that just now."`, `"Couldn't remove that just now."`,
		`var DOWNLOAD_BROKE = "Couldn't prepare that download. Try again.";`,
		`var IMPORT_BROKE = "Couldn't import that just now. Your file and idea title are still here; try again.";`,
		`say("Didn't catch that. Try again or type it.");`)
	// Each failure nudges what it names and gives the control back; nothing
	// falls back to a bare error page.
	for _, fn := range []string{"initEdit", "initManage", "initNext", "initImport", "initDownloads", "initStudy"} {
		body := funcBody(t, js, fn)
		if !strings.Contains(body, "nudge(") {
			t.Errorf("%s: a failure does not nudge its control", fn)
		}
		hasNot(t, body, "form.submit()")
	}
	// Snooze and Delete set the card's other actions aside, and give them back.
	has(t, funcBody(t, js, "initManage"), `setAside($$("[data-snooze], [data-delete], [data-restore], [data-check], [data-next], [data-again], [data-edit-save]")`, "giveBack();")
	// Import: a refusal is read from the page that came back and said in
	// place; the chosen file and the idea title are never cleared.
	imp := funcBody(t, js, "initImport")
	has(t, imp, "body: body", `$("[data-import-status][data-refused]", page)`, "refuse(refused.textContent.trim());", "location.assign(res.url);")
	hasNot(t, imp, "file.value", "form.reset")
	// Download: fetched as a file; a failed or redirected answer is a failure.
	has(t, funcBody(t, js, "initDownloads"), "if (!res.ok || res.redirected) throw", "URL.createObjectURL(blob)", "link.download = m ? m[1] : \"scry-export\";")

	// Without script the server keeps the idea title on a refusal.
	b := w.browser()
	b.signUp("refused@example.com")
	_, page := b.upload("notes.pdf", []byte("%PDF-1.4"), "Keep this title")
	has(t, page, "data-refused", `value="Keep this title"`)
}

func TestStory_S113_2_BackRestoredPagesGiveEveryControlBack(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	i := strings.Index(js, `window.addEventListener("pageshow", function (e) {
    if (e.persisted) {
      $$("[data-waiting]").forEach(release);`)
	if i < 0 {
		t.Fatal("a page restored by Back does not release its waits")
	}
	restore := js[i : i+700]
	has(t, restore, `$$("[data-set-aside]")`, `$$("[data-import-progress]").forEach(function (p) { p.hidden = true; });`,
		`$$("[data-answer-input]").forEach(function (i) { i.readOnly = false; });`, "if (growAnswer) growAnswer();")
	has(t, funcBody(t, js, "release"), `control.removeAttribute("aria-busy");`, `control.removeAttribute("data-waiting");`, "control.disabled = false;")
}

// When "still checking" cross-fades in over "checking", the leaving word
// must fade out, not keep pulsing on top of the new one. Both rules have the
// same specificity, so .is-leaving must come after .is-pulsing in the sheet
// (the later rule wins for a span holding both classes), it takes no
// pointer events, and waitWord drops the pulse from the word it retires
// before marking it leaving.
func TestStory_S113_2_LeavingWaitWordFadesOutAndNeverPulsesOverTheNext(t *testing.T) {
	w := newWorld(t)
	css, _ := staticAsset(t, w, "/static/app.css")
	pulsing := strings.Index(css, ".wait-word.is-pulsing {")
	leaving := strings.Index(css, ".wait-word.is-leaving {")
	if pulsing < 0 || leaving < 0 {
		t.Fatalf("app.css is missing a wait-word rule (pulsing at %d, leaving at %d)", pulsing, leaving)
	}
	if leaving < pulsing {
		t.Error(".wait-word.is-leaving comes before .wait-word.is-pulsing, so a leaving pulsing word keeps breathing over the next word")
	}
	if strings.Count(css, ".wait-word.is-leaving") != 1 {
		t.Error(".wait-word.is-leaving is declared more than once")
	}
	rule := css[leaving:]
	rule = rule[:strings.Index(rule, "}")+1]
	has(t, rule, "animation: status-out var(--motion-quick) var(--ease) both;", "pointer-events: none;")

	js, _ := staticAsset(t, w, "/static/app.js")
	ww := funcBody(t, js, "waitWord")
	drop := strings.Index(ww, `old.classList.remove("is-pulsing");`)
	mark := strings.Index(ww, `old.classList.add("is-leaving");`)
	if drop < 0 || mark < 0 || drop > mark {
		t.Errorf("waitWord must remove is-pulsing from the old word before adding is-leaving (remove at %d, add at %d)", drop, mark)
	}
}
