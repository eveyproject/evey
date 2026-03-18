package pluginrunner

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultLogDir = "/data/logs/plugins"

type PluginLogger struct {
	mu     sync.Mutex
	logDir string
	files  map[string]*os.File
}

func NewPluginLogger(logDir string) *PluginLogger {
	if logDir == "" {
		logDir = defaultLogDir
	}
	return &PluginLogger{
		logDir: logDir,
		files:  make(map[string]*os.File),
	}
}

func (l *PluginLogger) WriteLog(pluginID, level, msg string, fields map[string]string) {
	entry := map[string]any{
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"level":  level,
		"msg":    msg,
		"plugin": pluginID,
	}
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("marshal plugin log", slog.Any("err", err))
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := l.getOrCreateFile(pluginID)
	if err != nil {
		slog.Warn("open plugin log file", slog.String("pluginID", pluginID), slog.Any("err", err))
		return
	}
	if _, err := f.Write(data); err != nil {
		slog.Warn("write plugin log", slog.String("pluginID", pluginID), slog.Any("err", err))
	}
}

func (l *PluginLogger) getOrCreateFile(pluginID string) (*os.File, error) {
	if f, ok := l.files[pluginID]; ok {
		return f, nil
	}
	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(l.logDir, pluginID+".log")
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	l.files[pluginID] = f
	return f, nil
}

// LogWriter returns an io.Writer that writes to the plugin log file with the given level.
func (l *PluginLogger) LogWriter(pluginID, level string) io.Writer {
	return &pluginLogWriter{logger: l, pluginID: pluginID, level: level}
}

func (l *PluginLogger) ClosePlugin(pluginID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, ok := l.files[pluginID]; ok {
		f.Close()
		delete(l.files, pluginID)
	}
}

func (l *PluginLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range l.files {
		f.Close()
	}
	l.files = make(map[string]*os.File)
}

type pluginLogWriter struct {
	logger   *PluginLogger
	pluginID string
	level    string
}

func (w *pluginLogWriter) Write(p []byte) (int, error) {
	msg := string(p)
	if msg != "" && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	if msg != "" {
		w.logger.WriteLog(w.pluginID, w.level, msg, nil)
	}
	return len(p), nil
}
