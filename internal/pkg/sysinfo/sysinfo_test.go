package sysinfo

import (
	"os"
	"testing"
)

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
