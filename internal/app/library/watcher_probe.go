package library

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// watcher_probe.go - active liveness probe for the inotify watcher.
//
// The watcher had no way to know it had gone deaf. A watch can be lost long
// after it was successfully established: the kernel's max_user_watches limit is
// hit as the library grows, a filesystem is unmounted and remounted (the kernel
// invalidates the watches and nothing here re-arms them), the event queue
// overflows, or the watcher goroutine dies. In every one of those cases the
// server keeps reporting healthy real-time monitoring while silently seeing
// nothing, and new media only appears at the next full scan.
//
// So instead of trusting the watch, we test it: drop a small file in a watched
// directory, wait for our own event to come back, delete it. An event that does
// not return within the deadline means this library is not being watched, and
// the periodic scan must cover it until the probe succeeds again.
//
// ⚠️ KNOWN BLIND SPOT - do not read a passing probe as "real-time works".
// On a network mount (NFS/SMB/CIFS) the probe is written by THIS host, so the
// local kernel raises the event and the probe passes - while writes made by any
// OTHER client on that share remain completely invisible to inotify. The probe
// proves the watch is alive, not that it sees everything. That is exactly why
// the periodic full scan stays as a backstop rather than being replaced.

const (
	// probeTimeout is how long to wait for our own event. Generous: a busy
	// server can take a moment to drain the event channel, and a false
	// "unhealthy" verdict costs a full library scan.
	probeTimeout = 30 * time.Second

	// probeInterval is how often each library is probed.
	probeInterval = 5 * time.Minute
)

// isProbePath reports whether a path is one of our own probe files. Those
// events must never reach the debounce/scan machinery: the probe writes on a
// timer, so treating it as a real change would make the server scan its whole
// library every probeInterval - the opposite of the point.
func isProbePath(path string) bool {
	return library.IsProbeFile(path)
}

// probeState tracks, per library, whether the watch is answering.
type probeState struct {
	mu sync.Mutex
	// waiting maps an in-flight probe file path to the channel its event is
	// delivered on.
	waiting map[string]chan struct{}
	// unhealthy holds the libraries whose last probe went unanswered.
	unhealthy map[string]bool
}

func newProbeState() *probeState {
	return &probeState{
		waiting:   make(map[string]chan struct{}),
		unhealthy: make(map[string]bool),
	}
}

// deliver wakes the probe waiting on this path, if any. Called from the watcher
// event loop.
func (p *probeState) deliver(path string) {
	p.mu.Lock()
	ch, ok := p.waiting[path]
	if ok {
		delete(p.waiting, path)
	}
	p.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (p *probeState) arm(path string) chan struct{} {
	ch := make(chan struct{})
	p.mu.Lock()
	p.waiting[path] = ch
	p.mu.Unlock()
	return ch
}

func (p *probeState) disarm(path string) {
	p.mu.Lock()
	delete(p.waiting, path)
	p.mu.Unlock()
}

func (p *probeState) setHealth(libID string, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if healthy {
		delete(p.unhealthy, libID)
		return
	}
	p.unhealthy[libID] = true
}

// forget drops a library's verdict when its watch is removed: the next probe
// round judges the watch that replaces it, as it does at boot.
func (p *probeState) forget(libID string) {
	p.mu.Lock()
	delete(p.unhealthy, libID)
	p.mu.Unlock()
}

// UnhealthyLibraries returns the IDs whose watch failed its last probe. The
// scan loop uses it to decide which libraries still need frequent walking.
func (p *probeState) UnhealthyLibraries() map[string]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]bool, len(p.unhealthy))
	for id := range p.unhealthy {
		out[id] = true
	}
	return out
}

// probeOnce writes a probe file into dir, waits for the watcher to report it,
// and removes it. Returns nil when the event came back in time.
//
// The file is removed on EVERY path, including timeout and write failure, so a
// deaf watcher cannot accumulate droppings in someone's media library.
func (ws *WatcherService) probeOnce(ctx context.Context, dir string) error {
	name := fmt.Sprintf("%s%d", library.ProbeFilePrefix, time.Now().UnixNano())
	path := filepath.Join(dir, name)

	ch := ws.probes.arm(path)
	defer ws.probes.disarm(path)

	// O_EXCL: never clobber something that is already there.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// A read-only library root is a legitimate setup, not a broken watch.
		// Report it as inconclusive rather than unhealthy - marking it deaf
		// would pin that library to permanent full scans for no reason.
		return fmt.Errorf("probe write: %w", err)
	}
	_ = f.Close()
	defer func() {
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			ws.log.Warn("watcher probe: could not remove probe file - it may linger",
				slog.String("path", path), slog.Any("err", rmErr))
		}
	}()

	wait := ws.probeWait
	if wait <= 0 {
		wait = probeTimeout
	}
	select {
	case <-ch:
		return nil
	case <-time.After(wait):
		return fmt.Errorf("no inotify event for probe within %s", wait)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartProbe runs the liveness probe until ctx is done. Each round probes every
// watched library once and updates its health.
//
// A library that turns unhealthy is scanned immediately: the whole reason we
// care is that changes may already have been missed while the watch was dead.
func (ws *WatcherService) StartProbe(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = probeInterval
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ws.log.Error("watcher probe: panic recovered", slog.Any("panic", r))
			}
		}()
		// Say so at startup. Without this line a silent log is ambiguous: you
		// cannot tell a probe that is passing from a probe that never ran.
		ws.log.Info("watcher probe: inotify liveness probe started",
			slog.Duration("interval", interval), slog.Duration("timeout", ws.probeWait))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				ws.log.Info("watcher probe: stopped")
				return
			case <-ticker.C:
				ws.safeProbeRound(ctx)
			}
		}
	}()
}

// safeProbeRound runs one round, containing a panic to THAT round.
//
// The goroutine-level recover below is not enough on its own: it would catch
// the panic and then return, killing the prober for the rest of the process
// lifetime - silently, since nothing else reports on it. Every library would
// keep its last known health forever. Recovering per round means a bad round
// costs one round.
func (ws *WatcherService) safeProbeRound(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			ws.log.Error("watcher probe: round panicked, continuing with the next one",
				slog.Any("panic", r))
		}
	}()
	ws.probeRound(ctx)
}

// probeRound probes one directory per watched library.
func (ws *WatcherService) probeRound(ctx context.Context) {
	ws.mu.Lock()
	// One representative path per library is enough: a library whose watches
	// were dropped loses them together (same inotify instance, same limit).
	perLib := make(map[string]string, len(ws.pathToLib))
	for path, libID := range ws.pathToLib {
		if _, seen := perLib[libID]; !seen {
			perLib[libID] = path
		}
	}
	ws.mu.Unlock()

	if len(perLib) == 0 {
		// Nothing watched at all. Two very different situations share this
		// state and must not share a log level:
		//  - a library ASKED for realtime monitoring and every watch failed
		//    (failedLibs non-empty) → a fault, keep warning every round so a
		//    dead watcher can't fade into silence;
		//  - no library has monitoring enabled at all → an operator CHOICE,
		//    said once at INFO (prod 2026-07-23: warning every round produced
		//    247 of the day's 254 WARNs and buried the real signals).
		ws.mu.Lock()
		requested := len(ws.failedLibs) > 0
		notice := !requested && !ws.noWatchLogged
		if notice {
			ws.noWatchLogged = true
		}
		ws.mu.Unlock()
		switch {
		case requested:
			ws.log.Warn("watcher probe: no watched paths - real-time monitoring is not active on any library")
		case notice:
			ws.log.Info("watcher probe: no library has real-time monitoring enabled - periodic scan is the only discovery path")
		}
		return
	}

	for libID, dir := range perLib {
		// A network share defeats the probe by construction: our own write
		// raises the local event, so the probe passes while writes from any
		// other client stay invisible. Don't even write the file - just decline
		// to vouch, so this library keeps the frequent scan cadence.
		if ws.networkFS != nil {
			if fsType, isNet := ws.networkFS(dir); isNet {
				ws.probes.setHealth(libID, false)
				metrics.WatcherProbeTotal.WithLabelValues(libID, "network_fs").Inc()
				metrics.WatcherAlive.WithLabelValues(libID).Set(0)
				ws.log.Debug("watcher probe: network share - inotify cannot see other clients' writes, keeping periodic scans",
					slog.String("library_id", libID), slog.String("dir", dir), slog.String("fs_type", fsType))
				continue
			}
		}

		started := time.Now()
		err := ws.probeOnce(ctx, dir)
		elapsed := time.Since(started)
		switch {
		case err == nil:
			wasDead := ws.probes.UnhealthyLibraries()[libID]
			ws.probes.setHealth(libID, true)
			metrics.WatcherProbeTotal.WithLabelValues(libID, "alive").Inc()
			metrics.WatcherAlive.WithLabelValues(libID).Set(1)
			if wasDead {
				// Recovery is a state CHANGE - worth an Info even in a quiet log.
				ws.log.Info("watcher probe: watch is answering again, real-time monitoring restored",
					slog.String("library_id", libID), slog.Duration("rtt", elapsed))
				continue
			}
			// Steady-state success stays at Debug (it fires every round, per
			// library) but it MUST be emitted: without it there is no way to
			// tell a healthy probe from one that never started.
			ws.log.Debug("watcher probe: watch is alive",
				slog.String("library_id", libID), slog.String("dir", dir), slog.Duration("rtt", elapsed))
		case ctx.Err() != nil:
			return
		case strings.Contains(err.Error(), "probe write"):
			// Could not write (read-only mount, permissions). Inconclusive:
			// leave the previous verdict alone rather than guessing.
			//
			// Warn, not Debug: this means the probe is permanently unable to
			// vouch for this library, and an operator debugging "my new files
			// do not show up" needs to see that the health signal is blind
			// rather than green.
			metrics.WatcherProbeTotal.WithLabelValues(libID, "inconclusive").Inc()
			ws.log.Warn("watcher probe: cannot write a probe file, watch health is UNKNOWN for this library (read-only root?)",
				slog.String("library_id", libID), slog.String("dir", dir), slog.Any("err", err))
		default:
			metrics.WatcherProbeTotal.WithLabelValues(libID, "dead").Inc()
			metrics.WatcherAlive.WithLabelValues(libID).Set(0)
			ws.log.Warn("watcher probe: no event came back - treating this library as unwatched, falling back to scanning",
				slog.String("library_id", libID), slog.String("dir", dir),
				slog.Duration("waited", elapsed), slog.Any("err", err))
			ws.probes.setHealth(libID, false)
			// Changes may already have been missed while the watch was dead.
			if scanErr := ws.triggerScan(ctx, libID); scanErr != nil {
				ws.log.Warn("watcher probe: recovery scan could not start - this library may stay stale until the next pass",
					slog.String("library_id", libID), slog.Any("err", scanErr))
			} else {
				ws.log.Info("watcher probe: recovery scan started after a failed probe",
					slog.String("library_id", libID))
			}
		}
	}
}

// ProbeRoundForTest runs one probe round synchronously. Exported for the
// real-kernel integration test, which must drive a round without waiting out
// the production interval.
func (ws *WatcherService) ProbeRoundForTest(ctx context.Context) {
	ws.safeProbeRound(ctx)
}
