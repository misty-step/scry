package recovery

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/store"
)

const (
	archiveSuffix = ".scry-backup.zip"
	databaseEntry = "scry.sqlite"
	manifestEntry = "manifest.json"
	manifestLimit = 64 << 10
)

type databaseInfo struct {
	Schema        int    `json:"schema"`
	ApplicationID int    `json:"application_id"`
	SQLite        string `json:"sqlite"`
}

type binaryInfo struct {
	Module    string `json:"module"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	SHA256    string `json:"sha256"`
	Revision  string `json:"revision,omitempty"`
	Modified  bool   `json:"modified"`
}

// All assets are embedded in the application binary. Secrets are deliberately
// excluded: the operator must keep private environment/integration credentials
// outside the VM as well as retaining the compatible release identified here.
type manifest struct {
	Format        int          `json:"format"`
	ID            string       `json:"id"`
	CreatedAt     int64        `json:"created_at"`
	Database      databaseInfo `json:"database"`
	Bytes         int64        `json:"bytes"`
	SHA256        string       `json:"sha256"`
	Integrity     string       `json:"integrity"`
	Binary        binaryInfo   `json:"binary"`
	Assets        string       `json:"assets"`
	Configuration []string     `json:"configuration"`
	RestorePolicy string       `json:"restore_policy"`
}

// Check checks a live or closed database read-only, in one consistent read
// transaction. It never creates a missing database, migrates, or claims jobs.
// Unlike immutable snapshot inspection, this observes a live database's WAL.
func Check(ctx context.Context, path string) error {
	_, err := inspectDatabase(ctx, path, false)
	return err
}

func inspectDatabase(ctx context.Context, path string, immutable bool) (databaseInfo, error) {
	var info databaseInfo
	if err := regularFile(path); err != nil {
		return info, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return info, err
	}
	u := &url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {"ro"}}
	if immutable {
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return info, fmt.Errorf("open recovery database read-only: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA trusted_schema=OFF"); err != nil {
		return info, fmt.Errorf("disable trusted schema: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return info, fmt.Errorf("begin recovery inspection: %w", err)
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&info.ApplicationID); err != nil {
		return info, fmt.Errorf("read application identity: %w", err)
	}
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&info.Schema); err != nil {
		return info, fmt.Errorf("read database schema: %w", err)
	}
	if info.ApplicationID != store.ApplicationID || info.Schema != store.SchemaVersion {
		return info, fmt.Errorf("incompatible Scry database: application %d, schema %d; binary requires application %d, schema %d", info.ApplicationID, info.Schema, store.ApplicationID, store.SchemaVersion)
	}
	if err = tx.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&info.SQLite); err != nil {
		return info, fmt.Errorf("read SQLite version: %w", err)
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return info, fmt.Errorf("inspect database integrity: %w", err)
	}
	count := 0
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return info, fmt.Errorf("read integrity result: %w", err)
		}
		if result != "ok" {
			rows.Close()
			return info, errors.New("recovery database failed SQLite integrity_check")
		}
		count++
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return info, fmt.Errorf("finish database integrity inspection: %w", err)
	}
	if count != 1 {
		return info, errors.New("database integrity check returned no complete result")
	}
	rows, err = tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return info, fmt.Errorf("inspect database references: %w", err)
	}
	if rows.Next() {
		rows.Close()
		return info, errors.New("recovery database has broken foreign keys")
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return info, fmt.Errorf("finish reference inspection: %w", err)
	}
	return info, tx.Commit()
}

func buildMetadata(ctx context.Context) (binaryInfo, error) {
	var result binaryInfo
	if info, ok := debug.ReadBuildInfo(); ok {
		result.Module, result.Version, result.GoVersion = info.Main.Path, info.Main.Version, info.GoVersion
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				result.Revision = setting.Value
			case "vcs.modified":
				result.Modified = setting.Value == "true"
			}
		}
	}
	// The immutable release checksum identifies dirty/development builds too;
	// a VCS revision plus "modified" cannot identify their exact embedded assets.
	digest, _, err := checksum(ctx, "/proc/self/exe")
	if err != nil {
		return result, fmt.Errorf("identify compatible recovery binary: %w", err)
	}
	result.SHA256 = digest
	return result, nil
}

func writeArchive(ctx context.Context, path, database string, metadata manifest) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	for _, name := range []string{manifestEntry, databaseEntry} {
		h := &zip.FileHeader{Name: name, Method: zip.Store, Modified: time.UnixMilli(metadata.CreatedAt).UTC()}
		h.SetMode(0600)
		entry, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if name == manifestEntry {
			if _, err = entry.Write(encoded); err != nil {
				return err
			}
			continue
		}
		file, err := os.Open(database)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(entry, contextReader{ctx, file})
		closeErr := file.Close()
		if err = errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		if n != metadata.Bytes {
			return errors.New("snapshot size changed while packaging")
		}
	}
	if err = zw.Close(); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func archiveManifest(zr *zip.ReadCloser) (manifest, *zip.File, error) {
	var metadata manifest
	var database, descriptor *zip.File
	if len(zr.File) != 2 {
		return metadata, nil, errors.New("recovery archive must contain exactly its manifest and database")
	}
	for _, file := range zr.File {
		if !file.Mode().IsRegular() {
			return metadata, nil, errors.New("recovery archive contains a non-regular file")
		}
		switch file.Name {
		case databaseEntry:
			if database != nil {
				return metadata, nil, errors.New("recovery archive repeats its database")
			}
			if file.Method != zip.Store || file.CompressedSize64 != file.UncompressedSize64 {
				return metadata, nil, errors.New("recovery database must be an uncompressed format-1 archive entry")
			}
			database = file
		case manifestEntry:
			if descriptor != nil {
				return metadata, nil, errors.New("recovery archive repeats its manifest")
			}
			descriptor = file
		default:
			return metadata, nil, errors.New("recovery archive contains an unexpected entry")
		}
	}
	if database == nil || descriptor == nil || descriptor.UncompressedSize64 > manifestLimit {
		return metadata, nil, errors.New("recovery archive is incomplete or has oversized metadata")
	}
	reader, err := descriptor.Open()
	if err != nil {
		return metadata, nil, fmt.Errorf("read recovery manifest: %w", err)
	}
	encoded, readErr := io.ReadAll(io.LimitReader(reader, manifestLimit+1))
	err = errors.Join(readErr, reader.Close())
	if err != nil || len(encoded) > manifestLimit {
		return metadata, nil, errors.New("recovery manifest is corrupt or oversized")
	}
	if err = decodeManifest(encoded, &metadata); err != nil {
		return metadata, nil, err
	}
	if database.UncompressedSize64 != uint64(metadata.Bytes) {
		return metadata, nil, errors.New("recovery database size does not match its manifest")
	}
	return metadata, database, nil
}

func decodeManifest(encoded []byte, metadata *manifest) error {
	if err := json.Unmarshal(encoded, metadata); err != nil {
		return errors.New("recovery manifest is not valid JSON")
	}
	digest, err := hex.DecodeString(metadata.SHA256)
	if metadata.Format != 1 || metadata.Integrity != "ok" || metadata.Bytes <= 0 || metadata.Bytes >= (1<<63)-1 || metadata.CreatedAt <= 0 || !validID(metadata.ID) || err != nil || len(digest) != sha256.Size {
		return errors.New("recovery manifest is incomplete or has an unsupported format")
	}
	if metadata.Database.ApplicationID != store.ApplicationID || metadata.Database.Schema != store.SchemaVersion {
		return errors.New("recovery manifest requires a different application or schema version")
	}
	return nil
}

func validID(id string) bool {
	if !strings.HasPrefix(id, "scry-") {
		return false
	}
	stamp, random, ok := strings.Cut(strings.TrimPrefix(id, "scry-"), "-")
	if !ok || len(random) != 32 {
		return false
	}
	if _, err := time.Parse("20060102T150405.000000000Z", stamp); err != nil {
		return false
	}
	_, err := hex.DecodeString(random)
	return err == nil
}

func copyVerified(ctx context.Context, out io.Writer, in io.Reader, metadata manifest) error {
	digest := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, digest), contextReader{ctx, io.LimitReader(in, metadata.Bytes+1)})
	if err != nil {
		return fmt.Errorf("read complete recovery database: %w", err)
	}
	if n != metadata.Bytes || !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), metadata.SHA256) {
		return errors.New("recovery database checksum or length mismatch")
	}
	return nil
}

func checksum(ctx context.Context, path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	digest := sha256.New()
	n, err := io.Copy(digest, contextReader{ctx, file})
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), n, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("recovery input must be a regular file, not a symlink or device")
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
