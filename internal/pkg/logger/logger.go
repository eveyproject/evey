package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/lumberjack.v2"
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

func newFileHandler(path string, cfg Config) slog.Handler {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot create log dir: %v\n", err)
	}
	lj := &lumberjack.Logger{
		Filename:  path,
		MaxSize:   cfg.MaxSizeMB,
		MaxAge:    cfg.RetentionDays,
		LocalTime: false,
		Compress:  true,
	}
	return slog.NewJSONHandler(lj, &slog.HandlerOptions{
		Level:     parseLevel(cfg.Level),
		AddSource: false,
	})
}

// New builds the system logger. JSON → file always; colorized text → stderr in dev mode.
func New(cfg Config) *slog.Logger {
	path := filepath.Join(cfg.Dir, "evey.log")
	fileH := newFileHandler(path, cfg)

	if !cfg.DevMode {
		return slog.New(fileH)
	}

	// Dev: both JSON file + colorized stderr
	multiH := &multiHandler{handlers: []slog.Handler{
		fileH,
		&devHandler{
			w:     os.Stderr,
			level: parseLevel(cfg.Level),
		},
	}}
	return slog.New(multiH)
}

// NewWith returns a child logger with an added component attribute.
func NewWith(parent *slog.Logger, component string) *slog.Logger {
	return parent.With(slog.String("component", component))
}

// NewFileLogger creates a standalone logger writing only to the given file.
func NewFileLogger(path string, cfg Config) *slog.Logger {
	return slog.New(newFileHandler(path, cfg))
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
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
	return nil
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
	level slog.Level
	attrs []slog.Attr
	group string
}

func (h *devHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
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

	// pre-set attrs
	for _, a := range h.attrs {
		fmt.Fprintf(&sb, " %s%s=%v%s", colorGray, a.Key, a.Value, colorReset)
	}

	// record attrs
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&sb, " %s%s=%v%s", colorGray, a.Key, a.Value, colorReset)
		return true
	})

	sb.WriteString("\n")
	_, err := io.WriteString(h.w, sb.String())
	return err
}

func (h *devHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &devHandler{w: h.w, level: h.level, attrs: merged, group: h.group}
}

func (h *devHandler) WithGroup(name string) slog.Handler {
	return &devHandler{w: h.w, level: h.level, attrs: h.attrs, group: name}
}
