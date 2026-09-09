package recovery

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/misty-step/scry/internal/store"
)

// RemoteURL is a private object-prefix endpoint, not a public bucket URL or a
// pre-signed object URL. Each immutable key is appended to this prefix. A
// private exe integration may sign PUT/GET requests without a VM-held token.
// Keep bounds only recognized, complete local archives. This client never
// lists or deletes remote objects; configure and prove remote retention in the
// object service independently before relying on it.
type Config struct {
	Dir         string
	RemoteURL   string
	RemoteToken string
	Interval    time.Duration
	Keep        int
	HTTPClient  *http.Client
}

type Manager struct {
	store     *store.Store
	cfg       Config
	gate      chan struct{}
	remote    *url.URL
	client    *http.Client
	configErr error
	binary    *binaryInfo
}

// New does not start work. Configuration errors become visible backup failures
// rather than preventing the owner from opening the ordinary review surface.
func New(s *store.Store, cfg Config) *Manager {
	if cfg.Dir == "" {
		cfg.Dir = "backups"
		if s != nil {
			cfg.Dir = filepath.Join(filepath.Dir(s.Path()), "backups")
		}
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.Keep <= 0 {
		cfg.Keep = 30
	}
	m := &Manager{store: s, cfg: cfg, gate: make(chan struct{}, 1)}
	m.remote, m.configErr = remoteEndpoint(cfg.RemoteURL)
	m.client = privateClient(cfg.HTTPClient)
	return m
}

// Run attempts a backup at startup and then at the configured interval. A
// failed backup is durably recorded and logged, but never kills review or
// disables later backup attempts. Cancellation stops both HTTP and local work.
func (m *Manager) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := m.Backup(ctx); err != nil && ctx.Err() == nil {
			slog.Error("recovery backup failed", "error", err)
		}
		timer := time.NewTimer(m.cfg.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// Backup publishes one local archive atomically, then uploads and reads back
// that exact archive if a remote sink is configured. A returned error can
// accompany a usable local Path; only Remote=true proves checksum readback.
// In-process and advisory filesystem locks serialize scheduled and CLI work.
func (m *Manager) Backup(ctx context.Context) (record store.BackupRecord, resultErr error) {
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	case <-ctx.Done():
		return record, ctx.Err()
	}
	if m.store == nil {
		return record, errors.New("backup store is not configured")
	}
	now := time.Now().UTC()
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return record, fmt.Errorf("allocate backup identity: %w", err)
	}
	record.ID = "scry-" + now.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(entropy[:])
	record.CreatedAt = now.UnixMilli()
	defer func() {
		if resultErr != nil {
			record.Error = resultErr.Error()
			// An errored attempt is never advertised as remote-complete, even
			// when the error followed upload during local retention cleanup.
			record.Remote = false
		}
		// Preserve interrupted-upload status even when shutdown canceled the
		// operation, but do not let health recording hold shutdown indefinitely.
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := m.store.RecordBackup(recordCtx, record); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("record backup status: %w", err))
		}
	}()
	if err := os.MkdirAll(m.cfg.Dir, 0700); err != nil {
		return record, fmt.Errorf("create private backup directory: %w", err)
	}
	if err := os.Chmod(m.cfg.Dir, 0700); err != nil {
		return record, fmt.Errorf("restrict backup directory: %w", err)
	}
	info, err := os.Lstat(m.cfg.Dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return record, errors.New("backup directory must be a private directory (mode 0700), not a symlink")
	}
	dir, err := filepath.Abs(m.cfg.Dir)
	if err != nil {
		return record, err
	}
	unlock, err := lockBackups(ctx, dir)
	if err != nil {
		return record, err
	}
	defer unlock()
	work, err := os.MkdirTemp(dir, ".scry-backup-")
	if err != nil {
		return record, fmt.Errorf("create isolated backup workspace: %w", err)
	}
	defer os.RemoveAll(work)
	database := filepath.Join(work, databaseEntry)
	if err = m.store.Backup(ctx, database); err != nil {
		return record, fmt.Errorf("create consistent SQLite snapshot: %w", err)
	}
	if err = os.Chmod(database, 0600); err != nil {
		return record, err
	}
	dbInfo, err := inspectDatabase(ctx, database, true)
	if err != nil {
		return record, err
	}
	digest, size, err := checksum(ctx, database)
	if err != nil {
		return record, fmt.Errorf("checksum SQLite snapshot: %w", err)
	}
	if m.binary == nil {
		binary, err := buildMetadata(ctx)
		if err != nil {
			return record, err
		}
		m.binary = &binary
	}
	metadata := manifest{
		Format: 1, ID: record.ID, CreatedAt: record.CreatedAt,
		Database: dbInfo, Bytes: size, SHA256: digest, Integrity: "ok",
		Binary: *m.binary, Assets: "embedded in the compatible application binary; no separate uploaded files",
		Configuration: []string{"SCRY_DB", "SCRY_ADDR", "SCRY_MODE", "SCRY_OWNER_ID", "SCRY_SECRET", "SCRY_BASE_URL", "SCRY_TRUSTED_PROXY_IPS", "SCRY_MODEL_ENDPOINT", "SCRY_MODEL_API_KEY", "SCRY_MODEL", "SCRY_GENERATION_DAILY_BUDGET_MICROS", "SCRY_GENERATION_RESERVATION_MICROS", "SCRY_BACKUP_DIR", "SCRY_BACKUP_REMOTE_URL", "SCRY_BACKUP_REMOTE_TOKEN", "SCRY_BACKUP_INTERVAL", "SCRY_BACKUP_KEEP"},
		RestorePolicy: "Restore into an unused path with a compatible binary. Nonterminal jobs are paused. Keep private environment/integration credentials and the compatible release off-VM separately; no credentials are in this archive.",
	}
	archive := filepath.Join(work, "snapshot.zip")
	if err = writeArchive(ctx, archive, database, metadata); err != nil {
		return record, fmt.Errorf("complete recovery archive: %w", err)
	}
	record.SHA256, record.Bytes, err = checksum(ctx, archive)
	if err != nil {
		return record, fmt.Errorf("checksum recovery archive: %w", err)
	}
	completed := filepath.Join(dir, record.ID+archiveSuffix)
	if err = os.Link(archive, completed); err != nil {
		return record, fmt.Errorf("publish unique recovery archive: %w", err)
	}
	record.Path = completed
	if err = syncDir(dir); err != nil {
		return record, fmt.Errorf("persist recovery directory: %w", err)
	}
	remoteErr := m.configErr
	if remoteErr == nil && m.remote != nil {
		record.RemoteKey = filepath.Base(completed)
		remoteErr = m.upload(ctx, record)
		record.Remote = remoteErr == nil
	}
	retentionErr := m.prune(ctx, dir, completed)
	return record, errors.Join(remoteErr, retentionErr)
}

func lockBackups(ctx context.Context, dir string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(dir, ".backup.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open backup lock: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				file.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, fmt.Errorf("lock backup work: %w", err)
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Unknown, corrupt, foreign-schema and interrupted artifacts are not ours to
// delete. Only expired recognized archives pay for a full checksum read. The
// just-completed valid archive is protected even if the clock moved backwards.
// No remote delete is ever issued, including after an upload/readback failure.
func (m *Manager) prune(ctx context.Context, dir, newest string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read local recovery retention: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), archiveSuffix) && validID(strings.TrimSuffix(entry.Name(), archiveSuffix)) {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	complete := 0
	removed := false
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, name)
		zr, err := zip.OpenReader(path)
		if err != nil {
			continue
		}
		metadata, database, err := archiveManifest(zr)
		if err != nil || metadata.ID+archiveSuffix != name {
			zr.Close()
			continue
		}
		complete++
		if complete <= m.cfg.Keep || path == newest {
			zr.Close()
			continue
		}
		reader, err := database.Open()
		if err != nil {
			zr.Close()
			continue
		}
		err = copyVerified(ctx, io.Discard, reader, metadata)
		err = errors.Join(err, reader.Close(), zr.Close())
		if err != nil {
			continue
		}
		if err = os.Remove(path); err != nil {
			return fmt.Errorf("remove expired local recovery archive: %w", err)
		}
		removed = true
	}
	if removed {
		return syncDir(dir)
	}
	return ctx.Err()
}
