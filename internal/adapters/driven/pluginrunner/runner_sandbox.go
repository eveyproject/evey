package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/evey-project/evey/internal/domain/plugin"
)

// runner_sandbox.go holds the sandbox selection and application: mode
// resolution, the basic and bubblewrap sandboxes, uid separation and the
// reported sandbox status.

// applySandbox sets up the sandbox for a plugin process. The behaviour
// depends on the runner's sandboxMode (set via WithSandboxMode; production
// always sets it via config; tests that call NewRunner() without WithSandboxMode
// receive sandboxModeBasic as an internal fallback):
//
//   - "bubblewrap" (production default): bwrap namespace isolation.
//     Requires bwrap in $PATH; returns an error if absent.
//   - "strict": bubblewrap namespaces AND LD_PRELOAD seccomp lib.
//   - "basic": LD_PRELOAD seccomp lib + UID drop only (legacy/test fallback).
func (r *Runner) applySandbox(parentCtx context.Context, p *plugin.Plugin, cmd *exec.Cmd) (func(), error) {
	switch mode := r.effectiveMode(); mode {
	case sandboxModeBubblewrap, sandboxModeStrict:
		return r.applyBwrapSandbox(parentCtx, p, cmd, mode)
	case sandboxModeLandlock:
		return r.applyLandlockSandbox(parentCtx, p, cmd)
	default:
		return nil, r.applyBasicSandbox(parentCtx, p, cmd)
	}
}

// effectiveMode returns the concrete sandbox mode to apply. "auto" is resolved
// once (memoised) by probing the host; the empty string keeps the historical
// test fallback to "basic".
func (r *Runner) effectiveMode() string {
	mode := r.sandboxModeValue()
	if mode == "" {
		mode = sandboxModeBasic // internal fallback for tests; prod always calls WithSandboxMode
	}
	if mode != sandboxModeAuto {
		return mode
	}
	r.autoResolveOnce.Do(func() {
		r.autoResolved = resolveAutoMode()
		slog.Info("plugin sandbox auto-detection complete",
			slog.String("selected_mode", r.autoResolved))
	})
	return r.autoResolved
}

// applyBasicSandbox is the legacy LD_PRELOAD + UID-drop fallback, now upgraded
// to wrap with the launcher when it's available: best-effort Landlock plus the
// kernel seccomp denylist (incl. -deny-inet). This matters most on kernels
// without Landlock (e.g. an old NAS) where the LD_PRELOAD shim is the only
// other syscall layer and a static binary would bypass it - the launcher's
// kernel seccomp still applies. If the launcher binary is absent (some dev
// images) it falls back to LD_PRELOAD-only, preserving prior behaviour.
func (r *Runner) applyBasicSandbox(parentCtx context.Context, p *plugin.Plugin, cmd *exec.Cmd) error {
	launcherApplied := false
	if launcher, err := launcherPath(); err == nil {
		pluginDir := p.InstallDir
		if pluginDir == "" {
			pluginDir = defaultPluginDir
		}
		cmd.Path = launcher
		du, dg := r.pluginDropArgs(true)                              // basic = the case uid-sep protects ("auto" enables here)
		cmd.Args = buildLauncherArgv(launcher, p, cmd, false, du, dg) // best-effort Landlock
		cmd.Env = overrideEnv(cmd.Env, map[string]string{"HOME": pluginDir, "TMPDIR": pluginDir})
		launcherApplied = true
	} else {
		// The launcher carries the only kernel-level layer (Landlock + seccomp)
		// that survives execve. Losing it is a real degradation, so surface it
		// instead of swallowing the error - what follows is LD_PRELOAD-only (for
		// non-subprocess plugins) or a fail-closed refusal (subprocess, below).
		slog.WarnContext(parentCtx, "plugin launcher unavailable; basic sandbox degraded",
			slog.String("plugin_id", p.ID),
			slog.Any("err", err))
	}

	// LD_PRELOAD shim: belt-and-suspenders for dynamic plugins, and the only
	// syscall layer if the launcher is absent. It does NOT survive an exec of a
	// child, so it is useless for plugins granted the subprocess permission -
	// those rely on the launcher's kernel seccomp instead. Skipping the shim for
	// a subprocess plugin is therefore only safe when the launcher actually
	// applied; if it didn't, the plugin would run with no confinement at all, so
	// fall through to the same fail-closed guard as a missing sandbox lib.
	if slices.Contains(p.Permissions, subprocessPerm) {
		if launcherApplied {
			return nil
		}
		if !r.allowMissingSandbox {
			slog.ErrorContext(parentCtx, "refusing to start subprocess plugin without launcher confinement",
				slog.String("plugin_id", p.ID))
			r.publishPluginError(parentCtx, p.ID, "plugin launcher unavailable; subprocess plugin refused start")
			return fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		}
		slog.WarnContext(parentCtx, "starting subprocess plugin without sandbox (dev override)",
			slog.String("plugin_id", p.ID))
		return nil
	}
	libPath := r.ensureSandboxLib()
	if libPath != "" {
		// overrideEnv, not append: glibc getenv takes the first match, so a
		// pre-existing LD_PRELOAD would silently keep the shim unloaded.
		cmd.Env = overrideEnv(cmd.Env, map[string]string{"LD_PRELOAD": libPath})
		return nil
	}
	if !r.allowMissingSandbox {
		slog.ErrorContext(parentCtx, "refusing to start plugin without sandbox",
			slog.String("plugin_id", p.ID),
			slog.String("sandbox_lib", sandboxLibPath))
		r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start")
		return fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
	}
	slog.WarnContext(parentCtx, "starting plugin without sandbox (dev override)",
		slog.String("plugin_id", p.ID))
	return nil
}

// applyBwrapSandbox wraps cmd with bubblewrap namespace isolation.
// For "strict" mode it also applies the LD_PRELOAD seccomp lib.
// Mutates cmd.Path and cmd.Args in-place so the exec.Cmd already built in
// Start is reused unchanged (Env, SysProcAttr, Dir, Stdout/Stderr).
func (r *Runner) applyBwrapSandbox(parentCtx context.Context, p *plugin.Plugin, cmd *exec.Cmd, mode string) (func(), error) {
	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		slog.ErrorContext(parentCtx, "bwrap not found in PATH",
			slog.String("plugin_id", p.ID),
			slog.String("sandbox_mode", mode))
		r.publishPluginError(parentCtx, p.ID, "bwrap binary missing; plugin refused start")
		return nil, fmt.Errorf("pluginrunner: sandbox_mode=%q requires bwrap binary in $PATH (apk add bubblewrap)", mode)
	}

	pluginDir := p.InstallDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir
	}
	pluginSockDir := filepath.Join(sockDir, p.ID)
	prefix := buildBwrapCmd(pluginDir, pluginSockDir, mode)

	// cmd.Args[0] is the program name; the original full argv is cmd.Args.
	originalArgv := cmd.Args // e.g. ["/data/plugins/foo/plugin", "--arg"]

	// New argv: bwrap [flags] -- <original argv...>. prefix is freshly built by
	// buildBwrapCmd and not reused, so appending into it is safe.
	prefix = append(prefix, originalArgv...)

	cmd.Path = bwrapPath
	cmd.Args = prefix

	// "strict" = bubblewrap + LD_PRELOAD seccomp lib on top.
	if mode == sandboxModeStrict {
		libPath := r.ensureSandboxLib()
		switch {
		case libPath != "":
			// overrideEnv, not append: glibc getenv takes the first match, so a
			// pre-existing LD_PRELOAD would silently keep the shim unloaded.
			cmd.Env = overrideEnv(cmd.Env, map[string]string{"LD_PRELOAD": libPath})
		case !r.allowMissingSandbox:
			slog.ErrorContext(parentCtx, "refusing to start plugin without seccomp lib (strict mode)",
				slog.String("plugin_id", p.ID),
				slog.String("sandbox_lib", sandboxLibPath))
			r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start (strict)")
			return nil, fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		default:
			slog.WarnContext(parentCtx, "strict mode without seccomp lib (dev override)",
				slog.String("plugin_id", p.ID))
		}
	}

	slog.Info("plugin sandbox applied",
		slog.String("plugin_id", p.ID),
		slog.String("sandbox_mode", mode),
		slog.String("plugin_dir", pluginDir))
	// No per-plugin sandbox temp state to reclaim (the /etc/hosts DNS whitelist
	// was retired when direct plugin egress was cut in favour of the broker);
	// the cleanup contract is kept nil-safe for callers and future use.
	return nil, nil
}

// Deliberately absent: an injection of the launcher's -rlimit-as/-rlimit-nproc
// flags derived from the resource monitor's quota. It shipped on 2026-07-28 and
// took weather-v2 (nodejs) down at every boot in production; both flags are the
// wrong kernel primitive for a per-plugin quota, and the launcher's own flag
// help already said so ("risky for node", "per-UID, leaks across plugins").
//
//   - RLIMIT_NPROC counts every task of the REAL UID, host-wide. Under Landlock
//     the plugin keeps the server's uid (pluginDropArgs(false) returns 0,0
//     unless uid-sep is "on"), so a 64-task cap is measured against the server's
//     own thread count, which is already far past it: the launcher's next
//     clone() returned EAGAIN and it died with "fatal error: newosproc" before
//     ever exec'ing the plugin. No fixed value can work either - the baseline
//     moves with every ffmpeg and library scan.
//   - RLIMIT_AS caps VIRTUAL address space, not RSS. V8 reserves multi-GB
//     regardless of heap use, so 256 MiB is fatal at startup ("Failed to
//     reserve virtual memory for CodeRange"); the Go runtime reserves large
//     arenas too. Any value loose enough for node sits above the host's RAM and
//     would never bind before the RSS sweep does.
//
// The per-plugin quotas live where the semantics are right: checkResources
// polls /proc per process (procRSSBytes / procThreads) and kills on overrun.
// The kernel-hard half of the belt-and-braces is the seccomp process-creation
// denial (-deny-fork, buildLauncherArgv), which caps the fork bomb the sweep's
// ~2s poll window would otherwise miss. See runner_rlimit_test.go.

func pluginSysProcAttr() *syscall.SysProcAttr {
	a := &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	// Drop the plugin to a separate uid (evey-plugin) ONLY when we run as root -
	// because we must also chown the plugin's per-process sock dir + install dir
	// to that uid, which needs CAP_CHOWN, which a non-root server doesn't have.
	// Doing this non-root via file caps therefore requires CAP_SETUID+CAP_SETGID
	// *and* CAP_CHOWN plus chowning those dirs; that's a backlog item. Until then
	// a non-root server keeps the plugin at its own uid (the dir owner) so the
	// plugin can still bind its socket - see the sock-dir chown above, also
	// gated on root.
	if os.Getuid() == 0 {
		a.Credential = &syscall.Credential{Uid: pluginUID, Gid: pluginGID}
	}
	return a
}

// uid-separation modes (EVEY_PLUGINS_UID_SEPARATION).
const (
	uidSepAuto = "auto" // enable only when the effective mode is basic (no Landlock/bwrap)
	uidSepOn   = "on"   // always attempt the drop
	uidSepOff  = "off"  // never
)

// pluginDropArgs returns the uid/gid the launcher should setuid to before exec,
// or 0,0 to leave the plugin at the server's uid. isBasicMode is whether the
// effective sandbox is "basic" (no Landlock/bwrap FS isolation), which is the
// only place uid separation actually protects secrets. It returns a real uid
// only when the server is non-root (a root server drops via
// SysProcAttr.Credential) AND the mode says so: "on" always, "auto" only in
// basic, "off"/unset never. The gid is the server's own group so the plugin can
// write the group-shared socket dir.
func (r *Runner) pluginDropArgs(isBasicMode bool) (uid, gid int) {
	if os.Getuid() == 0 || !uidSepApplies(r.uidSepMode, isBasicMode) {
		return 0, 0
	}
	return pluginUID, os.Getgid()
}

// uidSepApplies decides whether to drop the plugin to a separate uid given the
// configured policy and whether the effective sandbox is basic: "on" always,
// "auto" only in basic (redundant under landlock/bwrap), "off"/unset never.
func uidSepApplies(mode string, isBasicMode bool) bool {
	switch mode {
	case uidSepOn:
		return true
	case uidSepAuto:
		return isBasicMode
	default:
		return false
	}
}

// WithUIDSeparation sets the uid-separation policy ("auto"|"on"|"off"). Unknown
// values map to "off" (safest). "auto" drops the plugin to the evey-plugin uid
// only when the sandbox resolves to "basic"; "on" always; both are best-effort
// (a host without cap_setuid, or using no-new-privileges, runs same-uid with a
// warning rather than failing closed).
func (r *Runner) WithUIDSeparation(mode string) *Runner {
	switch mode {
	case uidSepAuto, uidSepOn, uidSepOff:
		r.uidSepMode = mode
	default:
		r.uidSepMode = uidSepOff
	}
	return r
}

// EffectiveSandboxMode returns the concrete sandbox mode actually in force -
// "auto" resolved to the mode selected for this host (landlock/bubblewrap/basic).
// Exposed so the admin dashboard can show what's really active, not just "auto".
func (r *Runner) EffectiveSandboxMode() string {
	return r.effectiveMode()
}

// SandboxStatus is the host-level sandbox diagnosis surfaced by the admin
// security dashboard: the configured vs effective mode plus the individual
// host probes, so a degradation to "basic" comes with its reason (old kernel
// without Landlock, missing launcher binary, bubblewrap blocked by seccomp…).
type SandboxStatus struct {
	Configured       string
	Effective        string
	LandlockKernel   bool // kernel enforces Landlock
	LauncherPresent  bool // evey-plugin-launcher binary located
	BubblewrapUsable bool // a throwaway bwrap sandbox actually builds here
}

// SandboxStatus reports the host sandbox diagnosis. The probes are host facts
// (kernel LSM, shipped binaries, container seccomp) so they are memoised; the
// bwrap probe in particular spawns a throwaway sandbox and must not run on
// every dashboard refresh.
func (r *Runner) SandboxStatus() SandboxStatus {
	r.sandboxProbeOnce.Do(func() {
		r.probeLandlock = landlockAvailable()
		r.probeLauncher = launcherAvailable()
		r.probeBwrap = ProbeSandbox(sandboxModeBubblewrap) == nil
	})
	configured := r.sandboxModeValue()
	if configured == "" {
		configured = sandboxModeBasic // mirror effectiveMode's test fallback
	}
	return SandboxStatus{
		Configured:       configured,
		Effective:        r.effectiveMode(),
		LandlockKernel:   r.probeLandlock,
		LauncherPresent:  r.probeLauncher,
		BubblewrapUsable: r.probeBwrap,
	}
}

func (r *Runner) ensureSandboxLib() string {
	r.sandboxOnce.Do(func() {
		path := sandboxLibPath
		if r.sandboxLibOverride != "" {
			path = r.sandboxLibOverride
		}
		if _, err := os.Stat(path); err == nil {
			r.sandboxLib = path
			return
		}
		// sync.Once-guarded lazy probe with no fixed caller: threading a ctx
		// here would arbitrarily pin the log to whichever call happened to win
		// the race to run the closure.
		slog.Error("sandbox lib missing - build-time artifact required",
			slog.String("path", path))
	})
	return r.sandboxLib
}
