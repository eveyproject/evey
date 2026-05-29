package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseContentClass(t *testing.T) {
	cases := []struct {
		in   string
		want ContentClass
		ok   bool
	}{
		{"animation", ClassAnimation, true},
		{"ANIMATION", ClassAnimation, true},
		{" film_normal ", ClassFilmNormal, true},
		{"film_high_motion", ClassFilmHighMotion, true},
		{"sport", ClassSport, true},
		{"", "", false},
		{"junk", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseContentClass(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestLadderFor_UnknownClassFallsBackToNormal(t *testing.T) {
	got := LadderFor(ContentClass("garbage"))
	assert.Equal(t, LadderFilmNormal, got, "unknown class must fall back to LadderFilmNormal")
}

func TestLadderFor_EachKnownClassHasRungs(t *testing.T) {
	for _, c := range []ContentClass{
		ClassAnimation, ClassFilmLowMotion, ClassFilmNormal, ClassFilmHighMotion, ClassSport,
	} {
		ladder := LadderFor(c)
		assert.NotEmpty(t, ladder, "class %s must yield at least one rung", c)
	}
}

func TestCapLadderToSource_TrimsRungsAboveSource(t *testing.T) {
	// HighMotion has a 2160p rung. Source 1080p should cap it out.
	capped := CapLadderToSource(LadderFilmHighMotion, 1920, 1080)
	for _, p := range capped {
		assert.LessOrEqual(t, p.Height, 1080, "rung %s should not exceed 1080p source", p.Name)
	}
	assert.NotEmpty(t, capped, "cap must leave at least one rung")
}

func TestCapLadderToSource_LetterboxedSourceUsesLargerSide(t *testing.T) {
	// Letterboxed 1920×800 source: max side is 1920 so a 1080p rung
	// (1920×1080) must be kept (rung max = 1920, fits).
	capped := CapLadderToSource(LadderFilmHighMotion, 1920, 800)
	found1080 := false
	for _, p := range capped {
		if p.Height == 1080 {
			found1080 = true
		}
	}
	assert.True(t, found1080, "letterboxed source larger-side cap should keep the 1080p rung")
}

func TestCapLadderToSource_UnknownSourceLeavesLadderIntact(t *testing.T) {
	// Zero dims = "unknown source" → don't cap.
	capped := CapLadderToSource(LadderFilmHighMotion, 0, 0)
	assert.Equal(t, len(LadderFilmHighMotion), len(capped))
}

func TestCapLadderToSource_TinySourceKeepsAtLeastOne(t *testing.T) {
	// Pathological 200x150 source → every rung above. Must still return
	// at least one rung (the lowest pre-defined) for the player to have
	// something.
	capped := CapLadderToSource(LadderFilmNormal, 200, 150)
	assert.Len(t, capped, 1, "tiny source must return exactly the lowest rung")
}

func TestClassify_AnimationFromLowMetrics(t *testing.T) {
	m := ComplexityMetrics{
		MotionMean:         1.0,
		ScenecutsPerMinute: 3,
		SourceBitrateKbps:  2200,
		SourceHeight:       1080,
		FramesSampled:      30,
	}
	assert.Equal(t, ClassAnimation, Classify(m))
}

func TestClassify_4KHighBitrateForcesHighMotion(t *testing.T) {
	// 4K with 20 Mbps source — refuse to call it animation even if motion
	// is freakishly low. Bias toward high-motion ladder.
	m := ComplexityMetrics{
		MotionMean:         1.0,
		ScenecutsPerMinute: 3,
		SourceBitrateKbps:  20000,
		SourceHeight:       2160,
	}
	assert.Equal(t, ClassFilmHighMotion, Classify(m))
}

func TestClassify_4KLowBitrateBecomesNormal(t *testing.T) {
	// 4K with low bitrate (under-encoded master): fall back to normal
	// rather than animation (we'd under-allocate bandwidth on grain).
	m := ComplexityMetrics{
		MotionMean:         1.0,
		ScenecutsPerMinute: 3,
		SourceBitrateKbps:  6000,
		SourceHeight:       2160,
	}
	assert.Equal(t, ClassFilmNormal, Classify(m))
}

func TestClassify_DialogueDramaIsLowMotion(t *testing.T) {
	m := ComplexityMetrics{
		MotionMean:         2.5,
		ScenecutsPerMinute: 5,
		SourceBitrateKbps:  5500,
		SourceHeight:       1080,
		FramesSampled:      30,
	}
	assert.Equal(t, ClassFilmLowMotion, Classify(m))
}

func TestClassify_HighMotionAction(t *testing.T) {
	m := ComplexityMetrics{
		MotionMean:         7.0,
		ScenecutsPerMinute: 18,
		SourceBitrateKbps:  8000,
		SourceHeight:       1080,
		FramesSampled:      30,
	}
	assert.Equal(t, ClassFilmHighMotion, Classify(m))
}

func TestClassify_SustainedSportHasOwnClass(t *testing.T) {
	m := ComplexityMetrics{
		MotionMean:         6.0,
		ScenecutsPerMinute: 25,
		SourceBitrateKbps:  6000,
		SourceHeight:       1080,
		FramesSampled:      30,
	}
	assert.Equal(t, ClassSport, Classify(m))
}

func TestClassify_NormalIsTheSafeDefault(t *testing.T) {
	m := ComplexityMetrics{
		MotionMean:         4.0,
		ScenecutsPerMinute: 10,
		SourceBitrateKbps:  4500,
		SourceHeight:       1080,
		FramesSampled:      30,
	}
	assert.Equal(t, ClassFilmNormal, Classify(m))
}

func TestClassify_NoSamples_AnimationFromLowBitrate(t *testing.T) {
	// ffmpeg sample failed: only bitrate + resolution available.
	m := ComplexityMetrics{
		SourceBitrateKbps: 1800,
		SourceHeight:      1080,
		FramesSampled:     0,
	}
	assert.Equal(t, ClassAnimation, Classify(m))
}

func TestClassify_NoSamples_HighMotionFromHighBitrate(t *testing.T) {
	m := ComplexityMetrics{
		SourceBitrateKbps: 9000,
		SourceHeight:      1080,
		FramesSampled:     0,
	}
	assert.Equal(t, ClassFilmHighMotion, Classify(m))
}

func TestClassify_NoSamples_NormalIsFallback(t *testing.T) {
	m := ComplexityMetrics{
		SourceBitrateKbps: 4000,
		SourceHeight:      720,
		FramesSampled:     0,
	}
	assert.Equal(t, ClassFilmNormal, Classify(m))
}
