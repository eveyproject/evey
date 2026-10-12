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

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
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

	// A GENUINELY variable source: keep an irregular subset of frames (n%3==0 OR
	// n%7==0) so the gaps between kept frames vary (1, 2, 3 source-frame
	// periods), muxed with -fps_mode vfr so those real, uneven presentation
	// timestamps land in the container instead of being resampled to a constant
	// grid. mkvgo >= v0.25.1 classifies VFR from the FRACTION of out-of-tolerance
	// deltas (outlier-robust: a couple of dropped frames on an otherwise constant
	// track stay "cfr"), so the fixture must vary a meaningful share of its frame
	// durations - here ~46% - not just a lone glitch. -bf 0 keeps the block
	// timecodes measuring frame durations, not B-frame reorder jitter.
	vfr := filepath.Join(dir, "vfr.mkv")
	genVFR := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "6", "-vf", "select='not(mod(n\\,3))+not(mod(n\\,7))'",
		"-c:v", "libx264", "-bf", "0", "-fps_mode", "vfr", "-pix_fmt", "yuv420p",
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

// A structurally broken file must settle as "unknown", NOT as an error: the
// caller stamps unknowns and stops asking, whereas an error is retried on every
// sweep - which is how a handful of corrupt files produced a permanent stream
// of "analysis failed" warnings and a full-file walk each time.
func TestAnalyzeFrameRateMode_UnreadableSourceIsUnknownNotAnError(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.mkv")
	// Valid EBML magic so it gets past the container sniff, garbage after it.
	if err := os.WriteFile(broken, []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF}, 0o644); err != nil {
		t.Fatalf("write broken fixture: %v", err)
	}

	a := mkvgoadapter.New()
	mode, err := a.AnalyzeFrameRateMode(context.Background(), broken)
	if err != nil {
		t.Fatalf("a broken file must not surface as an error to retry: %v", err)
	}
	if mode != "" {
		t.Errorf("mode = %q, want \"\" (unknown)", mode)
	}
}

// A file we cannot even open IS an error: that can succeed later (a mount that
// came back, a permission that got fixed), so it must stay retryable.
func TestAnalyzeFrameRateMode_MissingSourceStaysAnError(t *testing.T) {
	a := mkvgoadapter.New()
	if _, err := a.AnalyzeFrameRateMode(context.Background(), filepath.Join(t.TempDir(), "nope.mkv")); err == nil {
		t.Error("an unavailable source must remain retryable, not be settled as unknown")
	}
}
