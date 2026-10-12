package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The declarative software bench ladder is DATA (the plan's "ladder déclaratif"):
// its contract is validated here without ffmpeg, so a typo in a margin, floor or
// encoder is caught by a unit test rather than only by a real calibration run.
func TestSwBenchLadderSpecs_Contract(t *testing.T) {
	t.Parallel()

	byLabel := make(map[string]swBenchLadderSpec, len(swBenchLadderSpecs))
	for _, s := range swBenchLadderSpecs {
		assert.NotContains(t, byLabel, s.label, "each ladder label appears once")
		byLabel[s.label] = s
	}

	// The labels MUST be the exact strings the domain's Derive groups samples by,
	// or a rung's evidence would land in no ladder and its ceiling would floor.
	assert.ElementsMatch(t,
		[]string{transcoding.LadderSoftware, transcoding.LadderSoftwareHDR, transcoding.LadderSoftwareAV1},
		[]string{swBenchLadderSpecs[0].label, swBenchLadderSpecs[1].label, swBenchLadderSpecs[2].label},
	)

	sw := byLabel[transcoding.LadderSoftware]
	assert.Equal(t, "libx264", sw.encoder)
	assert.InDelta(t, 1.0, sw.marginMult, 1e-9)
	assert.Equal(t, 720, sw.floor, "universal SDR floor")
	assert.False(t, sw.hdr)

	hdr := byLabel[transcoding.LadderSoftwareHDR]
	assert.Equal(t, "libx264", hdr.encoder)
	assert.InDelta(t, 1.0, hdr.marginMult, 1e-9)
	assert.Equal(t, 480, hdr.floor, "HDR is heavier: floors higher than SDR")
	assert.True(t, hdr.hdr, "HDR spec runs on the 10-bit sample + tonemap")

	av1 := byLabel[transcoding.LadderSoftwareAV1]
	assert.Equal(t, "libsvtav1", av1.encoder)
	assert.InDelta(t, av1RealtimeMarginMultiplier, av1.marginMult, 1e-9)
	assert.InDelta(t, 2.0, av1.marginMult, 1e-9, "AV1 must clear ~2x the base margin (cold-start guard)")
	assert.Equal(t, 0, av1.floor, "floor 0 => AV1 not viable stays hardware-only, no forced rung")
	assert.False(t, av1.hdr)

	// The SDR probe ladder tops out at 1440p (4K is HW-only by design).
	assert.Equal(t, []int{720, 1080, 1440}, swBenchLadder)
}
