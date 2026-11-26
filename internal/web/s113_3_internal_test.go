package web

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

// [S113-3] One mark everywhere: the S3-A deck of cards. These tests pin the
// drawing itself (geometry, palette, hinting, app icons); the served side
// (header, favicon, manifest, apple-touch icon) is
// TestStory_S113_3_OneMarkEverywhere.

// The S3-A palette (DESIGN.md, Color): receding cards are ink at 35% and 60%
// (ink-muted) composited over page.
var (
	s3aLight = map[string]string{"page": "#FFFFFF", "accent": "#0562EF", "near": "#6D7073", "far": "#AAABAD"}
	s3aDark  = map[string]string{"page": "#12101A", "accent": "#4D8DFF", "near": "#A09FA3", "far": "#65646A"}
)

func hexColor(s string) color.NRGBA {
	var c color.NRGBA
	c.A = 255
	for i, p := range []*uint8{&c.R, &c.G, &c.B} {
		var v uint8
		for _, ch := range strings.ToLower(s[1+2*i : 3+2*i]) {
			v *= 16
			if ch >= 'a' {
				v += uint8(ch-'a') + 10
			} else {
				v += uint8(ch - '0')
			}
		}
		*p = v
	}
	return c
}

func TestDesign_S113_3_InkStepsAreOverPage(t *testing.T) {
	mix := func(fg, bg string, a float64) string {
		f, b := hexColor(fg), hexColor(bg)
		ch := func(x, y uint8) uint8 { return uint8(math.Round(float64(y) + a*(float64(x)-float64(y)))) }
		return fmt.Sprintf("#%02X%02X%02X", ch(f.R, b.R), ch(f.G, b.G), ch(f.B, b.B))
	}
	for name, p := range map[string]map[string]string{"light": s3aLight, "dark": s3aDark} {
		ink := map[string]string{"light": "#0B1015", "dark": "#FFFFFF"}[name]
		if got := mix(ink, p["page"], 0.60); got != p["near"] {
			t.Errorf("%s ink-muted over page = %s, palette says %s", name, got, p["near"])
		}
		if got := mix(ink, p["page"], 0.35); got != p["far"] {
			t.Errorf("%s ink 35%% over page = %s, palette says %s", name, got, p["far"])
		}
	}
}

// The 24 px drawing: three identical 14 x 10 cards (rx 1.5), back to front,
// each 2.5 px up and right of the one in front; receding cards are an ink
// step with a page face inset by the 1.5 px outline; the top card is the
// accent with a 2 px round-capped prompt bar in page from x 5.5 to 12,
// centred on y 14.5. The inline mark and the raster sources are one drawing.
func TestDesign_S113_3_MarkGeometryIsOneDrawing(t *testing.T) {
	want := []string{
		`<rect class="mark-far" x="7.5" y="4.5" width="14" height="10" rx="1.5"`,
		`<rect class="mark-face" x="9" y="6" width="11" height="7"`,
		`<rect class="mark-near" x="5" y="7" width="14" height="10" rx="1.5"`,
		`<rect class="mark-face" x="6.5" y="8.5" width="11" height="7"`,
		`<rect class="mark-top" x="2.5" y="9.5" width="14" height="10" rx="1.5"`,
		`<rect class="mark-bar" x="5.5" y="13.5" width="6.5" height="2" rx="1"`,
	}
	inline := string(brandMark())
	if !strings.HasPrefix(inline, `<svg class="mark" viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false">`) {
		t.Errorf("inline mark opens %.120q", inline)
	}
	at := 0
	for _, w := range want {
		i := strings.Index(inline[at:], w)
		if i < 0 {
			t.Fatalf("inline mark is missing %q (in back-to-front order)", w)
		}
		at += i + len(w)
	}
	// Fallback fills are the light palette (the style sheet sets the theme's).
	has := func(where, body string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(body, w) {
				t.Errorf("%s is missing %q", where, w)
			}
		}
	}
	has("inline mark", inline, `mark-far" x="7.5" y="4.5" width="14" height="10" rx="1.5" fill="#AAABAD"`,
		`mark-near" x="5" y="7" width="14" height="10" rx="1.5" fill="#6D7073"`, `mark-top" x="2.5" y="9.5" width="14" height="10" rx="1.5" fill="#0562EF"`,
		`mark-bar" x="5.5" y="13.5" width="6.5" height="2" rx="1" fill="#FFFFFF"`)
	split := strings.Index(inline, `<g class="mark-hinted" display="none" shape-rendering="crispEdges">`)
	if split < 0 {
		t.Fatal("the inline mark has no 1x drawing")
	}
	exact, hinted := inline[:split], inline[split:]
	for _, bad := range []string{"<animate", "stroke=", "<circle", "<path"} {
		if strings.Contains(exact, bad) || (bad != "<path" && strings.Contains(hinted, bad)) {
			t.Errorf("inline mark contains %q", bad)
		}
	}

	// The rule, read back from the drawing: three identical 14 x 10 cards
	// with 1.5 px corners, each 2.5 px up and right of the one in front, the
	// deck centred on the grid, a 1.5 px outline, and a 2 px round-capped bar
	// from x 5.5 to 12 centred on the top card's height.
	rect := regexp.MustCompile(`<rect class="(mark-[a-z]+)" x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)"(?: rx="([\d.]+)")?`)
	type box struct{ x, y, w, h, rx float64 }
	var cards, faces []box
	var bar box
	for _, m := range rect.FindAllStringSubmatch(exact, -1) {
		var v [5]float64
		for i := range v {
			v[i], _ = strconv.ParseFloat(m[2+i], 64)
		}
		b := box{v[0], v[1], v[2], v[3], v[4]}
		switch m[1] {
		case "mark-far", "mark-near", "mark-top":
			cards = append(cards, b)
		case "mark-face":
			faces = append(faces, b)
		case "mark-bar":
			bar = b
		}
	}
	if len(cards) != 3 || len(faces) != 2 {
		t.Fatalf("%d cards and %d faces", len(cards), len(faces))
	}
	for i, c := range cards {
		if c.w != 14 || c.h != 10 || c.rx != 1.5 {
			t.Errorf("card %d is %vx%v rx %v", i, c.w, c.h, c.rx)
		}
		if i > 0 && (cards[i-1].x-c.x != 2.5 || c.y-cards[i-1].y != 2.5) {
			t.Errorf("card %d is not 2.5 px down-left of the one behind", i)
		}
		if i < 2 && faces[i] != (box{c.x + 1.5, c.y + 1.5, c.w - 3, c.h - 3, 0}) {
			t.Errorf("card %d's outline is not 1.5 px: face %+v", i, faces[i])
		}
	}
	if cards[2].x+cards[0].x+cards[0].w != 24 || cards[0].y+cards[2].y+cards[2].h != 24 {
		t.Error("the deck is not centred on the 24 px grid")
	}
	if top := cards[2]; bar.h != 2 || bar.rx != 1 || bar.x != 5.5 || bar.x+bar.w != 12 || bar.y+bar.h/2 != 14.5 || top.y+top.h/2 != 14.5 {
		t.Errorf("the prompt bar is %+v on the top card %+v", bar, top)
	}

	// The 1x drawing: the same deck on whole pixels, 3 px steps, 2 px
	// outlines 1 px apart, the bar 2 px tall and clear of the card's edges;
	// its raster source draws the same shapes.
	src, err := os.ReadFile("brand/mark-24-hinted-light.svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		`<path class="mark-far" d="M9 4H21V5H22V13H21V14H9V13H8V5H9Z"`, `<rect class="mark-face" x="10" y="6" width="10" height="6"`,
		`<path class="mark-near" d="M6 7H18V8H19V16H18V17H6V16H5V8H6Z"`, `<rect class="mark-face" x="7" y="9" width="10" height="6"`,
		`<path class="mark-top" d="M3 10H15V11H16V19H15V20H3V19H2V11H3Z"`, `<rect class="mark-bar" x="5" y="14" width="7" height="2"`,
	} {
		if !strings.Contains(hinted, w+" fill=") {
			t.Errorf("hinted header mark is missing %q", w)
		}
		if !strings.Contains(string(src), w+"/>") {
			t.Errorf("brand/mark-24-hinted-light.svg is missing %q", w)
		}
	}

	for file, pal := range map[string]map[string]string{"brand/mark-24-light.svg": s3aLight, "brand/mark-24-dark.svg": s3aDark} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, w := range want {
			has(file, s, w+"/>")
		}
		has(file, s, ".mark-far{fill:"+pal["far"]+"}", ".mark-near{fill:"+pal["near"]+"}",
			".mark-face,.mark-bar{fill:"+pal["page"]+"}", ".mark-top{fill:"+pal["accent"]+"}")
	}
}

// The style sheet colours the inline mark from the theme's own tokens and
// never animates it.
func TestDesign_S113_3_MarkFollowsTheThemeAndNeverAnimates(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, w := range []string{
		".mark { width: 24px; height: 24px; flex: none;",
		".mark-far { fill: color-mix(in srgb, var(--ink) 35%, var(--page)); }",
		".mark-near { fill: color-mix(in srgb, var(--ink) 60%, var(--page)); }",
		".mark-face, .mark-bar { fill: var(--page); }",
		".mark-top { fill: var(--accent); }",
		".wordmark { gap: var(--s2); }",
		"@media (max-resolution: 1dppx) {\n  .mark-exact { display: none; }\n  .mark-hinted { display: inline; }\n}",
	} {
		if !strings.Contains(css, w) {
			t.Errorf("app.css is missing %q", w)
		}
	}
	// Every rule whose selector names the mark (not .trail-mark, not the
	// highlighter <mark>) sets no motion.
	rule := regexp.MustCompile(`([^{}]*)\{([^{}]*)\}`)
	sel := regexp.MustCompile(`(^|[\s,>+~(])\.mark([\s,.:{\[>)-]|$)`)
	for _, m := range rule.FindAllStringSubmatch(css, -1) {
		if !sel.MatchString(m[1]) {
			continue
		}
		for _, motion := range []string{"animation", "transition", "transform", "view-transition"} {
			if strings.Contains(m[2], motion) {
				t.Errorf("the mark moves: %s{%s}", strings.TrimSpace(m[1]), m[2])
			}
		}
	}
}

// The 16 and 32 px favicons are hand-hinted: every pixel is exactly one of
// the palette's colours or fully transparent (no anti-aliased edge), parallel
// card edges are at least 1 px apart and the prompt bar is at least 1 px tall
// and clear of the top card's edges.
func TestDesign_S113_3_FaviconsAreHintedToWholePixels(t *testing.T) {
	type probe struct {
		x, y int
		part string
	}
	for _, tc := range []struct {
		file   string
		size   int
		probes []probe
	}{
		{"static/favicon-16.png", 16, []probe{
			// Column 8, top to bottom: far edge, gap, near edge, gap, top card,
			// bar, top card.
			{8, 1, ""}, {8, 2, "far"}, {8, 3, "page"}, {8, 4, "near"}, {8, 5, "page"},
			{8, 6, "accent"}, {8, 8, "accent"}, {5, 9, "page"}, {3, 9, "page"}, {7, 9, "page"}, {5, 10, "accent"}, {5, 12, "accent"}, {5, 13, ""},
			// Row 5, right to left: far edge, gap, near edge, gap.
			{15, 5, ""}, {14, 5, "far"}, {13, 5, "page"}, {12, 5, "near"}, {11, 5, "page"},
			// The bar is clear of the card's edges.
			{2, 9, "accent"}, {8, 9, "accent"}, {10, 9, "accent"},
			// Corners are notched: the hinted small radius.
			{1, 6, ""}, {2, 6, "accent"},
		}},
		{"static/favicon-32.png", 32, []probe{
			{15, 5, ""}, {15, 6, "far"}, {15, 7, "far"}, {15, 8, "page"}, {15, 9, "near"}, {15, 10, "near"}, {15, 11, "page"},
			{15, 12, "accent"}, {15, 16, "accent"}, {12, 17, "page"}, {12, 18, "page"}, {12, 19, "page"}, {12, 20, "accent"}, {12, 24, "accent"}, {12, 25, ""},
			{28, 14, ""}, {27, 14, "far"}, {26, 14, "far"}, {25, 14, "page"}, {24, 14, "near"}, {23, 14, "near"}, {22, 14, "page"}, {21, 14, "accent"},
			{8, 17, "accent"}, {7, 18, "accent"}, {8, 18, "page"}, {16, 18, "page"}, {17, 18, "accent"},
			{4, 12, ""}, {5, 12, "accent"},
		}},
	} {
		b, err := staticFS.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if r := img.Bounds(); r.Dx() != tc.size || r.Dy() != tc.size {
			t.Fatalf("%s is %v, want %dx%d", tc.file, r, tc.size, tc.size)
		}
		part := func(x, y int) string {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				return ""
			}
			for name, hex := range s3aLight {
				if c == hexColor(hex) {
					return name
				}
			}
			return "off-palette"
		}
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				if part(x, y) == "off-palette" {
					t.Fatalf("%s pixel (%d,%d) is anti-aliased: %v", tc.file, x, y, img.At(x, y))
				}
			}
		}
		for _, p := range tc.probes {
			if got := part(p.x, p.y); got != p.part {
				t.Errorf("%s pixel (%d,%d) is %q, want %q", tc.file, p.x, p.y, got, p.part)
			}
		}
	}

	// The adaptive SVG favicon is the same hinted 16 px drawing, in both
	// palettes, and /favicon.ico holds the 16 and 32 px images.
	fav, err := staticFS.ReadFile("static/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("brand/favicon-16-light.svg")
	if err != nil {
		t.Fatal(err)
	}
	drawing := func(s string) string { return s[strings.Index(s, "</style>"):] }
	if drawing(string(fav)) != drawing(string(src)) {
		t.Error("static/favicon.svg is not the hinted 16 px drawing")
	}
	for _, w := range []string{
		`viewBox="0 0 16 16"`, `shape-rendering="crispEdges"`,
		`<path class="mark-far" d="M6 2H14V3H15V8H14V9H6V8H5V3H6Z"/>`, `<rect class="mark-face" x="6" y="3" width="8" height="5"/>`,
		`<path class="mark-near" d="M4 4H12V5H13V10H12V11H4V10H3V5H4Z"/>`, `<rect class="mark-face" x="4" y="5" width="8" height="5"/>`,
		`<path class="mark-top" d="M2 6H10V7H11V12H10V13H2V12H1V7H2Z"/>`, `<rect class="mark-bar" x="3" y="9" width="5" height="1"/>`,
		".mark-far{fill:#AAABAD}.mark-near{fill:#6D7073}.mark-face,.mark-bar{fill:#FFFFFF}.mark-top{fill:#0562EF}",
		"@media (prefers-color-scheme:dark){.mark-far{fill:#65646A}.mark-near{fill:#A09FA3}.mark-face,.mark-bar{fill:#12101A}.mark-top{fill:#4D8DFF}}",
	} {
		if !strings.Contains(string(fav), w) {
			t.Errorf("favicon.svg is missing %q", w)
		}
	}
	ico, err := staticFS.ReadFile("static/favicon.ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(ico) < 6+32 || binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 2 || ico[6] != 16 || ico[6+16] != 32 {
		t.Errorf("favicon.ico is not the 16 and 32 px icon (% x)", ico[:min(len(ico), 24)])
	}
}

// The 64 px app icon: the mark at 40 px centred in a continuous-corner
// superellipse; #FFFFFF with a 1 px line edge in light, #12101A in dark. The
// dark icon is the home-screen icon (192, 512), the maskable icon (mark
// inside the central 80% circle) and the apple-touch and social preview
// image.
func TestDesign_S113_3_AppIconsSquircleAndMaskable(t *testing.T) {
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
	mark := []string{
		`<rect class="mark-far" x="24" y="19" width="24" height="17" rx="2.5"/>`,
		`<rect class="mark-near" x="20" y="23" width="24" height="17" rx="2.5"/>`,
		`<rect class="mark-top" x="16" y="27" width="24" height="17" rx="2.5"/>`,
		`<rect class="mark-bar" x="21" y="34" width="11" height="3" rx="1.5"/>`,
	}
	ground := regexp.MustCompile(`<path class="ground" d="M([^"]+)Z" fill="(#[0-9A-F]{6})"( stroke="(#[0-9A-F]{6})" stroke-width="1")?/>`)
	for file, want := range map[string][2]string{"static/app-icon.svg": {"#FFFFFF", "#E2E2E3"}, "static/app-icon-dark.svg": {"#12101A", ""}} {
		s := read(file)
		for _, w := range mark {
			if !strings.Contains(s, w) {
				t.Errorf("%s is missing %q", file, w)
			}
		}
		m := ground.FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("%s has no superellipse ground", file)
		}
		if m[2] != want[0] || m[4] != want[1] {
			t.Errorf("%s ground %s edge %q, want %s edge %q", file, m[2], m[4], want[0], want[1])
		}
		// A superellipse, not a rounded rect: many points, and its diagonal
		// sits between a circle's and a square's.
		pts := strings.Split(m[1], "L")
		if len(pts) < 64 || strings.Contains(m[1], "A") || strings.Contains(m[1], "C") {
			t.Errorf("%s ground is not a sampled superellipse (%d points)", file, len(pts))
		}
	}
	if s := read("brand/icon-maskable.svg"); !strings.Contains(s, `<rect class="ground" width="64" height="64" fill="#12101A"/>`) || !strings.Contains(s, mark[2]) {
		t.Error("the maskable source is not the full-bleed dark icon")
	}

	decode := func(name string, size int) image.Image {
		t.Helper()
		img, err := png.Decode(strings.NewReader(read(name)))
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
		bleed  bool
	}{
		{"static/app-icon-64.png", 64, "#FFFFFF", false},
		{"static/app-icon-64-dark.png", 64, "#12101A", false},
		{"static/icon-192.png", 192, "#12101A", false},
		{"static/icon-512.png", 512, "#12101A", false},
		{"static/icon-180.png", 180, "#12101A", true},
		{"static/icon-maskable-192.png", 192, "#12101A", true},
		{"static/icon-maskable-512.png", 512, "#12101A", true},
	} {
		img := decode(tc.file, tc.size)
		corner := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
		if tc.bleed != (corner.A == 255) {
			t.Errorf("%s corner alpha %d: full bleed %v", tc.file, corner.A, tc.bleed)
		}
		mid := tc.size / 8
		if !near(img.At(tc.size/2, mid), tc.ground) {
			t.Errorf("%s ground is %v, want %s", tc.file, img.At(tc.size/2, mid), tc.ground)
		}
		// Every pixel of the mark lies inside the maskable safe zone: the
		// central circle whose diameter is 80% of the icon.
		safe := 0.4 * float64(tc.size)
		c := float64(tc.size) / 2
		accent := false
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				px := img.At(x, y)
				if near(px, "#4D8DFF") || near(px, "#0562EF") {
					accent = true
				}
				if math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c) <= safe {
					continue
				}
				// A mark pixel is one of the mark's colours (the squircle's own
				// anti-aliased edge is not).
				part := false
				for _, pal := range []map[string]string{s3aLight, s3aDark} {
					for _, k := range []string{"accent", "near", "far"} {
						part = part || near(px, pal[k])
					}
				}
				if part {
					t.Fatalf("%s: mark pixel (%d,%d) %v outside the safe zone", tc.file, x, y, px)
				}
			}
		}
		if !accent {
			t.Errorf("%s has no accent top card", tc.file)
		}
	}
	// The light icon's 1 px line edge runs along the top of the squircle.
	if img := decode("static/app-icon-64.png", 64); !near(img.At(32, 0), "#E2E2E3") || !near(img.At(32, 1), "#FFFFFF") {
		t.Errorf("light app icon edge: %v over %v", img.At(32, 0), img.At(32, 1))
	}
}

// The scrying lens is retired from every surface: no template, static file
// or Go source still draws it, and its old files are gone.
func TestDesign_S113_3_NoLensMarkRemains(t *testing.T) {
	needles := []string{"le" + "ns", "fo" + "cal", "view" + "finder", "aper" + "ture", `r="2` + `.25"`, "M3 8V3" + "h5", "mark-" + "focal"}
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
