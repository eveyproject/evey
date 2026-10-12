package mkvgo_test

// Streaming-readiness validation against a real, well-formed ffmpeg MKV:
// it carries a Cues index and DefaultDuration, so ValidateStreaming must report
// no ERROR-severity findings (warnings like "no language" are fine).
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run Integration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

func TestValidateStreamingIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "15", "-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-metadata:s:a:0", "language=eng", "-metadata:s:v:0", "language=eng",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	a := mkvgoadapter.New()
	issues, err := a.ValidateStreaming(ctx, src)
	if err != nil {
		t.Fatalf("ValidateStreaming: %v", err)
	}
	for _, iss := range issues {
		t.Logf("%s: %s", iss.Severity, iss.Message)
		if iss.Severity == mkv.ValidationError {
			t.Errorf("unexpected ERROR on a well-formed indexed MKV: %s", iss.Message)
		}
	}
}
