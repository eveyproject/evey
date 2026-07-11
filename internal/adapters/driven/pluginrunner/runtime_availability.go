package pluginrunner

import (
	"log/slog"
	"os/exec"
	"sort"
)

// runtimeBinaries maps the manifest language identifier to the binary
// name(s) we look for on PATH. Order matters: the first match wins
// (e.g. lua 5.4 over plain lua).
//
// Compiled languages ("go") are intentionally absent — they have no
// runtime to resolve; isCompiled() short-circuits them as always-available.
var runtimeBinaries = map[string][]string{
	"python": {"python3", "python"},
	"nodejs": {"node", "nodejs"},
	"php":    {"php", "php8"},
	"ruby":   {"ruby"},
	"lua":    {"lua5.4", "lua5.3", "lua"},
	"bash":   {"bash"},
	"sh":     {"sh"},
}

// compiledLanguages need no interpreter — the plugin ships a binary.
var compiledLanguages = map[string]bool{
	"go": true,
}

// RuntimeAvailability is a PATH-scanning implementation of
// plugin.RuntimeAvailabilityChecker. It probes once at construction so
// the install hot path is lock-free + allocation-free.
//
// To re-detect after a runtime is installed at runtime, build a new
// instance. Plugin install is rare enough that hot-reloading is not
// worth the cache invalidation complexity.
type RuntimeAvailability struct {
	available map[string]bool
}

// NewRuntimeAvailability scans PATH for each known plugin runtime and
// returns a checker that can answer IsAvailable in O(1). Errors are
// logged at debug level since "binary not found" is the expected case
// for variants like evey:beta (python only).
func NewRuntimeAvailability() *RuntimeAvailability {
	r := &RuntimeAvailability{available: make(map[string]bool, len(runtimeBinaries)+len(compiledLanguages))}
	for lang, candidates := range runtimeBinaries {
		for _, bin := range candidates {
			if _, err := exec.LookPath(bin); err == nil {
				r.available[lang] = true
				slog.Debug("plugin runtime detected", slog.String("language", lang), slog.String("binary", bin))
				break
			}
		}
	}
	for lang := range compiledLanguages {
		r.available[lang] = true
	}
	return r
}

func (r *RuntimeAvailability) IsAvailable(language string) bool {
	if language == "" {
		// Empty language only applies to themes (no backend). The installer
		// already gates that case; treat as available here to avoid false
		// negatives if a theme ever flows through this checker by mistake.
		return true
	}
	return r.available[language]
}

func (r *RuntimeAvailability) Available() []string {
	out := make([]string, 0, len(r.available))
	for lang := range r.available {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}
