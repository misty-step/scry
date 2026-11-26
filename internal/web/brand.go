package web

import (
	_ "embed"
	"html/template"
	"strings"
)

// Scry's brand mark, L3-S3 · Fanned Scry Reveal · Cobalt Tilt (DESIGN.md,
// Brand mark and wordmark; [S113-3], restored exactly in [S125-1]): three
// solid portrait cards (16 x 19, rx 2.5) on a 32 x 32 grid, the back and
// middle cards upright and the cobalt top card rotated 10° clockwise around
// (19, 14) with its prompt bar.
//
// The drawing is never written here. brand/mark-light.svg and
// brand/mark-dark.svg are byte-for-byte copies of DESIGN.md's two canonical
// SVG blocks (scripts/render-brand.py copies them; TestDesign_S125_1_* checks
// the copies against DESIGN.md), and every surface derives from them: this
// inline header mark, the SVG favicon, the hinted PNG favicons and the app
// icons.
var (
	//go:embed brand/mark-light.svg
	markLightSVG string
	//go:embed brand/mark-dark.svg
	markDarkSVG string
)

// markDrawing is the drawing inside a canonical <svg>: its elements exactly
// as written, with only the indentation and line breaks between tags dropped.
func markDrawing(svg string) string {
	body := svg[strings.Index(svg, ">")+1 : strings.LastIndex(svg, "</svg>")]
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(strings.TrimSpace(line))
	}
	return b.String()
}

// brandMarkHTML is computed once: the canonical 32 x 32 drawing scaled whole
// to 24 px. It carries both fill sets; the style sheet shows the light or
// dark drawing by the active mode (the fills are the mark's own fixed brand
// colors, never theme tokens).
var brandMarkHTML = template.HTML(`<svg class="mark" viewBox="0 0 32 32" width="24" height="24" fill="none" aria-hidden="true" focusable="false">` +
	`<g class="mark-light">` + markDrawing(markLightSVG) + `</g>` +
	`<g class="mark-dark">` + markDrawing(markDarkSVG) + `</g></svg>`)

// brandMark renders the mark at 24 px for the top bar, 8 px before "scry".
// It is decorative: the wordmark link already says "scry, home". It never
// animates and only ever appears beside the wordmark (layout.html).
func brandMark() template.HTML { return brandMarkHTML }
