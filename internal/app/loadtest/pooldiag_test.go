package loadtest

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// retainedForTest is the allocation the heap profile must find: a named
// function of this module holding 64 MiB that stays reachable.
//
//go:noinline
func retainedForTest() []byte {
	// The allocation happens INSIDE the standard library: the leaf frame is
	// bytes.Repeat, the module's own function is its caller - exactly the case
	// (bufio, bytes, io) the attribution exists for.
	return bytes.Repeat([]byte{1}, 64<<20)
}

var heldByTest []byte

// The final heap profile must name the application function that holds the
// memory, not the runtime frame that performed the allocation: "mallocgc"
// helps nobody, "retainedForTest" is the culprit. (Allocations with no frame of
// this module at all are charged to their leaf, so runtime sites may appear.)
func TestHeapInUseTop_NamesTheApplicationFunctionHoldingTheBytes(t *testing.T) {
	heldByTest = retainedForTest()
	defer func() { heldByTest = nil }()

	sites, total := heapInUseTop(10)
	if total < 64<<20 {
		t.Fatalf("total in use = %d, want at least the 64 MiB held", total)
	}
	var found *HeapSite
	for i := range sites {
		if strings.HasSuffix(sites[i].Func, "retainedForTest") {
			found = &sites[i]
		}
	}
	if found == nil {
		t.Fatalf("retainedForTest not in the top sites: %+v", sites)
	}
	for _, s := range sites {
		if s.Bytes >= found.Bytes && strings.HasPrefix(s.Func, "runtime.") && s.Func == "runtime.mallocgc" && s.Bytes > 60<<20 {
			t.Fatalf("the 64 MiB was charged to %q instead of the module's own function", s.Func)
		}
	}
	if !strings.HasPrefix(found.Via, "bytes.") {
		t.Fatalf("via = %q, want the library that allocated on the function's behalf (bytes.Repeat)", found.Via)
	}
	if !strings.HasPrefix(found.Chain, "bytes.") || strings.Contains(found.Chain, "retainedForTest") {
		t.Fatalf("chain = %q, want the library frames between the allocation and the function, the function excluded", found.Chain)
	}
	if found.Bytes < 64<<20 || !strings.Contains(found.File, "internal/app/loadtest/") {
		t.Fatalf("site = %+v, want >= 64 MiB charged to a file under internal/app/loadtest/", *found)
	}
	if sites[0].Bytes < found.Bytes {
		t.Fatalf("sites not sorted by bytes: %+v", sites)
	}
}

// parkedThroughMarker blocks until released; its name is what the goroutine
// census looks for.
//
//go:noinline
func parkedThroughMarker(ready chan<- struct{}, release <-chan struct{}) {
	ready <- struct{}{}
	<-release
}

// Requests still being served hold their response: the census must count the
// goroutines parked inside a named function, and only those.
func TestGoroutinesThrough_CountsTheGoroutinesInsideANamedFunction(t *testing.T) {
	before, _ := goroutinesThrough("parkedThroughMarker")
	ready, release := make(chan struct{}), make(chan struct{})
	for range 3 {
		go parkedThroughMarker(ready, release)
	}
	for range 3 {
		<-ready
	}
	got, total := goroutinesThrough("parkedThroughMarker")
	close(release)
	if got-before != 3 {
		t.Fatalf("goroutines through the marker = %d (was %d), want 3 more", got, before)
	}
	if total < got {
		t.Fatalf("total %d < matching %d", total, got)
	}
	if n, _ := goroutinesThrough("noSuchFunctionAnywhere"); n != 0 {
		t.Fatalf("an unknown marker matched %d goroutines", n)
	}
}

// parkedForWaitSites is parkedThroughMarker under a name of its own: the census
// test above leaves goroutines that may still be exiting when this one runs,
// and sharing their name let it count one of them (4 instead of 3, seen in a
// clean-checkout run).
//
//go:noinline
func parkedForWaitSites(ready chan<- struct{}, release <-chan struct{}) {
	ready <- struct{}{}
	<-release
}

// The goroutines still serving are grouped by where they wait, so a run that
// ends with requests in flight says whether they sit on the disk or on a lock.
func TestWaitSitesThrough_GroupsTheParkedGoroutinesByWhereTheyWait(t *testing.T) {
	ready, release := make(chan struct{}), make(chan struct{})
	for range 3 {
		go parkedForWaitSites(ready, release)
	}
	for range 3 {
		<-ready
	}
	sites := waitSitesThrough("parkedForWaitSites", 5)
	close(release)
	if len(sites) != 1 || sites[0].Count != 3 || !strings.HasPrefix(sites[0].At, "loadtest.parkedForWaitSites") {
		t.Fatalf("wait sites = %+v, want the 3 goroutines in one group at loadtest.parkedForWaitSites", sites)
	}
}
