package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// pixelBudgetGPU models an accelerator with a fixed pixel throughput: the fps a
// probe achieves is the budget divided by the pixels of EVERY rung encoded in
// the same ffmpeg (the cumulative parallel ladder shares one engine). Pure, so
// the ladder shape decision is verified without a GPU.
func pixelBudgetGPU(pixelsPerSec float64) func(heights []int) (float64, bool) {
	return func(heights []int) (float64, bool) {
		total := 0.0
		for _, h := range heights {
			total += float64(h) * float64(h) * 16 / 9
		}
		return pixelsPerSec / total, true
	}
}

// The runtime default is single-variant: one rung per session. A small GPU
// that cannot sustain 720p AND 1080p in one process still sustains a lone
// 1080p session - which is all the runtime ever asks of it - and must be
// certified 1080p. The cumulative ladder (the shape the bench used to run
// regardless of the mode) pins the same GPU at 720p.
func TestWalkLadder_SingleVariantCertifiesRungTheCumulativeLadderCannot(t *testing.T) {
	t.Parallel()
	required := 24 * 1.2 // 28.8 fps
	// 1080p alone: 1920*1080 = 2.07 Mpx -> budget/2.07M >= 28.8 needs >= 59.7 Mpx/s.
	// 720+1080 cumulative: 0.92 + 2.07 = 2.99 Mpx -> 28.8 fps needs >= 86.2 Mpx/s.
	// 1440 alone: 3.69 Mpx -> needs >= 106 Mpx/s. Pick 70 Mpx/s: 1080 alone passes,
	// 720+1080 fails, 1440 fails.
	gpu := pixelBudgetGPU(70e6)

	single, singleSamples := walkLadder(hwBenchLadder, true, required, transcoding.LadderHW, gpu, nil)
	assert.Equal(t, 1080, single, "single-variant: a lone 1080p session sustains realtime")
	require.Len(t, singleSamples, 3, "720 pass, 1080 pass, 1440 fail then stop")
	for _, s := range singleSamples {
		assert.Equal(t, 1, s.ParallelEncodes, "single-variant probes exactly one rung")
		assert.Equal(t, transcoding.LadderHW, s.Ladder)
	}
	assert.True(t, singleSamples[1].Passing)
	assert.False(t, singleSamples[2].Passing)

	multi, multiSamples := walkLadder(hwBenchLadder, false, required, transcoding.LadderHW, gpu, nil)
	assert.Equal(t, 720, multi, "cumulative ladder: 720+1080 in one process is over budget")
	require.Len(t, multiSamples, 2)
	assert.Equal(t, []int{1, 2}, []int{multiSamples[0].ParallelEncodes, multiSamples[1].ParallelEncodes})
}

// An accelerator under realtime even at the lowest rung is rejected: no
// ceiling, one sample, the caller falls back to software.
func TestWalkLadder_HWUnderRealtimeAt720_Rejected(t *testing.T) {
	t.Parallel()
	best, samples := walkLadder(hwBenchLadder, true, 28.8, transcoding.LadderHW, pixelBudgetGPU(20e6), nil)
	assert.Equal(t, 0, best)
	require.Len(t, samples, 1, "short-circuits after the first failing rung")
	assert.False(t, samples[0].Passing)
	assert.Equal(t, 720, samples[0].Height)
}

// A GPU that sustains exactly 720p (and not 1080p) is certified at 720p - a
// real ceiling, never a fictitious 1080p.
func TestWalkLadder_HWOnly720Viable_Certified720(t *testing.T) {
	t.Parallel()
	// 720p alone: 0.92 Mpx -> 28.8 fps needs >= 26.5 Mpx/s; 1080 needs 59.7.
	best, samples := walkLadder(hwBenchLadder, true, 28.8, transcoding.LadderHW, pixelBudgetGPU(40e6), nil)
	assert.Equal(t, 720, best)
	require.Len(t, samples, 2)
	assert.True(t, samples[0].Passing)
	assert.False(t, samples[1].Passing)
}

// A probe that fails to run (ffmpeg error) is a failing rung, not a pass.
func TestWalkLadder_ProbeErrorFailsTheRung(t *testing.T) {
	t.Parallel()
	var seen []transcoding.BenchRungSample
	best, _ := walkLadder(hwBenchLadder, true, 28.8, transcoding.LadderHW,
		func([]int) (float64, bool) { return 0, false },
		func(s transcoding.BenchRungSample) { seen = append(seen, s) })
	assert.Equal(t, 0, best)
	require.Len(t, seen, 1, "onRung observes every probed rung")
	assert.False(t, seen[0].Passing)
}

// Route evidence measures every reference rung under the route's ceiling AND
// the ceiling itself: a 720p-only accelerator or the 720p/480p software floors
// get their one sample (their whole capacity) instead of none.
func TestRouteEvidenceRungsFor(t *testing.T) {
	t.Parallel()
	cases := map[int][]int{
		0:    nil,
		480:  {480},
		720:  {720},
		1080: {1080},
		1440: {1080, 1440},
		2160: {1080, 2160},
	}
	for ceiling, want := range cases {
		assert.Equal(t, want, routeEvidenceRungsFor(ceiling), "ceiling %d", ceiling)
	}
}

// A hardware route is measured up to ITS backend's certified ceiling, not the
// primary's: a 1080p-only VAAPI next to a 2160p QSV measures to 1080.
func TestRouteCeiling_HWUsesTheBackendsOwnCeiling(t *testing.T) {
	t.Parallel()
	r := &BenchmarkRunner{}
	cap := &transcoding.EncodeCapability{
		HWBackend: "qsv", HWMaxHeight: 2160, SoftwareMaxHeight: 1080,
		Backends: []transcoding.HWBackendCap{
			{Backend: "qsv", MaxHeight: 2160},
			{Backend: "vaapi", MaxHeight: 1080},
		},
	}
	assert.Equal(t, 2160, r.routeCeiling(evidenceRoute{backend: "qsv"}, cap))
	assert.Equal(t, 1080, r.routeCeiling(evidenceRoute{backend: "vaapi"}, cap))
	// A backend that recorded no ceiling (older row) reads the primary scalar.
	cap.Backends[1].MaxHeight = 0
	assert.Equal(t, 2160, r.routeCeiling(evidenceRoute{backend: "vaapi"}, cap))
	// Software routes keep their own caps.
	assert.Equal(t, 1080, r.routeCeiling(evidenceRoute{}, cap))
}

// A backend's own bridge ceiling is the tallest passing rung of ITS bridge
// ladder (0 when none passed) - recorded per backend, not read off the primary.
func TestTallestPassing_PerBackendBridgeCeiling(t *testing.T) {
	t.Parallel()
	_, ok := walkLadder(hwBenchLadder, true, 28.8, transcoding.LadderHWBridge, pixelBudgetGPU(70e6), nil)
	assert.Equal(t, 1080, tallestPassing(ok))
	_, none := walkLadder(hwBenchLadder, true, 28.8, transcoding.LadderHWBridge, pixelBudgetGPU(20e6), nil)
	assert.Equal(t, 0, tallestPassing(none))
	assert.Equal(t, 0, tallestPassing(nil))
}

// The bridge route is measured up to the backend's EFFECTIVE bridge ceiling: its
// own passing rung, else the software HDR ceiling (480p floor) - never the
// backend's plain encode ceiling.
func TestRouteCeiling_BridgeUsesTheEffectiveBridgeCeiling(t *testing.T) {
	t.Parallel()
	r := &BenchmarkRunner{}
	cap := &transcoding.EncodeCapability{
		HWBackend: "nvenc", HWMaxHeight: 2160, HWBridgeMaxHeight: 0, SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480,
		Backends: []transcoding.HWBackendCap{
			{Backend: "nvenc", MaxHeight: 2160, BridgeMaxHeight: 0},
			{Backend: "vaapi", MaxHeight: 2160, BridgeMaxHeight: 1080},
		},
	}
	assert.Equal(t, 480, r.routeCeiling(evidenceRoute{backend: "nvenc", route: transcoding.RouteCostBridge}, cap))
	assert.Equal(t, []int{480}, routeEvidenceRungsFor(r.routeCeiling(evidenceRoute{backend: "nvenc", route: transcoding.RouteCostBridge}, cap)))
	assert.Equal(t, 1080, r.routeCeiling(evidenceRoute{backend: "vaapi", route: transcoding.RouteCostBridge}, cap))
	assert.Equal(t, 2160, r.routeCeiling(evidenceRoute{backend: "nvenc", route: transcoding.RouteCostSDR}, cap), "the SDR route keeps the encode ceiling")
}
