package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestTxFromCtx_FallbackWhenNoTx(t *testing.T) {
	ctx := context.Background()
	var fallback dbExecutor = (*sql.DB)(nil)
	if got := txFromCtx(ctx, fallback); got != fallback {
		t.Fatalf("expected fallback, got %v", got)
	}
}

func TestTxFromCtx_ReturnsFallbackWhenNilTx(t *testing.T) {
	// ctxWithTx with nil shouldn't poison: store a non-nil interface with nil tx.
	ctx := context.WithValue(context.Background(), txCtxKey{}, (*sql.Tx)(nil))
	var fallback dbExecutor = (*sql.DB)(nil)
	if got := txFromCtx(ctx, fallback); got != fallback {
		t.Fatalf("expected fallback when tx is nil, got %v", got)
	}
}

func TestTxUnit_Do_NestedReusesAmbientTx(t *testing.T) {
	// Pre-populate ctx with a non-nil *sql.Tx sentinel; Do must NOT begin a
	// new transaction (which would nil-panic on the nil *sql.DB) — it should
	// call fn directly with the same ctx.
	u := NewTxUnit(nil)
	marker := &sql.Tx{}
	ctx := ctxWithTx(context.Background(), marker)

	called := false
	err := u.Do(ctx, func(inner context.Context) error {
		called = true
		if v := inner.Value(txCtxKey{}); v != marker {
			t.Fatalf("expected ambient tx preserved")
		}
		return errors.New("boom")
	})
	if !called {
		t.Fatal("fn not called in nested Do")
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected raw fn error propagated, got %v", err)
	}
}
