package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func privateApp(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "scry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h, err := New(s, Config{Mode: "production", OwnerID: "owner-123", Secret: strings.Repeat("s", 32), BaseURL: "https://scry.example", TrustProxy: true, TrustedProxyIPs: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, h
}

func ownerRequest(method, path string, form url.Values) *http.Request {
	body := ""
	if form != nil {
		body = form.Encode()
	}
	r := httptest.NewRequest(method, "https://scry.example"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Set("X-ExeDev-UserID", "owner-123")
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return r
}

func TestPrivateIdentityCannotBeForgedThroughAnotherPeerOrHeader(t *testing.T) {
	s, app := privateApp(t)
	const secretMaterial = "private-boundary-canary-material"
	if _, err := s.Capture(context.Background(), secretMaterial, randomToken()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		change  func(*http.Request)
		allowed bool
	}{
		{"approved identity on configured peer", func(r *http.Request) {}, true},
		{"missing identity", func(r *http.Request) { r.Header.Del("X-ExeDev-UserID") }, false},
		{"different owner", func(r *http.Request) { r.Header.Set("X-ExeDev-UserID", "other-owner") }, false},
		{"duplicated identity", func(r *http.Request) { r.Header.Add("X-ExeDev-UserID", "owner-123") }, false},
		{"joined identity", func(r *http.Request) { r.Header.Set("X-ExeDev-UserID", "owner-123, owner-123") }, false},
		{"direct backend forged forwarded peer", func(r *http.Request) {
			r.RemoteAddr = "192.0.2.12:4000"
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			r.Header.Set("Forwarded", "for=127.0.0.1;proto=https")
		}, false},
		{"other host with forged forwarded host", func(r *http.Request) { r.Host = "alternate.example"; r.Header.Set("X-Forwarded-Host", "scry.example") }, false},
		{"forwarded protocol cannot weaken authority", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ownerRequest(http.MethodGet, "/export", nil)
			tc.change(r)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if tc.allowed {
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), secretMaterial) {
					t.Fatalf("owner cannot read own export: status %d", w.Code)
				}
			} else {
				if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), secretMaterial) {
					t.Fatalf("private data boundary failed: status %d", w.Code)
				}
			}
			if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("private export response is cacheable")
			}
		})
	}
}

func bootstrapForm(t *testing.T, app http.Handler) (*http.Cookie, string, string) {
	t.Helper()
	r := ownerRequest(http.MethodGet, "/add", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("form bootstrap: %d", w.Code)
	}
	var form struct {
		CSRF      string `json:"csrf"`
		Operation string `json:"operation_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &form); err != nil {
		t.Fatal(err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one private form session, got %d", len(cookies))
	}
	if !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("private form cookie is not protected")
	}
	return cookies[0], form.CSRF, form.Operation
}

func TestMutationRequiresCanonicalOriginAndSessionBoundCSRF(t *testing.T) {
	s, app := privateApp(t)
	cookie, csrf, operation := bootstrapForm(t, app)
	otherCookie, _, _ := bootstrapForm(t, app)
	cases := []struct {
		name   string
		origin string
		token  string
		cookie *http.Cookie
	}{
		{"cross site", "https://attacker.example", csrf, cookie},
		{"missing origin and referer", "", csrf, cookie},
		{"missing csrf", "https://scry.example", "", cookie},
		{"other session token", "https://scry.example", csrf, otherCookie},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ownerRequest(http.MethodPost, "/add", url.Values{"text": {"must not be captured"}, "operation_id": {operation}, "csrf": {tc.token}})
			r.AddCookie(tc.cookie)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("unsafe mutation returned %d", w.Code)
			}
		})
	}
	before, err := s.Sources(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatal("rejected cross-site requests changed stored content")
	}
	r := ownerRequest(http.MethodPost, "/add", url.Values{"text": {"native form saved input"}, "operation_id": {operation}, "csrf": {csrf}})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	r.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("ordinary form did not redirect to saved input: %d", w.Code)
	}
	after, err := s.Sources(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Text != "native form saved input" {
		t.Fatal("valid private form was not durably captured")
	}
}

func TestOversizedCapturePreservesInputWithoutSavingOrTruncating(t *testing.T) {
	s, app := privateApp(t)
	cookie, csrf, operation := bootstrapForm(t, app)
	const attack = `</textarea><script id="injected-script">alert(1)</script>`
	padding := strings.Repeat("a", 32769)
	text := padding + attack
	r := ownerRequest(http.MethodPost, "/add", url.Values{"text": {text}, "operation_id": {operation}, "csrf": {csrf}})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized capture returned %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), padding) || !strings.Contains(w.Body.String(), operation) {
		t.Fatal("rejected input or retry identity was lost from the returned form")
	}
	if strings.Contains(w.Body.String(), attack) || !strings.Contains(w.Body.String(), "&lt;/textarea&gt;&lt;script") {
		t.Fatal("untrusted input was activated or erased instead of escaped in the error form")
	}
	items, err := s.Sources(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatal("oversized capture was saved or silently truncated")
	}
}

func TestAlternateHostsDoNotExposeContentOrReplayMutations(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	app, err := New(s, Config{
		Mode: "production", OwnerID: "owner-123", Secret: strings.Repeat("s", 32),
		BaseURL: "https://scry.example", TrustProxy: true, TrustedProxyIPs: []string{"127.0.0.1"},
		RedirectHosts: []string{"www.scry.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const material = "private-alternate-host-canary"
	if _, err := s.Capture(context.Background(), material, randomToken()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/export", "//attacker.example/export?download=1"} {
		r := ownerRequest(http.MethodGet, path, nil)
		r.Host = "www.scry.example"
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		location, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusPermanentRedirect || location.Scheme != "https" || location.Host != "scry.example" ||
			location.EscapedPath() != r.URL.EscapedPath() || location.RawQuery != r.URL.RawQuery {
			t.Fatalf("alternate address did not preserve the path on the canonical origin: %d %s", w.Code, location)
		}
		if strings.Contains(w.Body.String(), material) || len(w.Result().Cookies()) != 0 {
			t.Fatal("alternate address exposed private content or issued a private session")
		}
	}
	cookie, csrf, operation := bootstrapForm(t, app)
	r := ownerRequest(http.MethodPost, "/add", url.Values{
		"text": {"must not be captured"}, "operation_id": {operation}, "csrf": {csrf},
	})
	r.Host = "www.scry.example"
	r.Header.Set("Origin", "https://scry.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || w.Header().Get("Location") != "" {
		t.Fatalf("alternate-address mutation was accepted or redirected: %d", w.Code)
	}
	sources, err := s.Sources(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Text != material {
		t.Fatal("alternate-address mutation changed stored content")
	}
}
