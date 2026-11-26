package web

import "html/template"

// Scry's brand mark, L3-S3, the Fanned Scry Reveal · Cobalt Tilt (DESIGN.md,
// Brand mark and wordmark; [S113-3], restored exactly in [S125-1]). The
// canonical SVG is binding and verbatim: on a 32 x 32 grid, three solid
// portrait cards, each 16 x 19 with 2.5 px corners, back to front:
//
//   - the back card, upright, at (5, 10);
//   - the middle card, upright, at (7, 7.5);
//   - the top card at (11, 3.5), rotated 10 degrees clockwise around
//     (19, 14), with its prompt bar (9 x 2, rx 1, at 85% opacity) at
//     (14.5, 7) inside the same rotated group, so the bar rides the tilt.
//
// markLayers is that drawing with a class on each layer; the fill attributes
// are the light pair, and the style sheet switches every layer to the dark
// pair in dark mode (the mark's own fixed colours, never the theme tokens).
// scripts/render-brand.py writes the same numbers into every other surface;
// TestDesign_S125_1_CanonicalSVGIsDesignVerbatim reads DESIGN.md's blocks
// and checks both.
var markLayers = []string{
	`<rect class="mark-back" x="5" y="10" width="16" height="19" rx="2.5" fill="#CBD5E1"/>`,
	`<rect class="mark-middle" x="7" y="7.5" width="16" height="19" rx="2.5" fill="#1E293B"/>`,
	`<g transform="rotate(10 19 14)">`,
	`<rect class="mark-top" x="11" y="3.5" width="16" height="19" rx="2.5" fill="#0562EF"/>`,
	`<rect class="mark-bar" x="14.5" y="7" width="9" height="2" rx="1" fill="#FFFFFF" opacity="0.85"/>`,
	`</g>`,
}

// brandMark renders the canonical mark scaled whole to 24 px for the top
// bar, 8 px before "scry". It is decorative: the wordmark link already says
// "scry, home". It never animates and only ever appears beside the wordmark
// (layout.html).
func brandMark() template.HTML {
	s := `<svg class="mark" viewBox="0 0 32 32" width="24" height="24" fill="none" aria-hidden="true" focusable="false">`
	for _, part := range markLayers {
		s += part
	}
	return template.HTML(s + `</svg>`)
}
