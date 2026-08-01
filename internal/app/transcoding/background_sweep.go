package transcoding

import (
	"log/slog"
	"runtime/debug"
)

// sweep runs one tick of a long-lived background loop, turning a panic into a
// logged error instead of a dead server.
//
// These loops are singletons started once at construction and never respawned:
// the orchestrator's GC and throttle, the software producer's GC. They iterate
// the session maps SHARED by every active playback, so a panic in one tick -
// a nil session, an index derived from a plan that moved - unwinds a goroutine
// with no caller to propagate to and takes the whole process with it. Every
// viewer loses their stream because one sweep hit a bad session.
//
// The repository already applies this discipline to goroutines spawned per
// request or per session (hlsHandle.run says so explicitly); the loops that
// outlive them all had no net.
//
// Recovery is per TICK, not around the loop: recovering outside would end the
// loop, and a GC that silently stops leaks encoders and disk until the box
// fills. Here the bad tick is skipped, logged with its stack, and the next one
// runs.
func sweep(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in background sweep, tick skipped",
				slog.String("sweep", name),
				slog.Any("recover", r),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	fn()
}
