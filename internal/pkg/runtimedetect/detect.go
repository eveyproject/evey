package runtimedetect

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DetectedRuntime struct {
	Language string `json:"language"`
	Command  string `json:"command"`
	Version  string `json:"version"`
}

// knownRuntimes are the launchers whose version this package knows how to
// ask. It is a display catalogue, not a gate: the server starts a plugin
// through any command on its PATH, and DetectRuntimes also lists the ones
// installed plugins name that are absent from here.
var knownRuntimes = []struct {
	language string
	command  string
	args     []string
}{
	{"go", "go", []string{"version"}},
	{"python", "python3", []string{"--version"}},
	{"nodejs", "node", []string{"--version"}},
	{"php", "php", []string{"--version"}},
	{"ruby", "ruby", []string{"--version"}},
	{"lua", "lua", []string{"-v"}},
	{"java", "java", []string{"-version"}},
	{"dotnet", "dotnet", []string{"--list-runtimes"}},
	{"dart", "dart", []string{"--version"}},
	{"elixir", "elixir", []string{"--short-version"}},
}

// DetectRuntimes lists the plugin runtimes of this image: the known ones
// found on PATH, with their version, then each launcher an installed plugin
// starts through (usedLaunchers, absolute paths) that the catalogue does not
// know. Those carry no version: a command named by a plugin manifest is not
// run by the server outside the sandbox just to print one.
func DetectRuntimes(usedLaunchers ...string) []DetectedRuntime {
	var result []DetectedRuntime
	known := map[string]bool{}
	for _, rt := range knownRuntimes {
		known[rt.command] = true
		ver := probeRuntime(rt.command, rt.args)
		if ver != "" {
			result = append(result, DetectedRuntime{
				Language: rt.language,
				Command:  rt.command,
				Version:  ver,
			})
		}
	}
	for _, path := range usedLaunchers {
		name := filepath.Base(path)
		if known[name] {
			continue
		}
		known[name] = true
		result = append(result, DetectedRuntime{Language: name, Command: path})
	}
	return result
}

// probeTimeout bounds one `<runtime> --version` call. These answer instantly
// when the binary is healthy; without a bound, a single interpreter sitting on
// a stalled mount blocks the admin request that asked which runtimes exist -
// and every retry leaks another goroutine and file descriptor behind it.
const probeTimeout = 3 * time.Second

func probeRuntime(command string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	// G204: command and args come from the hardcoded knownRuntimes table above,
	// never from a request. Combined output: java prints its version on stderr.
	cmd := exec.CommandContext(ctx, command, args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return firstVersionLine(string(out))
}

// firstVersionLine keeps the first line of a version banner, without the
// install path dotnet appends ("Microsoft.NETCore.App 8.0.0 [/usr/share/...]").
func firstVersionLine(out string) string {
	ver := strings.TrimSpace(out)
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	if idx := strings.Index(ver, " ["); idx >= 0 {
		ver = ver[:idx]
	}
	return strings.TrimSpace(ver)
}
