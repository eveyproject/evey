package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestEncoderReorderProbeArgs(t *testing.T) {
	sw := strings.Join(encoderReorderProbeArgs(EncoderSpec{Codec: "h264"}, "/tmp/p.mp4"), " ")
	for _, want := range []string{"-c:v libx264", "-preset veryfast", "-x264opts", "-crf 20", "-f mp4"} {
		if !strings.Contains(sw, want) {
			t.Errorf("software probe must carry the production args (%s): %s", want, sw)
		}
	}
	if strings.Contains(sw, "hwupload") {
		t.Errorf("software probe must not upload: %s", sw)
	}
	hw := strings.Join(encoderReorderProbeArgs(EncoderSpec{Codec: "h264", Accel: transcoding.NVENC, Features: []string{"bframes"}}, "/tmp/p.mp4"), " ")
	for _, want := range []string{"-init_hw_device cuda=cu", "hwupload_cuda", "-c:v h264_nvenc", "-bf 3"} {
		if !strings.Contains(hw, want) {
			t.Errorf("nvenc probe must upload and carry the probed features (%s): %s", want, hw)
		}
	}
	ba := EncoderSpec{Codec: "h264", Features: []string{"b", "a"}}
	ab := EncoderSpec{Codec: "h264", Features: []string{"a", "b"}}
	if ba.key() != ab.key() {
		t.Error("the cache key must not depend on feature order")
	}
}

// The production libx264 configuration (veryfast, B-pyramid) delays presentation
// by two frames: the number the audio rendition is shifted by.
func TestEncoderReorderFramesIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest` - needs the bundled ffmpeg)")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	n, ok := BundledEncoderReorderFrames(context.Background(), EncoderSpec{Codec: "h264"})
	if !ok {
		t.Fatal("probe failed")
	}
	if n != 2 {
		t.Fatalf("libx264 veryfast reorder = %d frames, want 2", n)
	}
	if again, _ := BundledEncoderReorderFrames(context.Background(), EncoderSpec{Codec: "h264"}); again != n {
		t.Fatal("cached answer must match")
	}
}

// The delay is read off the first packet's presentation offset. has_b_frames
// was read before, and NVENC (-bf 3 -b_ref_mode middle) reports 1 there while
// presenting its first frame 3 frames late - the audio then ran 83 ms early.
func TestPresentationDelayFrames(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
	}{
		{"nvenc bf3 middle, measured", `{"packets":[{"pts_time":"0.125000","dts_time":"0.000000"}]}`, 3},
		{"libx264 veryfast", `{"packets":[{"pts_time":"0.083333","dts_time":"0.000000"}]}`, 2},
		{"no b-frames", `{"packets":[{"pts_time":"0.000000","dts_time":"0.000000"}]}`, 0},
	}
	for _, c := range cases {
		got, err := presentationDelayFrames([]byte(c.json))
		if err != nil || got != c.want {
			t.Errorf("%s: got %d (%v), want %d", c.name, got, err, c.want)
		}
	}
	if _, err := presentationDelayFrames([]byte(`{"packets":[]}`)); err == nil {
		t.Error("an output with no packet must fail the probe")
	}
}
