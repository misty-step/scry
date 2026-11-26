package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
)

// [S113-3] [S125-1] One mark everywhere: the L3-S3 fanned deck with its
// cobalt top card tilted, drawn from DESIGN.md's canonical SVG verbatim.
// These tests pin the drawing on every surface; the served side (header on
// every page, favicon links, manifest, apple-touch icon) is
// TestStory_S113_3_OneMarkEverywhere.

// The mark's own fixed brand colors (DESIGN.md, Brand mark): only the
// light/dark pair switches; they never follow the theme tokens.
var l3s3 = map[string]map[string]string{
	"light": {"back": "#CBD5E1", "middle": "#1E293B", "top": "#0562EF", "bar": "#FFFFFF"},
	"dark":  {"back": "#334155", "middle": "#64748B", "top": "#3B82F6", "bar": "#DBEAFE"},
}

// canonicalBlocks reads DESIGN.md's two ```svg blocks in the brand section.
func canonicalBlocks(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile("../../DESIGN.md")
	if err != nil {
		t.Fatal(err)
	}
	design := string(b)
	start := strings.Index(design, "## Brand mark and wordmark")
	if start < 0 {
		t.Fatal("DESIGN.md has no brand section")
	}
	end := strings.Index(design[start+1:], "\n## ") + start + 1
	blocks := regexp.MustCompile("(?s)```svg\n(.*?)\n```").FindAllStringSubmatch(design[start:end], -1)
	if len(blocks) != 2 {
		t.Fatalf("DESIGN.md brand section holds %d svg blocks, want 2", len(blocks))
	}
	return map[string]string{"light": blocks[0][1] + "\n", "dark": blocks[1][1] + "\n"}
}

// The committed sources are byte-for-byte copies of DESIGN.md's canonical
// light and dark SVG, and that SVG is the L3-S3 geometry: three solid
// portrait 16 x 19 cards (rx 2.5), back and middle upright, the top card
// and its prompt bar rotated 10° clockwise around (19, 14).
func TestDesign_S125_1_CanonicalSVGIsCopiedVerbatim(t *testing.T) {
	want := canonicalBlocks(t)
	for mode, pal := range l3s3 {
		got, err := os.ReadFile("brand/mark-" + mode + ".svg")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want[mode] {
			t.Errorf("brand/mark-%s.svg is not DESIGN.md's canonical %s block", mode, mode)
		}
		if embedded := map[string]string{"light": markLightSVG, "dark": markDarkSVG}[mode]; embedded != want[mode] {
			t.Errorf("the embedded %s mark is not DESIGN.md's canonical block", mode)
		}
		sum := sha256.Sum256(got)
		t.Logf("brand/mark-%s.svg sha256 %s", mode, hex.EncodeToString(sum[:]))
		s := string(got)
		for _, w := range []string{
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">`,
			`<rect x="5" y="10" width="16" height="19" rx="2.5" fill="` + pal["back"] + `"/>`,
			`<rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="` + pal["middle"] + `"/>`,
			`<g transform="rotate(10 19 14)">`,
			`<rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="` + pal["top"] + `"/>`,
			`<rect x="14.5" y="7" width="9" height="2" rx="1" fill="` + pal["bar"] + `" opacity="0.85"/>`,
		} {
			if !strings.Contains(s, w) {
				t.Errorf("canonical %s mark is missing %q", mode, w)
			}
		}
		for _, bad := range []string{"stroke", "<path", "<circle", "<animate"} {
			if strings.Contains(s, bad) {
				t.Errorf("canonical %s mark holds %q", mode, bad)
			}
		}
	}
}

// The header mark is the canonical drawing, both fill sets, scaled whole to
// 24 px: nothing redrawn, rounded or hinted, and the S3-A deck gone.
func TestDesign_S125_1_HeaderMarkIsTheCanonicalDrawing(t *testing.T) {
	blocks := canonicalBlocks(t)
	inline := string(brandMark())
	want := `<svg class="mark" viewBox="0 0 32 32" width="24" height="24" fill="none" aria-hidden="true" focusable="false">` +
		`<g class="mark-light"><rect x="5" y="10" width="16" height="19" rx="2.5" fill="#CBD5E1"/><rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="#1E293B"/>` +
		`<g transform="rotate(10 19 14)"><rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="#0562EF"/><rect x="14.5" y="7" width="9" height="2" rx="1" fill="#FFFFFF" opacity="0.85"/></g></g>` +
		`<g class="mark-dark"><rect x="5" y="10" width="16" height="19" rx="2.5" fill="#334155"/><rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="#64748B"/>` +
		`<g transform="rotate(10 19 14)"><rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="#3B82F6"/><rect x="14.5" y="7" width="9" height="2" rx="1" fill="#DBEAFE" opacity="0.85"/></g></g></svg>`
	if inline != want {
		t.Errorf("inline mark:\n%s\nwant:\n%s", inline, want)
	}
	// The same elements, in the same order, as each canonical block.
	tag := regexp.MustCompile(`<[^>]+>`)
	for mode, block := range blocks {
		body := block[strings.Index(block, ">")+1 : strings.LastIndex(block, "</svg>")]
		elems := strings.Join(tag.FindAllString(body, -1), "")
		if !strings.Contains(inline, `<g class="mark-`+mode+`">`+elems+`</g>`) {
			t.Errorf("inline mark does not carry the canonical %s drawing", mode)
		}
	}
}

// The style sheet shows the active mode's drawing (system setting unless
// Settings says otherwise), never recolors it from theme tokens and never
// animates it.
func TestDesign_S125_1_MarkSwitchesWithTheModeAndNeverAnimates(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, w := range []string{
		".mark { width: 24px; height: 24px; flex: none;",
		".mark-dark { display: none; }",
		"@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) .mark-light { display: none; }\n  :root:not([data-theme=\"light\"]) .mark-dark { display: inline; }\n}",
		":root[data-theme=\"dark\"] .mark-light { display: none; }",
		":root[data-theme=\"dark\"] .mark-dark { display: inline; }",
		".wordmark { gap: var(--s2); }",
	} {
		if !strings.Contains(css, w) {
			t.Errorf("app.css is missing %q", w)
		}
	}
	rule := regexp.MustCompile(`([^{}]*)\{([^{}]*)\}`)
	sel := regexp.MustCompile(`(^|[\s,>+~(])\.mark([\s,.:{\[>)-]|$)`)
	for _, m := range rule.FindAllStringSubmatch(css, -1) {
		if !sel.MatchString(m[1]) {
			continue
		}
		for _, bad := range []string{"animation", "transition", "transform", "view-transition", "fill", "var(--accent)"} {
			if strings.Contains(m[2], bad) {
				t.Errorf("the mark is restyled: %s{%s}", strings.TrimSpace(m[1]), m[2])
			}
		}
	}
	for _, gone := range []string{"mark-far", "mark-near", "mark-face", "mark-top", "mark-bar", "mark-hinted", "mark-exact"} {
		if strings.Contains(css, gone) {
			t.Errorf("app.css still styles the S3-A deck (%s)", gone)
		}
	}
}

// The SVG favicon is one 32 x 32 SVG carrying both canonical drawings,
// unchanged, switched by prefers-color-scheme.
func TestDesign_S125_1_SVGFaviconCarriesBothCanonicalDrawings(t *testing.T) {
	fav, err := staticFS.ReadFile("static/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	s := string(fav)
	for _, w := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">`,
		`<style>.mark-dark{display:none}@media (prefers-color-scheme:dark){.mark-light{display:none}.mark-dark{display:inline}}</style>`,
		`<g class="mark-light">` + markDrawing(markLightSVG) + `</g>`,
		`<g class="mark-dark">` + markDrawing(markDarkSVG) + `</g>`,
	} {
		if !strings.Contains(s, w) {
			t.Errorf("favicon.svg is missing %q", w)
		}
	}
	if strings.Contains(s, "crispEdges") || strings.Contains(s, "<path") {
		t.Error("favicon.svg is a redrawing, not the canonical mark")
	}
}

func nrgba(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func decodeStatic(t *testing.T, name string, size int) image.Image {
	t.Helper()
	b, err := staticFS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if r := img.Bounds(); r.Dx() != size || r.Dy() != size {
		t.Fatalf("%s is %v, want %dx%d", name, r, size, size)
	}
	return img
}

// The 16 and 32 px PNG favicons are rasterised from the canonical drawing
// with the upright cards snapped to whole pixels: their left and bottom
// edges are crisp (a solid brand pixel beside a transparent one), the cards
// stay portrait solid slabs, and the cobalt top card keeps its tilt and its
// lighter prompt bar.
func TestDesign_S125_1_PNGFaviconsAreHintedFromTheCanonicalMark(t *testing.T) {
	light := l3s3["light"]
	type probe struct {
		x, y int
		part string // "" transparent, or a palette key, exact and opaque
	}
	for _, tc := range []struct {
		file   string
		size   int
		probes []probe
		// columns whose topmost opaque top-card pixel shows the tilt
		left, right int
	}{
		// 16 px: back card x 3..11, y 5..15; middle x 4..12, y 4..14 (8 x 10).
		{"static/favicon-16.png", 16, []probe{
			{2, 9, ""}, {3, 9, "back"}, {4, 9, "middle"}, {6, 12, "middle"}, {6, 13, "middle"}, {6, 14, "back"}, {6, 15, ""},
		}, 7, 13},
		// 32 px: back card x 5..21, y 10..29; middle x 7..23, y 8..27 (both 16 x 19).
		{"static/favicon-32.png", 32, []probe{
			{4, 20, ""}, {5, 20, "back"}, {6, 20, "back"}, {7, 20, "middle"}, {12, 26, "middle"}, {12, 27, "back"}, {12, 28, "back"}, {12, 29, ""},
		}, 14, 27},
	} {
		img := decodeStatic(t, tc.file, tc.size)
		part := func(x, y int) string {
			c := nrgba(img, x, y)
			if c.A == 0 {
				return ""
			}
			for name, hx := range light {
				if c == hexColor(hx) {
					return name
				}
			}
			return "blend"
		}
		for _, p := range tc.probes {
			if got := part(p.x, p.y); got != p.part {
				t.Errorf("%s pixel (%d,%d) is %q %v, want %q", tc.file, p.x, p.y, got, img.At(p.x, p.y), p.part)
			}
		}
		// The cobalt top card is solid somewhere, and its prompt bar shows
		// lighter on it.
		top, bar := false, false
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				c := nrgba(img, x, y)
				top = top || c == hexColor(light["top"])
				bar = bar || (c.A == 255 && c.B > 230 && c.R > 120 && c.R < 250)
			}
		}
		if !top || !bar {
			t.Errorf("%s: solid top card %v, prompt bar %v", tc.file, top, bar)
		}
		// The tilt: the top card's top edge falls to the right (rotated 10°
		// clockwise), so its first solid pixel is lower in the right column.
		first := func(x int) int {
			for y := 0; y < tc.size; y++ {
				if c := nrgba(img, x, y); c.A == 255 && c.B > 200 && c.R < 120 {
					return y
				}
			}
			return -1
		}
		if l, r := first(tc.left), first(tc.right); l < 0 || r < 0 || r-l < tc.size/16 {
			t.Errorf("%s: the top card is not tilted (top edge at y %d in column %d, %d in column %d)", tc.file, l, tc.left, r, tc.right)
		}
		// Nothing clipped: the outermost row and column are clear.
		for i := 0; i < tc.size; i++ {
			for _, c := range []color.NRGBA{nrgba(img, i, 0), nrgba(img, 0, i), nrgba(img, i, tc.size-1), nrgba(img, tc.size-1, i)} {
				if c.A != 0 {
					t.Fatalf("%s: the mark touches the edge", tc.file)
				}
			}
		}
	}
	ico, err := staticFS.ReadFile("static/favicon.ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(ico) < 6+32 || binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 2 || ico[6] != 16 || ico[6+16] != 32 {
		t.Errorf("favicon.ico is not the 16 and 32 px icon (% x)", ico[:min(len(ico), 24)])
	}
	// The hinted sources move only the upright cards, to the target pixel
	// grid; the rotated top card and bar are the canonical elements.
	for _, size := range []string{"16", "32"} {
		for mode := range l3s3 {
			b, err := os.ReadFile("brand/favicon-" + size + "-" + mode + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			canon := map[string]string{"light": markLightSVG, "dark": markDarkSVG}[mode]
			g := canon[strings.Index(canon, "  <g transform"):]
			if !strings.Contains(s, g) || strings.Count(s, `width="16" height="`) != 3 {
				t.Errorf("brand/favicon-%s-%s.svg changes the rotated top card or the card size", size, mode)
			}
		}
	}
}

// The app icons: the canonical drawing scaled whole to 62.5% (40 px on 64)
// and centred in a continuous-corner squircle, #FFFFFF with a 1 px line
// edge and the light fills, or #12101A with the dark fills; the full-bleed
// dark icon is the apple-touch and maskable icon. Every mark pixel stays in
// the maskable safe zone.
func TestDesign_S125_1_AppIconsCarryTheCanonicalMark(t *testing.T) {
	read := func(name string) string {
		b, err := staticFS.ReadFile(name)
		if err != nil {
			b, err = os.ReadFile(name)
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	scaled := `<g class="mark" transform="translate(12 12) scale(1.25)">`
	ground := regexp.MustCompile(`<path class="ground" d="M([^"]+)Z" fill="(#[0-9A-F]{6})"( stroke="(#[0-9A-F]{6})" stroke-width="1")?/>`)
	for file, want := range map[string][3]string{
		"static/app-icon.svg":      {"#FFFFFF", "#E2E2E3", markLightSVG},
		"static/app-icon-dark.svg": {"#12101A", "", markDarkSVG},
	} {
		s := read(file)
		if !strings.Contains(s, scaled+markDrawing(want[2])+`</g>`) {
			t.Errorf("%s does not carry the canonical mark at 62.5%%", file)
		}
		m := ground.FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("%s has no superellipse ground", file)
		}
		if m[2] != want[0] || m[4] != want[1] {
			t.Errorf("%s ground %s edge %q, want %s edge %q", file, m[2], m[4], want[0], want[1])
		}
		if pts := strings.Split(m[1], "L"); len(pts) < 64 || strings.ContainsAny(m[1], "AC") {
			t.Errorf("%s ground is not a sampled superellipse (%d points)", file, len(pts))
		}
	}
	if s := read("brand/icon-maskable.svg"); !strings.Contains(s, `<rect class="ground" width="64" height="64" fill="#12101A"/>`) || !strings.Contains(s, scaled+markDrawing(markDarkSVG)+`</g>`) {
		t.Error("the maskable source is not the full-bleed dark icon with the canonical mark")
	}

	near := func(c color.Color, hx string) bool {
		a, b := color.NRGBAModel.Convert(c).(color.NRGBA), hexColor(hx)
		d := func(x, y uint8) int { return int(math.Abs(float64(x) - float64(y))) }
		return a.A == 255 && d(a.R, b.R) <= 2 && d(a.G, b.G) <= 2 && d(a.B, b.B) <= 2
	}
	for _, tc := range []struct {
		file, mode, ground string
		size               int
		bleed              bool
	}{
		{"static/app-icon-64.png", "light", "#FFFFFF", 64, false},
		{"static/app-icon-64-dark.png", "dark", "#12101A", 64, false},
		{"static/icon-192.png", "dark", "#12101A", 192, false},
		{"static/icon-512.png", "dark", "#12101A", 512, false},
		{"static/icon-180.png", "dark", "#12101A", 180, true},
		{"static/icon-maskable-192.png", "dark", "#12101A", 192, true},
		{"static/icon-maskable-512.png", "dark", "#12101A", 512, true},
	} {
		img := decodeStatic(t, tc.file, tc.size)
		if corner := nrgba(img, 0, 0); tc.bleed != (corner.A == 255) {
			t.Errorf("%s corner alpha %d: full bleed %v", tc.file, corner.A, tc.bleed)
		}
		if !near(img.At(tc.size/2, tc.size/16), tc.ground) {
			t.Errorf("%s ground is %v, want %s", tc.file, img.At(tc.size/2, tc.size/16), tc.ground)
		}
		pal := l3s3[tc.mode]
		safe := 0.4 * float64(tc.size)
		c := float64(tc.size) / 2
		seen := map[string]bool{}
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				px := img.At(x, y)
				for _, k := range []string{"back", "middle", "top"} {
					if near(px, pal[k]) {
						seen[k] = true
						if math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c) > safe {
							t.Fatalf("%s: mark pixel (%d,%d) %v outside the safe zone", tc.file, x, y, px)
						}
					}
				}
			}
		}
		if !seen["back"] || !seen["middle"] || !seen["top"] {
			t.Errorf("%s shows cards %v, want back, middle and top in the %s fills", tc.file, seen, tc.mode)
		}
	}
	if img := decodeStatic(t, "static/app-icon-64.png", 64); !near(img.At(32, 0), "#E2E2E3") || !near(img.At(32, 1), "#FFFFFF") {
		t.Errorf("light app icon edge: %v over %v", img.At(32, 0), img.At(32, 1))
	}
}
