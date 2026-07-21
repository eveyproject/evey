package library

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{"/media/tv/" + probeFilePrefix + "1730000000", true},
		{"/media/movies/Inception (2010).mkv", false},
		{"/media/movies/.hidden.mkv", false},
		// A user file that merely CONTAINS the prefix mid-name is not ours.
		{"/media/movies/my" + probeFilePrefix + "notes.txt", false},
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
				if strings.HasPrefix(e.Name(), probeFilePrefix) {
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
	ws := &WatcherService{log: slog.Default(), probes: newProbeState()}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := ws.probeOnce(ctx, dir)
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
		if strings.HasPrefix(e.Name(), probeFilePrefix) {
			t.Fatalf("probe file left behind: %s", e.Name())
		}
	}
}
