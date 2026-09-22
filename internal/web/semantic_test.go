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
}

func (f *fakeSemanticClient) Decide(_ context.Context, _ semantic.Request) (semantic.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.replies) == 0 {
		return semantic.Response{}, semantic.ErrUnavailable
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
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
	assessor := semantic.NewAssessor(s, fake, semantic.DefaultModel)
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
	if _, err := s.Capture(context.Background(), "Synthetic semantic web topic", randomToken()); err != nil {
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
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "semantic exact local", ">Correct</h2>", semanticQuiz().Answer, ">Next</button>")
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
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "semantic correct", ">Correct</h2>", answer, semanticQuiz().Answer, ">Next</button>")
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

func TestSemanticIncompleteShowsCueAfterDurableAssistanceFence(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{response: semanticResponse(0.10, 0.95, "partial", 0.91, 0.02)}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{
		"presentation_id": {current.ID}, "answer": {"The server can confirm whether a cached response changed."}, "operation_id": {operation},
	})
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "semantic incomplete", ">Almost</h2>", "Think about recognizing the stored representation.", "This counts as help.")
	persisted, err := s.Current(context.Background())
	if err != nil || persisted == nil || !persisted.Assisted || persisted.Graded {
		t.Fatalf("cue rendered without durable assistance: %+v %v", persisted, err)
	}
}

func TestSemanticFailureKeepsAnswerAndOffersRetryAndReveal(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{err: semantic.ErrUnavailable}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	answer := "A validator helps a cache ask whether content changed."
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{
		"presentation_id": {current.ID}, "answer": {answer}, "operation_id": {operation},
	})
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "semantic failed", ">Could not check meaning</h2>", "Your answer is saved.", answer, ">Retry meaning check</button>", ">Reveal answer</button>")
	persisted, err := s.Current(context.Background())
	if err != nil || persisted == nil || persisted.Graded || persisted.Answer != answer || persisted.AssessmentStatus != "failed" {
		t.Fatalf("failed assessment changed answer truth: %+v %v", persisted, err)
	}
}

func TestSemanticRevealAfterCueRetainsAssistance(t *testing.T) {
	fake := &fakeSemanticClient{replies: []semanticReply{{response: semanticResponse(0.10, 0.95, "partial", 0.90, 0.01)}}}
	s, app := privateSemanticApp(t, fake)
	current := seedSemanticReview(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	postReview(t, app, cookie, csrf, "/review/answer", url.Values{
		"presentation_id": {current.ID}, "answer": {"The origin checks whether it changed."}, "operation_id": {operation},
	})
	if persisted, err := s.Current(context.Background()); err != nil || persisted == nil || !persisted.Assisted {
		t.Fatalf("cue assistance did not persist before reveal: %+v %v", persisted, err)
	}
	postReview(t, app, cookie, csrf, "/review/reveal", url.Values{
		"presentation_id": {current.ID}, "operation_id": {randomToken()},
	})
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "semantic reveal after cue", ">Answer revealed</h2>", semanticQuiz().Answer, ">Next</button>")
	history, err := s.History(context.Background(), 10)
	if err != nil || len(history) != 1 || !history[0].Assisted || history[0].Outcome != "revealed" {
		t.Fatalf("reveal after cue lost assistance: %+v %v", history, err)
	}
}
