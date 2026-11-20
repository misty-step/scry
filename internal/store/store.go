// Package store is Scry's durable state: one SQLite database.
//
// Privacy is structural. Everything a learner owns is reached through a
// UserStore, which is bound to one user id and puts that id in every
// statement. Handlers never see a query that is not already scoped.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// ErrNotFound means the row does not exist for this user. A row that
// belongs to somebody else is indistinguishable from one that is missing.
var ErrNotFound = errors.New("not found")

// Store is the database handle for system-wide work: accounts, sessions,
// health and backup. Learner data goes through For.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (and if needed creates and migrates) the database at path.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + path + "?_txlock=immediate" +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v >= 1 {
		return s.migrateAdded()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.migrateAdded()
}

// migrateAdded creates the tables added after schema version 1. Each is
// idempotent, so it runs on every open.
func (s *Store) migrateAdded() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS reservations(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id), amount INTEGER NOT NULL CHECK(amount>0))`,
		topupSchema,
		`CREATE INDEX IF NOT EXISTS topups_pending ON topups(status, created_at)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle to adapters that keep their own tables.
func (s *Store) DB() *sql.DB { return s.db }

// Path is the database file.
func (s *Store) Path() string { return s.path }

// Check proves the database can be read and written. /healthz calls it.
func (s *Store) Check(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO health (id, at) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET at = excluded.at`, time.Now().Unix())
	return err
}

// Integrity runs SQLite's own consistency check.
func (s *Store) Integrity(ctx context.Context) error {
	var res string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("database integrity: %s", res)
	}
	return nil
}

// Snapshot writes a consistent copy of the whole database into dir while
// the app keeps running, and returns the new file's path.
func (s *Store) Snapshot(ctx context.Context, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Unique empty files also make concurrent/manual backups safe within a
	// single clock tick, and keep snapshot permissions private (0600).
	f, err := os.CreateTemp(dir, "scry-"+now.UTC().Format("20060102-150405.000000000")+"-*.db")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(dest)
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// PruneSnapshots keeps the newest keep snapshots in dir.
func PruneSnapshots(dir string, keep int) error {
	names, err := filepath.Glob(filepath.Join(dir, "scry-*.db"))
	if err != nil {
		return err
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(names[0]); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

// LatestSnapshot reports the newest snapshot in dir and when it was made.
func LatestSnapshot(dir string) (string, time.Time) {
	names, _ := filepath.Glob(filepath.Join(dir, "scry-*.db"))
	if len(names) == 0 {
		return "", time.Time{}
	}
	sort.Strings(names)
	last := names[len(names)-1]
	if fi, err := os.Stat(last); err == nil {
		return last, fi.ModTime()
	}
	return last, time.Time{}
}

// userTables lists every table holding learner-owned rows, parents first.
// Export, per-user restore and the privacy tests all walk this list.
var userTables = []string{
	"subscriptions", "reservations", "topups", "ledger", "usage", "ideas", "idea_links", "items",
	"study_sessions", "session_items", "reviews", "drafts", "suggestions", "agent_tokens",
}

// UserTables returns the names of the tables that hold learner-owned rows.
func UserTables() []string { return append([]string(nil), userTables...) }

// RestoreUser replaces one learner's data with what a snapshot holds for
// them, leaving every other learner untouched. It returns the number of
// rows restored.
func (s *Store) RestoreUser(ctx context.Context, snapshot, email string) (int64, error) {
	if _, err := os.Stat(snapshot); err != nil {
		return 0, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS snap`, snapshot); err != nil {
		return 0, err
	}
	defer conn.ExecContext(context.Background(), `DETACH DATABASE snap`)
	var uid int64
	err = conn.QueryRowContext(ctx, `SELECT id FROM snap.users WHERE email = ?`, email).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("snapshot has no account for %s", email)
	}
	if err != nil {
		return 0, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// A snapshot from a different database may reuse an ID for someone else.
	// Refuse before deleting anything, even if their owned tables are empty.
	var conflicts int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users
 WHERE (id = ? AND (email != ? OR auth_subject != (SELECT auth_subject FROM snap.users WHERE id = ?)))
 OR (email = ? AND id != ?)`, uid, email, uid, email, uid).Scan(&conflicts); err != nil {
		return 0, err
	}
	if conflicts != 0 {
		return 0, fmt.Errorf("snapshot account identity conflicts with destination")
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = 1`); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO users SELECT * FROM snap.users WHERE id = ?`, uid); err != nil {
		return 0, err
	}
	for i := len(userTables) - 1; i >= 0; i-- {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+userTables[i]+` WHERE user_id = ?`, uid); err != nil {
			return 0, err
		}
	}
	var total int64
	for _, t := range userTables {
		var present int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM snap.sqlite_master WHERE type = 'table' AND name = ?`, t).Scan(&present); err != nil {
			return 0, err
		}
		if present == 0 {
			continue // the snapshot predates this table
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO `+t+` SELECT * FROM snap.`+t+` WHERE user_id = ?`, uid)
		if err != nil {
			return 0, fmt.Errorf("restore %s: %w", t, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, tx.Commit()
}

func ts(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func tm(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func one(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
