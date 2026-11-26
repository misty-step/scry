package web_test

import (
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// PRODUCT.md [S113-3] "One mark everywhere: the S3-A deck of cards": the
// same brand mark meets me in the browser tab, the header and on my home
// screen: a small deck of cards whose top card is Signal Cobalt with a short
// prompt bar, beside the lowercase `scry` wordmark. It replaces the earlier
// lens mark everywhere (favicon, header, app icon, social preview).
func TestStory_S113_3_OneMarkEverywhere(t *testing.T) {
	w := newWorld(t)
	b := w.browser()

	// The header: the 24 px S3-A mark, exactly, 8 px before "scry" (the
	// style sheet's gap), on every page signed in or out; on a 1x screen the
	// style sheet swaps in the same deck hinted to whole pixels.
	mark := `<a class="wordmark" href="/" aria-label="scry, home">` +
		`<svg class="mark" viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false"><g class="mark-exact">` +
		`<rect class="mark-far" x="7.5" y="4.5" width="14" height="10" rx="1.5" fill="#AAABAD"/>` +
		`<rect class="mark-face" x="9" y="6" width="11" height="7" fill="#FFFFFF"/>` +
		`<rect class="mark-near" x="5" y="7" width="14" height="10" rx="1.5" fill="#6D7073"/>` +
		`<rect class="mark-face" x="6.5" y="8.5" width="11" height="7" fill="#FFFFFF"/>` +
		`<rect class="mark-top" x="2.5" y="9.5" width="14" height="10" rx="1.5" fill="#0562EF"/>` +
		`<rect class="mark-bar" x="5.5" y="13.5" width="6.5" height="2" rx="1" fill="#FFFFFF"/>` +
		`</g><g class="mark-hinted" display="none" shape-rendering="crispEdges">` +
		`<path class="mark-far" d="M9 4H21V5H22V13H21V14H9V13H8V5H9Z" fill="#AAABAD"/>` +
		`<rect class="mark-face" x="10" y="6" width="10" height="6" fill="#FFFFFF"/>` +
		`<path class="mark-near" d="M6 7H18V8H19V16H18V17H6V16H5V8H6Z" fill="#6D7073"/>` +
		`<rect class="mark-face" x="7" y="9" width="10" height="6" fill="#FFFFFF"/>` +
		`<path class="mark-top" d="M3 10H15V11H16V19H15V20H3V19H2V11H3Z" fill="#0562EF"/>` +
		`<rect class="mark-bar" x="5" y="14" width="7" height="2" fill="#FFFFFF"/>` +
		`</g></svg><span class="wordmark-text">scry</span></a>`
	head := []string{
		`<link rel="icon" href="/favicon.ico" sizes="32x32">`,
		`<link rel="icon" href="/static/favicon.svg" type="image/svg+xml">`,
		`<link rel="apple-touch-icon" href="/static/icon-180.png">`,
		`<link rel="manifest" href="/static/manifest.webmanifest">`,
		`<meta property="og:image" content="` + w.srv.URL + `/static/icon-maskable-512.png">`,
		`<meta property="og:image:width" content="512">`,
	}
	pages := map[string]string{"/": b.mustGet("/"), "/signin": b.mustGet("/signin"), "/signup": b.mustGet("/signup")}
	b.signUp("onemark@example.com")
	pages["/shelf"] = b.mustGet("/shelf")
	b.addSet("kitchen-numbers")
	for _, p := range []string{"/study", "/library", "/learn", "/billing", "/settings"} {
		pages[p] = b.mustGet(p)
	}
	for path, page := range pages {
		has(t, page, mark)
		has(t, page, head...)
		// One mark per page, only in the header beside the wordmark: never
		// on the accent, never alone, never animated.
		if n := strings.Count(page, `<svg class="mark"`); n != 1 {
			t.Errorf("%s shows the mark %d times", path, n)
		}
		top := page[strings.Index(page, `<header`):strings.Index(page, `</header>`)]
		if !strings.Contains(top, mark) {
			t.Errorf("%s: the mark is not in the header beside scry", path)
		}
		hasNot(t, page, "mark-focal", `r="2.25"`, "/static/icon.svg", "<animate")
	}
	// The Study navigation keeps its own monochrome line icon.
	has(t, pages["/study"], `<a href="/study" aria-current="page" title="Study: recall what's due today"><svg class="icon icon-cards"`)

	// The browser tab: the adaptive SVG favicon (its hinted 16 px deck in
	// both modes) and /favicon.ico with the hinted 16 and 32 px PNGs.
	fav, ct := staticAsset(t, w, "/static/favicon.svg")
	if ct != "image/svg+xml" {
		t.Errorf("favicon.svg Content-Type = %q", ct)
	}
	has(t, fav, `<path class="mark-top" d="M2 6H10V7H11V12H10V13H2V12H1V7H2Z"/>`, `<rect class="mark-bar" x="3" y="9" width="5" height="1"/>`,
		".mark-top{fill:#0562EF}", "@media (prefers-color-scheme:dark)", ".mark-top{fill:#4D8DFF}")
	if ico, ct := staticAsset(t, w, "/favicon.ico"); ct != "image/x-icon" || !strings.HasPrefix(ico, "\x00\x00\x01\x00\x02\x00") {
		t.Errorf("/favicon.ico = %q, %d bytes", ct, len(ico))
	}
	for path, size := range map[string]int{"/static/favicon-16.png": 16, "/static/favicon-32.png": 32, "/static/app-icon-64.png": 64, "/static/app-icon-64-dark.png": 64} {
		decodePNG(t, w, path, size)
	}
	for _, svg := range []string{"/static/app-icon.svg", "/static/app-icon-dark.svg"} {
		body, ct := staticAsset(t, w, svg)
		if ct != "image/svg+xml" {
			t.Errorf("%s Content-Type = %q", svg, ct)
		}
		has(t, body, `<rect class="mark-top" x="16" y="27" width="24" height="17" rx="2.5"/>`, `<path class="ground" d="M`)
	}

	// The home screen: the dark app icon at 192 and 512 for "any", the
	// full-bleed dark icon as the maskable entries, and the apple-touch icon
	// at 180, each at the size it declares.
	res, err := http.Get(w.srv.URL + "/static/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Icons []struct{ Src, Sizes, Type, Purpose string }
	}
	err = json.NewDecoder(res.Body).Decode(&m)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, icon := range m.Icons {
		got[icon.Purpose+" "+icon.Sizes] = icon.Src
		if icon.Type == "image/png" {
			var size, h int
			if _, err := fmt.Sscanf(icon.Sizes, "%dx%d", &size, &h); err != nil || size != h {
				t.Errorf("%s declares %q", icon.Src, icon.Sizes)
				continue
			}
			decodePNG(t, w, icon.Src, size)
		} else if _, ct := staticAsset(t, w, icon.Src); ct != icon.Type {
			t.Errorf("%s Content-Type = %q, declared %q", icon.Src, ct, icon.Type)
		}
	}
	for key, src := range map[string]string{
		"any 192x192": "/static/icon-192.png", "any 512x512": "/static/icon-512.png",
		"maskable 192x192": "/static/icon-maskable-192.png", "maskable 512x512": "/static/icon-maskable-512.png",
		"any any": "/static/app-icon-dark.svg",
	} {
		if got[key] != src {
			t.Errorf("manifest %s = %q, want %q", key, got[key], src)
		}
	}
	decodePNG(t, w, "/static/icon-180.png", 180)

	// Every icon is first-party, so the policy's own origin covers it.
	res, err = http.Get(w.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if p := res.Header.Get("Content-Security-Policy"); !strings.Contains(p, "default-src 'self'") || !strings.Contains(p, "img-src 'self'") {
		t.Errorf("CSP does not allow the icons: %q", p)
	}
}

func decodePNG(t *testing.T, w *world, path string, size int) {
	t.Helper()
	body, ct := staticAsset(t, w, path)
	if ct != "image/png" {
		t.Errorf("%s Content-Type = %q", path, ct)
	}
	img, err := png.Decode(strings.NewReader(body))
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return
	}
	if r := img.Bounds(); r.Dx() != size || r.Dy() != size {
		t.Errorf("%s is %dx%d, declared %dx%d", path, r.Dx(), r.Dy(), size, size)
	}
}
