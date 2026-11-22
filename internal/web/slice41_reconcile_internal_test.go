package web

import (
	"strings"
	"testing"

	"scry/internal/store"
)

// Reconciling 92350ed3: "never an idea title or any label that holds a word
// of the answer". One-letter answers, number words and answers made only of
// common words counted as nothing and were printed in the rail.
func TestSlice41Reconcile_Spoiler_ShortAndCommonAnswersAreWordsToo(t *testing.T) {
	for _, c := range []struct {
		topic, answer, want string
	}{
		{"Vitamin C", "C", "Current set"},
		{"Programming in C", "C++", "Current set"},
		{"Vitamin A", "A", "Current set"},
		{"One of a kind", "One", "Current set"},
		{"The two-party system", "Two houses", "Current set"},
		{"What it is", "It", "Current set"},
		{"No man is an island", "No", "Current set"},
		{"Newton's laws", "Newton", "Current set"},
		// A stray letter after an apostrophe is not a word, and common words
		// inside a longer answer still give nothing away.
		{"Newton's laws", "John's gospel", "Newton's laws"},
		{"The art of the deal", "A bird in the hand", "The art of the deal"},
		{"Vitamin C", "Citrus", "Vitamin C"},
	} {
		if got := spoilerSafeHeader(store.IdeaPlace{Title: c.topic, Topic: c.topic}, []string{c.answer}); got != c.want {
			t.Errorf("topic %q, answer %q: header = %q, want %q", c.topic, c.answer, got, c.want)
		}
	}
	// A nested idea keeps its neutral set number, and a sibling's answer counts.
	place := store.IdeaPlace{Title: "Scurvy", Topic: "Vitamin C", Nested: true, Ordinal: 1}
	if got := spoilerSafeHeader(place, []string{"Citrus", "C"}); got != "Set 1" {
		t.Errorf("a sibling's one-letter answer: header = %q, want %q", got, "Set 1")
	}
}

// DESIGN.md, Icons and Color: the three icon sizes, and crisp hover (a
// fill or border change in 150 ms; a bordered button's hairline turns ink).
func TestSlice41Reconcile_Design_IconSizesAndCrispControls(t *testing.T) {
	raw, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for _, want := range []string{
		".list-purpose { order: 1; flex-basis: 100%; max-width: none; }", // its own line under the Inbox title
		".tile-add .icon { width: 18px; height: 18px; }",
		".fact dt .icon { width: 16px; height: 16px; }",
		".page-title .icon { width: 16px; height: 16px;",
		".item-count .icon { width: 16px; height: 16px;",
		".tile-add:hover { border-color: var(--ink);",
		"color: var(--on-accent); transition: border-color var(--fast) var(--ease); }",
		".btn-accent:hover { background: var(--accent); border-color: var(--ink); }",
		".mic:active { transform: translateY(1px); }",
		".btn-approve:active { transform: translateY(1px); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css is missing %q", want)
		}
	}
	// The held-key guard itself is proved in a browser (verify-slice41.mjs).
	js, _ := staticFS.ReadFile("static/app.js")
	if !strings.Contains(string(js), `if (!e.repeat || (e.key !== "Enter" && e.key !== " ") || !$("[data-session]")) return;`) {
		t.Error("app.js lets a held Enter or Space run through the study session")
	}
}
