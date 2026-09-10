package recovery

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "live.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func localBackup(t *testing.T, s *store.Store) store.BackupRecord {
	t.Helper()
	record, err := New(s, Config{Dir: t.TempDir()}).Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("restore left an output at %s: %v", path+suffix, err)
		}
	}
}

func TestCheckNeverCreatesMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	if err := Check(context.Background(), path); err == nil {
		t.Fatal("read-only deployment check accepted an absent database")
	}
	assertAbsent(t, path)
}

func TestRestorePreservesCapturesAndPausesUncertainWork(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	first, err := s.Capture(ctx, "SQL terminology", "recovery-first-capture")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 1000, 1000000)
	if err != nil || job == nil {
		t.Fatalf("create an uncertain in-flight attempt: job=%v err=%v", job, err)
	}
	second, err := s.Capture(ctx, "Relational database normalization", "recovery-second-capture")
	if err != nil {
		t.Fatal(err)
	}
	record := localBackup(t, s)
	before, _, err := checksum(ctx, record.Path)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "restored.sqlite")
	if err = Restore(ctx, record.Path, destination); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	for _, original := range []store.Source{first, second} {
		recovered, err := restored.Source(ctx, original.ID)
		if err != nil || recovered.Text != original.Text {
			t.Fatalf("acknowledged capture was not recovered: %+v, %v", recovered, err)
		}
	}
	claimed, err := restored.ClaimJob(ctx, time.Minute, 1000, 1000000)
	if err != nil || claimed != nil {
		t.Fatalf("restored work must not start external requests: job=%+v err=%v", claimed, err)
	}
	summary, err := restored.Summary(ctx)
	if err != nil || !summary.CostUnknown {
		t.Fatalf("restored in-flight spend must remain uncertain: %+v, %v", summary, err)
	}
	original, err := s.Source(ctx, first.ID)
	if err != nil || original.Job == nil || original.Job.Status != "running" {
		t.Fatalf("restore modified the source store: %+v, %v", original, err)
	}
	after, _, err := checksum(ctx, record.Path)
	if err != nil || before != after {
		t.Fatalf("restore modified its recovery archive: %v", err)
	}
}

func TestRestoreRefusesOccupiedDatabaseOrSidecar(t *testing.T) {
	s := openTestStore(t)
	record := localBackup(t, s)
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		t.Run("occupied"+suffix, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "restored.sqlite")
			occupied := destination + suffix
			original := []byte("existing recovery material must survive unchanged")
			if err := os.WriteFile(occupied, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := Restore(context.Background(), record.Path, destination); err == nil {
				t.Fatal("restore overwrote an occupied database or sidecar")
			}
			after, err := os.ReadFile(occupied)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatalf("occupied recovery material changed: %v", err)
			}
			if suffix != "" {
				if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("restore published a database next to occupied sidecars: %v", err)
				}
			}
		})
	}
	t.Run("dangling-symlink", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err := os.Symlink("not-created.sqlite", destination); err != nil {
			t.Fatal(err)
		}
		if err := Restore(context.Background(), record.Path, destination); err == nil {
			t.Fatal("restore followed or replaced a dangling destination symlink")
		}
		if target, err := os.Readlink(destination); err != nil || target != "not-created.sqlite" {
			t.Fatalf("occupied symlink changed: %q, %v", target, err)
		}
	})
}

func TestRestoreRejectsCorruptIncompleteAndForeignRecovery(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	record := localBackup(t, s)
	complete, err := os.ReadFile(record.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("interrupted-archive", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "interrupted.zip")
		if err := os.WriteFile(path, complete[:len(complete)-30], 0600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err := Restore(ctx, path, destination); err == nil {
			t.Fatal("restore accepted an interrupted archive without a complete ZIP directory")
		}
		assertAbsent(t, destination)
	})
	t.Run("valid-zip-with-wrong-database-checksum", func(t *testing.T) {
		zr, err := zip.OpenReader(record.Path)
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()
		var corrupt bytes.Buffer
		zw := zip.NewWriter(&corrupt)
		for _, entry := range zr.File {
			reader, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if entry.Name == databaseEntry {
				data[len(data)-1] ^= 1
			}
			h := &zip.FileHeader{Name: entry.Name, Method: zip.Store}
			h.SetMode(0600)
			writer, err := zw.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = writer.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err = zw.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "corrupt.zip")
		if err = os.WriteFile(path, corrupt.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err = Restore(ctx, path, destination); err == nil {
			t.Fatal("restore trusted ZIP validity without the database checksum")
		}
		assertAbsent(t, destination)
	})
	t.Run("incompatible-schema", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "future.sqlite")
		if err := s.Backup(ctx, path); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", store.SchemaVersion+1))
		err = errors.Join(err, db.Close())
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err = Restore(ctx, path, destination); err == nil {
			t.Fatal("restore accepted a schema the binary cannot operate")
		}
		assertAbsent(t, destination)
	})
}

func TestRemoteCompletionRequiresReadbackAndFailurePreservesReview(t *testing.T) {
	for _, scenario := range []string{"put-rejected", "same-length-corrupt-readback", "complete-readback"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			if _, err := s.Capture(ctx, "SQL terminology", "backup-review-fixture"); err != nil {
				t.Fatal(err)
			}
			job, err := s.ClaimJob(ctx, time.Minute, 1000, 1000000)
			if err != nil || job == nil {
				t.Fatalf("claim authored fixture: %+v %v", job, err)
			}
			zero := int64(0)
			content := store.GenerationResult{Model: "authored-test-material", PromptVersion: "test", Quizzes: []store.GeneratedQuiz{{Kind: "recall", Prompt: "What does SQL stand for?", Answer: "Structured Query Language", Explanation: "SQL abbreviates Structured Query Language.", Basis: "topic"}}}
			if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, content, &zero); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var uploaded []byte
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-test-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				switch r.Method {
				case http.MethodPut:
					if scenario == "put-rejected" {
						http.Error(w, "synthetic private error must not escape", http.StatusServiceUnavailable)
						return
					}
					data, err := io.ReadAll(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					mu.Lock()
					uploaded = data
					mu.Unlock()
					w.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					mu.Lock()
					data := append([]byte(nil), uploaded...)
					mu.Unlock()
					if scenario == "same-length-corrupt-readback" && len(data) > 0 {
						data[len(data)-1] ^= 1
					}
					w.Write(data)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer sink.Close()
			m := New(s, Config{Dir: t.TempDir(), RemoteURL: sink.URL + "/private", RemoteToken: "synthetic-test-token"})
			record, backupErr := m.Backup(ctx)
			wantRemote := scenario == "complete-readback"
			if record.Remote != wantRemote || (backupErr == nil) != wantRemote {
				t.Fatalf("incorrect remote completion: record=%+v err=%v", record, backupErr)
			}
			if strings.Contains(record.Error, "synthetic-test-token") || strings.Contains(record.Error, "synthetic private error") {
				t.Fatal("private credentials or response body escaped into persistent backup health")
			}
			summary, err := s.Summary(ctx)
			if err != nil || summary.LastBackup == nil || summary.LastBackup.Remote != wantRemote || (!wantRemote && summary.LastBackup.Error == "") {
				t.Fatalf("backup outcome is not visible to the owner: %+v, %v", summary, err)
			}
			state, err := s.Review(ctx)
			if err != nil || state.Current == nil || state.Current.Quiz.Prompt != content.Quizzes[0].Prompt {
				t.Fatalf("backup outcome disabled the ordinary review surface: %+v, %v", state, err)
			}
			destination := filepath.Join(t.TempDir(), "restored.sqlite")
			if err = Restore(ctx, record.Path, destination); err != nil {
				t.Fatalf("local archive was not recoverable after remote outcome: %v", err)
			}
		})
	}
}

func TestRetentionPreservesNewestAndUnknownMaterial(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	m := New(s, Config{Dir: dir, Keep: 1})
	old, err := m.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(dir, "scry-20000101T000000.000000000Z-00000000000000000000000000000000"+archiveSuffix)
	unknownBytes := []byte("interrupted or unrecognized recovery material")
	if err = os.WriteFile(unknown, unknownBytes, 0600); err != nil {
		t.Fatal(err)
	}
	newest, err := m.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(old.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired complete local archive was not pruned: %v", err)
	}
	preserved, err := os.ReadFile(unknown)
	if err != nil || !bytes.Equal(preserved, unknownBytes) {
		t.Fatalf("retention deleted or rewrote unknown material: %v", err)
	}
	if err = Restore(ctx, newest.Path, filepath.Join(t.TempDir(), "restored.sqlite")); err != nil {
		t.Fatalf("retention lost the newest complete recovery set: %v", err)
	}
}
