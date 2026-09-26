package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The packager carries a copy-eligible video whether or not ffmpeg's copy rung
// is available: Route.Copy is ffmpeg's verdict (a wired copy producer, cache
// room), the packager needs neither. When they disagree the plan's video is
// the packager's, its route is priced for the ffmpeg path that serves the plan
// if the packager cannot build the source (an MPEG-TS), and the planned
// serving path says COPY - read from the route alone it said transcode, and
// every such master tripped the invariant counter (measured 2026-09-26).
func TestResolve_PackagerVideoOnATranscodeRoute(t *testing.T) {
	srcs := routeGoldenSources()
	var web goldenClient
	for _, c := range goldenClients() {
		if c.name == "web-h264" {
			web = c
		}
	}
	require.NotEmpty(t, web.name)
	vaapi := vaapiCapable()

	build := func(t *testing.T, wire func(pr *PlanResolver)) (PlaybackPlan, PlaybackRequest, *int) {
		t.Helper()
		pr, _ := routeGoldenResolver(&vaapi, true)
		pr.OnDemandPackager = true
		// The hybrid master's audio pipeline: the golden resolver wires none,
		// and without an audio path the whole presentation leaves the packager.
		pr.AudioRenditionEnabled, pr.AudioRenditionPipeline = true, true
		wire(pr)
		priced := 0
		admit := pr.AdmissionRoute
		pr.AdmissionRoute = func(key string, in RouteAdmission) RouteGrant {
			priced++
			return admit(key, in)
		}
		req := routeGoldenRequest(srcs["1080p-h264-8m"], web)
		plan, err := pr.Resolve(context.Background(), req)
		require.NoError(t, err)
		require.True(t, VideoCopyEligible(plan.Decision, req.Streams), "the scenario must be a copy-eligible video")
		return plan, req, &priced
	}

	for _, tc := range []struct {
		name string
		wire func(pr *PlanResolver)
	}{
		{"no ffmpeg copy producer", func(pr *PlanResolver) { pr.CopyProducer = nil }},
		{"the cache cannot hold a copy window", func(pr *PlanResolver) { pr.FreeBytes = func() (int64, error) { return 0, nil } }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, _, priced := build(t, tc.wire)
			require.Equal(t, EngineMkvgo, plan.Delivery.Video, "the packager carries the video")
			require.False(t, plan.Route.Copy, "ffmpeg's copy rung is not available")
			path := plan.Delivery.ServingPath(plan.Route, plan.Fingerprint)
			require.Equal(t, "copy", path.VideoDecision, "what the packager serves is a copy")
			require.Equal(t, "mkvgo", path.VideoEngine)
			// The ffmpeg path behind the packager transcodes: its ladder is
			// shaped and admitted with the plan, the insurance for a source the
			// packager cannot build. That reservation is deliberate.
			require.NotEmpty(t, plan.Ladder)
			for _, p := range plan.Ladder {
				require.False(t, p.Copy, "the fallback ladder is a transcode ladder")
			}
			require.Equal(t, 1, *priced, "the fallback route is priced once")
		})
	}

	t.Run("copy rung available: the route is the copy", func(t *testing.T) {
		plan, _, priced := build(t, func(*PlanResolver) {})
		require.Equal(t, EngineMkvgo, plan.Delivery.Video)
		require.True(t, plan.Route.Copy)
		require.Equal(t, "copy", plan.Delivery.ServingPath(plan.Route, plan.Fingerprint).VideoDecision)
		require.Len(t, plan.Ladder, 1)
		require.True(t, plan.Ladder[0].Copy)
		require.Equal(t, 0, *priced, "a copy rung takes no admission")
	})
}
