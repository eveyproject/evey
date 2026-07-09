package mkvgo_test

// Fast unit coverage for the non-EBML gate lives here; the real CFR/VFR
// classification against ffmpeg-generated MKVs is an integration test (real
// mkvgo ops.Analyze, real files), gated like the other real-file tests in
// this package.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run Integration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
)

func TestAnalyzeFrameRateMode_NonEBMLContainerIsUnknown(t *testing.T) {
	a := mkvgoadapter.New()
	mode, err := a.AnalyzeFrameRateMode(context.Background(), "/media/movie.mp4")
	if err != nil {
		t.Fatalf("AnalyzeFrameRateMode: %v", err)
	}
	if mode != "" {
		t.Errorf("mode = %q, want \"\" (unknown) for a non-EBML container", mode)
	}
}

func TestAnalyzeFrameRateModeIntegration_CFRAndVFR(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dir := t.TempDir()
	a := mkvgoadapter.New()

	// rate=25 gives an exact 40ms frame period (no millisecond rounding
	// jitter against Matroska's 1ms TimecodeScale) - the CFR case must be
	// unambiguous. -bf 0 disables B-frames: the block-header walk reads
	// timecodes in FILE (decode) order, and B-frame reordering would put
	// presentation-order timecodes out of sequence between consecutive
	// blocks even on a genuinely constant-rate source.
	cfr := filepath.Join(dir, "cfr.mkv")
	genCFR := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-t", "5", "-c:v", "libx264", "-bf", "0", "-fps_mode", "cfr", "-pix_fmt", "yuv420p",
		cfr)
	if out, err := genCFR.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg CFR source generation failed: %v\n%s", err, out)
	}

	mode, err := a.AnalyzeFrameRateMode(ctx, cfr)
	if err != nil {
		t.Fatalf("AnalyzeFrameRateMode(cfr): %v", err)
	}
	if mode != "cfr" {
		t.Errorf("cfr source classified as %q, want %q", mode, "cfr")
	}

	// A VFR source: frame count varies over time (a select filter that drops
	// frames irregularly), muxed with -fps_mode vfr so the real, uneven
	// presentation timestamps land in the container instead of being
	// resampled back to a constant grid.
	vfr := filepath.Join(dir, "vfr.mkv")
	genVFR := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "5", "-vf", "select='mod(n\\,3)+1'", "-vsync", "0",
		"-c:v", "libx264", "-fps_mode", "vfr", "-pix_fmt", "yuv420p",
		vfr)
	if out, genErr := genVFR.CombinedOutput(); genErr != nil {
		t.Fatalf("ffmpeg VFR source generation failed: %v\n%s", genErr, out)
	}

	mode, err = a.AnalyzeFrameRateMode(ctx, vfr)
	if err != nil {
		t.Fatalf("AnalyzeFrameRateMode(vfr): %v", err)
	}
	if mode != "vfr" {
		t.Errorf("vfr source classified as %q, want %q", mode, "vfr")
	}
}
