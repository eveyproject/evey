package sysinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// stageFS builds a fake filesystem root with the given file contents keyed by
// relative path (e.g. "sys/fs/cgroup/memory.max").
func stageFS(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMemoryBudgetBytes_CgroupV2Limit(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/memory.max": "2147483648\n",
		"proc/meminfo":             "MemTotal:       16384000 kB\n",
	})
	got, ok := memoryBudgetBytes(root)
	if !ok || got != 2147483648 {
		t.Errorf("got (%d, %v), want (2147483648, true)", got, ok)
	}
}

func TestMemoryBudgetBytes_CgroupV2Unlimited_FallsBackToHost(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/memory.max": "max\n",
		"proc/meminfo":             "MemTotal:       2048000 kB\n",
	})
	got, ok := memoryBudgetBytes(root)
	if !ok || got != 2048000*1024 {
		t.Errorf("got (%d, %v), want (%d, true)", got, ok, int64(2048000)*1024)
	}
}

func TestMemoryBudgetBytes_CgroupV1SentinelClampedToHost(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/memory/memory.limit_in_bytes": "9223372036854771712\n",
		"proc/meminfo": "MemTotal:       2048000 kB\n",
	})
	got, ok := memoryBudgetBytes(root)
	if !ok || got != 2048000*1024 {
		t.Errorf("got (%d, %v), want host total (%d, true)", got, ok, int64(2048000)*1024)
	}
}

func TestMemoryBudgetBytes_NothingReadable(t *testing.T) {
	got, ok := memoryBudgetBytes(t.TempDir())
	if ok || got != 0 {
		t.Errorf("got (%d, %v), want (0, false)", got, ok)
	}
}

func TestCPUBudget_CgroupV2Quota(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/cpu.max": "150000 100000\n",
	})
	if got := cpuBudget(root); got != 2 {
		t.Errorf("1.5 CPU quota should round up to 2, got %d", got)
	}
}

func TestCPUBudget_CgroupV1Quota(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/cpu/cpu.cfs_quota_us":  "400000\n",
		"sys/fs/cgroup/cpu/cpu.cfs_period_us": "100000\n",
	})
	if got := cpuBudget(root); got != 4 {
		t.Errorf("4 CPU quota, got %d", got)
	}
}

func TestCPUBudget_Unlimited_FallsBackToNumCPU(t *testing.T) {
	root := stageFS(t, map[string]string{
		"sys/fs/cgroup/cpu.max": "max 100000\n",
	})
	if got := cpuBudget(root); got != runtime.NumCPU() {
		t.Errorf("unlimited quota should fall back to NumCPU (%d), got %d", runtime.NumCPU(), got)
	}
}
