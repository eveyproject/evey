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

// WithAllowMissingSandbox lets tests and dev environments skip the
// strict "sandbox-or-refuse" check. Production MUST leave this false.
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
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		return fmt.Errorf("create sock dir: %w", err)
	}

	// Per-plugin socket subdirectory — isolates sockets between plugins
	pluginSockDir := filepath.Join(sockDir, p.ID)
	if err := os.MkdirAll(pluginSockDir, 0o700); err != nil {
		return fmt.Errorf("create plugin sock dir: %w", err)
	}
	if os.Getuid() == 0 {
		if err := os.Chown(pluginSockDir, pluginUID, pluginGID); err != nil {
			return fmt.Errorf("chown plugin sock dir: %w", err)
		}
	}

	sock := filepath.Join(pluginSockDir, "plugin.sock")
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove old plugin sock", slog.Any("err", err))
	}

	entrypoint := p.Entrypoint
	if entrypoint == "" {
		entrypoint = p.Name
	}

	parts := strings.Fields(entrypoint)
	if len(parts) == 0 {
		return fmt.Errorf("plugin %s: empty entrypoint", p.ID)
	}

	// Verify interpreter is installed before attempting to start
	if isInterpreter(parts[0]) {
		if _, lookErr := exec.LookPath(parts[0]); lookErr != nil {
			return fmt.Errorf("plugin %s: runtime %q not installed", p.ID, parts[0])
		}
	}

	var cmdName string
	var cmdArgs []string
	if isInterpreter(parts[0]) {
		cmdName = parts[0]
		if len(parts) > 1 {
			scriptPath := parts[1]
			if !filepath.IsAbs(scriptPath) && p.InstallDir != "" {
				scriptPath = filepath.Join(p.InstallDir, scriptPath)
			}
			if _, err := os.Stat(scriptPath); err != nil {
				return fmt.Errorf("plugin %s: script not found: %w", p.ID, err)
			}
			cmdArgs = append([]string{scriptPath}, parts[2:]...)
		}
	} else {
		resolved, resolveErr := r.validateEntrypoint(parts[0], p.InstallDir)
		if resolveErr != nil {
			return fmt.Errorf("plugin %s: %w", p.ID, resolveErr)
		}
		cmdName = resolved
		cmdArgs = parts[1:]
	}

	ctx, cancel := context.WithTimeout(parentCtx, pluginExecTimeout)
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	if p.InstallDir != "" {
		cmd.Dir = p.InstallDir
	}
	hostSock := filepath.Join(sockDir, "evey-host.sock")
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"NODE_PATH=/usr/local/lib/node_modules",
		"EVEY_PLUGIN_ID=" + p.ID,
		"EVEY_PLUGIN_SOCK=" + sock,
		"EVEY_HOST_SOCK=" + hostSock,
	}
	// Apply seccomp sandbox unless plugin has subprocess permission.
	// Fail-closed: if the sandbox lib is missing, refuse to start rather
	// than silently running the plugin unsandboxed. Tests / dev can opt
	// out via WithAllowMissingSandbox(true).
	if !slices.Contains(p.Permissions, subprocessPerm) {
		libPath := r.ensureSandboxLib()
		if libPath == "" {
			if !r.allowMissingSandbox {
				slog.Error("refusing to start plugin without sandbox",
					slog.String("plugin_id", p.ID),
					slog.String("sandbox_lib", sandboxLibPath))
				r.publishPluginError(parentCtx, p.ID, "sandbox library unavailable; plugin refused start")
				return fmt.Errorf("plugin %s: %w", p.ID, ErrSandboxUnavailable)
			}
			slog.Warn("starting plugin without sandbox (dev override)",
				slog.String("plugin_id", p.ID))
		} else {
			cmd.Env = append(cmd.Env, "LD_PRELOAD="+libPath)
		}
	}

	if r.logger != nil {
		cmd.Stdout = r.logger.Writer(p.ID)
		cmd.Stderr = r.logger.Writer(p.ID)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	sysProcAttr := &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	// Drop privileges: run plugins as evey-plugin (UID 1001) when server is root
	if os.Getuid() == 0 {
		sysProcAttr.Credential = &syscall.Credential{Uid: pluginUID, Gid: pluginGID}
	}
	cmd.SysProcAttr = sysProcAttr

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(pluginSockDir)
		return fmt.Errorf("start plugin %s: %w", p.ID, err)
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

	// Try to connect IPC client (non-blocking — plugin may not create socket immediately)
	go r.connectIPC(ctx, entry, p)

	r.mu.Lock()
	r.processes[p.ID] = entry
	r.mu.Unlock()

	go func() {
		defer close(entry.done)
		defer func() {
			if rv := recover(); rv != nil {
				slog.Error("panic in plugin runner", slog.Any("recover", rv), slog.String("plugin", p.ID))
			}
		}()
		err := cmd.Wait()
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
		if err != nil {
			slog.Warn("plugin process crashed", slog.String("plugin", p.ID), slog.Any("err", err))
			// Use a detached context: the crash can happen long after parentCtx
			// has been cancelled (request scope ended). A cancelled ctx would
			// drop the admin notification silently.
			r.publishPluginError(context.Background(), p.ID, "process crashed: "+err.Error())
		} else {
			slog.Info("plugin process exited", slog.String("plugin", p.ID))
		}
	}()

	return nil
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
		srcPath := filepath.Join("/app", "sandbox", "sandbox.c")
		if _, err := os.Stat(srcPath); err != nil {
			return
		}
		cmd := exec.Command("gcc", "-shared", "-fPIC", "-nostartfiles", "-o", sandboxLibPath, srcPath)
		if err := cmd.Run(); err != nil {
			slog.Warn("compile sandbox lib failed", slog.Any("err", err))
			return
		}
		slog.Info("compiled sandbox lib", slog.String("path", sandboxLibPath))
		r.sandboxLib = sandboxLibPath
	})
	return r.sandboxLib
}
