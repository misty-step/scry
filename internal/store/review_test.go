package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openAt(t *testing.T, path string, now *time.Time) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.clock = func() time.Time { return *now }
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	return openAt(t, filepath.Join(t.TempDir(), "personal.db"), &now), &now
}

func authoredChoice(prompt string) GeneratedQuiz {
	return GeneratedQuiz{Kind: "choice", Prompt: prompt, Answer: "second", Choices: []string{"first", "second", "third"}, Explanation: "The second option is the authored answer in this synthetic fixture.", Basis: "topic"}
}

// captureLegacy saves a topic capture whose first job is the legacy
// single-call "quizzes" kind, so fixtures publish unmapped questions exactly
// as before v5. Concept-chain behavior has its own tests.
func captureLegacy(t *testing.T, s *Store, text, operationID string) Source {
	t.Helper()
	ctx := context.Background()
	src, err := s.Capture(ctx, CaptureInput{Text: text, Mode: "topic"}, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE jobs SET kind='quizzes' WHERE source_id=? AND kind='research' AND status='queued'", src.ID); err != nil {
		t.Fatal(err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// legacyCapture is captureLegacy with Capture's signature. Material with a
// newline or at least 280 bytes is saved as the learner's text; shorter input
// as a topic, matching the pre-v5 fixtures these tests were written against.
func legacyCapture(ctx context.Context, s *Store, text, operationID string) (Source, error) {
	mode := "topic"
	if len(text) >= 280 || strings.Contains(text, "\n") {
		mode = "text"
	}
	src, err := s.Capture(ctx, CaptureInput{Text: text, Mode: mode}, operationID)
	if err != nil {
		return src, err
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE jobs SET kind='quizzes' WHERE source_id=? AND kind IN ('research','plan') AND status='queued'", src.ID); err != nil {
		return src, err
	}
	return s.Source(ctx, src.ID)
}

func publishFixture(t *testing.T, s *Store, content ...GeneratedQuiz) Source {
	t.Helper()
	ctx := context.Background()
	src := captureLegacy(t, s, "Synthetic fixture subject", newID())
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil {
		t.Fatalf("claim fixture: %v, %v", claim, err)
	}
	cost := int64(70)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, GenerationResult{Quizzes: content, Model: "authored-test-fixture", PromptVersion: "fixture-v1"}, &cost); err != nil {
		t.Fatal(err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestLostResponseResumeAndStaleNext(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredChoice("First prompt?"), authoredChoice("Second prompt?"))
	current, err := s.Current(ctx)
	if err != nil || current != nil {
		t.Fatalf("library-only Current invented an occurrence: %v %v", current, err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current == nil || state.Preview == nil {
		t.Fatalf("missing current/preview: %+v", state)
	}
	if state.Current.Quiz.Prompt != "First prompt?" || state.Preview.Quiz.Prompt != "Second prompt?" {
		t.Fatal("publication sequence was lost")
	}
	if state.Current.Quiz.Answer != "" || state.Preview.Quiz.Answer != "" || state.Preview.Quiz.Explanation != "" || state.Preview.ID != "" {
		t.Fatal("unassisted current/preview exposed answer or mutation authority")
	}
	id := state.Current.ID
	if _, err = s.Submit(ctx, id, "bad-choice", "1", false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("choice index accepted instead of exact option: %v", err)
	}
	receipt, err := s.Submit(ctx, id, "answer-once", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != "correct" || receipt.Assisted || receipt.Rating != 3 {
		t.Fatalf("unexpected feedback: %+v", receipt)
	}
	path := s.Path()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path, now)
	retried, err := s.Submit(ctx, id, "answer-once", "second", false)
	if err != nil || !reflect.DeepEqual(receipt, retried) {
		t.Fatalf("lost response retry changed receipt: %+v %v", retried, err)
	}
	if _, err = s.Submit(ctx, id, "answer-once", "first", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed operation payload accepted: %v", err)
	}
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.ReviewID != receipt.ReviewID {
		t.Fatalf("held feedback lost across restart: %+v %v", state, err)
	}
	next, err := s.Next(ctx, id)
	if err != nil || next.Current == nil || next.Current.ID == id {
		t.Fatalf("Next did not advance: %+v %v", next, err)
	}
	repeated, err := s.Next(ctx, id)
	if err != nil || repeated.Current == nil || repeated.Current.ID != next.Current.ID {
		t.Fatalf("repeated Next consumed unanswered work: %+v %v", repeated, err)
	}
	second, err := s.Submit(ctx, next.Current.ID, "second-answer", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err = s.Next(ctx, id)
	if err != nil || repeated.Current == nil || repeated.Current.ReviewID != second.ReviewID {
		t.Fatal("old Next consumed a newer held result")
	}
	history, err := s.History(ctx, 20)
	if err != nil || len(history) != 2 {
		t.Fatalf("duplicate event after retry: %d %v", len(history), err)
	}
}

func TestRevealFence(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	recall := GeneratedQuiz{Kind: "recall", Prompt: "Name the synthetic process.", Answer: "respiration", Explanation: "This is an authored fixture, not a factual learning claim.", Basis: "topic"}
	publishFixture(t, s, recall)
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Current.ID
	revealed, err := s.Submit(ctx, id, "reveal-once", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !revealed.Graded || !revealed.Assisted || revealed.Rating != 1 || revealed.Outcome != "revealed" || revealed.Quiz.Answer != recall.Answer || revealed.Authority != "reveal" {
		t.Fatalf("reveal lacked durable assistance: %+v", revealed)
	}
	if _, err = s.Submit(ctx, id, "stale-correct", recall.Answer, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("post-reveal cold success accepted: %v", err)
	}
	duplicate, err := s.Submit(ctx, id, "reveal-once", "", true)
	if err != nil || !reflect.DeepEqual(revealed, duplicate) {
		t.Fatal("reveal retry changed result")
	}
	if _, err = s.OverrideGrade(ctx, id, "override-reveal", true); !errors.Is(err, ErrConflict) {
		t.Fatalf("a revealed answer was overridden into a success: %v", err)
	}
}

// US-007/US-003: an answer that does not match the key is staged for the
// meaning check, never graded by similarity. When that check cannot run, the
// key is shown and the learner decides; after the key is visible no new answer
// is taken, and the learner's grade is recorded under learner authority.
func TestSelfCheckAfterUnclearExactAnswerUS007(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	recall := GeneratedQuiz{Kind: "recall", Prompt: "Explain the synthetic process.", Answer: "A process that releases stored chemical energy for cellular work", Explanation: "This is an authored fixture, not a factual learning claim.", Basis: "topic"}
	publishFixture(t, s, recall)
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Current.ID
	staged, err := s.Submit(ctx, id, "unclear", "Cells convert the energy they have stored into usable work", false)
	if err != nil || staged.Graded || !staged.Pending || staged.Quiz.Answer != "" {
		t.Fatalf("an unmatched exact-form answer was not staged for the meaning check: %+v %v", staged, err)
	}
	lease := beginSemantic(t, s, staged.AssessmentID)
	unclear, err := s.FailAssessment(ctx, staged.AssessmentID, lease.Token, AssessmentResult{Error: "unconfigured", NoSend: true})
	if err != nil || unclear.Graded || !unclear.SelfCheck || unclear.SelfCheckReason != "failed" || unclear.Quiz.Answer != recall.Answer {
		t.Fatalf("an unavailable check did not become a self-check with the key visible: %+v %v", unclear, err)
	}
	state, err = s.Next(ctx, id)
	if err != nil || state.Current == nil || state.Current.ID != id || !state.Current.SelfCheck {
		t.Fatal("an ungraded self-check advanced the occurrence")
	}
	if _, err = s.Submit(ctx, id, "after-key", recall.Answer, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("a new answer was accepted after the key was shown: %v", err)
	}
	graded, err := s.SelfGrade(ctx, id, "self-miss", false)
	if err != nil || !graded.Graded || graded.Outcome != "self_missed" || graded.Rating != 1 || graded.Authority != "learner" {
		t.Fatalf("self-grade was not recorded as learner authority: %+v %v", graded, err)
	}
	replayed, err := s.SelfGrade(ctx, id, "self-miss", false)
	if err != nil || !reflect.DeepEqual(graded, replayed) {
		t.Fatal("self-grade retry changed its result")
	}
	if _, err = s.SelfGrade(ctx, id, "self-second", true); !errors.Is(err, ErrConflict) {
		t.Fatalf("a graded occurrence was self-graded twice: %v", err)
	}
	// The durable record keeps both immutable events; History shows the
	// occurrence once, with its final grade and who decided it.
	var outcomes []string
	rows, err := s.db.QueryContext(ctx, "SELECT outcome||':'||rating||':'||grading FROM review_events ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var o string
		if err = rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, o)
	}
	rows.Close()
	if !reflect.DeepEqual(outcomes, []string{"self_missed:1:learner-v1"}) {
		t.Fatalf("self-check did not record exactly the learner's grade: %v", outcomes)
	}
	history, err := s.History(ctx, 20)
	if err != nil || len(history) != 1 || history[0].Rating != 1 || history[0].Authority != "learner" {
		t.Fatalf("history listed the superseded held attempt: %+v %v", history, err)
	}
	summary, err := s.Summary(ctx)
	if err != nil || summary.Reviews != 1 {
		t.Fatalf("recorded reviews counted the held attempt: %+v %v", summary, err)
	}
}

func TestEditArchiveAndDisputePreserveEvidence(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	original := authoredChoice("Original wording?")
	src := publishFixture(t, s, original)
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staleID := state.Current.ID
	edited := authoredChoice("Corrected wording?")
	updated, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, edited)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Submit(ctx, staleID, "stale-edit-answer", "second", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("superseded content accepted an answer: %v", err)
	}
	if _, err = s.EditQuiz(ctx, updated.ID, 1, original); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit overwrote a newer version: %v", err)
	}
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Submit(ctx, state.Current.ID, "corrected-answer", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditQuiz(ctx, updated.ID, updated.Version, authoredChoice("Future wording?")); err != nil {
		t.Fatal(err)
	}
	if err = s.Dispute(ctx, receipt.ReviewID, "The question was faulty", true); err != nil {
		t.Fatal(err)
	}
	if err = s.Dispute(ctx, receipt.ReviewID, "The question was faulty", true); err != nil {
		t.Fatal(err)
	}
	if err = s.ArchiveSource(ctx, src.ID); err != nil {
		t.Fatal(err)
	}
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Quiz.Prompt != edited.Prompt || !state.Current.Disputed {
		t.Fatalf("held historical wording/correction lost: %+v %v", state, err)
	}
	state, err = s.Next(ctx, receipt.ID)
	if err != nil || state.Current != nil || state.Total != 0 {
		t.Fatalf("archived material remained selectable: %+v %v", state, err)
	}
	history, err := s.History(ctx, 20)
	if err != nil || len(history) != 1 || history[0].Quiz.Version != 2 || history[0].Quiz.Prompt != edited.Prompt || !history[0].Disputed || history[0].Outcome != "correct" {
		t.Fatalf("content lifecycle fabricated or rewrote recall evidence: %+v %v", history, err)
	}
	data, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var exported struct {
		Versions    []json.RawMessage `json:"quiz_versions"`
		Corrections []json.RawMessage `json:"corrections"`
		Sources     []Source          `json:"sources"`
	}
	if err = json.Unmarshal(data, &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported.Versions) != 3 || len(exported.Corrections) != 1 || len(exported.Sources) != 1 {
		t.Fatal("portable export lost versions or duplicated schedule correction")
	}
	current, err := s.Current(ctx)
	if err != nil || current != nil {
		t.Fatal("export invented another review occurrence")
	}
}

func TestCompetingConnectionsCommitOnlyOneRecall(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredChoice("Competing tab question?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other := openAt(t, s.Path(), now)
	start := make(chan struct{})
	type outcome struct {
		receipt Presentation
		err     error
	}
	outcomes := make(chan outcome, 2)
	go func() {
		<-start
		p, err := s.Submit(ctx, state.Current.ID, "race-reveal", "", true)
		outcomes <- outcome{p, err}
	}()
	go func() {
		<-start
		p, err := other.Submit(ctx, state.Current.ID, "race-answer", "second", false)
		outcomes <- outcome{p, err}
	}()
	close(start)
	saved := 0
	for range 2 {
		result := <-outcomes
		if result.err == nil {
			saved++
			if result.receipt.Assisted && result.receipt.Rating != 1 {
				t.Fatal("assisted race produced cold success")
			}
		} else if !errors.Is(result.err, ErrConflict) {
			t.Fatalf("unexpected competing write failure: %v", result.err)
		}
	}
	history, err := s.History(ctx, 10)
	if err != nil || saved != 1 || len(history) != 1 {
		t.Fatalf("competing writers manufactured recall evidence: saved=%d history=%+v err=%v", saved, history, err)
	}
}

// US-003: a recall answer that is not the key is never graded locally, not
// even a case-only difference: it is staged for the one meaning check.
func TestUnmatchedRecallAlwaysStagesTheCheck(t *testing.T) {
	s, _ := newTestStore(t)
	publishFixture(t, s, GeneratedQuiz{Kind: "recall", Prompt: "Splitting which molecule releases oxygen in photosynthesis?", Answer: "Water", Explanation: "Photolysis splits water.", Basis: "topic"})
	p := answerCurrent(t, s, "case-only", "water")
	if !p.Pending || p.AssessmentID == "" || p.SelfCheck || p.Graded || p.Quiz.Answer != "" {
		t.Fatalf("a case-only difference was decided locally instead of staged: %+v", p)
	}
}
