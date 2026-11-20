package alert

import (
	"context"
	"github.com/getsentry/sentry-go"
	"log/slog"
)

// SentryHandler forwards every error-level event, without credentials, answers,
// request bodies or provider payloads. Local structured logs retain details.
type SentryHandler struct {
	slog.Handler
	Capture func(string)
}

func (h SentryHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		if h.Capture != nil {
			h.Capture(r.Message)
		} else {
			sentry.CaptureMessage(r.Message)
		}
	}
	return h.Handler.Handle(ctx, r)
}
func (h SentryHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return SentryHandler{h.Handler.WithAttrs(a), h.Capture}
}
func (h SentryHandler) WithGroup(g string) slog.Handler {
	return SentryHandler{h.Handler.WithGroup(g), h.Capture}
}
