package alert

import (
	"context"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

// SentryHandler forwards every error-level event, without credentials, answers,
// request bodies or provider payloads. Local structured logs retain details.
// Events reach Sentry at level "error" so error-level alert rules fire;
// sentry.CaptureMessage alone would record them as "info".
type SentryHandler struct {
	slog.Handler
	Capture func(string)
}

func (h SentryHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		if h.Capture != nil {
			h.Capture(r.Message)
		} else {
			sentry.CaptureEvent(errorEvent(r.Message))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// errorEvent is the Sentry event for one error-level log line: the message
// only, at level error.
func errorEvent(message string) *sentry.Event {
	return &sentry.Event{Message: message, Level: sentry.LevelError}
}

func (h SentryHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return SentryHandler{h.Handler.WithAttrs(a), h.Capture}
}
func (h SentryHandler) WithGroup(g string) slog.Handler {
	return SentryHandler{h.Handler.WithGroup(g), h.Capture}
}
