package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func stageCritic(t *testing.T, s *Store, configured bool, quizzes ...GeneratedQuiz) (*Job, GenerationResult) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Capture(ctx, "Synthetic critic topic", newID()); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx, time.Minute, 200_000, 1_000_000)
	if err != nil || j == nil {
		t.Fatalf("claim: %+v %v", j, err)
	}
	result := GenerationResult{Quizzes: quizzes, Model: "synthetic-generator", PromptVersion: "fixture-v1"}
	cost := int64(100)
	if err := s.SaveCandidates(ctx, j.ID, j.LeaseToken, result, &cost, configured); err != nil {
		t.Fatal(err)
	}
	return j, result
}

func prepareCritic(t *testing.T, s *Store, job *Job) []ContentAssessment {
	t.Helper()
	entries, err := s.PrepareContentAssessments(context.Background(), job.ID, job.LeaseToken)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func startCritic(t *testing.T, s *Store, job *Job, a ContentAssessment) string {
	t.Helper()
	token, send, err := s.BeginContentTransmission(context.Background(), a.ID, job.LeaseToken, "fixture-critic", []byte(`{"questions":{},"state":{}}`), testSpending)
	if err != nil || !send || token == "" {
		t.Fatalf("begin: %s %v %v", token, send, err)
	}
	return token
}

func criticResult(t *testing.T, q GeneratedQuiz, defect string) AssessmentResult {
	t.Helper()
	answers := map[string]any{}
	for _, key := range append(learning.CriticHardKeys(CriticParams(q)), learning.CriticSoftKey) {
		value := .01
		if key == defect {
			value = .99
		}
		answers[key] = map[string]any{"type": "noul", "noul": value}
	}
	raw, err := json.Marshal(map[string]any{"model": "fixture-critic", "answers": answers})
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(10)
	return AssessmentResult{ResponseModel: "fixture-critic", ResponseJSON: raw, CostMicros: &cost}
}

func TestCriticPersistsBeforeSendAndSingleConcurrentLease(t *testing.T) {
	s, _ := newTestStore(t)
	j, result := stageCritic(t, s, true, authoredChoice("Candidate question?"))
	a := prepareCritic(t, s, j)[0]
	var batch string
	if err := s.db.QueryRow(`SELECT candidates_json FROM jobs WHERE id=?`, j.ID).Scan(&batch); err != nil || !strings.Contains(batch, result.Quizzes[0].Prompt) {
		t.Fatalf("not persisted: %s %v", batch, err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, send, err := s.BeginContentTransmission(context.Background(), a.ID, j.LeaseToken, "fixture-critic", []byte(`{}`), testSpending)
			if err != nil && !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
			results <- send
		}()
	}
	wg.Wait()
	close(results)
	sends := 0
	for send := range results {
		if send {
			sends++
		}
	}
	if sends != 1 {
		t.Fatalf("send leases: %d", sends)
	}
	var reserved, transmissions int
	if err := s.db.QueryRow(`SELECT reserved_micros,transmissions FROM content_assessments WHERE id=?`, a.ID).Scan(&reserved, &transmissions); err != nil || reserved != 2000 || transmissions != 1 {
		t.Fatalf("reservation missing: %d %d %v", reserved, transmissions, err)
	}
	if _, err := s.FinishContentAssessment(context.Background(), a.ID, "foreign", criticResult(t, a.Candidate, "")); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign finish: %v", err)
	}
	cost := int64(100)
	if err := s.CompleteJob(context.Background(), j.ID, j.LeaseToken, result, &cost); !errors.Is(err, ErrConflict) {
		t.Fatalf("unjudged published: %v", err)
	}
}

func TestCriticCrashNeverResendsAndPreservesUnknownSpend(t *testing.T) {
	s, now := newTestStore(t)
	j, _ := stageCritic(t, s, true, authoredChoice("Crash question?"))
	a := prepareCritic(t, s, j)[0]
	startCritic(t, s, j, a)
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(31 * time.Second)
	s = openAt(t, path, now)
	_, send, err := s.BeginContentTransmission(context.Background(), a.ID, j.LeaseToken, "fixture-critic", []byte(`{}`), testSpending)
	if err != nil || send {
		t.Fatalf("crash resent: %v %v", send, err)
	}
	history, err := s.ContentHistory(context.Background(), j.ID)
	if err != nil || len(history) != 1 || history[0].Status != "failed" || history[0].Error != assessmentInterruptedErr || history[0].ReservedMicros != 2000 {
		t.Fatalf("lost outcome: %+v %v", history, err)
	}
	summary, err := s.Summary(context.Background())
	if err != nil || summary.CostMicros != 2100 || !summary.CostUnknown {
		t.Fatalf("lost usage: %+v %v", summary, err)
	}
}

func TestCriticReservationSharedWithGenerationAndSemantic(t *testing.T) {
	s, _ := newTestStore(t)
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "critic-shared", "It checks a host")
	beginSemantic(t, s, p.AssessmentID) // 2000 reserved + 70 fixture + 100 generator
	j, _ := stageCritic(t, s, true, authoredChoice("Budget question?"))
	a := prepareCritic(t, s, j)[0]
	_, send, err := s.BeginContentTransmission(context.Background(), a.ID, j.LeaseToken, "fixture", []byte(`{}`), SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 4000})
	if !errors.Is(err, ErrBudget) || send {
		t.Fatalf("shared budget escaped: %v %v", send, err)
	}
	history, _ := s.ContentHistory(context.Background(), j.ID)
	if history[0].Transmissions != 0 || history[0].ReservedMicros != 0 {
		t.Fatal(history)
	}
	// A critic reservation in turn constrains both other work paths.
	s2, _ := newTestStore(t)
	publishFixture(t, s2, authoredSemantic())
	p2 := stageSemantic(t, s2, "reverse-shared", "It checks a host")
	j2, _ := stageCritic(t, s2, true, authoredChoice("Reverse budget?"))
	a2 := prepareCritic(t, s2, j2)[0]
	startCritic(t, s2, j2, a2)
	if _, err := s2.BeginAssessmentTransmission(context.Background(), p2.AssessmentID, "fixture", []byte(`{}`), SemanticSpending{ReservationMicros: 2000, DailyBudgetMicros: 4000}); !errors.Is(err, ErrBudget) {
		t.Fatalf("critic not shared with grading: %v", err)
	}
	cost := int64(100)
	if err := s2.FailJob(context.Background(), j2.ID, j2.LeaseToken, "stop synthetic work", false, &cost); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Capture(context.Background(), "next generation", "budget-next"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.ClaimJob(context.Background(), time.Minute, 2000, 4000); !errors.Is(err, ErrBudget) {
		t.Fatalf("critic not shared with generation: %v", err)
	}
}

func TestCriticRetryReusesCandidatesAndJudgedRowsWithoutGenerationReservation(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	j, result := stageCritic(t, s, true, authoredChoice("Accepted first?"), authoredChoice("Unavailable second?"))
	entries := prepareCritic(t, s, j)
	token := startCritic(t, s, j, entries[0])
	if _, err := s.FinishContentAssessment(ctx, entries[0].ID, token, criticResult(t, entries[0].Candidate, "")); err != nil {
		t.Fatal(err)
	}
	token = startCritic(t, s, j, entries[1])
	if _, err := s.FinishContentAssessment(ctx, entries[1].ID, token, AssessmentResult{Error: "unavailable"}); err != nil {
		t.Fatal(err)
	}
	cost := int64(100)
	if err := s.FailJob(ctx, j.ID, j.LeaseToken, "critic pending", true, &cost); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(11 * time.Second)
	// Generation reservation cannot fit, but critic-only claim is known free.
	retry, err := s.ClaimJob(ctx, time.Minute, 200_000, 5000)
	if err != nil || retry == nil || retry.Candidates == nil || retry.ReservedMicros != 0 || retry.CriticStatus != "pending" {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	if payloadHash(retry.Candidates.Result) != payloadHash(result) {
		t.Fatal("regenerated candidates")
	}
	again := prepareCritic(t, s, retry)
	if again[0].ID != entries[0].ID || again[1].ID == entries[1].ID {
		t.Fatalf("bad reuse: %+v", again)
	}
	token = startCritic(t, s, retry, again[1])
	if _, err := s.FinishContentAssessment(ctx, again[1].ID, token, criticResult(t, again[1].Candidate, "")); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	if err := s.CompleteJob(ctx, retry.ID, retry.LeaseToken, result, &zero); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, retry.ID, retry.LeaseToken, result, &zero); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx)
	if err != nil || summary.CostMicros != 2120 || !summary.CostUnknown || summary.Quizzes != 2 {
		t.Fatalf("retry spend/publication: %+v %v", summary, err)
	}
}

func TestCriticPublicationRevalidatesAndRecordsRejections(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	j, result := stageCritic(t, s, true, authoredChoice("Rejected candidate?"), authoredChoice("Accepted candidate?"))
	entries := prepareCritic(t, s, j)
	for i, a := range entries {
		token := startCritic(t, s, j, a)
		defect := ""
		if i == 0 {
			defect = "no_defensible_answer"
		}
		decision, err := s.FinishContentAssessment(ctx, a.ID, token, criticResult(t, a.Candidate, defect))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (decision.Decision != "reject" || len(decision.Reasons) != 1) {
			t.Fatal(decision)
		}
	}
	// Persisted verdict strings cannot bypass recomputation from raw judgments.
	if _, err := s.db.Exec(`UPDATE content_assessments SET decision='accept' WHERE id=?`, entries[0].ID); err != nil {
		t.Fatal(err)
	}
	cost := int64(100)
	if err := s.CompleteJob(ctx, j.ID, j.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	source, err := s.Source(ctx, j.SourceID)
	if err != nil || len(source.Quizzes) != 1 || source.Quizzes[0].Prompt != "Accepted candidate?" || source.Job.Status != "partial" || source.Job.CriticStatus != "judged" {
		t.Fatalf("publication: %+v %v", source, err)
	}
	history, err := s.ContentHistory(ctx, j.ID)
	if err != nil || !strings.Contains(history[0].Reasons, "no_defensible_answer") {
		t.Fatalf("reasons lost: %+v %v", history, err)
	}
	exported, err := s.Export(ctx)
	if err != nil || !strings.Contains(string(exported), "candidates_json") || !strings.Contains(string(exported), entries[0].ID) {
		t.Fatalf("export: %v", err)
	}
}

func TestCriticUnconfiguredPreservesPublicationAndReservesNothing(t *testing.T) {
	s, _ := newTestStore(t)
	j, result := stageCritic(t, s, false, authoredChoice("Skipped critic?"))
	cost := int64(100)
	if err := s.CompleteJob(context.Background(), j.ID, j.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	source, err := s.Source(context.Background(), j.SourceID)
	if err != nil || len(source.Quizzes) != 1 || source.Job.CriticStatus != "skipped" {
		t.Fatalf("skipped: %+v %v", source, err)
	}
	history, err := s.ContentHistory(context.Background(), j.ID)
	if err != nil || len(history) != 0 {
		t.Fatal(history, err)
	}
}

func TestCriticMalformedAndSourceChangeCannotPublish(t *testing.T) {
	for _, mode := range []string{"malformed", "archive", "all-rejected"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := newTestStore(t)
			ctx := context.Background()
			j, result := stageCritic(t, s, true, authoredChoice("Fenced candidate?"))
			a := prepareCritic(t, s, j)[0]
			token := startCritic(t, s, j, a)
			response := criticResult(t, a.Candidate, "")
			if mode == "malformed" {
				response.ResponseJSON = []byte(`{"answers":{}}`)
			}
			if mode == "all-rejected" {
				response = criticResult(t, a.Candidate, "no_defensible_answer")
			}
			if mode == "archive" {
				if err := s.ArchiveSource(ctx, j.SourceID); err != nil {
					t.Fatal(err)
				}
			}
			decision, err := s.FinishContentAssessment(ctx, a.ID, token, response)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "malformed" && decision.Decision != "ungraded" {
				t.Fatal(decision)
			}
			cost := int64(100)
			if err := s.CompleteJob(ctx, j.ID, j.LeaseToken, result, &cost); err == nil {
				t.Fatal("unsafe publication accepted")
			}
			source, err := s.Source(ctx, j.SourceID)
			if err != nil || len(source.Quizzes) != 0 {
				t.Fatal(source, err)
			}
		})
	}
}
