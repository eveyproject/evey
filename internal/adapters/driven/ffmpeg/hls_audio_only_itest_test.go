package ffmpeg

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// A click at 1.000 s of the source must land at 1.000 + VideoCompositionDelay
// in the served rendition, whatever the shift and the encoder. Before the
// priming was dropped, an unshifted rendition (every mkvgo copy rung) put it
// 27 ms (native AAC) to 48 ms (libfdk_aac) late, while a shifted one was right.
func TestAudioRenditionLandsOnTheVideoTimelineIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	src := filepath.Join(t.TempDir(), "click.mka")
	click := "aevalsrc=if(gte(t\\,1)*lt(t\\,1.002)\\,0.9\\,0):s=44100:d=4:c=5.1"
	if b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", click, "-c:a", "ac3", "-b:a", "448k", src).CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, b)
	}
	clickAt := func(encoder string, delay float64) float64 {
		dir := t.TempDir()
		args := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{FilePath: src, OutputDir: dir, AudioCodec: "aac", AudioEncoder: encoder, AudioBitrate: 192, MaxAudioChannels: 2, SourceAudioChannels: 6, VideoCompositionDelay: delay})
		if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", encoder, err, b)
		}
		all, _ := os.ReadFile(filepath.Join(dir, "audio_0_init.mp4"))
		for i := range 3 {
			seg, _ := os.ReadFile(filepath.Join(dir, "audio_0_00"+strconv.Itoa(i)+".m4s"))
			all = append(all, seg...)
		}
		f := filepath.Join(dir, "all.mp4")
		if err := os.WriteFile(f, all, 0o644); err != nil {
			t.Fatal(err)
		}
		p0, _ := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "frame=pts_time", "-read_intervals", "%+#1", "-of", "csv=p=0", f).Output()
		start, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(string(p0)), ","), 64)
		if err != nil {
			t.Fatalf("first frame pts: %v", err)
		}
		pcm, _ := exec.Command("ffmpeg", "-v", "error", "-i", f, "-ac", "1", "-ar", "48000", "-f", "s16le", "-").Output()
		for i := 0; i+1 < len(pcm); i += 2 {
			if v := int16(binary.LittleEndian.Uint16(pcm[i:])); v > 3000 || v < -3000 {
				return start + float64(i/2)/48000 - delay
			}
		}
		t.Fatalf("%s: no click in the output", encoder)
		return 0
	}
	for _, enc := range []string{NativeAACEncoder, PreferredAACEncoder} {
		if exec.Command("ffmpeg", "-hide_banner", "-h", "encoder="+enc).Run() != nil {
			t.Logf("%s not in this ffmpeg, skipped", enc)
			continue
		}
		shifted, unshifted := clickAt(enc, 0.125), clickAt(enc, 0)
		if d := math.Abs(unshifted-shifted) * 1000; d > 3 {
			t.Errorf("%s: an unshifted rendition puts the click %.1f ms off a shifted one", enc, d)
		}
	}
}
