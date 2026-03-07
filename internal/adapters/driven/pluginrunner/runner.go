package pluginrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

const (
	sockDir         = "/tmp/evey/plugins"
	shutdownTimeout = 5 * time.Second
	killTimeout     = 3 * time.Second
)

type processEntry struct {
	cmd      *exec.Cmd
	sockPath string
	pluginID string
}

type Runner struct {
	mu        sync.RWMutex
	processes map[string]*processEntry
}

func NewRunner() *Runner {
	return &Runner{
		processes: make(map[string]*processEntry),
	}
}

func (r *Runner) Start(_ context.Context, p *plugin.Plugin) error {
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		return fmt.Errorf("create sock dir: %w", err)
	}

	sock := filepath.Join(sockDir, p.Name+".sock")
	os.Remove(sock)

	cmd := exec.Command(p.Name)
	cmd.Env = append(os.Environ(),
		"EVEY_PLUGIN_ID="+p.ID,
		"EVEY_PLUGIN_SOCK="+sock,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pdeathsig: syscall.SIGTERM,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start plugin %s: %w", p.Name, err)
	}

	entry := &processEntry{
		cmd:      cmd,
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

	if entry.cmd.Process == nil {
		os.Remove(entry.sockPath)
		return nil
	}

	// SIGTERM first
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
