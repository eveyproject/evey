package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLineCount_DefaultWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if got := parseLineCount(req, 200, 2000); got != 200 {
		t.Fatalf("want 200, got %d", got)
	}
}

func TestCurrentLogPath_PicksNewestEveyLog(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "evey-2026-05-15.log")
	newer := filepath.Join(dir, "evey-2026-05-16-10.log")
	if err := os.WriteFile(older, []byte("o"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := currentLogPath(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != newer {
		t.Fatalf("want %s, got %s", newer, got)
	}
}

func TestCurrentLogPath_IgnoresNonEveyAndDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evey-2026-05-16.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "evey-fake.log"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := currentLogPath(dir); err == nil {
		t.Fatal("expected ErrNotExist when no matching file")
	}
}

func TestCurrentLogPath_MissingDir(t *testing.T) {
	if _, err := currentLogPath("/nonexistent/log/dir"); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestParseLineCount_DefaultOnInvalid(t *testing.T) {
	for _, q := range []string{"abc", "-1", "99999"} {
		req := httptest.NewRequest(http.MethodGet, "/x?lines="+q, nil)
		if got := parseLineCount(req, 42, 1000); got != 42 {
			t.Fatalf("q=%q: want 42, got %d", q, got)
		}
	}
}

func TestParseLineCount_ValidValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?lines=500", nil)
	if got := parseLineCount(req, 200, 2000); got != 500 {
		t.Fatalf("want 500, got %d", got)
	}
}

func TestReadNewLogBytes_NoNewData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evey.log")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf, size, err := readNewLogBytes(path, 5, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if buf != nil {
		t.Fatalf("want nil buf, got %q", buf)
	}
	if size != 5 {
		t.Fatalf("want size 5, got %d", size)
	}
}

func TestReadNewLogBytes_FileTruncatedResetsOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evey.log")
	if err := os.WriteFile(path, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	// lastSize larger than file → treat as truncation; should re-read from 0.
	buf, size, err := readNewLogBytes(path, 999, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "short" {
		t.Fatalf("want 'short', got %q", buf)
	}
	if size != 5 {
		t.Fatalf("want 5, got %d", size)
	}
}

func TestReadNewLogBytes_AppendsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evey.log")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf, size, err := readNewLogBytes(path, 6, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Fatalf("want 'world', got %q", buf)
	}
	if size != 11 {
		t.Fatalf("want 11, got %d", size)
	}
}

func TestInitialFileSize_MissingReturnsZero(t *testing.T) {
	if got := initialFileSize("/nonexistent/path.log"); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestInitialFileSize_Exists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evey.log")
	if err := os.WriteFile(path, []byte("1234"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := initialFileSize(path); got != 4 {
		t.Fatalf("want 4, got %d", got)
	}
}

func TestHandleGetLogs_FileMissing(t *testing.T) {
	h := &adminHandler{logDir: filepath.Join(t.TempDir(), "nope")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	h.handleGetLogs(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestHandleGetLogs_ReturnsLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "evey-2026-05-16.log"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &adminHandler{logDir: dir}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?lines=2", nil)
	h.handleGetLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want application/json, got %q", ct)
	}
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Lines) != 2 || resp.Lines[0] != "beta" || resp.Lines[1] != "gamma" {
		t.Fatalf("unexpected lines: %v", resp.Lines)
	}
}

func TestHandleStreamLogs_StreamingUnsupported(t *testing.T) {
	h := &adminHandler{logDir: t.TempDir()}
	w := &nonFlusherWriter{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/stream", nil)
	h.handleStreamLogs(w, req)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.code)
	}
	if !strings.Contains(w.buf.String(), "streaming unsupported") {
		t.Fatalf("expected 'streaming unsupported', got %q", w.buf.String())
	}
}

func TestHandleStreamLogs_InitialLinesAndContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evey-2026-05-16.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &adminHandler{logDir: dir}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/stream?lines=10", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		h.handleStreamLogs(rec, req)
		close(done)
	}()

	// Wait for the initial lines to appear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := rec.body()
		if strings.Contains(body, "one") && strings.Contains(body, "two") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	body := rec.body()
	if !strings.Contains(body, "data: one") || !strings.Contains(body, "data: two") {
		t.Fatalf("expected initial lines in body, got %q", body)
	}
	if ct := rec.getHeader("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("want text/event-stream, got %q", ct)
	}
}
