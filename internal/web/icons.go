package web

import (
	"html/template"
	"sort"
)

// Scry's one icon family (DESIGN.md, Icons): inline SVG line icons on a
// 24 px grid, stroked with the current text color at width 1.75, never
// filled, hidden from assistive technology because the word beside each
// one says what it means. They are part of the page: no icon font, no
// sprite request, no emoji.
//
// Each entry is the inner markup of one icon. It is fixed, trusted markup
// written here, never built from learner or model text.
var iconPaths = map[string]string{
	// Navigation
	"cards":    `<rect x="3" y="8" width="14" height="11" rx="1.5"/><path d="M7 8V6.5A1.5 1.5 0 0 1 8.5 5h11A1.5 1.5 0 0 1 21 6.5v8a1.5 1.5 0 0 1-1.5 1.5H17"/>`,
	"library":  `<path d="M5 4v16M9.5 4v16"/><path d="M13.6 5.6l3.4-.9 3.6 13.7-3.4.9z"/>`,
	"create":   `<path d="M11 5l1.9 6.1L19 13l-6.1 1.9L11 21l-1.9-6.1L3 13l6.1-1.9z"/><path d="M19 2.5v4M17 4.5h4"/>`,
	"billing":  `<rect x="3" y="5.5" width="18" height="13" rx="1.5"/><path d="M3 10h18M7 14.5h3"/>`,
	"settings": `<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>`,

	// Actions and states
	"plus":     `<path d="M12 5v14M5 12h14"/>`,
	"check":    `<path d="M5 12.5l4.5 4.5L19 7.5"/>`,
	"arrow":    `<path d="M5 12h14M13 6l6 6-6 6"/>`,
	"help":     `<circle cx="12" cy="12" r="9"/><path d="M9.6 9.3a2.5 2.5 0 0 1 4.8.9c0 1.7-2.4 2-2.4 3.8M12 17h.01"/>`,
	"pencil":   `<path d="M4 20l1-4L16.5 4.5a2.1 2.1 0 0 1 3 3L8 19z"/><path d="M14.5 6.5l3 3"/>`,
	"interval": `<path d="M4 6v12M20 6v12M7.5 12h9M10 9.5L7.5 12l2.5 2.5M14 9.5l2.5 2.5-2.5 2.5"/>`,
	"anchor":   `<circle cx="12" cy="5.5" r="2"/><path d="M12 7.5V21M8.5 11h7M4.5 13.5a7.5 7.5 0 0 0 15 0"/>`,
	"gauge":    `<path d="M4.2 17.5a9 9 0 1 1 15.6 0"/><path d="M12 14l4.2-4.2"/><circle cx="12" cy="14" r="1.25"/>`,
	"history":  `<path d="M3.5 12a8.5 8.5 0 1 0 2.5-6"/><path d="M3.5 4v4.5H8"/><path d="M12 7.5V12l3 2"/>`,
	"calendar": `<rect x="4" y="5.5" width="16" height="14.5" rx="1.5"/><path d="M4 10h16M8 3.5v4M16 3.5v4"/>`,
	"spinner":  `<path d="M21 12a9 9 0 1 1-9-9"/>`,
	// Managing a card in Study (S106-5): Snooze, a crescent moon; Delete, a bin.
	"snooze": `<path d="M19.5 14.5A8 8 0 1 1 9.5 4.5a6.5 6.5 0 0 0 10 10z"/>`,
	"bin":    `<path d="M4 7h16M9.5 7V4.5h5V7M6 7l1 13h10l1-13M10 11v5.5M14 11v5.5"/>`,

	// Bridge cards (Slice 75): a bridge (a deck on two piers over an arch)
	// for the struggle cue and bridge cards, a pause in a circle for a held
	// card, a play mark in a circle for releasing it, and a cross to remove
	// a drafted bridge.
	"bridge":  `<path d="M3 8.5h18M5.5 8.5V19M18.5 8.5V19M5.5 19a6.5 6.5 0 0 1 13 0M9 8.5v4M12 8.5v3.5M15 8.5v4"/>`,
	"hold":    `<circle cx="12" cy="12" r="9"/><path d="M10 9v6M14 9v6"/>`,
	"release": `<circle cx="12" cy="12" r="9"/><path d="M10 8.75l5 3.25-5 3.25z"/>`,
	"remove":  `<path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/>`,

	// How Scry works (Slice 106): the landing page's four rows. A clock for
	// the five-minute daily loop, layered books for starter sets, capture
	// and imports, and a lock for privacy, exports and API tokens; the row
	// on judgment uses the Correct verdict's check in a circle.
	"clock": `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/>`,
	"books": `<rect x="3" y="15" width="18" height="5" rx="1"/><path d="M4.5 15v-4a1 1 0 0 1 1-1h12.5a1 1 0 0 1 1 1v4M6 10V6a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1v4M17 15v5M8 10v5M14 5v5"/>`,
	"lock":  `<rect x="4.5" y="10.5" width="15" height="10" rx="1.5"/><path d="M8 10.5V7.5a4 4 0 0 1 8 0v3M12 14.5V17"/>`,

	// Verdicts
	"verdict-right":  `<circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.8 2.8L16 9.5"/>`,
	"verdict-close":  `<circle cx="12" cy="12" r="9"/><path d="M8 12.5c1.3-1.6 2.7-1.6 4 0s2.7 1.6 4 0"/>`,
	"verdict-missed": `<circle cx="12" cy="12" r="9"/><path d="M9 9l6 6M15 9l-6 6"/>`,

	// Starter sets, by subject (starter.Icons)
	"radio":     `<path d="M12 11.5V21M9 21h6"/><circle cx="12" cy="10" r="1.5"/><path d="M8.2 13.8a5.4 5.4 0 0 1 0-7.6M15.8 6.2a5.4 5.4 0 0 1 0 7.6M5.4 16.6a9.4 9.4 0 0 1 0-13.2M18.6 3.4a9.4 9.4 0 0 1 0 13.2"/>`,
	"layers":    `<path d="M12 3l9 4.5-9 4.5-9-4.5z"/><path d="M3 12l9 4.5 9-4.5M3 16.5L12 21l9-4.5"/>`,
	"ruler":     `<rect x="2.5" y="8" width="19" height="8" rx="1"/><path d="M6.5 8v3M10.5 8v4.5M14.5 8v3M18.5 8v4.5"/>`,
	"globe":     `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a13.5 13.5 0 0 1 0 18a13.5 13.5 0 0 1 0-18z"/>`,
	"eye":       `<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.75"/>`,
	"orbit":     `<circle cx="12" cy="12" r="4"/><ellipse cx="12" cy="12" rx="10" ry="3.6" transform="rotate(-20 12 12)"/>`,
	"pulse":     `<path d="M3 12h4l2.5-6 4.5 12 2.5-6H21"/>`,
	"quote":     `<path d="M8 13H4V8h4v6a3 3 0 0 1-3 3M18 13h-4V8h4v6a3 3 0 0 1-3 3"/>`,
	"banknote":  `<rect x="2.5" y="6.5" width="19" height="11" rx="1.5"/><circle cx="12" cy="12" r="2.5"/><path d="M6 12h.01M18 12h.01"/>`,
	"branch":    `<circle cx="6" cy="5.5" r="2"/><circle cx="6" cy="18.5" r="2"/><circle cx="18" cy="8" r="2"/><path d="M6 7.5v9M18 10c0 3.5-2.5 5-6 5H6"/>`,
	"hourglass": `<path d="M6.5 3h11M6.5 21h11M7.5 3c0 4.5 4.5 5.5 4.5 9s-4.5 4.5-4.5 9M16.5 3c0 4.5-4.5 5.5-4.5 9s4.5 4.5 4.5 9"/>`,
	"atom":      `<circle cx="12" cy="12" r="1.25"/><ellipse cx="12" cy="12" rx="9.5" ry="3.8" transform="rotate(60 12 12)"/><ellipse cx="12" cy="12" rx="9.5" ry="3.8" transform="rotate(-60 12 12)"/>`,
	"leaf":      `<path d="M5 19c0-8 5-13.5 15-14 .5 10-5 15-13 14.6"/><path d="M5 19c3-5 6.5-8.2 11-10"/>`,
	"beaker":    `<path d="M9 3h6M10 3v6.2L4.9 18a2 2 0 0 0 1.7 3h10.8a2 2 0 0 0 1.7-3L14 9.2V3M7.5 15h9"/>`,
	"mountain":  `<path d="M2.5 19.5L9 7l4 7.2 2.2-3.4 6.3 8.7z"/>`,
	"sigma":     `<path d="M17.5 6.5V5h-11l6 7-6 7h11v-1.5"/>`,
	"bars":      `<path d="M5 20v-9M12 20V4M19 20v-6M3 20h18"/>`,
	"chip":      `<rect x="6" y="6" width="12" height="12" rx="1.5"/><rect x="9.75" y="9.75" width="4.5" height="4.5"/><path d="M9.5 3v3M14.5 3v3M9.5 18v3M14.5 18v3M3 9.5h3M3 14.5h3M18 9.5h3M18 14.5h3"/>`,
	"trend":     `<path d="M3 17l6-6 4 4 8-8M15.5 7H21v5.5"/>`,
	"bulb":      `<path d="M9 18h6M10 21h4M8.2 14.5a6.5 6.5 0 1 1 7.6 0c-.6.5-.8 1.2-.8 2H9c0-.8-.2-1.5-.8-2z"/>`,
	"column":    `<path d="M3.5 9.5L12 4l8.5 5.5zM7 10.5V17M12 10.5V17M17 10.5V17M4 20h16"/>`,
	"pen":       `<path d="M12 20l-7-7 6.5-9.5h1L19 13zM12 20v-7"/><circle cx="12" cy="11.5" r="1.25"/>`,
	"sprout":    `<path d="M12 21v-9M12 12c0-3.5-2.5-6-7-6 0 3.5 2.5 6 7 6zM12 15c0-3 2.2-5 7-5 0 3-2.2 5-7 5z"/>`,
	"flag":      `<path d="M5 21V4M5 4.5h12.5L15 8.75l2.5 4.25H5"/>`,
	"book":      `<path d="M12 6.5C10 5 7 4.5 3 4.5V18c4 0 7 .5 9 2 2-1.5 5-2 9-2V4.5c-4 0-7 .5-9 2zM12 6.5V20"/>`,
	"image":     `<rect x="3" y="4.5" width="18" height="15" rx="1.5"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="M21 15.5l-5-5-9 9"/>`,
	"note":      `<path d="M9 18V5.5l10-2V16"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="16.5" cy="16" r="2.5"/>`,
	"flame":     `<path d="M12 3c.5 3.5 5 5.5 5 11a5 5 0 0 1-10 0c0-2 .8-3.4 2-4.5.2 1.5.9 2.5 1.8 3C10.5 9.5 10.5 6 12 3z"/>`,
	"scroll":    `<path d="M6.5 17V5.5A1.5 1.5 0 0 1 8 4h10.5A1.5 1.5 0 0 1 20 5.5V7h-4M16 5.5v13a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 4 18.5V17h9.5v1.5a1.5 1.5 0 0 0 1.5 1.5M9.5 9H13M9.5 12.5H13"/>`,
	"bookmark":  `<path d="M6.5 4h11v16.5l-5.5-4-5.5 4z"/>`,
}

// iconNames lists every icon, sorted, for tests and the style sheet check.
func iconNames() []string {
	names := make([]string, 0, len(iconPaths))
	for name := range iconPaths {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// icon renders one icon inline. An unknown name renders nothing, so a
// missing icon can never break a page; the tests check every name used.
func icon(name string) template.HTML {
	inner, ok := iconPaths[name]
	if !ok {
		return ""
	}
	return template.HTML(`<svg class="icon icon-` + name + `" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">` + inner + `</svg>`)
}
