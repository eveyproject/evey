package cpuutil

import (
	"runtime"
	"testing"
)

func TestTrickplayWorkers_Adaptive(t *testing.T) {
	n := TrickplayWorkers(true)
	if n < 1 {
		t.Fatalf("expected >= 1, got %d", n)
	}
	if n > 4 {
		t.Fatalf("expected <= 4, got %d", n)
	}
	expected := runtime.NumCPU() / 4
	if expected < 1 {
		expected = 1
	}
	if expected > 4 {
		expected = 4
	}
	if n != expected {
		t.Fatalf("adaptive: expected %d, got %d (NumCPU=%d)", expected, n, runtime.NumCPU())
	}
}

func TestTrickplayWorkers_NonAdaptive(t *testing.T) {
	n := TrickplayWorkers(false)
	if n < 1 {
		t.Fatalf("expected >= 1, got %d", n)
	}
	if n > 4 {
		t.Fatalf("expected <= 4, got %d", n)
	}
	expected := runtime.NumCPU() / 2
	if expected < 1 {
		expected = 1
	}
	if expected > 4 {
		expected = 4
	}
	if n != expected {
		t.Fatalf("non-adaptive: expected %d, got %d (NumCPU=%d)", expected, n, runtime.NumCPU())
	}
}

func TestTrickplayWorkers_MinimumOne(t *testing.T) {
	n := TrickplayWorkers(true)
	if n < 1 {
		t.Fatal("must return at least 1")
	}
}
