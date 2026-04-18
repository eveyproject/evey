package library

import (
	"runtime"
	"testing"
)

func TestScanWorkerCount_DefaultNumCPU(t *testing.T) {
	t.Setenv("EVEY_LIBRARY_SCAN_WORKERS", "")
	if got, want := scanWorkerCount(), runtime.NumCPU(); got != want {
		t.Fatalf("default = NumCPU: got %d want %d", got, want)
	}
}

func TestScanWorkerCount_EnvOverride(t *testing.T) {
	t.Setenv("EVEY_LIBRARY_SCAN_WORKERS", "3")
	if got := scanWorkerCount(); got != 3 {
		t.Fatalf("override=3: got %d", got)
	}
}

func TestScanWorkerCount_InvalidFallsBack(t *testing.T) {
	t.Setenv("EVEY_LIBRARY_SCAN_WORKERS", "not-a-number")
	if got, want := scanWorkerCount(), runtime.NumCPU(); got != want {
		t.Fatalf("invalid env -> fallback NumCPU: got %d want %d", got, want)
	}
}

func TestScanWorkerCount_ZeroFallsBack(t *testing.T) {
	t.Setenv("EVEY_LIBRARY_SCAN_WORKERS", "0")
	if got, want := scanWorkerCount(), runtime.NumCPU(); got != want {
		t.Fatalf("zero -> fallback: got %d want %d", got, want)
	}
}
