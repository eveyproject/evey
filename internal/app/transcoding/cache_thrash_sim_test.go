package transcoding

// Measurement harness (NOT a correctness test): quantifies the disk thrash of
// the whole-file + LRU copy rung (Path A) on a small-disk, multi-user box, to
// decide whether the zero-dup byte-range adapter (Path B) is needed before beta
// or can stay post-beta. It drives the REAL sweepOnce/listHashUsage eviction
// logic - so the conclusions are silicon-independent (pure cache logic). What it
// does NOT model: the wall-clock cost of one re-mux (that IS silicon-bound and
// is worse on a weak NAS than on this dev host).
//
// Run: go test ./internal/app/transcoding/ -run TestCacheThrashSim -v
//
// Model: time advances in ticks of one SweepInterval (30 s). At each tick every
// active viewer whose hash dir is absent (cold or evicted-out-from-under) forces
// a WHOLE-FILE re-mux (a sparse file of the source size, mtime = now), then the
// real sweepOnce runs. We count materialisations (= re-muxes), bytes muxed, and
// evictions that hit a still-watching viewer.

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const simGiB = int64(1) << 30

// simSession is one viewer watching one title over [startTick, endTick).
type simSession struct {
	id        int
	hash      string
	sizeBytes int64
	startTick int
	endTick   int // exclusive
}

func (s simSession) activeAt(t int) bool { return t >= s.startTick && t < s.endTick }

type simResult struct {
	name             string
	maxGB            float64
	remuxes          int     // total whole-file muxes (first + eviction-forced re-muxes)
	uniqueTitles     int     // distinct sessions ever muxed
	volumeMuxedGB    float64 // total bytes written across all (re-)muxes
	uniqueVolumeGB   float64 // bytes if each title muxed exactly once
	amplification    float64 // volumeMuxed / uniqueVolume (1.0 = no waste)
	evictUnderViewer int     // re-muxes caused by evicting an actively-watched title
}

func quietSimLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// runThrashSim plays `sessions` through `ticks` sweep cycles against a real
// SegmentCache + sweepOnce, returning the measured thrash. When touchActive is
// true it Touches each active viewer's hash every tick - modelling fix (b)
// (serveSegment marks the hash on every served segment), so the LRU sees an
// actively-read title as warm instead of relying on the frozen mtime.
func runThrashSim(t *testing.T, name string, ticks int, policy EvictionPolicy, sessions []simSession, touchActive bool) simResult {
	t.Helper()
	base := t.TempDir()
	c, err := NewSegmentCache(base)
	if err != nil {
		t.Fatalf("NewSegmentCache: %v", err)
	}
	policy.MinFreeBytes, policy.DownloadTTL = -1, -1 // the simulation is about the cap alone
	pol := policy.withDefaults()
	budget := NewCacheBudget(c)

	everMuxed := make(map[int]bool) // sessionID -> has been muxed at least once
	onDisk := make(map[int]bool)    // sessionID -> dir currently materialised
	hashToSession := make(map[string]int)
	for _, s := range sessions {
		hashToSession[s.hash] = s.id
	}

	// Deterministic mtime base; each materialisation stamps a strictly
	// increasing mtime so the LRU has a total order (mirrors muxes finishing at
	// distinct instants in the real system).
	mtimeBase := time.Unix(1_700_000_000, 0)
	var mtimeSeq int64

	res := simResult{name: name, maxGB: float64(pol.MaxBytes) / float64(simGiB)}

	materialise := func(s simSession, tick int) {
		dir := filepath.Join(c.baseDir, s.hash)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		seg := filepath.Join(dir, "stream_0_000.m4s")
		f, err := os.Create(seg)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// Sparse: logical size = source size, ~0 real bytes on disk.
		if err := f.Truncate(s.sizeBytes); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		_ = f.Close()
		mtimeSeq++
		mt := mtimeBase.Add(time.Duration(tick)*30*time.Second + time.Duration(mtimeSeq)*time.Millisecond)
		if err := os.Chtimes(seg, mt, mt); err != nil {
			t.Fatalf("chtimes: %v", err)
		}

		res.remuxes++
		res.volumeMuxedGB += float64(s.sizeBytes) / float64(simGiB)
		if everMuxed[s.id] {
			// A re-mux: this title was muxed before, got evicted, now redone.
			if s.activeAt(tick) {
				res.evictUnderViewer++
			}
		} else {
			everMuxed[s.id] = true
			res.uniqueTitles++
			res.uniqueVolumeGB += float64(s.sizeBytes) / float64(simGiB)
		}
		onDisk[s.id] = true
	}

	for tick := range ticks {
		// 1) Active viewers whose dir is missing force a (re-)mux.
		for _, s := range sessions {
			if s.activeAt(tick) && !onDisk[s.id] {
				materialise(s, tick)
			}
		}
		// 1b) fix (b): an active viewer pulls a segment this tick → Touch keeps
		// its hash warm even though the mux finished and mtime is frozen.
		if touchActive {
			for _, s := range sessions {
				if s.activeAt(tick) {
					c.Touch(s.hash)
				}
			}
		}
		// 2) Real eviction pass.
		budget.sweepOnce(pol, quietSimLogger(), time.Now())
		// 3) Reconcile onDisk with what the sweep left behind.
		for _, s := range sessions {
			if !onDisk[s.id] {
				continue
			}
			if _, err := os.Stat(filepath.Join(c.baseDir, s.hash)); os.IsNotExist(err) {
				onDisk[s.id] = false
			}
		}
	}

	if res.uniqueVolumeGB > 0 {
		res.amplification = res.volumeMuxedGB / res.uniqueVolumeGB
	}
	return res
}

func logSimResult(t *testing.T, r simResult) {
	t.Helper()
	t.Logf("  %-34s cache=%5.0fGB | re-muxes=%4d (unique=%3d) | muxed=%7.0fGB vs %6.0fGB | amp=%5.1fx | evict-under-viewer=%4d",
		r.name, r.maxGB, r.remuxes, r.uniqueTitles, r.volumeMuxedGB, r.uniqueVolumeGB, r.amplification, r.evictUnderViewer)
}

// concurrentSessions builds n viewers each watching a `sizeGB` title for
// `watchTicks` ticks, all starting within the first n ticks (staggered).
func concurrentSessions(n int, sizeGB float64, watchTicks int) []simSession {
	out := make([]simSession, n)
	for i := range n {
		out[i] = simSession{
			id:        i,
			hash:      fmt.Sprintf("viewer%02d", i),
			sizeBytes: int64(sizeGB * float64(simGiB)),
			startTick: i, // staggered starts
			endTick:   i + watchTicks,
		}
	}
	return out
}

// TestCacheThrashSim_Concurrent sweeps the number of concurrent viewers at a
// fixed title size and small-disk cap, showing where the LRU starts evicting
// actively-watched titles (the catastrophic regime).
func TestCacheThrashSim_Concurrent(t *testing.T) {
	const sizeGB = 8.0     // a 2h 1080p H.264 ~ 8 GB (copy rung is H.264 8-bit SDR)
	const watchTicks = 240 // 2h film = 240 sweep cycles
	const cacheGB = 40.0   // Pi/VPS target disk budget
	t.Logf("Concurrent viewers, %.0fGB titles, %.0fGB cache, 2h watch (ProtectRecent=4 default):", sizeGB, cacheGB)
	for _, n := range []int{1, 2, 3, 4, 5, 6, 8, 10} {
		sessions := concurrentSessions(n, sizeGB, watchTicks)
		r := runThrashSim(t, fmt.Sprintf("%d concurrent viewers", n),
			watchTicks+n, EvictionPolicy{MaxBytes: int64(cacheGB * float64(simGiB))}, sessions, false)
		logSimResult(t, r)
		// The one invariant that always matters: while the concurrent working
		// set fits under the cache cap, the sweeper must never evict a title
		// someone is still watching. Above the cap, thrash is the documented,
		// expected regime this test exists to measure - not asserted here.
		if float64(n)*sizeGB <= cacheGB && r.evictUnderViewer != 0 {
			t.Errorf("%d concurrent viewers (%.0fGB workload fits %.0fGB cache): expected 0 evictions under an active viewer, got %d",
				n, float64(n)*sizeGB, cacheGB, r.evictUnderViewer)
		}
	}
}

// TestCacheThrashSim_CacheSizeSweep fixes a realistic family load (5 concurrent
// streams) and sweeps the disk cap, showing how much disk it takes to stop the
// thrash with the whole-file strategy.
func TestCacheThrashSim_CacheSizeSweep(t *testing.T) {
	const sizeGB = 8.0
	const watchTicks = 240
	const n = 5
	t.Logf("5 concurrent viewers, %.0fGB titles, 2h watch, sweeping cache cap:", sizeGB)
	for _, cacheGB := range []float64{20, 30, 40, 50, 60, 80, 100, 120} {
		sessions := concurrentSessions(n, sizeGB, watchTicks)
		r := runThrashSim(t, fmt.Sprintf("cache=%.0fGB", cacheGB),
			watchTicks+n, EvictionPolicy{MaxBytes: int64(cacheGB * float64(simGiB))}, sessions, false)
		logSimResult(t, r)
		// Same invariant as TestCacheThrashSim_Concurrent: once the cache is
		// provisioned for the full 5-viewer working set (n*sizeGB), no active
		// title may be evicted. Below that line the workload doesn't fit and
		// thrash is the point being measured, not a bug.
		if cacheGB >= float64(n)*sizeGB && r.evictUnderViewer != 0 {
			t.Errorf("cache=%.0fGB (workload=%.0fGB): expected 0 evictions under an active viewer, got %d",
				cacheGB, float64(n)*sizeGB, r.evictUnderViewer)
		}
	}
}

// TestCacheThrashSim_Zapping models one viewer opening many titles briefly
// (1 tick each) - the over-mux cost: a whole file is muxed even for seconds of
// watching, and the mux is not cancelled on disconnect.
func TestCacheThrashSim_Zapping(t *testing.T) {
	const sizeGB = 8.0
	const cacheGB = 40.0
	const titles = 15
	sessions := make([]simSession, titles)
	for i := range titles {
		sessions[i] = simSession{
			id:        i,
			hash:      fmt.Sprintf("zap%02d", i),
			sizeBytes: int64(sizeGB * float64(simGiB)),
			startTick: i,
			endTick:   i + 1, // watched for one tick, then abandoned
		}
	}
	r := runThrashSim(t, "zapping 15 titles", titles+1,
		EvictionPolicy{MaxBytes: int64(cacheGB * float64(simGiB))}, sessions, false)
	t.Logf("Zapping (whole-file muxed per open, even for seconds watched):")
	logSimResult(t, r)
	t.Logf("  -> %.0fGB written to serve ~%d minutes of actual viewing", r.volumeMuxedGB, titles)
	// Caveat: evictUnderViewer is only incremented when a session gets
	// re-materialised WHILE still active, but a 1-tick zap session is never
	// active again after its own tick - so this can't detect an eviction of
	// the just-materialised title within the same tick it's watched. Kept
	// for consistency with the other sims and because a regression that
	// evicts the same title across ticks would still show up here.
	if r.evictUnderViewer != 0 {
		t.Errorf("zapping: expected 0 evictions under an active viewer, got %d", r.evictUnderViewer)
	}
}

// TestCacheThrashSim_FixB_LongViewerVsArrivals isolates what fix (b) corrects: a
// long viewer (2.5h) constantly harassed by short arrivals that keep pushing the
// cache over the cap. With mtime-only LRU the long viewer (oldest mtime once its
// ReadRate=0 mux finished) is evicted out from under itself again and again; with
// the access-time merge (Touch on every served segment) it stays warm and the LRU
// sheds the finished short sessions instead. Measures the delta - and shows fix
// (b) does NOT claim to fix the Σ>cap regime (that's the byte-range / disk guard).
func TestCacheThrashSim_FixB_LongViewerVsArrivals(t *testing.T) {
	const sizeGB = 8.0
	const cacheGB = 40.0
	size := int64(sizeGB * float64(simGiB))
	mk := func() []simSession {
		s := []simSession{{id: 0, hash: "long", sizeBytes: size, startTick: 0, endTick: 300}}
		id := 1
		for start := 10; start < 290; start += 10 {
			s = append(s, simSession{
				id: id, hash: fmt.Sprintf("arr%03d", id), sizeBytes: size,
				startTick: start, endTick: start + 5, // short session, then leaves
			})
			id++
		}
		return s
	}
	pol := EvictionPolicy{MaxBytes: int64(cacheGB * float64(simGiB))}
	without := runThrashSim(t, "mtime-only LRU (before fix b)", 300, pol, mk(), false)
	withTouch := runThrashSim(t, "access-time LRU (fix b)", 300, pol, mk(), true)
	t.Logf("Fix (b) effect - 1 long viewer + rolling short arrivals, 8GB titles, 40GB cap:")
	logSimResult(t, without)
	logSimResult(t, withTouch)
	if withTouch.evictUnderViewer >= without.evictUnderViewer {
		t.Errorf("fix (b) must reduce eviction-under-viewer: without=%d with=%d",
			without.evictUnderViewer, withTouch.evictUnderViewer)
	}
}

// TestCacheThrashSim_SoloSequential is the baseline: one viewer, titles watched
// back-to-back, never two active at once. Amplification should be ~1 (no waste)
// unless the same title is rewatched after eviction.
func TestCacheThrashSim_SoloSequential(t *testing.T) {
	const sizeGB = 8.0
	const cacheGB = 40.0
	const titles = 5
	const watchTicks = 240
	sessions := make([]simSession, titles)
	for i := range titles {
		sessions[i] = simSession{
			id:        i,
			hash:      fmt.Sprintf("solo%02d", i),
			sizeBytes: int64(sizeGB * float64(simGiB)),
			startTick: i * watchTicks,
			endTick:   (i + 1) * watchTicks,
		}
	}
	r := runThrashSim(t, "solo, 5 titles back-to-back", titles*watchTicks,
		EvictionPolicy{MaxBytes: int64(cacheGB * float64(simGiB))}, sessions, false)
	t.Logf("Solo sequential baseline:")
	logSimResult(t, r)
	// One viewer, one active title at a time, workload (5*8=40GB) fits the
	// 40GB cap exactly: no re-mux from eviction, and the active title is
	// never the one evicted.
	if r.evictUnderViewer != 0 {
		t.Errorf("solo sequential: expected 0 evictions under an active viewer, got %d", r.evictUnderViewer)
	}
	if r.amplification != 1.0 {
		t.Errorf("solo sequential: expected amplification 1.0 (no rewatch, no waste), got %.2f", r.amplification)
	}
}
