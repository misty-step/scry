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

// The theme save sequence: an older scripted save never overwrites a newer
// one; a plain save always applies and advances the number past any earlier
// scripted save still on the way; the number is loaded with the user; the
// column is added idempotently on open.
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
	read := func() (theme string, seq int64) {
		t.Helper()
		if err := st.DB().QueryRow(`SELECT theme, theme_seq FROM users WHERE id = ?`, id).Scan(&theme, &seq); err != nil {
			t.Fatal(err)
		}
		return
	}
	step := func(theme string, seq int64, wantApplied bool, wantTheme string, wantSeq int64) {
		t.Helper()
		applied, err := us.SetThemeSeq(ctx, theme, seq)
		if err != nil {
			t.Fatal(err)
		}
		gotTheme, gotSeq := read()
		if applied != wantApplied || gotTheme != wantTheme || gotSeq != wantSeq {
			t.Fatalf("SetThemeSeq(%q, %d) = %v, stored (%q, %d); want %v, (%q, %d)", theme, seq, applied, gotTheme, gotSeq, wantApplied, wantTheme, wantSeq)
		}
	}
	step("light", 20, true, "light", 20)
	step("dark", 10, false, "light", 20) // older, arriving late
	step("dark", 20, false, "light", 20) // not newer
	step("dark", 21, true, "dark", 21)
	step("system", 0, true, "system", 21+ThemeSeqPlainStep)  // plain submit advances
	step("dark", 22, false, "system", 21+ThemeSeqPlainStep)  // earlier scripted save, held: ignored
	step("dark", 999, false, "system", 21+ThemeSeqPlainStep) // a burst of them too
	step("system", -1, true, "system", 21+2*ThemeSeqPlainStep)
	if u, err := st.UserByID(ctx, id); err != nil || u.ThemeSeq != 21+2*ThemeSeqPlainStep || u.Theme != "system" {
		t.Fatalf("UserByID = %+v, %v; want ThemeSeq %d", u, err, 21+2*ThemeSeqPlainStep)
	}
	step("light", 21+2*ThemeSeqPlainStep+1, true, "light", 21+2*ThemeSeqPlainStep+1) // next page: rendered seq + 1
	st.Close()
	st, err = Open(path) // the migration is idempotent
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if theme, seq := read(); theme != "light" || seq != 21+2*ThemeSeqPlainStep+1 {
		t.Fatalf("after reopen: %q %d", theme, seq)
	}
}
