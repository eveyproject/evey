package ffmpeg

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// TestAudioPassthroughIntegration proves, on real bytes, that the audio-only
// copy rendition carries the SOURCE codec the master advertises for the two
// opt-in passthroughs (mp3, opus) and for AAC: the init + first segment are
// muxed by the real ffmpeg and read back by the real ffprobe. The class of
// bug this pins: a master that says `CODECS="opus"` over segments that came
// out AAC (measured 2026-09-29, before the audio playlist re-baked the
// opt-in). Run with `make transcode-itest`.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ -run TestAudioPassthroughIntegration -v
func TestAudioPassthroughIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()

	cases := []struct {
		name, encoder, ext, codec, entry string
	}{
		{"opus in matroska", "libopus", "mkv", "opus", "Opus"},
		{"mp3 in matroska", "libmp3lame", "mkv", "mp3", "mp4a"},
		{"aac in mp4", "aac", "mp4", "aac", "mp4a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := filepath.Join(dir, "src_"+c.codec+"."+c.ext)
			gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
				"-f", "lavfi", "-i", "testsrc2=size=64x64:rate=24",
				"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
				"-t", "8", "-c:v", "libx264", "-preset", "ultrafast", "-b:v", "60k", "-pix_fmt", "yuv420p",
				"-c:a", c.encoder, "-b:a", "64k", src)
			if out, err := gen.CombinedOutput(); err != nil {
				t.Fatalf("generate source: %v\n%s", err, out)
			}
			out := t.TempDir()
			args := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
				FilePath: src, OutputDir: out, SourceTrackIdx: 0, AudioCodec: "copy", SourceAudioCodec: c.codec,
			})
			if res, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("copy rendition: %v\n%s", err, res)
			}
			segs, _ := filepath.Glob(filepath.Join(out, "audio_0_[0-9]*.m4s"))
			if len(segs) == 0 {
				t.Fatalf("no audio segment written under %s", out)
			}
			init, _ := filepath.Glob(filepath.Join(out, "audio_0_init.mp4"))
			if len(init) == 0 {
				t.Fatalf("no audio init written under %s", out)
			}
			// ffprobe reads a fragment only after its init: concatenate the two.
			joined := filepath.Join(out, "probe.mp4")
			initBytes, err := os.ReadFile(init[0])
			if err != nil {
				t.Fatal(err)
			}
			segBytes, err := os.ReadFile(segs[0])
			if err != nil {
				t.Fatal(err)
			}
			if werr := os.WriteFile(joined, append(initBytes, segBytes...), 0o644); werr != nil {
				t.Fatal(werr)
			}
			probe, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_name,codec_tag_string",
				"-of", "json", joined).Output()
			if err != nil {
				t.Fatalf("ffprobe: %v", err)
			}
			var parsed struct {
				Streams []struct {
					Codec string `json:"codec_name"`
					Tag   string `json:"codec_tag_string"`
				} `json:"streams"`
			}
			if err := json.Unmarshal(probe, &parsed); err != nil || len(parsed.Streams) == 0 {
				t.Fatalf("ffprobe output unreadable: %v\n%s", err, probe)
			}
			if got := parsed.Streams[0].Codec; got != c.codec {
				t.Fatalf("served segment carries %q, the master would advertise %q", got, c.codec)
			}
			if got := parsed.Streams[0].Tag; got != c.entry {
				t.Fatalf("sample entry %q, want %q", got, c.entry)
			}
		})
	}
}
