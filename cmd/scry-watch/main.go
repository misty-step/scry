// scry-watch is the single host-side incident and restart owner.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"scry/internal/watch"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func duration(k, d string) time.Duration {
	v, e := time.ParseDuration(env(k, d))
	if e != nil || v <= 0 {
		panic("invalid duration: " + k)
	}
	return v
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "watcher failed:", e)
		os.Exit(1)
	}
}
func run() error {
	file := env("SCRY_WATCH_STATE", "data/watch/incidents.json")
	if len(os.Args) > 1 && os.Args[1] == "incidents" {
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		fmt.Println(string(b))
		return nil
	}
	// flock prevents two restart owners sharing state, including across atomic renames.
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(file+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("watcher already owns state")
	}
	health := env("SCRY_WATCH_HEALTH_URL", "http://127.0.0.1:8080/healthz")
	test := env("SCRY_MODE", "live") == "test"
	u, e := url.Parse(health)
	if e != nil {
		return fmt.Errorf("invalid health URL")
	}
	if test && (u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1")) {
		return fmt.Errorf("test health must be loopback")
	}
	notify := func(ctx context.Context, s string) error {
		endpoint := os.Getenv("ALERT_WEBHOOK_URL")
		if test || endpoint == "" {
			fmt.Println("incident notification recorded locally:", s)
			return nil
		}
		b, _ := json.Marshal(map[string]string{"text": s})
		req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
		if e != nil {
			return fmt.Errorf("notification configuration invalid")
		}
		req.Header.Set("Content-Type", "application/json")
		c := &http.Client{Timeout: 5 * time.Second}
		r, e := c.Do(req)
		if e != nil {
			return fmt.Errorf("notification failed")
		}
		defer r.Body.Close()
		if r.StatusCode >= 300 {
			return fmt.Errorf("notification rejected")
		}
		return nil
	}
	w, e := watch.Open(watch.Config{File: file, HealthURL: health, Secret: os.Getenv("SCRY_WATCH_WEBHOOK_SECRET"), RestartCmd: os.Getenv("SCRY_WATCH_RESTART_CMD"), AgentCmd: os.Getenv("SCRY_WATCH_AGENT_CMD"), Commit: os.Getenv("SCRY_DEPLOY_COMMIT"), LogFile: os.Getenv("SCRY_WATCH_LOG_FILE"), Quiet: duration("SCRY_WATCH_QUIET", "2m"), Retry: duration("SCRY_WATCH_RETRY", "1m"), Notify: notify})
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	srv := &http.Server{Addr: env("SCRY_WATCH_LISTEN", "127.0.0.1:8091"), Handler: w, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 90 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	defer srv.Close()
	tick := time.NewTicker(duration("SCRY_WATCH_INTERVAL", "10s"))
	defer tick.Stop()
	for {
		if e = w.Tick(ctx, time.Now()); e != nil {
			return e
		}
		select {
		case <-ctx.Done():
			return nil
		case e := <-errs:
			return e
		case <-tick.C:
		}
	}
}
