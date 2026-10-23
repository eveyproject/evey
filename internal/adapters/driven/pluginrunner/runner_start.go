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
	"syscall"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// runner_start.go holds the plugin start path: entrypoint validation +
// containment, socket dir setup, command resolution, environment build, the
// process launch and its lifecycle/IPC connection tracking.

var knownInterpreters = map[string]bool{
	"python3": true, "python": true, "node": true, "php": true,
	"ruby": true, "perl": true, "lua": true, "bash": true, "sh": true,
}

func isInterpreter(cmd string) bool {
	return knownInterpreters[filepath.Base(cmd)]
}

func (r *Runner) validateEntrypoint(entrypoint, installDir string) (string, error) {
	resolved, info, err := r.containPluginFile(entrypoint, installDir)
	if err != nil {
		return "", err
	}
	if info.Mode()&0o111 == 0 {
		return "", errors.New("entrypoint is not executable")
	}
	return resolved, nil
}

// containPluginFile resolves a plugin-supplied file path (relative paths are
// anchored at installDir) and enforces that the resolved target lives under
// one of the allowed plugin directories. Shared by the binary entrypoint
// (which additionally requires the exec bit) and the interpreter SCRIPT
// argument - the script used to be joined unchecked, so "python ../../x.py"
// or an absolute path escaped the install dir (bounded by bwrap/Landlock at
// runtime, but the path must not point outside to begin with).
func (r *Runner) containPluginFile(path, installDir string) (string, os.FileInfo, error) {
	if strings.Contains(path, "..") {
		return "", nil, errors.New("plugin file path must not contain '..'")
	}

	candidate := path
	if !filepath.IsAbs(candidate) && installDir != "" {
		candidate = filepath.Join(installDir, candidate)
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", nil, fmt.Errorf("resolve plugin file path: %w", err)
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
		return "", nil, fmt.Errorf("plugin file %q outside allowed plugin directories", path)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("stat plugin file: %w", err)
	}
	if info.IsDir() {
		return "", nil, errors.New("plugin file is a directory")
	}
	return resolved, info, nil
}

// pluginProcContext builds the context governing a plugin process's lifetime.
// Detached from the caller's request context: the plugin must outlive the
// HTTP/gRPC request that triggered Start (without WithoutCancel, the request
// ctx is canceled as soon as the install handler returns, which cancels the
// cmd ctx and makes connectIPC's Initialize RPC fail with "code = Canceled"
// 200ms after the process spawns). No deadline either: plugins are
// long-running services, stopped only via Stop/Kill or the resource monitor -
// a deadline here SIGKILLs every plugin mid-flight (préprod: at exactly
// start+30min under the old pluginExecTimeout) and gets misread as an OOM.
func pluginProcContext(parentCtx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(parentCtx))
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

	// Refresh the server-shipped SDK stubs before spawning: they carry the
	// host RPC contract, and a plugin installed under an older server would
	// otherwise run against that older contract forever. Best-effort - a
	// read-only plugin dir must not block the start.
	if r.sdkStubs != nil && p.InstallDir != "" {
		if cpErr := r.sdkStubs.CopySDKStubs(p.Language, p.InstallDir); cpErr != nil {
			slog.WarnContext(parentCtx, "refresh SDK stubs", slog.String("pluginID", p.ID), slog.Any("err", cpErr))
		}
	}

	ctx, cancel := pluginProcContext(parentCtx)
	//nolint:gosec // G204: a plugin runner exists to exec plugin code; cmdName/cmdArgs come from resolveCommand→validateEntrypoint, which contains the target to the plugin install dir, and execution is then sandboxed (bwrap/landlock)
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	if p.InstallDir != "" {
		cmd.Dir = p.InstallDir
	}

	hostSock, err := r.registerHostSock(p.ID)
	if err != nil {
		r.rollbackStart(cancel, pluginSockDir, p.ID, false)
		return err
	}

	memCap, _, _ := r.limitsSnapshot()
	cmd.Env = buildPluginEnv(p.ID, sock, hostSock, memCap.defaults().memBytes)
	hostsCleanup, bwrapInfo, err := r.applySandbox(parentCtx, p, cmd)
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

	startErr := cmd.Start()
	for _, f := range cmd.ExtraFiles {
		_ = f.Close() // the child holds its own copy; ours would mask bwrap's EOF
	}
	if startErr != nil {
		if bwrapInfo != nil {
			_ = bwrapInfo.Close()
		}
		r.rollbackStart(cancel, pluginSockDir, p.ID, true)
		return fmt.Errorf("start plugin %s: %w", p.ID, startErr)
	}

	// The process that runs the plugin code. Under bubblewrap cmd.Process is
	// bwrap itself and the plugin its sandboxed child (--unshare-pid): that
	// child is the one connecting to the host socket and holding the memory.
	payloadPID := cmd.Process.Pid
	pidKnown := true
	if bwrapInfo != nil {
		childPID, infoErr := readBwrapChildPID(bwrapInfo)
		if infoErr != nil {
			// Fail closed: the host socket stays unbound and the plugin is
			// killed; trackLifecycle reports bwrap's own output as the crash.
			slog.ErrorContext(parentCtx, "bwrap did not report its child; killing plugin",
				slog.String("plugin_id", p.ID), slog.Any("err", infoErr))
			signalPluginGroup(parentCtx, cmd.Process.Pid, syscall.SIGKILL)
			pidKnown = false
		} else {
			payloadPID = childPID
		}
	}

	// Lower the plugin's scheduling priority so it cannot starve the server of
	// CPU under contention (preventive; the monitor only flags sustained pegging).
	r.applyNice(payloadPID)

	// Arm PID-based authentication on the host socket. From this point
	// on, only RPCs originating from payloadPID will be accepted -
	// SO_PEERCRED is filled in by the kernel and cannot be spoofed.
	if r.hostRegistry != nil && pidKnown {
		r.hostRegistry.SetExpectedPID(p.ID, int32(payloadPID)) //nolint:gosec // G115: a Linux PID fits int32 (pid_max <= 2^22)
	}

	if r.logger != nil {
		r.logger.WriteLog(p.ID, fmt.Sprintf("[INFO] plugin process started pid=%d", payloadPID))
	}

	entry := &processEntry{
		cmd:          cmd,
		payloadPID:   payloadPID,
		cancel:       cancel,
		sockPath:     sock,
		pluginID:     p.ID,
		done:         make(chan struct{}),
		installDir:   p.InstallDir,
		hostsCleanup: hostsCleanup,
		tail:         tail,
		hasGPU:       slices.Contains(p.Permissions, gpuPerm),
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
		// by the plugin group - otherwise the dropped process can't even search
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
		// can't traverse the 0700 parent to reach it - that was a real prod crash
		// ("listen EACCES /tmp/evey/plugins/<id>/plugin.sock"). Grant the group
		// SEARCH (0710: traverse only, no listing → can't enumerate other plugins)
		// on the parent. Holds only sockets, no secrets - the server's keys live
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
			// The script rides the same containment as a binary entrypoint
			// (minus the exec bit - the interpreter reads it): no "..", no
			// absolute path outside the allowed plugin directories.
			scriptPath, _, err := r.containPluginFile(parts[1], p.InstallDir)
			if err != nil {
				return "", nil, fmt.Errorf("plugin %s: script: %w", p.ID, err)
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
// registered) unregisters it. Keep the three ops in lock-step - drifting
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

func buildPluginEnv(pluginID, sock, hostSock string, memCapBytes int64) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"NODE_PATH=/usr/local/lib/node_modules",
		"EVEY_PLUGIN_ID=" + pluginID,
		"EVEY_PLUGIN_SOCK=" + sock,
		"EVEY_HOST_SOCK=" + hostSock,
	}
	// Align V8's heap ceiling with the resource monitor's RSS cap: node's
	// default old-space limit (~4 GB) lets a leaky plugin balloon straight
	// into the monitor's kill threshold instead of garbage-collecting below
	// it. 3/4 of the cap leaves room for the non-heap RSS (code, stack,
	// buffers). Harmless for non-node plugins - only node reads NODE_OPTIONS.
	if memCapBytes > 0 {
		heapMB := memCapBytes * 3 / 4 >> 20
		env = append(env, fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", heapMB))
	}
	return env
}

// trackLifecycle runs cmd.Wait in a goroutine and cleans up once the
// process exits. Uses a detached context for publishPluginError because
// the parent request scope may already be cancelled when the crash fires.
func (r *Runner) trackLifecycle(entry *processEntry, cancel context.CancelFunc, sock string, p *plugin.Plugin) {
	defer close(entry.done)
	defer func() {
		if rv := recover(); rv != nil {
			// Background lifecycle tracker: the request that started the
			// plugin has long since returned by the time it exits/crashes.
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
	// Every server-side stop drops the tracking entry first (see above), so a
	// clean exit here is the plugin quitting on its own: as dead as a crash,
	// and its output is the only explanation the admin will get.
	msg := classifyExitError(err)
	if err == nil {
		msg = "process exited on its own (status 0)"
	}
	if entry.tail != nil {
		if out := strings.TrimSpace(entry.tail.String()); out != "" {
			const maxTail = 600
			if len(out) > maxTail {
				out = "…" + out[len(out)-maxTail:]
			}
			msg += " - plugin output: " + out
		}
	}
	// Record before publishing so connectIPC (still waiting on the socket)
	// can surface this exact reason instead of a generic socket timeout.
	entry.setCrashReason(msg)
	slog.Warn("plugin process crashed", slog.String("plugin", p.ID), slog.String("classification", msg), slog.Any("err", err))
	r.publishPluginError(context.Background(), p.ID, msg)
}

// classifyExitError turns the raw exec.Cmd.Wait error into an admin-readable
// reason. This path only sees kills the server did NOT initiate: every
// server-side stop (Stop, Kill, the resource monitor's cap enforcement)
// removes the tracking entry before signaling, so trackLifecycle never
// reports it as a crash. A SIGKILL landing here therefore came from outside -
// in practice the kernel OOM-killer under container/host memory pressure, or
// a manual kill. Surface that instead of "signal: killed", and point at the
// resmon's own log line so admins don't chase the per-plugin cap by mistake.
func classifyExitError(err error) string {
	if err == nil {
		return ""
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		if ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL {
			return "process SIGKILLed from outside the server - likely the host/container OOM-killer under memory pressure, or a manual kill (a breach of the EVEY_PLUGINS_MEMORY_MB cap is logged as 'memory quota exceeded' instead)"
		}
	}
	return "process crashed: " + err.Error()
}

func (r *Runner) connectIPC(parentCtx context.Context, entry *processEntry, p *plugin.Plugin) {
	defer func() {
		if rv := recover(); rv != nil {
			slog.ErrorContext(parentCtx, "panic in connectIPC", slog.Any("recover", rv), slog.String("plugin", p.ID))
		}
	}()
	client, err := newIPCClient(entry.sockPath, entry.done)
	if err != nil {
		// The socket never came up. The process has very likely already
		// crashed - its exit reason (with captured output) is far more useful
		// than a bare "socket timeout". Wait briefly for trackLifecycle to
		// record it, then prefer it.
		select {
		case <-entry.done:
		case <-time.After(500 * time.Millisecond):
		}
		if cr := entry.getCrashReason(); cr != "" {
			err = fmt.Errorf("plugin exited before becoming ready: %s", cr)
		}
		slog.WarnContext(parentCtx, "plugin IPC connect failed", slog.String("pluginID", p.ID), slog.Any("err", err))
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
		slog.WarnContext(parentCtx, "plugin Initialize failed", slog.String("pluginID", p.ID), slog.Any("err", initErr))
		if r.onIPCResult != nil {
			r.onIPCResult(p.ID, initErr)
		}
		return
	}
	if r.onIPCResult != nil {
		r.onIPCResult(p.ID, nil)
	}
}
