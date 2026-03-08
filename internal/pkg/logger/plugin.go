package logger

import (
	"log/slog"
	"path/filepath"
	"sync"
)

// PluginLogManager manages per-plugin loggers backed by dedicated log files.
type PluginLogManager struct {
	cfg  Config
	mu   sync.Mutex
	logs map[string]*slog.Logger
}

func NewPluginLogManager(cfg Config) *PluginLogManager {
	return &PluginLogManager{cfg: cfg, logs: make(map[string]*slog.Logger)}
}

// Get returns (creating if needed) a logger for the given plugin ID.
func (m *PluginLogManager) Get(pluginID string) *slog.Logger {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.logs[pluginID]; ok {
		return l
	}
	path := filepath.Join(m.cfg.Dir, "plugins", pluginID+".log")
	l := NewFileLogger(path, m.cfg)
	l = l.With(slog.String("plugin_id", pluginID))
	m.logs[pluginID] = l
	return l
}
