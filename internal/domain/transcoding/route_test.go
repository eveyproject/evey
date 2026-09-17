package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Exactly the seven shapes are legal - every other tuple of the four stages
// is refused with its reason. A new stage or placement that widens the set
// must widen this test on purpose.
func TestTopologyFor_ExactlySevenLegalShapes(t *testing.T) {
	legal := map[transcoding.Topology]bool{}
	for _, tp := range transcoding.AllTopologies() {
		legal[tp] = true
		got, err := transcoding.TopologyFor(tp.Decode, tp.Encode, tp.ToneMap, tp.Output)
		require.NoError(t, err, tp.String())
		assert.Equal(t, tp, got)
		assert.True(t, tp.Legal())
	}
	total, refused := 0, 0
	for _, dec := range []transcoding.Stage{transcoding.StageCPU, transcoding.StageGPU} {
		for _, enc := range []transcoding.Stage{transcoding.StageCPU, transcoding.StageGPU} {
			for _, tm := range []transcoding.ToneMapPlacement{transcoding.ToneMapNone, transcoding.ToneMapGPU, transcoding.ToneMapCPU} {
				for _, out := range []transcoding.OutputMode{transcoding.OutputSDR, transcoding.OutputHDRPassthrough} {
					total++
					tp, err := transcoding.TopologyFor(dec, enc, tm, out)
					if err != nil {
						refused++
						var ill transcoding.ErrIllegalTopology
						require.ErrorAs(t, err, &ill)
						assert.NotEmpty(t, ill.Why)
						assert.False(t, transcoding.Topology{Decode: dec, Encode: enc, ToneMap: tm, Output: out}.Legal())
						continue
					}
					assert.True(t, legal[tp], "TopologyFor accepted %s, which AllTopologies does not list", tp)
				}
			}
		}
	}
	assert.Equal(t, 24, total)
	assert.Equal(t, 24-7, refused)
}

// The cost class is a function of the topology, and the boolean route reads
// its topology with the precedence CostKey always applied - so CostKey is
// byte-identical to what it was before the topology existed (the calibration
// evidence is indexed by it).
func TestEncodeRoute_CostKeyIsTheTopologyCostClass(t *testing.T) {
	want := map[transcoding.Topology]transcoding.RouteCostClass{
		transcoding.TopologySoftware:       transcoding.RouteCostSDR,
		transcoding.TopologySoftwareHDR:    transcoding.RouteCostSoftwareHDR,
		transcoding.TopologyZeroCopy:       transcoding.RouteCostSDR,
		transcoding.TopologyGPUToneMap:     transcoding.RouteCostTonemap,
		transcoding.TopologyBridge:         transcoding.RouteCostBridge,
		transcoding.TopologyDecoupled:      transcoding.RouteCostDecoupled,
		transcoding.TopologyHDRPassthrough: transcoding.RouteCostPassthrough,
	}
	for tp, class := range want {
		assert.Equal(t, class, tp.CostClass(), tp.String())
	}
	// Every boolean combination, including the over-specified ones, keys the
	// way the pre-topology switch did: passthrough > GPU tone-map > bridge >
	// decoupled > plain; software: tone-map > plain.
	legacy := func(r transcoding.EncodeRoute) transcoding.RouteCostClass {
		switch {
		case r.HWUsable && r.HDRPassthrough:
			return transcoding.RouteCostPassthrough
		case r.HWUsable && r.HWTonemap:
			return transcoding.RouteCostTonemap
		case r.HWUsable && r.Tonemap:
			return transcoding.RouteCostBridge
		case r.HWUsable && r.SoftwareDecode:
			return transcoding.RouteCostDecoupled
		case !r.HWUsable && r.Tonemap:
			return transcoding.RouteCostSoftwareHDR
		}
		return transcoding.RouteCostSDR
	}
	for i := range 64 {
		r := transcoding.EncodeRoute{
			Codec: "hevc", Accel: transcoding.VAAPI,
			HWUsable: i&1 != 0, SoftwareDecode: i&2 != 0, Tonemap: i&4 != 0, HWTonemap: i&8 != 0, Copy: i&16 != 0, HDRPassthrough: i&32 != 0,
		}
		if !r.HWUsable {
			r.Accel = ""
		}
		assert.Equal(t, transcoding.RouteCostKeyFor(r.Accel, transcoding.RouteEncoderName(r.Codec, string(r.Accel)), legacy(r)), r.CostKey(), "%+v", r)
		assert.Equal(t, r.HWUsable, r.Topology().HW(), "%+v", r)
	}
}

// The calibration matrix is the legal topologies x the codecs (passthrough
// HEVC only): 6 software shapes, 13 hardware ones.
func TestRuntimeRouteShapes_AreTheLegalTopologies(t *testing.T) {
	shapes := transcoding.RuntimeRouteShapes()
	require.Len(t, shapes, 19)
	sw, hw := 0, 0
	for _, sh := range shapes {
		if sh.HW {
			hw++
		} else {
			sw++
		}
		if sh.Route == transcoding.RouteCostPassthrough {
			assert.Equal(t, "hevc", sh.Codec)
		}
	}
	assert.Equal(t, 6, sw)
	assert.Equal(t, 13, hw)
	// Every shape names a legal topology's class.
	classes := map[transcoding.RouteCostClass]bool{}
	for _, tp := range transcoding.AllTopologies() {
		classes[tp.CostClass()] = true
	}
	for _, sh := range shapes {
		assert.True(t, classes[sh.Route], "%+v names no legal topology", sh)
	}
}
