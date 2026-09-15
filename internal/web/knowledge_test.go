package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

type knowledgeFixture struct {
	store           *store.Store
	app             http.Handler
	cookie          *http.Cookie
	csrf            string
	source          store.Source
	referenceSource store.Source
	goal            store.Goal
	unit            store.KnowledgeUnit
	material        store.Material
	current         store.Presentation
}

const knowledgeAnswer = "expected-secret-marker"
const knowledgeStatement = "Definition canary: the synthetic marker is expected-secret-marker."
const knowledgeBody = "Instruction body canary. <script>privateLearning()</script> The synthetic marker is expected-secret-marker."

func newKnowledgeFixture(t *testing.T) knowledgeFixture {
	t.Helper()
	s, app := privateApp(t)
	ctx := context.Background()
	source, err := s.Capture(ctx, "Synthetic assessment goal", randomToken())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil {
		t.Fatalf("claim assessment fixture: %v", err)
	}
	cost := int64(70)
	if err := s.CompleteJob(ctx, claim.ID, claim.LeaseToken, store.GenerationResult{
		Coverage: store.CoverageReport{Kind: "concepts", Complete: true},
		Units:    []store.GeneratedUnit{{Key: "marker", Statement: knowledgeStatement, Kind: "foundation"}},
		Quizzes:  []store.GeneratedQuiz{{Key: "probe", Kind: "recall", Prompt: "What is the synthetic marker?", Answer: knowledgeAnswer, Explanation: "Private assessment explanation canary.", Basis: "topic", Level: "target", EstimatedSeconds: 30, Links: []store.GeneratedLink{{UnitKey: "marker", Role: "assesses"}}}},
		Model:    "authored-web-boundary-fixture", PromptVersion: "fixture-v2",
	}, &cost); err != nil {
		t.Fatal(err)
	}
	goal, err := s.Goal(ctx, source.GoalID)
	if err != nil || len(goal.Units) != 1 {
		t.Fatalf("fixture represented unit: %+v %v", goal, err)
	}
	unit := goal.Units[0]

	// The teaching reference deliberately belongs to another source. A guard
	// that compares only source IDs cannot protect this shared knowledge.
	referenceSource, err := s.Capture(ctx, "Separate foundation reference goal", randomToken())
	if err != nil {
		t.Fatal(err)
	}
	claim, err = s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil {
		t.Fatalf("claim reference fixture: %v", err)
	}
	if err := s.CompleteJob(ctx, claim.ID, claim.LeaseToken, store.GenerationResult{
		Coverage:  store.CoverageReport{Kind: "concepts", Complete: true},
		Units:     []store.GeneratedUnit{{Key: "marker", ReuseID: unit.ID, Statement: unit.Statement, Kind: unit.Kind}},
		Materials: []store.GeneratedMaterial{{Key: "instruction", Kind: "explanation", Title: "A reusable foundation reference", Body: "Generated background: " + knowledgeBody, Basis: "background", EstimatedSeconds: 45, Links: []store.GeneratedLink{{UnitKey: "marker", Role: "teaches"}}}},
		Model:     "authored-web-boundary-fixture", PromptVersion: "fixture-v2",
	}, &cost); err != nil {
		t.Fatal(err)
	}
	referenceSource, err = s.Source(ctx, referenceSource.ID)
	if err != nil || len(referenceSource.Materials) != 1 {
		t.Fatalf("fixture reference: %+v %v", referenceSource, err)
	}
	goal, err = s.PlanGoal(ctx, goal.ID, store.PlanSettings{ExpectedRevision: goal.Revision, TimeBudgetSeconds: 300, NewAssessmentsPerDay: 5, Focus: "practice", Reason: "Choose assessment before inspection for this authored boundary scenario"}, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "quiz" || state.Current.Practice {
		t.Fatalf("fixture cold assessment: %+v %v", state, err)
	}
	cookie, csrf, _ := bootstrapForm(t, app)
	return knowledgeFixture{store: s, app: app, cookie: cookie, csrf: csrf, source: source, referenceSource: referenceSource, goal: goal, unit: unit, material: referenceSource.Materials[0], current: *state.Current}
}

func (f knowledgeFixture) request(t *testing.T, method, path, accept string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	if values != nil {
		values.Set("csrf", f.csrf)
		if values.Get("operation_id") == "" {
			values.Set("operation_id", randomToken())
		}
	}
	r := ownerRequest(method, path, values)
	r.AddCookie(f.cookie)
	r.Header.Set("Accept", accept)
	if method != http.MethodGet {
		r.Header.Set("Origin", "https://scry.example")
	}
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	return w
}

func (f knowledgeFixture) inspect(t *testing.T, kind, id, destination, operation string) *httptest.ResponseRecorder {
	t.Helper()
	return f.request(t, http.MethodPost, "/inspection/assist", "application/json", url.Values{"kind": {kind}, "id": {id}, "return_to": {destination}, "operation_id": {operation}})
}

func TestKnowledgeMutationsStayBehindOwnerAndCSRF(t *testing.T) {
	_, app := privateApp(t)
	cookie, _, _ := bootstrapForm(t, app)
	paths := []string{"/review/bridge", "/review/return", "/review/continue", "/review/assisted", "/inspection/assist", "/goals/missing/plan", "/suggestions/missing/choose", "/plans/missing/undo", "/units/missing/edit", "/materials/missing/edit", "/materials/missing/coverage", "/relations/edit", "/jobs/missing/retry"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for _, owner := range []bool{false, true} {
				r := ownerRequest(http.MethodPost, path, url.Values{"operation_id": {randomToken()}})
				r.Header.Set("Origin", "https://scry.example")
				r.Header.Set("Accept", "application/json")
				r.AddCookie(cookie)
				if !owner {
					r.Header.Set("X-ExeDev-UserID", "other-owner")
				}
				w := httptest.NewRecorder()
				app.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden {
					t.Fatalf("owner=%t unsafe mutation status %d", owner, w.Code)
				}
			}
		})
	}
}

func TestSharedKnowledgeInspectionIsExplicitAndLibraryDoesNotAuthorizeBodies(t *testing.T) {
	f := newKnowledgeFixture(t)
	ctx := context.Background()
	for _, path := range []string{"/sources/" + f.referenceSource.ID, "/goals/" + f.referenceSource.GoalID, "/units/" + f.unit.ID, "/materials/" + f.material.ID, "/library", "/export"} {
		for _, accept := range []string{"text/html", "application/json"} {
			w := f.request(t, http.MethodGet, path, accept, nil)
			want := http.StatusOK
			if accept == "application/json" {
				want = http.StatusConflict
			}
			if w.Code != want {
				t.Fatalf("guard %s %s: %d", accept, path, w.Code)
			}
			if strings.Contains(w.Body.String(), knowledgeBody) || strings.Contains(w.Body.String(), knowledgeStatement) || strings.Contains(w.Body.String(), knowledgeAnswer) {
				t.Fatalf("guard disclosed shared knowledge through %s", path)
			}
		}
	}
	current, err := f.store.Current(ctx)
	if err != nil || current == nil || current.Assisted || current.Practice || current.Graded {
		t.Fatalf("inspection GET invented help: %+v %v", current, err)
	}
	if w := f.inspect(t, "library", "", "/library", randomToken()); w.Code != http.StatusOK {
		t.Fatalf("open metadata: %d", w.Code)
	}
	w := f.request(t, http.MethodGet, "/library", "application/json", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), f.material.Title) {
		t.Fatalf("library lost readable material title: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Instruction body canary") || strings.Contains(w.Body.String(), knowledgeStatement) {
		t.Fatal("library scope serialized answer bodies or unit statements")
	}
	if w := f.request(t, http.MethodGet, "/materials/"+f.material.ID, "application/json", nil); w.Code != http.StatusConflict {
		t.Fatalf("library scope opened a material body: %d", w.Code)
	}

	operation := randomToken()
	before, err := f.store.Interactions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if w := f.inspect(t, "material", f.material.ID, "/materials/"+f.material.ID, operation); w.Code != http.StatusOK {
			t.Fatalf("explicit/replayed opening: %d", w.Code)
		}
	}
	after, err := f.store.Interactions(ctx, 100)
	if err != nil || len(after) != len(before)+1 {
		t.Fatalf("inspection replay was not one durable interaction: %d -> %d, %v", len(before), len(after), err)
	}
	current, err = f.store.Current(ctx)
	if err != nil || current == nil || !current.Assisted || !current.Practice || current.Graded {
		t.Fatalf("shared inspection did not preserve ungraded helped practice: %+v %v", current, err)
	}
	reviews, err := f.store.History(ctx, 100)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("inspection manufactured direct reviews: %+v %v", reviews, err)
	}
	w = f.request(t, http.MethodGet, "/materials/"+f.material.ID, "text/html", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Instruction body canary") || !strings.Contains(w.Body.String(), "&lt;script&gt;") || strings.Contains(w.Body.String(), "<script>privateLearning()") {
		t.Fatalf("explicit reference was missing or executable: %d", w.Code)
	}

	var first, second struct {
		Access store.InspectionAccess `json:"access"`
	}
	for i, value := range []any{&first, &second} {
		w := f.request(t, http.MethodGet, "/session?kind=material&id="+url.QueryEscape(f.material.ID), "application/json", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("scope read %d: %d", i, w.Code)
		}
		if err := json.Unmarshal(w.Body.Bytes(), value); err != nil {
			t.Fatal(err)
		}
	}
	if !first.Access.Allowed || first.Access.ExpiresAt <= first.Access.CheckedAt || first.Access.ExpiresAt != second.Access.ExpiresAt {
		t.Fatal("scope status failed to expose a fixed server-time expiry")
	}
	final, err := f.store.Interactions(ctx, 100)
	if err != nil || len(final) != len(after) {
		t.Fatal("scope or material GET manufactured visibility/reading telemetry")
	}
}

func TestReferenceBridgeContinuationAndReturnDoNotCreateRetrievalGrades(t *testing.T) {
	f := newKnowledgeFixture(t)
	ctx := context.Background()
	w := f.request(t, http.MethodPost, "/review/bridge", "application/json", url.Values{"presentation_id": {f.current.ID}})
	if w.Code != http.StatusOK {
		t.Fatalf("bridge request: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Review store.ReviewState `json:"review"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Review.Bridge == nil || response.Review.Bridge.TargetPresentationID != f.current.ID || response.Review.Current == nil || response.Review.Current.Kind != "reference" {
		t.Fatalf("bridge did not retain target and reuse instruction: %+v", response.Review)
	}
	referenceID, bridgeID := response.Review.Current.ID, response.Review.Bridge.ID
	operation := randomToken()
	for range 2 {
		w := f.request(t, http.MethodPost, "/review/continue", "application/json", url.Values{"presentation_id": {referenceID}, "operation_id": {operation}})
		if w.Code != http.StatusOK {
			t.Fatalf("reference continuation/replay: %d", w.Code)
		}
	}
	interactions, err := f.store.Interactions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, interaction := range interactions {
		if interaction.PresentationID == referenceID {
			count++
			if interaction.ReviewID != "" {
				t.Fatal("reference continuation acquired a direct review identity")
			}
		}
	}
	if count != 1 {
		t.Fatalf("reference continuation recorded %d interactions", count)
	}
	if w := f.inspect(t, "history", "", "/history", randomToken()); w.Code != http.StatusOK {
		t.Fatalf("open actual mixed timeline: %d", w.Code)
	}
	w = f.request(t, http.MethodGet, "/history", "text/html", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), f.material.Title) || !strings.Contains(w.Body.String(), "Instruction body canary") || strings.Contains(w.Body.String(), "<script>privateLearning()") {
		t.Fatalf("inspected history lost or activated the actual continued reference snapshot: %d", w.Code)
	}
	w = f.request(t, http.MethodPost, "/review/return", "application/json", url.Values{"bridge_id": {bridgeID}})
	if w.Code != http.StatusOK {
		t.Fatalf("return to retained target: %d", w.Code)
	}
	current, err := f.store.Current(ctx)
	if err != nil || current == nil || current.ID != f.current.ID || !current.Assisted || !current.Practice || current.Graded {
		t.Fatalf("return lost target or fabricated a grade: %+v %v", current, err)
	}
	reviews, err := f.store.History(ctx, 100)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("foundation path manufactured direct review history: %+v %v", reviews, err)
	}
}

func TestUnitCorrectionUsesSubmittedVersionAndRetainsRejectedDraft(t *testing.T) {
	f := newKnowledgeFixture(t)
	ctx := context.Background()
	if w := f.inspect(t, "unit", f.unit.ID, "/units/"+f.unit.ID, randomToken()); w.Code != http.StatusOK {
		t.Fatalf("open unit: %d", w.Code)
	}
	operation := randomToken()
	updatedStatement := "The synthetic marker has the authored name expected-secret-marker."
	values := url.Values{"version": {strconv.Itoa(f.unit.Version)}, "statement": {updatedStatement}, "kind": {"foundation"}, "reason": {"Clarify the exact synthetic naming contract"}, "operation_id": {operation}}
	for range 2 {
		w := f.request(t, http.MethodPost, "/units/"+f.unit.ID+"/edit", "application/json", values)
		if w.Code != http.StatusOK {
			t.Fatalf("unit correction/replay: %d", w.Code)
		}
	}
	detail, err := f.store.Unit(ctx, f.unit.ID, "recall", time.Time{})
	if err != nil || detail.Unit.Version != f.unit.Version+1 || detail.Unit.Statement != updatedStatement || len(detail.Corrections) != 1 {
		t.Fatalf("unit correction lost immutable versions or replayed twice: %+v %v", detail, err)
	}
	foundOriginal := false
	for _, version := range detail.Versions {
		if version.Version == f.unit.Version && version.Statement == knowledgeStatement {
			foundOriginal = true
		}
	}
	if !foundOriginal {
		t.Fatal("definition correction rewrote the encountered version")
	}
	if w := f.inspect(t, "unit", f.unit.ID, "/units/"+f.unit.ID, randomToken()); w.Code != http.StatusOK {
		t.Fatalf("open corrected definition: %d", w.Code)
	}
	values.Set("operation_id", randomToken())
	if w := f.request(t, http.MethodPost, "/units/"+f.unit.ID+"/edit", "application/json", values); w.Code != http.StatusConflict {
		t.Fatalf("stale submitted definition version was accepted: %d", w.Code)
	}
	const draft = "Rejected draft </textarea><script>doNotRun()</script>"
	values.Set("version", strconv.Itoa(detail.Unit.Version))
	values.Set("statement", draft)
	values.Set("kind", "unsupported-kind")
	values.Set("operation_id", randomToken())
	w := f.request(t, http.MethodPost, "/units/"+f.unit.ID+"/edit", "text/html", values)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Rejected draft &lt;/textarea&gt;") || strings.Contains(w.Body.String(), "<script>doNotRun()") {
		t.Fatalf("rejected correction lost or activated draft: %d", w.Code)
	}
}

func TestReferencePreviewDoesNotSerializeInstructionOrNestedCoverage(t *testing.T) {
	material := &store.Material{ID: "reference", Kind: "diagram", Title: "private title", Body: knowledgeBody, Evidence: "private quote", ReferenceURL: "https://example.test/private-reference", Diagram: &store.Diagram{Caption: "private caption", Nodes: []store.DiagramNode{{ID: "a", Label: knowledgeAnswer}}}, Provenance: store.Provenance{Evidence: knowledgeStatement}}
	state := privateReview(store.ReviewState{Current: &store.Presentation{ID: "quiz", Kind: "quiz", Material: material, Quiz: store.Quiz{Prompt: "Try the marker", Answer: knowledgeAnswer}}, Preview: &store.Presentation{ID: "not-committed", Kind: "reference", Material: material}, Suspended: []*store.Presentation{{ID: "target", Kind: "quiz", Material: material, Quiz: store.Quiz{Prompt: knowledgeStatement, Answer: knowledgeAnswer}}}})
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{knowledgeAnswer, "private title", "private quote", "private-reference", "private caption", "Instruction body canary", "not-committed"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("speculative or suspended transport disclosed %q", secret)
		}
	}
	if material.Body != knowledgeBody || material.Provenance.Evidence != knowledgeStatement {
		t.Fatal("transport redaction mutated the stored snapshot")
	}
	graded := privateReview(store.ReviewState{Current: &store.Presentation{ID: "graded", Kind: "quiz", Graded: true, Material: material, Quiz: store.Quiz{Prompt: "Try the marker", Answer: "acknowledged answer"}}})
	raw, err = json.Marshal(graded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "acknowledged answer") || strings.Contains(string(raw), "Instruction body canary") || strings.Contains(string(raw), knowledgeStatement) {
		t.Fatal("held feedback either lost its acknowledged answer or opened unrelated material content")
	}
	reference := privateReview(store.ReviewState{Current: &store.Presentation{ID: "committed-reference", Kind: "reference", Material: material}})
	raw, err = json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Instruction body canary") || strings.Contains(string(raw), knowledgeStatement) {
		t.Fatal("committed reference did not separate its shown content from richer material provenance")
	}
}

func TestInspectionReturnAndReferencesCannotEscapeTheirBoundary(t *testing.T) {
	for _, path := range []string{"/materials/id", "/units/id", "/goals/id/plan", "/library?q=cell+energy", "/export"} {
		if safeReturn(path) != path {
			t.Fatalf("legitimate inspection return rejected: %s", path)
		}
	}
	for _, path := range []string{"//attacker.example/materials/id", "/materials/../export", "/materials/%2e%2e", "/library?q=a&q=b", "/library?redirect=https://attacker.example", "/materials/id?next=//attacker.example", "/goals/id/delete", "https://scry.example/materials/id"} {
		if safeReturn(path) != "" {
			t.Fatalf("unsafe inspection destination accepted: %s", path)
		}
	}
	for _, reference := range []string{"javascript:alert(1)", "http://example.test/video", "https://name:password@example.test/video", "//example.test/video", "https://example.test/a\nHeader:value"} {
		if safeReference(reference) != "" {
			t.Fatalf("unsafe outbound reference accepted: %s", reference)
		}
	}
	if safeReference("https://example.test/video?t=20") == "" {
		t.Fatal("real credential-free HTTPS reference was hidden")
	}
}
