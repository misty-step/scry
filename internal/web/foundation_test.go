package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func TestFoundationRoutesRetainPrivacyCSRFAndInertContent(t *testing.T) {
	ctx := context.Background()
	s, app := privateApp(t)
	if _, err := s.Capture(ctx, "Calvin cycle", "foundation-capture"); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(0)
	if err = s.CompleteJob(ctx, j.ID, j.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{{Kind: "recall", Prompt: "Which molecule supplies reducing electrons?", Answer: "NADPH", Explanation: "NADPH supplies electrons, unlike ATP's energy transfer.", Basis: "topic"}}, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RequestFoundation(ctx, state.Current.ID, "foundation-request", "", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	content := &store.FoundationContent{Units: []store.FoundationUnit{{Key: "electrons", Definition: "NADPH supplies reducing electrons.", Kind: "foundation"}}, Materials: []store.FoundationMaterial{
		{Kind: "instruction", Title: "Private foundation canary", Body: `<script>window.foundationInjection=true</script> NADPH donates electrons; ATP transfers energy.`, Links: []store.FoundationLink{{Unit: "electrons", Role: "teaches", Provenance: "Synthetic authored boundary fixture"}}},
		{Kind: "practice", Title: "Electron donor", Quiz: &store.GeneratedQuiz{Kind: "recall", Prompt: "Which molecule supplies reducing electrons?", Answer: "NADPH", Explanation: "NADPH supplies electrons; ATP transfers energy.", Basis: "topic"}, Links: []store.FoundationLink{{Unit: "electrons", Role: "directly-assesses", Provenance: "Identifies the electron donor"}}},
	}}
	if err = s.CompleteJob(ctx, j.ID, j.LeaseToken, store.GenerationResult{Foundation: content, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	b, err := s.FoundationBridge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	cookie, csrf, _ := bootstrapForm(t, app)
	path := "/foundations/" + id
	form := url.Values{"csrf": {csrf}, "operation_id": {"open-once"}, "revision": {strconv.Itoa(b.Revision)}, "action": {"open"}}
	for _, bad := range []string{"missing-csrf", "cross-site", "forged-peer", "other-owner"} {
		r := ownerRequest("POST", path, form)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://scry.example")
		switch bad {
		case "missing-csrf":
			r.Header.Set("Content-Type", "text/plain")
		case "cross-site":
			r.Header.Set("Origin", "https://evil.example")
		case "forged-peer":
			r.RemoteAddr = "192.0.2.1:4000"
		case "other-owner":
			r.Header.Set("X-ExeDev-UserID", "other")
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code < 400 || strings.Contains(w.Body.String(), "Private foundation canary") {
			t.Fatalf("%s mutation boundary failed: %d", bad, w.Code)
		}
	}
	fresh, err := s.FoundationBridge(ctx, id)
	if err != nil || fresh.Revision != b.Revision || fresh.Current != nil {
		t.Fatal("rejected mutation exposed/changed material")
	}
	r := ownerRequest("POST", path, form)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("owner instruction acknowledgment: %d %s", w.Code, w.Body.String())
	}
	// The same operation can reconcile a lost response without another exposure.
	r = ownerRequest("POST", path, form)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("exact transport retry failed")
	}
	fresh, err = s.FoundationBridge(ctx, id)
	if err != nil || fresh.Revision != b.Revision+1 {
		t.Fatal("transport retry duplicated progress")
	}
	for _, route := range []string{path, "/foundations", "/export"} {
		r = ownerRequest("GET", route, nil)
		r.Header.Del("X-ExeDev-UserID")
		w = httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 403 || strings.Contains(w.Body.String(), "Private foundation canary") || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("private read leaked: %s %d", route, w.Code)
		}
	}
	r = ownerRequest("GET", path, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "&lt;script&gt;") || strings.Contains(w.Body.String(), "<script>window.foundationInjection") {
		t.Fatal("instruction markup became executable")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("foundation route lost CSP")
	}
}
