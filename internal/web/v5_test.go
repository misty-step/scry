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
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewMutationsRequireOwnerAndCSRF(t *testing.T) {
	_, app := privateApp(t)
	cookie, csrf, _ := bootstrapForm(t, app)
	for _, path := range []string{"/review/self", "/review/override", "/review/intro", "/concepts/c/practice", "/concepts/c/questions", "/concepts/c/archive", "/goals/g", "/quizzes/q/fix", "/settings"} {
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
	for _, path := range []string{"app.css", "app.js", "icon.svg", "icon-192.png", "icon-512.png", "manifest.webmanifest", "fonts/Literata.woff2", "fonts/Literata-Italic.woff2", "fonts/AtkinsonHyperlegibleNext.woff2", "fonts/OFL.txt"} {
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
		return &store.NoteContent{Title: title, Basis: "topic", Body: "A synthetic note long enough to explain this concept in a controlled test of the web layer."}
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
	m, err := s.Map(ctx)
	if err != nil || len(m.Goals) != 1 || len(m.Goals[0].Concepts) != 2 {
		t.Fatalf("map: %+v %v", m, err)
	}
	sibling := m.Goals[0].Concepts[0].ID
	if sibling == cold.Concept.ID {
		sibling = m.Goals[0].Concepts[1].ID
	}
	// A sibling question from the same capture, with a fix that stopped:
	// its edit page stays reachable but carries nothing from the capture.
	siblingPage, err := s.ConceptPage(ctx, sibling)
	if err != nil || len(siblingPage.Questions) != 1 {
		t.Fatalf("sibling concept: %+v %v", siblingPage, err)
	}
	siblingQuiz := siblingPage.Questions[0].ID
	if err = s.RequestFix(ctx, siblingQuiz, "Make it shorter", randomToken()); err != nil {
		t.Fatal(err)
	}
	fixJob, err := s.ClaimJob(ctx, time.Minute, 1, 1000000)
	if err != nil || fixJob == nil || fixJob.Kind != "fix" {
		t.Fatalf("claim fix: %+v %v", fixJob, err)
	}
	if err = s.FailJob(ctx, fixJob.ID, fixJob.LeaseToken, "synthetic provider failure", false, &zero); err != nil {
		t.Fatal(err)
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
	for _, path := range []string{"/", "/map", "/concepts/" + sibling, "/quizzes/" + siblingQuiz + "/edit"} {
		body := get(path, http.StatusOK)
		if strings.Contains(body, "CAPTURE-TEXT") || strings.Contains(body, "GOAL-TITLE") {
			t.Fatalf("%s exposed the cold question's capture material: %s", path, body)
		}
	}
	if body := get("/", http.StatusOK); !strings.Contains(body, currentMaterial) {
		t.Fatalf("stream receipt was not relabeled: %s", body)
	}
	get("/concepts/"+cold.Concept.ID, http.StatusConflict)
	// Once assistance is recorded, the material is the learner's again.
	if _, err = s.Submit(ctx, cold.ID, randomToken(), "", true); err != nil {
		t.Fatal(err)
	}
	if body := get("/map", http.StatusOK); !strings.Contains(body, "GOAL-TITLE") {
		t.Fatalf("goal title stayed hidden after assistance: %s", body)
	}
}

// A finished fix pre-fills the ordinary edit form and changes nothing until
// the learner saves it; the current version stays one click away.
func TestFixDraftPrefillsTheEditForm(t *testing.T) {
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
	edit := func(query string) string {
		t.Helper()
		r := ownerRequest(http.MethodGet, "/quizzes/"+q.ID+"/edit"+query, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("edit page: %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	body := edit("")
	for _, want := range []string{">SUGGESTED which record holds an IPv4 address?</textarea>", ">Address record</textarea>", "Shorter prompt, please", "?draft=off"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the draft does not fill the form (%q): %s", want, body)
		}
	}
	current := edit("?draft=off")
	if !strings.Contains(current, ">Which record maps a hostname to an IPv4 address?</textarea>") {
		t.Fatalf("the current version is not one click away: %s", current)
	}
	// Only the form that shows the draft saves it as the draft, so its grading
	// is never installed from a form that showed the current version.
	if !strings.Contains(body, `name="from_draft"`) || strings.Contains(current, `name="from_draft"`) {
		t.Fatal("the edit form does not say which version it showed")
	}
	if current, err := s.Quiz(ctx, q.ID); err != nil || current.Version != q.Version || strings.HasPrefix(current.Prompt, "SUGGESTED") {
		t.Fatalf("the draft replaced the question before it was saved: %+v %v", current, err)
	}
	form := url.Values{"csrf": {csrf}, "operation_id": {operation}, "version": {strconv.Itoa(q.Version)}, "kind": {"recall"},
		"prompt": {"SUGGESTED which record holds an IPv4 address?"}, "answer": {"A"}, "variants": {"Address record"}, "explanation": {"SUGGESTED An A record stores one IPv4 address."}, "from_draft": {"1"}}
	r := ownerRequest(http.MethodPost, "/quizzes/"+q.ID+"/edit", form)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("saving the draft: %d %s", w.Code, w.Body.String())
	}
	if current, err := s.Quiz(ctx, q.ID); err != nil || current.Version != q.Version+1 || !strings.HasPrefix(current.Prompt, "SUGGESTED") {
		t.Fatalf("the saved draft was not installed: %+v %v", current, err)
	}
	if body := edit(""); strings.Contains(body, "?draft=off") {
		t.Fatal("a saved draft is still offered")
	}
}

// A fix that stops is reported on its question, and its capture offers no
// retry for it: re-running a fix is asking again from the question.
func TestStoppedFixIsReportedOnItsQuestion(t *testing.T) {
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
	if err = s.FailJob(ctx, job.ID, job.LeaseToken, "synthetic provider failure", false, &zero); err != nil {
		t.Fatal(err)
	}
	cookie, _, _ := bootstrapForm(t, app)
	get := func(path string) string {
		t.Helper()
		r := ownerRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := get("/quizzes/" + q.ID + "/edit"); !strings.Contains(body, "synthetic provider failure") {
		t.Fatalf("the question does not say why its fix stopped: %s", body)
	}
	if body := get("/sources/" + src.ID); strings.Contains(body, "/sources/"+src.ID+"/retry") {
		t.Fatalf("the capture offers to retry a question's fix: %s", body)
	}
}
