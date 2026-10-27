package pluginrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const defaultLogDir = "/data/logs/plugins"
const defaultMaxFileSize int64 = 5 << 20 // 5MB

const defaultMaxDayBytes int64 = 50 << 20 // 50MB per plugin per day

const logCapNotice = "[WARN] daily log budget reached; further plugin output is dropped until tomorrow\n"

// PluginLister returns installed plugin IDs (used for orphan cleanup).
type PluginLister interface {
	ListIDs(ctx context.Context) ([]string, error)
}

type pluginLogWriter struct {
	mu       sync.Mutex
	dir      string
	pluginID string
	maxSize  int64
	current  *os.File
	curDate  string
	curSize  int64
	// Rotation alone never bounds disk use: past maxSize a plugin opens
	// hourly files, and an hour's file keeps growing. This caps the day.
	maxDayBytes int64
	dayBytes    int64
	capped      bool
}

func (w *pluginLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if w.current == nil || w.curDate != today || w.curSize+int64(len(p)) > w.maxSize {
		w.rotate(today)
	}
	if w.current == nil {
		return 0, errors.New("no log file")
	}
	if w.dayBytes+int64(len(p)) > w.maxDayBytes {
		if !w.capped {
			w.capped = true
			n, _ := io.WriteString(w.current, logCapNotice) //nolint:errcheck // best-effort notice; the capped write reports success anyway
			w.curSize += int64(n)
		}
		// Report success: an error would break the copy from the plugin's
		// stdout pipe and stall or kill the plugin on its next write.
		return len(p), nil
	}
	n, err := w.current.Write(p)
	w.curSize += int64(n)
	w.dayBytes += int64(n)
	return n, err
}

// existingDayBytes sums the plugin's files for day, so a restarted server
// resumes the budget instead of granting a fresh one.
func (w *pluginLogWriter) existingDayBytes(day string) int64 {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0
	}
	prefix := w.pluginID + "-" + day
	var total int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		if rest := strings.TrimSuffix(name[len(prefix):], ".log"); rest != "" && (len(rest) != 3 || rest[0] != '-') {
			continue
		}
		if info, infoErr := e.Info(); infoErr == nil {
			total += info.Size()
		}
	}
	return total
}

func (w *pluginLogWriter) rotate(today string) {
	if w.current != nil {
		w.current.Close()
		w.current = nil
	}

	if w.curDate != today {
		w.dayBytes = w.existingDayBytes(today)
		w.capped = w.dayBytes >= w.maxDayBytes
	}

	name := w.pluginID + "-" + today + ".log"
	path := filepath.Join(w.dir, name)

	// If file exists and exceeds max size, add hour to filename
	if info, err := os.Stat(path); err == nil && info.Size() >= w.maxSize {
		name = w.pluginID + "-" + today + "-" + time.Now().Format("15") + ".log"
		path = filepath.Join(w.dir, name)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		slog.Warn("open plugin log file", slog.String("path", path), slog.Any("err", err))
		return
	}
	w.current = f
	w.curDate = today
	if info, statErr := f.Stat(); statErr == nil {
		w.curSize = info.Size()
	}
}

func (w *pluginLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current != nil {
		err := w.current.Close()
		w.current = nil
		return err
	}
	return nil
}

type PluginLogger struct {
	mu         sync.Mutex
	logDir     string
	writers    map[string]*pluginLogWriter
	maxAgeDays int
}

func NewPluginLogger(logDir string, maxAgeDays int) *PluginLogger {
	if logDir == "" {
		logDir = defaultLogDir
	}
	if maxAgeDays <= 0 {
		maxAgeDays = 10
	}
	return &PluginLogger{
		logDir:     logDir,
		writers:    make(map[string]*pluginLogWriter),
		maxAgeDays: maxAgeDays,
	}
}

func (l *PluginLogger) getOrCreateWriter(pluginID string) *pluginLogWriter {
	if w, ok := l.writers[pluginID]; ok {
		return w
	}
	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		slog.Warn("plugin logger: create log dir", slog.Any("err", err))
	}
	w := &pluginLogWriter{
		dir:         l.logDir,
		pluginID:    pluginID,
		maxSize:     defaultMaxFileSize,
		maxDayBytes: defaultMaxDayBytes,
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
		w.Close()
		delete(l.writers, pluginID)
	}
}

// Close closes all plugin log writers.
func (l *PluginLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, w := range l.writers {
		w.Close()
		delete(l.writers, id)
	}
}

// SetRetentionDays updates retention for future cleanups.
func (l *PluginLogger) SetRetentionDays(days int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if days <= 0 {
		days = 10
	}
	l.maxAgeDays = days
}

// StartCleanup periodically removes old log files and orphan logs.
func (l *PluginLogger) StartCleanup(ctx context.Context, lister PluginLister) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered in plugin log cleanup", "error", r, "stack", string(debug.Stack()))
			}
		}()
		// Run once at startup
		l.cleanOldLogs()
		l.cleanOrphanLogs(ctx, lister)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.cleanOldLogs()
				l.cleanOrphanLogs(ctx, lister)
			}
		}
	}()
}

// cleanOldLogs removes log files older than maxAgeDays by parsing date from filename.
func (l *PluginLogger) cleanOldLogs() {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -l.maxAgeDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		date, ok := extractDateFromFilename(e.Name())
		if !ok {
			continue
		}
		if date.Before(cutoff) {
			path := filepath.Join(l.logDir, e.Name())
			if rmErr := os.Remove(path); rmErr == nil {
				slog.Info("plugin logger: removed old log", slog.String("path", path))
			}
		}
	}
}

// extractDateFromFilename parses YYYY-MM-DD from
// "{pluginID}-YYYY-MM-DD.log" or "{pluginID}-YYYY-MM-DD-HH.log".
// The second return is false when no parseable date is found - explicit
// signal rather than overloading time.Time{} as "absent", which confuses
// IsZero() vs "the unix epoch midpoint" for future edge cases.
func extractDateFromFilename(name string) (time.Time, bool) {
	name = strings.TrimSuffix(name, ".log")
	// Find the date part - last occurrence of YYYY-MM-DD pattern
	parts := strings.Split(name, "-")
	// Need at least pluginID prefix + year + month + day
	for i := 0; i+2 < len(parts); i++ {
		if len(parts[i]) == 4 {
			dateStr := parts[i] + "-" + parts[i+1] + "-" + parts[i+2]
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func (l *PluginLogger) cleanOrphanLogs(ctx context.Context, lister PluginLister) {
	ids, err := lister.ListIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "plugin logger: list plugins for cleanup", slog.Any("err", err))
		return
	}

	installed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		installed[id] = struct{}{}
	}

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		matched := false
		for id := range installed {
			if strings.HasPrefix(e.Name(), id+"-") {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		path := filepath.Join(l.logDir, e.Name())
		if rmErr := os.Remove(path); rmErr == nil {
			slog.Info("plugin logger: removed orphan log", slog.String("path", path))
		}
	}
}
