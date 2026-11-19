package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// WatchHandler mirrors a content-free error signal to the local incident owner.
// In test mode only loopback delivery is allowed. Sentry remains independently optional.
type WatchHandler struct {
	slog.Handler
	URL, Secret string
}

func Watch(base slog.Handler, endpoint, secret string, test bool) slog.Handler {
	if endpoint == "" || secret == "" {
		return base
	}
	u, e := url.Parse(endpoint)
	if e != nil {
		return base
	}
	if test && (u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1")) {
		return base
	}
	return WatchHandler{base, endpoint, secret}
}
func (h WatchHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		b := []byte(`{"action":"triggered"}`)
		mac := hmac.New(sha256.New, []byte(h.Secret))
		mac.Write(b)
		c := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		for i := 0; i < 3; i++ {
			req, e := http.NewRequestWithContext(ctx, "POST", h.URL, bytes.NewReader(b))
			if e != nil {
				break
			}
			req.Header.Set("Sentry-Hook-Signature", hex.EncodeToString(mac.Sum(nil)))
			resp, e := c.Do(req)
			if e == nil {
				resp.Body.Close()
				if resp.StatusCode < 300 {
					break
				}
			}
		}
	}
	return h.Handler.Handle(ctx, r)
}
func (h WatchHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return WatchHandler{h.Handler.WithAttrs(a), h.URL, h.Secret}
}
func (h WatchHandler) WithGroup(g string) slog.Handler {
	return WatchHandler{h.Handler.WithGroup(g), h.URL, h.Secret}
}
