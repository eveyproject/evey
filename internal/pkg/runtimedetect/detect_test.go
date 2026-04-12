package runtimedetect

import (
	"testing"
)

func TestDetectRuntimes_ReturnsGoAtLeast(t *testing.T) {
	runtimes := DetectRuntimes()
	found := false
	for _, rt := range runtimes {
		if rt.Language == "go" {
			found = true
			if rt.Command != "go" {
				t.Errorf("expected command=go, got %q", rt.Command)
			}
			if rt.Version == "" {
				t.Error("expected non-empty version")
			}
		}
		if rt.Language == "" {
			t.Error("language must not be empty")
		}
		if rt.Version == "" {
			t.Errorf("version empty for %s", rt.Language)
		}
	}
	if !found {
		t.Fatal("expected Go runtime to be detected (tests run in Go)")
	}
}

func TestProbeRuntime_InvalidCommand(t *testing.T) {
	ver := probeRuntime("definitely-not-a-command-that-exists", []string{"--version"})
	if ver != "" {
		t.Fatalf("expected empty version for missing command, got %q", ver)
	}
}

func TestProbeRuntime_Go(t *testing.T) {
	ver := probeRuntime("go", []string{"version"})
	if ver == "" {
		t.Fatal("expected go version output")
	}
}
