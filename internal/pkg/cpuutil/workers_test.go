package cpuutil

import (
	"testing"

	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// expectedWorkers mirrors the sizing rule against the live CPU budget - the
// tests must not assume the host core count equals the budget (inside a
// container with a CPU quota they differ; that mismatch was the bug).
func expectedWorkers(divisor int) int {
	n := sysinfo.CPUBudget() / divisor
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

func TestTrickplayWorkers_Adaptive(t *testing.T) {
	n := TrickplayWorkers(true)
	if n < 1 || n > 4 {
		t.Fatalf("expected 1..4, got %d", n)
	}
	if expected := expectedWorkers(4); n != expected {
		t.Fatalf("adaptive: expected %d, got %d (budget=%d)", expected, n, sysinfo.CPUBudget())
	}
}

func TestTrickplayWorkers_NonAdaptive(t *testing.T) {
	n := TrickplayWorkers(false)
	if n < 1 || n > 4 {
		t.Fatalf("expected 1..4, got %d", n)
	}
	if expected := expectedWorkers(2); n != expected {
		t.Fatalf("non-adaptive: expected %d, got %d (budget=%d)", expected, n, sysinfo.CPUBudget())
	}
	if adaptive := TrickplayWorkers(true); adaptive > n {
		t.Fatalf("adaptive (%d) must never exceed non-adaptive (%d)", adaptive, n)
	}
}

func TestTrickplayWorkers_MinimumOne(t *testing.T) {
	if n := TrickplayWorkers(true); n < 1 {
		t.Fatal("must return at least 1")
	}
}
