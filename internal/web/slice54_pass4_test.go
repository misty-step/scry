package web_test

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"scry/internal/store"
)

// PRODUCT.md / DESIGN.md (Settings, mode follows the system with a toggle):
// the mode last chosen is the mode kept. Settings renders the stored theme
// save sequence (data-theme-seq); scripted saves carry theme_seq, that
// number plus one per choice, with no clock involved. An older save that
// reaches Scry after a newer one (held up on the way, overtaken by the
// keepalive flush of a later choice or by a plain Save) is answered as usual
// but does not overwrite the newer mode. A plain form submit carries no
// theme_seq, always applies, and advances the stored sequence past any
// earlier scripted save still on the way.
func TestStory_Settings_AnOlderThemeSaveArrivingLateNeverOverwritesANewerOne(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("theme-seq@example.com")
	// state reads Settings as a fresh page would: the checked mode and the
	// sequence the page numbers its saves from.
	state := func() (string, int64) {
		t.Helper()
		page := b.mustGet("/settings")
		mode := ""
		for _, v := range []string{"system", "light", "dark"} {
			if strings.Contains(page, `value="`+v+`" checked`) {
				mode = v
			}
		}
		m := regexp.MustCompile(`data-theme-form data-theme-seq="(\d+)"`).FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("Settings renders no data-theme-seq on the appearance form")
		}
		seq, _ := strconv.ParseInt(m[1], 10, 64)
		return mode, seq
	}
	want := func(label, mode string, seq int64) {
		t.Helper()
		if m, q := state(); m != mode || q != seq {
			t.Fatalf("%s: Settings shows (%q, data-theme-seq=%d); want (%q, %d)", label, m, q, mode, seq)
		}
	}
	post := func(kv ...string) {
		t.Helper()
		if st, _ := b.post("/settings/theme", kv...); st != http.StatusOK {
			t.Fatalf("POST /settings/theme %v = %d", kv, st)
		}
	}
	n := func(v int64) string { return strconv.FormatInt(v, 10) }

	// A new account renders sequence 0, and the plain form has no
	// theme_seq field, so a submit without script takes the plain path.
	want("new account", "system", 0)
	if page := b.mustGet("/settings"); strings.Contains(page, `name="theme_seq"`) {
		t.Fatal("the appearance form carries a theme_seq field; plain submits must omit it")
	}

	// Same tab: Dark (seq 1) is held, Light (seq 2) is the keepalive flush
	// and arrives first; the held Dark arrives after it and is ignored.
	post("theme", "light", "theme_seq", "2")
	want("after Light (seq 2)", "light", 2)
	post("theme", "dark", "theme_seq", "1")
	want("held Dark (seq 1) after Light (seq 2)", "light", 2)
	post("theme", "dark", "theme_seq", "2") // the same number is not newer
	want("replayed seq 2", "light", 2)

	// Clock skew cannot matter: a fresh page (any clock) reads seq 2 and
	// numbers its next choice 3, which applies.
	post("theme", "dark", "theme_seq", "3")
	want("fresh page, Dark (seq 3)", "dark", 3)

	// A JS page loaded now numbers its next choice 4; it is held. A plain
	// no-JS Save of Light applies and advances the sequence by 1000; the
	// held Dark (and any burst of earlier choices) then arrives and is ignored.
	post("theme", "light")
	want("plain Save after scripted saves", "light", 3+store.ThemeSeqPlainStep)
	for _, held := range []int64{4, 5, 3 + store.ThemeSeqPlainStep} {
		post("theme", "dark", "theme_seq", n(held))
		want("held Dark (seq "+n(held)+") after a plain Save", "light", 3+store.ThemeSeqPlainStep)
	}
	// The next Settings page counts up from the rendered number and applies.
	_, seq := state()
	post("theme", "dark", "theme_seq", n(seq+1))
	want("next page's first choice", "dark", seq+1)

	// Junk or negative numbers are treated as a plain submit (and advance).
	post("theme", "system", "theme_seq", "-9")
	want("theme_seq=-9", "system", seq+1+store.ThemeSeqPlainStep)
	post("theme", "light", "theme_seq", "abc")
	want("theme_seq=abc", "light", seq+1+2*store.ThemeSeqPlainStep)

	// Another learner's numbers never touch this one's mode.
	_, mine := state()
	o := w.browser()
	o.signUp("theme-seq-other@example.com")
	if st, _ := o.post("/settings/theme", "theme", "dark", "theme_seq", "9999999999999"); st != http.StatusOK {
		t.Fatalf("other learner POST = %d", st)
	}
	post("theme", "dark", "theme_seq", n(mine+1))
	want("after another learner's large seq", "dark", mine+1)

	// The page numbers its saves from data-theme-seq, one per choice, and
	// sends that number with every scripted save, the keepalive flush
	// included. No clock and no tab storage are involved.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, `var seq = parseInt(form.dataset.themeSeq || "0", 10) || 0;`, "seq += 1;", `b.set("theme_seq", String(choice.seq))`)
	theme := js[strings.Index(js, "function initTheme()"):]
	theme = theme[:strings.Index(theme, "function init()")]
	for _, banned := range []string{"Date.now", "Date(", "performance.now", "sessionStorage", "localStorage"} {
		if strings.Contains(theme, banned) {
			t.Fatalf("initTheme numbers saves with %s; it must count from data-theme-seq only", banned)
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
