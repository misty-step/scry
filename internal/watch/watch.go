// Package watch owns monitoring state independently of the application database.
package watch

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Incident struct {
	ID                                                  string
	Detected, Alerted, Recovered, FixProposed, Resolved time.Time
	LastSignal                                          time.Time
	Signal, Brief, FixStatus                            string
	Evidence                                            []string
	Count                                               int
	RecoverySeconds                                     float64
}
type State struct{ Incidents []Incident }
type Config struct {
	File, HealthURL, Secret, RestartCmd, AgentCmd, Commit, LogFile string
	Quiet, Retry                                                   time.Duration
	Threshold                                                      int
	Notify                                                         func(context.Context, string) error
}
type Watch struct {
	mu sync.Mutex
	Config
	State       State
	HTTP        *http.Client
	streak      int
	healthy     bool
	lastRestart time.Time
}

// Status is the last probe's result and the current failure streak.
func (w *Watch) Status() (healthy bool, streak int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.healthy, w.streak
}

func Open(c Config) (*Watch, error) {
	if c.Threshold < 1 {
		c.Threshold = 3
	}
	if c.Quiet <= 0 {
		c.Quiet = time.Minute
	}
	if c.Retry <= 0 {
		c.Retry = time.Minute
	}
	w := &Watch{Config: c, HTTP: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	b, e := os.ReadFile(c.File)
	if e == nil {
		e = json.Unmarshal(b, &w.State)
	}
	if os.IsNotExist(e) {
		e = nil
	}
	return w, e
}
func (w *Watch) save() error {
	if e := os.MkdirAll(filepath.Dir(w.File), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(w.State, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(w.File+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(w.File+".tmp", w.File); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(w.File))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (w *Watch) active() *Incident {
	for i := len(w.State.Incidents) - 1; i >= 0; i-- {
		if w.State.Incidents[i].Resolved.IsZero() {
			return &w.State.Incidents[i]
		}
	}
	return nil
}

// Logs use an allowlist of structural fields only. Messages, attributes and bodies
// are never copied, even when they look harmless.
func Logs(path string) []string {
	f, e := os.Open(path)
	if e != nil {
		return nil
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil
	}
	off := st.Size() - 16384
	if off < 0 {
		off = 0
	}
	f.Seek(off, 0)
	b, _ := io.ReadAll(io.LimitReader(f, 16384))
	var lines []string
	for _, line := range bytes.Split(b, []byte("\n")) {
		var v struct {
			Level string `json:"level"`
		}
		if json.Unmarshal(line, &v) != nil {
			continue
		}
		switch v.Level {
		case "INFO", "WARN", "ERROR", "DEBUG":
			lines = append(lines, "application log level="+v.Level+" (content omitted)")
		}
	}
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return lines
}
func commit(s string) string {
	if len(s) < 7 || len(s) > 64 {
		return "unknown"
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "unknown"
		}
	}
	return s
}
func command(ctx context.Context, cmd, input, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
	c.Stdin = bytes.NewBufferString(input)
	c.Env = append(os.Environ(), "SCRY_INCIDENT_ID="+id)
	// Never persist stdout/stderr: agents and restart tools can print secrets.
	return c.Run()
}
func (w *Watch) signal(ctx context.Context, now time.Time, kind, evidence string) error {
	in := w.active()
	if in == nil {
		w.State.Incidents = append(w.State.Incidents, Incident{ID: fmt.Sprintf("%d", now.UnixNano()), Detected: now, Signal: kind})
		in = w.active()
	}
	if kind == "health" {
		in.Recovered = time.Time{}
		in.RecoverySeconds = 0
	}
	in.LastSignal = now
	in.Count++
	if len(in.Evidence) == 0 || in.Evidence[len(in.Evidence)-1] != evidence {
		in.Evidence = append(in.Evidence, evidence)
		if len(in.Evidence) > 20 {
			in.Evidence = in.Evidence[1:]
		}
	}
	in.Brief = fmt.Sprintf("Scry incident %s\nDetected: %s\nSignal: %s\nDeployed commit: %s\nEvidence: %v\nRecent sanitized logs: %v\nTask: investigate this repository and open a fix pull request. Do not deploy. Deduplicate using SCRY_INCIDENT_ID.\n", in.ID, in.Detected.Format(time.RFC3339), in.Signal, commit(w.Commit), in.Evidence, Logs(w.LogFile))
	if w.AgentCmd == "" {
		in.FixStatus = "fix proposal not configured"
	}
	if e := w.save(); e != nil {
		return e
	}
	return nil
}
func (w *Watch) handoff(ctx context.Context, in *Incident, now time.Time) error {
	if in.Alerted.IsZero() && w.Notify != nil {
		if e := w.Notify(ctx, in.Brief+"\n"+in.FixStatus); e == nil {
			in.Alerted = now
		}
	}
	if in.FixProposed.IsZero() && w.AgentCmd != "" {
		if e := command(ctx, w.AgentCmd, in.Brief, in.ID); e == nil {
			in.FixProposed = now
			in.FixStatus = "fix command accepted proposal responsibility"
		} else {
			in.FixStatus = "fix command failed; retry pending"
		}
	}
	return w.save()
}
func (w *Watch) Tick(ctx context.Context, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	req, e := http.NewRequestWithContext(ctx, "GET", w.HealthURL, nil)
	healthy := false
	if e == nil {
		r, err := w.HTTP.Do(req)
		if err == nil {
			healthy = r.StatusCode == 200
			r.Body.Close()
		}
	}
	// A probe cut short because the watcher itself is stopping says
	// nothing about Scry: never count it toward an incident.
	if !healthy && ctx.Err() != nil {
		return nil
	}
	w.healthy = healthy
	if !healthy {
		w.streak++
		if w.streak >= w.Threshold {
			if e := w.signal(ctx, now, "health", "GET /healthz failed (response body omitted)"); e != nil {
				return e
			}
			if e := w.handoff(ctx, w.active(), now); e != nil {
				return e
			}
			if w.RestartCmd != "" && (w.lastRestart.IsZero() || now.Sub(w.lastRestart) >= w.Retry) {
				w.lastRestart = now
				if e := command(ctx, w.RestartCmd, "", w.active().ID); e != nil {
					w.active().Evidence = append(w.active().Evidence, "restart command failed")
				}
			}
		}
	} else {
		w.streak = 0
		if in := w.active(); in != nil {
			if in.Recovered.IsZero() {
				in.Recovered = now
				in.RecoverySeconds = now.Sub(in.Detected).Seconds()
			}
			if e := w.handoff(ctx, in, now); e != nil {
				return e
			}
			if now.Sub(in.LastSignal) >= w.Quiet {
				in.Resolved = now
			}
		}
	}
	return w.save()
}

// Signed Sentry-style webhook: authenticate the exact raw body; never trust or
// retain event text. One open incident groups concurrent symptoms of an outage.
func (w *Watch) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if w.Secret == "" {
		http.NotFound(rw, r)
		return
	}
	if r.Method != "POST" {
		rw.WriteHeader(405)
		return
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 65537))
	if e != nil || len(b) > 65536 {
		rw.WriteHeader(413)
		return
	}
	mac := hmac.New(sha256.New, []byte(w.Secret))
	mac.Write(b)
	sig, e := hex.DecodeString(r.Header.Get("Sentry-Hook-Signature"))
	if e != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		rw.WriteHeader(401)
		return
	}
	var v struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(b, &v) != nil {
		rw.WriteHeader(400)
		return
	}
	if v.Action != "triggered" {
		rw.WriteHeader(204)
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if e = w.signal(r.Context(), time.Now(), "error", "authenticated error alert (payload omitted)"); e != nil {
		rw.WriteHeader(503)
		return
	}
	rw.WriteHeader(204)
}
