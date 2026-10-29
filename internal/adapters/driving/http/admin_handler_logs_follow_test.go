package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitForBody(t *testing.T, rec *syncRecorder, want string) {
	t.Helper()
	// Generous: a healthy run gets there in about a second, a loaded -race box
	// needs more.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stream never sent %q, got %q", want, rec.body())
}

// The live tail sends what is appended after it opened, scrubbed like the
// initial lines, and moves to the next file when the log rotates without
// dropping the connection.
func TestHandleStreamLogs_FollowsAppendsRedactedAndRotation(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "evey-2026-05-16.log")
	if err := os.WriteFile(first, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &adminHandler{logDir: dir}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/stream?lines=0", nil).WithContext(ctx)
	rec := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		h.handleStreamLogs(rec, req)
		close(done)
	}()

	// The tail starts at the size the file has when the handler opens it: a
	// line appended before that is history, not news. Its first ping says the
	// size is taken; a fixed sleep lost that race on a loaded box.
	waitForBody(t, rec, ": ping")
	f, err := os.OpenFile(first, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("GET /api/v1/media/m1/stream?token=s3cr3t 200\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	waitForBody(t, rec, "data: GET /api/v1/media/m1/stream?token=[redacted] 200")

	second := filepath.Join(dir, "evey-2026-05-17.log")
	if err = os.WriteFile(second, []byte("after rotation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err = os.Chtimes(second, later, later); err != nil {
		t.Fatal(err)
	}
	waitForBody(t, rec, "data: after rotation")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}
	body := rec.body()
	if strings.Contains(body, "s3cr3t") {
		t.Fatalf("followed line leaked the token: %q", body)
	}
	if strings.Contains(body, "data: before") {
		t.Fatalf("lines=0 must not replay the existing file: %q", body)
	}
}
