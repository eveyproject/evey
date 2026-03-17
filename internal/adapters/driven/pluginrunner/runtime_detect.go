package pluginrunner

import (
	"os/exec"
	"strings"
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

// IsRuntimeAvailable checks if a runtime for the given language is installed.
func IsRuntimeAvailable(language string) bool {
	lang := strings.ToLower(strings.TrimSpace(language))
	for _, rt := range knownRuntimes {
		if rt.language == lang || lang == rt.command {
			return probeRuntime(rt.command, rt.args) != ""
		}
	}
	// Go compiled binaries don't need a runtime
	return lang == "go" || lang == "rust" || lang == "c" || lang == "c++"
}

func probeRuntime(command string, args []string) string {
	cmd := exec.Command(command, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	ver := strings.TrimSpace(string(out))
	// take first line only
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	return ver
}
