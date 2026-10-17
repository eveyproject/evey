package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// bench_fixes_test.go - the 2026-10-15 cross-check found the report wrong on
// six counts (a disk counter blind behind mergerfs, an app-read counter the
// hardened server cannot read, a disk verdict on a busy percentage that
// missed every stall, one CPU figure for two machines, peaks flattened by a
// two-second cadence, a header and a table reading different tiers). Each
// test here pins one of the corrections to a measurement.

// A disk is saturated when its reads WAIT, read against the server at rest:
// the tiers that stalled on 2026-10-15 had their disks 56-82 % busy (under the
// old 80 % bar) and every read waiting 27-46 ms against 7-8 ms at the tier
// that held, with the machine 63-81 % in iowait.
func TestDiskVerdict_ReadsTheWaitAgainstTheBaselineNotABusyPercentage(t *testing.T) {
	baseline := ResourceStats{Disks: []DiskStat{{Device: "sda", AwaitMsAvg: 7}}}

	held := ResourceStats{MachineIOWaitPercentAvg: 21, Disks: []DiskStat{{Device: "sda", UtilPctAvg: 85, AwaitMsAvg: 8.4, QueueDepthAvg: 0.9}}}
	if v := diskVerdict(held, baseline); v.Saturated {
		t.Fatalf("a disk busy 85 %% of the time but answering in 8 ms is not saturated: %+v", v)
	}

	broke := ResourceStats{MachineIOWaitPercentAvg: 65, Disks: []DiskStat{
		{Device: "sdc", UtilPctAvg: 56, AwaitMsAvg: 19, QueueDepthAvg: 2},
		{Device: "sda", UtilPctAvg: 68, AwaitMsAvg: 27, AwaitMsPeak: 46, QueueDepthAvg: 3.5, ReadBytesPerSecAvg: 38 << 20},
	}}
	v := diskVerdict(broke, baseline)
	if !v.Saturated || v.Device != "sda" {
		t.Fatalf("27 ms per read against 7 at rest, 68 %% busy, 65 %% iowait: saturated sda, got %+v", v)
	}
	if v.AwaitBaselineMs != 7 || v.AwaitMsPeak != 46 || v.IOWaitPct != 65 || v.QueueDepth != 3.5 {
		t.Fatalf("the verdict carries the figures it was read on: %+v", v)
	}

	// A pool slow at rest needs proportionally more: 2.5x its 15 ms rest wait is
	// 37.5, so 27 ms is not a queue there.
	slowAtRest := ResourceStats{Disks: []DiskStat{{Device: "sda", AwaitMsAvg: 15}}}
	if v := diskVerdict(broke, slowAtRest); v.Saturated {
		t.Fatalf("27 ms on a disk that rests at 15 ms is not a formed queue: %+v", v)
	}
	// A disk waiting long but idle, on a machine not in iowait, is not what held
	// the audience back.
	idle := ResourceStats{MachineIOWaitPercentAvg: 2, Disks: []DiskStat{{Device: "sda", UtilPctAvg: 5, AwaitMsAvg: 40}}}
	if v := diskVerdict(idle, baseline); v.Saturated {
		t.Fatalf("a disk busy 5 %% of the time is not the limit however slow its few reads: %+v", v)
	}
	if v := diskVerdict(ResourceStats{}, baseline); v.Device != "" || v.Saturated {
		t.Fatalf("no device, no verdict: %+v", v)
	}
}

// The whole machine beside the container: /proc/stat's busy share, with
// iowait counted apart and never as busy.
func TestMachineTicks_SplitsBusyAndIOWait(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(line string) {
		if err := os.WriteFile(filepath.Join(root, "proc/stat"), []byte(line+"\ncpu0 1 2 3 4 5 6 7 8 0 0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	//            user nice system idle iowait irq softirq steal
	write("cpu 100 0 50 800 50 0 0 0 0 0")
	a := readMachineTicks(root)
	write("cpu 200 0 100 1000 350 0 0 50 0 0")
	b := readMachineTicks(root)
	if !a.ok || !b.ok {
		t.Fatal("the cpu line must parse")
	}
	// Delta: user 100, system 50, steal 50 = 200 busy; iowait 300; idle 200; total 700.
	busy, iowait := machineDeltas(a, b)
	if int(busy*10) != 285 || int(iowait*10) != 428 {
		t.Fatalf("busy %.1f %% iowait %.1f %%, want 28.6 and 42.9 (iowait is not busy)", busy, iowait)
	}
	if busy, iowait := machineDeltas(b, a); busy != 0 || iowait != 0 {
		t.Fatalf("counters going backwards read as nothing, got %v %v", busy, iowait)
	}
}

// The server process alone, from /proc/self/status, which stays readable
// when the process is non-dumpable (/proc/self/io does not).
func TestProcessRSS_ReadsVmRSS(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc/self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc/self/status"), []byte("Name:\tevey\nVmPeak:\t 999 kB\nVmRSS:\t  473088 kB\nRssAnon:\t 400000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readProcessRSS(root); got != 473088*1024 {
		t.Fatalf("VmRSS = %d, want 473088 kB", got)
	}
	if got := readProcessRSS(t.TempDir()); got != 0 {
		t.Fatalf("no status file reads as 0, got %d", got)
	}
}

// The queue depth comes from the weighted I/O time column, and the peaks are
// the busiest one-second interval, which a tier's average flattens.
func TestDiskDeltas_QueueDepthAndPeaks(t *testing.T) {
	prev := map[string]diskCounters{"sda": {reads: 100, sectorsRead: 2000, readMs: 800, ioTicksMs: 500, weightedMs: 1000}}
	cur := map[string]diskCounters{"sda": {reads: 200, sectorsRead: 4000, readMs: 3500, ioTicksMs: 1100, weightedMs: 4500}}
	s := diskDeltas(prev, cur, 1)
	if len(s) != 1 || s[0].Device != "sda" {
		t.Fatalf("samples = %+v", s)
	}
	if s[0].QueueDepth != 3.5 || s[0].AwaitMs != 27 || int(s[0].UtilPct) != 60 || s[0].ReadBytesPerSec != 2000*512 {
		t.Fatalf("sample = %+v, want queue 3.5, await 27, util 60, 1 MiB/s", s[0])
	}
	agg := aggregateDisks([]ResourceSample{
		{Disks: []DiskSample{{Device: "sda", UtilPct: 20, ReadBytesPerSec: 10 << 20, AwaitMs: 8, QueueDepth: 1}}},
		{Disks: []DiskSample{{Device: "sda", UtilPct: 90, ReadBytesPerSec: 60 << 20, AwaitMs: 46, QueueDepth: 6}}},
		{Disks: []DiskSample{{Device: "sda", UtilPct: 40, ReadBytesPerSec: 20 << 20, AwaitMs: 12, QueueDepth: 2}}},
	})
	if len(agg) != 1 {
		t.Fatalf("aggregate = %+v", agg)
	}
	d := agg[0]
	if d.ReadBytesPerSecPeak != 60<<20 || d.AwaitMsPeak != 46 || d.UtilPctPeak != 90 || d.QueueDepthAvg != 3 || d.ReadBytesPerSecAvg != 30<<20 || d.AwaitMsAvg != 22 {
		t.Fatalf("aggregate = %+v, want 1 s peaks 60 MiB/s, 46 ms, 90 %%; averages 30 MiB/s, 22 ms, queue 3", d)
	}
}

// A device id resolves to the WHOLE disk the kernel accounts in diskstats: a
// partition to its parent, a volume to itself, an anonymous device (FUSE, a
// network share) to nothing.
func TestWholeDiskOf_ResolvesPartitionsVolumesAndNothing(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mk("sys/block/sda")
	mk("sys/block/dm-0")
	mk("sys/dev/block")
	mk("sys/devices/pci0/host0/block/sda/sda1")
	mk("sys/devices/virtual/block/dm-0")
	link := func(name, target string) {
		if err := os.Symlink(target, filepath.Join(root, "sys/dev/block", name)); err != nil {
			t.Fatal(err)
		}
	}
	link("8:1", "../../devices/pci0/host0/block/sda/sda1")
	link("8:0", "../../devices/pci0/host0/block/sda")
	link("253:0", "../../devices/virtual/block/dm-0")
	dev := func(major, minor uint64) uint64 { return major<<8 | minor }
	if got := wholeDiskOf(root, dev(8, 1)); got != "sda" {
		t.Fatalf("sda1 -> %q, want its disk sda", got)
	}
	if got := wholeDiskOf(root, dev(8, 0)); got != "sda" {
		t.Fatalf("sda -> %q", got)
	}
	if got := wholeDiskOf(root, dev(253, 0)); got != "dm-0" {
		t.Fatalf("dm-0 -> %q (a volume is the device the kernel accounts)", got)
	}
	if got := wholeDiskOf(root, dev(0, 47)); got != "" {
		t.Fatalf("an anonymous device (0:47, a FUSE mount) -> %q, want nothing", got)
	}
	if got := mappedDevices([]DeviceMapping{{Location: "/a", Device: "sda"}, {Location: "/b", Device: "sda"}, {Location: "/c"}, {Location: "/d", Device: "dm-0"}}); len(got) != 2 || got[0] != "sda" || got[1] != "dm-0" {
		t.Fatalf("mapped devices = %v, want sda and dm-0 once each", got)
	}
}

// mountPointOf is the longest mount under the path, octal escapes decoded.
func TestMountPointOf_LongestPrefix(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc/self"), 0o755); err != nil {
		t.Fatal(err)
	}
	mounts := "/dev/sda1 / ext4 rw 0 0\n/dev/sdb1 /mnt ext4 rw 0 0\n/dev/sdc1 /mnt/media\\0402 ext4 rw 0 0\nmergerfs /mnt/media fuse.mergerfs rw 0 0\n"
	if err := os.WriteFile(filepath.Join(root, "proc/self/mounts"), []byte(mounts), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/mnt/media/films/a.mkv": "/mnt/media",
		"/mnt/media 2/x.mkv":     "/mnt/media 2",
		"/mnt/other/x.mkv":       "/mnt",
		"/home/x.mkv":            "/",
		"/mnt/mediafoo/x.mkv":    "/mnt",
	} {
		if got := mountPointOf(root, path); got != want {
			t.Errorf("mountPointOf(%q) = %q, want %q", path, got, want)
		}
	}
}

// The viewers go through the public proxy whenever one is configured and
// answers, and hit the server directly otherwise - nothing to choose; the
// report says which path was taken, and the seed it drew.
func TestRunner_GoesThroughTheProxyWhenThereIsOne(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()
	proxy := httptest.NewServer(st.handler())
	defer proxy.Close()

	run := func(public string) *Report {
		r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
		r.counters = &modelledMachine{cores: 8, cpuPctPerViewer: 1, memPerViewer: 1 << 20, memLimit: 16 << 30, diskPerViewer: 1 << 20, viewers: func() int { return 1 }}
		cfg := Config{
			BaseURL: srv.URL, PublicBaseURL: public, Token: "tkn", Scenario: ScenarioMkvgo,
			MaxViewers: 1, RampEvery: 200 * time.Millisecond, FastHold: 200 * time.Millisecond, MeasureHold: 300 * time.Millisecond, MaxDuration: 10 * time.Second,
			Seed: 1792073232940793883,
		}
		if err := r.Start(context.Background(), cfg); err != nil {
			t.Fatalf("Start: %v", err)
		}
		return waitForReport(t, r, 10*time.Second)
	}
	direct := run("")
	if direct.ViaProxy || direct.Endpoint != srv.URL {
		t.Fatalf("no proxy configured: endpoint %q via proxy %v, want the server's own %q", direct.Endpoint, direct.ViaProxy, srv.URL)
	}
	if direct.Seed != 1792073232940793883 {
		t.Fatalf("the report must carry the seed it drew with, got %d", direct.Seed)
	}
	viaProxy := run(proxy.URL)
	if !viaProxy.ViaProxy || viaProxy.Endpoint != proxy.URL {
		t.Fatalf("proxy configured: endpoint %q via proxy %v, want %q", viaProxy.Endpoint, viaProxy.ViaProxy, proxy.URL)
	}
	unreachable := run("http://127.0.0.1:1")
	if unreachable.ViaProxy || unreachable.Endpoint != srv.URL {
		t.Fatalf("proxy that does not answer: endpoint %q via proxy %v, want the server's own %q", unreachable.Endpoint, unreachable.ViaProxy, srv.URL)
	}
}

// Every hold is a row: the FAST hunt, the tier that broke, the retry that
// confirmed it, the tier that held - each with its own latency percentiles,
// and the machine's figures beside the container's. The baseline is read
// before the first viewer joins.
func TestRunner_ReportsEveryHoldWithItsVerdict(t *testing.T) {
	st := newServerState()
	st.segCount = 60
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	machine := &modelledMachine{cores: 4, cpuPctPerViewer: 30, memPerViewer: 1 << 20, memLimit: 16 << 30, diskPerViewer: 1 << 20, viewers: func() int { return r.Status().Viewers }}
	r.counters = machine
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 0, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 500 * time.Millisecond, MaxDuration: 30 * time.Second,
		ResourceSampleEvery: 50 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopCPUSaturated {
		t.Fatalf("stop = %s (%s), want the modelled CPU to saturate", report.StopReason, report.StopDetail)
	}
	if len(report.Windows) < 2 {
		t.Fatalf("windows = %d, want every hold of the climb", len(report.Windows))
	}
	var broke, held int
	for i, w := range report.Windows {
		if w.Tier <= 0 || w.Phase == "" {
			t.Fatalf("window %d = %+v: no tier or phase", i, w)
		}
		if w.EndedAt.Before(w.StartedAt) {
			t.Fatalf("window %d ends before it starts: %+v", i, w)
		}
		if w.Broke {
			broke++
			if w.Reason == "" {
				t.Fatalf("a broken window names what broke it: %+v", w)
			}
		} else {
			held++
		}
	}
	if broke == 0 || held == 0 {
		t.Fatalf("the climb must show both held and broken holds, got %d held, %d broke", held, broke)
	}
	last := report.Windows[len(report.Windows)-1]
	if last.Resources.Samples == 0 && last.Phase != phaseFast.String() {
		t.Fatalf("a measured hold carries the machine's samples: %+v", last)
	}
	if report.BreakViewers > 0 && report.BreakReason != StopCPUSaturated {
		t.Fatalf("the break tier's reason must be named: %q", report.BreakReason)
	}
	// The baseline was read from the idle server before the first viewer, and
	// its memory is the modelled server's own footprint.
	if report.ResourceBaseline.Samples == 0 {
		t.Fatalf("no baseline samples: the idle server must be measured before the first viewer joins")
	}
	if report.ResourceBaseline.MemUsedBytesAvg != 256<<20 {
		t.Fatalf("baseline memory = %d, want the idle server's 256 MiB (no viewer yet)", report.ResourceBaseline.MemUsedBytesAvg)
	}
	// The modelled machine reports its disk as measured and nothing else: the
	// app-read figure is then absent, not zero.
	if !report.DiskReadMeasured && len(report.DiskDevices) > 0 {
		t.Fatalf("a mapped device means the disk figure is measured")
	}
	for _, tc := range report.Tiers {
		if tc.CPUPercent < 0 {
			t.Fatalf("tier %+v", tc)
		}
	}
}

// A latency sample lands in the hold that was open when it arrived, raw and
// scaled; the percentiles come back per hold.
func TestSampleState_PerWindowPercentiles(t *testing.T) {
	s := newSampleState()
	s.recordSegment(0, viewerTypeMkvgo, 100, 50) // no hold open: dropped
	for _, ms := range []int{100, 200, 300, 400, 1000} {
		s.recordSegment(1, viewerTypeMkvgo, ms, ms/2)
	}
	s.recordTTFF(1, 700)
	s.recordSegment(2, viewerTypeMkvgo, 5000, 2500)
	p50, p95, p95s, n, ttff := s.windowPercentiles(1)
	if p50 != 300 || p95 != 1000 || p95s != 500 || n != 5 || ttff != 700 {
		t.Fatalf("window 1 = %d %d %d %d %d", p50, p95, p95s, n, ttff)
	}
	if _, p95, p95s, n, _ := s.windowPercentiles(2); p95 != 5000 || p95s != 2500 || n != 1 {
		t.Fatalf("window 2 = %d %d %d", p95, p95s, n)
	}
	if _, _, _, n, _ := s.windowPercentiles(0); n != 0 {
		t.Fatalf("window 0 holds nothing")
	}
}

// A tier measured twice is two rows, the first marked broken with its
// reason, the second held; the window's samples are its own.
func TestResourceState_ATierMeasuredTwiceIsTwoWindows(t *testing.T) {
	at := time.Now()
	rs := newResourceState(0, at)
	rs.startTier(10, at, true, "narrowing")
	rs.observe(10, ResourceSample{CPUPercent: 50, ServedBytesPerSec: 1 << 20}, at.Add(time.Second))
	rs.closeWindow(at.Add(2*time.Second), StopSLOBreach, "p95 7000ms")
	rs.startTier(10, at.Add(2*time.Second), true, "narrowing")
	rs.observe(10, ResourceSample{CPUPercent: 40, ServedBytesPerSec: 2 << 20}, at.Add(3*time.Second))
	rs.closeWindow(at.Add(4*time.Second), "", "")
	rs.closeWindow(at.Add(5*time.Second), StopCPUSaturated, "late") // a second close changes nothing

	rows := rs.windowReports(1, nil, ResourceStats{})
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want two holds of tier 10", len(rows))
	}
	if !rows[0].Broke || rows[0].Reason != StopSLOBreach || rows[0].Detail != "p95 7000ms" || rows[0].Resources.CPUPercentAvg != 50 {
		t.Fatalf("first hold = %+v", rows[0])
	}
	if rows[1].Broke || rows[1].Resources.CPUPercentAvg != 40 || rows[1].Resources.ServedBytesPerSecAvg != 2<<20 {
		t.Fatalf("second hold = %+v", rows[1])
	}
	if rows[1].EndedAt != at.Add(4*time.Second) {
		t.Fatalf("the second hold closed at its own verdict, not at the late one: %v", rows[1].EndedAt)
	}
	// The server's share scales the CPU of every row.
	if half := rs.windowReports(0.5, nil, ResourceStats{}); half[0].Resources.CPUPercentAvg != 25 {
		t.Fatalf("server share not applied: %v", half[0].Resources.CPUPercentAvg)
	}
}

// The branch shares are those of the audience watching NOW, not of every
// viewer the run ever spawned.
func TestRampState_LiveBranchSharesFollowTheAudience(t *testing.T) {
	tally := newBranchTally(func(path string) string {
		switch path {
		case "/mnt/media/a.mkv":
			return "/mnt/disk_1"
		case "/mnt/media/b.mkv":
			return "/mnt/disk_2"
		}
		return ""
	})
	ramp := rampState{cancelBySlot: map[int]context.CancelFunc{}, filePathBySlot: map[int]string{}}
	noop := func() {}
	ramp.cancelBySlot[1], ramp.filePathBySlot[1] = noop, "/mnt/media/a.mkv"
	ramp.cancelBySlot[2], ramp.filePathBySlot[2] = noop, "/mnt/media/a.mkv"
	ramp.cancelBySlot[3], ramp.filePathBySlot[3] = noop, "/mnt/media/b.mkv"
	got := ramp.liveBranchShares(tally)
	if len(got) != 2 || got[0].Branch != "/mnt/disk_1" || got[0].Viewers != 2 || got[1].Viewers != 1 {
		t.Fatalf("shares = %+v", got)
	}
	delete(ramp.cancelBySlot, 1)
	delete(ramp.cancelBySlot, 2)
	if got := ramp.liveBranchShares(tally); len(got) != 1 || got[0].Branch != "/mnt/disk_2" {
		t.Fatalf("after two viewers left: %+v", got)
	}
	if got := ramp.liveBranchShares(nil); got != nil {
		t.Fatalf("no pool, no shares: %+v", got)
	}
}
