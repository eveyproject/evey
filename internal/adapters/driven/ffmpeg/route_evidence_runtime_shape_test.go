package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/require"
)

// The cost evidence is measured in the shape the runtime emits: the low-power
// encoder is opt-in, so no evidence route carries a low_power feature - the
// calibration used to price every route with an encoder the runtime, opted
// out by default, never ran.
func TestRouteEvidencePlan_MeasuresTheRuntimeShape(t *testing.T) {
	cap := &transcoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		Backends: []transcoding.HWBackendCap{{
			Backend: "vaapi", Device: "/dev/dri/renderD128",
			Codecs:    []string{"h264", "hevc"},
			Features:  []string{transcoding.LowPowerFeature("h264"), transcoding.BFramesFeature("h264", false), transcoding.FeatureTonemapOpenCL},
			Routes:    []string{transcoding.HWRouteSDR, transcoding.HWRouteTonemap},
			HWTonemap: true, MaxHeight: 1080,
		}},
	}
	routes := routeEvidencePlan(cap)
	hw := 0
	for _, er := range routes {
		if er.backend == "" {
			continue
		}
		hw++
		for _, f := range er.features {
			require.False(t, strings.HasPrefix(f, "low_power_"), "route %s/%s priced with the low-power encoder (%v)", er.route, er.codec, er.features)
		}
		require.Contains(t, er.features, transcoding.BFramesFeature("h264", false), "the other features stay")
	}
	require.Positive(t, hw, "the backend's routes are in the plan")
}
