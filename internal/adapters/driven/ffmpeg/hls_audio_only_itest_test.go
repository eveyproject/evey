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

	"github.com/eveyproject/evey/internal/domain/transcoding"
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

// A click at 7.500 s of the source must land at 7.500 in a copy rendition
// seeked to 7.300, whose fragments the rebase moves by the target. The source
// has a keyframe every 5 s, the Matroska's only index points: the demuxer seeks
// to 5.0 and, without dropping what precedes the target, the copy ran from
// there and put the click 2.3 s late.
func TestSeekedAudioCopyLandsOnTheTargetIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	src := filepath.Join(t.TempDir(), "keyframes.mkv")
	click := "aevalsrc=if(gte(t\\,7.5)*lt(t\\,7.502)\\,0.9\\,0):s=48000:d=20:c=stereo"
	if b, err := exec.Command("ffmpeg", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=s=160x90:r=24:d=20", "-f", "lavfi", "-i", click,
		"-map", "0", "-map", "1", "-c:v", "mpeg2video", "-g", "120", "-sc_threshold", "0",
		"-c:a", "ac3", "-b:a", "192k", src).CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, b)
	}
	const target = 7.3
	dir := t.TempDir()
	args := BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{FilePath: src, OutputDir: dir, AudioCodec: "copy", SourceAudioCodec: "ac3", MaxAudioChannels: 2, SourceAudioChannels: 2, StartSec: target, StartNumber: 2})
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("rendition: %v %s", err, b)
	}
	all, _ := os.ReadFile(filepath.Join(dir, "audio_0_init.mp4"))
	for i := 2; i < 5; i++ {
		seg, err := os.ReadFile(filepath.Join(dir, "audio_0_00"+strconv.Itoa(i)+".m4s"))
		if err != nil {
			t.Fatalf("segment %d: %v", i, err)
		}
		all = append(all, seg...)
	}
	f := filepath.Join(dir, "all.mp4")
	if err := os.WriteFile(f, all, 0o644); err != nil {
		t.Fatal(err)
	}
	// The fragments' own time from the target, before the rebase adds it.
	p0, _ := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "frame=pts_time", "-read_intervals", "%+#1", "-of", "csv=p=0", f).Output()
	start, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(string(p0)), ","), 64)
	if err != nil {
		t.Fatalf("first frame pts: %v", err)
	}
	pcm, _ := exec.Command("ffmpeg", "-v", "error", "-i", f, "-ac", "1", "-ar", "48000", "-f", "s16le", "-").Output()
	for i := 0; i+1 < len(pcm); i += 2 {
		if v := int16(binary.LittleEndian.Uint16(pcm[i:])); v > 3000 || v < -3000 {
			at := target + start + float64(i/2)/48000
			// One AC-3 frame (32 ms) of slack: the copy starts on a frame.
			if d := math.Abs(at-7.5) * 1000; d > 32 {
				t.Errorf("the click lands at %.3f s, %.0f ms off 7.500", at, d)
			}
			return
		}
	}
	t.Fatal("no click in the rendition")
}
