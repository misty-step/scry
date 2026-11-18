package store

import (
	"context"
	"database/sql"
)

type transactionKey struct{}
type billingQueries interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type transactionContext struct {
	store *Store
	tx    *sql.Tx
}

func (s *Store) billingDB(ctx context.Context) billingQueries {
	if v, ok := ctx.Value(transactionKey{}).(transactionContext); ok && v.store == s {
		return v.tx
	}
	return s.db
}

// BillingTransaction serializes billing effects and their idempotency marker.
func (s *Store) BillingTransaction(ctx context.Context, fn func(context.Context) error) error {
	if v, ok := ctx.Value(transactionKey{}).(transactionContext); ok && v.store == s {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(context.WithValue(ctx, transactionKey{}, transactionContext{s, tx})); err != nil {
		return err
	}
	return tx.Commit()
}
