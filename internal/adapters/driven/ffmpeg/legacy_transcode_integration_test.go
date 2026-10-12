package ffmpeg

// Legacy-codec (B3) INTEGRATION test: the idet content detector + the
// transcode builder's deinterlace/anamorphic-scale fallbacks against REAL
// ffmpeg on a synthetic interlaced anamorphic source. Proves what the unit
// tests (pure filter strings) can't: that the produced filters actually
// deinterlace the content and correct the display aspect end to end.
//
// OPT-IN like the copy-pipeline test - run via `make transcode-itest` or
// EVEY_TRANSCODE_ITEST=1.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func itestSkip(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// TestInterlaceDetectIntegration: the idet probe must call genuinely
// interlaced content interlaced, and progressive content progressive.
func TestInterlaceDetectIntegration(t *testing.T) {
	ctx, cancel := itestSkip(t)
	defer cancel()
	dir := t.TempDir()
	prober := NewProber("ffprobe")

	// Interlaced source: progressive testsrc (has motion) → `interlace`
	// filter weaves it into fields → mpeg2video. Motion between fields is what
	// idet keys on.
	interlaced := filepath.Join(dir, "interlaced.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=704x576:rate=25,interlace=lowpass=0",
		"-t", "90",
		"-c:v", "mpeg2video", "-flags", "+ilme+ildct", "-b:v", "4000k",
		interlaced)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("interlaced source generation: %v\n%s", err, out)
	}
	got, err := prober.ProbeInterlaceDetect(ctx, interlaced)
	if err != nil {
		t.Fatalf("idet probe (interlaced): %v", err)
	}
	if !got {
		t.Error("idet must detect the interlaced source as interlaced")
	}

	// Progressive source: plain testsrc (idet-clean motion).
	progressive := filepath.Join(dir, "progressive.mkv")
	gen = exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=704x576:rate=25",
		"-t", "90",
		"-c:v", "mpeg2video", "-b:v", "4000k",
		progressive)
	if out, genErr := gen.CombinedOutput(); genErr != nil {
		t.Fatalf("progressive source generation: %v\n%s", genErr, out)
	}
	got, err = prober.ProbeInterlaceDetect(ctx, progressive)
	if err != nil {
		t.Fatalf("idet probe (progressive): %v", err)
	}
	if got {
		t.Error("idet must detect the progressive source as progressive")
	}
}

// TestLegacyTranscodeIntegration: a transcode of an interlaced ANAMORPHIC
// mpeg4 AVI through the real builder must produce a segment that is (a)
// progressive (deinterlaced) and (b) the SQUARE-PIXEL display resolution, not
// the squashed storage shape.
func TestLegacyTranscodeIntegration(t *testing.T) {
	ctx, cancel := itestSkip(t)
	defer cancel()
	dir := t.TempDir()

	// Anamorphic interlaced mpeg4 in an AVI: 704x576 storage, SAR 16:11
	// (DAR ~1.78 = 16:9), interlaced. This is the classic PAL DVD-rip shape.
	src := filepath.Join(dir, "legacy.avi")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=704x576:rate=25,interlace=lowpass=0,setsar=16/11",
		"-t", "30",
		"-c:v", "mpeg4", "-vtag", "XVID", "-flags", "+ilme+ildct", "-b:v", "3000k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("legacy source generation: %v\n%s", err, out)
	}

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	opts := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           src,
		Profiles:           []transcoding.HLSProfile{{Name: "576p", Width: 640, Height: 360, VideoBitrate: 2000, AudioBitrate: 0}},
		VideoDecision:      "transcode",
		AudioTrackIndex:    -1, // video-only source: no audio map
		SubtitleTrackIndex: -1,
		UseFMP4:            true,
		DeinterlaceAll:     true,
		SourceSAR:          "16:11",
	}
	args := BuildFFmpegArgs(opts)
	run := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("transcode run: %v\nargs: %s\n%s", err, strings.Join(args, " "), out)
	}

	clip := filepath.Join(dir, "served.mp4")
	initB, err := os.ReadFile(filepath.Join(outDir, "stream_0_init.mp4"))
	if err != nil {
		t.Fatalf("read init (transcode produced no output?): %v", err)
	}
	segB, err := os.ReadFile(filepath.Join(outDir, "stream_0_000.m4s"))
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	if wErr := os.WriteFile(clip, append(initB, segB...), 0o644); wErr != nil {
		t.Fatal(wErr)
	}

	// Display geometry: 704x576 SAR 16:11 → DAR 1024:576 = 16:9. Target width
	// 640 ⇒ 640x360 square pixels. The storage-ratio scale would have given
	// 640x523-ish; assert the corrected height and square SAR.
	probeOut, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,sample_aspect_ratio",
		"-of", "csv=p=0", clip).Output()
	if err != nil {
		t.Fatalf("ffprobe served clip: %v", err)
	}
	got := strings.TrimSpace(string(probeOut))
	if !strings.Contains(got, "640,360") {
		t.Errorf("served resolution = %q, want 640x360 (anamorphic display aspect, not squashed storage)", got)
	}
	if !strings.Contains(got, "1:1") {
		t.Errorf("served SAR = %q, want square pixels (1:1)", got)
	}

	// Deinterlace: idet on the OUTPUT must now read progressive - bwdif
	// deint=all cleared the fields.
	prober := NewProber("ffprobe")
	stillInterlaced, err := prober.ProbeInterlaceDetect(ctx, clip)
	if err != nil {
		// A short single segment may not survive -ss 60 inside the probe; the
		// retry-from-start path covers it. A hard error here is a real failure.
		t.Fatalf("idet on output: %v", err)
	}
	if stillInterlaced {
		t.Error("the transcoded output must be progressive - bwdif deint=all should have cleared the combing")
	}
}
