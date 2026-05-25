package pluginrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
)

const (
	sockDir           = "/tmp/evey/plugins"
	defaultPluginDir  = "/data/plugins"
	shutdownTimeout   = 5 * time.Second
	killTimeout       = 3 * time.Second
	pluginExecTimeout = 30 * time.Minute
	hostVersion       = "0.1.0"
	pluginUID         = 1001 // evey-plugin user
	pluginGID         = 1001
	sandboxLibPath    = "/app/libevey-sandbox.so"
	subprocessPerm    = "subprocess"

	// Sandbox modes for WithSandboxMode.
	sandboxModeBasic      = "basic"      // LD_PRELOAD + UID drop (legacy/weak fallback)
	sandboxModeBubblewrap = "bubblewrap" // bwrap namespace isolation (needs mount; bare-metal only)
	sandboxModeStrict     = "strict"     // bubblewrap + LD_PRELOAD seccomp
	sandboxModeLandlock   = "landlock"   // Landlock LSM self-confinement (no mount; runs under default Docker security)
	sandboxModeAuto       = "auto"       // detect best available at boot: landlock > bubblewrap > basic

	defaultSandboxMode = sandboxModeAuto
)

type processEntry struct {
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	sockPath     string
	pluginID     string
	ipc          *ipcClient
	done         chan struct{} // closed when cmd.Wait() returns in Start goroutine
	installDir   string        // plugin install dir, for the resource monitor's disk check
	hostsCleanup func()        // removes the temp /etc/hosts file; nil if not built
	tail         *tailBuffer   // last bytes of the process stdout/stderr (crash diagnostics)

	crashMu     sync.Mutex
	crashReason string // set by trackLifecycle on abnormal exit, read by connectIPC
}

func (e *processEntry) setCrashReason(s string) {
	e.crashMu.Lock()
	e.crashReason = s
	e.crashMu.Unlock()
}

func (e *processEntry) getCrashReason() string {
	e.crashMu.Lock()
	defer e.crashMu.Unlock()
	return e.crashReason
}

// OnIPCResult is called after IPC Initialize completes (success or failure).
type OnIPCResult func(pluginID string, err error)

// EventBus is the minimal slice the runner needs — matches shared.EventBus
// Publish signature (event must satisfy shared.Event, i.e. have EventName()).
type EventBus interface {
	Publish(ctx context.Context, event shared.Event) error
}

// HostRegistry is the slice the runner needs to bind a plugin to its
// dedicated host gRPC socket. Implemented by pluginhost.HostServer. The
// runner registers the plugin to obtain a per-plugin socket path BEFORE
// cmd.Start (so EVEY_HOST_SOCK is available in the process env), then
// arms PID-based authentication via SetExpectedPID once the OS PID is
// known. Unregister tears the socket and gRPC server down on Stop.
type HostRegistry interface {
	RegisterPlugin(pluginID string) (sockPath string, err error)
	SetExpectedPID(pluginID string, pid int32)
	UnregisterPlugin(pluginID string)
}

type Runner struct {
	mu                  sync.RWMutex
	processes           map[string]*processEntry
	pluginDirs          []string
	logger              *PluginLogger
	sandboxLib          string
	sandboxOnce         sync.Once
	onIPCResult         OnIPCResult
	allowMissingSandbox bool
	uidSepMode          string // "auto"|"on"|"off": drop plugins to evey-plugin uid via launcher cap_setuid
	eventBus            EventBus
	hostRegistry        HostRegistry
	autoResolveOnce     sync.Once // resolves sandboxModeAuto to a concrete mode once
	autoResolved        string
	resMonOnce          sync.Once // starts the resource-monitor sweeper at most once

	// limMu guards the live-editable runtime knobs below. They are read by the
	// resource-monitor sweeper and by Start while the admin may update them at
	// runtime (ServerConfigChanged → SetX), so every access goes through the
	// lock. Resource caps apply on the next sweep; nice and sandbox mode apply
	// to plugins started after the change.
	limMu        sync.RWMutex
	sandboxMode  string
	resLimits    resourceLimits
	cpuNice      int   // scheduling nice applied to plugins (0 = leave default)
	diskMaxBytes int64 // per-plugin disk quota; 0 = disabled
}

// ErrSandboxUnavailable is returned by Start when a plugin without the
// 'subprocess' permission is about to run but the LD_PRELOAD sandbox
// library cannot be located or compiled — the previous behaviour
// silently started the plugin without sandbox. Fail-closed is the
// expected behaviour when third-party plugins are allowed; tests and
// dev environments can opt out via WithAllowMissingSandbox.
var ErrSandboxUnavailable = errors.New("plugin sandbox library unavailable")

func NewRunner(pluginDirs ...string) *Runner {
	dirs := pluginDirs
	if len(dirs) == 0 {
		dirs = []string{defaultPluginDir}
	}
	return &Runner{
		processes:  make(map[string]*processEntry),
		pluginDirs: dirs,
	}
}

func (r *Runner) WithLogger(l *PluginLogger) *Runner {
	r.logger = l
	return r
}

func (r *Runner) WithIPCCallback(cb OnIPCResult) *Runner {
	r.onIPCResult = cb
	return r
}

// WithAllowMissingSandbox lets tests and local dev environments skip the
// strict "sandbox-or-refuse" check when libevey-sandbox.so is absent.
//
// CONTRACT:
//   - Production MUST pass allow=false (fail-closed: plugin start is refused
//     when the sandbox lib is missing).
//   - Pass allow=true ONLY when cfg.Log.DevMode is true (developer laptop) or
//     inside test binaries.  Passing true in a production environment is a
//     security misconfiguration — callers in wire.go must emit a visible
//     warning so ops can catch it (see cmd/evey/wire.go).
func (r *Runner) WithAllowMissingSandbox(allow bool) *Runner {
	r.allowMissingSandbox = allow
	return r
}

// WithEventBus lets the runner emit plugin.error events on sandbox refusal
// and process crash so the notification service can page the admin.
func (r *Runner) WithEventBus(bus EventBus) *Runner {
	r.eventBus = bus
	return r
}

// WithHostRegistry wires the per-plugin host socket factory. When set,
// the runner provisions a dedicated host gRPC socket for each plugin and
// pins authentication to the plugin process PID. When nil (tests), the
// plugin starts without a host socket bound to it — the EVEY_HOST_SOCK
// env var is left unset.
func (r *Runner) WithHostRegistry(reg HostRegistry) *Runner {
	r.hostRegistry = reg
	return r
}

// WithSandboxMode sets the isolation strategy used when launching plugins.
// Valid values: "auto" (default), "landlock", "bubblewrap", "strict", "basic".
//   - "auto"       — detect the best available at boot: landlock > bubblewrap
//     (only where Landlock is absent but bwrap runs under the host security) >
//     basic. Recommended for the official image: prefers Landlock (no privilege,
//     runs under Docker's default security), falls back as the host allows.
//   - "landlock"   — Landlock LSM self-confinement (filesystem allowlist). Needs
//     no mount/unshare, so it runs under Docker's default seccomp+AppArmor. Needs
//     kernel ≥5.13 with Landlock in the active LSM list. Refuses to start a
//     plugin if Landlock cannot be enforced (fail closed).
//   - "bubblewrap" — bwrap namespace isolation (user/pid/uts/ipc/net). Requires
//     the bwrap binary AND the mount syscall, so inside Docker it only works
//     with relaxed security; effectively bare-metal only.
//   - "strict"     — bubblewrap namespaces AND LD_PRELOAD seccomp lib.
//   - "basic"      — LD_PRELOAD seccomp lib + UID drop only (no per-path FS
//     isolation; last-resort fallback for locked-down old kernels).
//
// Any unrecognised value (including empty string) is silently mapped to
// defaultSandboxMode ("auto") — callers should validate upstream.
func (r *Runner) WithSandboxMode(mode string) *Runner {
	r.SetSandboxMode(mode)
	return r
}

// WithResourceLimits sets the per-plugin memory and PID caps the resource
// monitor enforces (it polls /proc and kills a plugin that exceeds them —
// cgroups are unusable in a non-privileged Docker container). Pass 0 for either
// value to use the package default (256 MiB / 64 PIDs).
func (r *Runner) WithResourceLimits(memBytes int64, pidsMax int) *Runner {
	r.SetResourceLimits(memBytes, pidsMax)
	return r
}

// WithCPUNice sets the scheduling nice value applied to each plugin at spawn so
// a plugin cannot starve the server's CPU under contention. Pass 0 to leave the
// default priority.
func (r *Runner) WithCPUNice(nice int) *Runner {
	r.SetCPUNice(nice)
	return r
}

// WithDiskQuota caps each plugin's install-directory size; the resource monitor
// measures the directory periodically and stops a plugin that grows past
// maxBytes. Pass 0 to disable.
func (r *Runner) WithDiskQuota(maxBytes int64) *Runner {
	r.SetDiskQuota(maxBytes)
	return r
}

// SetSandboxMode updates the isolation mode at runtime. Any unrecognised value
// (including empty) maps to the default ("auto"). Applies to plugins started
// after the change; already-running plugins keep their current sandbox.
func (r *Runner) SetSandboxMode(mode string) {
	switch mode {
	case sandboxModeBasic, sandboxModeBubblewrap, sandboxModeStrict, sandboxModeLandlock, sandboxModeAuto:
	default:
		mode = defaultSandboxMode
	}
	r.limMu.Lock()
	r.sandboxMode = mode
	r.limMu.Unlock()
}

// SetResourceLimits updates the memory/PID caps at runtime; the next sweep
// enforces them. Pass 0 for either to use the package default.
func (r *Runner) SetResourceLimits(memBytes int64, pidsMax int) {
	r.limMu.Lock()
	r.resLimits = resourceLimits{memBytes: memBytes, pidsMax: pidsMax}
	r.limMu.Unlock()
}

// SetCPUNice updates the scheduling nice applied to plugins at spawn. Applies to
// plugins started after the change.
func (r *Runner) SetCPUNice(nice int) {
	r.limMu.Lock()
	r.cpuNice = nice
	r.limMu.Unlock()
}

// SetDiskQuota updates the per-plugin install-directory cap at runtime; the next
// disk sweep enforces it. Pass 0 to disable.
func (r *Runner) SetDiskQuota(maxBytes int64) {
	r.limMu.Lock()
	r.diskMaxBytes = maxBytes
	r.limMu.Unlock()
}

// limitsSnapshot returns the live resource caps + nice + disk quota under the
// lock, for the sweeper and Start to read consistently.
func (r *Runner) limitsSnapshot() (lim resourceLimits, nice int, diskMax int64) {
	r.limMu.RLock()
	defer r.limMu.RUnlock()
	return r.resLimits, r.cpuNice, r.diskMaxBytes
}

func (r *Runner) sandboxModeValue() string {
	r.limMu.RLock()
	defer r.limMu.RUnlock()
	return r.sandboxMode
}

// ResourceConfig returns the current effective per-plugin caps for display in
// the admin UI: memory (MiB), max PIDs, disk (MiB), nice, and sandbox mode.
// Zero memory/PIDs are reported as the package defaults the monitor applies.
func (r *Runner) ResourceConfig() (memMB int, pidsMax int, diskMB int, nice int, sandbox string) {
	lim, n, disk := r.limitsSnapshot()
	lim = lim.defaults()
	return int(lim.memBytes >> 20), lim.pidsMax, int(disk >> 20), n, r.sandboxModeValue()
}

// publishPluginError emits the domain plugin.error event so subscribers
// (notification service) can react. Best-effort; logs and moves on.
func (r *Runner) publishPluginError(ctx context.Context, pluginID, msg string) {
	if r.eventBus == nil {
		return
	}
	if err := r.eventBus.Publish(ctx, plugin.PluginError{
		PluginID:     pluginID,
		ErrorMessage: msg,
		OccurredAt:   time.Now(),
	}); err != nil {
		slog.Warn("publish plugin.error failed", slog.String("plugin", pluginID), slog.Any("err", err))
	}
}

var knownInterpreters = map[string]bool{
	"python3": true, "python": true, "node": true, "php": true,
	"ruby": true, "perl": true, "lua": true, "bash": true, "sh": true,
}

func isInterpreter(cmd string) bool {
	return knownInterpreters[filepath.Base(cmd)]
}

func (r *Runner) validateEntrypoint(entrypoint, installDir string) (string, error) {
	if strings.Contains(entrypoint, "..") {
		return "", errors.New("entrypoint must not contain '..'")
	}

	candidate := entrypoint
	if !filepath.IsAbs(candidate) && installDir != "" {
		candidate = filepath.Join(installDir, candidate)
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve entrypoint path: %w", err)
	}
	resolved = filepath.Clean(resolved)

	allowed := false
	for _, dir := range r.pluginDirs {
		cleanDir := filepath.Clean(dir)
		if strings.HasPrefix(resolved, cleanDir+string(filepath.Separator)) || resolved == cleanDir {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("entrypoint %q outside allowed plugin directories", entrypoint)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat entrypoint: %w", err)
	}
	if info.IsDir() {
		return "", errors.New("entrypoint is a directory")
	}
	if info.Mode()&0o111 == 0 {
		return "", errors.New("entrypoint is not executable")
	}

	return resolved, nil
}

func (r *Runner) Start(parentCtx context.Context, p *plugin.Plugin) error {
	pluginSockDir, sock, err := r.setupSocketDir(p.ID)
	if err != nil {
		return err
	}

	cmdName, cmdArgs, err := r.resolveCommand(p)
	if err != nil {
		return err
	}

	// Detach from the caller's request context: the plugin must outlive the
	// HTTP/gRPC request that triggered Start. Without WithoutCancel, the
	// request ctx is canceled as soon as the install handler returns, which
	// in turn cancels the cmd ctx — making connectIPC's Initialize RPC fail
	// with "code = Canceled" 200ms after the process spawns.
	// We keep parentCtx's deadline (if any) so a true timeout still aborts.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), pluginExecTimeout)
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	if p.InstallDir != "" {
		cmd.Dir = p.InstallDir
	}

	hostSock, err := r.registerHostSock(p.ID)
	if err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, false)
		return err
	}

	cmd.Env = buildPluginEnv(p.ID, sock, hostSock)
	hostsCleanup, err := r.applySandbox(parentCtx, p, cmd)
	if err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, true)
		return err
	}
	// hostsCleanup must run if Start fails between applySandbox and the point
	// where the cleanup is handed off to the lifecycle goroutines below.
	hostsCleanedUp := false
	defer func() {
		if !hostsCleanedUp && hostsCleanup != nil {
			hostsCleanup()
		}
	}()

	// Tee stdout/stderr through a bounded tail buffer so an abnormal exit can
	// surface the real cause. Use ONE writer instance for both streams: os/exec
	// then serialises writes (no interleave race) when Stdout == Stderr.
	tail := newTailBuffer(4096)
	var out io.Writer
	if r.logger != nil {
		out = io.MultiWriter(r.logger.Writer(p.ID), tail)
	} else {
		out = io.MultiWriter(os.Stdout, tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	sysProcAttr := pluginSysProcAttr()
	cmd.SysProcAttr = sysProcAttr

	// Resource limits without cgroups (unusable in a non-privileged Docker
	// container): a background sweeper polls /proc and enforces per-plugin
	// memory/PID/disk caps; the low nice below prevents CPU starvation.
	r.startResourceMonitor()

	if err := cmd.Start(); err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, true)
		return fmt.Errorf("start plugin %s: %w", p.ID, err)
	}

	// Lower the plugin's scheduling priority so it cannot starve the server of
	// CPU under contention (preventive; the monitor only flags sustained pegging).
	r.applyNice(cmd.Process.Pid)

	// Arm PID-based authentication on the host socket. From this point
	// on, only RPCs originating from cmd.Process.Pid will be accepted —
	// SO_PEERCRED is filled in by the kernel and cannot be spoofed.
	if r.hostRegistry != nil {
		r.hostRegistry.SetExpectedPID(p.ID, int32(cmd.Process.Pid))
	}

	if r.logger != nil {
		r.logger.WriteLog(p.ID, fmt.Sprintf("[INFO] plugin process started pid=%d", cmd.Process.Pid))
	}

	entry := &processEntry{
		cmd:          cmd,
		cancel:       cancel,
		sockPath:     sock,
		pluginID:     p.ID,
		done:         make(chan struct{}),
		installDir:   p.InstallDir,
		hostsCleanup: hostsCleanup,
		tail:         tail,
	}
	hostsCleanedUp = true // ownership handed to entry; trackLifecycle/Stop will call it
	go r.connectIPC(ctx, entry, p)

	r.mu.Lock()
	r.processes[p.ID] = entry
	r.mu.Unlock()

	go r.trackLifecycle(entry, cancel, sock, p)
	return nil
}

// setupSocketDir provisions the per-plugin socket subdirectory and returns
// the dir + target sock path. Also clears a stale sock file left behind
// from a prior unclean exit.
func (r *Runner) setupSocketDir(pluginID string) (string, string, error) {
	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create sock dir: %w", err)
	}
	pluginSockDir := filepath.Join(sockDir, pluginID)
	if err := os.MkdirAll(pluginSockDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create plugin sock dir: %w", err)
	}
	if os.Getuid() == 0 {
		// Root drops the plugin to evey-plugin (1001) via SysProcAttr.Credential.
		// Own the per-plugin dir to that identity, and make the PARENT traversable
		// by the plugin group — otherwise the dropped process can't even search
		// the 0700 parent to reach its own socket ("listen EACCES").
		if err := os.Chown(pluginSockDir, pluginUID, pluginGID); err != nil {
			return "", "", fmt.Errorf("chown plugin sock dir: %w", err)
		}
		if err := r.makeSockParentTraversable(pluginGID); err != nil {
			return "", "", err
		}
	} else if uidSepApplies(r.uidSepMode, r.effectiveMode() == sandboxModeBasic) {
		// Non-root + uid separation will drop the plugin to evey-plugin (1001):
		// the server (the dir owner) can't chown to that uid (no CAP_CHOWN), so
		// make the dir group-writable + setgid instead. The plugin runs in the
		// server's group, so it can create plugin.sock here, and the setgid bit
		// keeps created sockets in that shared group.
		if err := os.Chmod(pluginSockDir, os.FileMode(0o770)|os.ModeSetgid); err != nil {
			return "", "", fmt.Errorf("chmod plugin sock dir for uid separation: %w", err)
		}
		// The per-plugin dir being group-writable is useless if the dropped uid
		// can't traverse the 0700 parent to reach it — that was a real prod crash
		// ("listen EACCES /tmp/evey/plugins/<id>/plugin.sock"). Grant the group
		// SEARCH (0710: traverse only, no listing → can't enumerate other plugins)
		// on the parent. Holds only sockets, no secrets — the server's keys live
		// under /data (0600 owned by the server) and stay unreadable to uid 1001.
		if err := r.makeSockParentTraversable(-1); err != nil {
			return "", "", err
		}
	}
	sock := filepath.Join(pluginSockDir, "plugin.sock")
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove old plugin sock", slog.Any("err", err))
	}
	return pluginSockDir, sock, nil
}

// makeSockParentTraversable grants the plugin group SEARCH on the shared sock
// parent dir so a uid-dropped plugin can traverse it to reach its own per-plugin
// socket dir. 0710 = owner rwx, group --x (traverse only, no listing → a plugin
// can't enumerate other plugins' dirs), other none. groupGID >= 0 also re-owns
// the parent's group (the root path, where we hold CAP_CHOWN); pass -1 to leave
// the group as-is (non-root: the parent already carries the server's shared
// group). Only plugin sockets live here, never secrets.
func (r *Runner) makeSockParentTraversable(groupGID int) error {
	if groupGID >= 0 {
		if err := os.Chown(sockDir, 0, groupGID); err != nil {
			return fmt.Errorf("chown sock parent dir: %w", err)
		}
	}
	if err := os.Chmod(sockDir, 0o710); err != nil {
		return fmt.Errorf("chmod sock parent dir for uid separation: %w", err)
	}
	return nil
}

// resolveCommand parses p.Entrypoint (falling back to p.Name) and returns
// the fully resolved command name and args. Interpreters (python, node…)
// are looked up in PATH; binaries are path-confined to r.pluginDirs.
func (r *Runner) resolveCommand(p *plugin.Plugin) (string, []string, error) {
	entrypoint := p.Entrypoint
	if entrypoint == "" {
		entrypoint = p.Name
	}
	parts := strings.Fields(entrypoint)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("plugin %s: empty entrypoint", p.ID)
	}

	if isInterpreter(parts[0]) {
		if _, lookErr := exec.LookPath(parts[0]); lookErr != nil {
			return "", nil, fmt.Errorf("plugin %s: runtime %q not installed", p.ID, parts[0])
		}
		var cmdArgs []string
		if len(parts) > 1 {
			scriptPath := parts[1]
			if !filepath.IsAbs(scriptPath) && p.InstallDir != "" {
				scriptPath = filepath.Join(p.InstallDir, scriptPath)
			}
			if _, err := os.Stat(scriptPath); err != nil {
				return "", nil, fmt.Errorf("plugin %s: script not found: %w", p.ID, err)
			}
			cmdArgs = append([]string{scriptPath}, parts[2:]...)
		}
		return parts[0], cmdArgs, nil
	}

	resolved, resolveErr := r.validateEntrypoint(parts[0], p.InstallDir)
	if resolveErr != nil {
		return "", nil, fmt.Errorf("plugin %s: %w", p.ID, resolveErr)
	}
	return resolved, parts[1:], nil
}

// registerHostSock provisions the per-plugin host gRPC socket before the
// process starts so EVEY_HOST_SOCK is populated. Returns the empty string
// when no HostRegistry is wired (tests).
func (r *Runner) registerHostSock(pluginID string) (string, error) {
	if r.hostRegistry == nil {
		return "", nil
	}
	path, regErr := r.hostRegistry.RegisterPlugin(pluginID)
	if regErr != nil {
		return "", fmt.Errorf("register plugin host socket: %w", regErr)
	}
	return path, nil
}

// rollbackStart undoes partial Start side-effects on failure paths: cancels
// the ctx, removes the socket dir, and (when the host socket was already
// registered) unregisters it. Keep the three ops in lock-step — drifting
// leaks a socket dir or a dangling gRPC host entry.
func (r *Runner) rollbackStart(cancel context.CancelFunc, pluginSockDir, pluginID string, hostRegistered bool) {
	cancel()
	if err := os.RemoveAll(pluginSockDir); err != nil {
		slog.Warn("rollback remove plugin sock dir", slog.Any("err", err))
	}
	if hostRegistered && r.hostRegistry != nil {
		r.hostRegistry.UnregisterPlugin(pluginID)
	}
}

func buildPluginEnv(pluginID, sock, hostSock string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"NODE_PATH=/usr/local/lib/node_modules",
		"EVEY_PLUGIN_ID=" + pluginID,
		"EVEY_PLUGIN_SOCK=" + sock,
		"EVEY_HOST_SOCK=" + hostSock,
	}
}

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
// other syscall layer and a static binary would bypass it — the launcher's
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
		// instead of swallowing the error — what follows is LD_PRELOAD-only (for
		// non-subprocess plugins) or a fail-closed refusal (subprocess, below).
		slog.Warn("plugin launcher unavailable; basic sandbox degraded",
			slog.String("plugin_id", p.ID),
			slog.Any("err", err))
	}

	// LD_PRELOAD shim: belt-and-suspenders for dynamic plugins, and the only
	// syscall layer if the launcher is absent. It does NOT survive an exec of a
	// child, so it is useless for plugins granted the subprocess permission —
	// those rely on the launcher's kernel seccomp instead. Skipping the shim for
	// a subprocess plugin is therefore only safe when the launcher actually
	// applied; if it didn't, the plugin would run with no confinement at all, so
	// fall through to the same fail-closed guard as a missing sandbox lib.
	if slices.Contains(p.Permissions, subprocessPerm) {
		if launcherApplied {
			return nil
		}
		if !r.allowMissingSandbox {
			slog.Error("refusing to start subprocess plugin without launcher confinement",
				slog.String("plugin_id", p.ID))
			r.publishPluginError(parentCtx, p.ID, "plugin launcher unavailable; subprocess plugin refused start")
			return fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		}
		slog.Warn("starting subprocess plugin without sandbox (dev override)",
			slog.String("plugin_id", p.ID))
		return nil
	}
	libPath := r.ensureSandboxLib()
	if libPath != "" {
		cmd.Env = append(cmd.Env, "LD_PRELOAD="+libPath)
		return nil
	}
	if !r.allowMissingSandbox {
		slog.Error("refusing to start plugin without sandbox",
			slog.String("plugin_id", p.ID),
			slog.String("sandbox_lib", sandboxLibPath))
		r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start")
		return fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
	}
	slog.Warn("starting plugin without sandbox (dev override)",
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
		slog.Error("bwrap not found in PATH",
			slog.String("plugin_id", p.ID),
			slog.String("sandbox_mode", mode))
		r.publishPluginError(parentCtx, p.ID, "bwrap binary missing; plugin refused start")
		return nil, fmt.Errorf("pluginrunner: sandbox_mode=%q requires bwrap binary in $PATH (apk add bubblewrap)", mode)
	}

	pluginDir := p.InstallDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir
	}
	hasNet := slices.Contains(p.Permissions, networkPerm)

	hostsFile, hostsCleanup, hostsErr := buildHostsFile(p.ID, p.ExternalFetch)
	if hostsErr != nil {
		slog.Warn("build /etc/hosts file failed; running without DNS whitelist",
			slog.String("plugin_id", p.ID), slog.Any("err", hostsErr))
	}

	pluginSockDir := filepath.Join(sockDir, p.ID)
	prefix := buildBwrapCmd(pluginDir, pluginSockDir, hasNet, mode, hostsFile)

	// cmd.Args[0] is the program name; the original full argv is cmd.Args.
	originalArgv := cmd.Args // e.g. ["/data/plugins/foo/plugin", "--arg"]

	// New argv: bwrap [flags] -- <original argv...>
	newArgs := append(prefix, originalArgv...)

	cmd.Path = bwrapPath
	cmd.Args = newArgs

	// "strict" = bubblewrap + LD_PRELOAD seccomp lib on top.
	if mode == sandboxModeStrict {
		libPath := r.ensureSandboxLib()
		if libPath != "" {
			cmd.Env = append(cmd.Env, "LD_PRELOAD="+libPath)
		} else if !r.allowMissingSandbox {
			slog.Error("refusing to start plugin without seccomp lib (strict mode)",
				slog.String("plugin_id", p.ID),
				slog.String("sandbox_lib", sandboxLibPath))
			r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start (strict)")
			if hostsCleanup != nil {
				hostsCleanup()
			}
			return nil, fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
		} else {
			slog.Warn("strict mode without seccomp lib (dev override)",
				slog.String("plugin_id", p.ID))
		}
	}

	slog.Info("plugin sandbox applied",
		slog.String("plugin_id", p.ID),
		slog.String("sandbox_mode", mode),
		slog.String("plugin_dir", pluginDir),
		slog.Bool("has_network_perm", hasNet))
	return hostsCleanup, nil
}

func pluginSysProcAttr() *syscall.SysProcAttr {
	a := &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	// Drop the plugin to a separate uid (evey-plugin) ONLY when we run as root —
	// because we must also chown the plugin's per-process sock dir + install dir
	// to that uid, which needs CAP_CHOWN, which a non-root server doesn't have.
	// Doing this non-root via file caps therefore requires CAP_SETUID+CAP_SETGID
	// *and* CAP_CHOWN plus chowning those dirs; that's a backlog item. Until then
	// a non-root server keeps the plugin at its own uid (the dir owner) so the
	// plugin can still bind its socket — see the sock-dir chown above, also
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

// EffectiveSandboxMode returns the concrete sandbox mode actually in force —
// "auto" resolved to the mode selected for this host (landlock/bubblewrap/basic).
// Exposed so the admin dashboard can show what's really active, not just "auto".
func (r *Runner) EffectiveSandboxMode() string {
	return r.effectiveMode()
}

// trackLifecycle runs cmd.Wait in a goroutine and cleans up once the
// process exits. Uses a detached context for publishPluginError because
// the parent request scope may already be cancelled when the crash fires.
func (r *Runner) trackLifecycle(entry *processEntry, cancel context.CancelFunc, sock string, p *plugin.Plugin) {
	defer close(entry.done)
	defer func() {
		if rv := recover(); rv != nil {
			slog.Error("panic in plugin runner", slog.Any("recover", rv), slog.String("plugin", p.ID))
		}
	}()
	err := entry.cmd.Wait()
	r.mu.Lock()
	_, stillTracked := r.processes[p.ID]
	if stillTracked {
		delete(r.processes, p.ID)
	}
	r.mu.Unlock()
	if !stillTracked {
		return // Stop() already handled cleanup
	}

	cancel()
	if err2 := os.Remove(sock); err2 != nil {
		slog.Warn("remove plugin sock on crash", slog.Any("err", err2))
	}
	if entry.hostsCleanup != nil {
		entry.hostsCleanup()
	}
	if r.hostRegistry != nil {
		r.hostRegistry.UnregisterPlugin(p.ID)
	}
	if err != nil {
		msg := classifyExitError(err)
		if entry.tail != nil {
			if out := strings.TrimSpace(entry.tail.String()); out != "" {
				const maxTail = 600
				if len(out) > maxTail {
					out = "…" + out[len(out)-maxTail:]
				}
				msg += " — plugin output: " + out
			}
		}
		// Record before publishing so connectIPC (still waiting on the socket)
		// can surface this exact reason instead of a generic socket timeout.
		entry.setCrashReason(msg)
		slog.Warn("plugin process crashed", slog.String("plugin", p.ID), slog.String("classification", msg), slog.Any("err", err))
		r.publishPluginError(context.Background(), p.ID, msg)
	} else {
		slog.Info("plugin process exited", slog.String("plugin", p.ID))
	}
}

// classifyExitError turns the raw exec.Cmd.Wait error into an admin-readable
// reason. A SIGKILL is usually either the resource monitor stopping a plugin
// that breached its memory/PID cap, or the host/container OOM-killer under
// memory pressure — surface an actionable hint instead of "signal: killed".
func classifyExitError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		if ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL {
			return "process SIGKILLed — exceeded its memory cap (EVEY_PLUGINS_MEMORY_MB), or the host/container OOM-killer fired under memory pressure"
		}
	}
	return "process crashed: " + err.Error()
}

func (r *Runner) connectIPC(parentCtx context.Context, entry *processEntry, p *plugin.Plugin) {
	defer func() {
		if rv := recover(); rv != nil {
			slog.Error("panic in connectIPC", slog.Any("recover", rv), slog.String("plugin", p.ID))
		}
	}()
	client, err := newIPCClient(entry.sockPath)
	if err != nil {
		// The socket never came up. The process has very likely already
		// crashed — its exit reason (with captured output) is far more useful
		// than a bare "socket timeout". Wait briefly for trackLifecycle to
		// record it, then prefer it.
		select {
		case <-entry.done:
		case <-time.After(500 * time.Millisecond):
		}
		if cr := entry.getCrashReason(); cr != "" {
			err = fmt.Errorf("plugin exited before becoming ready: %s", cr)
		}
		slog.Warn("plugin IPC connect failed", slog.String("pluginID", p.ID), slog.Any("err", err))
		if r.onIPCResult != nil {
			r.onIPCResult(p.ID, err)
		}
		return
	}

	r.mu.Lock()
	entry.ipc = client
	r.mu.Unlock()

	cfgMap := make(map[string]string)
	for k, v := range p.Config {
		cfgMap[k] = fmt.Sprintf("%v", v)
	}

	initCtx, cancel := context.WithTimeout(parentCtx, ipcCallTimeout)
	defer cancel()
	if initErr := client.Initialize(initCtx, p.ID, cfgMap, hostVersion); initErr != nil {
		slog.Warn("plugin Initialize failed", slog.String("pluginID", p.ID), slog.Any("err", initErr))
		if r.onIPCResult != nil {
			r.onIPCResult(p.ID, initErr)
		}
		return
	}
	if r.onIPCResult != nil {
		r.onIPCResult(p.ID, nil)
	}
}

func (r *Runner) Stop(ctx context.Context, pluginID string) error {
	// Always clear the host-socket registry on the way out — including the early
	// returns below — so a desynced orphan (gone from r.processes but still
	// registered, e.g. after a failed restart) can't lock out a later re-enable
	// with "plugin already registered".
	if r.hostRegistry != nil {
		defer r.hostRegistry.UnregisterPlugin(pluginID)
	}
	r.mu.Lock()
	entry, ok := r.processes[pluginID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("plugin %s not running", pluginID)
	}
	delete(r.processes, pluginID)
	ipc := entry.ipc // capture under lock to avoid race with Start goroutine
	r.mu.Unlock()

	defer entry.cancel()

	// Try gRPC Shutdown first
	if ipc != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, ipcCallTimeout)
		if err := ipc.Shutdown(shutdownCtx, "host requested shutdown"); err != nil {
			slog.Warn("plugin Shutdown RPC failed", slog.String("pluginID", pluginID), slog.Any("err", err))
		}
		shutdownCancel()
		if err := ipc.Close(); err != nil {
			slog.Warn("close plugin IPC", slog.Any("err", err))
		}
	}

	if entry.cmd.Process == nil {
		os.RemoveAll(filepath.Dir(entry.sockPath))
		return nil
	}

	if err := entry.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		slog.Warn("send SIGTERM to plugin", slog.Any("err", err))
	}

	select {
	case <-entry.done:
	case <-time.After(shutdownTimeout):
		if err := entry.cmd.Process.Signal(syscall.SIGKILL); err != nil {
			slog.Warn("send SIGKILL to plugin", slog.Any("err", err))
		}
		select {
		case <-entry.done:
		case <-time.After(killTimeout):
		}
	}

	// Remove entire socket subdirectory
	if err := os.RemoveAll(filepath.Dir(entry.sockPath)); err != nil {
		slog.Warn("remove plugin sock dir", slog.Any("err", err))
	}

	if entry.hostsCleanup != nil {
		entry.hostsCleanup()
	}
	// Host-socket unregistration is handled by the deferred call at the top so
	// every exit path (incl. the early returns) cleans the registry.

	if r.logger != nil {
		r.logger.WriteLog(pluginID, "[INFO] plugin process stopped")
		r.logger.ClosePlugin(pluginID)
	}
	return nil
}

// Kill sends SIGKILL to the process group of the plugin, bypassing gRPC
// shutdown. Used when Stop returns an error or the process refuses SIGTERM.
// The process entry is removed from the tracking map so trackLifecycle does
// not try to handle an exit that we already own. Returns nil when the plugin
// is not tracked (already dead or never started).
func (r *Runner) Kill(pluginID string) error {
	// Always clear the host-socket registry on the way out (see Stop) so a
	// killed/untracked plugin can be re-enabled without "already registered".
	if r.hostRegistry != nil {
		defer r.hostRegistry.UnregisterPlugin(pluginID)
	}
	r.mu.Lock()
	entry, ok := r.processes[pluginID]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.processes, pluginID)
	r.mu.Unlock()

	defer entry.cancel()

	if entry.cmd.Process == nil {
		return nil
	}

	// SIGKILL the entire process group — the plugin may have spawned children
	// (interpreters, worker threads) that need to die along with the leader.
	// Setpgid:true in SysProcAttr guarantees pgid == pid for the leader.
	pgid := -entry.cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		slog.Warn("kill plugin process group", slog.String("pluginID", pluginID), slog.Any("err", err))
	}

	// Wait briefly for the OS to recycle FDs so the install dir can be removed.
	select {
	case <-entry.done:
	case <-time.After(2 * time.Second):
	}

	if err := os.RemoveAll(filepath.Dir(entry.sockPath)); err != nil {
		slog.Warn("remove plugin sock dir after kill", slog.Any("err", err))
	}
	if entry.hostsCleanup != nil {
		entry.hostsCleanup()
	}
	// Host-socket unregistration handled by the deferred call at the top.
	if r.logger != nil {
		r.logger.WriteLog(pluginID, "[INFO] plugin process killed (SIGKILL)")
		r.logger.ClosePlugin(pluginID)
	}
	return nil
}

func (r *Runner) Health(ctx context.Context, pluginID string) (bool, error) {
	r.mu.RLock()
	entry, ok := r.processes[pluginID]
	r.mu.RUnlock()

	if !ok {
		return false, nil
	}

	if entry.cmd.ProcessState != nil && entry.cmd.ProcessState.Exited() {
		return false, nil
	}

	// If IPC client available, do a real health check
	r.mu.RLock()
	ipc := entry.ipc
	r.mu.RUnlock()
	if ipc != nil {
		hcCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
		defer cancel()
		healthy, err := ipc.HealthCheck(hcCtx)
		if err != nil {
			return true, nil // process running but gRPC unavailable, still "alive"
		}
		return healthy, nil
	}

	return true, nil
}

func (r *Runner) SendEvent(ctx context.Context, pluginID string, event any) error {
	r.mu.RLock()
	entry, ok := r.processes[pluginID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin %s not running", pluginID)
	}

	r.mu.RLock()
	ipc := entry.ipc
	r.mu.RUnlock()
	if ipc == nil {
		return fmt.Errorf("plugin %s IPC not connected", pluginID)
	}

	payload := make(map[string]any)
	if m, ok := event.(map[string]any); ok {
		payload = m
	}

	eventType := "unknown"
	if named, ok := event.(interface{ EventName() string }); ok {
		eventType = named.EventName()
	}

	evtCtx, cancel := context.WithTimeout(ctx, ipcCallTimeout)
	defer cancel()
	return ipc.OnEvent(evtCtx, eventType, payload, "host")
}

func (r *Runner) ensureSandboxLib() string {
	r.sandboxOnce.Do(func() {
		if _, err := os.Stat(sandboxLibPath); err == nil {
			r.sandboxLib = sandboxLibPath
			return
		}
		slog.Error("sandbox lib missing — build-time artifact required",
			slog.String("path", sandboxLibPath))
	})
	return r.sandboxLib
}
