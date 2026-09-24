package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

type semanticReply struct {
	response semantic.Response
	err      error
}

type fakeSemanticClient struct {
	mu      sync.Mutex
	replies []semanticReply
	calls   int
	// hold, when set, blocks each Decide until released so concurrent
	// duplicate submits can be exercised while a request is in flight.
	hold chan struct{}
	// onDecide, when set, runs inside the provider call so a test can drop
	// the learner's connection while the paid request is in flight.
	onDecide func()
}

func (f *fakeSemanticClient) Decide(_ context.Context, _ semantic.Request) (semantic.Response, error) {
	f.mu.Lock()
	f.calls++
	hold := f.hold
	onDecide := f.onDecide
	var reply semanticReply
	if len(f.replies) == 0 {
		reply = semanticReply{err: semantic.ErrUnavailable}
	} else {
		reply = f.replies[0]
		f.replies = f.replies[1:]
	}
	f.mu.Unlock()
	if onDecide != nil {
		onDecide()
	}
	if hold != nil {
		<-hold
	}
	return reply.response, reply.err
}

func (f *fakeSemanticClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func privateSemanticApp(t *testing.T, fake *fakeSemanticClient) (*store.Store, http.Handler) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "scry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// The web boundary is exercised with every class enabled so the fenced
	// cue path renders; production keeps incomplete/incorrect in shadow.
	params := learning.SemanticV1Params()
	params.IncompleteEnabled, params.IncorrectEnabled = true, true
	s.SetSemanticParams(params)
	assessor := semantic.NewAssessor(s, fake, semantic.DefaultModel, semantic.Spending{ReservationMicros: 2_000, DailyBudgetMicros: 1_000_000})
	h, err := New(s, Config{
		Mode: "production", OwnerID: "owner-123", Secret: strings.Repeat("s", 32), BaseURL: "https://scry.example",
		TrustProxy: true, TrustedProxyIPs: []string{"127.0.0.1"}, Semantic: assessor,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, h
}

func semanticQuiz() store.GeneratedQuiz {
	return store.GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Prompt: "How do validators support cache reuse?",
		Answer:      "They identify a representation and let the server confirm whether it changed.",
		Explanation: "Validators identify a representation for a conditional freshness check.", Basis: "topic",
		Rubric: &store.Rubric{
			Required: []store.RubricIdea{
				{Text: "A validator identifies a representation", Cue: "Think about recognizing the stored representation."},
				{Text: "The server confirms whether the representation changed"},
			},
			Contradictions: []store.RubricClaim{{Text: "Validators require downloading every response body", Feedback: "Unchanged content can be reused without another body."}},
		},
	}
}

func semanticResponse(idea0, idea1 float64, relation string, relationProbability, contradiction float64) semantic.Response {
	injection := 0.01
	cost := int64(19)
	return semantic.Response{
		Model: "typesafe/jev-1.13-20260917", RequestID: "synthetic-decision", LatencyMS: 12,
		Raw:   []byte(`{"model":"typesafe/jev-1.13-20260917","answers":{"synthetic":true}}`),
		Usage: semantic.Usage{InputTokens: 120, OutputTokens: 15, CostMicros: &cost},
		Answers: map[string]semantic.Answer{
			"idea_0":          {Noul: float64ptr(idea0)},
			"idea_1":          {Noul: float64ptr(idea1)},
			"contradiction_0": {Noul: float64ptr(contradiction)},
			"relation":        {Choice: relation, Probabilities: map[string]float64{relation: relationProbability}},
			"injection":       {Noul: &injection},
		},
	}
}

func float64ptr(value float64) *float64 { return &value }

func seedSemanticReview(t *testing.T, s *store.Store) *store.Presentation {
	t.Helper()
	if _, err := s.Capture(context.Background(), store.CaptureInput{Text: "Synthetic semantic web topic", Mode: "topic"}, randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, semanticQuiz())
	return openReviewState(t, s)
}

func postReview(t *testing.T, app http.Handler, cookie *http.Cookie, csrf, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	values.Set("csrf", csrf)
	r := ownerRequest(http.MethodPost, path, values)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST %s: %d %s", path, w.Code, w.Body.String())
	}
	return w
}

func TestSemanticExactLocalMatchDoesNotCallClient(t *testing.T) {
	fake := &fakeSemanticClient{}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{
		"presentation_id": {current.ID}, "answer": {semanticQuiz().Answer}, "operation_id": {operation},
	})
	if fake.callCount() != 0 {
		t.Fatalf("exact semantic answer called model: %d", fake.callCount())
	}
	history, err := s.History(context.Background(), 10)
	if err != nil || len(history) != 1 || history[0].Grading != "exact-v1" {
		t.Fatalf("local semantic match history: %+v %v", history, err)
	}
}

func TestSemanticSubmitFinalizesCorrectAndReplayDoesNotCallAgain(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{response: semanticResponse(0.96, 0.94, "equivalent", 0.93, 0.02)}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	answer := "A validator names cached content so the origin can verify whether it changed."
	form := url.Values{"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}}
	postReview(t, app, cookie, csrf, "/review/answer", form)
	if fake.callCount() != 1 {
		t.Fatalf("model calls=%d want 1", fake.callCount())
	}
	postReview(t, app, cookie, csrf, "/review/answer", form)
	if fake.callCount() != 1 {
		t.Fatalf("judged replay called model again: %d", fake.callCount())
	}
	history, err := s.History(context.Background(), 10)
	if err != nil || len(history) != 1 || history[0].Grading != "semantic-v1" {
		t.Fatalf("semantic history: %+v %v", history, err)
	}
}

func TestSemanticConcurrentDuplicatePostsSendOneModelRequest(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{response: semanticResponse(0.96, 0.94, "equivalent", 0.93, 0.02)}}, hold: make(chan struct{})}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	answer := "A validator names cached content so the origin can verify whether it changed."
	form := url.Values{"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}, "csrf": {csrf}}
	post := func() *httptest.ResponseRecorder {
		r := ownerRequest(http.MethodPost, "/review/answer", form)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://scry.example")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- post() }()
	// Wait until the first request holds the lease inside the provider call.
	deadline := time.Now().Add(5 * time.Second)
	for fake.callCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("first request never reached the provider")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// The duplicate arrives while the first is in flight: same operation,
	// same answer. It must not obtain a lease or send a second request.
	duplicate := post()
	if duplicate.Code != http.StatusSeeOther && duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate POST status %d: %s", duplicate.Code, duplicate.Body.String())
	}
	if fake.callCount() != 1 {
		t.Fatalf("duplicate POST sent a second model request: %d", fake.callCount())
	}
	close(fake.hold)
	if w := <-first; w.Code != http.StatusSeeOther {
		t.Fatalf("first POST: %d %s", w.Code, w.Body.String())
	}
	// A later exact replay reconciles the judged result without resending.
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}})
	if fake.callCount() != 1 {
		t.Fatalf("replay after judgment sent a model request: %d", fake.callCount())
	}
	history, err := s.History(context.Background(), 10)
	if err != nil || len(history) != 1 || history[0].Grading != "semantic-v1" {
		t.Fatalf("duplicate submits produced wrong history: %+v %v", history, err)
	}
}

func TestSemanticJudgmentPersistsWhenLearnerDisconnectsMidRequest(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{response: semanticResponse(0.96, 0.94, "equivalent", 0.93, 0.02)}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	answer := "A validator names cached content so the origin can verify whether it changed."
	// The learner's connection drops while the paid request is in flight:
	// the request context is canceled before the judgment can be written.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onDecide = cancel
	r := ownerRequest(http.MethodPost, "/review/answer", url.Values{
		"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}, "csrf": {csrf},
	}).WithContext(ctx)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	app.ServeHTTP(httptest.NewRecorder(), r)
	if fake.callCount() != 1 {
		t.Fatalf("model calls=%d want 1", fake.callCount())
	}
	// The response to a vanished client is irrelevant; the durable truth is
	// not. The transmitted judgment must be recorded, not left as a dangling
	// lease that later reconciles to an interrupted unknown outcome.
	persisted, err := s.Current(context.Background())
	if err != nil || persisted == nil || !persisted.Graded || persisted.AssessmentStatus != "judged" {
		t.Fatalf("judgment lost after disconnect: %+v %v", persisted, err)
	}
	history, err := s.History(context.Background(), 10)
	if err != nil || len(history) != 1 || history[0].Grading != "semantic-v1" || history[0].Outcome != "correct" {
		t.Fatalf("semantic history after disconnect: %+v %v", history, err)
	}
	// An exact replay from the reconnected learner reconciles without resending.
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}})
	if fake.callCount() != 1 {
		t.Fatalf("replay after disconnect sent a model request: %d", fake.callCount())
	}
}

func TestSemanticFailureOffersHonestSelfCheck(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{err: semantic.ErrUnavailable}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	answer := "A validator helps a cache ask whether content changed."
	r := ownerRequest(http.MethodPost, "/review/answer", url.Values{"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation}, "csrf": {csrf}})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	persisted, err := s.Current(context.Background())
	if err != nil || persisted == nil || persisted.Graded || persisted.Answer != answer || !persisted.SelfCheck || persisted.SelfCheckReason != "failed" {
		t.Fatalf("failed check changed answer truth: %+v %v", persisted, err)
	}
	page := reviewPage(t, app, cookie)
	if !strings.Contains(page, semanticQuiz().Answer) || !strings.Contains(page, `action="/review/self"`) || !strings.Contains(page, `>Retry check</button>`) {
		t.Fatal("failed check did not offer answer comparison, self-grade and retry")
	}
}
