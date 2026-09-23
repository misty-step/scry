package web

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/misty-step/scry/internal/store"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewMutationsRequireOwnerAndCSRF(t *testing.T) {
	_, app := privateApp(t)
	cookie, csrf, _ := bootstrapForm(t, app)
	for _, path := range []string{"/review/self", "/review/override", "/review/intro", "/concepts/c/practice", "/concepts/c/note", "/concepts/c/questions", "/concepts/c/archive", "/goals/g", "/quizzes/q/fix", "/quizzes/q/proposal", "/settings"} {
		t.Run(path, func(t *testing.T) {
			for _, change := range []struct {
				name  string
				apply func(*http.Request)
			}{
				{"missing csrf", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
				{"wrong origin", func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }},
				{"wrong owner", func(r *http.Request) { r.Header.Set("X-ExeDev-UserID", "other") }},
			} {
				r := ownerRequest("POST", path, url.Values{"csrf": {csrf}, "operation_id": {"op"}})
				r.AddCookie(cookie)
				r.Header.Set("Origin", "https://scry.example")
				change.apply(r)
				w := httptest.NewRecorder()
				app.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden {
					t.Errorf("%s: %d", change.name, w.Code)
				}
			}
		})
	}
}

func TestCaptureRequiresModeAndBoundsPhoto(t *testing.T) {
	_, app := privateApp(t)
	cookie, csrf, op := bootstrapForm(t, app)
	submit := func(values url.Values) *httptest.ResponseRecorder {
		t.Helper()
		values.Set("csrf", csrf)
		values.Set("operation_id", op)
		r := ownerRequest("POST", "/add", values)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://scry.example")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	for _, values := range []url.Values{{"text": {"some topic"}}, {"text": {"some topic"}, "mode": {"incorrect"}}, {"mode": {"photo"}}} {
		if w := submit(values); w.Code != 422 {
			t.Errorf("capture accepted missing/invalid mode or photo: %d %s", w.Code, w.Body.String())
		}
	}
	upload := func(payload []byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for key, value := range map[string]string{"csrf": csrf, "operation_id": op, "mode": "photo"} {
			if err := form.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		part, err := form.CreateFormFile("photo", "photo.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err = form.Close(); err != nil {
			t.Fatal(err)
		}
		r := ownerRequest("POST", "/add", nil)
		r.Body = io.NopCloser(&body)
		r.ContentLength = int64(body.Len())
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "https://scry.example")
		r.Header.Set("Accept", "application/json")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	if w := upload([]byte("not an image")); w.Code != 422 {
		t.Errorf("invalid image accepted: %d", w.Code)
	}
	if w := upload(make([]byte, 5<<20)); w.Code != 413 {
		t.Errorf("oversize body accepted: %d", w.Code)
	}
	photo, err := os.ReadFile("assets/icon-192.png")
	if err != nil {
		t.Fatal(err)
	}
	w := upload(photo)
	if w.Code != 200 {
		t.Fatalf("valid private photo: %d %s", w.Code, w.Body.String())
	}
	var source struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &source); err != nil || source.ID == "" {
		t.Fatalf("saved photo ID: %v %s", err, w.Body.String())
	}
	image := httptest.NewRecorder()
	r := ownerRequest("GET", "/sources/"+source.ID+"/image", nil)
	r.AddCookie(cookie)
	app.ServeHTTP(image, r)
	if image.Code != 200 || image.Header().Get("Content-Type") != "image/png" || !bytes.Equal(image.Body.Bytes(), photo) {
		t.Fatalf("private image retrieval: %d", image.Code)
	}
	unauthorized := httptest.NewRecorder()
	r.Header.Del("X-ExeDev-UserID")
	app.ServeHTTP(unauthorized, r)
	if unauthorized.Code != 403 || bytes.Contains(unauthorized.Body.Bytes(), photo) {
		t.Fatal("photo escaped private boundary")
	}
}

func TestAssetAllowlistBytesAndHeaders(t *testing.T) {
	_, app := privateApp(t)
	for _, path := range []string{"app.css", "app.js", "icon.svg", "icon-192.png", "icon-512.png", "manifest.webmanifest", "fonts/Fraunces.woff2", "fonts/Fraunces-Italic.woff2", "fonts/InstrumentSans.woff2", "fonts/OFL.txt"} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, ownerRequest("GET", "/assets/"+path, nil))
		expected, err := os.ReadFile(filepath.Join("assets", path))
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), expected) {
			t.Errorf("asset %s: %d or changed bytes", path, w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "manifest-src 'self'") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self' blob:") || !strings.Contains(w.Header().Get("Permissions-Policy"), "microphone=(self)") {
			t.Errorf("missing security headers: %s", path)
		}
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, ownerRequest("GET", "/assets/unknown", nil))
	if w.Code != 404 {
		t.Fatalf("allowlist bypass: %d", w.Code)
	}
	var manifest struct {
		ShareTarget struct {
			Action string `json:"action"`
			Method string `json:"method"`
		} `json:"share_target"`
	}
	data, err := os.ReadFile("assets/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ShareTarget.Action != "/add" || manifest.ShareTarget.Method != "GET" {
		t.Fatal("share target does not prefill capture")
	}
}

// Without JavaScript a form POST is a navigation. Under Referrer-Policy
// no-referrer browsers send it with Origin: null, which the origin check
// rightly refuses, so no answer could be saved without JS. The served policy
// keeps the same-origin Origin intact, and a null origin stays refused.
func TestNoScriptFormPostKeepsVerifiableOrigin(t *testing.T) {
	s, app := privateApp(t)
	if _, err := s.Capture(context.Background(), store.CaptureInput{Text: "Synthetic records", Mode: "topic"}, randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "choice", Prompt: "Which record maps a name to IPv4?", Answer: "A", Choices: []string{"A", "MX", "TXT"}, Explanation: "An A record holds an IPv4 address.", Basis: "topic"})
	current := openReviewState(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, ownerRequest(http.MethodGet, "/", nil))
	if policy := w.Header().Get("Referrer-Policy"); policy != "same-origin" {
		t.Fatalf("Referrer-Policy %q makes browsers send Origin: null on no-JS form posts", policy)
	}
	form := url.Values{"csrf": {csrf}, "operation_id": {operation}, "presentation_id": {current.ID}, "answer": {"A"}}
	for _, tc := range []struct {
		origin string
		status int
	}{{"null", http.StatusForbidden}, {"https://scry.example", http.StatusSeeOther}} {
		r := ownerRequest(http.MethodPost, "/review/answer", form)
		r.AddCookie(cookie)
		r.Header.Set("Origin", tc.origin)
		w = httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("plain form POST with Origin %q: %d, want %d", tc.origin, w.Code, tc.status)
		}
	}
}

func TestConceptGateAndEmptyRecallReveal(t *testing.T) {
	s, app := privateApp(t)
	_, err := s.Capture(context.Background(), store.CaptureInput{Text: "Synthetic electron carrier", Mode: "topic"}, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "recall", Prompt: "Name the synthetic carrier.", Answer: "NADPH", Explanation: "NADPH carries synthetic electrons.", Basis: "topic"})
	current := openReviewState(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	path := "/concepts/" + current.Quiz.ConceptID
	mapRequest := ownerRequest("GET", "/map", nil)
	mapRequest.AddCookie(cookie)
	mapResponse := httptest.NewRecorder()
	app.ServeHTTP(mapResponse, mapRequest)
	if mapResponse.Code != 200 || !strings.Contains(mapResponse.Body.String(), `class="constellation"`) || !strings.Contains(mapResponse.Body.String(), `href="`+path+`"`) {
		t.Fatalf("map did not show the goal constellation with its concept: %d %s", mapResponse.Code, mapResponse.Body.String())
	}
	for _, accept := range []string{"application/json", "text/html"} {
		r := ownerRequest("GET", path, nil)
		r.AddCookie(cookie)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if accept == "application/json" && w.Code != 409 {
			t.Fatalf("concept gate JSON: %d", w.Code)
		}
		if accept == "text/html" && (w.Code != 200 || !strings.Contains(w.Body.String(), `action="/review/reveal"`) || strings.Contains(w.Body.String(), "NADPH")) {
			t.Fatalf("concept gate HTML: %d", w.Code)
		}
	}
	r := ownerRequest("POST", "/review/answer", url.Values{"csrf": {csrf}, "operation_id": {operation}, "presentation_id": {current.ID}, "answer": {""}})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("empty recall reveal: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Review    store.ReviewState `json:"review"`
		CSRF      string            `json:"csrf"`
		Operation string            `json:"operation_id"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Review.Current == nil || response.Review.Current.Outcome != "revealed" || !response.Review.Current.Graded || response.CSRF == "" || response.Operation == "" {
		t.Fatalf("empty recall not recorded as reveal: %+v", response.Review)
	}
	r = ownerRequest("GET", path, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "NADPH") {
		t.Fatalf("concept not accessible after reveal: %d", w.Code)
	}
}

func TestAutomaticMissCanBeCorrectedInPlace(t *testing.T) {
	s, app := privateApp(t)
	if _, err := s.Capture(context.Background(), store.CaptureInput{Text: "Synthetic carriers", Mode: "topic"}, randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "choice", Prompt: "Which synthetic carrier supplies electrons?", Answer: "NADPH", Choices: []string{"ATP", "NADPH", "FADH2"}, Explanation: "NADPH supplies electrons.", Basis: "topic"})
	current := openReviewState(t, s)
	cookie, csrf, _ := bootstrapForm(t, app)
	post := func(path string, values url.Values) {
		t.Helper()
		values.Set("csrf", csrf)
		values.Set("operation_id", randomToken())
		r := ownerRequest("POST", path, values)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://scry.example")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 303 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	post("/review/answer", url.Values{"presentation_id": {current.ID}, "answer": {"ATP"}})
	persisted, err := s.Current(context.Background())
	if err != nil || persisted.Outcome != "wrong" {
		t.Fatalf("automatic miss: %+v %v", persisted, err)
	}
	post("/review/override", url.Values{"presentation_id": {current.ID}, "correct": {"yes"}})
	persisted, err = s.Current(context.Background())
	if err != nil || persisted.Override != "correct" {
		t.Fatalf("override missing: %+v %v", persisted, err)
	}
	page := reviewPage(t, app, cookie)
	if !strings.Contains(page, `class="feedback feedback-correct"`) || !strings.Contains(page, `>Correct.</h2>`) || strings.Contains(page, `>I was right</button>`) {
		t.Fatal("corrected result still appears as a miss or offers duplicate correction")
	}
}

// US-005: shared material is prefilled, but no mode is chosen for the
// learner; Topic and Link send material to web research only by explicit choice.
func TestShareTargetNeverChoosesCaptureModeUS005(t *testing.T) {
	_, app := privateApp(t)
	for _, tc := range []struct{ query, text string }{
		{"text=shared+excerpt&url=https%3A%2F%2Fexample.org%2Farticle&title=Shared+title", "https://example.org/article"},
		{"text=my+private+note&title=Shared+title", "my private note"},
		{"title=Shared+title", "Shared title"},
		{"", ""},
	} {
		r := ownerRequest("GET", "/add?"+tc.query, nil)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		body := w.Body.String()
		if w.Code != 200 || strings.Contains(body, "checked") || !strings.Contains(body, ">"+html.EscapeString(tc.text)+"</textarea>") {
			t.Fatalf("share %q: status %d, prefill or mode wrong: %s", tc.query, w.Code, body)
		}
		r = ownerRequest("GET", "/add?"+tc.query, nil)
		r.Header.Set("Accept", "application/json")
		w = httptest.NewRecorder()
		app.ServeHTTP(w, r)
		var prefill struct{ Text, Mode string }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &prefill) != nil || prefill.Text != tc.text || prefill.Mode != "" {
			t.Fatalf("share %q JSON prefill: %d %s", tc.query, w.Code, w.Body.String())
		}
	}
}

// Material from the cold question's own capture (its text, goal title, and
// preparation receipts) stays hidden on every route until assistance is
// recorded; a sibling concept page never carries the capture's text.
func TestCurrentCaptureMaterialHiddenUntilAssisted(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, store.CaptureInput{Text: "CAPTURE-TEXT certificate trust", Mode: "topic"}, randomToken()); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	publish := func(kind string, result func(*store.Job) store.GenerationResult) {
		t.Helper()
		job, err := s.ClaimJob(ctx, time.Minute, 1, 1000000)
		if err != nil || job == nil || job.Kind != kind {
			t.Fatalf("claim %s: %+v %v", kind, job, err)
		}
		content := result(job)
		content.Model, content.PromptVersion = "authored-test-fixture", "fixture-v5"
		if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, content, &zero); err != nil {
			t.Fatal(err)
		}
	}
	note := func(title string) *store.NoteContent {
		return &store.NoteContent{Level: "standard", Title: title, Basis: "topic", Body: "A synthetic note long enough to explain this concept in a controlled test of the web layer."}
	}
	publish("research", func(*store.Job) store.GenerationResult { return store.GenerationResult{} })
	publish("plan", func(*store.Job) store.GenerationResult {
		return store.GenerationResult{Plan: &store.PlanContent{Goal: "GOAL-TITLE certificate trust", Concepts: []store.PlannedConcept{
			{Key: "a", Name: "Chain of trust", Summary: "Certificates chain to a trusted root.", Note: note("Chain of trust")},
			{Key: "b", Name: "Hostname check", Summary: "The certificate names the host.", Note: note("Hostname check")},
		}}}
	})
	publish("questions", func(job *store.Job) store.GenerationResult {
		jc, err := s.JobContext(ctx, job.ID)
		if err != nil || len(jc.Concepts) != 2 {
			t.Fatalf("questions context: %+v %v", jc, err)
		}
		return store.GenerationResult{Quizzes: []store.GeneratedQuiz{
			{Kind: "recall", Level: "recall", AnswerForm: "exact", Concept: jc.Concepts[0].ID, Prompt: "Where does a chain of trust end?", Answer: "A root", Explanation: "At a trusted root.", Basis: "topic"},
			{Kind: "recall", Level: "recall", AnswerForm: "exact", Concept: jc.Concepts[1].ID, Prompt: "What must a certificate name?", Answer: "The host", Explanation: "The host it serves.", Basis: "topic"},
		}}
	})
	var cold *store.Presentation
	for state, err := s.Review(ctx); cold == nil; {
		if err != nil || (state.Intro == nil && state.Current == nil) {
			t.Fatalf("no question reached: %+v %v", state, err)
		}
		if state.Current != nil {
			cold = state.Current
			break
		}
		state, err = s.AcknowledgeIntro(ctx, state.Intro.Concept.ID, randomToken(), false)
	}
	page, err := s.ConceptPage(ctx, cold.Concept.ID)
	if err != nil || len(page.Goals) != 1 {
		t.Fatalf("current concept: %+v %v", page, err)
	}
	m, err := s.Map(ctx, "")
	if err != nil || len(m.Goals) != 1 || len(m.Goals[0].Concepts) != 2 {
		t.Fatalf("map: %+v %v", m, err)
	}
	sibling := m.Goals[0].Concepts[0].ID
	if sibling == cold.Concept.ID {
		sibling = m.Goals[0].Concepts[1].ID
	}
	// A live preparation for the same capture puts its receipt beside the question.
	if err = s.RequestQuestions(ctx, sibling, randomToken()); err != nil {
		t.Fatal(err)
	}
	get := func(path string, status int) string {
		t.Helper()
		r := ownerRequest(http.MethodGet, path, nil)
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, path := range []string{"/", "/map", "/map?q=certificate", "/concepts/" + sibling} {
		body := get(path, http.StatusOK)
		if strings.Contains(body, "CAPTURE-TEXT") || strings.Contains(body, "GOAL-TITLE") {
			t.Fatalf("%s exposed the cold question's capture material: %s", path, body)
		}
	}
	if body := get("/", http.StatusOK); !strings.Contains(body, currentMaterial) {
		t.Fatalf("stream receipt was not relabeled: %s", body)
	}
	// Searching the cold question's answer, or anything else from its
	// capture (sibling concepts, notes, questions), finds nothing yet.
	for _, q := range []string{"root", "host", "trust", "synthetic"} {
		if body := get("/map?q="+q, http.StatusOK); !strings.Contains(body, `"hits":[]`) {
			t.Fatalf("search %q found the cold question's own capture: %s", q, body)
		}
	}
	get("/concepts/"+cold.Concept.ID, http.StatusConflict)
	// Once assistance is recorded, the material is the learner's again.
	if _, err = s.Submit(ctx, cold.ID, randomToken(), "", true); err != nil {
		t.Fatal(err)
	}
	if body := get("/map", http.StatusOK); !strings.Contains(body, "GOAL-TITLE") {
		t.Fatalf("goal title stayed hidden after assistance: %s", body)
	}
	if body := get("/map?q=host", http.StatusOK); strings.Contains(body, `"hits":[]`) {
		t.Fatalf("search stayed empty after assistance: %s", body)
	}
}

// A requested fix is shown as a suggestion beside the current question and
// changes nothing until the learner chooses "Use this version".
func TestSuggestedFixWaitsForTheLearner(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	src, err := s.Capture(ctx, store.CaptureInput{Text: "Synthetic records", Mode: "topic"}, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{Kind: "choice", Prompt: "Which record maps a hostname to an IPv4 address?", Answer: "A", Choices: []string{"A", "MX", "TXT"}, Explanation: "An A record holds an IPv4 address.", Basis: "topic"})
	saved, err := s.Source(ctx, src.ID)
	if err != nil || len(saved.Quizzes) != 1 {
		t.Fatalf("fixture question: %+v %v", saved.Quizzes, err)
	}
	q := saved.Quizzes[0]
	if err = s.RequestFix(ctx, q.ID, "Shorter prompt, please", randomToken()); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 1, 1000000)
	if err != nil || job == nil || job.Kind != "fix" {
		t.Fatalf("claim fix: %+v %v", job, err)
	}
	zero := int64(0)
	if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, store.GenerationResult{Model: "authored-test-fixture", PromptVersion: "fixture-v5", Quizzes: []store.GeneratedQuiz{{Kind: "recall", Level: "recall", AnswerForm: "exact",
		Concept: q.ConceptID, Prompt: "SUGGESTED which record holds an IPv4 address?", Answer: "A", Variants: []string{"Address record"}, Explanation: "SUGGESTED An A record stores one IPv4 address.", Basis: "topic"}}}, &zero); err != nil {
		t.Fatal(err)
	}
	cookie, csrf, operation := bootstrapForm(t, app)
	edit := func() string {
		t.Helper()
		r := ownerRequest(http.MethodGet, "/quizzes/"+q.ID+"/edit", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("edit page: %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	body := edit()
	start := strings.Index(body, `class="proposal`)
	if start < 0 || !strings.Contains(body, "Scry suggests a fix") {
		t.Fatalf("the suggestion was not offered: %s", body)
	}
	// Accepting installs every field, so the comparison shows every field on
	// both sides: style, choices, accepted variants, and explanation.
	panel := body[start : start+strings.Index(body[start:], "</section>")]
	for _, want := range []string{"Pick an answer", "MX", "An A record holds an IPv4 address.",
		"Answer from memory", "SUGGESTED which record", "Address record", "SUGGESTED An A record stores one IPv4 address."} {
		if !strings.Contains(panel, want) {
			t.Fatalf("the comparison hides %q: %s", want, panel)
		}
	}
	if current, err := s.Quiz(ctx, q.ID); err != nil || current.Version != q.Version || strings.HasPrefix(current.Prompt, "SUGGESTED") {
		t.Fatalf("the suggestion replaced the question before it was accepted: %+v %v", current, err)
	}
	fix, err := s.QuizFix(ctx, q.ID)
	if err != nil || fix.Proposal == nil {
		t.Fatalf("pending suggestion: %+v %v", fix, err)
	}
	r := ownerRequest(http.MethodPost, "/quizzes/"+q.ID+"/proposal", url.Values{"csrf": {csrf}, "operation_id": {operation}, "proposal_id": {fix.Proposal.ID}, "decision": {"accept"}})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("accepting the suggestion: %d %s", w.Code, w.Body.String())
	}
	if current, err := s.Quiz(ctx, q.ID); err != nil || current.Version != q.Version+1 || !strings.HasPrefix(current.Prompt, "SUGGESTED") {
		t.Fatalf("the accepted suggestion was not installed: %+v %v", current, err)
	}
	if body := edit(); strings.Contains(body, "Scry suggests a fix") {
		t.Fatal("an accepted suggestion is still offered")
	}
}
