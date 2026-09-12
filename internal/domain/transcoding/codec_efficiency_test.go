package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeEfficiency() CodecEfficiencyModel {
	return CodecEfficiencyModel{
		Version: 7,
		Entries: map[EncoderKey][]EfficiencyEntry{
			{Codec: "hevc", Accel: NVENC, Preset: "p4"}: {
				{Height: 1080, Factor: 0.6, Confidence: 1, DispP90: 1.2, SampleCount: 30},
				{Height: 2160, Factor: 0.7, Confidence: 0.5, DispP90: 1.3, SampleCount: 9},
			},
			{Codec: "h264", Accel: VAAPI, Preset: "driver"}: {
				{Height: 1080, Factor: 1.4, Confidence: 0, SampleCount: 2}, // unusable
			},
		},
	}
}

func TestCodecEfficiencyModel_Lookup(t *testing.T) {
	m := fakeEfficiency()
	e, ok := m.Lookup(EncoderKey{Codec: "HEVC", Accel: NVENC, Preset: "p4"}, 720)
	require.True(t, ok)
	assert.Equal(t, 1080, e.Height, "nearest stratum")
	e, _ = m.Lookup(EncoderKey{Codec: "hevc", Accel: NVENC, Preset: "p4"}, 2160)
	assert.Equal(t, 2160, e.Height)
	_, ok = m.Lookup(EncoderKey{Codec: "h264", Accel: VAAPI, Preset: "driver"}, 1080)
	assert.False(t, ok, "zero confidence is no cell")
	_, ok = m.Lookup(EncoderKey{Codec: "h264", Accel: QSV, Preset: "veryfast"}, 1080)
	assert.False(t, ok, "never measured")
	_, ok = m.Lookup(EncoderKey{Codec: "hevc", Accel: NVENC, Preset: "p4", HDR: true}, 1080)
	assert.False(t, ok, "the HDR operating point is not calibrated")
}

func TestCodecEfficiencyModel_PriceLadder(t *testing.T) {
	m := fakeEfficiency()
	in := []HLSProfile{
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2000, CostModelled: true},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, CostModelled: true},
		{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 20000, CostModelled: true},
	}
	out := m.PriceLadder(in, EncoderKey{Codec: "hevc", Accel: NVENC, Preset: "p4"})
	assert.Equal(t, int64(1440), out[0].VideoBitrate, "0.6 x 1.2 margin from the nearest (1080) cell")
	assert.Equal(t, int64(3600), out[1].VideoBitrate)
	assert.Equal(t, int64(18200), out[2].VideoBitrate, "0.7 x 1.3 from the 2160 cell")
	for _, p := range out {
		assert.Equal(t, 7, p.EfficiencyVersion)
	}
	assert.Equal(t, int64(5000), in[1].VideoBitrate, "input not mutated")
	assert.Equal(t, 0, in[1].EfficiencyVersion)
}

func TestEncodeRoute_EncoderKey(t *testing.T) {
	assert.Equal(t, EncoderKey{Codec: "hevc", Accel: NVENC, Preset: "p4"}, EncodeRoute{Codec: "HEVC", Accel: NVENC, HWUsable: true}.EncoderKey())
	assert.Equal(t, EncoderKey{Codec: "h264", Preset: "veryfast"}, EncodeRoute{Codec: "h264"}.EncoderKey())
	assert.True(t, EncodeRoute{Codec: "hevc", Accel: VAAPI, HWUsable: true, HDRPassthrough: true}.EncoderKey().HDR)
}

// The builder prices a calibrated GPU route from the cost reference and the
// measured efficiency, never from the two-thirds rule; an uncalibrated GPU
// route keeps the class table.
func TestStaticLadderBuilder_GPUPricedByMeasuredEfficiency(t *testing.T) {
	eff := fakeEfficiency()
	b := StaticLadderBuilder{Efficiency: &eff}
	cap := EncodeCapability{SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc"}, HWDecodeCodecs: []string{"h264", "hevc"}}
	in := LadderInput{
		Source:     SourceFacts{Width: 1920, Height: 1080, Codec: "h264", FPS: 24},
		Complexity: ComplexityVector{Class: ClassFilmNormal, Metrics: ComplexityMetrics{MotionMean: 4, FramesSampled: 200}},
		Route:      EncodeRoute{Codec: "hevc", Accel: NVENC, HWUsable: true},
		Delivery:   DeliveryConstraints{Capability: cap},
	}
	gpu := b.Build(in)
	cpu := b.Build(LadderInput{Source: in.Source, Complexity: in.Complexity, Route: EncodeRoute{Codec: "h264"}, Delivery: in.Delivery})
	require.Len(t, gpu, len(cpu))
	for i := range gpu {
		assert.True(t, gpu[i].CostModelled)
		assert.Equal(t, 7, gpu[i].EfficiencyVersion)
		assert.InDelta(t, 0.72, float64(gpu[i].VideoBitrate)/float64(cpu[i].VideoBitrate), 0.01, "rung %s: 0.6 x 1.2 of the CPU reference price", gpu[i].Name)
	}

	uncalibrated := in
	uncalibrated.Route = EncodeRoute{Codec: "h264", Accel: NVENC, HWUsable: true}
	table := b.Build(uncalibrated)
	for i, p := range table {
		assert.False(t, p.CostModelled, "rung %d: no cell for nvenc h264, the table applies", i)
		assert.Equal(t, LadderFilmNormal[i].VideoBitrate, p.VideoBitrate)
	}

	tableHEVC := in
	tableHEVC.Complexity.Metrics = ComplexityMetrics{} // no cost prediction at all
	legacy := b.Build(tableHEVC)
	assert.Equal(t, LadderFilmNormal[3].VideoBitrate*HEVCBitrateNum/HEVCBitrateDen, legacy[3].VideoBitrate, "the two-thirds rule survives only for table-priced ladders")
}
