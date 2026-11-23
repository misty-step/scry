package web

import (
	"strings"
	"testing"
)

// The brand mark is one drawing: the inline top-bar mark and the static app
// icons share its geometry (brackets, lens ring, focal point r 2.25), on the
// icon family's stroke (1.75, round caps and joins). The dark-ground icon is
// #12101A with a white aperture and a #4D8DFF focal point.
func TestBrand_MarkIsOneDrawingEverywhere(t *testing.T) {
	inline := string(brandMark())
	for _, want := range []string{markBrackets, markRing, markFocal, `stroke-width="1.75"`, `stroke-linecap="round"`, `stroke-linejoin="round"`, `width="20" height="20"`} {
		if !strings.Contains(inline, want) {
			t.Errorf("inline mark is missing %q", want)
		}
	}
	for file, wants := range map[string][]string{
		"static/icon.svg":      {`stroke:#0B1015`, `fill:#0562EF`, `prefers-color-scheme:dark`, `stroke:#FFFFFF`, `fill:#4D8DFF`},
		"static/icon-dark.svg": {`fill="#12101A"`, `stroke="#FFFFFF"`, `fill="#4D8DFF"`},
	} {
		b, err := staticFS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range append(wants, markBrackets, markRing, `r="2.25"`, `stroke-width="1.75"`, `stroke-linecap="round"`, `stroke-linejoin="round"`) {
			if !strings.Contains(s, want) {
				t.Errorf("%s is missing %q", file, want)
			}
		}
	}
	for _, png := range []string{"static/icon-180.png", "static/icon-192.png", "static/icon-512.png"} {
		b, err := staticFS.ReadFile(png)
		if err != nil || !strings.HasPrefix(string(b), "\x89PNG") {
			t.Errorf("%s is not a PNG", png)
		}
	}
}
