package runtimedetect

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type DetectedRuntime struct {
	Language string `json:"language"`
	Command  string `json:"command"`
	Version  string `json:"version"`
}

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
}

// DetectRuntimes probes the system PATH for known plugin runtimes.
func DetectRuntimes() []DetectedRuntime {
	var result []DetectedRuntime
	for _, rt := range knownRuntimes {
		ver := probeRuntime(rt.command, rt.args)
		if ver != "" {
			result = append(result, DetectedRuntime{
				Language: rt.language,
				Command:  rt.command,
				Version:  ver,
			})
		}
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
	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	ver := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	return ver
}
