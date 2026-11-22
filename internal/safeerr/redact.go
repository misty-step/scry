// Package safeerr removes configured credentials at provider boundaries.
package safeerr

import (
	"errors"
	"net/url"
	"strings"
)

// RedactString replaces every configured secret (and its URL-escaped
// form) in s with [redacted]. Call it on provider text before truncating
// it, so a secret straddling the cut can never survive as a fragment.
func RedactString(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, url.QueryEscape(secret), "[redacted]")
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	return s
}

func Redact(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	before := err.Error()
	after := RedactString(before, secrets...)
	if after == before {
		return err
	}
	return errors.New(after)
}
