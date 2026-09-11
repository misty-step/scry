package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CheckSchema validates the complete known schema, not only a version integer.
// Version one is readable and migratable by this binary; only Open migrates it.
// The reference database is isolated in memory and never reads or writes user data.
func CheckSchema(ctx context.Context, tx *sql.Tx, version int) error {
	if version != 1 && version != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema %d", ErrInvalid, version)
	}
	reference, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer reference.Close()
	reference.SetMaxOpenConns(1)
	if _, err = reference.ExecContext(ctx, schemaV1); err != nil {
		return err
	}
	if version == SchemaVersion {
		if _, err = reference.ExecContext(ctx, schemaV2); err != nil {
			return err
		}
	}
	const query = "SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name"
	expected, err := reference.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer expected.Close()
	actual, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer actual.Close()
	for expected.Next() {
		var want, got [4]string
		if err = expected.Scan(&want[0], &want[1], &want[2], &want[3]); err != nil {
			return err
		}
		if !actual.Next() {
			return fmt.Errorf("%w: incomplete schema %d", ErrInvalid, version)
		}
		if err = actual.Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("%w: incompatible schema object %q", ErrInvalid, got[1])
		}
	}
	if actual.Next() {
		return fmt.Errorf("%w: unexpected schema objects", ErrInvalid)
	}
	return errors.Join(expected.Err(), actual.Err())
}
