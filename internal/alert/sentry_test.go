package alert

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
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

// recordingTransport keeps the events the Sentry client would have sent.
type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (r *recordingTransport) Configure(sentry.ClientOptions) {}
func (r *recordingTransport) SendEvent(e *sentry.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}
func (r *recordingTransport) Flush(time.Duration) bool              { return true }
func (r *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (r *recordingTransport) Close()                                {}

// Without a Capture override (the live path), an error-level log line
// reaches Sentry as a level "error" event carrying only the message, so
// Sentry's error-level alert rules fire; info lines are not sent.
func TestSentryLivePathSendsErrorLevel(t *testing.T) {
	tr := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "https://public@sentry.invalid/1", Transport: tr}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) })
	log := slog.New(SentryHandler{Handler: slog.NewTextHandler(io.Discard, nil)})
	log.Info("healthy")
	log.Warn("degraded")
	log.With("secret", "not forwarded").Error("client csp_violation route=/signup/card")
	sentry.Flush(time.Second)
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.events) != 1 {
		t.Fatalf("Sentry got %d events, want 1", len(tr.events))
	}
	e := tr.events[0]
	if e.Level != sentry.LevelError {
		t.Fatalf("level = %q, want %q", e.Level, sentry.LevelError)
	}
	if e.Message != "client csp_violation route=/signup/card" {
		t.Fatalf("message = %q", e.Message)
	}
	if len(e.Contexts["secret"]) != 0 || len(e.Tags) != 0 {
		t.Fatalf("attributes leaked: %v %v", e.Tags, e.Contexts)
	}
}
