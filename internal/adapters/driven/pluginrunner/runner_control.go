package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// runner_control.go holds the runtime control surface of a running plugin:
// graceful stop, kill, health check and event delivery.

// signalPluginGroup sends sig to the plugin's process group (Setpgid makes
// pgid == the leader pid), falling back to the leader pid alone if the group
// send fails. ESRCH (already gone) is not worth a log line. Mirrors the
// group-termination the boot-time reaper and Kill already use, so a plugin's
// forked children die with it on graceful stop too.
func signalPluginGroup(ctx context.Context, pid int, sig syscall.Signal) {
	if err := signalPlugin(ctx, pid, sig); err != nil {
		slog.WarnContext(ctx, "signal plugin", slog.Int("pid", pid), slog.String("signal", sig.String()), slog.Any("err", err))
	}
}

func (r *Runner) Stop(ctx context.Context, pluginID string) error {
	// Always clear the host-socket registry on the way out - including the early
	// returns below - so a desynced orphan (gone from r.processes but still
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
			slog.WarnContext(ctx, "plugin Shutdown RPC failed", slog.String("pluginID", pluginID), slog.Any("err", err))
		}
		shutdownCancel()
		if err := ipc.Close(); err != nil {
			slog.WarnContext(ctx, "close plugin IPC", slog.Any("err", err))
		}
	}

	if entry.cmd.Process == nil {
		os.RemoveAll(filepath.Dir(entry.sockPath))
		return nil
	}

	// Signal the whole process GROUP, not just the leader: a plugin holding the
	// "subprocess" permission can fork children (the seccomp fork-deny is lifted
	// for it), and a SIGTERM/SIGKILL to the leader alone leaves those children
	// running - reparented and untracked until the next boot's ReapStaleProcesses.
	// Plugins are spawned with Setpgid, so pgid == the leader pid; -pid reaches
	// every descendant. Fall back to the leader pid if the group send fails.
	signalPluginGroup(ctx, entry.cmd.Process.Pid, syscall.SIGTERM)

	select {
	case <-entry.done:
	case <-time.After(entry.termGrace()):
		signalPluginGroup(ctx, entry.cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-entry.done:
		case <-time.After(killTimeout):
		}
	}

	// Remove entire socket subdirectory
	if err := os.RemoveAll(filepath.Dir(entry.sockPath)); err != nil {
		slog.WarnContext(ctx, "remove plugin sock dir", slog.Any("err", err))
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

	// SIGKILL the entire process group - the plugin may have spawned children
	// (interpreters, worker threads) that need to die along with the leader.
	// Setpgid:true in SysProcAttr guarantees pgid == pid for the leader.
	if err := signalPlugin(context.Background(), entry.cmd.Process.Pid, syscall.SIGKILL); err != nil {
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

	// Reading entry.cmd.ProcessState here races trackLifecycle's cmd.Wait(),
	// which writes it on exit. entry.done is closed once Wait returns, and
	// channel receive is synchronised - so a non-blocking receive is the
	// race-free way to ask "has the process exited?".
	select {
	case <-entry.done:
		return false, nil
	default:
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

// Ask is plugin.PluginRunner: one event the plugin answers, its result kept.
func (r *Runner) Ask(ctx context.Context, pluginID, eventType string, payload map[string]any, timeout time.Duration) (map[string]any, bool, error) {
	r.mu.RLock()
	entry, ok := r.processes[pluginID]
	var ipc *ipcClient
	if ok {
		ipc = entry.ipc
	}
	r.mu.RUnlock()
	if !ok {
		return nil, false, fmt.Errorf("plugin %s not running", pluginID)
	}
	if ipc == nil {
		return nil, false, fmt.Errorf("plugin %s IPC not connected", pluginID)
	}
	return ipc.Ask(ctx, eventType, payload, "host", timeout)
}
