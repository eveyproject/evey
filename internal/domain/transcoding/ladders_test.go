package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	assert.Len(t, capped, len(LadderFilmHighMotion))
}

func TestCapLadderToSource_TinySourceKeepsAtLeastOne(t *testing.T) {
	// Pathological 200x150 source → every rung above. Must still return
	// at least one rung (the lowest pre-defined) for the player to have
	// something.
	capped := CapLadderToSource(LadderFilmNormal, 200, 150)
	assert.Len(t, capped, 1, "tiny source must return exactly the lowest rung")
}

// capHeightTestLadder is a deterministic 4-rung ladder so CapLadderToHeight
// assertions don't depend on the exact composition of the production ladders.
var capHeightTestLadder = []HLSProfile{
	{Name: "360p", Height: 360},
	{Name: "480p", Height: 480},
	{Name: "720p", Height: 720},
	{Name: "1080p", Height: 1080},
}

func TestCapLadderToHeight_ZeroMeansNoCap(t *testing.T) {
	got := CapLadderToHeight(capHeightTestLadder, 0)
	assert.Len(t, got, len(capHeightTestLadder), "maxHeight<=0 must not cap")
}

func TestCapLadderToHeight_DropsRungsAboveCap(t *testing.T) {
	// The HDR software-tonemap safety valve: a weak host caps the ladder to
	// SoftwareHDRMaxHeight (e.g. 480p) so it stays real-time on 4K HDR.
	got := CapLadderToHeight(capHeightTestLadder, 480)
	assert.Len(t, got, 2, "480p cap keeps only the 360p + 480p rungs")
	for _, p := range got {
		assert.LessOrEqual(t, p.Height, 480, "rung %s (%dp) must not exceed the cap", p.Name, p.Height)
	}
}

func TestCapLadderToHeight_CapOnBoundaryKeepsThatRung(t *testing.T) {
	got := CapLadderToHeight(capHeightTestLadder, 720)
	assert.Len(t, got, 3, "a 720p cap keeps 360/480/720 and drops 1080")
	assert.Equal(t, 720, got[len(got)-1].Height)
}

func TestCapLadderToHeight_CapAboveTopKeepsAll(t *testing.T) {
	got := CapLadderToHeight(capHeightTestLadder, 10000)
	assert.Len(t, got, len(capHeightTestLadder), "a cap above every rung keeps the whole ladder")
}

func TestCapLadderToHeight_BelowLowestKeepsExactlyOne(t *testing.T) {
	// A cap below the smallest rung must still return one rung (never empty),
	// so even a pathologically weak host is offered the lowest quality.
	got := CapLadderToHeight(capHeightTestLadder, 1)
	assert.Len(t, got, 1, "cap below the lowest rung returns exactly the lowest rung")
	assert.Equal(t, 360, got[0].Height)
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

// boostTestLadder carries distinct bitrates so the boost assertions are
// unambiguous about WHICH rung's budget was adopted.
var boostTestLadder = []HLSProfile{
	{Name: "360p", Height: 360, VideoBitrate: 800, AudioBitrate: 96},
	{Name: "480p", Height: 480, VideoBitrate: 1400, AudioBitrate: 128},
	{Name: "720p", Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
	{Name: "1080p", Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
}

// The user-visible contract: a 4K HDR source on a box whose HDR ceiling is
// 720p serves 720p at the 1080p rung's bitrate — the budget the viewer lost
// to the cap is spent at the height the box sustains.
func TestBoostCappedTopRung_TopAdoptsFirstDroppedBudget(t *testing.T) {
	capped := CapLadderToHeight(boostTestLadder, 720)
	got := BoostCappedTopRung(boostTestLadder, capped)

	assert.Len(t, got, 3)
	assert.Equal(t, int64(5000), got[2].VideoBitrate, "720p rung adopts the dropped 1080p rung's video budget")
	assert.Equal(t, "720p", got[2].Name, "name/height unchanged — only the bit budget moves")
	assert.Equal(t, 720, got[2].Height)
	assert.Equal(t, int64(128), got[2].AudioBitrate, "audio budget untouched (video grain is the loss, not audio)")
	assert.Equal(t, int64(800), got[0].VideoBitrate, "lower rungs untouched")
	assert.Equal(t, int64(1400), got[1].VideoBitrate)
}

// First dropped, not highest dropped: a 480p cap adopts the 720p budget,
// not the 1080p one — the next rung up is what the viewer actually lost.
func TestBoostCappedTopRung_FirstDroppedNotHighest(t *testing.T) {
	capped := CapLadderToHeight(boostTestLadder, 480)
	got := BoostCappedTopRung(boostTestLadder, capped)

	assert.Len(t, got, 2)
	assert.Equal(t, int64(2800), got[1].VideoBitrate, "480p adopts the 720p budget, not 1080p's")
}

func TestBoostCappedTopRung_NoDropNoBoost(t *testing.T) {
	capped := CapLadderToHeight(boostTestLadder, 1080)
	got := BoostCappedTopRung(boostTestLadder, capped)
	assert.Equal(t, capped, got, "nothing dropped ⇒ ladder unchanged")
}

// The single-lowest-rung fallback (cap below the lowest rung) still boosts:
// the bit budget is nearly free for the encoder, only the height is costly.
func TestBoostCappedTopRung_SingleRungFallbackBoosts(t *testing.T) {
	capped := CapLadderToHeight(boostTestLadder, 1)
	got := BoostCappedTopRung(boostTestLadder, capped)

	assert.Len(t, got, 1)
	assert.Equal(t, 360, got[0].Height)
	assert.Equal(t, int64(1400), got[0].VideoBitrate, "lowest rung adopts the 480p budget")
}

// CapLadderToHeight can return a sub-slice of a PACKAGE-LEVEL ladder (the
// no-cap and lowest-rung fallback paths) — the boost must copy, never mutate.
func TestBoostCappedTopRung_NeverMutatesInputs(t *testing.T) {
	before := make([]HLSProfile, len(boostTestLadder))
	copy(before, boostTestLadder)

	capped := CapLadderToHeight(boostTestLadder, 1) // aliasing fallback: ladder[:1]
	_ = BoostCappedTopRung(boostTestLadder, capped)

	assert.Equal(t, before, boostTestLadder, "package-level ladder must not be mutated through the aliased slice")
}

func TestBoostCappedTopRung_EmptyAndDegenerate(t *testing.T) {
	assert.Empty(t, BoostCappedTopRung(boostTestLadder, nil))
	assert.Equal(t, boostTestLadder, BoostCappedTopRung(boostTestLadder, boostTestLadder), "identical slices ⇒ unchanged")
}
