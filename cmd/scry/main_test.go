package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func TestSeedFixtureRefusesOccupiedPaths(t *testing.T) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.sqlite")
			original := []byte("unrelated durable recovery material")
			if err := os.WriteFile(path+suffix, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := seedFixture([]string{"--db", path}); err == nil {
				t.Fatal("seed-fixture accepted occupied data or a sidecar")
			}
			after, err := os.ReadFile(path + suffix)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatalf("occupied material changed: %v", err)
			}
			if suffix != "" {
				if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("fixture was published next to an occupied sidecar: %v", err)
				}
			}
		})
	}
	t.Run("dangling-symlink", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fixture.sqlite")
		if err := os.Symlink("unavailable.sqlite", path); err != nil {
			t.Fatal(err)
		}
		if err := seedFixture([]string{"--db", path}); err == nil {
			t.Fatal("seed-fixture followed a dangling symlink")
		}
		if target, err := os.Readlink(path); err != nil || target != "unavailable.sqlite" {
			t.Fatalf("fixture replaced the symlink: %q %v", target, err)
		}
	})
}

func TestSeedFixtureCannotClaimExistingCapture(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owner.sqlite")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source, err := db.Capture(ctx, "An unrelated owner capture", "owner-capture")
	if err != nil {
		t.Fatal(err)
	}
	if err = seedFixture([]string{"--db", path}); err == nil {
		t.Fatal("fixture accepted an existing owner database")
	}
	after, err := db.Source(ctx, source.ID)
	if err != nil || after.Job == nil || after.Job.Status != "queued" || after.Job.Attempts != 0 || after.Text != source.Text {
		t.Fatalf("fixture claimed or modified unrelated work: %+v %v", after, err)
	}
}
