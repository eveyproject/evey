package mkvgo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// TestOnDemandPackagerIntegration_StatMatchesOpen checks the plan's Stat
// (mkvgo 0.56) against what Open hands out for the same segment - the HTTP
// adapter answers a HEAD and a revalidation from it - and that a name the
// plan does not serve is the port's ErrNoResource, a 404, not a source
// failure. Same opt-in as the other integration tests.
func TestOnDemandPackagerIntegration_StatMatchesOpen(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	src := filepath.Join(t.TempDir(), "src.mkv")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "20",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	pkg := NewOnDemandPackager()
	defer pkg.Close()
	plan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	stater, ok := plan.(apptranscoding.OnDemandStater)
	if !ok {
		t.Fatal("the plan must offer Stat")
	}

	size, etag, ct, known, err := stater.Stat(ctx, "seg00001.m4s")
	if err != nil || !known {
		t.Fatalf("Stat(seg00001.m4s): known=%v err=%v", known, err)
	}
	res, err := plan.(apptranscoding.OnDemandOpener).Open(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("Open(seg00001.m4s): %v", err)
	}
	defer func() { _ = res.Close() }()
	if size != res.Size() || etag != res.ETag() || ct != res.ContentType() {
		t.Fatalf("Stat says %d %s %s, Open hands out %d %s %s", size, etag, ct, res.Size(), res.ETag(), res.ContentType())
	}

	for _, name := range []string{"seg99999.m4s", "nothing.here"} {
		if _, err := plan.(apptranscoding.OnDemandOpener).Open(ctx, name); !errors.Is(err, apptranscoding.ErrNoResource) {
			t.Errorf("Open(%s): err = %v, want ErrNoResource", name, err)
		}
		if _, _, _, _, err := stater.Stat(ctx, name); !errors.Is(err, apptranscoding.ErrNoResource) {
			t.Errorf("Stat(%s): err = %v, want ErrNoResource", name, err)
		}
	}
}
