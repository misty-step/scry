package store

import (
	"context"
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
