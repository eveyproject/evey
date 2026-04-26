package pluginrunner

import (
	"context"
	"errors"
	"fmt"
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
)

type processEntry struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	sockPath string
	pluginID string
	ipc      *ipcClient
	done     chan struct{} // closed when cmd.Wait() returns in Start goroutine
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
	eventBus            EventBus
	hostRegistry        HostRegistry
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

	ctx, cancel := context.WithTimeout(parentCtx, pluginExecTimeout)
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
	if err := r.applySandbox(parentCtx, p, cmd); err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, true)
		return err
	}

	if r.logger != nil {
		cmd.Stdout = r.logger.Writer(p.ID)
		cmd.Stderr = r.logger.Writer(p.ID)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.SysProcAttr = pluginSysProcAttr()

	if err := cmd.Start(); err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, true)
		return fmt.Errorf("start plugin %s: %w", p.ID, err)
	}

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
		cmd:      cmd,
		cancel:   cancel,
		sockPath: sock,
		pluginID: p.ID,
		done:     make(chan struct{}),
	}
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
		if err := os.Chown(pluginSockDir, pluginUID, pluginGID); err != nil {
			return "", "", fmt.Errorf("chown plugin sock dir: %w", err)
		}
	}
	sock := filepath.Join(pluginSockDir, "plugin.sock")
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove old plugin sock", slog.Any("err", err))
	}
	return pluginSockDir, sock, nil
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

// applySandbox appends LD_PRELOAD=<sandbox.so> to cmd.Env unless the plugin
// has the subprocess permission. Fail-closed: a plugin without subprocess
// AND without a sandbox lib is refused, with a plugin.error published so
// an admin gets paged. Dev/tests can opt out via WithAllowMissingSandbox.
func (r *Runner) applySandbox(parentCtx context.Context, p *plugin.Plugin, cmd *exec.Cmd) error {
	if slices.Contains(p.Permissions, subprocessPerm) {
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

func pluginSysProcAttr() *syscall.SysProcAttr {
	a := &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	if os.Getuid() == 0 {
		a.Credential = &syscall.Credential{Uid: pluginUID, Gid: pluginGID}
	}
	return a
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
	if r.hostRegistry != nil {
		r.hostRegistry.UnregisterPlugin(p.ID)
	}
	if err != nil {
		slog.Warn("plugin process crashed", slog.String("plugin", p.ID), slog.Any("err", err))
		r.publishPluginError(context.Background(), p.ID, "process crashed: "+err.Error())
	} else {
		slog.Info("plugin process exited", slog.String("plugin", p.ID))
	}
}

func (r *Runner) connectIPC(parentCtx context.Context, entry *processEntry, p *plugin.Plugin) {
	defer func() {
		if rv := recover(); rv != nil {
			slog.Error("panic in connectIPC", slog.Any("recover", rv), slog.String("plugin", p.ID))
		}
	}()
	client, err := newIPCClient(entry.sockPath)
	if err != nil {
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

	if r.hostRegistry != nil {
		r.hostRegistry.UnregisterPlugin(pluginID)
	}

	if r.logger != nil {
		r.logger.WriteLog(pluginID, "[INFO] plugin process stopped")
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
