// Package safeerr removes configured credentials at provider boundaries.
package safeerr

import (
	"errors"
	"net/url"
	"strings"
)

func Redact(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	before := err.Error()
	after := before
	for _, secret := range secrets {
		if secret != "" {
			after = strings.ReplaceAll(after, url.QueryEscape(secret), "[redacted]")
			after = strings.ReplaceAll(after, secret, "[redacted]")
		}
	}
	if after == before {
		return err
	}
	return errors.New(after)
}
