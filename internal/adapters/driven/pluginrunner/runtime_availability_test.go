package pluginrunner

import (
	"slices"
	"testing"
)

func TestRuntimeAvailability_DetectsHostBinaries(t *testing.T) {
	r := NewRuntimeAvailability()
	// "sh" is mandated by POSIX-compliant systems and present in every
	// Linux/macOS CI runner — safe anchor for the detection path.
	if !r.IsAvailable("sh") {
		t.Errorf("expected sh to be available on a POSIX host, got false. available=%v", r.Available())
	}
}

func TestRuntimeAvailability_CompiledLangAlwaysAvailable(t *testing.T) {
	r := NewRuntimeAvailability()
	if !r.IsAvailable("go") {
		t.Error("go is compiled — should be available regardless of PATH")
	}
}

func TestRuntimeAvailability_UnknownLanguageMissing(t *testing.T) {
	r := NewRuntimeAvailability()
	if r.IsAvailable("cobol-85") {
		t.Error("a fabricated language should report missing")
	}
}

func TestRuntimeAvailability_EmptyLanguageTreatedAsAvailable(t *testing.T) {
	// Theme plugins have no language; the installer already validates that
	// upstream. Empty must not falsely block.
	r := NewRuntimeAvailability()
	if !r.IsAvailable("") {
		t.Error("empty language should not report as missing")
	}
}

func TestRuntimeAvailability_AvailableIsSorted(t *testing.T) {
	r := NewRuntimeAvailability()
	got := r.Available()
	if !slices.IsSorted(got) {
		t.Errorf("Available() must return sorted slice, got %v", got)
	}
}
