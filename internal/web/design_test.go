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

func TestG12_SpeechUploadsOnlyAtStop(t *testing.T) {
	b, _ := staticFS.ReadFile("static/app.js")
	s := string(b)
	if strings.Contains(s, "hear(false)") || strings.Contains(s, "X-Audio-Seconds") {
		t.Fatal("repeated transcription or untrusted duration")
	}
	if !strings.Contains(s, "audio/wav") {
		t.Fatal("must send sample-counted PCM")
	}
}

func TestG14_InlineLinksKeepTouchTargets(t *testing.T) {
	b, _ := staticFS.ReadFile("static/app.css")
	if strings.Contains(string(b), "p .link, .meta .link { display: inline; min-height: 0; }") {
		t.Fatal("inline links override 44px minimum")
	}
}

func TestVerifierDesignRegressions(t *testing.T) {
	for file, wants := range map[string][]string{
		"static/app.js":        {"Didn't catch that. Try again or type it.", "Allow mic in site settings.", "Allow mic, then retry."},
		"static/app.css":       {"animation: strike 200ms", ".wordmark, .nav a, [data-again]"},
		"templates/draft.html": {"Not enough allowance"},
	} {
		var b []byte
		if strings.HasPrefix(file, "static/") {
			b, _ = staticFS.ReadFile(file)
		} else {
			b, _ = templateFS.ReadFile(file)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s missing %s", file, want)
			}
		}
	}
}

func TestD8_ReadingMeasure(t *testing.T) {
	b, _ := staticFS.ReadFile("static/app.css")
	for _, rule := range []string{"--measure: 68ch;", "p { max-width: var(--measure); }", ".lede { font-size: var(--reading); max-width: var(--measure); }"} {
		if !strings.Contains(string(b), rule) {
			t.Errorf("reading measure missing %s", rule)
		}
	}
}

func TestD11_KeyHints(t *testing.T) {
	b, _ := templateFS.ReadFile("templates/layout.html")
	for _, hint := range []string{"g s study", "g l library", "g n create", "g b billing", "/ search", "esc leave field", "tab/enter other actions"} {
		if !strings.Contains(string(b), hint) {
			t.Errorf("missing key hint %s", hint)
		}
	}
	// S106-4: there is no Inbox, so no g i.
	if strings.Contains(string(b), "g i inbox") || strings.Contains(string(b), "inbox") {
		t.Error("the key hints still offer g i inbox")
	}
	b, _ = templateFS.ReadFile("templates/study.html")
	for _, hint := range []string{"a again", "enter or space for", "enter to check", "space to start/stop speaking"} {
		if !strings.Contains(string(b), hint) {
			t.Errorf("missing study hint %s", hint)
		}
	}
}
