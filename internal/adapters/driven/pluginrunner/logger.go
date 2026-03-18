package pluginrunner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/lumberjack.v2"
)

const defaultLogDir = "/data/logs/plugins"

// PluginLister returns installed plugin IDs (used for orphan cleanup).
type PluginLister interface {
	ListIDs(ctx context.Context) ([]string, error)
}

type PluginLogger struct {
	mu         sync.Mutex
	logDir     string
	writers    map[string]*lumberjack.Logger
	maxAgeDays int
}

func NewPluginLogger(logDir string, maxAgeDays int) *PluginLogger {
	if logDir == "" {
		logDir = defaultLogDir
	}
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}
	return &PluginLogger{
		logDir:     logDir,
		writers:    make(map[string]*lumberjack.Logger),
		maxAgeDays: maxAgeDays,
	}
}

func (l *PluginLogger) getOrCreateWriter(pluginID string) *lumberjack.Logger {
	if w, ok := l.writers[pluginID]; ok {
		return w
	}
	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		slog.Warn("plugin logger: create log dir", slog.Any("err", err))
	}
	logPath := filepath.Join(l.logDir, pluginID+".log")
	w := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // MB
		MaxBackups: 3,
		MaxAge:     l.maxAgeDays,
		Compress:   true,
	}
	l.writers[pluginID] = w
	return w
}

// Writer returns an io.Writer for the given plugin, suitable for cmd.Stdout/Stderr.
func (l *PluginLogger) Writer(pluginID string) io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.getOrCreateWriter(pluginID)
}

// WriteLog writes a timestamped line to the plugin's log file.
func (l *PluginLogger) WriteLog(pluginID, line string) {
	l.mu.Lock()
	w := l.getOrCreateWriter(pluginID)
	l.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(w, "%s %s\n", ts, line)
}

// ClosePlugin closes and removes the writer for a specific plugin.
func (l *PluginLogger) ClosePlugin(pluginID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w, ok := l.writers[pluginID]; ok {
		_ = w.Close()
		delete(l.writers, pluginID)
	}
}

// Close closes all plugin log writers.
func (l *PluginLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, w := range l.writers {
		_ = w.Close()
		delete(l.writers, id)
	}
}

// SetRetentionDays updates retention on all existing and future writers.
func (l *PluginLogger) SetRetentionDays(days int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if days <= 0 {
		days = 30
	}
	l.maxAgeDays = days
	for _, w := range l.writers {
		w.MaxAge = days
	}
}

// StartCleanup periodically removes log files for plugins that no longer exist.
func (l *PluginLogger) StartCleanup(ctx context.Context, lister PluginLister) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.cleanOrphanLogs(ctx, lister)
			}
		}
	}()
}

func (l *PluginLogger) cleanOrphanLogs(ctx context.Context, lister PluginLister) {
	ids, err := lister.ListIDs(ctx)
	if err != nil {
		slog.Warn("plugin logger: list plugins for cleanup", slog.Any("err", err))
		return
	}

	installed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		installed[id] = struct{}{}
	}

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		slog.Warn("plugin logger: read log dir for cleanup", slog.Any("err", err))
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Strip .gz and .log extensions; rotated files have "-<timestamp>" suffix
		base := strings.TrimSuffix(name, ".gz")
		base = strings.TrimSuffix(base, ".log")
		matched := false
		for id := range installed {
			if base == id || strings.HasPrefix(base, id+"-") {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		path := filepath.Join(l.logDir, name)
		if err := os.Remove(path); err != nil {
			slog.Warn("plugin logger: remove orphan log", slog.String("path", path), slog.Any("err", err))
		} else {
			slog.Info("plugin logger: removed orphan log", slog.String("path", path))
		}
	}
}
