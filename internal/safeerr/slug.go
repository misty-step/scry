package safeerr

import (
	"strings"
	"unicode"
)

// IsRouterSlug reports whether m is an OpenRouter model slug: "vendor/model"
// (more path segments are allowed, as in "vendor/family/model"), with no
// whitespace or control characters anywhere, no leading or trailing slash
// and no empty segment. It is the one check every OpenRouter model override
// (SCRY_DRAFT_MODEL, SCRY_EXPLAIN_MODEL, SCRY_SPEECH_MODEL) goes through, so
// a direct provider's model name left in the environment fails the boot
// instead of failing every call.
func IsRouterSlug(m string) bool {
	if strings.IndexFunc(m, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	parts := strings.Split(m, "/")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}
