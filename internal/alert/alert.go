// Package alert tells a human when Scry is unwell, without anyone asking.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Notifier delivers an alert to whoever is on call.
type Notifier interface {
	Notify(ctx context.Context, title, detail string)
}

// Log is the test-mode adapter: alerts become structured error logs.
type Log struct{ Logger *slog.Logger }

func (l Log) Notify(ctx context.Context, title, detail string) {
	l.Logger.ErrorContext(ctx, "alert", "alert", true, "title", title, "detail", detail)
}

// Webhook posts alerts as JSON ({"text": ...}) to ALERT_WEBHOOK_URL. The
// shape is what Slack-compatible incoming webhooks accept.
type Webhook struct {
	URL    string
	Logger *slog.Logger
	HTTP   *http.Client
}

func (w Webhook) Notify(ctx context.Context, title, detail string) {
	w.Logger.ErrorContext(ctx, "alert", "alert", true, "title", title, "detail", detail)
	client := w.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, _ := json.Marshal(map[string]string{"text": "Scry: " + title + "\n" + detail})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		w.Logger.ErrorContext(ctx, "alert delivery failed", "err", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		w.Logger.ErrorContext(ctx, "alert delivery failed", "status", resp.StatusCode)
	}
}

// Quiet wraps a notifier so the same alert is sent at most once per
// window: a failing dependency should page once, not once per request.
type Quiet struct {
	Next   Notifier
	Window time.Duration
	Now    func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

func (q *Quiet) Notify(ctx context.Context, title, detail string) {
	now := time.Now()
	if q.Now != nil {
		now = q.Now()
	}
	q.mu.Lock()
	if q.last == nil {
		q.last = map[string]time.Time{}
	}
	prev, seen := q.last[title]
	if seen && now.Sub(prev) < q.Window {
		q.mu.Unlock()
		return
	}
	q.last[title] = now
	q.mu.Unlock()
	q.Next.Notify(ctx, title, detail)
}

// Recorder keeps alerts in memory. Tests read them back.
type Recorder struct {
	mu     sync.Mutex
	Alerts []string
}

func (r *Recorder) Notify(_ context.Context, title, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Alerts = append(r.Alerts, title+": "+detail)
}

// Titles returns the alerts seen so far.
func (r *Recorder) Titles() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.Alerts...)
}
