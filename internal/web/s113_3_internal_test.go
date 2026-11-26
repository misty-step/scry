package web

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// [S113-3] [S125-1] One mark everywhere: the L3-S3 fanned deck with its
// cobalt top card tilted, drawn on every surface from DESIGN.md's one
// canonical SVG, verbatim. These tests pin the drawing itself (geometry,
// fills, hinting, app icons); the served side (header, favicon, manifest,
// apple-touch icon) is TestStory_S113_3_S125_1_OneL3S3MarkEverywhere.

// The layer table's fixed brand fills (DESIGN.md, Brand mark and wordmark).
var (
	l3Light = map[string]string{"back": "#CBD5E1", "middle": "#1E293B", "top": "#0562EF", "bar": "#FFFFFF"}
	l3Dark  = map[string]string{"back": "#334155", "middle": "#64748B", "top": "#3B82F6", "bar": "#DBEAFE"}
)

func hexColor(s string) color.NRGBA {
	var c color.NRGBA
	c.A = 255
	for i, p := range []*uint8{&c.R, &c.G, &c.B} {
		v, _ := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		*p = uint8(v)
	}
	return c
}

func readRepo(t *testing.T, name string) string {
	t.Helper()
	b, err := staticFS.ReadFile(name)
	if err != nil {
		b, err = os.ReadFile(name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// designMarks returns DESIGN.md's two canonical SVG blocks (light, dark),
// verbatim, from the "Brand mark and wordmark" section.
func designMarks(t *testing.T) (light, dark string) {
	t.Helper()
	doc := readRepo(t, "../../DESIGN.md")
	start := strings.Index(doc, "\n## Brand mark and wordmark")
	if start < 0 {
		t.Fatal("DESIGN.md has no Brand mark and wordmark section")
	}
	sec := doc[start+1:]
	if end := strings.Index(sec, "\n## "); end >= 0 {
		sec = sec[:end]
	}
	blocks := regexp.MustCompile("(?s)```svg\n(.*?)```").FindAllStringSubmatch(sec, -1)
	if len(blocks) != 2 {
		t.Fatalf("DESIGN.md's brand section has %d svg blocks, want 2", len(blocks))
	}
	if !strings.Contains(sec, "Light (`page` `#FFFFFF`):\n\n```svg") || !strings.Contains(sec, "Dark (`page` `#12101A`):\n\n```svg") {
		t.Fatal("DESIGN.md's svg blocks are not labelled Light then Dark")
	}
	return blocks[0][1], blocks[1][1]
}

// inner is an SVG's drawing without its <svg> wrapper, the whitespace
// between tags or any class attribute: the geometry and fills alone.
func inner(svg string) string {
	svg = regexp.MustCompile(`>\s+<`).ReplaceAllString(strings.TrimSpace(svg), "><")
	svg = regexp.MustCompile(` class="[^"]*"`).ReplaceAllString(svg, "")
	svg = regexp.MustCompile(`(?s)<style>.*?</style>`).ReplaceAllString(svg, "")
	svg = strings.TrimSuffix(svg[strings.Index(svg, ">")+1:], "</svg>")
	return svg
}

// The canonical SVG is binding and verbatim: DESIGN.md's light and dark
// blocks are byte for byte the committed sources, and every surface (the
// inline header mark, the SVG favicon, the 64 px app icons and the maskable
// icon) draws exactly the same elements with the same numbers and fills.
func TestDesign_S125_1_CanonicalSVGIsDesignVerbatim(t *testing.T) {
	light, dark := designMarks(t)
	if got := readRepo(t, "brand/mark-light.svg"); got != light {
		t.Errorf("brand/mark-light.svg is not DESIGN.md's light block:\n%s\nwant\n%s", got, light)
	}
	if got := readRepo(t, "brand/mark-dark.svg"); got != dark {
		t.Errorf("brand/mark-dark.svg is not DESIGN.md's dark block:\n%s\nwant\n%s", got, dark)
	}
	// The blocks themselves say what the layer table says: three portrait
	// 16 x 19 cards (rx 2.5) back to front, the top one and its bar inside
	// one 10 degree clockwise rotation about (19, 14); only fills differ.
	for name, c := range map[string]struct {
		svg string
		pal map[string]string
	}{"light": {light, l3Light}, "dark": {dark, l3Dark}} {
		want := `<rect x="5" y="10" width="16" height="19" rx="2.5" fill="` + c.pal["back"] + `"/>` +
			`<rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="` + c.pal["middle"] + `"/>` +
			`<g transform="rotate(10 19 14)">` +
			`<rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="` + c.pal["top"] + `"/>` +
			`<rect x="14.5" y="7" width="9" height="2" rx="1" fill="` + c.pal["bar"] + `" opacity="0.85"/>` +
			`</g>`
		if got := inner(c.svg); got != want {
			t.Errorf("DESIGN.md %s block draws\n%s\nwant the layer table's\n%s", name, got, want)
		}
		if !strings.HasPrefix(c.svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">`) {
			t.Errorf("DESIGN.md %s block is not on the 32 x 32 grid", name)
		}
	}
	lightInner, darkInner := inner(light), inner(dark)

	// The header: the canonical light drawing scaled whole to 24 px (the
	// style sheet switches the fills to the dark pair in dark mode).
	header := string(brandMark())
	if !strings.HasPrefix(header, `<svg class="mark" viewBox="0 0 32 32" width="24" height="24" fill="none" aria-hidden="true" focusable="false">`) {
		t.Errorf("header mark opens %.120q", header)
	}
	if got := inner(header); got != lightInner {
		t.Errorf("header mark draws\n%s\nwant\n%s", got, lightInner)
	}
	for _, cls := range []string{"mark-back", "mark-middle", "mark-top", "mark-bar"} {
		if strings.Count(header, `class="`+cls+`"`) != 1 {
			t.Errorf("header mark has no single %s layer", cls)
		}
	}

	// The SVG favicon: the same grid and drawing, both fill sets switched by
	// prefers-color-scheme.
	fav := readRepo(t, "static/favicon.svg")
	if !strings.HasPrefix(fav, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"`) {
		t.Errorf("favicon.svg opens %.100q", fav)
	}
	if got := inner(fav); got != lightInner {
		t.Errorf("favicon.svg draws\n%s\nwant\n%s", got, lightInner)
	}
	if !strings.Contains(fav, "<style>@media (prefers-color-scheme:dark){.mark-back{fill:#334155}.mark-middle{fill:#64748B}.mark-top{fill:#3B82F6}.mark-bar{fill:#DBEAFE}}</style>") {
		t.Error("favicon.svg does not switch to the dark fills by prefers-color-scheme")
	}

	// The app icons: the canonical drawing scaled whole to 62.5% (40 px on
	// 64), centred: light fills on the light icon, dark on the dark and
	// maskable ones.
	group := regexp.MustCompile(`<g class="mark" transform="translate\(12 12\) scale\(1\.25\)">(.*)</g></svg>`)
	for file, want := range map[string]string{
		"static/app-icon.svg": lightInner, "static/app-icon-dark.svg": darkInner, "brand/icon-maskable.svg": darkInner,
	} {
		m := group.FindStringSubmatch(readRepo(t, file))
		if m == nil {
			t.Errorf("%s has no mark scaled whole to 40 px", file)
			continue
		}
		if m[1] != want {
			t.Errorf("%s draws\n%s\nwant\n%s", file, m[1], want)
		}
	}

	// The hinted favicon sources: the tilted top card and its bar exactly as
	// drawn; the two upright cards only snapped so their edges land on whole
	// pixels at the favicon's size, still portrait, still fanned up and
	// right (middle 2 units right of and above the back card), each within
	// half a pixel of the canonical position.
	rotated := lightInner[strings.Index(lightInner, "<g "):]
	rect := regexp.MustCompile(`^<rect x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)" rx="2.5" fill="(#[0-9A-F]{6})"/>`)
	for _, tc := range []struct {
		file string
		size float64
		pal  map[string]string
	}{
		{"brand/favicon-16-light.svg", 16, l3Light}, {"brand/favicon-16-dark.svg", 16, l3Dark},
		{"brand/favicon-32-light.svg", 32, l3Light}, {"brand/favicon-32-dark.svg", 32, l3Dark},
	} {
		src := readRepo(t, tc.file)
		if !strings.Contains(src, `viewBox="0 0 32 32" width="`+strconv.Itoa(int(tc.size))+`"`) {
			t.Errorf("%s is not the 32 grid at %v px", tc.file, tc.size)
		}
		body := inner(src)
		wantRot := rotated
		if tc.pal["top"] != l3Light["top"] {
			wantRot = darkInner[strings.Index(darkInner, "<g "):]
		}
		if !strings.HasSuffix(body, wantRot) {
			t.Errorf("%s does not draw the canonical tilted top card: %s", tc.file, body)
		}
		px := tc.size / 32
		var cards [2][4]float64
		rest := body
		for i, layer := range []string{"back", "middle"} {
			m := rect.FindStringSubmatch(rest)
			if m == nil {
				t.Fatalf("%s: no upright %s card in %s", tc.file, layer, rest)
			}
			if m[5] != tc.pal[layer] {
				t.Errorf("%s %s card fill %s, want %s", tc.file, layer, m[5], tc.pal[layer])
			}
			for k := 0; k < 4; k++ {
				cards[i][k], _ = strconv.ParseFloat(m[1+k], 64)
			}
			x, y, w, h := cards[i][0], cards[i][1], cards[i][2], cards[i][3]
			for _, edge := range []float64{x, y, x + w, y + h} {
				if v := edge * px; v != math.Round(v) {
					t.Errorf("%s %s card edge %v lands on %v px", tc.file, layer, edge, v)
				}
			}
			if !(h > w) || w != 16 {
				t.Errorf("%s %s card is %vx%v, not a portrait 16-wide card", tc.file, layer, w, h)
			}
			canon := map[string][2]float64{"back": {5, 10}, "middle": {7, 7.5}}[layer]
			if math.Abs(x-canon[0])*px > 0.5 || math.Abs(y-canon[1])*px > 0.5 || math.Abs(h-19)*px > 0.5 {
				t.Errorf("%s %s card moved more than half a pixel: %v", tc.file, layer, cards[i])
			}
			rest = rest[len(m[0]):]
		}
		if cards[1][0]-cards[0][0] != 2 || cards[0][1]-cards[1][1] != 2 {
			t.Errorf("%s: the middle card is not fanned up and right of the back card: %v", tc.file, cards)
		}
	}
}

// The style sheet gives each layer its fixed brand fill for the active mode
// (light by default, the dark pair under a dark system setting unless
// Settings says light, and under Settings' dark), never the theme's accent,
// and never animates the mark.
func TestDesign_S125_1_MarkFillsFollowTheModeAndNeverAnimate(t *testing.T) {
	css := readRepo(t, "static/app.css")
	for _, w := range []string{
		".mark { width: 24px; height: 24px; flex: none;",
		".mark-back { fill: var(--mark-back); }",
		".mark-middle { fill: var(--mark-middle); }",
		".mark-top { fill: var(--mark-top); }",
		".mark-bar { fill: var(--mark-bar); }",
		".wordmark { gap: var(--s2); }",
	} {
		if !strings.Contains(css, w) {
			t.Errorf("app.css is missing %q", w)
		}
	}
	block := func(open string) string {
		i := strings.Index(css, open)
		if i < 0 {
			t.Fatalf("app.css has no %q block", open)
		}
		j := strings.Index(css[i:], "\n}")
		return css[i : i+j]
	}
	for open, pal := range map[string]map[string]string{
		":root {\n": l3Light, ":root:not([data-theme=\"light\"]) {": l3Dark, ":root[data-theme=\"dark\"] {": l3Dark,
	} {
		b := block(open)
		for _, layer := range []string{"back", "middle", "top", "bar"} {
			if !strings.Contains(b, "--mark-"+layer+": "+pal[layer]+";") {
				t.Errorf("%s sets no --mark-%s: %s", open, layer, pal[layer])
			}
		}
	}
	if !strings.Contains(css, "@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) {") {
		t.Error("the dark mark fills do not follow the system setting")
	}
	// Every rule whose selector names the mark (not .trail-mark, not the
	// highlighter <mark>) sets no motion and no theme accent.
	rule := regexp.MustCompile(`([^{}]*)\{([^{}]*)\}`)
	sel := regexp.MustCompile(`(^|[\s,>+~(])\.mark([\s,.:{\[>)-]|$)`)
	for _, m := range rule.FindAllStringSubmatch(css, -1) {
		if !sel.MatchString(m[1]) {
			continue
		}
		for _, bad := range []string{"animation", "transition", "transform", "view-transition", "--accent"} {
			if strings.Contains(m[2], bad) {
				t.Errorf("the mark rule sets %s: %s{%s}", bad, strings.TrimSpace(m[1]), m[2])
			}
		}
	}
}

// The 16 and 32 px PNG favicons are rasterised from the canonical SVG and
// hinted: the upright cards' edges land on whole pixels (each pixel on
// either side of an edge is wholly one layer), the cards stay portrait and
// solid, the cobalt top card stays tilted with its lighter prompt bar, and
// nothing is clipped at the image's edge.
func TestDesign_S125_1_FaviconsAreHintedAndKeepTheMark(t *testing.T) {
	type probe struct {
		x, y int
		part string
	}
	for _, tc := range []struct {
		file   string
		size   int
		probes []probe
		back   [4]int // x0, y0, x1, y1 in pixels (exclusive end)
		middle [4]int
	}{
		{"static/favicon-16.png", 16, []probe{
			// Row 9: nothing, back's 1 px left edge, the middle card.
			{1, 9, ""}, {2, 9, "back"}, {3, 9, "middle"}, {4, 9, "middle"},
			// Column 6: the back card's 1 px foot below the middle card.
			{6, 13, "middle"}, {6, 14, "back"}, {6, 15, ""},
		}, [4]int{2, 5, 10, 15}, [4]int{3, 4, 11, 14}},
		{"static/favicon-32.png", 32, []probe{
			{4, 20, ""}, {5, 20, "back"}, {6, 20, "back"}, {7, 20, "middle"}, {8, 20, "middle"},
			{12, 26, "middle"}, {12, 27, "back"}, {12, 28, "back"}, {12, 29, ""},
		}, [4]int{5, 10, 21, 29}, [4]int{7, 8, 23, 27}},
	} {
		img, err := png.Decode(strings.NewReader(readRepo(t, tc.file)))
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if r := img.Bounds(); r.Dx() != tc.size || r.Dy() != tc.size {
			t.Fatalf("%s is %v, want %dx%d", tc.file, r, tc.size, tc.size)
		}
		at := func(x, y int) color.NRGBA { return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) }
		part := func(x, y int) string {
			c := at(x, y)
			if c.A == 0 {
				return ""
			}
			for _, name := range []string{"back", "middle", "top"} {
				if c == hexColor(l3Light[name]) {
					return name
				}
			}
			return "blend"
		}
		for _, p := range tc.probes {
			if got := part(p.x, p.y); got != p.part {
				t.Errorf("%s pixel (%d,%d) is %q %v, want %q", tc.file, p.x, p.y, got, at(p.x, p.y), p.part)
			}
		}
		// Whole-pixel upright edges: along each card's straight left and
		// bottom edges (clear of the rounded corners and the tilted card),
		// the pixel just inside is solid and the one just outside is not that
		// card's colour at partial coverage.
		// (Rows start below both cards' rounded top corners, so the pixel
		// outside the middle card is the back card's solid face.)
		for name, b := range map[string][4]int{"back": tc.back, "middle": tc.middle} {
			solid := 0
			for y := max(b[1], tc.back[1]) + 2; y < b[3]-2; y++ {
				if part(b[0], y) == name {
					solid++
				}
				if out := part(b[0]-1, y); out == "blend" || out == name {
					t.Errorf("%s: the %s card's left edge bleeds into x %d at y %d: %v", tc.file, name, b[0]-1, y, at(b[0]-1, y))
				}
			}
			if solid < (b[3]-b[1])/2 {
				t.Errorf("%s: the %s card's left edge is solid on only %d rows", tc.file, name, solid)
			}
			if w, h := b[2]-b[0], b[3]-b[1]; !(h > w) {
				t.Errorf("%s: the %s card is %dx%d, not portrait", tc.file, name, w, h)
			}
		}
		// The top card is tilted clockwise: its left edge runs down and to
		// the left (in a canonical 10 degree tilt, x falls by about 3.3 grid
		// units over its 19-unit height).
		left := func(y int) int {
			for x := 0; x < tc.size; x++ {
				if part(x, y) == "top" {
					return x
				}
			}
			return -1
		}
		hi, lo := left(tc.size*7/32), left(tc.size*19/32)
		if hi < 0 || lo < 0 || hi-lo < tc.size/16 {
			t.Errorf("%s: the top card is not tilted (left edge x %d at the top, %d lower down)", tc.file, hi, lo)
		}
		// The prompt bar: opaque pixels clearly lighter than the cobalt
		// card, inside its upper part.
		top := hexColor(l3Light["top"])
		bar := 0
		for y := 0; y < tc.size/2; y++ {
			for x := tc.size / 3; x < tc.size; x++ {
				if c := at(x, y); c.A == 255 && int(c.R) > int(top.R)+90 && int(c.G) > int(top.G)+60 {
					bar++
				}
			}
		}
		if bar < tc.size/8 {
			t.Errorf("%s: the prompt bar is not readable (%d light pixels)", tc.file, bar)
		}
		// Nothing clipped: the image's outermost ring is empty.
		for i := 0; i < tc.size; i++ {
			for _, p := range [][2]int{{i, 0}, {i, tc.size - 1}, {0, i}, {tc.size - 1, i}} {
				if at(p[0], p[1]).A != 0 {
					t.Errorf("%s: the mark touches the edge at %v", tc.file, p)
				}
			}
		}
	}
	ico := readRepo(t, "static/favicon.ico")
	if len(ico) < 6+32 || binary.LittleEndian.Uint16([]byte(ico[2:])) != 1 || binary.LittleEndian.Uint16([]byte(ico[4:])) != 2 || ico[6] != 16 || ico[6+16] != 32 {
		t.Errorf("favicon.ico is not the 16 and 32 px icon")
	}
}

// The app icons, 64, 180, 192 and 512 px: the mark at 62.5% of the icon,
// centred in a continuous-corner superellipse; #FFFFFF with a 1 px line edge
// and the light fills, or #12101A with the dark fills. The dark icon is the
// home-screen (192, 512), the maskable icon (mark inside the central 80%
// circle) and the apple-touch and social preview image.
func TestDesign_S125_1_AppIconsSquircleAndMaskable(t *testing.T) {
	ground := regexp.MustCompile(`<path class="ground" d="M([^"]+)Z" fill="(#[0-9A-F]{6})"( stroke="(#[0-9A-F]{6})" stroke-width="1")?/>`)
	for file, want := range map[string][2]string{"static/app-icon.svg": {"#FFFFFF", "#E2E2E3"}, "static/app-icon-dark.svg": {"#12101A", ""}} {
		s := readRepo(t, file)
		m := ground.FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("%s has no superellipse ground", file)
		}
		if m[2] != want[0] || m[4] != want[1] {
			t.Errorf("%s ground %s edge %q, want %s edge %q", file, m[2], m[4], want[0], want[1])
		}
		// A superellipse, not a rounded rect: many points, no arcs or curves.
		pts := strings.Split(m[1], "L")
		if len(pts) < 64 || strings.Contains(m[1], "A") || strings.Contains(m[1], "C") {
			t.Errorf("%s ground is not a sampled superellipse (%d points)", file, len(pts))
		}
	}
	if s := readRepo(t, "brand/icon-maskable.svg"); !strings.Contains(s, `<rect class="ground" width="64" height="64" fill="#12101A"/>`) {
		t.Error("the maskable source is not the full-bleed dark icon")
	}

	decode := func(name string, size int) image.Image {
		t.Helper()
		img, err := png.Decode(strings.NewReader(readRepo(t, name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r := img.Bounds(); r.Dx() != size || r.Dy() != size {
			t.Fatalf("%s is %v, want %dx%d", name, r, size, size)
		}
		return img
	}
	near := func(c color.Color, hex string) bool {
		a, b := color.NRGBAModel.Convert(c).(color.NRGBA), hexColor(hex)
		d := func(x, y uint8) int { return int(math.Abs(float64(x) - float64(y))) }
		return a.A == 255 && d(a.R, b.R) <= 2 && d(a.G, b.G) <= 2 && d(a.B, b.B) <= 2
	}
	for _, tc := range []struct {
		file   string
		size   int
		ground string
		pal    map[string]string
		bleed  bool
	}{
		{"static/app-icon-64.png", 64, "#FFFFFF", l3Light, false},
		{"static/app-icon-64-dark.png", 64, "#12101A", l3Dark, false},
		{"static/icon-192.png", 192, "#12101A", l3Dark, false},
		{"static/icon-512.png", 512, "#12101A", l3Dark, false},
		{"static/icon-180.png", 180, "#12101A", l3Dark, true},
		{"static/icon-maskable-192.png", 192, "#12101A", l3Dark, true},
		{"static/icon-maskable-512.png", 512, "#12101A", l3Dark, true},
	} {
		img := decode(tc.file, tc.size)
		corner := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
		if tc.bleed != (corner.A == 255) {
			t.Errorf("%s corner alpha %d: full bleed %v", tc.file, corner.A, tc.bleed)
		}
		if !near(img.At(tc.size/2, tc.size/8), tc.ground) {
			t.Errorf("%s ground is %v, want %s", tc.file, img.At(tc.size/2, tc.size/8), tc.ground)
		}
		// Every layer shows in its mode's fill, and every mark pixel lies
		// inside the maskable safe zone (the central circle whose diameter
		// is 80% of the icon).
		safe := 0.4 * float64(tc.size)
		c := float64(tc.size) / 2
		seen := map[string]bool{}
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				px := img.At(x, y)
				in := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c) <= safe
				for _, k := range []string{"back", "middle", "top"} {
					if near(px, tc.pal[k]) {
						seen[k] = true
						if !in {
							t.Fatalf("%s: mark pixel (%d,%d) %v outside the safe zone", tc.file, x, y, px)
						}
					}
				}
			}
		}
		for _, k := range []string{"back", "middle", "top"} {
			if !seen[k] {
				t.Errorf("%s shows no %s card in %s", tc.file, k, tc.pal[k])
			}
		}
	}
	// The light icon's 1 px line edge runs along the top of the squircle.
	if img := decode("static/app-icon-64.png", 64); !near(img.At(32, 0), "#E2E2E3") || !near(img.At(32, 1), "#FFFFFF") {
		t.Errorf("light app icon edge: %v over %v", img.At(32, 0), img.At(32, 1))
	}
}

// The scrying lens and the interim flat outline deck of PR #515 are retired
// from every surface ([S125-1]: "neither may ship anywhere"): no template,
// static file, brand source, script or Go source still draws either, and the
// lens's old files are gone.
func TestDesign_S125_1_NoLensOrInterimDeckRemains(t *testing.T) {
	needles := []string{"le" + "ns", "fo" + "cal", "view" + "finder", "aper" + "ture", `r="2` + `.25"`, "M3 8V3" + "h5", "mark-" + "focal",
		// The interim S3-A deck: landscape 14 x 10 outlined cards with
		// page-filled faces and a 1x hinted twin.
		"s3" + "-a", "mark-" + "far", "mark-" + "near", "mark-" + "face", "mark-" + "hinted", "mark-" + "exact", `width="14` + `" height="10"`}
	check := func(name string, b []byte) {
		s := strings.ToLower(string(b))
		for _, n := range needles {
			if strings.Contains(s, n) {
				t.Errorf("%s still holds the old mark (%q)", name, n)
			}
		}
	}
	for _, root := range []string{"static", "templates"} {
		_ = fs.WalkDir(staticFS, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasPrefix(p, "static/fonts/") || strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".ico") {
				return nil
			}
			b, _ := staticFS.ReadFile(p)
			check(p, b)
			return nil
		})
		_ = fs.WalkDir(templateFS, root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := templateFS.ReadFile(p)
				check(p, b)
			}
			return nil
		})
	}
	for _, root := range []string{"../../internal", "../../cmd", "../../scripts"} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(p)
			if strings.HasSuffix(p, "_test.go") || (ext != ".go" && ext != ".svg" && ext != ".html" && ext != ".css" && ext != ".js" && ext != ".mjs" && ext != ".py") {
				return nil
			}
			b, _ := os.ReadFile(p)
			check(p, b)
			return nil
		})
	}
	for _, gone := range []string{"static/icon.svg", "static/icon-dark.svg"} {
		if _, err := staticFS.ReadFile(gone); err == nil {
			t.Errorf("%s still ships", gone)
		}
	}
}

// The Study icon (a stack of cards) is an interface icon: a monochrome line
// glyph in its text colour, never filled or coloured, so it is never
// mistaken for the brand mark.
func TestDesign_S113_3_StudyIconStaysALineGlyph(t *testing.T) {
	study := string(icon("cards"))
	for _, w := range []string{`class="icon icon-cards"`, `fill="none"`, `stroke="currentColor"`, `stroke-width="1.75"`} {
		if !strings.Contains(study, w) {
			t.Errorf("the Study icon is missing %q", w)
		}
	}
	inner := iconPaths["cards"]
	for _, bad := range []string{"fill", "#", "class=", "mark"} {
		if strings.Contains(inner, bad) {
			t.Errorf("the Study icon's drawing holds %q", bad)
		}
	}
	if strings.Count(inner, "<rect") > 1 || strings.Contains(string(brandMark()), inner) {
		t.Error("the Study icon draws like the brand mark")
	}
}
