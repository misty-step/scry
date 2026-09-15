package recovery

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

func legacyDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	schema, err := os.ReadFile("testdata/go-v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(string(schema))
	if err = errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	return path
}

func archiveDatabase(t *testing.T, database, path, id string, declaredSchema int) manifest {
	t.Helper()
	ctx := context.Background()
	info, err := inspectCompatibleDatabase(ctx, database, true, true)
	if err != nil {
		t.Fatal(err)
	}
	info.Schema = declaredSchema
	digest, size, err := checksum(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	metadata := manifest{Format: 1, ID: id, CreatedAt: 1704067200000, Database: info, Bytes: size, SHA256: digest, Integrity: "ok"}
	if err = writeArchive(ctx, path, database, metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestUpgradePreflightIsReadOnlyAndStrictCheckRejectsV1(t *testing.T) {
	ctx := context.Background()
	path := legacyDatabase(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Check(ctx, path); err == nil {
		t.Fatal("strict rollback check accepted an old schema")
	}
	report, err := CheckForMigration(ctx, path)
	if err != nil || !report.Accepted || report.Compatible || !report.MigrationRequired || report.Migrated || !report.ReadOnly || report.SourceSchema != 1 || report.TargetSchema != store.SchemaVersion {
		t.Fatalf("upgrade preflight confused source acceptance with completed migration: %+v %v", report, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("upgrade preflight modified the v1 database: %v", err)
	}
	if err = noSourceSidecars(path); err != nil {
		t.Fatal(err)
	}
	if _, err = CheckForMigration(ctx, filepath.Join(t.TempDir(), "absent.sqlite")); err == nil {
		t.Fatal("upgrade preflight accepted an absent database")
	}
}

func TestRestoreMigratesOnlyPrivateCopyAndRejectsManifestMismatch(t *testing.T) {
	ctx := context.Background()
	source := legacyDatabase(t)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	id := "scry-20240101T000000.000000000Z-00000000000000000000000000000000"
	archive := filepath.Join(t.TempDir(), id+archiveSuffix)
	archiveDatabase(t, source, archive, id, 1)
	for _, input := range []string{source, archive} {
		t.Run(filepath.Base(input), func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "restored.sqlite")
			if err := Restore(ctx, input, destination); err != nil {
				t.Fatal(err)
			}
			if err := Check(ctx, destination); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(destination)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			src, err := db.Source(ctx, "legacy-running-source")
			if err != nil || src.Job == nil || src.Job.Status != "paused" || !src.Job.CostUnknown {
				t.Fatalf("migration lost the paused uncertain job: %+v %v", src, err)
			}
			claimed, err := db.ClaimJob(ctx, time.Minute, 0, 0)
			if err != nil || claimed != nil {
				t.Fatalf("restore made external work runnable: %+v %v", claimed, err)
			}
		})
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("restore migrated the original v1 snapshot: %v", err)
	}
	t.Run("archive-declaration", func(t *testing.T) {
		mismatch := filepath.Join(t.TempDir(), "mismatch.zip")
		archiveDatabase(t, source, mismatch, id, store.SchemaVersion)
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err := Restore(ctx, mismatch, destination); err == nil {
			t.Fatal("restore accepted a checksum-valid archive whose schema declaration differs from its database")
		}
		assertAbsent(t, destination)
	})
	t.Run("standalone-declaration", func(t *testing.T) {
		metadata := archiveDatabase(t, source, filepath.Join(t.TempDir(), "descriptor.zip"), id, store.SchemaVersion)
		encoded, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(source+".manifest.json", encoded, 0600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "restored.sqlite")
		if err = Restore(ctx, source, destination); err == nil {
			t.Fatal("restore accepted a mismatched standalone manifest")
		}
		assertAbsent(t, destination)
	})
}

func TestOldSchemaArchivesNeitherExpireNorConsumeCurrentRetention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(openTestStore(t), Config{Dir: dir, Keep: 3})
	current := make([]store.BackupRecord, 3)
	for i := range current {
		var err error
		current[i], err = m.Backup(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	legacy := legacyDatabase(t)
	oldArchives := map[string][]byte{}
	for _, year := range []string{"1900", "9999"} {
		id := "scry-" + year + "0101T000000.000000000Z-00000000000000000000000000000000"
		path := filepath.Join(dir, id+archiveSuffix)
		archiveDatabase(t, legacy, path, id, 1)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		oldArchives[path] = data
	}
	m.cfg.Keep = 2
	if err := m.prune(ctx, dir, current[2].Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(current[0].Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current-schema retention did not expire its oldest archive: %v", err)
	}
	for _, retained := range current[1:] {
		if err := regularFile(retained.Path); err != nil {
			t.Fatalf("v1 archive consumed a current-schema retention slot: %v", err)
		}
	}
	for path, before := range oldArchives {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("old-schema archive was deleted or modified: %s %v", path, err)
		}
	}
}
