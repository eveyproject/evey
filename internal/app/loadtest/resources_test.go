package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pprofprofile "github.com/google/pprof/profile"
)

// modelledMachine is a counterSource whose load is a FUNCTION OF THE VIEWERS,
// integrated at the instant the sampler reads it. No goroutine, no clock of its
// own, no files: a test that models a server heating up must not also be racing
// the scheduler, or it measures the scheduler.
//
// cpuPctPerViewer is how many points of CPU each admitted viewer adds.
type modelledMachine struct {
	cores           float64
	cpuPctPerViewer float64
	memPerViewer    int64
	cacheBytes      int64
	memLimit        int64
	diskPerViewer   int64 // bytes/sec per viewer
	viewers         func() int

	mu      sync.Mutex
	last    time.Time
	cpuAcc  int64 // integrated CPU microseconds
	diskAcc int64 // integrated bytes read
}

func (m *modelledMachine) cpuAllowance() float64 { return m.cores }

// integrate advances the cumulative counters to now, at the load the current
// viewer count implies. Called from the sampler's own goroutine, so the
// integration lands exactly when the measurement does.
func (m *modelledMachine) integrate() (cpuUsec, diskBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if m.last.IsZero() {
		m.last = now
		return m.cpuAcc, m.diskAcc
	}
	dt := now.Sub(m.last).Seconds()
	m.last = now

	v := float64(m.viewers())
	// CPU time burned = load (as a fraction of the allowance) x cores x elapsed.
	pct := min(m.cpuPctPerViewer*v, 99)
	m.cpuAcc += int64(pct / 100 * m.cores * dt * 1e6)
	m.diskAcc += int64(float64(m.diskPerViewer) * v * dt)
	return m.cpuAcc, m.diskAcc
}

func (m *modelledMachine) cpuUsageUsec() int64 {
	cpu, _ := m.integrate()
	return cpu
}

func (m *modelledMachine) diskReadBytes() (int64, bool) {
	_, disk := m.integrate()
	return disk, true
}

// A modelled machine whose code asks for exactly what the disk delivers: the
// readahead tax is a property of real kernels, not of this model.
func (m *modelledMachine) appReadBytes() int64 {
	_, disk := m.integrate()
	return disk
}

func (m *modelledMachine) memory() (used, cache, limit int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base := int64(256 << 20) // an idle server's own footprint
	// The page cache grows with every film read and is NOT consumption: a
	// modelled machine says so, so a test can prove the report ignores it.
	cache = m.cacheBytes
	return base + m.memPerViewer*int64(m.viewers()), cache, m.memLimit
}

// A saturated CPU is a capacity verdict: the run stops there, names the CPU as
// the bottleneck, reports the tier below as what the server sustains, and says
// what one more viewer would have cost.
func TestRunner_CPUSaturationStopsTheRunAndNamesTheBottleneck(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	// 25 points of CPU per viewer on a single core: the 4th viewer crosses the
	// 92% saturation line, so the server sustains 3. Driving the load off the
	// admitted-viewer count (not off elapsed time) locks the CPU curve to the
	// ramp, whatever the scheduler does under -race.
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	// The load must follow the LIVE audience, which rises AND falls as the
	// bisection sheds viewers - a cumulative request counter would keep the fake
	// machine pinned at its peak forever.
	machine := &modelledMachine{
		cores: 1, cpuPctPerViewer: 25,
		memPerViewer: 64 << 20, memLimit: 8 << 30,
		diskPerViewer: 5 << 20,
		viewers:       func() int { return r.Status().Viewers },
	}
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, RampEvery: 600 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, MaxDuration: 30 * time.Second, ResourceSampleEvery: 50 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopCPUSaturated {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopCPUSaturated, report.StopDetail)
	}
	if report.StopDetail == "" {
		t.Fatalf("a saturation verdict must say which resource ran out and at how many viewers")
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained = %d, want 3 (the CPU gave out on the 4th viewer)", report.SustainedViewers)
	}
	// The report carries the evidence behind its own verdict.
	if cpu := report.Resources.CPUPercentAvg; cpu < 65 || cpu > 85 {
		t.Fatalf("CPU avg at the sustained tier = %.1f%%, want ~75%%", cpu)
	}
	// And it says what one more viewer costs: ~25 points of CPU, ~64 MiB.
	if !report.HasPerViewerCost {
		t.Fatalf("no per-viewer cost measured")
	}
	if c := report.PerViewerCost.CPUPercentAvg; c < 18 || c > 32 {
		t.Fatalf("per-viewer CPU cost = %.1f points, want ~25", c)
	}
	if mb := report.PerViewerCost.MemUsedBytesAvg / (1 << 20); mb < 48 || mb > 80 {
		t.Fatalf("per-viewer memory cost = %d MiB, want ~64", mb)
	}
	if report.Resources.DiskReadBytesPerSecAvg <= 0 {
		t.Fatalf("disk read not measured: a viewer reading a file must show up as I/O")
	}
}

// Memory can be the bottleneck instead of the CPU, and the verdict must say so:
// a machine with plenty of CPU but a tight limit runs out of RAM first.
func TestRunner_MemorySaturationIsItsOwnVerdict(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	// Idle floor is 256 MiB; each viewer adds 256 MiB against a 1 GiB limit, so
	// the 3rd viewer crosses 90%. The CPU stays cold throughout.
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	machine := &modelledMachine{
		cores: 16, cpuPctPerViewer: 1,
		memPerViewer: 256 << 20, memLimit: 1 << 30,
		viewers: func() int { return r.Status().Viewers },
	}
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, RampEvery: 600 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, MaxDuration: 30 * time.Second, ResourceSampleEvery: 50 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopMemorySaturated {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopMemorySaturated, report.StopDetail)
	}
	if report.Resources.CPUPercentAvg > 20 {
		t.Fatalf("CPU avg = %.1f%%: this machine ran out of MEMORY, the CPU was never the problem", report.Resources.CPUPercentAvg)
	}
}

// An idle machine must never trip a saturation verdict: the ramp climbs to its
// configured ceiling, and the report still carries the resource evidence.
func TestRunner_IdleMachineReachesMaxViewersAndStillReportsResources(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(4)}, zeroActive, testLogger())
	machine := &modelledMachine{
		cores: 8, cpuPctPerViewer: 2,
		memPerViewer: 32 << 20, memLimit: 16 << 30,
		diskPerViewer: 2 << 20,
		viewers:       func() int { return r.Status().Viewers },
	}
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 3, RampEvery: 400 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, MaxDuration: 30 * time.Second, ResourceSampleEvery: 50 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.Resources.Samples == 0 {
		t.Fatalf("no resource samples recorded: the report must always say what the run consumed")
	}
	if report.Resources.MemLimitBytes == 0 {
		t.Fatalf("memory limit not reported")
	}
	if report.Resources.ServedBytesPerSecAvg <= 0 {
		t.Fatalf("served throughput = 0: the viewers pulled bytes, they must be counted")
	}
}

// --- cgroup reading ---------------------------------------------------------

// stageCgroup writes a cgroup v2 tree the real cgroupSource can read.
func stageCgroup(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	cg := filepath.Join(root, "sys/fs/cgroup")
	if err := os.MkdirAll(cg, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(cg, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// The CPU percentage is a share of the server's ALLOWANCE (the cgroup quota),
// not of the host's cores: a container capped at 2 cores on a 16-core host
// saturates at 2, exactly as the operator configured it.
func TestCgroupSource_ReadsQuotaUsageMemoryAndIO(t *testing.T) {
	root := stageCgroup(t, map[string]string{
		"cpu.max":        "200000 100000\n", // 2 cores
		"cpu.stat":       "usage_usec 4200000\nuser_usec 1\n",
		"memory.current": "1073741824\n",
		// memory.stat is what readMemory actually trusts: memory.current lumps the
		// page cache in with the process's own memory, and a load test fills that
		// cache with films.
		"memory.stat": "anon 805306368\nfile 3221225472\nkernel 268435456\nslab 12345\n",
		"memory.max":  "4294967296\n",
	})
	// The disk figure is the machine's block counters over the devices the
	// library was mapped to: sda and sdb here, sdc is someone else's.
	for _, d := range []string{"sda", "sdb", "sdc"} {
		if err := os.MkdirAll(filepath.Join(root, "sys/block", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	stats := "8 0 sda 10 0 2 50 0 0 0 0 0 90 0\n" +
		"8 16 sdb 10 0 4 50 0 0 0 0 0 90 0\n" +
		"8 32 sdc 10 0 1000 50 0 0 0 0 0 90 0\n"
	if err := os.WriteFile(filepath.Join(root, "proc/diskstats"), []byte(stats), 0o600); err != nil {
		t.Fatal(err)
	}
	src := cgroupSource{fsRoot: root, devices: []string{"sda", "sdb"}}

	if got := src.cpuAllowance(); got != 2 {
		t.Fatalf("cpuAllowance = %v, want 2 (the cgroup quota, not the host's cores)", got)
	}
	if got := src.cpuUsageUsec(); got != 4_200_000 {
		t.Fatalf("cpuUsageUsec = %d, want 4200000", got)
	}
	// anon (768 MiB) + kernel (256 MiB) = 1 GiB genuinely held. The 3 GiB of
	// `file` is page cache - films the kernel will hand straight back - and must
	// NOT be counted as consumption, or a load test reports a server at its
	// memory ceiling when it is nowhere near it.
	used, cache, limit := src.memory()
	if used != 1<<30 {
		t.Fatalf("memory used = %d MiB, want 1024 (anon+kernel, NOT the page cache)", used>>20)
	}
	if cache != 3<<30 {
		t.Fatalf("memory cache = %d MiB, want 3072 (reported, but apart from consumption)", cache>>20)
	}
	if limit != 4<<30 {
		t.Fatalf("memory limit = %d, want 4GiB", limit)
	}
	// Disk reads sum the library's devices only: 2 + 4 sectors of 512 bytes.
	if got, measured := src.diskReadBytes(); !measured || got != 6*512 {
		t.Fatalf("diskReadBytes = %d (measured %v), want 3072 summed over sda and sdb", got, measured)
	}
	// No device mapped: the figure is absent, never a zero that reads as "idle".
	if got, measured := (cgroupSource{fsRoot: root}).diskReadBytes(); measured || got != 0 {
		t.Fatalf("unmapped diskReadBytes = %d (measured %v), want absent", got, measured)
	}
	// A hook that nothing wired reads as 0; the sampler then says "not measured".
	if got := src.appReadBytes(); got != 0 {
		t.Fatalf("appReadBytes without a counter = %d, want 0", got)
	}
}

// An unlimited cgroup ("max") is not a zero ceiling: the machine's own RAM is
// the real limit, and reporting 0 would make every run look saturated.
func TestCgroupSource_UnlimitedMemoryFallsBackToMachineRAM(t *testing.T) {
	root := stageCgroup(t, map[string]string{
		"memory.current": "1073741824\n",
		"memory.max":     "max\n",
	})
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o755); err != nil {
		t.Fatalf("mkdir proc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc/meminfo"),
		[]byte("MemTotal:       16000000 kB\nMemAvailable:    8000000 kB\n"), 0o600); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	_, _, limit := cgroupSource{fsRoot: root}.memory()
	if limit != 16_000_000*1024 {
		t.Fatalf("limit = %d, want the machine's total RAM (16000000 kB)", limit)
	}
}

// A tier's warm-up samples are start-up transients (ffmpeg cold start, buffer
// burst) and must never be counted, nor trip a saturation verdict.
func TestResourceState_DiscardsWarmupSamples(t *testing.T) {
	start := time.Now()
	rs := newResourceState(30*time.Second, start)

	hot := ResourceSample{CPUPercent: 99, MemUsedBytes: 1, MemLimitBytes: 100}
	for i := range saturationSamples + 2 {
		at := start.Add(time.Duration(i) * time.Second) // all inside the warm-up
		if reason, _ := rs.observe(1, hot, at); reason != "" {
			t.Fatalf("sample %d inside the warm-up tripped %s: transients are not verdicts", i, reason)
		}
	}
	if got := rs.stats(1).Samples; got != 0 {
		t.Fatalf("warm-up samples recorded: %d, want 0", got)
	}

	// Past the warm-up, the same readings do count - and do saturate.
	var reason StopReason
	for i := range saturationSamples {
		at := start.Add(31*time.Second + time.Duration(i)*time.Second)
		reason, _ = rs.observe(1, hot, at)
	}
	if reason != StopCPUSaturated {
		t.Fatalf("steady-state CPU at 99%% did not saturate: got %q", reason)
	}
}

// A single spike is not a capacity verdict: an encoder starting or a GC pause
// must not end the run.
func TestResourceState_SingleSpikeIsNotSaturation(t *testing.T) {
	start := time.Now()
	rs := newResourceState(0, start)

	hot := ResourceSample{CPUPercent: 99}
	cool := ResourceSample{CPUPercent: 30}
	at := start
	for range 5 {
		at = at.Add(time.Second)
		if reason, _ := rs.observe(1, hot, at); reason != "" {
			// One hot sample then a cool one: the run must survive.
			if reason == StopCPUSaturated {
				break
			}
		}
		at = at.Add(time.Second)
		if reason, _ := rs.observe(1, cool, at); reason != "" {
			t.Fatalf("a cool sample reported %s", reason)
		}
	}
	// Alternating hot/cool never reaches saturationSamples in a row.
	if rs.cpuOverRun >= saturationSamples {
		t.Fatalf("alternating spikes counted as sustained saturation")
	}
}

// The ceiling of a media server is usually neither its CPU nor its RAM: it is
// how fast it can READ the films. That bottleneck is invisible to every gauge -
// the CPU idles, memory sits still - and shows up only as the bytes-out curve
// going flat while viewers keep arriving. The run must catch it and name it.
func TestResourceState_FlatThroughputIsTheStorageCeiling(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()

	// 10 viewers pull 100 MB/s. 20 viewers pull... still 100 MB/s: the storage
	// is already giving everything it has, and the newcomers only take a slice
	// of the same pie. Both tiers measured under full windows, disk visibly
	// delivering the bytes - the honest version of this verdict.
	rs.startTier(10, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(10, ResourceSample{CPUPercent: 12, ServedBytesPerSec: 100 << 20, DiskReadBytesPerSec: 100 << 20}, at)
	}
	rs.startTier(20, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(20, ResourceSample{CPUPercent: 12, ServedBytesPerSec: 100 << 20, DiskReadBytesPerSec: 100 << 20}, at)
	}

	plateaued, ceiling := rs.throughputPlateaued(20)
	if !plateaued {
		t.Fatalf("doubling the viewers without pushing a single extra byte is a saturated storage path; the run must say so")
	}
	if ceiling != 100<<20 {
		t.Fatalf("ceiling = %d, want 100 MiB/s (what the box could not exceed)", ceiling)
	}
}

// A tier that genuinely serves MORE data has not hit any ceiling, however busy
// the box looks.
func TestResourceState_GrowingThroughputIsNotAPlateau(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()
	rs.startTier(10, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(10, ResourceSample{ServedBytesPerSec: 100 << 20, DiskReadBytesPerSec: 100 << 20}, at)
	}
	rs.startTier(20, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(20, ResourceSample{ServedBytesPerSec: 190 << 20, DiskReadBytesPerSec: 190 << 20}, at)
	}
	if plateaued, _ := rs.throughputPlateaued(20); plateaued {
		t.Fatalf("throughput nearly doubled with the viewers: nothing is saturated")
	}
}

// THE REGRESSION. Straight from a production run: 21 viewers moved 511 Mb/s, and
// 22 viewers moved 535 Mb/s - the server scaling perfectly, on a box at 25% CPU,
// 36% memory and half its measured disk bandwidth.
//
// The old rule demanded a flat 5% more throughput from any tier, and one extra
// viewer out of 21 can only ever add 1/21 = 4.76%. So the run called it a
// plateau, blamed the storage, and stopped - and it stopped at the same ~21
// viewers on a one-disk dev box and a two-disk production server, because the
// number was coming from the threshold, not from the hardware.
//
// A single viewer joining a large audience CANNOT move the needle 5%. Any rule
// that requires it to is measuring its own arithmetic.
func TestResourceState_OneMoreViewerInALargeAudienceIsNotAPlateau(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()

	const perViewer = 24 << 20 // each viewer pulls its bitrate, and no more

	rs.startTier(21, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(21, ResourceSample{CPUPercent: 25, ServedBytesPerSec: 21 * perViewer, DiskReadBytesPerSec: 21 * perViewer}, at)
	}
	// The 22nd viewer is served exactly as well as the other 21: +4.76%.
	rs.startTier(22, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(22, ResourceSample{CPUPercent: 25, ServedBytesPerSec: 22 * perViewer, DiskReadBytesPerSec: 22 * perViewer}, at)
	}

	if plateaued, ceiling := rs.throughputPlateaued(22); plateaued {
		t.Fatalf("22 viewers each served their full bitrate is a server that is SCALING, not one that plateaued "+
			"(reported ceiling %d MiB/s). This is the bug that pinned every machine to ~21 viewers.", ceiling>>20)
	}
}

// The fix must not blunt the detector: a server that takes on 50% more viewers
// and moves barely any extra data HAS stopped scaling, and saying so is the
// entire job. Only half the expected gain is required - this delivers a tenth of
// it.
func TestResourceState_ManyMoreViewersWithoutTheBytesIsStillAPlateau(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()

	rs.startTier(20, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(20, ResourceSample{ServedBytesPerSec: 500 << 20, DiskReadBytesPerSec: 500 << 20}, at)
	}
	// 30 viewers should move ~750 MiB/s. They move 525: the newcomers are eating
	// the same pie, not being served.
	rs.startTier(30, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(30, ResourceSample{ServedBytesPerSec: 525 << 20, DiskReadBytesPerSec: 525 << 20}, at)
	}

	plateaued, ceiling := rs.throughputPlateaued(30)
	if !plateaued {
		t.Fatal("half again as many viewers for 5% more bytes is a saturated source; the run must still catch it")
	}
	if ceiling != 500<<20 {
		t.Fatalf("ceiling = %d MiB/s, want 500", ceiling>>20)
	}
}

// THE 2026-07-14 REGRESSION, part one. Between two judged tiers the run REBUILDS
// its audience: every viewer restarts at once, and for the first seconds past the
// fixed warm-up the audience is still climbing back to its regime. The dev run
// measured tier 22 at 50 Mb/s against tier 21's established 512 Mb/s - not
// because the server collapsed, but because the window opened while the viewers
// were still coming back. Every tier above 21 was condemned the same way, so the
// "plateau" was the measurement, not the machine.
//
// The window must not open on a fixed clock: it opens when the audience has
// actually RESUMED - its per-viewer rate back in the neighbourhood of the last
// measured regime - or after a hard cap, whichever comes first.
func TestResourceState_ARestartingAudienceIsNotMeasured(t *testing.T) {
	start := time.Now()
	rs := newResourceState(10*time.Second, start)
	const perViewer = int64(3 << 20)

	// Tier 21, measured in steady state: 21 viewers, each at its bitrate.
	rs.startTier(21, start, true, "test")
	at := start.Add(11 * time.Second)
	for range 5 {
		rs.observe(21, ResourceSample{CPUPercent: 25, ServedBytesPerSec: 21 * perViewer, DiskReadBytesPerSec: 21 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}

	// The audience is rebuilt at 22: everyone restarts. Past the fixed warm-up
	// the viewers are still recovering, pulling a tenth of their regime.
	t2 := at
	rs.startTier(22, t2, true, "test")
	at = t2.Add(11 * time.Second)
	for range 3 {
		rs.observe(22, ResourceSample{CPUPercent: 2, ServedBytesPerSec: 21 * perViewer / 10, DiskReadBytesPerSec: 21 * perViewer / 10}, at)
		at = at.Add(2 * time.Second)
	}
	if got := rs.stats(22).Samples; got != 0 {
		t.Fatalf("recorded %d samples while the audience was still restarting: a tier measured mid-recovery condemns a server that is fine", got)
	}

	// The audience reaches its regime: from here the samples are the tier's own.
	for range 5 {
		rs.observe(22, ResourceSample{CPUPercent: 26, ServedBytesPerSec: 22 * perViewer, DiskReadBytesPerSec: 22 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}
	if got := rs.stats(22).Samples; got == 0 {
		t.Fatalf("no samples recorded once the audience was back at its regime")
	}
	if avg := rs.stats(22).ServedBytesPerSecAvg; avg < 22*perViewer*9/10 {
		t.Fatalf("tier 22 avg served = %d, want ~%d: the restart dip leaked into the tier's record", avg, 22*perViewer)
	}
	if plateaued, _ := rs.throughputPlateaued(22); plateaued {
		t.Fatalf("22 viewers each served their full bitrate is a server that is scaling; its restart transient must not be its measurement")
	}
}

// The regime gate must be TWO-sided. After a rebuild every viewer burst-fills
// its buffer at once: for the first seconds the audience pulls MORE per viewer
// than any regime does - the 2026-07-16 dev run quoted 468 Mb/s at tier 21
// against a 217 Mb/s regime at 20 - and a window that opens on "at least half
// the reference" opens right into that burst. Above the band is as unsettled
// as below it.
func TestResourceState_AJoinBurstDoesNotOpenTheWindow(t *testing.T) {
	start := time.Now()
	rs := newResourceState(10*time.Second, start)
	const perViewer = int64(3 << 20)

	rs.startTier(20, start, true, "test")
	at := start.Add(11 * time.Second)
	for range 5 {
		rs.observe(20, ResourceSample{ServedBytesPerSec: 20 * perViewer, DiskReadBytesPerSec: 20 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}

	// Rebuild at 21: everyone restarts and burst-fills, at three times the
	// regime, still inside the cap.
	t2 := at
	rs.startTier(21, t2, true, "test")
	at = t2.Add(11 * time.Second)
	for range 3 {
		rs.observe(21, ResourceSample{ServedBytesPerSec: 3 * 21 * perViewer, DiskReadBytesPerSec: 3 * 21 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}
	if got := rs.stats(21).Samples; got != 0 {
		t.Fatalf("recorded %d burst samples: a tier's throughput must not be quoted from its own join burst", got)
	}

	// The burst subsides into the regime: from here the samples are the tier's.
	for range 5 {
		rs.observe(21, ResourceSample{ServedBytesPerSec: 21 * perViewer, DiskReadBytesPerSec: 21 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}
	if got := rs.stats(21).Samples; got == 0 {
		t.Fatalf("no samples recorded once the burst subsided into the regime")
	}
	if avg := rs.stats(21).ServedBytesPerSecAvg; avg > 21*perViewer*11/10 {
		t.Fatalf("tier 21 avg served = %d, want ~%d: the join burst leaked into the tier's record", avg, 21*perViewer)
	}
}

// If the audience NEVER regains its regime, that failure IS the tier's evidence:
// past a hard cap the samples count, dip or not, and the degraded throughput is
// judged for what it is. The gate delays the window, it never blinds the run.
func TestResourceState_ARegimeThatNeverResumesIsMeasuredAnyway(t *testing.T) {
	start := time.Now()
	rs := newResourceState(10*time.Second, start)
	const perViewer = int64(3 << 20)

	rs.startTier(21, start, true, "test")
	at := start.Add(11 * time.Second)
	for range 5 {
		rs.observe(21, ResourceSample{ServedBytesPerSec: 21 * perViewer, DiskReadBytesPerSec: 21 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}

	// Tier 32 genuinely cannot feed its viewers: throughput stays at a tenth of
	// the regime, forever.
	t2 := at
	rs.startTier(32, t2, true, "test")
	at = t2.Add(11 * time.Second)
	for range 20 { // well past any reasonable cap
		rs.observe(32, ResourceSample{ServedBytesPerSec: 21 * perViewer / 10, DiskReadBytesPerSec: 21 * perViewer / 10}, at)
		at = at.Add(2 * time.Second)
	}
	if rs.stats(32).Samples == 0 {
		t.Fatalf("a tier whose audience never regains its regime must still be measured past the cap - that failure IS the evidence")
	}
	if plateaued, _ := rs.throughputPlateaued(32); !plateaued {
		t.Fatalf("32 viewers moving a tenth of 21's bytes, disk visibly delivering: the plateau must still be caught")
	}
}

// THE 2026-07-14 REGRESSION, part two. The reference tier itself was rotten: 21
// was only ever observed on a FAST hold, seconds after a ten-viewer batch joined.
// A join burst pulls far more than any regime (buffer filling) and burns CPU no
// steady state ever will - the dev run's table showed 61% CPU at 21 viewers
// against 1-9% everywhere else, and its 512 Mb/s became the bar every honest
// tier was condemned against.
//
// A tier that was never measured under a full window must neither anchor a
// throughput verdict nor appear in the table an operator sizes a machine with.
func TestResourceState_AFastTierNeverAnchorsTheVerdict(t *testing.T) {
	start := time.Now()
	rs := newResourceState(0, start)
	const perViewer = int64(3 << 20)

	// Tier 21 on a FAST hold, mid join-burst: three times the regime rate.
	rs.startTier(21, start, false, "test")
	at := start.Add(time.Second)
	for range 2 {
		rs.observe(21, ResourceSample{CPUPercent: 61, ServedBytesPerSec: 3 * 21 * perViewer, DiskReadBytesPerSec: 3 * 21 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}

	// Tier 31, measured for real: every viewer at its full bitrate.
	rs.startTier(31, at, true, "test")
	for range 5 {
		rs.observe(31, ResourceSample{CPUPercent: 9, ServedBytesPerSec: 31 * perViewer, DiskReadBytesPerSec: 31 * perViewer}, at)
		at = at.Add(2 * time.Second)
	}

	// 31 honest viewers move fewer bytes than 21's join-burst did. That is not a
	// plateau: the reference was never a regime.
	if plateaued, _ := rs.throughputPlateaued(31); plateaued {
		t.Fatalf("a FAST join-burst became the throughput reference: 31 viewers at full bitrate condemned against 21's burst")
	}
	// And the burst must not sit in the operator's table either: 61% CPU at 21
	// next to 9% at 31 is the incoherence that made the whole report unusable.
	costs := rs.tierCosts(1)
	if len(costs) != 1 || costs[0].Viewers != 31 {
		rows := make([]int, 0, len(costs))
		for _, c := range costs {
			rows = append(rows, c.Viewers)
		}
		t.Fatalf("tier table rows = %v, want exactly [31]: only tiers measured under a full window belong in it", rows)
	}
}

// THE 2026-07-14 REGRESSION, part three. The report's verdict was "the server
// cannot read the media any faster" while its own disk column read 0.0: every
// byte had come from the page cache (or the kernel simply cannot see the reads -
// 9p, network shares). A verdict that names the storage must be backed by the
// storage having actually done something.
func TestResourceState_AStarvedDiskCannotBeTheStorageCeiling(t *testing.T) {
	build := func(diskPerSec int64) *resourceState {
		rs := newResourceState(0, time.Now())
		at := time.Now()
		rs.startTier(10, at, true, "test")
		for range 5 {
			at = at.Add(time.Second)
			rs.observe(10, ResourceSample{ServedBytesPerSec: 100 << 20, DiskReadBytesPerSec: diskPerSec, DiskReadMeasured: true}, at)
		}
		rs.startTier(20, at, true, "test")
		for range 5 {
			at = at.Add(time.Second)
			rs.observe(20, ResourceSample{ServedBytesPerSec: 100 << 20, DiskReadBytesPerSec: diskPerSec, DiskReadMeasured: true}, at)
		}
		return rs
	}

	// The same flat curve with the disk actually delivering the bytes is a real
	// storage ceiling, and the run must keep saying so.
	if plateaued, _ := build(100 << 20).throughputPlateaued(20); !plateaued {
		t.Fatalf("a flat curve with the disk at full tilt IS the storage ceiling; the guard must not blunt the detector")
	}
	// With the disk at zero: same curve, no storage verdict. Whatever limits
	// this box, it is not something the storage was seen doing.
	if plateaued, _ := build(0).throughputPlateaued(20); plateaued {
		t.Fatalf("the disk read nothing and the verdict still blamed the storage - the exact lie of the 2026-07-14 report")
	}
}

// Storage that SERIALISES is invisible to every gauge: the CPU idles, memory sits
// still, and the disk looks fine at low concurrency. It only shows up as viewers
// pile on - throughput falls away, a single read starts taking seconds - and it
// is what actually decides how many people can watch. The probe must catch it.
func TestDiskProbe_DetectsStorageThatCollapsesUnderConcurrency(t *testing.T) {
	// A 9p mount multiplexed over one channel, measured for real: it peaks at 4
	// readers and falls off a cliff by 16.
	d := &DiskThroughput{Levels: []DiskLevel{
		{Readers: 1, BytesPerSec: 253 << 20, WorstReadMs: 4},
		{Readers: 4, BytesPerSec: 388 << 20, WorstReadMs: 21},
		{Readers: 16, BytesPerSec: 60 << 20, WorstReadMs: 2287},
	}}
	summarise(d)

	if d.BestReaders != 4 {
		t.Fatalf("best width = %d readers, want 4", d.BestReaders)
	}
	if d.CollapsesAt != 16 {
		t.Fatalf("collapse not detected (got %d, want 16): throughput fell to a sixth and a single read took 2.3s", d.CollapsesAt)
	}
}

// A healthy disk PLATEAUS - the same throughput however many readers pile on -
// and that must never be reported as a collapse. Crying wolf here would send an
// operator hunting a storage problem that doesn't exist.
func TestDiskProbe_APlateauIsNotACollapse(t *testing.T) {
	d := &DiskThroughput{Levels: []DiskLevel{
		{Readers: 1, BytesPerSec: 400 << 20, WorstReadMs: 3},
		{Readers: 4, BytesPerSec: 900 << 20, WorstReadMs: 6},
		{Readers: 16, BytesPerSec: 880 << 20, WorstReadMs: 12},
	}}
	summarise(d)

	if d.CollapsesAt != 0 {
		t.Fatalf("a disk that holds its throughput as readers pile on has not collapsed (got %d)", d.CollapsesAt)
	}
	if d.BytesPerSec != 900<<20 {
		t.Fatalf("best = %d, want 900 MiB/s", d.BytesPerSec/(1<<20))
	}
}

// A direct-play server's ceiling IS its read bandwidth. If it reads four bytes
// to serve one, three quarters of that bandwidth goes on data it throws away -
// and the audience it holds is a quarter of what the hardware could give. No
// gauge anywhere reports this, and it is invisible in the raw figures: the disk
// looks busy, which looks like the disk being the limit.
func TestReport_ReadAmplificationExposesWastedBandwidth(t *testing.T) {
	// Measured on a real box: 221 MiB/s read off the disks, 57 MiB/s handed to
	// viewers.
	got := readAmplification(ResourceStats{
		Samples:                20,
		DiskReadBytesPerSecAvg: 221 << 20, DiskReadMeasured: true,
		ServedBytesPerSecAvg: 57 << 20,
	})
	if got < 3.8 || got > 4.0 {
		t.Fatalf("read amplification = %.2f, want ~3.9 (221 MiB/s read for 57 MiB/s served)", got)
	}

	// A server that reads exactly what it serves is doing the best it can.
	if perfect := readAmplification(ResourceStats{
		Samples:                20,
		DiskReadBytesPerSecAvg: 100 << 20, DiskReadMeasured: true,
		ServedBytesPerSecAvg: 100 << 20,
	}); perfect < 0.99 || perfect > 1.01 {
		t.Fatalf("a server reading exactly what it serves has an amplification of 1, got %.2f", perfect)
	}

	// A counter the kernel could not account (network share, 9p) must NOT produce
	// a ratio: a number derived from a blind counter is worse than no number.
	if blind := readAmplification(ResourceStats{
		Samples:                20,
		DiskReadBytesPerSecAvg: 0, DiskReadMeasured: true,
		ServedBytesPerSecAvg: 100 << 20,
	}); blind != 0 {
		t.Fatalf("a blind I/O counter must not yield an amplification ratio, got %.2f", blind)
	}
}

// Reporting only what the DISK delivered says "the server reads 2.5x what it
// serves" without saying whether the CODE is greedy or the KERNEL is guessing.
// Those have entirely different fixes, and only both figures together tell them
// apart.
func TestReport_SeparatesAGreedyPackagerFromAGuessingKernel(t *testing.T) {
	// The code asks for 1.3x and the disk delivers 2.6x: the packager is nearly
	// optimal and the kernel is reading ahead into data nobody wanted.
	stats := ResourceStats{
		Samples:                20,
		ServedBytesPerSecAvg:   100 << 20,
		AppReadBytesPerSecAvg:  130 << 20,
		DiskReadBytesPerSecAvg: 260 << 20, DiskReadMeasured: true,
	}
	app, disk := appReadAmplification(stats), readAmplification(stats)
	if app < 1.25 || app > 1.35 {
		t.Fatalf("app amplification = %.2f, want ~1.3 (what the packager asked for)", app)
	}
	if disk < 2.55 || disk > 2.65 {
		t.Fatalf("disk amplification = %.2f, want ~2.6 (what the hardware paid)", disk)
	}
	if disk <= app {
		t.Fatalf("the disk cannot deliver less than the code asked for")
	}

	// A blind block counter (a 9p or network mount) must not produce a disk
	// figure - but the app figure still works, which is exactly why it is worth
	// having.
	blind := ResourceStats{Samples: 20, ServedBytesPerSecAvg: 100 << 20, AppReadBytesPerSecAvg: 130 << 20}
	if readAmplification(blind) != 0 {
		t.Fatalf("a blind block counter must not yield a disk amplification")
	}
	if a := appReadAmplification(blind); a < 1.25 || a > 1.35 {
		t.Fatalf("the app figure must still work where the block counter is blind, got %.2f", a)
	}
}

// A load test reads films by the gigabyte, and the kernel caches every one of
// them. memory.current counts that cache, so a naive reading reports 4.3 GB
// "used" by 21 viewers on a packager measured at under a megabyte each - and an
// operator plans his hardware around films sitting in reclaimable cache.
//
// The cache is not consumption. It must be reported apart, and it must never
// count toward saturation.
func TestResources_PageCacheIsNotConsumption(t *testing.T) {
	machine := &modelledMachine{
		cores: 4, cpuPctPerViewer: 0.1,
		memPerViewer: 1 << 20, // 1 MiB per viewer: what the packager really holds
		cacheBytes:   4 << 30, // 4 GiB of films sitting in cache
		memLimit:     8 << 30, // on an 8 GiB box
		viewers:      func() int { return 21 },
	}

	sampler := newResourceSampler(machine)
	at := time.Now()
	_, _ = sampler.sample(0, at) // baseline: no previous reading to diff against

	s, ok := sampler.sample(1<<20, at.Add(time.Second))
	if !ok {
		t.Fatal("no sample")
	}

	// 256 MiB idle + 21 MiB of viewers. NOT 4 GiB.
	if s.MemUsedBytes > 512<<20 {
		t.Errorf("MemUsedBytes = %d MiB: the page cache is being charged as consumption", s.MemUsedBytes>>20)
	}
	if s.MemCacheBytes != 4<<30 {
		t.Errorf("MemCacheBytes = %d MiB, want 4096 (reported, but apart)", s.MemCacheBytes>>20)
	}
	// The cache alone is half the box. If it counted toward the limit, this run
	// would report memory pressure that does not exist.
	if float64(s.MemUsedBytes)/float64(s.MemLimitBytes)*100 >= memSaturationPercent {
		t.Error("a server holding 277 MiB on an 8 GiB box is not memory-saturated; the cache must not count")
	}
}

// The load generator lives in the server's cgroup: its goroutines open the
// connections, drain the loopback traffic and throw it away, and the kernel bills
// all of it to the same CPU counter as the packager. Billing that to the server
// makes a viewer look several times more expensive than it is.
//
// The split is what the pprof label buys: samples carrying it are the harness's,
// everything else is the server's.
func TestCPUSplit_ChargesTheHarnessToTheHarness(t *testing.T) {
	t.Parallel()

	prof := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "cpu", Unit: "nanoseconds"},
		},
		Sample: []*pprofprofile.Sample{
			// The server packaging films: 750ms of CPU, unlabelled.
			{Value: []int64{75, 750_000_000}},
			// The harness pretending to watch them: 250ms, labelled.
			{
				Value: []int64{25, 250_000_000},
				Label: map[string][]string{harnessLabelKey: {harnessLabelVal}},
			},
		},
	}

	split := splitByLabel(prof)
	if !split.Measured {
		t.Fatal("a profile with samples must be measured")
	}
	if got := split.HarnessFraction; got < 0.24 || got > 0.26 {
		t.Errorf("HarnessFraction = %.3f, want ~0.25", got)
	}
	if got := split.serverShare(); got < 0.74 || got > 0.76 {
		t.Errorf("serverShare = %.3f, want ~0.75", got)
	}
}

// It must weigh samples by CPU TIME, not by count. A profile where the harness
// takes many cheap samples and the server takes few expensive ones would
// otherwise credit the harness with most of the CPU - and hand the operator a
// server that looks free.
func TestCPUSplit_WeighsByCPUTimeNotSampleCount(t *testing.T) {
	t.Parallel()

	prof := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "cpu", Unit: "nanoseconds"},
		},
		Sample: []*pprofprofile.Sample{
			// One server sample worth 900ms.
			{Value: []int64{1, 900_000_000}},
			// Nine harness samples worth 100ms in total.
			{
				Value: []int64{9, 100_000_000},
				Label: map[string][]string{harnessLabelKey: {harnessLabelVal}},
			},
		},
	}

	split := splitByLabel(prof)
	if got := split.HarnessFraction; got > 0.11 {
		t.Errorf("HarnessFraction = %.3f, want ~0.10 - counting SAMPLES instead of nanoseconds would give 0.90", got)
	}
}

// An unmeasured split must not silently zero the server's CPU. When no profile
// could be taken, everything is charged to the server: the pessimistic reading,
// which is the only safe one for someone sizing hardware.
func TestCPUSplit_UnmeasuredChargesEverythingToTheServer(t *testing.T) {
	t.Parallel()

	var none cpuSplit
	if none.Measured {
		t.Fatal("a zero split is not a measurement")
	}
	if got := none.serverShare(); got != 1 {
		t.Errorf("serverShare = %v, want 1 (charge it all to the server when unknown)", got)
	}
	if got := splitByLabel(&pprofprofile.Profile{}); got.Measured {
		t.Error("an empty profile is not a measurement")
	}
}

// A join burst can fill the memory inside a tier's warm-up window, where every
// normal verdict is still discarded. The critical threshold must fire there,
// on one sample, or the OOM killer decides before the run does.
func TestResourceState_CriticalMemoryStopsInsideWarmup(t *testing.T) {
	start := time.Now()
	rs := newResourceState(10*time.Second, start)
	rs.observe(0, ResourceSample{}, start)
	rs.startTier(100, start, false, "test")

	hot := ResourceSample{MemUsedBytes: 96 << 20, MemLimitBytes: 100 << 20}
	if reason, _ := rs.observe(100, hot, start.Add(time.Second)); reason != StopMemorySaturated {
		t.Fatalf("96%% memory inside warm-up: reason = %q, want %q", reason, StopMemorySaturated)
	}

	busy := ResourceSample{MemUsedBytes: 92 << 20, MemLimitBytes: 100 << 20}
	rs2 := newResourceState(10*time.Second, start)
	rs2.observe(0, ResourceSample{}, start)
	rs2.startTier(100, start, false, "test")
	if reason, _ := rs2.observe(100, busy, start.Add(time.Second)); reason != "" {
		t.Fatalf("92%% memory inside warm-up must not stop the run on its own, got %q", reason)
	}
}

// A tier's Go-runtime figures are averages over its samples, with the peak and
// the limit kept: that is what lets a report say "the collector took 40% of the
// CPU while the heap sat at the limit".
func TestResourceStats_AggregatesTheGoRuntime(t *testing.T) {
	rs := newResourceState(0, time.Now())
	rs.byTier[5] = []ResourceSample{
		{GoHeapLiveBytes: 4 << 30, GoGCCPUPercent: 10, GoMemLimitBytes: 8 << 30},
		{GoHeapLiveBytes: 8 << 30, GoGCCPUPercent: 50, GoMemLimitBytes: 8 << 30},
	}
	got := rs.stats(5)
	if got.GoHeapLiveBytesAvg != 6<<30 || got.GoHeapLiveBytesMax != 8<<30 {
		t.Fatalf("heap avg/max = %d/%d, want 6 GiB / 8 GiB", got.GoHeapLiveBytesAvg, got.GoHeapLiveBytesMax)
	}
	if got.GoGCCPUPercentAvg != 30 || got.GoGCCPUPercentPeak != 50 || got.GoMemLimitBytes != 8<<30 {
		t.Fatalf("gc avg/peak = %.0f/%.0f limit %d, want 30 / 50 / 8 GiB", got.GoGCCPUPercentAvg, got.GoGCCPUPercentPeak, got.GoMemLimitBytes)
	}
}

// Two sizes a few percent apart cannot tell a plateau from noise: 67 viewers
// against 65 expect 1.5 % more bytes, less than the spread between two
// windows of one size (2026-10-16). The verdict needs a reference at least
// plateauMinHeadcountRatio smaller.
func TestResourceState_NoPlateauVerdictBetweenNeighbouringSizes(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()
	for _, tier := range []int{65, 67} {
		rs.startTier(tier, at, true, "test")
		for range 5 {
			at = at.Add(time.Second)
			rs.observe(tier, ResourceSample{ServedBytesPerSec: 40 << 20, DiskReadBytesPerSec: 40 << 20}, at)
		}
	}
	if plateaued, _ := rs.throughputPlateaued(67); plateaued {
		t.Fatalf("67 against 65 viewers: a flat curve between neighbours is noise, not the storage ceiling")
	}
	// Against a size far enough below, the same flat curve is the ceiling.
	rs.startTier(100, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(100, ResourceSample{ServedBytesPerSec: 40 << 20, DiskReadBytesPerSec: 40 << 20}, at)
	}
	if plateaued, _ := rs.throughputPlateaued(100); !plateaued {
		t.Fatalf("100 viewers pushing what 65 did: that is the ceiling")
	}
}

// THE 2026-10-19 REGRESSION. Prod, through the proxy: 51 viewers held four
// windows at 222, 280, 263 and 279 Mb/s, one size moving 16 % between two of
// its own windows. The first window at 59 moved 267 Mb/s and was condemned
// "throughput flat at 263 Mb/s: the server cannot read the media any faster"
// with every disk 1-12 % busy; the next four windows at 59 moved 307-329. A
// single window against a size 16 % smaller expects 8 % more bytes, half the
// spread of one window: the verdict needs a reference far enough below that
// the gain it demands clears the spread.
func TestResourceState_OneNoisyWindowAboveANearbySizeIsNotAPlateau(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()
	const mbps = 1e6 / 8
	rs.startTier(51, at, true, "test")
	for i, served := range []float64{222, 280, 263, 279} {
		if i > 0 {
			rs.nextWindow(51, at, "test")
		}
		for range 5 {
			at = at.Add(time.Second)
			rs.observe(51, ResourceSample{ServedBytesPerSec: int64(served * mbps), DiskReadBytesPerSec: 35 << 20, DiskReadMeasured: true}, at)
		}
	}
	rs.startTier(59, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(59, ResourceSample{ServedBytesPerSec: int64(267 * mbps), DiskReadBytesPerSec: 35 << 20, DiskReadMeasured: true}, at)
	}
	if plateaued, ceiling := rs.throughputPlateaued(59); plateaued {
		t.Fatalf("59 viewers condemned against 51's %d Mb/s on one window inside 51's own spread", ceiling*8/1e6)
	}
}

// A size judged on several windows is judged on each window's own
// throughput: a starved first window must not condemn the next one.
func TestResourceState_PlateauIsJudgedOnTheWindowInProgress(t *testing.T) {
	rs := newResourceState(0, time.Now())
	at := time.Now()
	rs.startTier(50, at, true, "test")
	for range 5 {
		at = at.Add(time.Second)
		rs.observe(50, ResourceSample{ServedBytesPerSec: 50 << 20, DiskReadBytesPerSec: 50 << 20}, at)
	}
	rs.startTier(80, at, true, "test")
	for range 5 { // first window: flat
		at = at.Add(time.Second)
		rs.observe(80, ResourceSample{ServedBytesPerSec: 50 << 20, DiskReadBytesPerSec: 50 << 20}, at)
	}
	if plateaued, _ := rs.throughputPlateaued(80); !plateaued {
		t.Fatalf("first window flat: plateau")
	}
	rs.nextWindow(80, at, "test")
	for range 5 { // second window: scaling
		at = at.Add(time.Second)
		rs.observe(80, ResourceSample{ServedBytesPerSec: 80 << 20, DiskReadBytesPerSec: 80 << 20}, at)
	}
	if plateaued, _ := rs.throughputPlateaued(80); plateaued {
		t.Fatalf("the second window pushed 80 MiB/s: it must not inherit the first window's verdict")
	}
}

// A mixed run: half the bytes went to ffmpeg viewers, whose source reads the
// packager's counter never sees. Its reads over EVERY viewer's bytes read 0.65,
// "the code reads less than it serves", and against a 2.6 disk figure sent an
// operator after the kernel's readahead for what was ffmpeg's I/O. Its reads
// over what it served itself say 1.3.
func TestAppReadAmplification_IsThePackagersOwnRatioInAMixedRun(t *testing.T) {
	mixed := ResourceStats{
		Samples:                 20,
		ServedBytesPerSecAvg:    200 << 20,
		AppServedBytesPerSecAvg: 100 << 20,
		AppReadBytesPerSecAvg:   130 << 20,
	}
	if a := appReadAmplification(mixed); a < 1.25 || a > 1.35 {
		t.Fatalf("app amplification = %.2f, want ~1.3 (its reads over what IT served)", a)
	}
}

// The sampler turns the packager's served counter into a rate beside its reads.
func TestResourceSampler_RatesThePackagersServedBytes(t *testing.T) {
	src := &appServedFakeSource{}
	s := newResourceSampler(src)
	t0 := time.Unix(1000, 0)
	src.read, src.served = 1, 1
	s.sample(1, t0)
	src.read, src.served = 1+(13<<20), 1+(10<<20)
	out, ok := s.sample(1+(20<<20), t0.Add(time.Second))
	if !ok {
		t.Fatal("second sample must report rates")
	}
	if out.AppServedBytesPerSec != 10<<20 {
		t.Fatalf("AppServedBytesPerSec = %d, want %d", out.AppServedBytesPerSec, 10<<20)
	}
	if out.ServedBytesPerSec != 20<<20 {
		t.Fatalf("ServedBytesPerSec = %d, want every viewer's %d", out.ServedBytesPerSec, 20<<20)
	}
}

type appServedFakeSource struct{ read, served, copied int64 }

func (f *appServedFakeSource) cpuUsageUsec() int64                { return 0 }
func (f *appServedFakeSource) memory() (used, cache, limit int64) { return 0, 0, 0 }
func (f *appServedFakeSource) diskReadBytes() (int64, bool)       { return 0, false }
func (f *appServedFakeSource) appReadBytes() int64                { return f.read }
func (f *appServedFakeSource) cpuAllowance() float64              { return 1 }
func (f *appServedFakeSource) appServedBytes() int64              { return f.served }
func (f *appServedFakeSource) appCopyBytes() int64                { return f.copied }

// The packager's segment copies (mkvgo 0.58 CopyReadBytes) are rated apart
// from its window walks: a local file's copies come from the page cache, and
// counted inside the reads they made the app figure 3.35 where the walks were
// 1.37.
func TestResourceSampler_RatesTheSegmentCopiesApart(t *testing.T) {
	src := &appServedFakeSource{}
	s := newResourceSampler(src)
	t0 := time.Unix(1000, 0)
	src.read, src.served, src.copied = 1, 1, 1
	s.sample(1, t0)
	src.read, src.served, src.copied = 1+(13<<20), 1+(10<<20), 1+(20<<20)
	out, ok := s.sample(1+(10<<20), t0.Add(time.Second))
	if !ok {
		t.Fatal("second sample must report rates")
	}
	if out.AppReadBytesPerSec != 13<<20 || out.AppCopyBytesPerSec != 20<<20 {
		t.Fatalf("walks %d, copies %d: want %d and %d, apart", out.AppReadBytesPerSec, out.AppCopyBytesPerSec, 13<<20, 20<<20)
	}
	stats := statsOf([]ResourceSample{out})
	if a := appReadAmplification(stats); a < 1.25 || a > 1.35 {
		t.Fatalf("app amplification = %.2f, want 1.3: the walks alone", a)
	}
	if c := appCopyReadAmplification(stats); c < 1.95 || c > 2.05 {
		t.Fatalf("copy amplification = %.2f, want 2.0", c)
	}
}
