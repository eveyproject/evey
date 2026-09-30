package transcoding

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The reason list tells the truth about the picture: a plan whose bytes are
// tone-mapped says so, and a plan that says so tone-maps.
//
// Stream info reads TranscodeReasons, the encoder reads the fingerprint. They
// used to be decided in two places: the decision judged the HDR keepable on a
// GPU that encodes HDR10, then the route spilled to the CPU (per-title block,
// full GPU pool) and x264 tone-mapped to SDR while the reasons still read
// "bitrate_exceeded" alone. Every route the golden matrix exercises is
// checked, both directions.
func TestToneMappedRouteReportsItsReason(t *testing.T) {
	t.Parallel()
	for _, sc := range routeGoldenScenarios() {
		pr, req := sc.build()
		plan, err := pr.Resolve(context.Background(), req)
		if err != nil {
			continue
		}
		reported := slices.Contains(plan.Decision.TranscodeReasons, "hdr_tonemapping_needed")
		switch {
		case plan.Fingerprint.HDRTonemap && !reported:
			t.Errorf("%s: tone-mapped route reported without hdr_tonemapping_needed (reasons %v)", sc.name, plan.Decision.TranscodeReasons)
		case reported && !plan.Fingerprint.HDRTonemap && plan.Decision.VideoDecision == "transcode":
			t.Errorf("%s: hdr_tonemapping_needed reported on a route that does not tone-map (reasons %v)", sc.name, plan.Decision.TranscodeReasons)
		}
	}
}

// Stream info answers before playback with Reported, which knows the route but
// not the pool: every tone-map the route alone decides (a GPU that cannot serve
// the source, a blocked title) is announced, and nothing is announced that the
// served plan does not do. A spill on a full pool happens at serve time and is
// the one case Stream info cannot see in advance.
func TestStreamInfoAnnouncesTheRoutesToneMap(t *testing.T) {
	t.Parallel()
	for _, sc := range routeGoldenScenarios() {
		pr, req := sc.build()
		reported := pr.Reported(context.Background(), req)
		pr, req = sc.build()
		plan, err := pr.Resolve(context.Background(), req)
		if err != nil {
			continue
		}
		announced := slices.Contains(reported.TranscodeReasons, "hdr_tonemapping_needed")
		poolState := strings.HasSuffix(sc.name, "| gpu-full")
		switch {
		case plan.Fingerprint.HDRTonemap && !announced && !poolState:
			t.Errorf("%s: the served plan tone-maps but Stream info does not say so (reported %v)", sc.name, reported.TranscodeReasons)
		case announced && !plan.Fingerprint.HDRTonemap:
			t.Errorf("%s: Stream info announces a tone-map the served plan does not do (reported %v)", sc.name, reported.TranscodeReasons)
		}
	}
}
