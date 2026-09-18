package transcoding

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Hot paths of every playback request: `make perf` reports them so a
// regression (a probe in the decision, an allocation storm in the hash) is
// seen before it reaches a box serving twenty players.
func BenchmarkMakePlaybackDecision(b *testing.B) {
	req := ladderRequest(2160)
	for b.Loop() {
		MakePlaybackDecision(context.Background(), req.Streams, "matroska", transcoding.DefaultWebProfile, 0)
	}
}

func BenchmarkShapeLadder(b *testing.B) {
	pr := &PlanResolver{}
	req := ladderRequest(2160)
	req.SourceBitrateKbps = 70000
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	for b.Loop() {
		_ = pr.planFor(context.Background(), req, decision).Ladder
	}
}

func BenchmarkFingerprintHash(b *testing.B) {
	fp := DecisionFingerprint{MediaID: "m", SourcePath: "/f.mkv", HDRTonemap: true, HWTonemap: true, HWFeatures: []string{"tonemap_opencl", "bframes_h264"}, Profiles: []ProfileFingerprint{{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 70000}}}
	for b.Loop() {
		fp.Hash()
	}
}
