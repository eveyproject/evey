package pluginrunner

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

const (
	sockDir          = "/tmp/evey/plugins"
	defaultPluginDir = "/data/plugins"
	shutdownTimeout  = 5 * time.Second
	killTimeout      = 3 * time.Second
	hostVersion      = "0.1.0"
	pluginUID        = 1001 // evey-plugin user
	pluginGID        = 1001
	sandboxLibPath   = "/app/libevey-sandbox.so"
	subprocessPerm   = "subprocess"
	gpuPerm          = "hardware.gpu" // grants the DRM render node (highest-risk; admin-only, first-party tier)

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
	payloadPID   int // runs the plugin code: cmd.Process.Pid, or bwrap's sandboxed child
	cancel       context.CancelFunc
	sockPath     string
	pluginID     string
	ipc          *ipcClient
	done         chan struct{} // closed when cmd.Wait() returns in Start goroutine
	installDir   string        // plugin install dir, for the resource monitor's disk check
	hostsCleanup func()        // removes the temp /etc/hosts file; nil if not built
	tail         *tailBuffer   // last bytes of the process stdout/stderr (crash diagnostics)
	hasGPU       bool          // plugin holds hardware.gpu → the resmon also watches DRM engine pegging

	crashMu     sync.Mutex
	crashReason string // set by trackLifecycle on abnormal exit, read by connectIPC
}

// pid is the process running the plugin code, for the per-process /proc
// reads; process-group signals go to cmd.Process.Pid (the pgid leader).
func (e *processEntry) pid() int {
	if e.payloadPID > 0 {
		return e.payloadPID
	}
	return e.cmd.Process.Pid
}

// bwrapTermGrace is how long Stop waits after the SIGTERM before the SIGKILL
// for a plugin under bubblewrap. The SIGTERM was never the plugin's graceful
// exit there (the Shutdown RPC is): it kills bwrap, whose --die-with-parent
// then SIGKILLs the plugin within milliseconds. What it cannot reach is a
// plugin stopped while bwrap is still setting it up: as PID 1 of its
// namespace (--as-pid-1) that child ignores a SIGTERM it has no handler for,
// and its parent-death signal is not armed yet, so it outlived bwrap until the
// full shutdownTimeout escalation (measured 2026-10-23, 6 s and counting).
const bwrapTermGrace = time.Second

// termGrace is the wait between Stop's SIGTERM and its SIGKILL.
func (e *processEntry) termGrace() time.Duration {
	if e.cmd != nil && e.cmd.Process != nil && e.payloadPID > 0 && e.payloadPID != e.cmd.Process.Pid {
		return bwrapTermGrace
	}
	return shutdownTimeout
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

// EventBus is the minimal slice the runner needs - matches shared.EventBus
// Publish signature (event must satisfy shared.Event, i.e. have EventName()).
type EventBus interface {
	Publish(ctx context.Context, event shared.Event) error
}

// SDKStubCopier refreshes the server-shipped SDK stubs (plugin_api.proto,
// generated clients) inside a plugin's install dir. Implemented by
// filesystem.PluginFileSystem. The stubs are the SERVER's contract: they are
// re-copied at every Start so installed plugins follow the server's proto
// across upgrades instead of keeping the copy frozen at install time.
type SDKStubCopier interface {
	CopySDKStubs(language, installDir string) error
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
	sandboxLibOverride  string // test-only: override sandboxLibPath so the fail-closed path is deterministic regardless of ambient lib presence
	uidSepMode          string // "auto"|"on"|"off": drop plugins to evey-plugin uid via launcher cap_setuid
	eventBus            EventBus
	hostRegistry        HostRegistry
	sdkStubs            SDKStubCopier
	autoResolveOnce     sync.Once // resolves sandboxModeAuto to a concrete mode once
	autoResolved        string
	resMonOnce          sync.Once // starts the resource-monitor sweeper at most once
	sandboxProbeOnce    sync.Once // memoises the host sandbox probes for SandboxStatus
	probeLandlock       bool
	probeLauncher       bool
	probeBwrap          bool

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
// library cannot be located or compiled - the previous behaviour
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
//     security misconfiguration - callers in wire.go must emit a visible
//     warning so ops can catch it (see cmd/evey/wire.go).
func (r *Runner) WithAllowMissingSandbox(allow bool) *Runner {
	r.allowMissingSandbox = allow
	return r
}

// WithSandboxLibPath overrides the path probed for libevey-sandbox.so. It exists
// so the fail-closed tests can force a "lib absent" condition deterministically
// (otherwise they depend on the ambient absence of /app/libevey-sandbox.so and
// pass on a dev laptop while failing in the prod-like container that builds it).
// Production wiring MUST NOT call this (see wire.go).
func (r *Runner) WithSandboxLibPath(path string) *Runner {
	r.sandboxLibOverride = path
	return r
}

// WithEventBus lets the runner emit plugin.error events on sandbox refusal
// and process crash so the notification service can page the admin.
func (r *Runner) WithEventBus(bus EventBus) *Runner {
	r.eventBus = bus
	return r
}

// WithSDKStubs wires the SDK stub refresh done at every plugin Start.
// When nil (tests), Start skips the refresh.
func (r *Runner) WithSDKStubs(c SDKStubCopier) *Runner {
	r.sdkStubs = c
	return r
}

// WithHostRegistry wires the per-plugin host socket factory. When set,
// the runner provisions a dedicated host gRPC socket for each plugin and
// pins authentication to the plugin process PID. When nil (tests), the
// plugin starts without a host socket bound to it - the EVEY_HOST_SOCK
// env var is left unset.
func (r *Runner) WithHostRegistry(reg HostRegistry) *Runner {
	r.hostRegistry = reg
	return r
}

// WithSandboxMode sets the isolation strategy used when launching plugins.
// Valid values: "auto" (default), "landlock", "bubblewrap", "strict", "basic".
//   - "auto"       - detect the best available at boot: landlock > bubblewrap
//     (only where Landlock is absent but bwrap runs under the host security) >
//     basic. Recommended for the official image: prefers Landlock (no privilege,
//     runs under Docker's default security), falls back as the host allows.
//   - "landlock"   - Landlock LSM self-confinement (filesystem allowlist). Needs
//     no mount/unshare, so it runs under Docker's default seccomp+AppArmor. Needs
//     kernel ≥5.13 with Landlock in the active LSM list. Refuses to start a
//     plugin if Landlock cannot be enforced (fail closed).
//   - "bubblewrap" - bwrap namespace isolation (user/pid/uts/ipc/net). Requires
//     the bwrap binary AND the mount syscall, so inside Docker it only works
//     with relaxed security; effectively bare-metal only.
//   - "strict"     - bubblewrap namespaces AND LD_PRELOAD seccomp lib.
//   - "basic"      - LD_PRELOAD seccomp lib + UID drop only (no per-path FS
//     isolation; last-resort fallback for locked-down old kernels).
//
// Any unrecognised value (including empty string) is silently mapped to
// defaultSandboxMode ("auto") - callers should validate upstream.
func (r *Runner) WithSandboxMode(mode string) *Runner {
	r.SetSandboxMode(mode)
	return r
}

// WithResourceLimits sets the per-plugin memory and PID caps the resource
// monitor enforces (it polls /proc and kills a plugin that exceeds them -
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
		slog.WarnContext(ctx, "publish plugin.error failed", slog.String("plugin", pluginID), slog.Any("err", err))
	}
}
