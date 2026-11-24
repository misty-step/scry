package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRule_RecoveryCannotOverwriteAnotherLearnersIdentity(t *testing.T) {
	ctx := context.Background()
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	_, err = source.CreateUser(ctx, "alice@example.com", "alice", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := source.Snapshot(ctx, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dest, err := Open(filepath.Join(t.TempDir(), "dest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	bob, err := dest.CreateUser(ctx, "bob@example.com", "bob", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dest.RestoreUser(ctx, snap, "alice@example.com"); err == nil {
		t.Fatal("accepted conflicting account identity")
	}
	got, err := dest.UserByID(ctx, bob.ID)
	if err != nil || got.Email != bob.Email {
		t.Fatalf("changed other learner: %+v %v", got, err)
	}
}

func TestRule_Durability_BackupsInSameClockTickDoNotCollide(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir, now := t.TempDir(), time.Now()
	first, err := s.Snapshot(context.Background(), dir, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Snapshot(context.Background(), dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("snapshots collided")
	}
	for _, name := range []string{first, second} {
		fi, err := os.Stat(name)
		if err != nil || fi.Size() == 0 || fi.Mode().Perm() != 0600 {
			t.Fatalf("snapshot not private/complete: %v %v", fi, err)
		}
	}
}

// The theme save pair (theme_tab, theme_seq): within one tab an older or
// equal scripted save never overwrites a newer one; a save from another tab
// (another already-open page, a device with a skewed clock) always applies,
// whatever its number; a plain save always applies and clears the pair; the
// columns are added idempotently on open.
func TestThemeSeqCompareAndSet(t *testing.T) {
	path := t.TempDir() + "/seq.db"
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := st.DB().Exec(`INSERT INTO users(email, auth_subject, created_at) VALUES ('seq@example.com', 'seq', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	us := st.For(id)
	read := func() (theme, tab string, seq int64) {
		t.Helper()
		if err := st.DB().QueryRow(`SELECT theme, theme_tab, theme_seq FROM users WHERE id = ?`, id).Scan(&theme, &tab, &seq); err != nil {
			t.Fatal(err)
		}
		return
	}
	step := func(theme, tab string, seq int64, wantApplied bool, wantTheme, wantTab string, wantSeq int64) {
		t.Helper()
		applied, err := us.SetUserTheme(ctx, theme, tab, seq)
		if err != nil {
			t.Fatal(err)
		}
		gotTheme, gotTab, gotSeq := read()
		if applied != wantApplied || gotTheme != wantTheme || gotTab != wantTab || gotSeq != wantSeq {
			t.Fatalf("SetUserTheme(%q, %q, %d) = %v, stored (%q, %q, %d); want %v, (%q, %q, %d)",
				theme, tab, seq, applied, gotTheme, gotTab, gotSeq, wantApplied, wantTheme, wantTab, wantSeq)
		}
	}
	// A new account: no scripted save yet.
	if theme, tab, seq := read(); theme != "system" || tab != "" || seq != 0 {
		t.Fatalf("new account stores (%q, %q, %d)", theme, tab, seq)
	}
	step("light", "", 0, true, "light", "", 0) // plain submit before any scripted save
	// Same tab: newer applies, older or equal is ignored.
	step("dark", "A", 2, true, "dark", "A", 2)
	step("light", "A", 1, false, "dark", "A", 2) // held, arriving late
	step("light", "A", 2, false, "dark", "A", 2) // not newer
	step("system", "A", 3, true, "system", "A", 3)
	// Another tab: applies with an equal or lower number.
	step("light", "B", 3, true, "light", "B", 3)
	step("dark", "C", 1, true, "dark", "C", 1)
	step("light", "A", 1, true, "light", "A", 1) // A again, after another tab
	// Plain submit after scripted saves: applies and clears the pair; then
	// any scripted save applies again.
	step("system", "", 0, true, "system", "", 0)
	step("dark", "A", 1, true, "dark", "A", 1)
	step("light", "A", -1, true, "light", "", 0) // a non-positive number is plain
	step("dark", "", 7, true, "dark", "", 0)     // so is a missing tab
	step("system", "D", 1, true, "system", "D", 1)
	st.Close()
	st, err = Open(path) // the migration is idempotent
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if theme, tab, seq := read(); theme != "system" || tab != "D" || seq != 1 {
		t.Fatalf("after reopen: %q %q %d", theme, tab, seq)
	}
	if u, err := st.UserByID(ctx, id); err != nil || u.Theme != "system" {
		t.Fatalf("UserByID = %+v, %v; want Theme system", u, err)
	}
}
