package web_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"scry/internal/auth"
)

// Slice 12: under Clerk auth every structured request line carries the
// signed-in learner's id, as it already did under local auth.
func TestClerkRequestLogsCarryUserID(t *testing.T) {
	w := newWorld(t)
	w.app.Auth = &auth.Clerk{SecretKey: "stand-in-only", Resolve: func(_ context.Context, token, _ string) (string, string, error) {
		if token != "verified" {
			return "", "", errors.New("verify email")
		}
		return "user_clerk_log", "logged@example.com", nil
	}}
	b := w.browser()
	target, _ := url.Parse(w.srv.URL)
	b.c.Jar.SetCookies(target, []*http.Cookie{{Name: "__session", Value: "verified"}})
	b.get("/billing")
	b.get("/settings")
	u := w.user("logged@example.com")
	if u.ID == 0 {
		t.Fatal("no learner")
	}
	// A signed-out request logs user 0.
	anon := w.browser()
	anon.c.Jar.SetCookies(target, []*http.Cookie{{Name: "__session", Value: "forged"}})
	anon.get("/settings")

	seen := map[string]float64{}
	var anonUser float64 = -1
	sc := bufio.NewScanner(strings.NewReader(w.logs.String()))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		var line map[string]any
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil || line["msg"] != "request" {
			continue
		}
		path, _ := line["path"].(string)
		user, _ := line["user"].(float64)
		if path == "/billing" || path == "/settings" {
			if _, ok := seen[path]; !ok {
				seen[path] = user
			} else if path == "/settings" {
				anonUser = user
			}
		}
	}
	if seen["/billing"] != float64(u.ID) || seen["/settings"] != float64(u.ID) {
		t.Fatalf("Clerk request logs user = %v, want %d\n%s", seen, u.ID, w.logs.String())
	}
	if anonUser != 0 {
		t.Fatalf("signed-out request logged user %v", anonUser)
	}
}

// /healthz keeps the fields the deploy gate and scry-watch read, and adds
// readiness: build, mode and adapter labels, never a credential.
func TestHealthzReadinessFields(t *testing.T) {
	w := newWorld(t)
	w.app.Cfg.BuildSHA = "0123abc"
	w.app.Auth = &auth.Clerk{PublishableKey: "pk_test_standin", SecretKey: "sk_test_SHOULDNOTLEAK"}
	resp, err := http.Get(w.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		raw.WriteString(sc.Text())
	}
	body := raw.String()
	var h map[string]any
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.Contains(body, `"status":"ok"`) || h["database"] != "ok" || h["db"] != true {
		t.Fatal(body)
	}
	if h["build_sha"] != "0123abc" || h["mode"] != "test" || h["auth_mode"] != "clerk-test" || h["billing_mode"] != "test" {
		t.Fatal(body)
	}
	for _, k := range []string{"grading", "drafting", "explainer", "speech"} {
		if h[k] != false {
			t.Fatalf("%s = %v with test adapters", k, h[k])
		}
	}
	p, _ := h["providers"].(map[string]any)
	if p["grading"] != "spy" || p["drafting"] != "spy" || p["auth"] != "clerk-test" || p["explainer"] != "none" {
		t.Fatal(p)
	}
	if strings.Contains(body, "SHOULDNOTLEAK") || strings.Contains(body, "sk_") || strings.Contains(body, "pk_") {
		t.Fatalf("/healthz leaks a key: %s", body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("healthz is cacheable")
	}
}
