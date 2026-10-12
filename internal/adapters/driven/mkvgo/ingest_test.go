package mkvgo_test

// Fast unit coverage for the target-default plumbing lives here; the real
// serving-plan decision against ffmpeg-generated files is an integration
// test (real mkvgo ops.Ingest, real files), gated like the other real-file
// tests in this package.
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
	"github.com/eveyproject/evey/internal/domain/library"
)

func TestPlan_NonExistentFile_Errors(t *testing.T) {
	a := mkvgoadapter.New()
	_, err := a.Plan(context.Background(), "/no/such/file.mkv", "")
	if err == nil {
		t.Fatal("Plan on a non-existent file must error, got nil")
	}
}

func TestPlanIntegration_DirectPlaySource(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "3",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	a := mkvgoadapter.New()
	plan, err := a.Plan(ctx, src, "")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Target != "mse-generic" {
		t.Errorf("Target = %q, want the default mse-generic (empty target passed)", plan.Target)
	}
	if plan.Strategy == "" {
		t.Errorf("Strategy must not be empty")
	}
	if plan.Strategy == library.ServingStrategyRemuxHLS && !plan.NeedsReindex {
		// ffmpeg's matroska muxer writes Cues by default, so this source
		// should already carry a head-discoverable index - document the
		// expectation rather than assert it strictly (mkvgo's own
		// Playability heuristics own the exact strategy pick).
		t.Logf("remux-hls source reports an existing seek index (NeedsReindex=false)")
	}
}
