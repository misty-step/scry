package safeerr

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// LooksLikeJWT reports whether key has the shape of a JSON Web Token: three
// dot-separated base64url segments whose first starts "eyJ" (the encoding
// of `{"`) or decodes to a JSON object. Provider API keys never look like
// this; a JWT pasted in their place (a session or OAuth token) fails every
// call, so it is refused loudly instead.
func LooksLikeJWT(key string) bool {
	parts := strings.Split(strings.TrimSpace(key), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, p := range parts {
		for _, r := range strings.TrimRight(p, "=") {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
	}
	if strings.HasPrefix(parts[0], "eyJ") {
		return true
	}
	head, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[0], "="))
	var obj map[string]any
	return err == nil && json.Unmarshal(head, &obj) == nil
}

// CheckAPIKey checks a provider key named by its environment variable: it
// must be set and must not be JWT-shaped. Its errors name the variable and
// never contain the key.
func CheckAPIKey(name, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("%s is missing", name)
	}
	if LooksLikeJWT(key) {
		return fmt.Errorf("%s is JWT-shaped (three dot-separated base64url segments), not a provider API key: every call would fail; set the API key from the provider's dashboard", name)
	}
	return nil
}
