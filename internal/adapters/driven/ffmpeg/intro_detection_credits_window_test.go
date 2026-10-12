package ffmpeg

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// The credits decode must never reach farther back than parseEndCredits can
// ACCEPT: past the plausibility bound every frame is decoded to produce a
// cluster the same function then refuses. And it must never reach farther than
// the historic window either - aligning upwards would widen the decode on the
// runtimes where credits detection is already a desert (films), buying nothing
// measurable.
func TestCreditsScanWindowSec(t *testing.T) {
	t.Parallel()
	legacy := func(d float64) float64 {
		if d*0.2 < 600 {
			return d * 0.2
		}
		return 600
	}
	cases := []struct {
		name        string
		durationSec float64
		want        float64
	}{
		{"11-min episode stays on the historic window", 660, 132},
		{"20-min episode: bound and window already agree", 1200, 240},
		{"40-min episode: bound 288s replaces a 480s decode", 2400, 288},
		{"45-min episode: bound 324s replaces a 540s decode", 2700, 324},
		{"50-min episode: bound 360s replaces the 600s cap", 3000, 360},
		{"60-min episode: bound 432s replaces the 600s cap", 3600, 432},
		{"83-min film: bound meets the cap exactly", 5000, 600},
		{"130-min film keeps the 600s cap, not its 936s bound", 7800, 600},
		{"180-min film keeps the 600s cap, not its 1296s bound", 10800, 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := creditsScanWindowSec(tc.durationSec)
			assert.InDelta(t, tc.want, got, 0.001)
			assert.LessOrEqual(t, got, legacy(tc.durationSec)+0.001,
				"the window must never decode more than the historic one")
			leadMax := float64(domainlibrary.MaxPlausibleCreditsLeadMs(
				int64(tc.durationSec*1000), domainlibrary.SuspiciousCreditsLeadMs)) / 1000
			assert.LessOrEqual(t, got, leadMax+0.001,
				"the window must never decode past what the plausibility bound can accept")
		})
	}
}

// A credits roll starting EXACTLY at the plausibility bound is the last one the
// parser accepts (the bound is strict), so it has to land at offset 0 of the
// window rather than one frame outside it.
func TestCreditsScanWindow_CreditsExactlyAtTheBoundAreStillParsed(t *testing.T) {
	t.Parallel()
	const durationSec = 2700.0 // 45-min episode, bound = 324s
	startSec := durationSec - creditsScanWindowSec(durationSec)
	leadMax := float64(domainlibrary.MaxPlausibleCreditsLeadMs(
		int64(durationSec*1000), domainlibrary.SuspiciousCreditsLeadMs)) / 1000
	require.InDelta(t, durationSec-leadMax, startSec, 0.001,
		"the window has to open exactly on the bound for a credits roll sitting on it")

	// 6s of black at offset 0: a lone region that far out is only trusted on its
	// black length (creditsClusterMinBlackSec), which is what a fade into the
	// credits looks like.
	input := "[blackdetect @ 0x1] black_start:0 black_end:6.0 black_duration:6.0"
	startMs, endMs, found := parseEndCredits(input, startSec, durationSec)
	require.True(t, found, "credits sitting exactly on the bound must survive the scan")
	assert.Equal(t, int64(startSec*1000), startMs)
	assert.Equal(t, int64(durationSec*1000), endMs)
}

// The narrowed window is not merely cheaper: a black region between the new and
// the old window edge used to be aggregated into the trailing cluster and drag
// its first member past the bound, so the WHOLE cluster was refused. Dropping
// that region from the decode turns a rejection into a detection.
func TestCreditsScanWindow_EarlyFadeNoLongerPoisonsTheCluster(t *testing.T) {
	t.Parallel()
	const durationSec = 2700.0
	// Two black regions 98s apart (inside creditsClusterGapSec): one at lead
	// 400s, one at lead 300s.
	const earlyStart, lateStart = 2300.0, 2400.0

	wideStart := durationSec - 540 // the historic 20% window
	wideInput := fmt.Sprintf(
		"[blackdetect @ 0x1] black_start:%.1f black_end:%.1f black_duration:2.0\n"+
			"[blackdetect @ 0x2] black_start:%.1f black_end:%.1f black_duration:4.0",
		earlyStart-wideStart, earlyStart-wideStart+2, lateStart-wideStart, lateStart-wideStart+4)
	_, _, found := parseEndCredits(wideInput, wideStart, durationSec)
	require.False(t, found, "the wide window drags the cluster to lead 400s and the bound refuses it")

	narrowStart := durationSec - creditsScanWindowSec(durationSec)
	require.Greater(t, narrowStart, earlyStart, "the narrowed window must exclude the early fade")
	narrowInput := fmt.Sprintf(
		"[blackdetect @ 0x2] black_start:%.1f black_end:%.1f black_duration:4.0",
		lateStart-narrowStart, lateStart-narrowStart+4)
	startMs, _, found := parseEndCredits(narrowInput, narrowStart, durationSec)
	require.True(t, found, "alone, the trailing region sits at a plausible lead")
	assert.Equal(t, int64(lateStart*1000), startMs)
}

// The non-loss property, executable: over random black layouts, every credits
// roll the historic window ACCEPTED is still found at the very same timestamp
// once the decode is clamped to the bound. Only the converse is allowed to
// differ - the narrowed window may accept where the wide one refused.
func TestCreditsScanWindow_NarrowingNeverLosesAnAcceptedDetection(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1))
	durations := []float64{660, 1500, 2400, 2700, 3600, 5400, 7800}
	gained := 0
	for range 20000 {
		durationSec := durations[rng.Intn(len(durations))]
		wideStart := durationSec - math.Min(creditsScanCapSec, durationSec*creditsScanFraction)
		narrowStart := durationSec - creditsScanWindowSec(durationSec)

		// Random black layout over the wide window, ascending and disjoint.
		var regions []floatRange
		cursor := wideStart
		for cursor < durationSec && len(regions) < 8 {
			cursor += rng.Float64() * 200
			dur := 0.4 + rng.Float64()*7
			if cursor+dur >= durationSec {
				break
			}
			regions = append(regions, floatRange{cursor, cursor + dur})
			cursor += dur
		}
		if len(regions) == 0 {
			continue
		}

		wideStartMs, _, wideFound := parseEndCredits(renderBlackdetect(regions, wideStart), wideStart, durationSec)
		// What blackdetect reports for the clamped decode: regions clipped to the
		// narrowed window, those left too short dropped by minCreditsBlackSec.
		var clipped []floatRange
		for _, r := range regions {
			s := math.Max(r.start, narrowStart)
			if r.end > s {
				clipped = append(clipped, floatRange{s, r.end})
			}
		}
		narrowStartMs, _, narrowFound := parseEndCredits(renderBlackdetect(clipped, narrowStart), narrowStart, durationSec)

		if wideFound {
			require.True(t, narrowFound,
				"duration=%.0f regions=%v: the wide window accepted, the narrowed one must too", durationSec, regions)
			// Not bit-identical: blackdetect reports offsets relative to the
			// window start, so re-basing them onto a different -ss moves the
			// reconstructed absolute start by the rounding of one offset. A
			// millisecond on a credits marker is not a behaviour change.
			require.InDelta(t, wideStartMs, narrowStartMs, 2,
				"duration=%.0f regions=%v: same credits start", durationSec, regions)
		} else if narrowFound {
			gained++
		}
	}
	assert.Positive(t, gained, "the narrowed window should also unlock detections the wide one poisoned")
	t.Logf("layouts the narrowed window newly detects: %d", gained)
}

// renderBlackdetect writes regions back as the ffmpeg lines parseEndCredits
// reads, with offsets relative to windowStart like a real -ss decode.
func renderBlackdetect(regions []floatRange, windowStart float64) string {
	var b strings.Builder
	for _, r := range regions {
		fmt.Fprintf(&b, "[blackdetect @ 0x1] black_start:%.3f black_end:%.3f black_duration:%.3f\n",
			r.start-windowStart, r.end-windowStart, r.end-r.start)
	}
	return b.String()
}
