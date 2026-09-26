package sysinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestCPUSignature_StableAcrossCalls(t *testing.T) {
	// CPU identity doesn't change between calls - same hash every time.
	// Empty on non-Linux is acceptable (caller handles "unknown identity").
	first := CPUSignature()
	second := CPUSignature()
	if first != second {
		t.Errorf("CPUSignature drift: %q vs %q", first, second)
	}
}

func TestCPUSignature_HexFormat(t *testing.T) {
	sig := CPUSignature()
	if sig == "" {
		t.Skip("/proc/cpuinfo unavailable (non-Linux runner)")
	}
	// 16 bytes = 32 hex chars
	if len(sig) != 32 {
		t.Errorf("expected 32 hex chars, got %d (%q)", len(sig), sig)
	}
	for _, r := range sig {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Errorf("non-hex char in signature: %q", sig)
			break
		}
	}
}

func TestCollect_ReturnsNonNegative(t *testing.T) {
	s := Collect()
	if s.CPUPercent < 0 || s.CPUPercent > 100 {
		t.Errorf("CPU%% out of range: %.2f", s.CPUPercent)
	}
	if s.MemoryPercent < 0 || s.MemoryPercent > 100 {
		t.Errorf("Memory%% out of range: %.2f", s.MemoryPercent)
	}
	if _, err := os.Stat("/proc/meminfo"); err == nil {
		if s.MemTotalBytes <= 0 {
			t.Errorf("expected positive MemTotalBytes on Linux, got %d", s.MemTotalBytes)
		}
		if s.MemUsedBytes < 0 {
			t.Errorf("expected non-negative MemUsedBytes, got %d", s.MemUsedBytes)
		}
	}
}

func TestGetStorageStats_CurrentDir(t *testing.T) {
	stats := GetStorageStats([]string{"/"})
	if len(stats) == 0 {
		t.Fatal("expected at least 1 storage stat for /")
	}
	if stats[0].TotalBytes <= 0 {
		t.Errorf("expected positive TotalBytes, got %d", stats[0].TotalBytes)
	}
	if stats[0].FreeBytes < 0 {
		t.Errorf("expected non-negative FreeBytes, got %d", stats[0].FreeBytes)
	}
}

func TestGetStorageStats_DeduplicatesPaths(t *testing.T) {
	stats := GetStorageStats([]string{"/", "/", "/"})
	if len(stats) != 1 {
		t.Errorf("expected 1 result after dedup, got %d", len(stats))
	}
}

func TestGetStorageStats_GroupsPathsOnSameDisk(t *testing.T) {
	// Two distinct paths on the same filesystem (subdirs of one temp dir share
	// the device id) must collapse into a single disk entry - the same drive's
	// capacity is not counted twice - while still listing both library paths.
	root := t.TempDir()
	a := filepath.Join(root, "movies")
	b := filepath.Join(root, "tv")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatal(err)
	}

	stats := GetStorageStats([]string{a, b})
	if len(stats) != 1 {
		t.Fatalf("expected 1 disk entry for 2 paths on the same device, got %d", len(stats))
	}
	if len(stats[0].Paths) != 2 {
		t.Errorf("expected both paths grouped under the disk, got %v", stats[0].Paths)
	}
	if stats[0].TotalBytes <= 0 {
		t.Errorf("expected positive TotalBytes, got %d", stats[0].TotalBytes)
	}
}

func TestGetStorageStats_InvalidPath(t *testing.T) {
	stats := GetStorageStats([]string{"/nonexistent_path_xyz_123"})
	if len(stats) != 0 {
		t.Errorf("expected 0 results for invalid path, got %d", len(stats))
	}
}

func TestReadMeminfo(t *testing.T) {
	if _, err := os.Stat("/proc/meminfo"); err != nil {
		t.Skip("no /proc/meminfo")
	}
	total, avail := readMeminfo()
	if total <= 0 {
		t.Errorf("expected positive total, got %d", total)
	}
	if avail <= 0 {
		t.Errorf("expected positive available, got %d", avail)
	}
	if avail > total {
		t.Errorf("available (%d) > total (%d)", avail, total)
	}
}

func TestCollect_CachesResult(t *testing.T) {
	// Reset cache
	collectMu.Lock()
	cacheExpiresAt = time.Time{}
	collectMu.Unlock()

	s1 := Collect()
	s2 := Collect() // should return cached value instantly

	if s1 != s2 {
		t.Error("second Collect() should return cached result")
	}

	// Expire cache and verify refresh
	collectMu.Lock()
	cacheExpiresAt = time.Now().Add(-time.Second)
	collectMu.Unlock()

	s3 := Collect()
	_ = s3 // just ensure no panic on refresh
}

func TestTreeCPUTicks_CountsTheForkedChildren(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	cmd := exec.Command("sh", "-c", "while :; do :; done")
	if err := cmd.Start(); err != nil {
		t.Skip("no sh")
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	_, before, err := procStat(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	// Frozen before the reads: a running child gains a tick between two of
	// them and the leaf's own tree then misses by one (12 CI runs in 100).
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, child, _ := procStat(cmd.Process.Pid)
	if child <= before {
		t.Skip("the busy child was not scheduled")
	}
	if tree := treeCPUTicks(os.Getpid()); tree < child {
		t.Fatalf("the tree (%d ticks) must include the busy child (%d)", tree, child)
	}
	if other := treeCPUTicks(cmd.Process.Pid); other != child {
		t.Fatalf("a leaf's tree is itself: %d != %d", other, child)
	}
}

func TestReadCPUStat(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); err != nil {
		t.Skip("no /proc/stat")
	}
	sample, ok := readCPUStat()
	if !ok {
		t.Fatal("failed to read /proc/stat")
	}
	if sample.total == 0 {
		t.Error("expected non-zero total")
	}
}

// A probe that fails must not be remembered as "the answer". Under the old
// sync.Once, one failing (or hanging) execution was final: every later caller
// got the empty result forever, and a hang blocked them all permanently.
//
// Nuance since 2026-08-01: a failure IS remembered, but for seconds only
// (ffmpegRetryAt), so a host without ffmpeg does not re-fork on every call.
// This test covers ffmpegProbed, the "final" commitment, which a failure must
// never set. Both halves of the contract live in ffmpeg_probe_cache_test.go.
func TestFFmpegVersion_FailureIsNotCached(t *testing.T) {
	ffmpegMu.Lock()
	ffmpegCached, ffmpegProbed = "", false
	ffmpegMu.Unlock()

	if got := FFmpegVersion("/nonexistent/ffmpeg-does-not-exist"); got != "" {
		t.Fatalf("expected empty version for a missing binary, got %q", got)
	}
	ffmpegMu.Lock()
	probed := ffmpegProbed
	ffmpegMu.Unlock()
	if probed {
		t.Error("a failed probe was cached: the next call would never retry, even once the binary is back")
	}
}

// Process by process: a delta for one alive at both samples, its whole count
// for one born in between (a reused pid is a new process: same pid, new start
// time), nothing for one that exited - never its whole life taken away.
func TestWindowTicks(t *testing.T) {
	before := map[proc]uint64{
		{10, 1}: 100, // keeps running
		{11, 1}: 500, // exits in the window
		{12, 1}: 900, // exits, and pid 12 is reused
	}
	after := map[proc]uint64{
		{10, 1}: 130,
		{12, 7}: 5,
		{13, 8}: 20, // born in the window
	}
	if got := windowTicks(before, after); got != 30+5+20 {
		t.Fatalf("window = %d ticks, want 55", got)
	}
}

// Measured on the old sum: a busy child exiting between the two samples made
// the second sum smaller than the first, and Evey read 0 % while another
// child kept a core busy. The per-process window keeps the survivor's work.
func TestWindowTicks_AChildExitingMidWindowDoesNotZeroTheOthers(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	start := func() *exec.Cmd {
		cmd := exec.Command("sh", "-c", "while :; do :; done")
		if err := cmd.Start(); err != nil {
			t.Skip("no sh")
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	exiting := start()
	time.Sleep(time.Second) // a long life the old sum would take away
	survivor := start()
	roots := []int{os.Getpid()}
	t1 := treeTicks(roots)
	_ = exiting.Process.Kill()
	_ = exiting.Wait()
	time.Sleep(300 * time.Millisecond)
	// Frozen before the read, as in TestTreeCPUTicks_CountsTheForkedChildren.
	if err := survivor.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t2 := treeTicks(roots)
	var sum1, sum2 uint64
	for _, v := range t1 {
		sum1 += v
	}
	for _, v := range t2 {
		sum2 += v
	}
	if sum2 >= sum1 {
		t.Skip("the exiting child had not outrun the survivor; nothing to show")
	}
	if got := windowTicks(t1, t2); got == 0 {
		t.Fatalf("the survivor's work vanished (sums %d -> %d)", sum1, sum2)
	}
}

func TestCompanionPIDs(t *testing.T) {
	cases := map[string][]int{
		"":            nil,
		"123 456":     {123, 456},
		" 789 ":       {789},
		"1 abc 42 -3": {42}, // pid 1 would be the whole container
	}
	for env, want := range cases {
		got := companionPIDs(env)
		if len(got) != len(want) {
			t.Fatalf("%q -> %v, want %v", env, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%q -> %v, want %v", env, got, want)
			}
		}
	}
}

// A companion root brings its whole tree: the embedded Postgres forks one
// backend per connection, all of them its children, none of them ours.
func TestTreeTicks_CompanionRootBringsItsTree(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		t.Skip("no sh")
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	time.Sleep(200 * time.Millisecond)
	tree := treeTicks([]int{cmd.Process.Pid})
	var pids []int
	for p := range tree {
		pids = append(pids, p.pid)
		if p.pid == os.Getpid() {
			t.Fatal("the companion's tree must not climb up to its parent")
		}
	}
	if len(pids) < 2 {
		t.Fatalf("companion tree %v: want the root and its sleep child", pids)
	}
}
