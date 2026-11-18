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
	if strings.Contains(string(b), "answered-why sr") {
		t.Fatal("right-answer reasons must remain visible")
	}
	b, _ = templateFS.ReadFile("templates/layout.html")
	if !strings.Contains(string(b), `class="meta">test mode`) {
		t.Fatal("non-keyboard text must use meta, not micro")
	}
}

func TestG6_CheckUsesAccentForeground(t *testing.T) {
	b, _ := templateFS.ReadFile("templates/study.html")
	if strings.Contains(string(b), "btn-accent check") {
		t.Fatal("checkbox label color overrides accent contrast")
	}
	css, _ := staticFS.ReadFile("static/app.css")
	for _, token := range []string{"--on-accent: #FFFFFF;", "--on-accent: #12101A;", "color: var(--on-accent);"} {
		if !strings.Contains(string(css), token) {
			t.Fatalf("missing accent contrast token: %s", token)
		}
	}
}

func TestG7_MissedStrikeFollowsText(t *testing.T) {
	b, _ := templateFS.ReadFile("templates/study.html")
	if strings.Contains(string(b), `class="struck-line"`) {
		t.Fatal("overlay strike cannot follow wrapped text")
	}
	css, _ := staticFS.ReadFile("static/app.css")
	if !strings.Contains(string(css), ".struck-text { text-decoration: line-through;") {
		t.Fatal("missing native multiline strike")
	}
}
