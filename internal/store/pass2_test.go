package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// Durability (Pass 2): a database from before the top-up attempt's
// processor columns and the review's graded snapshot is migrated in place
// on open, and a snapshot taken before them still restores a learner.
func TestRule_Durability_Pass2ColumnsMigrateAndOldSnapshotsRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "old@example.com", "old", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO topups (user_id, token, idem_key, status, amount_cents, credit_micros, created_at) VALUES (?, 'tok-old-000000000001', 'topup-old-1', 'pending', 500, 4000000, 1)`, u.ID); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Snapshot(ctx, filepath.Join(dir, "snaps"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Make both the live database and the snapshot look as they did before
	// this change.
	for _, p := range []string{path, snap} {
		db, err := sql.Open("sqlite", "file:"+p)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`DROP INDEX IF EXISTS topups_charge`,
			`ALTER TABLE topups DROP COLUMN customer_ref`, `ALTER TABLE topups DROP COLUMN payment_ref`,
			`ALTER TABLE reviews DROP COLUMN graded_prompt`, `ALTER TABLE reviews DROP COLUMN graded_expected`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		db.Close()
	}
	s, err = Open(path)
	if err != nil {
		t.Fatalf("migrating an old database: %v", err)
	}
	defer s.Close()
	att, err := s.For(u.ID).Topup(ctx, "tok-old-000000000001")
	if err != nil || att.Status != TopupPending || att.CustomerRef != "" {
		t.Fatalf("old attempt after migration: %+v %v", att, err)
	}
	if cols, err := s.columns(ctx, s.DB(), "main", "reviews"); err != nil || cols[len(cols)-1] != "graded_expected" {
		t.Fatalf("reviews columns after migration: %v %v", cols, err)
	}
	n, err := s.RestoreUser(ctx, snap, "old@example.com")
	if err != nil || n == 0 {
		t.Fatalf("restoring from an old snapshot: %d %v", n, err)
	}
	if att, err := s.For(u.ID).Topup(ctx, "tok-old-000000000001"); err != nil || att.Key != "topup-old-1" {
		t.Fatalf("restored attempt: %+v %v", att, err)
	}
}
