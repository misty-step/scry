package web

import "html/template"

// Scry's brand mark, S3-A (DESIGN.md, Brand mark and wordmark; [S113-3]): a
// small deck of three identical 14 x 10 cards with 1.5 px corners on a 24 px
// grid, each 2.5 px up and right of the one in front. Back to front:
//
//   - the farther card at (7.5, 4.5), a 1.5 px outline in ink at 35%;
//   - the nearer card at (5, 7), a 1.5 px outline in ink-muted;
//   - the top card at (2.5, 9.5), solid accent, with a 2 px prompt bar with
//     round caps knocked out in page, from x 5.5 to 12, centred at y 14.5.
//
// Each receding card is filled with page so it hides the one behind it. Its
// outline is drawn inside the card (an ink-step card with a page face inset
// by the 1.5 px stroke), so all three cards share one 14 x 10 outline and
// every edge lands on a whole device pixel at 2x and above.
//
// markCards is the one 24 px drawing: the inline header mark renders it and
// scripts/render-brand.py writes the same rectangles into the raster
// sources (TestDesign_S113_3_MarkGeometryIsOneDrawing checks both). The
// fills are the light palette as a fallback; the style sheet sets each part
// from the page's own tokens, so the mark follows the theme.
var markCards = []string{
	`<rect class="mark-far" x="7.5" y="4.5" width="14" height="10" rx="1.5" fill="#AAABAD"/>`,
	`<rect class="mark-face" x="9" y="6" width="11" height="7" fill="#FFFFFF"/>`,
	`<rect class="mark-near" x="5" y="7" width="14" height="10" rx="1.5" fill="#6D7073"/>`,
	`<rect class="mark-face" x="6.5" y="8.5" width="11" height="7" fill="#FFFFFF"/>`,
	`<rect class="mark-top" x="2.5" y="9.5" width="14" height="10" rx="1.5" fill="#0562EF"/>`,
	`<rect class="mark-bar" x="5.5" y="13.5" width="6.5" height="2" rx="1" fill="#FFFFFF"/>`,
}

// markHinted is the same deck hinted for a 1x screen, where the 2.5 px step
// and 1.5 px outline would fall between pixels and blur: 14 x 10 cards
// stepped 3 px, 2 px outlines (1 px apart), corners notched by one pixel and
// a 2 px bar from x 5 to 12 centred on the top card. The style sheet shows it
// only at 1 dppx (and the exact drawing everywhere else), like the hinted 16
// and 32 px favicons.
var markHinted = []string{
	`<path class="mark-far" d="M9 4H21V5H22V13H21V14H9V13H8V5H9Z" fill="#AAABAD"/>`,
	`<rect class="mark-face" x="10" y="6" width="10" height="6" fill="#FFFFFF"/>`,
	`<path class="mark-near" d="M6 7H18V8H19V16H18V17H6V16H5V8H6Z" fill="#6D7073"/>`,
	`<rect class="mark-face" x="7" y="9" width="10" height="6" fill="#FFFFFF"/>`,
	`<path class="mark-top" d="M3 10H15V11H16V19H15V20H3V19H2V11H3Z" fill="#0562EF"/>`,
	`<rect class="mark-bar" x="5" y="14" width="7" height="2" fill="#FFFFFF"/>`,
}

// brandMark renders the mark at 24 px for the top bar, 8 px before "scry".
// It is decorative: the wordmark link already says "scry, home". It never
// animates and only ever appears beside the wordmark (layout.html).
func brandMark() template.HTML {
	s := `<svg class="mark" viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false"><g class="mark-exact">`
	for _, part := range markCards {
		s += part
	}
	s += `</g><g class="mark-hinted" display="none" shape-rendering="crispEdges">`
	for _, part := range markHinted {
		s += part
	}
	return template.HTML(s + `</g></svg>`)
}
