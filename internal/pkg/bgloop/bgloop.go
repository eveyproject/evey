// Package bgloop holds the one thing every long-lived background loop in the
// server needs and cannot express locally: a panic in a goroutine nobody waits
// on kills the whole process.
//
// The distinction that matters is between a goroutine spawned PER REQUEST -
// where the repository already recovers, and where losing the goroutine loses
// one request - and the loops started once at construction that outlive every
// request: session GC, cache eviction, filesystem watching, token cleanup,
// plugin resource monitoring. Those iterate state SHARED by everyone, so a
// panic on one bad entry takes the server down for all of them. They had no
// net at all until 2026-08-01.
package bgloop

import (
	"log/slog"
	"runtime/debug"
)

// Tick runs one iteration of a background loop, turning a panic into a logged
// error instead of a dead server.
//
// Recovery is per TICK, deliberately: recovering around the loop would END it,
// and a GC or an eviction sweep that silently stops is its own outage - the
// disk fills, the session table grows, and nothing says why. Here the bad
// iteration is skipped with its stack, and the next one runs.
//
// name identifies the loop in the log; keep it stable, it is what an operator
// greps when something sweeps badly.
func Tick(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in background loop, iteration skipped",
				slog.String("loop", name),
				slog.Any("recover", r),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	fn()
}
