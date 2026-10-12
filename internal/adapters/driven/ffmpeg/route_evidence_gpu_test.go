package ffmpeg

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The calibration produces per-route cost evidence on the real GPU: warm,
// single-session, route-faithful. Gated on EVEY_GPU_TEST=nvenc, run in the
// image with the GPU attached.
func TestGPU_RouteEvidenceIsMeasured(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	r := NewBenchmarkRunner(BenchmarkOpts{FfmpegPath: ffmpeg, SamplePath: filepath.Join(t.TempDir(), "sample.hevc"), SingleVariant: true}, nil)
	cap, err := r.Run(context.Background(), transcoding.HWPreference{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cap.RouteSamples) == 0 {
		t.Fatal("no route evidence produced")
	}
	seen := map[transcoding.RouteCostKey]bool{}
	for _, s := range cap.RouteSamples {
		if s.FPS <= 0 || s.SourceFPS <= 0 {
			t.Errorf("bad sample %+v", s)
		}
		seen[s.Key] = true
		t.Logf("%s/%s/%s %dp: %.0f fps, %.2f cpu cores, swdec=%v", s.Key.Backend, s.Key.Encoder, s.Key.Route, s.Height, s.FPS, s.CPUCores, s.SoftwareDecode)
	}
	// The zero-copy SDR route on the GPU and the software x264/x265 routes are
	// always measured; the GPU SDR must be there on an NVENC host.
	if !seen[transcoding.RouteCostKeyFor(transcoding.NVENC, "h264_nvenc", transcoding.RouteCostSDR)] {
		t.Error("the NVENC SDR route was not measured")
	}
	if !seen[transcoding.RouteCostKeyFor("", "libx264", transcoding.RouteCostSDR)] {
		t.Error("the software x264 route was not measured")
	}
	// A GPU zero-copy session must record NO software decode; a decoupled one must.
	for _, s := range cap.RouteSamples {
		if s.Key.Backend == "nvenc" && s.Key.Route == transcoding.RouteCostSDR && s.SoftwareDecode {
			t.Error("a zero-copy NVENC SDR session must not be flagged software-decode")
		}
	}
}
