package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The key is the identity a measured route is looked up by, so its
// normalisation is a contract: an accelerator and an encoder spelled with
// different cases must land on the same sample, and the CPU is the empty
// backend.
func TestRouteCostKeyFor_NormalisesCaseAndCPU(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		RouteCostKey{Backend: "vaapi", Encoder: "hevc_vaapi", Route: RouteCostTonemap},
		RouteCostKeyFor("VAAPI", "HEVC_VAAPI", RouteCostTonemap))
	assert.Equal(t,
		RouteCostKey{Backend: "", Encoder: "libx264", Route: RouteCostSoftwareHDR},
		RouteCostKeyFor("", "libx264", RouteCostSoftwareHDR), "CPU route has an empty backend")
	assert.NotEqual(t,
		RouteCostKeyFor(VAAPI, "h264_vaapi", RouteCostSDR),
		RouteCostKeyFor(VAAPI, "hevc_vaapi", RouteCostSDR),
		"x264/x265 and h264/hevc VAAPI are never one price")
}

// The preset is part of the calibration key: an accelerator whose encoder
// takes no -preset is keyed "driver", everything else at the reference preset.
func TestProductionPreset_PerAccelerator(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "p4", ProductionPreset(NVENC))
	assert.Equal(t, "driver", ProductionPreset(VAAPI))
	assert.Equal(t, "driver", ProductionPreset(RKMPP), "MPP tunes through -rc_mode only, like VAAPI")
	assert.Equal(t, "veryfast", ProductionPreset(QSV))
	assert.Equal(t, "veryfast", ProductionPreset(None))
	assert.Equal(t, "veryfast", ProductionPreset(""))
}
