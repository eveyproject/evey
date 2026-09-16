package sysinfo

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func skipUnlessLinuxProc(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc not mounted")
	}
}

// The admin health page reads these for the server process and for each
// ffmpeg child; a dead pid must surface as an error (the row is dropped),
// never as a zero that looks like an idle process.
func TestProcRSSAndCPU_SelfAndDeadPid(t *testing.T) {
	skipUnlessLinuxProc(t)

	rss, err := ProcRSSBytes(os.Getpid())
	if err != nil || rss <= 0 {
		t.Fatalf("ProcRSSBytes(self) = %d, %v", rss, err)
	}
	if got := SelfRSSBytes(); got <= 0 {
		t.Errorf("SelfRSSBytes = %d", got)
	}
	cpu, err := ProcCPUSeconds(os.Getpid())
	if err != nil || cpu < 0 {
		t.Fatalf("ProcCPUSeconds(self) = %v, %v", cpu, err)
	}

	// pid_max on Linux is at most 2^22; nothing lives here.
	const deadPid = 4194304 + 1000
	if _, err := ProcRSSBytes(deadPid); err == nil {
		t.Error("ProcRSSBytes(dead pid) should fail")
	}
	if _, err := ProcCPUSeconds(deadPid); err == nil {
		t.Error("ProcCPUSeconds(dead pid) should fail")
	}
}

func TestLoadAverageAndMemInfo_Live(t *testing.T) {
	skipUnlessLinuxProc(t)

	l1, l5, l15, ok := LoadAverage()
	if !ok || l1 < 0 || l5 < 0 || l15 < 0 {
		t.Fatalf("LoadAverage = %v %v %v ok=%v", l1, l5, l15, ok)
	}

	mi := ReadMemInfo()
	if mi.TotalBytes <= 0 {
		t.Fatalf("TotalBytes = %d", mi.TotalBytes)
	}
	if mi.AvailableBytes < 0 || mi.AvailableBytes > mi.TotalBytes {
		t.Errorf("AvailableBytes = %d out of [0, %d]", mi.AvailableBytes, mi.TotalBytes)
	}
	// A budget is only reported when it caps the host, so it can never exceed it.
	if mi.BudgetBytes < 0 || mi.BudgetBytes >= mi.TotalBytes {
		t.Errorf("BudgetBytes = %d against total %d", mi.BudgetBytes, mi.TotalBytes)
	}
	if b, ok := MemoryBudgetBytes(); ok && b <= 0 {
		t.Errorf("MemoryBudgetBytes = %d with ok", b)
	}
	if n := CPUBudget(); n < 1 {
		t.Errorf("CPUBudget = %d, want at least 1", n)
	}
	if a, ok := MemAvailableBytes(); ok && a < 0 {
		t.Errorf("MemAvailableBytes = %d", a)
	}
}

// Live probes of the two /proc readers that have no injectable input: the
// mount table must resolve the root, and IsWSL must agree with the kernel
// version string this very process reads.
func TestMountFSTypeAndIsWSL_Live(t *testing.T) {
	skipUnlessLinuxProc(t)

	if got := MountFSType("/"); got == "" {
		t.Error("MountFSType(\"/\") is empty on a live Linux host")
	}
	if got := MountFSType(t.TempDir()); got == "" {
		t.Error("a fresh temp dir sits on some mount")
	}

	v, err := os.ReadFile("/proc/version")
	if err != nil {
		t.Skip("no /proc/version")
	}
	lower := strings.ToLower(string(v))
	want := strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
	if got := IsWSL(); got != want {
		t.Errorf("IsWSL = %v, kernel version %q", got, strings.TrimSpace(string(v)))
	}
}
