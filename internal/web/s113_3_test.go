package web_test

import (
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// PRODUCT.md [S113-3] [S125-1] "One mark everywhere: the L3-S3 fanned deck
// with its cobalt top card tilted": the same brand mark meets me in the
// browser tab, the header and on my home screen, drawn from the one
// canonical SVG in DESIGN.md, beside the lowercase `scry` wordmark. It
// replaces the earlier lens mark and the interim S3-A outline deck
// everywhere (favicon, header, app icon, social preview).
func TestStory_S113_3_OneMarkEverywhere(t *testing.T) {
	w := newWorld(t)
	b := w.browser()

	// The header: the 24 px L3-S3 mark ([S125-1]), the canonical 32 x 32
	// drawing in both fill sets (the style sheet shows the active mode's), 8 px
	// before "scry" (the style sheet's gap), on every page signed in or out.
	lightMark := `<rect x="5" y="10" width="16" height="19" rx="2.5" fill="#CBD5E1"/><rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="#1E293B"/>` +
		`<g transform="rotate(10 19 14)"><rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="#0562EF"/><rect x="14.5" y="7" width="9" height="2" rx="1" fill="#FFFFFF" opacity="0.85"/></g>`
	darkMark := `<rect x="5" y="10" width="16" height="19" rx="2.5" fill="#334155"/><rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="#64748B"/>` +
		`<g transform="rotate(10 19 14)"><rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="#3B82F6"/><rect x="14.5" y="7" width="9" height="2" rx="1" fill="#DBEAFE" opacity="0.85"/></g>`
	mark := `<a class="wordmark" href="/" aria-label="scry, home">` +
		`<svg class="mark" viewBox="0 0 32 32" width="24" height="24" fill="none" aria-hidden="true" focusable="false">` +
		`<g class="mark-light">` + lightMark + `</g><g class="mark-dark">` + darkMark + `</g>` +
		`</svg><span class="wordmark-text">scry</span></a>`
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
		hasNot(t, page, "mark-focal", `r="2.25"`, "/static/icon.svg", "<animate", "mark-hinted", "mark-top", `width="14" height="10"`)
	}
	// The Study navigation keeps its own monochrome line icon.
	has(t, pages["/study"], `<a href="/study" aria-current="page" title="Study: recall what's due today"><svg class="icon icon-cards"`)

	// The browser tab: the adaptive SVG favicon (both canonical drawings,
	// switched by prefers-color-scheme) and /favicon.ico with the hinted 16
	// and 32 px PNGs.
	fav, ct := staticAsset(t, w, "/static/favicon.svg")
	if ct != "image/svg+xml" {
		t.Errorf("favicon.svg Content-Type = %q", ct)
	}
	has(t, fav, `viewBox="0 0 32 32"`, `<g class="mark-light">`+lightMark+`</g>`, `<g class="mark-dark">`+darkMark+`</g>`,
		"@media (prefers-color-scheme:dark){.mark-light{display:none}.mark-dark{display:inline}}")
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
		has(t, body, `<g class="mark" transform="translate(12 12) scale(1.25)"><rect x="5" y="10" width="16" height="19" rx="2.5"`, `<g transform="rotate(10 19 14)"><rect x="11" y="3.5" width="16" height="19" rx="2.5"`, `<path class="ground" d="M`)
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
