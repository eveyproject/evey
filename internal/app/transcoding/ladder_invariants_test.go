package transcoding

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The ladder invariants hold for EVERY plan of the golden matrix, whatever
// builder or policy shapes it. A golden diff says "something changed"; these
// say "and it is still a ladder": monotone, without dead rungs, under every
// delivery cap, and never above what the host can encode in real time.
func TestLadderInvariants(t *testing.T) {
	classes := []transcoding.ContentClass{transcoding.ClassFilmNormal, transcoding.ClassAnimation, transcoding.ClassFilmLowMotion, transcoding.ClassFilmHighMotion, transcoding.ClassSport}
	for _, class := range classes {
		for _, host := range goldenHosts(class) {
			for _, src := range goldenSources() {
				for _, cl := range goldenClients() {
					name := fmt.Sprintf("%s/%s/%s/%s", class, host.name, src.name, cl.name)
					pr := host.pr()
					req := PlaybackRequest{
						MediaID: "m1", SourcePath: "/golden.mkv", Container: "matroska",
						Streams: src.streams, Client: cl.client, Prefs: cl.prefs,
						AudioTrack: 0, SubtitleBurnInTrack: -1,
						MaxBitrateKbps: cl.capKb, SourceBytes: src.bytes, SourceBitrateKbps: src.kbps, ScreenHeight: cl.screen,
					}
					plan, err := pr.Resolve(context.Background(), req)
					require.NoError(t, err, name)
					assertLadderInvariants(t, name, pr, req, plan)
				}
			}
		}
	}
}

func assertLadderInvariants(t *testing.T, name string, pr *PlanResolver, req PlaybackRequest, plan PlaybackPlan) {
	t.Helper()
	ladder := plan.Ladder
	require.NotEmpty(t, ladder, "%s: a plan always advertises at least one rung", name)

	isCopy := plan.Packager == PackagerCopyFFmpeg
	if isCopy {
		require.Len(t, ladder, 1, "%s: a copy is one rung", name)
		assert.True(t, ladder[0].Copy, "%s: the copy rung is flagged", name)
		return
	}

	// Monotone and non-redundant: heights strictly up, bitrates never down.
	for i := 1; i < len(ladder); i++ {
		assert.Greater(t, ladder[i].Height, ladder[i-1].Height, "%s: rung %d does not climb (%dp after %dp)", name, i, ladder[i].Height, ladder[i-1].Height)
		assert.GreaterOrEqual(t, ladder[i].VideoBitrate, ladder[i-1].VideoBitrate, "%s: rung %d costs less than the one below", name, i)
		assert.False(t, ladder[i].Copy, "%s: no copy rung inside a transcode ladder", name)
	}

	route := pr.encodeRoute(plan.Decision, req.Streams, false)
	dc := pr.deliveryConstraints(req, plan.Decision)
	src := pr.sourceFacts(req)

	// Delivery caps. The lowest rung is exempt from the height caps: a cap
	// under every rung keeps the smallest one rather than nothing.
	for i, p := range ladder {
		if i > 0 {
			if dc.MaxHeight > 0 {
				assert.LessOrEqual(t, p.Height, dc.MaxHeight, "%s: %s above the decoder/pick ceiling", name, p.Name)
			}
			if dc.QoEMaxHeight > 0 {
				assert.LessOrEqual(t, p.Height, dc.QoEMaxHeight, "%s: %s above the QoE ceiling", name, p.Name)
			}
			if route.Codec == "h264" && dc.MaxH264Level > 0 {
				assert.LessOrEqual(t, p.Height, transcoding.H264LevelMaxHeight(dc.MaxH264Level), "%s: %s above the H.264 level", name, p.Name)
			}
			if src.Height > 0 {
				assert.LessOrEqual(t, p.Height, src.Height, "%s: %s upscales the source", name, p.Name)
			}
		}
		if dc.VideoBitrateCeilingKbps > 0 {
			ceiling := dc.VideoBitrateCeilingKbps
			if route.Codec == "hevc" {
				ceiling = ceiling * transcoding.HEVCBitrateNum / transcoding.HEVCBitrateDen
			}
			assert.LessOrEqual(t, p.VideoBitrate, ceiling, "%s: %s over the bitrate ceiling", name, p.Name)
		}
	}
	if dc.ScreenHeight > 0 && len(ladder) > 1 {
		// No rung above the smallest one that already covers the screen.
		covering := 0
		for _, p := range ladder {
			if p.Height >= dc.ScreenHeight && (covering == 0 || p.Height < covering) {
				covering = p.Height
			}
		}
		if covering > 0 {
			assert.Equal(t, covering, ladder[len(ladder)-1].Height, "%s: a rung above the screen-covering one", name)
		}
	}

	// Throughput is the hard gate: no rung above what the host encodes in real
	// time on this route (lowest rung exempt, as above).
	cap := dc.Capability
	var ceiling int
	switch {
	case route.Bridge():
		ceiling = cap.HWBridgeMaxHeight
		if ceiling == 0 {
			ceiling = cap.SoftwareHDRMaxHeight
		}
		if ceiling == 0 {
			ceiling = cap.MaxHeightFor(true)
		}
	case route.HWUsable:
		ceiling = cap.MaxHeightFor(src.Codec == "" || cap.HWCanDecode(src.Codec))
	default:
		ceiling = cap.SoftwareCeilingFor(route.Codec)
		if route.Tonemap && cap.SoftwareHDRMaxHeight > 0 && cap.SoftwareHDRMaxHeight < ceiling {
			ceiling = cap.SoftwareHDRMaxHeight
		}
	}
	for _, p := range ladder[1:] {
		assert.LessOrEqual(t, p.Height, ceiling, "%s: %s above the host's real-time ceiling (%d, hw=%t bridge=%t)", name, p.Name, ceiling, route.HWUsable, route.Bridge())
	}

	// The encoder plan: one target per rung, and on a bitrate-targeted route
	// the manifest's BANDWIDTH (the rung's bitrate) is the encoder's ceiling.
	// The CPU route's capped CRF still carries 160 % of headroom over what the
	// manifest promises - a known gap the quality model closes (plan P1).
	require.Len(t, plan.Fingerprint.QualityTargets, len(ladder), "%s: one rate-control target per rung", name)
	for i, qt := range plan.Fingerprint.QualityTargets {
		assert.False(t, qt.IsZero(), "%s: rung %d has no rate control", name, i)
		if route.HWUsable {
			assert.Equal(t, ladder[i].VideoBitrate, qt.MaxKbps, "%s: rung %d promises %d kbps but may emit %d", name, i, ladder[i].VideoBitrate, qt.MaxKbps)
		}
	}
}
