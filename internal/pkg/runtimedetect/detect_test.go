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

// A launcher an installed plugin starts through is listed even when the
// catalogue does not know it, once, and without running it for a version.
func TestDetectRuntimes_ListsLaunchersPluginsUse(t *testing.T) {
	runtimes := DetectRuntimes("/opt/acme/bin/acme-vm", "/opt/acme/bin/acme-vm", "/usr/bin/java")
	var acme, java int
	for _, rt := range runtimes {
		switch {
		case rt.Language == "acme-vm":
			acme++
			if rt.Command != "/opt/acme/bin/acme-vm" || rt.Version != "" {
				t.Errorf("unexpected entry %+v", rt)
			}
		case rt.Language == "java" && rt.Version == "":
			java++
		}
	}
	if acme != 1 {
		t.Errorf("want the plugin's launcher listed once, got %d in %+v", acme, runtimes)
	}
	if java != 0 {
		t.Errorf("a catalogue launcher must come from its probe, not from the plugin list: %+v", runtimes)
	}
}

func TestFirstVersionLine(t *testing.T) {
	cases := map[string]string{
		"openjdk version \"21.0.1\" LTS\nOpenJDK Runtime":          `openjdk version "21.0.1" LTS`,
		"Microsoft.NETCore.App 8.0.0 [/usr/share/dotnet/shared]\n": "Microsoft.NETCore.App 8.0.0",
		"v22.1.0\n": "v22.1.0",
	}
	for in, want := range cases {
		if got := firstVersionLine(in); got != want {
			t.Errorf("firstVersionLine(%q) = %q, want %q", in, got, want)
		}
	}
}
