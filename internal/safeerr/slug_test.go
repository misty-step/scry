package safeerr

import "testing"

func TestIsRouterSlug(t *testing.T) {
	for m, want := range map[string]bool{
		"anthropic/claude-sonnet-5.5":   true,
		"google/gemini-2.5-flash":       true,
		"openai/gpt-4o:free":            true,
		"vendor/family/model":           true,
		"":                              false,
		"claude-opus-5-5":               false,
		"/":                             false,
		"/model":                        false,
		"vendor/":                       false,
		"/vendor/model":                 false,
		"vendor/model/":                 false,
		"vendor//model":                 false,
		"vendor/ model":                 false,
		" vendor/model":                 false,
		"vendor/model ":                 false,
		"vendor/mo del":                 false,
		"vendor/model\n":                false,
		"vendor/\tmodel":                false,
		"vendor/model ":                 false,
		"vendor/model\x00":              false,
		"anthropic/claude-sonnet-5.5\r": false,
	} {
		if got := IsRouterSlug(m); got != want {
			t.Errorf("IsRouterSlug(%q) = %v, want %v", m, got, want)
		}
	}
}
