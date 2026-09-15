package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Backup uses SQLite's own coherent snapshot operation, never a raw live-file
// copy. Only this call's newly-created destination is removed on failure.
func (s *Store) Backup(ctx context.Context, destination string) (err error) {
	if destination == "" {
		return fmt.Errorf("%w: backup destination is required", ErrInvalid)
	}
	path, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if path == s.path {
		return fmt.Errorf("%w: backup must not overwrite the database", ErrConflict)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("create unused snapshot path: %w", err)
	}
	if err = file.Close(); err != nil {
		os.Remove(path)
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err = s.db.ExecContext(ctx, "VACUUM main INTO ?", path); err != nil {
		return fmt.Errorf("create SQLite snapshot: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: path}
	query := url.Values{"mode": {"ro"}, "_pragma": {"foreign_keys(1)", "trusted_schema(OFF)"}}
	uri.RawQuery = query.Encode()
	snapshot, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return err
	}
	snapshot.SetMaxOpenConns(1)
	if err = integrity(ctx, snapshot); err == nil {
		var version, app int
		if err = snapshot.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil {
			err = snapshot.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app)
		}
		if err == nil && (version != SchemaVersion || app != ApplicationID) {
			err = errors.New("snapshot schema metadata mismatch")
		}
	}
	closeErr := snapshot.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr = file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr = dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (s *Store) RecordBackup(ctx context.Context, record BackupRecord) error {
	if record.ID == "" {
		record.ID = newID()
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = s.now()
	}
	if record.Bytes < 0 || record.CreatedAt < 0 {
		return fmt.Errorf("%w: invalid backup size or time", ErrInvalid)
	}
	if err := validText("backup ID", record.ID, 200, true); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{{"backup path", record.Path}, {"backup remote key", record.RemoteKey}, {"backup error", record.Error}} {
		if err := validText(field.name, field.value, 4096, false); err != nil {
			return err
		}
	}
	if record.SHA256 != "" {
		digest, err := hex.DecodeString(record.SHA256)
		if err != nil || len(digest) != 32 {
			return fmt.Errorf("%w: invalid backup SHA-256", ErrInvalid)
		}
	}
	if record.Error == "" && (record.Path == "" || record.SHA256 == "" || record.Bytes == 0) {
		return fmt.Errorf("%w: a completed snapshot needs path, checksum and bytes", ErrInvalid)
	}
	if record.Remote && (record.RemoteKey == "" || record.SHA256 == "" || record.Bytes == 0 || record.Error != "") {
		return fmt.Errorf("%w: a remote backup needs successful verified upload metadata", ErrInvalid)
	}
	// A manager can enrich the same attempt with verified remote metadata, but
	// cannot silently replace the snapshot identity behind its existing ID.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var path, digest string
	err = tx.QueryRowContext(ctx, "SELECT path,sha256 FROM backups WHERE id=?", record.ID).Scan(&path, &digest)
	if err == nil && (path != record.Path || (digest != "" && digest != record.SHA256)) {
		return fmt.Errorf("%w: backup ID already identifies a different snapshot", ErrConflict)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO backups(id,path,remote_key,sha256,error,created_at,bytes,remote) VALUES(?,?,?,?,?,?,?,?)
	 ON CONFLICT(id) DO UPDATE SET remote_key=excluded.remote_key,sha256=excluded.sha256,error=excluded.error,bytes=excluded.bytes,remote=excluded.remote`,
		record.ID, record.Path, record.RemoteKey, record.SHA256, record.Error, record.CreatedAt, record.Bytes, record.Remote)
	if err != nil {
		return err
	}
	return tx.Commit()
}
