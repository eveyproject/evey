package pluginrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

const (
	sockDir           = "/tmp/evey/plugins"
	defaultPluginDir  = "/data/plugins"
	shutdownTimeout   = 5 * time.Second
	killTimeout       = 3 * time.Second
	pluginExecTimeout = 30 * time.Minute
)

type processEntry struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	sockPath string
	pluginID string
}

type Runner struct {
	mu         sync.RWMutex
	processes  map[string]*processEntry
	pluginDirs []string
}

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

func (r *Runner) validateEntrypoint(entrypoint, installDir string) (string, error) {
	if strings.Contains(entrypoint, "..") {
		return "", fmt.Errorf("entrypoint must not contain '..'")
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
		return "", fmt.Errorf("entrypoint is a directory")
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("entrypoint is not executable")
	}

	return resolved, nil
}

func (r *Runner) Start(_ context.Context, p *plugin.Plugin) error {
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		return fmt.Errorf("create sock dir: %w", err)
	}

	sock := filepath.Join(sockDir, p.ID+".sock")
	os.Remove(sock)

	entrypoint := p.Entrypoint
	if entrypoint == "" {
		entrypoint = p.Name
	}

	resolved, err := r.validateEntrypoint(entrypoint, p.InstallDir)
	if err != nil {
		return fmt.Errorf("plugin %s: %w", p.ID, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pluginExecTimeout)
	cmd := exec.CommandContext(ctx, resolved)
	if p.InstallDir != "" {
		cmd.Dir = p.InstallDir
	}
	hostSock := filepath.Join(sockDir, "evey-host.sock")
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"EVEY_PLUGIN_ID=" + p.ID,
		"EVEY_PLUGIN_SOCK=" + sock,
		"EVEY_HOST_SOCK=" + hostSock,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start plugin %s: %w", p.ID, err)
	}

	entry := &processEntry{
		cmd:      cmd,
		cancel:   cancel,
		sockPath: sock,
		pluginID: p.ID,
	}

	r.mu.Lock()
	r.processes[p.ID] = entry
	r.mu.Unlock()

	return nil
}

func (r *Runner) Stop(_ context.Context, pluginID string) error {
	r.mu.Lock()
	entry, ok := r.processes[pluginID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("plugin %s not running", pluginID)
	}
	delete(r.processes, pluginID)
	r.mu.Unlock()

	defer entry.cancel()

	if entry.cmd.Process == nil {
		os.Remove(entry.sockPath)
		return nil
	}

	entry.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- entry.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		entry.cmd.Process.Signal(syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(killTimeout):
		}
	}

	os.Remove(entry.sockPath)
	return nil
}

func (r *Runner) Health(_ context.Context, pluginID string) (bool, error) {
	r.mu.RLock()
	entry, ok := r.processes[pluginID]
	r.mu.RUnlock()

	if !ok {
		return false, nil
	}

	if entry.cmd.ProcessState != nil && entry.cmd.ProcessState.Exited() {
		return false, nil
	}

	return true, nil
}

func (r *Runner) SendEvent(_ context.Context, pluginID string, event interface{}) error {
	r.mu.RLock()
	_, ok := r.processes[pluginID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin %s not running", pluginID)
	}

	return nil
}
