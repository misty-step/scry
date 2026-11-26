package web

import (
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// [S113-3] One mark everywhere. The drawing itself (geometry, palette,
// hinting, app icons) is now the L3-S3 mark of [S125-1], pinned in
// s125_1_internal_test.go; these tests keep the lens mark retired and the
// Study icon a line glyph.

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
