package pluginrunner

import (
	"archive/zip"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"
)

// shebangInterpreters maps a manifest language to the interpreter a script
// the plugin ships runs through ("entrypoint: main.py" + "#!/usr/bin/env
// python3"): such an entrypoint does not name its interpreter, the language
// does. An entrypoint that names its launcher ("java -jar plugin.jar",
// "dotnet plugin.dll") is checked on PATH, whatever its language.
var shebangInterpreters = map[string][]string{
	"python": {"python3", "python"},
	"nodejs": {"node", "nodejs"},
	"php":    {"php", "php8"},
	"ruby":   {"ruby"},
	"lua":    {"lua5.4", "lua5.3", "lua"},
	"bash":   {"bash"},
	"sh":     {"sh"},
}

// RuntimeAvailability is the PATH-scanning implementation of
// plugin.RuntimeAvailabilityChecker. It reads the image as it is at each
// install, so a runtime added to the image is usable without a restart.
type RuntimeAvailability struct {
	// pluginDirs are the directories plugins are installed under: an absolute
	// entrypoint inside them is a file of the plugin, never a launcher.
	pluginDirs []string
}

func NewRuntimeAvailability(pluginDirs ...string) *RuntimeAvailability {
	return &RuntimeAvailability{pluginDirs: pluginDirs}
}

// CheckEntrypoint mirrors how the runner starts the plugin (resolveCommand):
// a first word naming a file the zip ships is the plugin's own binary or
// script, anything else must be a launcher installed in the image.
func (r *RuntimeAvailability) CheckEntrypoint(zipPath, language, entrypoint string) error {
	fields := strings.Fields(entrypoint)
	if len(fields) == 0 {
		return errors.New("empty entrypoint")
	}
	name := fields[0]
	shipped, err := zipShips(zipPath, name)
	if err != nil {
		return err
	}
	if shipped {
		candidates, interpreted := shebangInterpreters[language]
		if !interpreted || anyOnPath(candidates) {
			return nil
		}
		return fmt.Errorf("the %s interpreter (%s) is not installed in this image", language, strings.Join(candidates, " or "))
	}
	if _, ok := lookupLauncher(name, r.pluginDirs); ok {
		return nil
	}
	return fmt.Errorf("the entrypoint runs %q, which is neither installed in this image nor shipped by the plugin", name)
}

// Available lists the script languages whose interpreter this image has, for
// the hint of a refused install.
func (r *RuntimeAvailability) Available() []string {
	var out []string
	for lang, candidates := range shebangInterpreters {
		if anyOnPath(candidates) {
			out = append(out, lang)
		}
	}
	sort.Strings(out)
	return out
}

func anyOnPath(candidates []string) bool {
	for _, bin := range candidates {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// zipShips reports whether the plugin archive holds a file at name, relative
// to its root (where plugin.yaml lives). An absolute name is never shipped:
// the zip extracts under the plugin's own dir.
func zipShips(zipPath, name string) (bool, error) {
	if strings.HasPrefix(name, "/") {
		return false, nil
	}
	want := path.Clean(name)
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false, fmt.Errorf("open plugin archive: %w", err)
	}
	defer zr.Close() //nolint:errcheck
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && path.Clean(f.Name) == want {
			return true, nil
		}
	}
	return false, nil
}
