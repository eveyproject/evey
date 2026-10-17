package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/procfork"
)

// The ffmpeg subtitle fallbacks book a memory admission before they start
// ffmpeg. A start that fails - an expired extraction deadline, a client gone,
// a fork refused under memory pressure - must hand the booking back: one that
// outlives its process is subtracted from free memory for the life of the
// server, and a few of them refuse every transcode on an idle host (the
// procfork leak of 2026-10-17, same class).
func TestExtractSubtitleWithFFmpeg_FailedStartReleasesItsAdmission(t *testing.T) {
	before := procfork.Reserved()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extractSubtitleWithFFmpeg(ctx, "/nonexistent.ts", 0); err == nil {
		t.Fatal("a cancelled extraction must fail")
	}
	if got := procfork.Reserved(); got != before {
		t.Fatalf("admission still booked after a failed start: %d bytes, was %d", got, before)
	}
}

func TestExternalSubtitleFFmpeg_FailedStartReleasesItsAdmission(t *testing.T) {
	dir := t.TempDir()
	// Not a format mkvgo converts, so it goes to the ffmpeg fork directly.
	path := filepath.Join(dir, "subs.smi")
	if err := os.WriteFile(path, []byte("<SAMI></SAMI>"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := procfork.Reserved()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/sub.vtt", nil).WithContext(ctx)
	serveExternalSubtitle(httptest.NewRecorder(), req, path, dir, "")
	if got := procfork.Reserved(); got != before {
		t.Fatalf("admission still booked after a failed start: %d bytes, was %d", got, before)
	}
}
