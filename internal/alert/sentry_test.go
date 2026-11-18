package alert

import (
	"io"
	"log/slog"
	"testing"
)

func TestSentryErrors(t *testing.T) {
	var messages []string
	log := slog.New(SentryHandler{Handler: slog.NewTextHandler(io.Discard, nil), Capture: func(s string) { messages = append(messages, s) }})
	log.Info("healthy")
	log.With("secret", "not forwarded").Error("request failed")
	if len(messages) != 1 || messages[0] != "request failed" {
		t.Fatal(messages)
	}
}
