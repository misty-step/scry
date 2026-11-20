// scry-watch is the single host-side incident and restart owner.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
func duration(k, d string) (time.Duration, error) {
	v, e := time.ParseDuration(env(k, d))
	if e != nil || v <= 0 {
		return 0, fmt.Errorf("invalid duration: %s", k)
	}
	return v, nil
}

// log is one JSON line per event on stdout, for journald.
var log = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func main() {
	if e := run(); e != nil {
		log.Error("scry-watch stopped", "err", e.Error())
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
			log.Warn("incident notification recorded locally", "notification", s)
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
	quiet, e := duration("SCRY_WATCH_QUIET", "2m")
	if e != nil {
		return e
	}
	retry, e := duration("SCRY_WATCH_RETRY", "1m")
	if e != nil {
		return e
	}
	interval, e := duration("SCRY_WATCH_INTERVAL", "10s")
	if e != nil {
		return e
	}
	w, e := watch.Open(watch.Config{File: file, HealthURL: health, Secret: os.Getenv("SCRY_WATCH_WEBHOOK_SECRET"), RestartCmd: os.Getenv("SCRY_WATCH_RESTART_CMD"), AgentCmd: os.Getenv("SCRY_WATCH_AGENT_CMD"), Commit: os.Getenv("SCRY_DEPLOY_COMMIT"), LogFile: os.Getenv("SCRY_WATCH_LOG_FILE"), Quiet: quiet, Retry: retry, Notify: notify})
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	srv := &http.Server{Addr: env("SCRY_WATCH_LISTEN", "127.0.0.1:8091"), Handler: w, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 90 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	defer srv.Close()
	// Only the host, port and path of the health URL are logged: never a
	// query string or credentials that might be in it.
	log.Info("scry-watch started", "health", u.Scheme+"://"+u.Host+u.Path, "listen", srv.Addr, "interval", interval.String(),
		"state", file, "test_mode", test, "webhook", w.Secret != "", "restart_cmd", w.RestartCmd != "")
	tick := time.NewTicker(interval)
	defer tick.Stop()
	last := ""
	for {
		if e = w.Tick(ctx, time.Now()); e != nil {
			if ctx.Err() != nil {
				break
			}
			return e
		}
		// One line per change of health, not one per probe.
		if healthy, streak := w.Status(); ctx.Err() == nil {
			now := "unhealthy"
			if healthy {
				now = "healthy"
			}
			if now != last {
				if healthy {
					log.Info("scry is healthy", "recovered", last == "unhealthy")
				} else {
					log.Warn("scry health check failed", "streak", streak)
				}
				last = now
			}
		}
		select {
		case <-ctx.Done():
		case e := <-errs:
			if errors.Is(e, http.ErrServerClosed) {
				return nil
			}
			return e
		case <-tick.C:
			continue
		}
		break
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	e = srv.Shutdown(shutdown)
	log.Info("scry-watch stopped cleanly")
	return e
}
