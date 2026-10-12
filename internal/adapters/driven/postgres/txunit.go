package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// dbExecutor is the subset of *sql.DB / *sql.Tx used by repos. Lets repos run
// inside a caller-supplied transaction when one is attached to ctx.
type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txCtxKey struct{}

func ctxWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

// detachTx returns a context with any ambient transaction removed, so
// txFromCtx falls back to the pooled *sql.DB. Used by read-only fan-out queries
// that run in concurrent goroutines: database/sql forbids concurrent use of a
// single *sql.Tx, so those goroutines must each take their own pooled
// connection rather than share the caller's transaction.
func detachTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, txCtxKey{}, (*sql.Tx)(nil))
}

// txFromCtx returns the ambient tx if set, otherwise fallback.
func txFromCtx(ctx context.Context, fallback dbExecutor) dbExecutor {
	if v := ctx.Value(txCtxKey{}); v != nil {
		if tx, ok := v.(*sql.Tx); ok && tx != nil {
			return tx
		}
	}
	return fallback
}

// TxUnit is a postgres TransactionalUnit: wraps fn in BEGIN/COMMIT, rolling
// back on error or panic. Nested calls reuse the ambient tx.
type TxUnit struct {
	db *sql.DB
}

func NewTxUnit(db *sql.DB) *TxUnit {
	return &TxUnit{db: db}
}

func (t *TxUnit) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if v := ctx.Value(txCtxKey{}); v != nil {
		if _, ok := v.(*sql.Tx); ok {
			return fn(ctx)
		}
	}
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("TxUnit.Do begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(ctxWithTx(ctx, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("TxUnit.Do rollback (%v): %w", rbErr, err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("TxUnit.Do commit: %w", err)
	}
	return nil
}

var _ shared.TransactionalUnit = (*TxUnit)(nil)
