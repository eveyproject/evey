package main

import (
	"io"
	"log/slog"
	"runtime/debug"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

// GOMEMLIMIT is derived from the real budget (half of it, never below the
// floor where a limit only thrashes the GC), and an explicit GOMEMLIMIT env
// is left alone - the runtime already honoured it.
func TestApplyMemoryLimit(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	restore := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(restore) })

	t.Setenv("GOMEMLIMIT", "1GiB")
	sentinel := int64(123 << 20)
	debug.SetMemoryLimit(sentinel)
	applyMemoryLimit(log)
	if got := debug.SetMemoryLimit(-1); got != sentinel {
		t.Fatalf("an explicit GOMEMLIMIT must be left alone, limit = %d", got)
	}

	t.Setenv("GOMEMLIMIT", "")
	applyMemoryLimit(log)
	got := debug.SetMemoryLimit(-1)
	if got == sentinel {
		t.Skip("no memory budget readable on this host")
	}
	budget, ok := sysinfo.MemoryBudgetBytes()
	if !ok {
		t.Fatal("a limit was set without a readable budget")
	}
	if want := max(budget/memLimitFraction, memLimitFloorBytes); got != want {
		t.Errorf("limit = %d, want %d (half of the %d budget, floored)", got, want, budget)
	}
}
