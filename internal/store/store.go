package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	path  string
	clock func() time.Time
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil, fmt.Errorf("%w: supply a local database file path", ErrInvalid)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(absolute); statErr == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: database must be a regular local file", ErrInvalid)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	file, err := os.OpenFile(absolute, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "synchronous(FULL)")
	params.Add("_pragma", "trusted_schema(OFF)")
	params.Set("_txlock", "immediate")
	u.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, path: absolute, clock: time.Now}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = s.initialize(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open Scry database: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }
func (s *Store) now() int64   { return s.clock().UTC().UnixMilli() }

func (s *Store) initialize(ctx context.Context) error {
	var engine string
	if err := s.db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&engine); err != nil {
		return err
	}
	parts := strings.Split(engine, ".")
	if len(parts) != 3 {
		return fmt.Errorf("unrecognized SQLite version %q", engine)
	}
	numbers := [3]int{}
	for i := range parts {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return fmt.Errorf("unrecognized SQLite version %q", engine)
		}
		numbers[i] = n
	}
	if numbers[0]*1000000+numbers[1]*1000+numbers[2] < 3051003 {
		return fmt.Errorf("SQLite %s lacks the required WAL-reset fix; require >=3.51.3", engine)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version, app int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app); err != nil {
		return err
	}
	if version == 0 && app == 0 {
		var tables int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return fmt.Errorf("%w: refusing to initialize a nonempty foreign database", ErrInvalid)
		}
		if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("migration 1: %w", err)
		}
		version, app = 1, ApplicationID
	} else if (version != 1 && version != SchemaVersion) || app != ApplicationID {
		return fmt.Errorf("%w: incompatible database application/schema (%d/%d), require %d/%d", ErrInvalid, app, version, ApplicationID, SchemaVersion)
	}
	if err = CheckSchema(ctx, tx, version); err != nil {
		return err
	}
	if version == 1 {
		if _, err = tx.ExecContext(ctx, schemaV2); err != nil {
			return fmt.Errorf("migration 2: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var journal string
	if err = s.db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journal); err != nil {
		return err
	}
	if journal != "wal" {
		return fmt.Errorf("SQLite WAL unavailable: %s", journal)
	}
	var foreignKeys, synchronous int
	if err = s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return err
	}
	if err = s.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return err
	}
	if foreignKeys != 1 || synchronous != 2 {
		return errors.New("SQLite durability configuration was not applied")
	}
	if err = integrity(ctx, s.db); err != nil {
		return err
	}
	// This join also checks that the core schema and singleton exist, rather
	// than treating a forged user_version as proof of readiness.
	var singleton int
	return s.db.QueryRowContext(ctx, `SELECT r.singleton FROM review_session r
	 LEFT JOIN presentations p ON p.id=r.current_id LEFT JOIN quizzes q ON q.id=p.quiz_id
	 LEFT JOIN schedules sc ON sc.quiz_id=q.id LEFT JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 LEFT JOIN sources src ON src.id=q.source_id WHERE r.singleton=1`).Scan(&singleton)
}

func integrity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("SQLite integrity check failed: %s", result)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("SQLite foreign key check failed")
	}
	return rows.Err()
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("cryptographic randomness unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

func marshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func payloadHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	} // callers pass only bounded strings, integers and structs
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func validText(name, value string, max int, required bool) error {
	if !utf8.ValidString(value) || len(value) > max || (required && strings.TrimSpace(value) == "") {
		requirement := ""
		if required {
			requirement = " (not blank)"
		}
		return fmt.Errorf("%w: %s must be valid text%s, at most %d bytes", ErrInvalid, name, requirement, max)
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("%w: %s contains unsupported control characters", ErrInvalid, name)
		}
	}
	return nil
}

func validOperation(id string) error { return validText("operation ID", id, 200, true) }

func existingOperation(ctx context.Context, tx *sql.Tx, id, kind, hash string) (resultID, resultJSON string, found bool, err error) {
	var oldKind, oldHash string
	var result sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT kind,payload_hash,result_id,result_json FROM operations WHERE id=?", id).Scan(&oldKind, &oldHash, &resultID, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if oldKind != kind || oldHash != hash {
		return "", "", false, fmt.Errorf("%w: this operation ID was already used for a different request", ErrConflict)
	}
	return resultID, result.String, true, nil
}

func saveOperation(ctx context.Context, tx *sql.Tx, id, kind, hash, resultID string, result any, now int64) error {
	var encoded any
	if result != nil {
		text, err := marshal(result)
		if err != nil {
			return err
		}
		encoded = text
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO operations(id,kind,payload_hash,result_id,result_json,created_at) VALUES(?,?,?,?,?,?)", id, kind, hash, resultID, encoded, now)
	return err
}

func notFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	}
	return err
}
