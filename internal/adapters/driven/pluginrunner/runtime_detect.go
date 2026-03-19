package pluginrunner

import (
	"strings"

	"github.com/gravity-zero/evey/internal/pkg/runtimedetect"
)

type DetectedRuntime = runtimedetect.DetectedRuntime

// DetectRuntimes probes the system PATH for known plugin runtimes.
func DetectRuntimes() []DetectedRuntime {
	return runtimedetect.DetectRuntimes()
}

// IsRuntimeAvailable checks if a runtime for the given language is installed.
func IsRuntimeAvailable(language string) bool {
	lang := strings.ToLower(strings.TrimSpace(language))
	for _, rt := range runtimedetect.DetectRuntimes() {
		if rt.Language == lang || lang == rt.Command {
			return true
		}
	}
	return lang == "go" || lang == "rust" || lang == "c" || lang == "c++"
}
