package web

import "html/template"

// Scry's brand mark (DESIGN.md, Brand mark and wordmark): a geometric
// scrying lens drawn on the icon family's 24 px stroke grid (stroke width
// 1.75, round caps and joins). Four crisp corner viewfinder brackets frame a
// centered lens ring in ink, with one solid focal point (r 2.25) in accent
// at the exact center.
//
// markGeometry is the one source of the drawing: the inline top-bar mark
// renders it, and the static app icons (static/icon.svg, static/icon-dark.svg
// and the PNGs rendered from icon-dark.svg) carry the same paths, which
// TestBrand_MarkIsOneDrawingEverywhere checks.
const (
	markBrackets = `M3 8V3h5M16 3h5v5M21 16v5h-5M8 21H3v-5`
	markRing     = `<circle cx="12" cy="12" r="5.25"/>`
	markFocal    = `<circle class="mark-focal" cx="12" cy="12" r="2.25"/>`
)

// brandMark renders the mark at 20 px for the top bar beside the wordmark.
// It is decorative: the wordmark link already says "scry, home".
func brandMark() template.HTML {
	return template.HTML(`<svg class="mark" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">` +
		`<path d="` + markBrackets + `"/>` + markRing + markFocal + `</svg>`)
}
