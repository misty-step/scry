package web_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// PRODUCT.md / DESIGN.md (Settings, mode follows the system with a toggle):
// the mode last chosen is the mode kept. Each Settings page load makes its
// own tab token and numbers its scripted saves 1, 2, … (theme_tab,
// theme_seq), with no clock involved. Within one tab an older save that
// reaches Scry after a newer one (held up on the way and overtaken by the
// same page's keepalive flush of a later choice) is answered as usual but
// does not overwrite the newer mode. Saves from different tabs (two
// already-open Settings pages, a device whose clock is behind) are
// last-writer-wins, and a plain form submit carries neither field and
// always applies.
func TestStory_Settings_AnOlderThemeSaveArrivingLateNeverOverwritesANewerOne(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("theme-seq@example.com")
	mode := func() string {
		t.Helper()
		page := b.mustGet("/settings")
		for _, v := range []string{"system", "light", "dark"} {
			if strings.Contains(page, `value="`+v+`" checked`) {
				return v
			}
		}
		return ""
	}
	want := func(label, m string) {
		t.Helper()
		if got := mode(); got != m {
			t.Fatalf("%s: Settings shows %q; want %q", label, got, m)
		}
	}
	post := func(kv ...string) {
		t.Helper()
		if st, _ := b.post("/settings/theme", kv...); st != http.StatusOK {
			t.Fatalf("POST /settings/theme %v = %d", kv, st)
		}
	}
	scripted := func(theme, tab string, seq int) {
		t.Helper()
		post("theme", theme, "theme_tab", tab, "theme_seq", strconv.Itoa(seq))
	}

	// The plain form has no tab or number fields, so a submit without
	// script takes the plain path.
	want("new account", "system")
	page := b.mustGet("/settings")
	for _, f := range []string{`name="theme_tab"`, `name="theme_seq"`, `data-theme-seq`} {
		if strings.Contains(page, f) {
			t.Fatalf("the appearance form carries %s; plain submits must send neither field", f)
		}
	}
	post("theme", "light") // plain Save before any scripted save
	want("plain Save first", "light")

	// 1. Same tab: Dark (seq 1) is held, Light (seq 2) is the keepalive
	// flush and arrives first; the held Dark arrives after it and is ignored,
	// as is a replay of the same number.
	scripted("light", "tab-a", 2)
	want("Light (A, 2)", "light")
	scripted("dark", "tab-a", 1)
	want("held Dark (A, 1) after Light (A, 2)", "light")
	scripted("dark", "tab-a", 2)
	want("replayed (A, 2)", "light")
	scripted("dark", "tab-a", 3)
	want("Dark (A, 3)", "dark")

	// 2. Two already-open tabs: B was loaded with A and numbers its first
	// choice 1, lower than A's 3; it applies. So does a device whose clock
	// is behind (its numbers know nothing of clocks), and A again after B.
	scripted("light", "tab-b", 1)
	want("already-open Tab B, Light (B, 1)", "light")
	scripted("dark", "skewed-device", 1)
	want("skewed-clock device, Dark", "dark")
	scripted("system", "tab-a", 4)
	want("Tab A again", "system")

	// 3. A plain Save after scripted saves applies, and scripted saves after
	// it apply too.
	post("theme", "dark")
	want("plain Save after scripted saves", "dark")
	scripted("light", "tab-a", 1)
	want("Tab A after a plain Save", "light")

	// Junk numbers or tokens take the plain path, which always applies.
	post("theme", "system", "theme_tab", "tab-a", "theme_seq", "-9")
	want("theme_seq=-9", "system")
	post("theme", "dark", "theme_tab", "tab-a", "theme_seq", "abc")
	want("theme_seq=abc", "dark")
	post("theme", "light", "theme_tab", strings.Repeat("x", 65), "theme_seq", "1")
	want("overlong theme_tab", "light")
	post("theme", "dark", "theme_tab", "tab\x01", "theme_seq", "1")
	want("unprintable theme_tab", "dark")

	// Another learner's saves never touch this one's mode.
	o := w.browser()
	o.signUp("theme-seq-other@example.com")
	if st, _ := o.post("/settings/theme", "theme", "light", "theme_tab", "tab-a", "theme_seq", "9999999999999"); st != http.StatusOK {
		t.Fatalf("other learner POST = %d", st)
	}
	want("after another learner's save", "dark")

	// 4. The page makes one tab token per load and numbers its saves from 0,
	// one per choice, and sends both with every scripted save, the keepalive
	// flush included. No clock and no tab storage are involved.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `const tabId = (crypto.randomUUID ? crypto.randomUUID() : String(Math.random()).slice(2) + "-" + String(Math.random()).slice(2));`,
		"let seq = 0;", "seq += 1;", `latest = { theme: radio.value, tabId: tabId, seq: seq };`,
		`b.set("theme_tab", choice.tabId);`, `b.set("theme_seq", String(choice.seq))`)
	theme := js[strings.Index(js, "function initTheme()"):]
	theme = theme[:strings.Index(theme, "function init()")]
	for _, banned := range []string{"Date.now", "Date(", "performance.now", "sessionStorage", "localStorage", "dataset.themeSeq"} {
		if strings.Contains(theme, banned) {
			t.Fatalf("initTheme numbers saves with %s; it must count per page load only", banned)
		}
	}
}

// DESIGN.md, Post-answer review: submitting an answer reframes the page at
// the top, so a long question scrolled up while answering on a phone is in
// view again when its review opens; only that transition resets scroll.
func TestStory_Review_AnswerSubmitReframesALongQuestionAtTheTop(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "swap(r.html, r.url, { top: true });", "if (opts && opts.top && $(\"[data-review]\")) window.scrollTo(0, 0);")
}
