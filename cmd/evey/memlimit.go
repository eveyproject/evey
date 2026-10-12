package main

import (
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

const (
	// The Go heap gets half the container budget: the rest belongs to the
	// embedded Meilisearch (its indexing arena is capped at ~20% by the
	// entrypoint), the per-plugin node runtimes and the ffmpeg processes the
	// transcoder spawns. Without a limit the GC happily lets the heap grow
	// toward the whole budget before collecting.
	memLimitFraction = 2 // divisor: budget / 2

	// Below this a GOMEMLIMIT causes GC thrash instead of savings; tiny
	// budgets are better served by the low-memory behaviors than by an
	// aggressive heap target.
	memLimitFloorBytes = 256 << 20
)

// applyMemoryLimit derives GOMEMLIMIT from the container's real memory budget
// (cgroup limit, else host RAM) so an idle or loaded server converges to a
// heap size proportional to the machine it runs on - a 2 GB NAS and a 32 GB
// rig should not share GC behavior. An explicit GOMEMLIMIT env always wins:
// the Go runtime already honored it, so this no-ops.
func applyMemoryLimit(log *slog.Logger) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	budget, ok := sysinfo.MemoryBudgetBytes()
	if !ok {
		return
	}
	limit := max(budget/memLimitFraction, memLimitFloorBytes)
	debug.SetMemoryLimit(limit)
	log.Info("memory budget detected",
		slog.Int64("budget_mb", budget>>20),
		slog.Int64("gomemlimit_mb", limit>>20))
}
