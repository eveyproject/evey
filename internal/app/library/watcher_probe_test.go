package library

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// A probe event must never look like a real change. The probe fires on a timer,
// so if its own event reached the debounce/scan machinery the server would walk
// the entire library every probe round - the exact cost this whole mechanism
// exists to remove.
func TestIsProbePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/media/tv/" + library.ProbeFilePrefix + "1730000000", true},
		{"/media/movies/Some Movie (2010).mkv", false},
		{"/media/movies/.hidden.mkv", false},
		// A user file that merely CONTAINS the prefix mid-name is not ours.
		{"/media/movies/my" + library.ProbeFilePrefix + "notes.txt", false},
	}
	for _, tc := range cases {
		if got := isProbePath(tc.path); got != tc.want {
			t.Errorf("isProbePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// The happy path: the watch answers, so the library is healthy and no probe
// file survives the round.
func TestProbeOnce_EventDelivered_LeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	ws := &WatcherService{log: slog.Default(), probes: newProbeState()}

	// Stand in for the watcher loop: as soon as a probe file appears, report it.
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), library.ProbeFilePrefix) {
					ws.probes.deliver(filepath.Join(dir, e.Name()))
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	if err := ws.probeOnce(context.Background(), dir); err != nil {
		t.Fatalf("probe should succeed when the event comes back: %v", err)
	}
	assertNoProbeFiles(t, dir)
}

// The failure path is the whole point: a deaf watch must be reported. And it
// must NOT leave its probe file behind - a watcher that has been dead for a
// week would otherwise have littered the user's media library.
func TestProbeOnce_NoEvent_FailsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	ws := &WatcherService{log: slog.Default(), probes: newProbeState(), probeWait: 50 * time.Millisecond}

	err := ws.probeOnce(context.Background(), dir)
	if err == nil {
		t.Fatal("a probe whose event never arrives must fail")
	}
	assertNoProbeFiles(t, dir)
}

// A read-only library root is a legitimate setup, not a broken watch. It must
// be reported as a WRITE failure so the caller can leave the verdict alone
// rather than pinning that library to permanent full scans.
func TestProbeOnce_UnwritableDir_ReportsWriteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	ws := &WatcherService{log: slog.Default(), probes: newProbeState()}

	err := ws.probeOnce(context.Background(), dir)
	if err == nil {
		t.Fatal("an unwritable directory must report an error")
	}
	if !strings.Contains(err.Error(), "probe write") {
		t.Fatalf("must be distinguishable from a dead watch, got %v", err)
	}
}

// Health is per library, and recovery must clear it - a watch that comes back
// after a remount should stop forcing full scans.
func TestProbeState_HealthTransitions(t *testing.T) {
	p := newProbeState()
	if len(p.UnhealthyLibraries()) != 0 {
		t.Fatal("nothing probed yet, nothing unhealthy")
	}

	p.setHealth("lib-a", false)
	p.setHealth("lib-b", true)
	got := p.UnhealthyLibraries()
	if !got["lib-a"] || got["lib-b"] {
		t.Fatalf("only lib-a must be unhealthy, got %v", got)
	}

	p.setHealth("lib-a", true)
	if len(p.UnhealthyLibraries()) != 0 {
		t.Fatal("a recovered watch must stop being reported as unhealthy")
	}
}

// The returned map must be a snapshot: a caller iterating it while the prober
// updates health must not race or observe a later mutation.
func TestProbeState_UnhealthyLibrariesIsACopy(t *testing.T) {
	p := newProbeState()
	p.setHealth("lib-a", false)

	got := p.UnhealthyLibraries()
	got["lib-injected"] = true
	p.setHealth("lib-c", false)

	fresh := p.UnhealthyLibraries()
	if fresh["lib-injected"] {
		t.Fatal("mutating the returned map must not reach the prober's state")
	}
	if !fresh["lib-a"] || !fresh["lib-c"] {
		t.Fatalf("real state lost, got %v", fresh)
	}
}

func assertNoProbeFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), library.ProbeFilePrefix) {
			t.Fatalf("probe file left behind: %s", e.Name())
		}
	}
}

// Observability is a feature: these are the signals someone debugging a
// production "my new media does not appear" report has to work with, so they
// are asserted rather than assumed.
func TestProbeRound_EmitsMetricsForEachOutcome(t *testing.T) {
	dir := t.TempDir()
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{dir: "lib-dead"},
		probeWait: 50 * time.Millisecond,
	}
	metrics.WatcherProbeTotal.Reset()
	metrics.WatcherAlive.Reset()

	// Nobody answers → the round must record a dead watch, not stay silent.
	// The probe must time out on ITS OWN deadline, not on a cancelled context:
	// a cancelled context means the server is shutting down, and marking every
	// library dead on the way out would trigger a stampede of recovery scans on
	// the next boot.
	ws.probeRound(context.Background())

	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-dead", "dead")); got != 1 {
		t.Fatalf("a failed probe must be counted, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.WatcherAlive.WithLabelValues("lib-dead")); got != 0 {
		t.Fatalf("a failed probe must drive the alive gauge to 0, got %v", got)
	}
}

// A read-only root must be counted as INCONCLUSIVE, never as a dead watch:
// conflating them would make a legitimate setup look like an outage and pin
// that library to permanent full scans.
func TestProbeRound_UnwritableRootCountsInconclusive(t *testing.T) {
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{filepath.Join(t.TempDir(), "missing"): "lib-ro"},
		probeWait: 50 * time.Millisecond,
	}
	metrics.WatcherProbeTotal.Reset()

	ws.probeRound(context.Background())

	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-ro", "inconclusive")); got != 1 {
		t.Fatalf("an unwritable root must count as inconclusive, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-ro", "dead")); got != 0 {
		t.Fatalf("an unwritable root must NOT be reported as a dead watch, got %v", got)
	}
}

// A panicking round must cost ONE round, not the prober. The goroutine-level
// recover alone would swallow the panic and return, leaving every library
// frozen on its last known health for the rest of the process lifetime - and
// nothing would say so.
func TestSafeProbeRound_ContainsPanic(t *testing.T) {
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{t.TempDir(): "lib-x"},
		probeWait: 10 * time.Millisecond,
		// scanner is nil: the dead-probe path will try a recovery scan.
	}

	// Must not panic out of the round.
	ws.safeProbeRound(context.Background())

	// And the prober must still be usable afterwards.
	ws.safeProbeRound(context.Background())
}

// A network share must NEVER be credited with a passing probe. The probe is
// written by this server, so the local kernel raises the event and the probe
// passes - while writes from any other client on that share raise nothing at
// all. Trusting that green would grant the library the spaced-out backstop
// cadence and leave remotely-added media undiscovered for up to an hour.
func TestProbeRound_NetworkShareIsNeverVouchedFor(t *testing.T) {
	dir := t.TempDir()
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{dir: "lib-nas"},
		probeWait: 50 * time.Millisecond,
		networkFS: func(string) (string, bool) { return "cifs", true },
	}
	metrics.WatcherProbeTotal.Reset()
	metrics.WatcherAlive.Reset()

	// Answer the probe immediately, the way a local kernel would on a share.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), library.ProbeFilePrefix) {
					ws.probes.deliver(filepath.Join(dir, e.Name()))
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ws.probeRound(context.Background())

	if !ws.probes.UnhealthyLibraries()["lib-nas"] {
		t.Fatal("a network share must not be vouched for, even when its probe would pass")
	}
	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-nas", "network_fs")); got != 1 {
		t.Fatalf("a network share must be counted distinctly from a dead watch, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-nas", "alive")); got != 0 {
		t.Fatalf("a network share must never count as alive, got %v", got)
	}
	// No probe file may be written at all on a share - we decline before writing.
	assertNoProbeFiles(t, dir)
}

// A local filesystem must still be probed normally - the detector must not
// become a blanket opt-out.
func TestProbeRound_LocalFSStillProbed(t *testing.T) {
	dir := t.TempDir()
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{dir: "lib-local"},
		probeWait: 50 * time.Millisecond,
		networkFS: func(string) (string, bool) { return "ext4", false },
	}
	metrics.WatcherProbeTotal.Reset()

	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), library.ProbeFilePrefix) {
					ws.probes.deliver(filepath.Join(dir, e.Name()))
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ws.probeRound(context.Background())

	if ws.probes.UnhealthyLibraries()["lib-local"] {
		t.Fatal("a local filesystem whose probe answered must be vouched for")
	}
	if got := testutil.ToFloat64(metrics.WatcherProbeTotal.WithLabelValues("lib-local", "alive")); got != 1 {
		t.Fatalf("a local probe must still be counted alive, got %v", got)
	}
}
