package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// resolveRenderNode returns the host's primary DRM render node
// (/dev/dri/renderD12N), or "" if none exists. Used ONLY to honour the
// hardware.gpu permission. Returns the lowest-numbered render node and never a
// card/primary node (/dev/dri/card*) - the launcher re-validates the shape
// before granting the ioctl-bearing rule.
func resolveRenderNode() string {
	matches, err := filepath.Glob("/dev/dri/renderD[0-9][0-9][0-9]")
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[0]
}

const launcherBinaryName = "evey-plugin-launcher"

// systemReadOnlyDirs are the host paths a plugin needs to read/execute its
// interpreter and shared libraries. Everything else - notably /data (media,
// JWT key, DB socket) and sibling plugin directories - is denied by Landlock.
// /app is included so the dynamic linker can load the LD_PRELOAD seccomp lib
// (/app/libevey-sandbox.so); /app holds no secrets (just the open-source
// binaries, the SDK and the seccomp profile).
var systemReadOnlyDirs = []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc", "/app"}

// safeDevNodes are the only device nodes a plugin may open. We grant them
// individually rather than the whole /dev directory: on a hardware-transcode
// box (the NVIDIA/VAAPI image, `--gpus` + /dev/dri mapped in) the directory
// also holds GPU render nodes (/dev/dri/renderD128, card0), the WSL2 GPU
// paravirt device (/dev/dxg) and NVIDIA nodes (/dev/nvidia*). Allowing all of
// /dev would put those inside the Landlock allowlist, leaving only DAC between
// a plugin and the GPU - a cryptominer could then open a render node and submit
// work. By listing only the harmless pseudo-devices, the GPU nodes stay denied
// by Landlock regardless of DAC, on every vendor (AMD iGPU, Intel, NVIDIA).
// Each is skipped by the launcher if absent. /dev/urandom covers Node/Python
// crypto; /dev/null is the common sink (hence read-write).
//
// /dev/tty is deliberately NOT here: it is the SERVER's controlling terminal,
// not a stateless pseudo-device, and read-write access to it hands every plugin
// ioctl(TIOCSTI) - characters injected into the terminal of whoever launched
// the server. dev.tty.legacy_tiocsti only landed in Linux 6.2 while Landlock
// needs >=5.13, so an ordinary NAS kernel is exposed. No plugin needs it: Start
// already wires stdout/stderr and a daemon has no legitimate terminal.
var safeDevNodes = []string{"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom"}

// buildLauncherArgv builds the evey-plugin-launcher argv wrapping the plugin
// command: a filesystem allowlist (system dirs read-only; the plugin's own dir
// + socket dir read-write), the TCP egress policy, and the kernel seccomp
// denylist. requireLandlock=true fails the plugin closed if Landlock can't be
// enforced (landlock mode); false keeps Landlock best-effort - the basic-mode
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
	args = append(args, "-rw", pluginDir, "-rw", pluginSockDir)
	for _, d := range safeDevNodes {
		args = append(args, "-dev", d)
	}
	// hardware.gpu (highest-risk, admin-only, first-party tier): grant the DRM
	// render node ALONE - with the GPU ioctl surface - via -render-dev. Resolved
	// to the real renderD12N here (never a card node, never /dev/dxg or
	// /dev/nvidia*, never /dev/kfd); skipped if the plugin lacks the permission
	// or the host exposes no render node. NVIDIA compute would need a separate,
	// even-more-privileged tier (not implied by hardware.gpu).
	if slices.Contains(p.Permissions, gpuPerm) {
		if node := resolveRenderNode(); node != "" {
			args = append(args, "-render-dev", node)
		}
	}

	// TCP egress: denied for ALL plugins (-restrict-net with no -net-connect
	// grants, Landlock ABI v4+). No plugin gets direct network egress; the
	// network permission instead unlocks the host Fetch broker, reached over the
	// AF_UNIX host socket (unaffected by these network restrictions).
	args = append(args, "-restrict-net")

	// Kernel seccomp denylist installed right before exec, so it also constrains
	// static/compiled plugin binaries (which never load the LD_PRELOAD
	// libevey-sandbox.so and would otherwise bypass that layer). -deny-inet
	// denies socket(AF_INET/AF_INET6) for EVERY plugin - a kernel-enforced
	// no-IP-egress that holds even on Landlock ABI <4 where -restrict-net is a
	// no-op. AF_UNIX (the host broker socket) is untouched.
	args = append(args, "-seccomp", "-deny-inet")

	// -deny-fork: refuse process creation (fork/vfork, and clone without
	// CLONE_THREAD) in the kernel. This is the half of the resource story an
	// rlimit cannot do - the userspace sweep polls /proc every ~2s, and a fork
	// bomb saturates the PID table long before the next tick. Thread creation
	// stays allowed, so node/python/Go runtimes are untouched.
	//
	// Gated on the SAME permission that already governs the LD_PRELOAD shim: a
	// plugin declaring "subprocess" is asking to spawn children and keeps the
	// right to. Without it, execve is already denied by libevey-sandbox.so, so
	// forking could only ever clone the plugin itself - no legitimate use, and
	// the analyzer flags the imports that lead there (child_process, os.popen)
	// at upload time so the author learns which permission to declare.
	if !slices.Contains(p.Permissions, subprocessPerm) {
		args = append(args, "-deny-fork")
	}

	// Drop to the dedicated plugin uid (distinct from the server) when uid
	// separation is on and the server is non-root; the launcher carries
	// cap_setuid for this. dropUID==0 leaves the plugin at the server's uid.
	// Best-effort only on top of an enforced Landlock: in basic mode the uid is
	// the secret wall, so a failed drop refuses the plugin instead.
	if dropUID > 0 {
		args = append(args, "-uid", strconv.Itoa(dropUID), "-gid", strconv.Itoa(dropGID))
		if requireLandlock {
			args = append(args, "-uid-besteffort")
		}
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
		slog.ErrorContext(parentCtx, "evey-plugin-launcher not found",
			slog.String("plugin_id", p.ID), slog.Any("err", err))
		r.publishPluginError(parentCtx, p.ID, "plugin launcher missing; plugin refused start")
		return nil, fmt.Errorf("plugin %s: sandbox_mode=landlock requires %s: %w", p.ID, launcherBinaryName, err)
	}

	pluginDir := p.InstallDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir
	}
	cmd.Path = launcher
	du, dg := r.pluginDropArgs(p.ID, false) // landlock confines the FS; uid-sep only via "on"
	cmd.Args = buildLauncherArgv(launcher, p, cmd, true, du, dg)

	// The plugin dir is the only writable allowlisted location the plugin owns,
	// so point HOME/TMPDIR there. A plugin writing scratch to $HOME=/tmp would
	// otherwise hit a Landlock denial.
	cmd.Env = overrideEnv(cmd.Env, map[string]string{"HOME": pluginDir, "TMPDIR": pluginDir})

	// Layer the LD_PRELOAD seccomp filter (libevey-sandbox.so) on top of
	// Landlock. Landlock confines the filesystem and network; this denylist
	// blocks dangerous syscalls - execve/execveat (so a plugin without the
	// "subprocess" permission cannot spawn children), plus setns, bpf, kexec,
	// keyctl, finit_module, process_vm_readv/writev, perf_event_open. Plugins
	// that declare "subprocess" skip the filter (they are allowed to spawn).
	if !slices.Contains(p.Permissions, subprocessPerm) {
		libPath := r.ensureSandboxLib()
		switch {
		case libPath != "":
			// overrideEnv, not append: see its doc. glibc getenv takes the FIRST
			// match, so appending behind a pre-existing LD_PRELOAD would silently
			// leave the seccomp shim unloaded - no error, no log. buildPluginEnv is
			// a closed allowlist today, so there is none; that is an unexpressed
			// invariant this must not depend on the day plugins can set env vars.
			cmd.Env = overrideEnv(cmd.Env, map[string]string{"LD_PRELOAD": libPath})
		case !r.allowMissingSandbox:
			slog.ErrorContext(parentCtx, "refusing to start plugin without seccomp lib (landlock mode)",
				slog.String("plugin_id", p.ID), slog.String("sandbox_lib", sandboxLibPath))
			r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start")
			return nil, fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		default:
			slog.WarnContext(parentCtx, "landlock mode without seccomp lib (dev override)",
				slog.String("plugin_id", p.ID))
		}
	}

	slog.Info("plugin sandbox applied",
		slog.String("plugin_id", p.ID),
		slog.String("sandbox_mode", sandboxModeLandlock),
		slog.String("plugin_dir", pluginDir))
	return nil, nil
}

// resolveAutoMode picks the sandbox best suited to the host, preferring
// Landlock: it needs no privilege, runs under Docker's default seccomp +
// AppArmor with zero host changes, and can't be relaxed once applied - the
// right default for the primary (containerised) deployment. Bubblewrap is the
// fallback only where Landlock is absent but a throwaway bwrap sandbox actually
// builds (bare-metal / privileged container with userns + mount), then basic.
// Order: landlock -> bubblewrap -> basic.
func resolveAutoMode() string {
	// landlock mode is driven by the evey-plugin-launcher shim, so only pick it
	// when the kernel enforces Landlock AND the launcher binary is actually
	// present - otherwise auto would resolve to a mode that can't run (e.g. a
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
// located - used by auto-detection to avoid resolving to landlock mode (which
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
	info, err := os.Stat(p) //nolint:gosec // stat-only probe of operator-supplied launcher candidates
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// overrideEnv returns env with the given KEY=VALUE pairs replaced in place (or
// appended if absent). glibc getenv returns the first match, so appending a
// duplicate key would have no effect - existing entries must be rewritten.
func overrideEnv(env []string, kv map[string]string) []string {
	out := make([]string, 0, len(env)+len(kv))
	seen := make(map[string]bool, len(kv))
	for _, e := range env {
		key := e
		if before, _, ok := strings.Cut(e, "="); ok {
			key = before
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
