package loadtest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiskDeltas_BusyDiskAndAwait(t *testing.T) {
	prev := map[string]diskCounters{"sdc": {reads: 100, sectorsRead: 1000, readMs: 5000, ioTicksMs: 10000}}
	cur := map[string]diskCounters{"sdc": {reads: 300, sectorsRead: 1000 + 2*1024*100, readMs: 5000 + 200*80, ioTicksMs: 10000 + 1800}}
	got := diskDeltas(prev, cur, 2)
	if len(got) != 1 {
		t.Fatalf("samples = %+v, want one", got)
	}
	d := got[0]
	if d.Device != "sdc" || d.UtilPct < 89 || d.UtilPct > 91 {
		t.Fatalf("util = %.1f%% on %s, want ~90%% on sdc", d.UtilPct, d.Device)
	}
	if want := int64(2 * 1024 * 100 * 512 / 2); d.ReadBytesPerSec != want {
		t.Fatalf("read rate = %d, want %d", d.ReadBytesPerSec, want)
	}
	if d.AwaitMs < 79 || d.AwaitMs > 81 {
		t.Fatalf("await = %.1f ms, want ~80", d.AwaitMs)
	}
}

func TestDiskDeltas_QuietAndNewDevicesAreLeftOut(t *testing.T) {
	prev := map[string]diskCounters{"sdb": {sectorsRead: 5, ioTicksMs: 7}}
	cur := map[string]diskCounters{
		"sdb": {sectorsRead: 5, ioTicksMs: 7},      // did nothing
		"sdd": {sectorsRead: 999, ioTicksMs: 9999}, // no previous reading
	}
	if got := diskDeltas(prev, cur, 2); len(got) != 0 {
		t.Fatalf("got %+v, want none: a quiet disk is the absence of a line", got)
	}
}

func TestAggregateDisks_AveragesOverAppearancesAndSortsBusiestFirst(t *testing.T) {
	samples := []ResourceSample{
		{Disks: []DiskSample{{Device: "sdb", UtilPct: 20, ReadBytesPerSec: 10 << 20, AwaitMs: 10}, {Device: "sdc", UtilPct: 90, ReadBytesPerSec: 100 << 20, AwaitMs: 100}}},
		{Disks: []DiskSample{{Device: "sdc", UtilPct: 80, ReadBytesPerSec: 80 << 20, AwaitMs: 60}}},
	}
	got := aggregateDisks(samples)
	if len(got) != 2 || got[0].Device != "sdc" || got[1].Device != "sdb" {
		t.Fatalf("order = %+v, want sdc then sdb", got)
	}
	if got[0].UtilPctAvg != 85 || got[0].UtilPctPeak != 90 || got[0].AwaitMsAvg != 80 {
		t.Fatalf("sdc = %+v, want util avg 85 / peak 90 / await 80", got[0])
	}
	if got[1].UtilPctAvg != 20 {
		t.Fatalf("sdb averaged over the one sample it appeared in, got %.1f", got[1].UtilPctAvg)
	}
}

func TestReadDiskCounters_KeepsWholeDisksOnly(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "proc"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "sys/block/sdc"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "sys/block/loop0"), 0o755))
	// major minor name reads merged sectors readMs writes wmerged wsectors wMs inflight ioTicks weighted
	stats := "8 32 sdc 100 0 2048 500 0 0 0 0 0 900 0\n" +
		"8 33 sdc1 100 0 2048 500 0 0 0 0 0 900 0\n" +
		"7 0 loop0 5 0 40 1 0 0 0 0 0 1 0\n"
	must(os.WriteFile(filepath.Join(root, "proc/diskstats"), []byte(stats), 0o644))

	got := readDiskCounters(root)
	if len(got) != 1 {
		t.Fatalf("devices = %v, want only sdc (partition and loop device excluded)", got)
	}
	if c := got["sdc"]; c.reads != 100 || c.sectorsRead != 2048 || c.readMs != 500 || c.ioTicksMs != 900 {
		t.Fatalf("sdc counters = %+v", c)
	}
}

func TestBranchTally_CountsPerBranchAndCachesLookups(t *testing.T) {
	lookups := 0
	where := map[string]string{"/m/a.mkv": "/mnt/disk_2", "/m/b.mkv": "/mnt/disk_2", "/m/c.mkv": "/mnt/disk_1", "/m/x.mkv": ""}
	bt := newBranchTally(func(p string) string { lookups++; return where[p] })
	for _, p := range []string{"/m/a.mkv", "/m/a.mkv", "/m/b.mkv", "/m/c.mkv", "/m/x.mkv", ""} {
		bt.record(p)
	}
	got := bt.shares()
	if len(got) != 2 || got[0].Branch != "/mnt/disk_2" || got[0].Viewers != 3 || got[1].Viewers != 1 {
		t.Fatalf("shares = %+v, want disk_2 x3 then disk_1 x1 (unknown branch not counted)", got)
	}
	if lookups != 4 {
		t.Fatalf("lookups = %d, want 4: one per distinct non-empty path", lookups)
	}
	var nilTally *branchTally
	nilTally.record("/m/a.mkv")
	if nilTally.shares() != nil {
		t.Fatal("a nil tally (library not on a pool) must be a silent no-op")
	}
}

// The Go runtime's own accounting is read from the live process: the heap is
// never negative, the GC share is a percentage, and a first reading (no previous
// one to diff) reports no GC share rather than a made-up one.
func TestReadGoRuntime_IsSaneAndGCPercentNeedsTwoReadings(t *testing.T) {
	// The runtime refreshes these at the end of a collection: force one, or a
	// fresh test process reports zeros that prove nothing about the reader.
	garbage := make([][]byte, 0, 64)
	for range 64 {
		garbage = append(garbage, make([]byte, 1<<20))
	}
	runtime.GC()
	a := readGoRuntime()
	runtime.KeepAlive(garbage)
	if !a.ok {
		t.Skip("this toolchain does not expose the runtime metrics the report needs")
	}
	if a.totalCPUSec <= 0 || a.heapLiveBytes <= 0 {
		t.Fatalf("after a forced collection heap=%d total=%.3fs, want both positive: the reader reads nothing", a.heapLiveBytes, a.totalCPUSec)
	}
	if a.heapLiveBytes < 0 || a.memLimitBytes < 0 {
		t.Fatalf("heap=%d limit=%d, want non-negative", a.heapLiveBytes, a.memLimitBytes)
	}
	if got := gcCPUPercent(goRuntime{}, a); got != 0 {
		t.Fatalf("gc%% against an empty previous reading = %.1f, want 0", got)
	}
	// 2 s of runtime CPU of which 0.5 s collecting is 25%.
	prev := goRuntime{ok: true, gcCPUSeconds: 1, totalCPUSec: 10}
	cur := goRuntime{ok: true, gcCPUSeconds: 1.5, totalCPUSec: 12}
	if got := gcCPUPercent(prev, cur); got < 24.9 || got > 25.1 {
		t.Fatalf("gc%% = %.2f, want 25", got)
	}
	// A counter that went backwards (a restart) must not report a negative share.
	if got := gcCPUPercent(cur, prev); got != 0 {
		t.Fatalf("gc%% on a backwards counter = %.2f, want 0", got)
	}
}
