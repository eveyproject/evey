package logger_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/pkg/logger"
)

func todayLogFile(dir string) string {
	return filepath.Join(dir, "evey-"+time.Now().Format("2006-01-02")+".log")
}

func TestNew_NotNil(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "info"})
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	l.Info("init")
	if _, err := os.Stat(todayLogFile(dir)); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestNew_DevMode(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug", DevMode: true})
	if l == nil {
		t.Fatal("expected non-nil logger in dev mode")
	}
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

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "test message") {
		t.Fatalf("log file missing message, got: %s", data)
	}
}

func TestStartCleanup_RemovesOldLogs(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	if err := os.WriteFile(filepath.Join(dir, "evey-"+old+".log"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	if err := os.WriteFile(filepath.Join(dir, "evey-"+today+".log"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger.StartCleanup(ctx, dir, 30)
	time.Sleep(100 * time.Millisecond)
	cancel()

	if _, err := os.Stat(filepath.Join(dir, "evey-"+old+".log")); !os.IsNotExist(err) {
		t.Fatal("old log should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "evey-"+today+".log")); err != nil {
		t.Fatal("today log should still exist")
	}
}
