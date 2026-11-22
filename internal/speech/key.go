package speech

import (
	"errors"
	"fmt"
	"strings"

	"scry/internal/safeerr"
)

// LooksLikeJWT reports whether key is JWT-shaped; see safeerr.LooksLikeJWT.
func LooksLikeJWT(key string) bool { return safeerr.LooksLikeJWT(key) }

// ValidateAPIKey checks a speech key named by its environment variable:
// set, and not JWT-shaped. Its errors name the variable and never contain
// the key. The adapters check again inside Transcribe.
func ValidateAPIKey(name, key string) error { return safeerr.CheckAPIKey(name, key) }

// NewTranscriber chooses the real speech adapter with the same precedence
// as ai.NewJev: OpenRouter whenever OPENROUTER_API_KEY is set, the direct
// OpenAI adapter only when it is not and OPENAI_API_KEY is. A JWT-shaped
// key on the chosen route stops the boot. model (SCRY_SPEECH_MODEL)
// overrides the chosen route's default; on OpenRouter it must be a
// vendor/model slug.
func NewTranscriber(openRouterKey, openAIKey, model string) (Transcriber, error) {
	return NewTranscriberWithRoute(openRouterKey, openAIKey, model, "")
}

// NewTranscriberWithRoute is NewTranscriber with SCRY_SPEECH_ROUTE (route,
// see ParseSpeechRoute): "", "auto", "transcriptions" or "chat". Any other
// value is an error. The direct OpenAI adapter has only its transcriptions
// endpoint, so "chat" without OPENROUTER_API_KEY is an error too.
func NewTranscriberWithRoute(openRouterKey, openAIKey, model, route string) (Transcriber, error) {
	openRouterKey, openAIKey, model = strings.TrimSpace(openRouterKey), strings.TrimSpace(openAIKey), strings.TrimSpace(model)
	route, err := ParseSpeechRoute(route)
	if err != nil {
		return nil, err
	}
	if openRouterKey != "" {
		if err := ValidateAPIKey("OPENROUTER_API_KEY", openRouterKey); err != nil {
			return nil, err
		}
		if model != "" && !safeerr.IsRouterSlug(model) {
			return nil, fmt.Errorf("SCRY_SPEECH_MODEL=%q is not an OpenRouter model slug (vendor/model) and OPENROUTER_API_KEY is set", model)
		}
		return OpenRouter{APIKey: openRouterKey, Model: model, RouteOverride: route}, nil
	}
	if openAIKey != "" {
		if err := ValidateAPIKey("OPENAI_API_KEY", openAIKey); err != nil {
			return nil, err
		}
		if route == RouteForceChat {
			return nil, errors.New("SCRY_SPEECH_ROUTE=chat needs OPENROUTER_API_KEY: the direct OpenAI speech route only has audio/transcriptions")
		}
		return OpenAI{APIKey: openAIKey, Model: model}, nil
	}
	return nil, errors.New("set OPENROUTER_API_KEY or OPENAI_API_KEY for speech")
}
