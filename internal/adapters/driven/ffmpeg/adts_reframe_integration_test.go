package ffmpeg

// ADTS re-framing INTEGRATION test: an AAC track copied out of a real
// MPEG-TS lands in fMP4 segments only with -bsf:a aac_adtstoasc. Measured
// on the dev server (2026-09-25, ffmpeg 7.1.4): without it the rendition
// died on its first segment ("Terminating thread with return code -1
// (Operation not permitted)"), every segment request answered 503 and the
// orchestrator respawned it at each one.
//
// OPT-IN like the other transcode itests: run via `make transcode-itest` or
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestADTSReframeIntegration -v

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestADTSReframeIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	mkv := genGridSource(t, ctx, dir, "25")
	ts := filepath.Join(dir, "src.ts")
	remux := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-i", mkv, "-c", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "mpegts", ts)
	if out, err := remux.CombinedOutput(); err != nil {
		t.Fatalf("remux to MPEG-TS: %v\n%s", err, out)
	}

	run := func(t *testing.T, args []string) ([]byte, error) {
		t.Helper()
		return exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput()
	}

	// The separate rendition, copied: the case that failed in production.
	rend := t.TempDir()
	out, err := run(t, BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: ts, OutputDir: rend, SourceTrackIdx: 0, AudioCodec: "copy", SourceAudioCodec: "aac",
	}))
	if err != nil {
		t.Fatalf("copied rendition out of an MPEG-TS: %v\n%s", err, out)
	}
	if segs, _ := filepath.Glob(filepath.Join(rend, "audio_0_[0-9]*.m4s")); len(segs) < 4 {
		t.Fatalf("copied rendition produced %d segments, want the whole 12 s", len(segs))
	}

	// The muxed rung with the audio copied beside a transcode.
	muxed := t.TempDir()
	out, err = run(t, BuildFFmpegArgs(transcoding.HLSProcessOpts{
		FilePath: ts, OutputDir: muxed, VideoDecision: "transcode", AudioDecision: "copy", AudioCodec: "aac",
		AudioTrackIndex: 0, SubtitleTrackIndex: -1, UseFMP4: true,
		Profiles: []transcoding.HLSProfile{{Name: "180p", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96}},
	}))
	if err != nil {
		t.Fatalf("muxed rung copying the MPEG-TS audio: %v\n%s", err, out)
	}
	if segs, _ := filepath.Glob(filepath.Join(muxed, "stream_0_[0-9]*.m4s")); len(segs) < 4 {
		t.Fatalf("muxed rung produced %d segments, want the whole 12 s", len(segs))
	}

	// And the Matroska source is untouched by the filter: same args, no
	// re-framing, still fine.
	plain := t.TempDir()
	out, err = run(t, BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: mkv, OutputDir: plain, SourceTrackIdx: 0, AudioCodec: "copy", SourceAudioCodec: "aac",
	}))
	if err != nil {
		t.Fatalf("copied rendition out of a Matroska: %v\n%s", err, out)
	}
}
