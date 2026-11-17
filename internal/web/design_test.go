package web

import (
	"strings"
	"testing"
)

func TestDesign_TouchTargetsAndFocusedInbox(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, want := range []string{
		"align-items: center; min-height: var(--touch);",
		"a.row-main { min-height: var(--touch); }",
		".sug:focus, .sug:focus-within { background: var(--raised); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing design rule %q", want)
		}
	}
}
func TestDesign_ScrollbackIsOneLineAndMicroIsOnlyForKeys(t *testing.T) {
	b, _ := templateFS.ReadFile("templates/study.html")
	if !strings.Contains(string(b), "answered-why sr") {
		t.Fatal("scrollback reasons must not create extra visible lines")
	}
	b, _ = templateFS.ReadFile("templates/layout.html")
	if !strings.Contains(string(b), `class="meta">test mode`) {
		t.Fatal("non-keyboard text must use meta, not micro")
	}
}
