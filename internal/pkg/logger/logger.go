package logger

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

	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	Level         string
	Dir           string
	MaxSizeMB     int
	RetentionDays int
	DevMode       bool
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// globalLevel is the live log level shared by every handler the system logger
// builds, so SetLevel can raise/lower verbosity at runtime (e.g. an admin
// flips to debug without a restart) and it takes effect immediately.
var globalLevel = new(slog.LevelVar)

// SetLevel changes the system log level at runtime. Unknown values map to info.
func SetLevel(s string) { globalLevel.Set(parseLevel(s)) }

// rotatingWriter writes to date-stamped log files with size rotation.
type rotatingWriter struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	maxSize int64
	current *os.File
	curDate string
	curSize int64
}

func newRotatingWriter(dir, prefix string, maxSizeMB int) *rotatingWriter {
	if maxSizeMB <= 0 {
		maxSizeMB = 50
	}
	return &rotatingWriter{
		dir:     dir,
		prefix:  prefix,
		maxSize: int64(maxSizeMB) << 20,
	}
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if w.current == nil || w.curDate != today || w.curSize+int64(len(p)) > w.maxSize {
		w.rotate(today)
	}
	if w.current == nil {
		return os.Stderr.Write(p) // fallback
	}
	n, err := w.current.Write(p)
	w.curSize += int64(n)
	return n, err
}

func (w *rotatingWriter) rotate(today string) {
	if w.current != nil {
		w.current.Close()
		w.current = nil
	}

	name := w.prefix + "-" + today + ".log"
	path := filepath.Join(w.dir, name)

	if info, err := os.Stat(path); err == nil && info.Size() >= w.maxSize {
		name = w.prefix + "-" + today + "-" + time.Now().Format("15") + ".log"
		path = filepath.Join(w.dir, name)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: open %s: %v\n", path, err)
		return
	}
	w.current = f
	w.curDate = today
	if info, statErr := f.Stat(); statErr == nil {
		w.curSize = info.Size()
	}
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current != nil {
		err := w.current.Close()
		w.current = nil
		return err
	}
	return nil
}

func newFileHandler(dir string, cfg Config) slog.Handler {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot create log dir: %v\n", err)
	}
	w := newRotatingWriter(dir, "evey", cfg.MaxSizeMB)
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       globalLevel, // live level (see SetLevel)
		AddSource:   false,
		ReplaceAttr: redactAttr, // see redact.go
	})
}

// contextHandler is a slog.Handler that automatically injects request_id and
// trace_id from the request context into every log record. It wraps an inner
// handler so the injection survives logger.With(...) / NewWith chaining.
type contextHandler struct {
	inner slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromCtx(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if sc := trace.SpanContextFromContext(ctx); sc.TraceID().IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{inner: h.inner.WithGroup(name)}
}

// New builds the system logger. JSON → file always; colorized text → stderr in dev mode.
func New(cfg Config) *slog.Logger {
	globalLevel.Set(parseLevel(cfg.Level))
	fileH := newFileHandler(cfg.Dir, cfg)

	if !cfg.DevMode {
		return slog.New(&contextHandler{inner: fileH})
	}

	multiH := &multiHandler{handlers: []slog.Handler{
		fileH,
		&devHandler{w: os.Stderr},
	}}
	return slog.New(&contextHandler{inner: multiH})
}

// NewWith returns a child logger with an added component attribute.
func NewWith(parent *slog.Logger, component string) *slog.Logger {
	return parent.With(slog.String("component", component))
}

// StartCleanup removes log files older than retention days. The optional
// initDone channel(s) are closed after the initial cleanOldLogs pass -
// tests pass one to wait deterministically instead of sleeping.
func StartCleanup(ctx context.Context, dir string, retentionDays int, initDone ...chan<- struct{}) {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// No ctx: detached log-retention goroutine, and this is the
				// logger's own bootstrap - there is no request in flight.
				slog.Error("panic in log cleanup", slog.Any("recover", r))
			}
		}()
		cleanOldLogs(dir, retentionDays)
		for _, ch := range initDone {
			if ch != nil {
				close(ch)
			}
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanOldLogs(dir, retentionDays)
			}
		}
	}()
}

func cleanOldLogs(dir string, retentionDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".log")
		parts := strings.Split(name, "-")
		for i := 0; i+2 < len(parts); i++ {
			if len(parts[i]) == 4 {
				dateStr := parts[i] + "-" + parts[i+1] + "-" + parts[i+2]
				if t, pErr := time.Parse("2006-01-02", dateStr); pErr == nil {
					if t.Before(cutoff) {
						path := filepath.Join(dir, e.Name())
						if rmErr := os.Remove(path); rmErr == nil {
							slog.Info("removed old server log", slog.String("path", path))
						}
					}
					break
				}
			}
		}
	}
}

// --- multi-handler ---

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// --- dev handler (colorized stderr) ---

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
)

type devHandler struct {
	w     io.Writer
	attrs []slog.Attr
	// prefix is the dotted group chain ("http.request.") applied to record attrs
	// and to attrs added after a WithGroup. Empty at the top level. Replaces a
	// previously-stored group name that Handle never read (WithGroup was a no-op).
	prefix string
}

func (h *devHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= globalLevel.Level()
}

func (h *devHandler) Handle(_ context.Context, r slog.Record) error {
	color := colorGreen
	label := "INF"
	switch r.Level {
	case slog.LevelDebug:
		color = colorCyan
		label = "DBG"
	case slog.LevelWarn:
		color = colorYellow
		label = "WRN"
	case slog.LevelError:
		color = colorRed
		label = "ERR"
	}

	ts := r.Time.Format(time.TimeOnly + ".000")

	var sb strings.Builder
	sb.WriteString(colorGray)
	sb.WriteString(ts)
	sb.WriteString(" ")
	sb.WriteString(color)
	sb.WriteString(label)
	sb.WriteString(colorReset)
	sb.WriteString(" ")
	sb.WriteString(r.Message)

	for _, a := range h.attrs {
		fmt.Fprintf(&sb, " %s%s=%v%s", colorGray, a.Key, a.Value, colorReset)
	}

	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&sb, " %s%s%s=%v%s", colorGray, h.prefix, a.Key, a.Value, colorReset)
		return true
	})

	sb.WriteString("\n")
	_, err := io.WriteString(h.w, sb.String())
	return err
}

func (h *devHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	for _, a := range attrs {
		// Attrs added after a WithGroup belong under that group, so qualify the key
		// with the current prefix now (prefix is "" before any group, leaving
		// top-level attrs untouched).
		merged = append(merged, slog.Attr{Key: h.prefix + a.Key, Value: a.Value})
	}
	return &devHandler{w: h.w, attrs: merged, prefix: h.prefix}
}

func (h *devHandler) WithGroup(name string) slog.Handler {
	// An empty group name is a no-op per the slog.Handler contract.
	if name == "" {
		return h
	}
	return &devHandler{w: h.w, attrs: h.attrs, prefix: h.prefix + name + "."}
}
