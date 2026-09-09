package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func generationStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "scry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func captureAndClaim(t *testing.T, s *store.Store, text string, reservation int64) (store.Source, *store.Job) {
	t.Helper()
	ctx := context.Background()
	source, err := s.Capture(ctx, text, "capture-boundary")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, reservation, 1_000_000)
	if err != nil || job == nil {
		t.Fatalf("claim: job=%+v err=%v", job, err)
	}
	if job.Status != "running" || job.Attempts != 1 {
		t.Fatalf("HTTP attempt was not durably claimed: %+v", job)
	}
	return source, job
}

func TestWorkerPublishesUsefulPartialAndRecordsRejectedSpend(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	good, bad := topicDraft(), topicDraft()
	bad.Prompt = "Why is ATP used for cellular energy transfer?"
	server := responseServer(t, envelopeJSON(t, outputJSON(t, "concepts", good, bad), "stop", json.RawMessage(`0.0018031`)), http.StatusOK)
	worker := New(s, localConfig(server.URL))
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "partial" || saved.Job == nil || saved.Job.Status != "partial" || len(saved.Quizzes) != 1 || saved.Quizzes[0].Answer != "ATP" {
		t.Fatalf("useful partial material was not published honestly: %+v", saved)
	}
	if saved.Job.CostUnknown || saved.Job.CostMicros != 1_804 || saved.Job.Attempts != 1 {
		t.Fatalf("paid rejected output or fractional micro-dollar was lost: %+v", saved.Job)
	}
	review, err := s.Review(context.Background())
	if err != nil || review.Current == nil || review.Current.Quiz.ID != saved.Quizzes[0].ID {
		t.Fatalf("validated partial quiz is not immediately reviewable: %+v %v", review, err)
	}
}

func TestArchivedSourceCannotPublishLatePaidCompletion(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	body := envelopeJSON(t, outputJSON(t, "concepts", topicDraft()), "stop", json.RawMessage(`0.002`))
	archived := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The HTTP boundary is outside the write transaction; source lifecycle
		// changes while a paid generation request is in flight must win.
		archived <- s.ArchiveSource(context.Background(), source.ID)
		io.WriteString(w, body)
	}))
	defer server.Close()
	worker := New(s, localConfig(server.URL))
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := <-archived; err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Archived || len(saved.Quizzes) != 0 || saved.Job == nil || saved.Job.Status != "canceled" {
		t.Fatalf("late completion resurrected archived material: %+v", saved)
	}
	if saved.Job.CostUnknown || saved.Job.CostMicros != 2_000 {
		t.Fatalf("stale output's actual paid receipt was discarded: %+v", saved.Job)
	}
}

func TestMissingConfigurationFailsDurablyWithoutTransmission(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 0)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	worker := New(s, Config{Endpoint: server.URL}) // Deliberately no model or spend authorization.
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || saved.Text != source.Text || len(saved.Quizzes) != 0 || saved.Job == nil || saved.Job.Status != "failed" || saved.Job.Error == "" {
		t.Fatalf("unconfigured work did not become an honest saved failure: calls=%d source=%+v", calls.Load(), saved)
	}
	if saved.Job.CostUnknown || saved.Job.CostMicros != 0 {
		t.Fatalf("non-transmitted work should be known free: %+v", saved.Job)
	}
}

func TestSuccessfulUnpricedCompletionKeepsSpendUnknown(t *testing.T) {
	s := generationStore(t)
	source, job := captureAndClaim(t, s, "mitochondria", 100_000)
	server := responseServer(t, envelopeJSON(t, outputJSON(t, "concepts", topicDraft()), "stop", nil), http.StatusOK)
	worker := New(s, localConfig(server.URL))
	if err := worker.process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "ready" || saved.Job == nil || saved.Job.Status != "complete" || !saved.Job.CostUnknown || len(saved.Quizzes) != 1 {
		t.Fatalf("accepted content must not turn missing price into free work: %+v", saved)
	}
	summary, err := s.Summary(context.Background())
	if err != nil || !summary.CostUnknown {
		t.Fatalf("unknown spend disappeared from aggregate: %+v %v", summary, err)
	}
}
