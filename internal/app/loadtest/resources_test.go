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

func (m *modelledMachine) diskReadBytes() int64 {
	_, disk := m.integrate()
	return disk
}

func (m *modelledMachine) memory() (used, limit int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base := int64(256 << 20) // an idle server's own footprint
	return base + m.memPerViewer*int64(m.viewers()), m.memLimit
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
	machine := &modelledMachine{
		cores: 1, cpuPctPerViewer: 25,
		memPerViewer: 64 << 20, memLimit: 8 << 30,
		diskPerViewer: 5 << 20,
		viewers:       st.viewersAdmitted,
	}

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, RampEvery: 600 * time.Millisecond, MaxDuration: 30 * time.Second,
		ResourceWarmup: 50 * time.Millisecond, ResourceSampleEvery: 50 * time.Millisecond,
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
	machine := &modelledMachine{
		cores: 16, cpuPctPerViewer: 1,
		memPerViewer: 256 << 20, memLimit: 1 << 30,
		viewers: st.viewersAdmitted,
	}

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, RampEvery: 600 * time.Millisecond, MaxDuration: 30 * time.Second,
		ResourceWarmup: 50 * time.Millisecond, ResourceSampleEvery: 50 * time.Millisecond,
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

	machine := &modelledMachine{
		cores: 8, cpuPctPerViewer: 2,
		memPerViewer: 32 << 20, memLimit: 16 << 30,
		diskPerViewer: 2 << 20,
		viewers:       st.viewersAdmitted,
	}

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(4)}, zeroActive, testLogger())
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 3, RampEvery: 400 * time.Millisecond, MaxDuration: 30 * time.Second,
		ResourceWarmup: 50 * time.Millisecond, ResourceSampleEvery: 50 * time.Millisecond,
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
		"memory.max":     "4294967296\n",
		"io.stat":        "8:0 rbytes=1000 wbytes=5 rios=1\n8:16 rbytes=2000 wbytes=5 rios=1\n",
	})
	src := cgroupSource{fsRoot: root}

	if got := src.cpuAllowance(); got != 2 {
		t.Fatalf("cpuAllowance = %v, want 2 (the cgroup quota, not the host's cores)", got)
	}
	if got := src.cpuUsageUsec(); got != 4_200_000 {
		t.Fatalf("cpuUsageUsec = %d, want 4200000", got)
	}
	used, limit := src.memory()
	if used != 1<<30 || limit != 4<<30 {
		t.Fatalf("memory = (%d, %d), want (1GiB, 4GiB)", used, limit)
	}
	// Disk reads sum across every block device the container touched.
	if got := src.diskReadBytes(); got != 3000 {
		t.Fatalf("diskReadBytes = %d, want 3000 (summed across devices)", got)
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

	_, limit := cgroupSource{fsRoot: root}.memory()
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
