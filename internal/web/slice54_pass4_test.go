package web_test

import (
	"net/http"
	"strings"
	"testing"
)

// PRODUCT.md / DESIGN.md (Settings, mode follows the system with a toggle):
// the mode last chosen is the mode kept. Scripted saves carry theme_seq, the
// page's strictly increasing number for each choice. An older save that
// reaches Scry after a newer one (held up on the way, overtaken by the
// keepalive flush of a later choice) is answered as usual but does not
// overwrite the newer mode; a newer one, or a plain form submit with no
// theme_seq, applies.
func TestStory_Settings_AnOlderThemeSaveArrivingLateNeverOverwritesANewerOne(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("theme-seq@example.com")
	mode := func() string {
		page := b.mustGet("/settings")
		for _, v := range []string{"system", "light", "dark"} {
			if strings.Contains(page, `value="`+v+`" checked`) {
				return v
			}
		}
		return ""
	}
	post := func(kv ...string) {
		t.Helper()
		if st, _ := b.post("/settings/theme", kv...); st != http.StatusOK {
			t.Fatalf("POST /settings/theme %v = %d", kv, st)
		}
	}

	// The newer choice (Light, the keepalive flush) arrives first ...
	post("theme", "light", "theme_seq", "1700000000002")
	if m := mode(); m != "light" {
		t.Fatalf("after Light (seq 2): %q", m)
	}
	// ... then the older, held Dark save arrives: answered, not applied.
	post("theme", "dark", "theme_seq", "1700000000001")
	if m := mode(); m != "light" {
		t.Fatalf("an older Dark save (seq 1) overwrote the newer Light: %q", m)
	}
	// The same number again is not newer either.
	post("theme", "dark", "theme_seq", "1700000000002")
	if m := mode(); m != "light" {
		t.Fatalf("a replayed seq overwrote Light: %q", m)
	}
	// A newer scripted save applies.
	post("theme", "dark", "theme_seq", "1700000000003")
	if m := mode(); m != "dark" {
		t.Fatalf("a newer Dark save (seq 3) did not apply: %q", m)
	}
	// A plain form submit without script carries no theme_seq and always
	// applies, even after scripted saves with large numbers ...
	post("theme", "system")
	if m := mode(); m != "system" {
		t.Fatalf("a no-JS submit after scripted saves did not apply: %q", m)
	}
	// ... and clears the stored number, so the page's next scripted save
	// (from a fresh tab, numbered from the clock) applies again.
	post("theme", "light", "theme_seq", "5")
	if m := mode(); m != "light" {
		t.Fatalf("a scripted save after a no-JS submit did not apply: %q", m)
	}
	// Junk or negative numbers are treated as a plain submit.
	post("theme", "dark", "theme_seq", "-9")
	if m := mode(); m != "dark" {
		t.Fatalf("theme_seq=-9 not treated as a plain save: %q", m)
	}
	post("theme", "system", "theme_seq", "abc")
	if m := mode(); m != "system" {
		t.Fatalf("theme_seq=abc not treated as a plain save: %q", m)
	}

	// Another learner's numbers never touch this one's mode.
	o := w.browser()
	o.signUp("theme-seq-other@example.com")
	if st, _ := o.post("/settings/theme", "theme", "dark", "theme_seq", "9999999999999"); st != http.StatusOK {
		t.Fatalf("other learner POST = %d", st)
	}
	post("theme", "light", "theme_seq", "6")
	if m := mode(); m != "light" {
		t.Fatalf("another learner's theme_seq blocked this learner's save: %q", m)
	}

	// The page sends theme_seq with every scripted save, the keepalive
	// flush included, numbered max(last + 1, now) and kept for the tab.
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "Math.max(last + 1, Date.now())", `sessionStorage.setItem("scry.theme_seq"`, `b.set("theme_seq", String(choice.seq))`)
}

// DESIGN.md, Post-answer review: submitting an answer reframes the page at
// the top, so a long question scrolled up while answering on a phone is in
// view again when its review opens; only that transition resets scroll.
func TestStory_Review_AnswerSubmitReframesALongQuestionAtTheTop(t *testing.T) {
	w := newWorld(t)
	js, _ := staticAsset(t, w, "/static/app.js")
	has(t, js, "swap(r.html, r.url, { top: true });", "if (opts && opts.top && $(\"[data-review]\")) window.scrollTo(0, 0);")
}
