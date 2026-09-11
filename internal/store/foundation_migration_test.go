package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Build a populated v1 fixture with the original schema and exact old table
// values. This is test fixture construction, never an application down-migration.
func populatedV1(t *testing.T) (string, map[string]json.RawMessage) {
	t.Helper()
	ctx := context.Background()
	s, _ := newTestStore(t)
	src := publishFixture(t, s, authoredChoice("Old target before correction?"), authoredChoice("Old second target?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.Submit(ctx, state.Current.ID, "old-reveal", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Dispute(ctx, review.ReviewID, "Synthetic v1 correction", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditQuiz(ctx, src.Quizzes[0].ID, 1, authoredChoice("Corrected future wording?")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Capture(ctx, "Unknown paid v1 work", "old-unknown-capture"); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	if err = s.FailJob(ctx, j.ID, j.LeaseToken, "Unknown provider outcome retained", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Capture(ctx, "Queued v1 capture", "old-queued"); err != nil {
		t.Fatal(err)
	}
	encoded, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var before map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &before); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "populated-v1.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "ATTACH DATABASE ? AS fixture", s.Path()); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sources", "source_revisions", "jobs", "job_attempts", "quizzes", "quiz_versions", "schedules", "presentations", "review_events", "corrections", "operations", "backups"} {
		if _, err = tx.ExecContext(ctx, "INSERT INTO main."+table+" SELECT * FROM fixture."+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE main.review_session SET current_id=(SELECT current_id FROM fixture.review_session)"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, before
}

func TestPopulatedV1FoundationMigrationPreservesEveryHistoricalSection(t *testing.T) {
	ctx := context.Background()
	path, before := populatedV1(t)
	// The read-only preflight accepts a complete v1 schema without upgrading it.
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckSchema(ctx, tx, 1); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatal("preflight changed v1")
	}
	if err = errors.Join(tx.Rollback(), db.Close()); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	encoded, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &after); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"sources", "source_revisions", "jobs", "job_attempts", "quizzes", "quiz_versions", "schedules", "presentations", "review_session", "review_events", "corrections", "operations", "backups"} {
		if !reflect.DeepEqual(before[section], after[section]) {
			t.Fatalf("migration reinterpreted historical %s", section)
		}
	}
	for _, section := range []string{"foundation_requests", "foundation_bundles", "foundation_units", "foundation_materials", "foundation_links", "foundation_bridges", "foundation_interactions"} {
		if string(after[section]) != "[]" {
			t.Fatalf("migration invented historical knowledge: %s", section)
		}
	}
	old, err := s.Submit(ctx, mustCurrentID(t, s), "old-reveal", "", true)
	if err != nil || old.Outcome != "revealed" || !old.Assisted {
		t.Fatalf("old operation receipt changed: %+v %v", old, err)
	}
}

func mustCurrentID(t *testing.T, s *Store) string {
	t.Helper()
	p, err := s.Current(context.Background())
	if err != nil || p == nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestFoundationMigrationRejectsPartialOrForgedSchemaBeforeMutation(t *testing.T) {
	ctx := context.Background()
	path, _ := populatedV1(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "DROP TRIGGER immutable_review_update"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("incomplete v1 accepted")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, tables int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='foundation_bridges'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if version != 1 || tables != 0 {
		t.Fatal("failed preflight partially migrated database")
	}
}
