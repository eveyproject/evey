package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/evey/internal/pkg/logger"
)

func TestNew_NotNil(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "info"})
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	// lumberjack creates file lazily; write to trigger creation
	l.Info("init")
	if _, err := os.Stat(filepath.Join(dir, "evey.log")); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestNew_DevMode(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug", DevMode: true})
	if l == nil {
		t.Fatal("expected non-nil logger in dev mode")
	}
	// dev mode logger should be enabled at debug level
	if !l.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("dev mode logger should be enabled at debug level")
	}
}

func TestNew_ProdNotDebug(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "warn"})
	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("warn-level logger should not be enabled at info")
	}
}

func TestNewWith(t *testing.T) {
	dir := t.TempDir()
	parent := logger.New(logger.Config{Dir: dir, Level: "info"})
	child := logger.NewWith(parent, "http")
	if child == nil {
		t.Fatal("expected non-nil child logger")
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := logger.WithRequestID(context.Background(), "req-123")
	got := logger.RequestIDFromCtx(ctx)
	if got != "req-123" {
		t.Fatalf("expected req-123, got %s", got)
	}
}

func TestRequestIDFromCtx_Empty(t *testing.T) {
	got := logger.RequestIDFromCtx(context.Background())
	if got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "info"})
	l.Info("test message")

	// force flush by reading file
	data, err := os.ReadFile(filepath.Join(dir, "evey.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !bytes.Contains(data, []byte("test message")) {
		t.Fatalf("log file missing message, got: %s", data)
	}
}
