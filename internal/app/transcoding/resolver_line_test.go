package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// lineResolver is a bare resolver whose copy rung is wired, so a copyable
// request naturally resolves to a copy plan the line can then judge.
func lineResolver() *PlanResolver {
	return &PlanResolver{CopyProducer: &stubProducer{profiles: []transcoding.HLSProfile{{Name: "copy", Height: 1080}}}}
}

// lineRequest is a copyable 1080p H.264 source whose copy pushes peakKbps
// down the line, with the client's constraints set.
func lineRequest(peakKbps, lineKbps int64, floor bool) PlaybackRequest {
	req := ladderRequest(1080)
	req.Streams = deliveryStreams("aac")
	req.Client = transcoding.ClientProfile{VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}}
	req.SourcePeakKbps = peakKbps
	req.LineKbps = lineKbps
	req.ABRFloor = floor
	return req
}

// transcodeLadderOf is the natural transcode ladder of the same request (the
// vd=transcode pin) - the fitted plan's ladder MUST be this one: a fit
// narrows the master, never the ladder the child URLs index.
func transcodeLadderOf(t *testing.T, pr *PlanResolver, req PlaybackRequest) PlaybackPlan {
	t.Helper()
	req.LineKbps, req.ABRFloor = 0, false
	req.PinnedVideoDecision = "transcode"
	plan, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	require.False(t, plan.Route.Copy)
	require.NotEmpty(t, plan.Ladder)
	return plan
}

// Copy-first: a line that carries the copy peak with margin leaves the copy alone.
func TestResolveLine_CopyFirstWhenTheLineCarriesIt(t *testing.T) {
	plan, err := lineResolver().Resolve(context.Background(), lineRequest(4000, 12000, false))
	require.NoError(t, err)
	assert.True(t, plan.Route.Copy)
	assert.False(t, plan.Fit.Single)
}

// A copy the line cannot carry becomes the HIGHEST transcoded rung the line
// carries - mono-variant (the master's presentation), on the full ladder
// (the plan's), with its own reason.
func TestResolveLine_FitsTheHighestRungTheLineCarries(t *testing.T) {
	pr := lineResolver()
	req := lineRequest(7400, 6000, false)
	natural := transcodeLadderOf(t, pr, req)
	want := 0
	for i, p := range natural.Ladder {
		if (p.VideoBitrate+p.AudioBitrate)*12 <= 6000*10 {
			want = i
		}
	}

	plan, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, plan.Route.Copy, "a 7.4 Mbps copy cannot ride a 6 Mbps line")
	assert.Equal(t, LineFit{Single: true, Rung: want}, plan.Fit)
	assert.Equal(t, natural.Ladder, plan.Ladder, "the ladder stays the full transcode one: child URLs index it")
	assert.Equal(t, natural.FingerprintHash, plan.FingerprintHash, "same bytes as any transcode viewer of the title")
	assert.Contains(t, plan.Decision.TranscodeReasons, reasonLineExceeded)
	assert.NotContains(t, plan.Decision.TranscodeReasons, "decision_pinned")
	assert.Equal(t, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, plan.Delivery)
}

// The floor is the lowest rung, whatever the line says.
func TestResolveLine_FloorIsTheLowestRung(t *testing.T) {
	pr := lineResolver()
	plan, err := pr.Resolve(context.Background(), lineRequest(4000, 0, true))
	require.NoError(t, err)
	assert.False(t, plan.Route.Copy)
	assert.Equal(t, LineFit{Single: true, Rung: 0}, plan.Fit)
	assert.Contains(t, plan.Decision.TranscodeReasons, reasonFloorRequest)
}

// A floor only when it halves the bitrate: a lean source keeps its copy (a
// sidegrade would cost an encode for no relief).
func TestResolveLine_FloorRefusedOnLeanSource(t *testing.T) {
	plan, err := lineResolver().Resolve(context.Background(), lineRequest(1500, 0, true))
	require.NoError(t, err)
	assert.True(t, plan.Route.Copy)
	assert.False(t, plan.Fit.Single)
}

// Precedence as the receiver stacks them: the measured line first; a line
// that carries the copy hands over to the floor when both are asked.
func TestResolveLine_LineThenFloor(t *testing.T) {
	pr := lineResolver()
	plan, err := pr.Resolve(context.Background(), lineRequest(4000, 12000, true))
	require.NoError(t, err)
	assert.Equal(t, LineFit{Single: true, Rung: 0}, plan.Fit, "the line carries the copy, the floor still applies")
	assert.Contains(t, plan.Decision.TranscodeReasons, reasonFloorRequest)

	req := lineRequest(7400, 6000, true)
	natural := transcodeLadderOf(t, pr, req)
	plan, err = pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	require.True(t, plan.Fit.Single)
	assert.Contains(t, plan.Decision.TranscodeReasons, reasonLineExceeded, "a line that does not carry the copy wins over the floor")
	assert.Equal(t, natural.Ladder, plan.Ladder)
}

// A transcode ladder is the player's to pick from: the line judges the copy
// rung only (a client that cannot copy already gets the shaped ladder).
func TestResolveLine_TranscodePlanIsNotNarrowed(t *testing.T) {
	req := lineRequest(7400, 6000, true)
	req.Client.VideoCodecs = []string{"hevc"} // the h264 source is not decodable: a transcode, not a copy
	plan, err := lineResolver().Resolve(context.Background(), req)
	require.NoError(t, err)
	require.False(t, plan.Route.Copy)
	assert.False(t, plan.Fit.Single)
	assert.Greater(t, len(plan.Ladder), 1)
}

// A host without the ffmpeg copy rung still copies through the packager: the
// line judges that copy the same way.
func TestResolveLine_PackagerCopyIsJudgedToo(t *testing.T) {
	pr := &PlanResolver{OnDemandPackager: true, AudioRenditionEnabled: true, AudioRenditionPipeline: true}
	natural, err := pr.Resolve(context.Background(), lineRequest(4000, 0, false))
	require.NoError(t, err)
	require.Equal(t, EngineMkvgo, natural.Delivery.Video, "premise: the packager carries the copy")
	require.False(t, natural.Route.Copy, "premise: no ffmpeg copy rung on this host")

	plan, err := pr.Resolve(context.Background(), lineRequest(4000, 0, true))
	require.NoError(t, err)
	assert.Equal(t, LineFit{Single: true, Rung: 0}, plan.Fit)
	assert.Equal(t, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, plan.Delivery)
}

// A child URL carries the master's verdict in its pin (vd=): it is never
// re-judged, so the copy kept by a refused floor stays a copy and a fitted
// master's child resolves the very plan the master sealed.
func TestResolveLine_ChildInheritsTheVerdictThroughThePin(t *testing.T) {
	pr := lineResolver()
	child := lineRequest(1500, 0, true)
	child.PinnedVideoDecision = "copy"
	plan, err := pr.Resolve(context.Background(), child)
	require.NoError(t, err)
	assert.True(t, plan.Route.Copy)

	master := lineRequest(7400, 6000, false)
	fitted, err := pr.Resolve(context.Background(), master)
	require.NoError(t, err)
	require.True(t, fitted.Fit.Single)
	child = master
	child.PinnedVideoDecision = "transcode"
	plan, err = pr.Resolve(context.Background(), child)
	require.NoError(t, err)
	assert.Equal(t, fitted.FingerprintHash, plan.FingerprintHash, "the sealed fp resolves on the child")
	assert.False(t, plan.Fit.Single, "children index the full ladder")
}
