package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The plan's cost is a queryable property, read off the route and the
// delivery strategy, never reconstructed from the decision: a real encode is
// billed by the limiter; any ffmpeg engine spawns a process; the mkvgo
// presentation is free.
func TestPlaybackPlan_CostHelpers(t *testing.T) {
	t.Parallel()
	copyRoute := ResolvedRoute{Copy: true}
	cases := []struct {
		name        string
		route       ResolvedRoute
		delivery    DeliveryStrategy
		wantEncode  bool // spends encode CPU/GPU (billed by the limiter)
		wantsFFmpeg bool // spawns any ffmpeg process (not the zero-process mkvgo presentation)
	}{
		{"mkvgo copy = free", copyRoute, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo}, false, false},
		{"hybrid = packager video, ffmpeg audio", copyRoute, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineFFmpeg}, false, true},
		{"wasm audio over the packager = free", copyRoute, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineClient}, false, false},
		{"ffmpeg copy = process, no encode", copyRoute, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, false, true},
		{"ffmpeg transcode = costly", ResolvedRoute{}, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := PlaybackPlan{Route: tc.route, Delivery: tc.delivery}
			assert.Equal(t, tc.wantEncode, p.NeedsVideoEncode())
			assert.Equal(t, tc.wantsFFmpeg, p.UsesFFmpeg())
		})
	}
}
