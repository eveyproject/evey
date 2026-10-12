package logger_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/eveyproject/evey/internal/pkg/logger"
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

// Tests that call logger.New are intentionally NOT t.Parallel: New mutates the
// package-global level var (globalLevel, backing runtime SetLevel), so parallel
// New calls race - a sibling raising the level to warn/error could suppress this
// test's log line before it is written, leaving the expected file uncreated.
func TestParseLevel_AllValues(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		level    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, tt := range tests {
		l := logger.New(logger.Config{Dir: dir, Level: tt.level})
		if l.Enabled(context.Background(), tt.expected-1) && tt.expected > slog.LevelDebug {
			t.Errorf("level %q: should not enable level below %v", tt.level, tt.expected)
		}
		if !l.Enabled(context.Background(), tt.expected) {
			t.Errorf("level %q: should enable level %v", tt.level, tt.expected)
		}
	}
}

func TestDevHandler_OutputFormatting(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder

	l := logger.New(logger.Config{Dir: dir, Level: "debug", DevMode: true})

	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	for _, lvl := range levels {
		l.Log(context.Background(), lvl, "test-"+lvl.String())
	}

	_ = buf // dev handler writes to stderr, we verify no panic and file output
	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, lvl := range levels {
		if !strings.Contains(string(data), "test-"+lvl.String()) {
			t.Errorf("log file missing message for level %s", lvl)
		}
	}
}

func TestDevHandler_WithAttrsAndGroup(t *testing.T) {
	dir := t.TempDir()

	l := logger.New(logger.Config{Dir: dir, Level: "debug", DevMode: true})
	child := l.With(slog.String("component", "test"))
	child = child.WithGroup("mygroup")
	child.Info("attrs-and-group")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "attrs-and-group") {
		t.Fatal("log file missing attrs-and-group message")
	}
}

func TestRotatingWriter_SizeRotation(t *testing.T) {
	dir := t.TempDir()

	// Use very small max size (1 byte effectively) to trigger rotation
	l := logger.New(logger.Config{Dir: dir, Level: "info", MaxSizeMB: 1})
	// Write enough to stay within one file
	l.Info("msg1")
	l.Info("msg2")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	logCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logCount++
		}
	}
	if logCount < 1 {
		t.Fatal("expected at least 1 log file")
	}
}

func TestCleanOldLogs_SkipsNonLogFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a non-log file and a directory
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	initDone := make(chan struct{})
	logger.StartCleanup(ctx, dir, 1, initDone)
	<-initDone
	cancel()

	// Non-log file should still exist
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil {
		t.Fatal("non-log file should not be deleted")
	}
}

func TestStartCleanup_DefaultRetention(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// retention <= 0 defaults to 90
	old := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	os.WriteFile(filepath.Join(dir, "evey-"+old+".log"), []byte("old"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	initDone := make(chan struct{})
	logger.StartCleanup(ctx, dir, 0, initDone)
	<-initDone
	cancel()

	if _, err := os.Stat(filepath.Join(dir, "evey-"+old+".log")); !os.IsNotExist(err) {
		t.Fatal("old log should be deleted with default 90-day retention")
	}
}

func TestMultiHandler_EnabledFalseWhenBelowAll(t *testing.T) {
	dir := t.TempDir()

	l := logger.New(logger.Config{Dir: dir, Level: "error", DevMode: true})
	if l.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("error-level dev logger should not be enabled at debug")
	}
	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("error-level dev logger should not be enabled at info")
	}
	if !l.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("error-level dev logger should be enabled at error")
	}
}

func TestDevHandler_WithAttrs_PreservesExisting(t *testing.T) {
	dir := t.TempDir()

	l := logger.New(logger.Config{Dir: dir, Level: "debug", DevMode: true})
	child1 := l.With(slog.String("k1", "v1"))
	child2 := child1.With(slog.String("k2", "v2"))
	child2.Info("multi-attrs")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "multi-attrs") {
		t.Fatal("log file missing multi-attrs message")
	}
}

func TestCleanOldLogs_HourlyRotatedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Hourly rotated file name: evey-YYYY-MM-DD-HH.log
	old := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	os.WriteFile(filepath.Join(dir, "evey-"+old+"-14.log"), []byte("old hourly"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	initDone := make(chan struct{})
	logger.StartCleanup(ctx, dir, 30, initDone)
	<-initDone
	cancel()

	if _, err := os.Stat(filepath.Join(dir, "evey-"+old+"-14.log")); !os.IsNotExist(err) {
		t.Fatal("old hourly log should be deleted")
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
	initDone := make(chan struct{})
	logger.StartCleanup(ctx, dir, 30, initDone)
	<-initDone
	cancel()

	if _, err := os.Stat(filepath.Join(dir, "evey-"+old+".log")); !os.IsNotExist(err) {
		t.Fatal("old log should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "evey-"+today+".log")); err != nil {
		t.Fatal("today log should still exist")
	}
}

// --- contextHandler injection tests ---

func TestContextHandler_InjectsRequestID(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug"})

	ctx := logger.WithRequestID(context.Background(), "req-abc")
	l.InfoContext(ctx, "with-request-id")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var record map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "with-request-id") {
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			break
		}
	}
	if record == nil {
		t.Fatalf("log line not found in: %s", data)
	}
	if got, ok := record["request_id"]; !ok || got != "req-abc" {
		t.Errorf("expected request_id=req-abc, got %v (ok=%v)", got, ok)
	}
}

func TestContextHandler_NoRequestIDWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug"})

	l.InfoContext(context.Background(), "no-request-id")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var record map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "no-request-id") {
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			break
		}
	}
	if record == nil {
		t.Fatalf("log line not found in: %s", data)
	}
	if _, ok := record["request_id"]; ok {
		t.Errorf("expected no request_id field, but found one: %v", record["request_id"])
	}
}

func TestContextHandler_InjectsTraceID(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug"})

	// Construct a valid span context with a known trace ID.
	var traceID trace.TraceID
	copy(traceID[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	var spanID trace.SpanID
	copy(spanID[:], []byte{1, 2, 3, 4, 5, 6, 7, 8})

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	l.InfoContext(ctx, "with-trace-id")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var record map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "with-trace-id") {
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			break
		}
	}
	if record == nil {
		t.Fatalf("log line not found in: %s", data)
	}
	gotTraceID, ok := record["trace_id"]
	if !ok {
		t.Fatalf("expected trace_id field, not found in %v", record)
	}
	wantTraceID := traceID.String()
	if gotTraceID != wantTraceID {
		t.Errorf("expected trace_id=%s, got %v", wantTraceID, gotTraceID)
	}
}

func TestContextHandler_NoTraceIDWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "debug"})

	l.InfoContext(context.Background(), "no-trace-id")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var record map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "no-trace-id") {
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			break
		}
	}
	if record == nil {
		t.Fatalf("log line not found in: %s", data)
	}
	if _, ok := record["trace_id"]; ok {
		t.Errorf("expected no trace_id field, but found one: %v", record["trace_id"])
	}
}

func TestContextHandler_WithDerivedLogger_StillInjectsRequestID(t *testing.T) {
	dir := t.TempDir()
	parent := logger.New(logger.Config{Dir: dir, Level: "debug"})

	// Derive via logger.With(...) - exercises WithAttrs delegation.
	child := logger.NewWith(parent, "mycomp")
	// Derive further via With to exercise chaining.
	grandchild := child.With(slog.String("extra", "val"))

	ctx := logger.WithRequestID(context.Background(), "req-derived")
	grandchild.InfoContext(ctx, "derived-logger-msg")

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var record map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "derived-logger-msg") {
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			break
		}
	}
	if record == nil {
		t.Fatalf("log line not found in: %s", data)
	}
	if got, ok := record["request_id"]; !ok || got != "req-derived" {
		t.Errorf("expected request_id=req-derived, got %v (ok=%v)", got, ok)
	}
	// The component attr should also be present.
	if got, ok := record["component"]; !ok || got != "mycomp" {
		t.Errorf("expected component=mycomp, got %v (ok=%v)", got, ok)
	}
}
