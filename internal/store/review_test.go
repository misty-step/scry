package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
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

func publishFixture(t *testing.T, s *Store, content ...GeneratedQuiz) Source {
	t.Helper()
	ctx := context.Background()
	src, err := s.Capture(ctx, "Synthetic fixture subject", newID())
	if err != nil {
		t.Fatal(err)
	}
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

func TestRevealFenceAndUngradedRetry(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	recall := GeneratedQuiz{Kind: "recall", Prompt: "Explain the synthetic process.", Answer: "A process that releases stored chemical energy for cellular work", Explanation: "This is an authored fixture, not a factual learning claim.", Basis: "topic"}
	publishFixture(t, s, recall)
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Current.ID
	ambiguous, err := s.Submit(ctx, id, "ambiguous", "Cells convert the energy they have stored into usable work", false)
	if err != nil || ambiguous.Graded || ambiguous.Outcome != "ungraded" || ambiguous.Quiz.Answer != "" {
		t.Fatalf("semantic answer was overclaimed: %+v %v", ambiguous, err)
	}
	state, err = s.Next(ctx, id)
	if err != nil || state.Current == nil || state.Current.ID != id || state.Current.DueAt != ambiguous.DueAt {
		t.Fatal("ungraded attempt advanced the occurrence or schedule")
	}
	revealed, err := s.Submit(ctx, id, "reveal-once", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !revealed.Graded || !revealed.Assisted || revealed.Rating != 1 || revealed.Outcome != "revealed" || revealed.Quiz.Answer != recall.Answer {
		t.Fatalf("reveal lacked durable assistance: %+v", revealed)
	}
	if _, err = s.Submit(ctx, id, "stale-correct", recall.Answer, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("post-reveal cold success accepted: %v", err)
	}
	duplicate, err := s.Submit(ctx, id, "reveal-once", "", true)
	if err != nil || !reflect.DeepEqual(revealed, duplicate) {
		t.Fatal("reveal retry changed result")
	}
	history, err := s.History(ctx, 20)
	if err != nil || len(history) != 2 || history[0].Rating != 1 || history[1].Rating != 0 {
		t.Fatalf("ungraded/assisted history was rewritten: %+v %v", history, err)
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
