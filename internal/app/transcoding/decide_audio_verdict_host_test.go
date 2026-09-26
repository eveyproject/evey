package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Decide reads the resolver live (no frozen capability, no route costs: it
// runs without Resolve for the audio renditions and the audio resync). That
// is sound only while the audio verdict reads no host toggle: the same track
// must be judged the same on a bare resolver and on a calibrated, hardware
// one - a rendition served from a fresh resolver then agrees with the master
// its plan was frozen for.
func TestDecide_AudioVerdictReadsNoHostToggle(t *testing.T) {
	srcs := routeGoldenSources()
	var tv goldenClient
	for _, c := range goldenClients() {
		if c.name == "tv-level41" {
			tv = c
		}
	}
	require.NotEmpty(t, tv.name)
	vaapi := vaapiCapable()
	calibrated, _ := routeGoldenResolver(&vaapi, true)
	calibrated.OnDemandPackager = true
	calibrated.AudioRenditionPipeline = true
	bare := &PlanResolver{}

	for _, name := range []string{"1080p-h264-8m", "1080p-h264-25m-bluray", "2160p-hevc-hdr10-70m"} {
		src := srcs[name]
		for track := range 2 {
			req := routeGoldenRequest(src, tv)
			req.AudioTrack = track
			a := calibrated.Decide(context.Background(), req)
			b := bare.Decide(context.Background(), req)
			require.Equal(t, a.AudioDecision, b.AudioDecision, "%s track %d", name, track)
			require.Equal(t, a.AudioCodec, b.AudioCodec, "%s track %d", name, track)
			require.Equal(t, a.MaxAudioChannels, b.MaxAudioChannels, "%s track %d", name, track)
		}
	}
}
