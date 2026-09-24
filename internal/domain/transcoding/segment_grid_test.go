package transcoding

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The broadcast rates the probes report as floats map back to their exact
// rationals, and a segment is a whole number of frames whose length is exact
// in ticks: 72 frames at 23.976 fps are 72072 ticks at 24000, i.e. 3.003 s.
func TestGridForFrameRate_FrameExactGrids(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fps               float64
		num, den, frames  int
		timescale         int
		segTicks          int64
		segSeconds        float64
		extinf            string
		frameRate, hlsTim string
	}{
		{23.976, 24000, 1001, 72, 24000, 72072, 3.003, "3.003", "24000/1001", "3.003000"},
		{23.976023976, 24000, 1001, 72, 24000, 72072, 3.003, "3.003", "24000/1001", "3.003000"},
		{24, 24, 1, 72, 12288, 36864, 3, "3.000", "24", "3.000000"},
		{25, 25, 1, 75, 12800, 38400, 3, "3.000", "25", "3.000000"},
		{29.97, 30000, 1001, 90, 30000, 90090, 3.003, "3.003", "30000/1001", "3.003000"},
		{30, 30, 1, 90, 15360, 46080, 3, "3.000", "30", "3.000000"},
		{50, 50, 1, 150, 25600, 76800, 3, "3.000", "50", "3.000000"},
		{59.94, 60000, 1001, 180, 60000, 180180, 3.003, "3.003", "60000/1001", "3.003000"},
	}
	for _, c := range cases {
		g := GridForFrameRate(c.fps)
		require.True(t, g.Exact(), "%v fps must be exact", c.fps)
		assert.Equal(t, c.num, g.FPSNum, "%v", c.fps)
		assert.Equal(t, c.den, g.FPSDen, "%v", c.fps)
		assert.Equal(t, c.frames, g.FramesPerSegment, "%v", c.fps)
		assert.Equal(t, c.timescale, g.Timescale, "%v", c.fps)
		assert.Equal(t, c.segTicks, g.SegmentTicks(), "%v", c.fps)
		assert.InDelta(t, c.segSeconds, g.SegmentSeconds(), 1e-9, "%v", c.fps)
		assert.Equal(t, c.extinf, formatExtinf(g.SegmentDurationSeconds(0, 3_600_000)), "%v", c.fps)
		assert.Equal(t, c.frameRate, g.FrameRateString(), "%v", c.fps)
		assert.Equal(t, c.hlsTim, g.HLSTime(), "%v", c.fps)
		assert.Equal(t, "expr:eq(mod(n,"+itoa(c.frames)+"),0)", g.ForceKeyframesExpr())
		// A frame is a whole number of ticks, whatever the rate.
		assert.Equal(t, int64(0), int64(g.Timescale)*int64(g.FPSDen)%int64(g.FPSNum), "%v: frame must be whole ticks", c.fps)
	}
}

// Anything the grid cannot make exact falls back to the legacy 3-second
// clock grid, which hashes as nothing so existing caches keep their names.
func TestGridForFrameRate_LegacyForUnknownOrExoticRates(t *testing.T) {
	t.Parallel()
	for _, fps := range []float64{0, -1, 23.5, 27.3, 1000} {
		g := GridForFrameRate(fps)
		assert.False(t, g.Exact(), "%v", fps)
		assert.Empty(t, g.String(), "%v", fps)
		assert.InDelta(t, 3.0, g.SegmentSeconds(), 0)
		assert.Equal(t, "3", g.HLSTime())
		assert.Equal(t, "expr:gte(t,n_forced*3)", g.ForceKeyframesExpr())
		assert.Equal(t, int64(0), g.SegmentTicks())
	}
}

// Segment count and the last segment follow the frame count, not a float
// division: a 20 min title at 23.976 fps holds 28772 frames, 399 full
// segments of 72 and a last one of 44 frames.
func TestSegmentGrid_CountsAndLastSegmentFromFrames(t *testing.T) {
	t.Parallel()
	g := GridForFrameRate(23.976)
	const dur = int64(1_200_042)
	assert.Equal(t, int64(28772), g.FrameCount(dur))
	assert.Equal(t, 400, g.SegmentCount(dur))
	assert.Equal(t, int64(72072), g.SegmentDurationTicks(0, dur))
	assert.Equal(t, int64(72072), g.SegmentDurationTicks(398, dur))
	assert.Equal(t, int64(44*1001), g.SegmentDurationTicks(399, dur), "the leftover frames")
	assert.Equal(t, int64(0), g.SegmentDurationTicks(400, dur), "past the end")
	assert.InDelta(t, 1.835, g.SegmentDurationSeconds(399, dur), 1e-9)
	assert.Equal(t, int64(399*72072), g.SegmentStartTicks(399))
	assert.InDelta(t, 399*3.003, g.SegmentStartSeconds(399), 1e-6)
	assert.Equal(t, 39, g.SegmentIndexAt(39*3.003+0.5))
	assert.Equal(t, 38, g.SegmentIndexAt(39*3.003-0.001))

	// A duration that is an exact multiple has no short tail.
	g25 := GridForFrameRate(25)
	assert.Equal(t, 400, g25.SegmentCount(1_200_000))
	assert.Equal(t, int64(38400), g25.SegmentDurationTicks(399, 1_200_000))

	// The legacy grid keeps its former arithmetic.
	var legacy SegmentGrid
	assert.Equal(t, 401, legacy.SegmentCount(1_200_042))
	assert.InDelta(t, 0.042, legacy.SegmentDurationSeconds(400, 1_200_042), 1e-9)
	assert.InDelta(t, 3, legacy.SegmentDurationSeconds(3, 1_200_042), 0)
	assert.Equal(t, 0, legacy.SegmentCount(0))
}

// The rungs of a ladder share one grid, so their timelines agree segment for
// segment in ticks whatever the rung - the invariant a DASH switch set needs.
func TestSegmentGrid_SameTimelineForEveryRung(t *testing.T) {
	t.Parallel()
	g := GridForFrameRate(29.97)
	const dur = int64(7_200_000)
	for n := range g.SegmentCount(dur) {
		assert.Equal(t, int64(n)*90090, g.SegmentStartTicks(n))
	}
	assert.Equal(t, "30000/1001:90@30000", g.String())
}

func formatExtinf(sec float64) string { return fmt.Sprintf("%.3f", sec) }
func itoa(n int) string               { return strconv.Itoa(n) }
