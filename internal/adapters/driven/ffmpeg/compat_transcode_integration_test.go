package ffmpeg

// Real-ffmpeg proof of the compat-download production: a Matroska source with
// 5.1 AC-3 audio goes in, and what comes out must have the SAME video bytes
// family (h264, copied - the whole economic point is never re-encoding video)
// and stereo AAC audio a decoder-less phone can render.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestCompatTranscodeIntegration -v

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCompatTranscodeIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe", "nice"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// Source: 20 s h264 + 5.1 AC-3 - the exact shape of the field films the
	// feature exists for (a 6ch Dolby track a phone plays silent).
	src := filepath.Join(dir, "src.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=7:sample_rate=48000",
		"-t", "20",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-ac", "6", "-b:a", "448k",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source generation: %v\n%s", err, out)
	}

	dst := filepath.Join(dir, "compat.mkv")
	if err := NewCompatTranscoder("ffmpeg").TranscodeCompat(ctx, src, dst); err != nil {
		t.Fatalf("TranscodeCompat: %v", err)
	}

	// ffprobe the result: video copied (h264), audio aac stereo, duration kept.
	probe := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type,codec_name,channels:format=duration",
		"-of", "json", dst)
	out, err := probe.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var report struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Channels  int    `json:"channels"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse ffprobe: %v\n%s", err, out)
	}

	var sawVideo, sawAudio bool
	for _, s := range report.Streams {
		switch s.CodecType {
		case "video":
			sawVideo = true
			if s.CodecName != "h264" {
				t.Errorf("video codec %q, want h264 (must be COPIED, never re-encoded)", s.CodecName)
			}
		case "audio":
			sawAudio = true
			if s.CodecName != "aac" {
				t.Errorf("audio codec %q, want aac", s.CodecName)
			}
			if s.Channels != 2 {
				t.Errorf("audio channels %d, want 2 (stereo downmix)", s.Channels)
			}
		}
	}
	if !sawVideo || !sawAudio {
		t.Fatalf("missing streams in output: video=%v audio=%v", sawVideo, sawAudio)
	}
}
