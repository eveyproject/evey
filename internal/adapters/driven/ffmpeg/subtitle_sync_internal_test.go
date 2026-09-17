package ffmpeg

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A synthetic window: ~60 irregularly spaced cues, and an envelope whose
// speech bursts sit `early` seconds BEFORE their cues over a -40 dB music
// bed. The measure must read `early` back as the lag (positive = the audio
// must move later), sign included.
func syntheticWindow(early float64) ([]float64, []subCue, float64) {
	const at, length = 100.0, 480.0
	n := int(length / subSyncBinSec)
	env := make([]float64, n)
	for i := range env {
		env[i] = -40
	}
	var cues []subCue
	seed := uint32(7)
	for start := at + 8.0; start < at+length-10; {
		seed = seed*1664525 + 1013904223 // deterministic irregular spacing
		dur := 1.2 + float64(seed>>24)/255*2.5
		cues = append(cues, subCue{start, start + dur})
		for t := start - early; t < start-early+dur; t += subSyncBinSec {
			if b := int((t - at) / subSyncBinSec); b >= 0 && b < n {
				env[b] = -22 + 3*math.Sin(t*17)
			}
		}
		seed = seed*1664525 + 1013904223
		start += dur + 2 + float64(seed>>24)/255*9
	}
	return env, cues, at
}

func TestCorrelateCues_ReadsTheShiftBack(t *testing.T) {
	for _, early := range []float64{0, 0.9, -1.4, 12} {
		env, cues, at := syntheticWindow(early)
		w := correlateCues(env, cues, at)
		assert.InDelta(t, early, w.lag, 0.05, "early %v", early)
		assert.GreaterOrEqual(t, w.cues, subSyncMinCues)
		assert.Greater(t, w.r, subSyncMinR)
		assert.Greater(t, w.prom, subSyncMinProm)
	}
}

func TestSettleWindows(t *testing.T) {
	good := func(lag float64) subSyncWindow { return subSyncWindow{lag: lag, r: 0.4, prom: 3, cues: 50} }
	// Two windows agreeing: the median, with the weakest r as confidence.
	lag, conf, agree, ok := settleWindows([]subSyncWindow{good(0.9), {lag: 0.92, r: 0.2, prom: 2, cues: 30}, good(-7)})
	require.True(t, ok)
	assert.InDelta(t, 0.9, lag, 0.03)
	assert.Equal(t, 2, agree)
	assert.InDelta(t, 0.2, conf, 1e-9)
	// One window alone never accuses a file; neither does a window short of cues.
	_, _, _, ok = settleWindows([]subSyncWindow{good(0.9), good(-7)})
	assert.False(t, ok)
	_, _, _, ok = settleWindows([]subSyncWindow{{lag: 0.9, r: 0.4, prom: 3, cues: 5}, {lag: 0.9, r: 0.4, prom: 3, cues: 5}})
	assert.False(t, ok)
}

func TestSubSyncWindows(t *testing.T) {
	ws := subSyncWindows(7000, 900)
	require.Len(t, ws, 3)
	assert.InDelta(t, subSyncWindowSec, ws[0].length, 1e-9)
	assert.GreaterOrEqual(t, ws[0].at, subSyncMaxLagSec)
	assert.Len(t, subSyncWindows(1800, 900), 2, "a short episode gets two windows")
	assert.Len(t, subSyncWindows(7000, 120), 2, "sparse cues widen the windows to halves")
}

func TestSrtSeconds(t *testing.T) {
	assert.InDelta(t, 3723.456, srtSeconds([]string{"1", "2", "3", "456"}), 1e-9)
	assert.True(t, isTextSubtitleCodec("subrip"))
	assert.False(t, isTextSubtitleCodec("hdmv_pgs_subtitle"))
}
