package recovery

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/misty-step/scry/internal/store"
)

// Restore accepts a complete Scry archive or a closed standalone SQLite
// snapshot (optionally accompanied by snapshotPath+".manifest.json"). It
// preserves the source, never overwrites a database or SQLite sidecar, and
// publishes only after integrity/schema checks and pausing nonterminal jobs.
// The destination's parent must already exist. This does not activate a service
// or reconcile/retry restored external work; those are explicit owner actions.
func Restore(ctx context.Context, snapshotPath, destinationPath string) error {
	if snapshotPath == "" || destinationPath == "" {
		return errors.New("restore requires a snapshot and an unused destination")
	}
	if err := regularFile(snapshotPath); err != nil {
		return fmt.Errorf("read recovery source: %w", err)
	}
	destination, err := filepath.Abs(destinationPath)
	if err != nil {
		return err
	}
	if err = unusedDestination(destination); err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	work, err := os.MkdirTemp(parent, ".scry-restore-")
	if err != nil {
		return fmt.Errorf("create isolated restore workspace in destination directory: %w", err)
	}
	defer os.RemoveAll(work)
	staged := filepath.Join(work, "restoring.sqlite")
	metadata, err := extractSnapshot(ctx, snapshotPath, staged)
	if err != nil {
		return err
	}
	sourceInfo, err := inspectCompatibleDatabase(ctx, staged, true, true)
	if err != nil {
		return err
	}
	if metadata != nil && (metadata.Database.ApplicationID != sourceInfo.ApplicationID || metadata.Database.Schema != sourceInfo.Schema) {
		return errors.New("recovery manifest does not describe the embedded database application and schema")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	restored, err := store.Open(staged)
	if err != nil {
		return fmt.Errorf("open compatible isolated restore: %w", err)
	}
	prepared := filepath.Join(work, databaseEntry)
	err = restored.PauseRestoredJobs(ctx)
	if err == nil {
		// Obtain a new complete snapshot after pausing. Never move just the
		// database out from under WAL state written by PauseRestoredJobs.
		err = restored.Backup(ctx, prepared)
	}
	err = errors.Join(err, restored.Close())
	if err != nil {
		return fmt.Errorf("prepare restored database with external jobs paused: %w", err)
	}
	if _, err = inspectDatabase(ctx, prepared, true); err != nil {
		return err
	}
	if err = os.Chmod(prepared, 0600); err != nil {
		return err
	}
	file, err := os.OpenFile(prepared, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = errors.Join(file.Sync(), file.Close())
	if err != nil {
		return fmt.Errorf("persist paused restore: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = unusedDestination(destination); err != nil {
		return err
	}
	// Link is an atomic no-replace publication on the same filesystem. Rename
	// would silently overwrite a path created by a competing restore/process.
	if err = os.Link(prepared, destination); err != nil {
		return fmt.Errorf("publish restored database without overwriting: %w", err)
	}
	if err = syncDir(parent); err != nil {
		return fmt.Errorf("restored database is present but directory durability could not be confirmed: %w", err)
	}
	return nil
}

func unusedDestination(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if _, err := os.Lstat(candidate); err == nil {
			return errors.New("restore destination or a SQLite sidecar already exists; choose an unused path")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect restore destination: %w", err)
		}
	}
	return nil
}

func extractSnapshot(ctx context.Context, source, destination string) (*manifest, error) {
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	var signature [16]byte
	_, readErr := io.ReadFull(file, signature[:])
	file.Close()
	if readErr != nil {
		return nil, errors.New("recovery input is incomplete")
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	var metadata *manifest
	if string(signature[:]) == "SQLite format 3\x00" {
		metadata, err = copyClosedSnapshot(ctx, out, source)
	} else {
		zr, openErr := zip.OpenReader(source)
		if openErr != nil {
			return nil, errors.New("recovery archive is incomplete, corrupt, or not a Scry backup")
		}
		defer zr.Close()
		declared, database, metadataErr := readArchiveManifest(zr, true)
		if metadataErr != nil {
			return nil, metadataErr
		}
		metadata = &declared
		reader, openErr := database.Open()
		if openErr != nil {
			return nil, fmt.Errorf("open archived database: %w", openErr)
		}
		err = errors.Join(copyVerified(ctx, out, reader, declared), reader.Close())
	}
	if err != nil {
		return nil, err
	}
	return metadata, errors.Join(out.Sync(), out.Close())
}

func copyClosedSnapshot(ctx context.Context, out io.Writer, source string) (*manifest, error) {
	if err := noSourceSidecars(source); err != nil {
		return nil, err
	}
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var metadata *manifest
	metadataPath := source + ".manifest.json"
	if _, err = os.Lstat(metadataPath); err == nil {
		if err = regularFile(metadataPath); err != nil {
			return nil, err
		}
		metadataFile, err := os.Open(metadataPath)
		if err != nil {
			return nil, err
		}
		encoded, readErr := io.ReadAll(io.LimitReader(metadataFile, manifestLimit+1))
		err = errors.Join(readErr, metadataFile.Close())
		if err != nil || len(encoded) > manifestLimit {
			return nil, errors.New("standalone recovery manifest is unreadable or oversized")
		}
		metadata = &manifest{}
		if err = decodeCompatibleManifest(encoded, metadata, true); err != nil {
			return nil, err
		}
		if err = copyVerified(ctx, out, file, *metadata); err != nil {
			return nil, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if _, err = io.Copy(out, contextReader{ctx, file}); err != nil {
			return nil, fmt.Errorf("copy standalone recovery snapshot: %w", err)
		}
	} else {
		return nil, fmt.Errorf("inspect standalone recovery metadata: %w", err)
	}
	after, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("standalone recovery snapshot changed during restore; use a completed online backup")
	}
	return metadata, noSourceSidecars(source)
}

func noSourceSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return errors.New("standalone recovery input has SQLite sidecars; use a completed online backup, not a live database copy")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
