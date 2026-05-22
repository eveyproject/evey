package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/domain/plugin"
)

const launcherBinaryName = "evey-plugin-launcher"

// systemReadOnlyDirs are the host paths a plugin needs to read/execute its
// interpreter and shared libraries. Everything else — notably /data (media,
// JWT key, DB socket) and sibling plugin directories — is denied by Landlock.
var systemReadOnlyDirs = []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc"}

// applyLandlockSandbox wraps cmd with the evey-plugin-launcher shim, which
// applies a Landlock filesystem allowlist (system dirs read-only; the plugin's
// own dir and socket dir read-write) before exec'ing the plugin. Unlike
// bubblewrap it needs no mount/unshare, so it runs under Docker's default
// seccomp + AppArmor with zero host configuration. UID drop and cgroup limits
// are still handled by the caller (SysProcAttr / cgroup attach).
func (r *Runner) applyLandlockSandbox(parentCtx context.Context, p *plugin.Plugin, cmd *exec.Cmd) (func(), error) {
	launcher, err := launcherPath()
	if err != nil {
		slog.Error("evey-plugin-launcher not found",
			slog.String("plugin_id", p.ID), slog.Any("err", err))
		r.publishPluginError(parentCtx, p.ID, "plugin launcher missing; plugin refused start")
		return nil, fmt.Errorf("plugin %s: sandbox_mode=landlock requires %s: %w", p.ID, launcherBinaryName, err)
	}

	pluginDir := p.InstallDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir
	}
	pluginSockDir := filepath.Join(sockDir, p.ID)

	args := make([]string, 0, len(systemReadOnlyDirs)*2+8+len(cmd.Args))
	args = append(args, launcher)
	for _, d := range systemReadOnlyDirs {
		args = append(args, "-ro", d)
	}
	args = append(args, "-rw", pluginDir, "-rw", pluginSockDir, "-require-landlock", "--")
	args = append(args, cmd.Args...) // original argv: interpreter + script (+ extra args)

	cmd.Path = launcher
	cmd.Args = args

	// The plugin dir is the only writable allowlisted location the plugin owns,
	// so point HOME/TMPDIR there. A plugin writing scratch to $HOME=/tmp would
	// otherwise hit a Landlock denial.
	cmd.Env = overrideEnv(cmd.Env, map[string]string{"HOME": pluginDir, "TMPDIR": pluginDir})

	slog.Info("plugin sandbox applied",
		slog.String("plugin_id", p.ID),
		slog.String("sandbox_mode", sandboxModeLandlock),
		slog.String("plugin_dir", pluginDir))
	return nil, nil
}

// resolveAutoMode picks the strongest sandbox the current host actually
// supports: bubblewrap if a throwaway sandbox builds (bare-metal with
// userns + mount), else Landlock if the kernel enforces it, else basic.
func resolveAutoMode() string {
	if ProbeSandbox(sandboxModeBubblewrap) == nil {
		return sandboxModeBubblewrap
	}
	if landlockAvailable() {
		return sandboxModeLandlock
	}
	slog.Warn("plugin sandbox auto-detection fell back to basic mode: no per-path " +
		"filesystem isolation available (bubblewrap unusable here and Landlock absent). " +
		"Plugins run with UID-drop + seccomp only. A kernel ≥5.13 with Landlock in the " +
		"active LSM list enables strong isolation with no host changes.")
	return sandboxModeBasic
}

// launcherPath locates the evey-plugin-launcher binary: an explicit override
// via EVEY_PLUGIN_LAUNCHER, then alongside the running server binary (the image
// ships them in the same directory), then $PATH.
func launcherPath() (string, error) {
	if p := os.Getenv("EVEY_PLUGIN_LAUNCHER"); p != "" {
		if isExecutableFile(p) {
			return p, nil
		}
		return "", fmt.Errorf("EVEY_PLUGIN_LAUNCHER=%q is not an executable file", p)
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), launcherBinaryName)
		if isExecutableFile(cand) {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(launcherBinaryName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found (set EVEY_PLUGIN_LAUNCHER or place it next to the server binary)", launcherBinaryName)
}

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// overrideEnv returns env with the given KEY=VALUE pairs replaced in place (or
// appended if absent). glibc getenv returns the first match, so appending a
// duplicate key would have no effect — existing entries must be rewritten.
func overrideEnv(env []string, kv map[string]string) []string {
	out := make([]string, 0, len(env)+len(kv))
	seen := make(map[string]bool, len(kv))
	for _, e := range env {
		key := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			key = e[:i]
		}
		if v, ok := kv[key]; ok {
			out = append(out, key+"="+v)
			seen[key] = true
			continue
		}
		out = append(out, e)
	}
	for k, v := range kv {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}
