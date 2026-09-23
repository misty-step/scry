package generation

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/misty-step/scry/internal/store"
	_ "modernc.org/sqlite"
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

// A synthetic unmigrated quiz job must never reach the retired v4 model prompt.
// The live migration and RetrySource normally cut these jobs over to a plan.
func TestLegacyQuizJobWithoutCandidatesFailsClosedWithoutSending(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scry.sqlite")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source, err := s.Capture(ctx, store.CaptureInput{Text: "cell energy", Mode: "topic"}, "legacy-no-candidates")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if _, err := fixture.ExecContext(ctx, `UPDATE jobs SET kind='quizzes' WHERE source_id=? AND status='queued'`, source.ID); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, jobLease, 100_000, 1_000_000)
	if err != nil || job == nil || job.Kind != "quizzes" {
		t.Fatalf("legacy claim: %+v %v", job, err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	if err := New(s, localConfig(server.URL)).process(ctx, job); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Source(ctx, source.ID)
	if err != nil || calls.Load() != 0 || saved.Job == nil || saved.Job.Status != "failed" || saved.Job.CostMicros != 0 || saved.Job.CostUnknown || saved.Job.Error != "This older preparation needs a retry." {
		t.Fatalf("legacy work was transmitted or incorrectly settled: %+v calls=%d err=%v", saved, calls.Load(), err)
	}
}
