package ffmpeg

// A seeked session writes into a private directory and its rebase pump
// publishes each fragment into the cache. Done() is what the orchestrators
// reap on: it tears the session down and fails every request still waiting
// on it. It used to fire with the process exit, alongside the pump's last
// sweep, so the final segment of a session run to the end of the file was
// answered 503 while it was being published (and 200 on the next request).
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestRebaseDoneAfterLastSegmentIntegration -v

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestRebaseDoneAfterLastSegmentIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	src := filepath.Join(t.TempDir(), "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-t", "15", "-i", "testsrc2=size=160x90:rate=25",
		"-f", "lavfi", "-t", "15", "-i", "sine=frequency=440:sample_rate=48000",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "ac3", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v\n%s", err, out)
	}
	grid := transcoding.GridForFrameRate(25)
	const durationMs = 15000
	last := grid.SegmentCount(durationMs) - 1
	start := last - 2

	for run := range 5 {
		t.Run(fmt.Sprintf("audio %d", run), func(t *testing.T) {
			dir := t.TempDir()
			h, err := NewHLSTranscoder().StartAudioOnly(ctx, transcoding.AudioProcessOpts{
				FilePath: src, OutputDir: dir, SourceTrackIdx: 0, AudioCodec: "aac", AudioBitrate: 96,
				MaxAudioChannels: 2, Grid: grid, StartNumber: start, StartSec: grid.SegmentStartSeconds(start),
			})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			<-h.Done()
			for n := start; n <= last; n++ {
				if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("audio_0_%03d.m4s", n))); err != nil {
					t.Fatalf("Done() fired before segment %d was published: %v", n, err)
				}
			}
		})
		t.Run(fmt.Sprintf("video %d", run), func(t *testing.T) {
			dir := t.TempDir()
			h, err := NewHLSTranscoder().Start(ctx, transcoding.HLSProcessOpts{
				FilePath: src, OutputDir: dir, VideoDecision: "transcode", AudioDecision: "transcode",
				AudioTrackIndex: -1, SubtitleTrackIndex: -1, UseFMP4: true, Grid: grid, DeclaredDurationMs: durationMs,
				StartSec: grid.SegmentStartSeconds(start), StartNumber: start,
				Profiles: []transcoding.HLSProfile{{Name: "90p-itest", Width: 160, Height: 90, VideoBitrate: 200, AudioBitrate: 96}},
			})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			<-h.Done()
			for n := start; n <= last; n++ {
				if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("stream_0_%03d.m4s", n))); err != nil {
					t.Fatalf("Done() fired before segment %d was published: %v", n, err)
				}
			}
		})
	}
}
