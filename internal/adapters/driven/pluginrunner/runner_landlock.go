package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/plugin"
)

const launcherBinaryName = "evey-plugin-launcher"

// systemReadOnlyDirs are the host paths a plugin needs to read/execute its
// interpreter and shared libraries. Everything else — notably /data (media,
// JWT key, DB socket) and sibling plugin directories — is denied by Landlock.
// /app is included so the dynamic linker can load the LD_PRELOAD seccomp lib
// (/app/libevey-sandbox.so); /app holds no secrets (just the open-source
// binaries, the SDK and the seccomp profile).
var systemReadOnlyDirs = []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc", "/app"}

// devDir exposes the container's pseudo-devices (/dev/null, /dev/zero,
// /dev/random, /dev/urandom, /dev/tty…). Many runtimes and libraries open
// /dev/null and /dev/urandom, so a deny-by-default allowlist would break them.
// It is read-write because /dev/null is written to; this is safe because a
// default Docker container's /dev holds no disk/memory devices, and DAC still
// guards the root-owned nodes against an unprivileged plugin.
const devDir = "/dev"

// buildLauncherArgv builds the evey-plugin-launcher argv wrapping the plugin
// command: a filesystem allowlist (system dirs read-only; the plugin's own dir
// + socket dir read-write), the TCP egress policy, and the kernel seccomp
// denylist. requireLandlock=true fails the plugin closed if Landlock can't be
// enforced (landlock mode); false keeps Landlock best-effort — the basic-mode
// fallback, where the launcher still delivers seccomp (incl. -deny-inet) and
// Landlock-if-available on top of the LD_PRELOAD shim.
func buildLauncherArgv(launcher string, p *plugin.Plugin, cmd *exec.Cmd, requireLandlock bool, dropUID, dropGID int) []string {
	pluginDir := p.InstallDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir
	}
	pluginSockDir := filepath.Join(sockDir, p.ID)

	args := make([]string, 0, len(systemReadOnlyDirs)*2+16+len(cmd.Args))
	args = append(args, launcher)
	for _, d := range systemReadOnlyDirs {
		args = append(args, "-ro", d)
	}
	args = append(args, "-rw", pluginDir, "-rw", pluginSockDir, "-rw", devDir)

	// TCP egress: deny by default (-restrict-net, Landlock ABI v4+). A plugin
	// with the network permission may connect to standard HTTP(S) ports plus any
	// explicit external_fetch port; one without it is cut off from TCP.
	args = append(args, "-restrict-net")
	if slices.Contains(p.Permissions, networkPerm) {
		for _, port := range landlockConnectPorts(p.ExternalFetch) {
			args = append(args, "-net-connect", strconv.Itoa(port))
		}
	}

	// Kernel seccomp denylist installed right before exec, so it also constrains
	// static/compiled plugin binaries (which never load the LD_PRELOAD
	// libevey-sandbox.so and would otherwise bypass that layer). -deny-inet adds
	// socket(AF_INET/AF_INET6) denial for non-network plugins — kernel
	// no-IP-egress that holds even on Landlock ABI <4 where -restrict-net is a no-op.
	args = append(args, "-seccomp")
	if !slices.Contains(p.Permissions, networkPerm) {
		args = append(args, "-deny-inet")
	}

	// Drop to the dedicated plugin uid (distinct from the server) when uid
	// separation is on and the server is non-root; the launcher carries
	// cap_setuid for this. dropUID==0 leaves the plugin at the server's uid.
	if dropUID > 0 {
		args = append(args, "-uid", strconv.Itoa(dropUID), "-gid", strconv.Itoa(dropGID), "-uid-besteffort")
	}

	if requireLandlock {
		args = append(args, "-require-landlock")
	}
	args = append(args, "--")
	args = append(args, cmd.Args...) // original argv: interpreter + script (+ extra args)
	return args
}

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
	cmd.Path = launcher
	du, dg := r.pluginDropArgs(false) // landlock confines the FS; uid-sep only via "on"
	cmd.Args = buildLauncherArgv(launcher, p, cmd, true, du, dg)

	// The plugin dir is the only writable allowlisted location the plugin owns,
	// so point HOME/TMPDIR there. A plugin writing scratch to $HOME=/tmp would
	// otherwise hit a Landlock denial.
	cmd.Env = overrideEnv(cmd.Env, map[string]string{"HOME": pluginDir, "TMPDIR": pluginDir})

	// Layer the LD_PRELOAD seccomp filter (libevey-sandbox.so) on top of
	// Landlock. Landlock confines the filesystem and network; this denylist
	// blocks dangerous syscalls — execve/execveat (so a plugin without the
	// "subprocess" permission cannot spawn children), plus setns, bpf, kexec,
	// keyctl, finit_module, process_vm_readv/writev, perf_event_open. Plugins
	// that declare "subprocess" skip the filter (they are allowed to spawn).
	if !slices.Contains(p.Permissions, subprocessPerm) {
		libPath := r.ensureSandboxLib()
		switch {
		case libPath != "":
			cmd.Env = append(cmd.Env, "LD_PRELOAD="+libPath)
		case !r.allowMissingSandbox:
			slog.Error("refusing to start plugin without seccomp lib (landlock mode)",
				slog.String("plugin_id", p.ID), slog.String("sandbox_lib", sandboxLibPath))
			r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start")
			return nil, fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		default:
			slog.Warn("landlock mode without seccomp lib (dev override)",
				slog.String("plugin_id", p.ID))
		}
	}

	slog.Info("plugin sandbox applied",
		slog.String("plugin_id", p.ID),
		slog.String("sandbox_mode", sandboxModeLandlock),
		slog.String("plugin_dir", pluginDir))
	return nil, nil
}

// landlockConnectPorts returns the TCP ports a network-permitted plugin may
// connect to: the standard HTTP/HTTPS ports plus any explicit ":port" declared
// in its external_fetch list. Landlock filters by port, not host, so the
// per-host DNS whitelist (external_fetch) remains enforced elsewhere; this only
// narrows which TCP ports are reachable at all.
func landlockConnectPorts(externalFetch []string) []int {
	ports := []int{80, 443}
	seen := map[int]bool{80: true, 443: true}
	for _, entry := range externalFetch {
		if _, portStr, err := net.SplitHostPort(strings.TrimSpace(entry)); err == nil {
			if n, perr := strconv.Atoi(portStr); perr == nil && n > 0 && n <= 65535 && !seen[n] {
				ports = append(ports, n)
				seen[n] = true
			}
		}
	}
	return ports
}

// resolveAutoMode picks the sandbox best suited to the host, preferring
// Landlock: it needs no privilege, runs under Docker's default seccomp +
// AppArmor with zero host changes, and can't be relaxed once applied — the
// right default for the primary (containerised) deployment. Bubblewrap is the
// fallback only where Landlock is absent but a throwaway bwrap sandbox actually
// builds (bare-metal / privileged container with userns + mount), then basic.
// Order: landlock -> bubblewrap -> basic.
func resolveAutoMode() string {
	// landlock mode is driven by the evey-plugin-launcher shim, so only pick it
	// when the kernel enforces Landlock AND the launcher binary is actually
	// present — otherwise auto would resolve to a mode that can't run (e.g. a
	// dev tree where only the server binary was built). Bubblewrap is the next
	// choice where it builds (bare-metal), then basic.
	if landlockAvailable() && launcherAvailable() {
		return sandboxModeLandlock
	}
	if ProbeSandbox(sandboxModeBubblewrap) == nil {
		return sandboxModeBubblewrap
	}
	slog.Warn("plugin sandbox auto-detection fell back to basic mode: no per-path " +
		"filesystem isolation available (bubblewrap unusable here and Landlock absent). " +
		"Plugins run with UID-drop + seccomp only. A kernel ≥5.13 with Landlock in the " +
		"active LSM list enables strong isolation with no host changes.")
	return sandboxModeBasic
}

// launcherAvailable reports whether the evey-plugin-launcher binary can be
// located — used by auto-detection to avoid resolving to landlock mode (which
// requires the launcher) on a host that doesn't ship it.
func launcherAvailable() bool {
	_, err := launcherPath()
	return err == nil
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
