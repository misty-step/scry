package watch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func send(t *testing.T, w *Watch) {
	t.Helper()
	b := `{"action":"triggered","data":{"message":"SECRET user content"}}`
	mac := hmac.New(sha256.New, []byte(w.Secret))
	mac.Write([]byte(b))
	r := httptest.NewRequest("POST", "/", strings.NewReader(b))
	r.Header.Set("Sentry-Hook-Signature", hex.EncodeToString(mac.Sum(nil)))
	out := httptest.NewRecorder()
	w.ServeHTTP(out, r)
	if out.Code != 204 {
		t.Fatal(out.Code)
	}
}
func TestMonitoringDedupTriageFixQuietDurability(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	os.WriteFile(log, []byte("{\"level\":\"ERROR\",\"msg\":\"SECRET\",\"token\":\"SECRET\"}\n"), 0600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	calls := 0
	cfg := Config{File: filepath.Join(dir, "state"), Secret: "standin", HealthURL: srv.URL, Quiet: time.Second, Commit: "b68abb0", LogFile: log, AgentCmd: "cat > " + filepath.Join(dir, "brief"), Notify: func(context.Context, string) error { calls++; return nil }}
	w, e := Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	send(t, w)
	send(t, w)
	if e = w.Tick(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if len(w.State.Incidents) != 1 || w.active().Count != 2 || calls != 1 {
		t.Fatal(w.State)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "brief"))
	if strings.Contains(string(b), "SECRET") || !strings.Contains(string(b), "b68abb0") || !strings.Contains(string(b), "log level=ERROR") {
		t.Fatal(string(b))
	}
	if w.active().FixProposed.IsZero() || w.active().Alerted.IsZero() {
		t.Fatal(w.State)
	}
	w, e = Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Tick(context.Background(), time.Now().Add(2*time.Second)); e != nil {
		t.Fatal(e)
	}
	if w.active() != nil || w.State.Incidents[0].Recovered.IsZero() {
		t.Fatal(w.State)
	}
}
func freePort(t *testing.T) string {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}
func TestStory_ContinuousMonitoringFaultDrillRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "scry")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/scry")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, b)
	}
	port := freePort(t)
	health := "http://127.0.0.1:" + port + "/healthz"
	w, e := Open(Config{File: filepath.Join(dir, "state"), Secret: "standin", HealthURL: health, Threshold: 1, Quiet: time.Second, AgentCmd: "cat > " + filepath.Join(dir, "brief")})
	if e != nil {
		t.Fatal(e)
	}
	receiver := httptest.NewServer(w)
	defer receiver.Close()
	start := func() *exec.Cmd {
		c := exec.Command(binary)
		c.Env = append(os.Environ(), "SCRY_MODE=test", "PORT="+port, "SCRY_DB="+filepath.Join(dir, "app.db"), "SCRY_FAULT_DRILL=1", "SCRY_FAULT_DRILL_TOKEN=standin", "SCRY_WATCH_WEBHOOK_URL="+receiver.URL, "SCRY_WATCH_WEBHOOK_SECRET=standin")
		if e := c.Start(); e != nil {
			t.Fatal(e)
		}
		return c
	}
	proc := start()
	defer func() { proc.Process.Kill(); proc.Wait() }()
	ready := func() {
		t.Helper()
		for i := 0; i < 100; i++ {
			r, e := http.Get(health)
			if e == nil {
				r.Body.Close()
				if r.StatusCode == 200 {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("service not ready")
	}
	ready()
	req, _ := http.NewRequest("POST", "http://127.0.0.1:"+port+"/ops/fault-drill", nil)
	req.Header.Set("Authorization", "Bearer standin")
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 500 {
		t.Fatal(r.StatusCode)
	}
	// HTTP middleware completes its synchronous signed delivery before response returns.
	w.mu.Lock()
	if len(w.State.Incidents) != 1 {
		t.Fatal(w.State)
	}
	w.mu.Unlock()
	proc.Process.Kill()
	proc.Wait()
	// Configured restart command launches a replacement real process.
	marker := filepath.Join(dir, "restart")
	w.RestartCmd = "touch " + marker + "; nohup setsid env"
	for _, v := range proc.Env {
		w.RestartCmd += " '" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
	}
	w.RestartCmd += " " + binary + " >/dev/null 2>&1 </dev/null & echo $! > " + filepath.Join(dir, "pid")
	defer func() {
		b, _ := os.ReadFile(filepath.Join(dir, "pid"))
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	if e = w.Tick(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(marker); e != nil {
		t.Fatal(e)
	}
	ready()
	if e = w.Tick(context.Background(), time.Now().Add(2*time.Second)); e != nil {
		t.Fatal(e)
	}
	if len(w.State.Incidents) != 1 || w.active() != nil || w.State.Incidents[0].FixProposed.IsZero() || w.State.Incidents[0].Recovered.IsZero() {
		t.Fatal(w.State)
	}
	if _, e = os.Stat(filepath.Join(dir, "brief")); e != nil {
		t.Fatal(e)
	}
}
func TestWebhookAuthenticationAndDisabled(t *testing.T) {
	for _, secret := range []string{"", "secret"} {
		w, _ := Open(Config{File: filepath.Join(t.TempDir(), "state"), Secret: secret})
		out := httptest.NewRecorder()
		w.ServeHTTP(out, httptest.NewRequest("POST", "/", strings.NewReader("{}")))
		if out.Code != 401 && out.Code != 404 {
			t.Fatal(out.Code)
		}
		if len(w.State.Incidents) != 0 {
			t.Fatal(w.State)
		}
	}
}

func TestUnconfiguredFixAndFailedHandoffRetry(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	dir := t.TempDir()
	w, _ := Open(Config{File: filepath.Join(dir, "state"), Secret: "standin", HealthURL: srv.URL, Quiet: time.Hour})
	send(t, w)
	if w.active().FixStatus != "fix proposal not configured" {
		t.Fatal(w.active())
	}
	w.AgentCmd = "exit 1"
	send(t, w)
	if e := w.Tick(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	if !w.active().FixProposed.IsZero() {
		t.Fatal("failed command marked successful")
	}
	w.AgentCmd = "cat > " + filepath.Join(dir, "brief")
	if e := w.Tick(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	if w.active().FixProposed.IsZero() {
		t.Fatal("handoff not retried")
	}
}

// Slice 12: a probe cut short by the watcher's own SIGTERM is not an outage,
// so stopping scry-watch never opens an incident or runs a restart.
func TestCanceledProbeIsNotAnOutage(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer down.Close()
	file := filepath.Join(t.TempDir(), "incidents.json")
	w, e := Open(Config{File: file, HealthURL: down.URL, Threshold: 1, Notify: func(context.Context, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := w.Tick(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	if healthy, streak := w.Status(); healthy || streak != 0 || len(w.State.Incidents) != 0 {
		t.Fatalf("canceled probe counted: healthy=%v streak=%d incidents=%d", healthy, streak, len(w.State.Incidents))
	}
	// A real failure still counts.
	if e := w.Tick(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if healthy, streak := w.Status(); healthy || streak != 1 || len(w.State.Incidents) != 1 {
		t.Fatalf("real failure not counted: healthy=%v streak=%d incidents=%d", healthy, streak, len(w.State.Incidents))
	}
}
