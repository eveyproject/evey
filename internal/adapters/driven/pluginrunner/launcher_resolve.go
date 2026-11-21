package pluginrunner

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// A plugin entrypoint names either a file the plugin ships ("plugin",
// "./bin/plugin", "main.py") or a launcher installed in the image ("java -jar
// plugin.jar", "dotnet plugin.dll", "python3 main.py"). Any launcher on the
// server's PATH qualifies: adding a runtime to the image (see
// docker/Dockerfile.plugins.example) is enough, no language list to extend.

// boundSystemDirs are visible in every sandbox mode (bwrap binds them, Landlock
// allows them). A runtime installed elsewhere needs its own read-only grant.
var boundSystemDirs = []string{"/usr", "/bin", "/sbin", "/lib", "/lib64"}

// runtimeRootPrefixes are the only places a runtime installed outside
// boundSystemDirs is exposed from: where an image or a host puts add-on
// software. A launcher resolving anywhere else (a data directory, a home, the
// server's own tree) is refused rather than have its surroundings opened.
var runtimeRootPrefixes = []string{"/opt"}

// runtimeScanLimit bounds the walk of a runtime's install tree looking for
// links into /etc. A JRE or a .NET runtime holds a few hundred entries.
const runtimeScanLimit = 20000

// etcDirsNeverExposed stay out of the sandbox even when a runtime links into them.
var etcDirsNeverExposed = map[string]bool{"/etc/ssh": true, "/etc/sudoers.d": true}

// runtimeReadPaths caches, per launcher path, the paths its runtime reads
// outside boundSystemDirs. Filled only by lookupLauncher, so a binary the
// plugin ships never gets a grant computed from its own (attacker-shaped) tree.
var runtimeReadPaths sync.Map // string -> []string

// lookupLauncher resolves an entrypoint's first word to a launcher installed
// in the image: a bare command name found on the server's PATH, or the
// absolute path of an executable in one of its directories. Returns the
// absolute path the plugin is exec'd through (argv[0] keeps its name, so a
// multi-call binary still dispatches) and records what its runtime reads.
func lookupLauncher(name string, pluginDirs []string) (string, bool) {
	var path string
	switch {
	case filepath.IsAbs(name):
		path = filepath.Clean(name)
		if insideAny(path, pluginDirs) || !onPathDir(path) || !isExecutableFile(path) {
			return "", false
		}
	case strings.ContainsRune(name, '/'):
		return "", false
	default:
		found, err := exec.LookPath(name)
		if err != nil || !filepath.IsAbs(found) {
			return "", false
		}
		path = found
	}
	if _, ok := runtimeReadPaths.Load(path); !ok {
		paths, err := computeRuntimeReadPaths(path, pluginDirs)
		if err != nil {
			slog.Warn("plugin launcher refused", slog.String("launcher", path), slog.Any("err", err))
			return "", false
		}
		runtimeReadPaths.Store(path, paths)
	}
	return path, true
}

// launcherReadPaths returns the extra read-only paths recorded for a launcher
// resolved by lookupLauncher, nil for anything else.
func launcherReadPaths(path string) []string {
	if v, ok := runtimeReadPaths.Load(path); ok {
		paths, _ := v.([]string)
		return paths
	}
	return nil
}

// onPathDir reports whether path sits directly in a directory of the server's
// PATH: "/usr/bin/java" names the launcher "java" does, an executable anywhere
// else in the image is no launcher.
func onPathDir(path string) bool {
	dir := filepath.Dir(path)
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.IsAbs(d) && filepath.Clean(d) == dir {
			return true
		}
	}
	return false
}

func insideAny(path string, dirs []string) bool {
	for _, d := range dirs {
		d = filepath.Clean(d)
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// computeRuntimeReadPaths lists what a launcher needs beyond boundSystemDirs:
//   - the directories in /etc its symlink chain passes through (Debian's
//     /usr/bin/java -> /etc/alternatives/java -> /usr/lib/jvm/.../bin/java);
//   - its install root when it lives outside the system dirs (/opt/...);
//   - the directories in /etc its install root links into (the Debian JRE keeps
//     conf/ and lib/security/ as links to /etc/java-<N>-openjdk).
//
// The install root is the directory holding the real binary, or the parent of
// that directory when it is a "bin". A root shared by unrelated packages (/usr,
// /usr/local) is not walked: only a runtime's own tree is. A root outside the
// system dirs must pass runtimeRootExposable, or the launcher is refused.
func computeRuntimeReadPaths(launcher string, pluginDirs []string) ([]string, error) {
	set := map[string]bool{}
	cur := launcher
	for range 16 {
		target, err := os.Readlink(cur)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = filepath.Clean(target)
		addEtcDir(set, cur)
	}

	real, err := filepath.EvalSymlinks(launcher)
	if err != nil {
		return nil, fmt.Errorf("resolve launcher: %w", err)
	}
	if insideAny(real, boundSystemDirs) {
		root := filepath.Dir(real)
		if filepath.Base(root) == "bin" {
			root = filepath.Dir(root)
		}
		// Its own tree (/usr/lib/jvm/<jre>), not one shared by every package.
		if len(strings.Split(strings.Trim(root, "/"), "/")) >= 3 {
			scanEtcLinks(set, root)
		}
		return sortedKeys(set), nil
	}
	root := filepath.Dir(real)
	if filepath.Base(root) == "bin" {
		root = filepath.Dir(root)
	}
	if err := runtimeRootExposable(root, pluginDirs); err != nil {
		return nil, err
	}
	set[root] = true
	scanEtcLinks(set, root)
	return sortedKeys(set), nil
}

// runtimeRootExposable decides whether the install root of a runtime living
// outside the system dirs may be opened read-only to the plugins it runs. It
// must sit strictly below one of runtimeRootPrefixes, and hold neither the
// plugin directories (nor sit inside one: another plugin's files) nor the
// server itself (its executable, its working directory): on a host where
// everything lives under one tree, /opt/evey/bin/java would otherwise open
// /opt/evey, data included.
func runtimeRootExposable(root string, pluginDirs []string) error {
	below := false
	for _, prefix := range runtimeRootPrefixes {
		if root != prefix && insideAny(root, []string{prefix}) {
			below = true
			break
		}
	}
	if !below {
		return fmt.Errorf("runtime root %q is outside the system directories and not below %v", root, runtimeRootPrefixes)
	}
	protected := slices.Clone(pluginDirs)
	if exe, err := os.Executable(); err == nil {
		protected = append(protected, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		protected = append(protected, wd)
	}
	for _, dir := range protected {
		dir = filepath.Clean(dir)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}
		if insideAny(dir, []string{root}) || insideAny(root, []string{dir}) {
			return fmt.Errorf("runtime root %q overlaps %q, which plugins must not read", root, dir)
		}
	}
	return nil
}

func scanEtcLinks(set map[string]bool, root string) {
	seen := 0
	errStop := errors.New("scan limit")
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree only loses its own links
		}
		seen++
		if seen > runtimeScanLimit {
			return errStop
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		target, rerr := os.Readlink(path)
		if rerr != nil {
			return nil //nolint:nilerr // a racing removal only loses this link
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		addEtcDir(set, filepath.Clean(target))
		return nil
	})
	if walkErr != nil {
		// Past the limit the links found so far stand; the runtime starts without the rest.
		slog.Warn("runtime tree too large to scan for /etc links", slog.String("root", root))
	}
}

// addEtcDir records the top-level /etc directory holding path. Only
// directories: a file at the root of /etc (passwd, shadow, group) never is.
func addEtcDir(set map[string]bool, path string) {
	rest, ok := strings.CutPrefix(path, "/etc/")
	if !ok {
		return
	}
	top, _, _ := strings.Cut(rest, "/")
	dir := "/etc/" + top
	if top == "" || top == "." || top == ".." || etcDirsNeverExposed[dir] {
		return
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		set[dir] = true
	}
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
