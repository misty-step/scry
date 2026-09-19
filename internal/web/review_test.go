package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func completeSyntheticQuiz(t *testing.T, s *store.Store, quiz store.GeneratedQuiz) {
	t.Helper()
	ctx := context.Background()
	job, err := s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil || job == nil {
		t.Fatalf("claim quiz job: %+v %v", job, err)
	}
	cost := int64(70)
	if err := s.CompleteJob(ctx, job.ID, job.LeaseToken, store.GenerationResult{
		Quizzes:       []store.GeneratedQuiz{quiz},
		Model:         "synthetic-authored",
		PromptVersion: "fixture-v1",
	}, &cost); err != nil {
		t.Fatal(err)
	}
}

// reviewPage renders the real browser document (not the HTMX main swap) so the
// assertions cover the page chrome around the focused review stage.
func reviewPage(t *testing.T, app http.Handler, cookie *http.Cookie) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := ownerRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	app.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("render review page: %d", w.Code)
	}
	return w.Body.String()
}

func requirePresent(t *testing.T, page, label string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Errorf("%s: missing %q", label, want)
		}
	}
}

func requireAbsent(t *testing.T, page, label string, gone ...string) {
	t.Helper()
	for _, g := range gone {
		if strings.Contains(page, g) {
			t.Errorf("%s: still contains removed chrome %q", label, g)
		}
	}
}

// reviewRemovals is the MIS-157 contract: this chrome must not appear anywhere
// on the review document (page or empty state), in any review state.
var reviewRemovals = []string{
	"Too advanced",
	"Fix or inspect",
	"Stop reviewing this question",
	"Saved foundations",
	"/foundations",
	"Flag a problem",
	"Inspect the question",
	"scheduled a review for",
	"Helped, not unaided recall",
	// Navigation punches out; it is never an always-visible bar on review.
	`class="utility-nav"`,
	`<nav class="dock"`,
}

func openReviewState(t *testing.T, s *store.Store) *store.Presentation {
	t.Helper()
	state, err := s.Review(context.Background())
	if err != nil || state.Current == nil {
		t.Fatalf("open review state: %+v %v", state, err)
	}
	return state.Current
}

func TestReviewUngradedSurfaceIsQuestionAnswerAndSingleSubmit(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "Synthetic focused-flow topic", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{
		Kind: "recall", Prompt: "Name the synthetic electron carrier.",
		Answer: "NADPH", Explanation: "NADPH carries reducing equivalents to the fixing reactions.", Basis: "topic",
	})
	current := openReviewState(t, s)
	cookie, _, _ := bootstrapForm(t, app)
	page := reviewPage(t, app, cookie)

	requirePresent(t, page, "ungraded recall surface",
		// The question dominates: it is the focused h1 of the stage.
		`<h1 class="question" tabindex="-1" data-focus>Name the synthetic electron carrier.</h1>`,
		// Exactly one answer control: the recall field with one submit.
		`id="recall-answer"`,
		`name="answer"`,
		`>Check answer</button>`,
		// Navigation punches out beside the wordmark, not as an always-visible bar.
		`<details class="menu-punchout"><summary>Menu</summary>`,
		// Reachability moved into the one overflow, not removed.
		`<details class="overflow">`,
		"I don't know yet",
		`/quizzes/`+current.Quiz.ID+`/edit`,
		`/quizzes/`+current.Quiz.ID+`/archive`,
	)
	if got := strings.Count(page, ">Check answer</button>"); got != 1 {
		t.Fatalf("expected exactly one recall submit control, got %d", got)
	}
	requireAbsent(t, page, "ungraded recall surface", reviewRemovals...)
	requireAbsent(t, page, "ungraded recall surface", `class="review-context"`)
}

func TestReviewUngradedChoiceSubmitsByTapWithoutExtraControls(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "Synthetic focused-flow choice topic", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{
		Kind: "choice", Prompt: "Pick the synthetic electron carrier.",
		Answer: "NADPH", Choices: []string{"ATP", "NADPH", "FADH2"}, Explanation: "NADPH is the reducing equivalent.", Basis: "topic",
	})
	cookie, _, _ := bootstrapForm(t, app)
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "ungraded choice surface",
		`>Pick the synthetic electron carrier.</h1>`,
		`<button class="choice" type="submit" name="answer" value="ATP"`,
		`<button class="choice" type="submit" name="answer" value="NADPH"`,
	)
	// Tapping a choice submits; there is no separate submit or recall field.
	requireAbsent(t, page, "ungraded choice surface",
		`id="recall-answer"`, ">Check answer</button>")
	requireAbsent(t, page, "ungraded choice surface", reviewRemovals...)
}

// TestReviewNavigationIsOnePunchOut pins the menu contract: the five
// destinations (Review, Add, Library, History, Settings) live behind one or
// two punch-out controls, and the review document never ships the
// always-visible dock plus utility-nav combo.
func TestReviewNavigationIsOnePunchOut(t *testing.T) {
	_, app := privateApp(t)
	cookie, _, _ := bootstrapForm(t, app)
	page := reviewPage(t, app, cookie)
	// The always-visible combo is gone from the review document.
	requireAbsent(t, page, "review nav", `<nav class="dock"`, `class="utility-nav"`)
	// One or two punch-out controls, not zero.
	if count := strings.Count(page, `<details class="menu-punchout">`); count < 1 || count > 2 {
		t.Fatalf("menu punch-out controls = %d, want 1 or 2", count)
	}
	// All five destinations are inside the punch-out nav.
	requirePresent(t, page, "review nav",
		`<nav aria-label="Main menu">`,
		`href="/"`, `>Review</a>`,
		`href="/add"`, `>Add</a>`,
		`href="/library"`, `>Library</a>`,
		`href="/history"`, `>History</a>`,
		`href="/settings"`, `>Settings</a>`,
	)
}

func TestReviewResultShowsOutcomeAndNextOnly(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "Synthetic focused-flow graded topic", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{
		Kind: "choice", Prompt: "Pick the graded carrier.",
		Answer: "NADPH", Choices: []string{"ATP", "NADPH", "FADH2"}, Explanation: "NADPH carries reducing equivalents.", Basis: "topic",
	})
	current := openReviewState(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	r := ownerRequest(http.MethodPost, "/review/answer", url.Values{
		"presentation_id": {current.ID},
		"answer":          {"NADPH"},
		"csrf":            {csrf},
		"operation_id":    {operation},
	})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("submit answer: %d %s", w.Code, w.Body.String())
	}

	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "graded surface",
		`>Pick the graded carrier.</h1>`,
		`>Correct</h2>`,
		`data-next`,
		`>Next</button>`,
		// Overflow keeps edit/archive reachable; reveal is gone once graded.
		`<details class="overflow">`,
		`/quizzes/`+current.Quiz.ID+`/edit`,
		`/quizzes/`+current.Quiz.ID+`/archive`,
	)
	requireAbsent(t, page, "graded surface", append(reviewRemovals, "FSRS", "I don't know yet")...)
}

func TestReviewRevealRecordsHelpedResultWithNextOnly(t *testing.T) {
	s, app := privateApp(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "Synthetic focused-flow reveal topic", randomToken()); err != nil {
		t.Fatal(err)
	}
	completeSyntheticQuiz(t, s, store.GeneratedQuiz{
		Kind: "recall", Prompt: "Name the revealed carrier.", Answer: "NADPH", Explanation: "NADPH carries reducing equivalents.", Basis: "topic",
	})
	current := openReviewState(t, s)
	cookie, csrf, operation := bootstrapForm(t, app)
	r := ownerRequest(http.MethodPost, "/review/reveal", url.Values{
		"presentation_id": {current.ID},
		"csrf":            {csrf},
		"operation_id":    {operation},
	})
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://scry.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("reveal: %d %s", w.Code, w.Body.String())
	}

	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "revealed surface",
		`>Name the revealed carrier.</h1>`,
		`>Answer revealed</h2>`,
		`>NADPH</p>`,
		`>Next</button>`,
	)
	requireAbsent(t, page, "revealed surface", reviewRemovals...)
}

func TestReviewEmptyStateHasNoFoundationChrome(t *testing.T) {
	_, app := privateApp(t)
	cookie, _, _ := bootstrapForm(t, app)
	page := reviewPage(t, app, cookie)
	requirePresent(t, page, "empty review",
		`>No active questions.</h1>`,
		`<a class="button primary" href="/add">Add something</a>`,
	)
	requireAbsent(t, page, "empty review", reviewRemovals...)
}
