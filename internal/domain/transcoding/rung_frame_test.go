package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The backlog symptom: a 640x480 source capped to maxHeight=360 was served a
// rung taller than the cap. No rung of the high-motion ladder fits under 360,
// and the fallback kept the lowest rung (854x480) whole.
func TestCapLadderToHeight_FallbackNeverExceedsTheCap(t *testing.T) {
	got := CapLadderToHeight(LadderFilmHighMotion, 360)
	require.Len(t, got, 1)
	assert.LessOrEqual(t, got[0].Height, 360, "the fallback rung must fit under the cap")
	assert.LessOrEqual(t, got[0].Width, 854, "shrunk, never grown")
	assert.Equal(t, 0, got[0].Height%2, "even height")
	assert.Equal(t, 0, got[0].Width%2, "even width")
}

// A 4:3 source at 640x480 fits the 480p box (854x480) exactly: its picture in
// that box IS the source. Comparing the widest sides (854 > 640) dropped it.
func TestCapLadderToSource_NarrowSourceKeepsTheBoxItFills(t *testing.T) {
	got := CapLadderToSource(LadderFilmNormal, 640, 480)
	names := make([]string, 0, len(got))
	for _, p := range got {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"360p", "480p"}, names)
}

// The end-to-end shape of the symptom: the ladder built for a 640x480 source
// under a 360 delivery cap never carries a rung above the cap.
func TestBuild_NarrowSourceUnderHeightCapStaysUnderIt(t *testing.T) {
	for _, class := range []ContentClass{ClassFilmNormal, ClassFilmHighMotion, ClassSport} {
		in := LadderInput{
			Source:     SourceFacts{Width: 640, Height: 480, Codec: "h264"},
			Complexity: ComplexityVector{Class: class},
			Route:      EncodeRoute{Codec: "h264"},
			Delivery:   DeliveryConstraints{MaxHeight: 360},
		}
		for _, p := range (StaticLadderBuilder{}).Build(in) {
			assert.LessOrEqual(t, p.Height, 360, "%s: rung %s", class, p.Name)
		}
	}
}

// The picture a rung is drawn at: the source's display aspect fitted in the
// box, both sides even, never above the source. The wide cases are the ones
// ffmpeg's scale=W:-2 already drew (measured: 1920x800 at 1280:-2 is
// 1280x534) and must not move; the narrow ones are the bug.
func TestRungFrame(t *testing.T) {
	box := func(w, h int) HLSProfile { return HLSProfile{Width: w, Height: h} }
	for _, tt := range []struct {
		name         string
		box          HLSProfile
		src          SourceFacts
		wantW, wantH int
	}{
		{"16:9 keeps its box (360p)", box(640, 360), SourceFacts{Width: 1920, Height: 1080}, 640, 360},
		{"16:9 keeps its box (480p)", box(854, 480), SourceFacts{Width: 1920, Height: 1080}, 854, 480},
		{"16:9 keeps its box (2160p)", box(3840, 2160), SourceFacts{Width: 3840, Height: 2160}, 3840, 2160},
		{"4:3 pins the height (360p)", box(640, 360), SourceFacts{Width: 640, Height: 480}, 480, 360},
		{"4:3 fills the 480p box at its own size", box(854, 480), SourceFacts{Width: 640, Height: 480}, 640, 480},
		{"4:3 HD in the 720p box", box(1280, 720), SourceFacts{Width: 1440, Height: 1080}, 960, 720},
		{"scope source at its own height (1080p)", box(1920, 1080), SourceFacts{Width: 1920, Height: 800}, 1920, 800},
		{"scope source, height rounded like -2 (720p)", box(1280, 720), SourceFacts{Width: 1920, Height: 800}, 1280, 534},
		{"scope source, height rounded like -2 (480p)", box(854, 480), SourceFacts{Width: 1920, Height: 800}, 854, 356},
		{"scope source, height rounded like -2 (360p)", box(640, 360), SourceFacts{Width: 1920, Height: 800}, 640, 266},
		{"anamorphic 16:9 PAL DVD", box(640, 360), SourceFacts{Width: 720, Height: 576, SARNum: 64, SARDen: 45}, 640, 360},
		{"anamorphic 4:3 PAL DVD", box(640, 360), SourceFacts{Width: 720, Height: 576, SARNum: 16, SARDen: 15}, 480, 360},
		{"never above a tiny source", box(640, 360), SourceFacts{Width: 320, Height: 240}, 320, 240},
		{"odd source stays even", box(1920, 1080), SourceFacts{Width: 1917, Height: 1079}, 1916, 1078},
		{"unknown source keeps the box", box(1280, 720), SourceFacts{}, 1280, 720},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w, h := RungFrame(tt.box, tt.src)
			assert.Equal(t, tt.wantW, w, "width")
			assert.Equal(t, tt.wantH, h, "height")
			assert.LessOrEqual(t, w, tt.box.Width, "inside the box")
			assert.LessOrEqual(t, h, tt.box.Height, "inside the box")
			assert.Zero(t, w%2, "even width")
			assert.Zero(t, h%2, "even height")
		})
	}
}

// The symptom end to end: the 360-capped 640x480 ladder is stamped 480x360,
// never 854x640; the scope source keeps the 1920x800 it always drew; a copy
// rung (the source itself) stays unframed; the input ladder is not mutated.
func TestFrameLadderToSource(t *testing.T) {
	capped := CapLadderToHeight(CapLadderToSource(LadderFilmHighMotion, 640, 480), 360)
	framed := FrameLadderToSource(capped, SourceFacts{Width: 640, Height: 480})
	require.Len(t, framed, 1)
	w, h := framed[0].Frame()
	assert.Equal(t, [2]int{480, 360}, [2]int{w, h})
	assert.True(t, framed[0].Framed())

	scope := FrameLadderToSource(LadderFilmNormal, SourceFacts{Width: 1920, Height: 800})
	w, h = scope[len(scope)-1].Frame()
	assert.Equal(t, [2]int{1920, 800}, [2]int{w, h})
	assert.Zero(t, LadderFilmNormal[3].FrameWidth, "package-level ladder untouched")

	copyRung := FrameLadderToSource(CopyRung(1920, 800), SourceFacts{Width: 1920, Height: 800})
	assert.False(t, copyRung[0].Framed(), "a copy rung is the source, never framed")

	unknown := FrameLadderToSource(LadderFilmNormal, SourceFacts{})
	assert.False(t, unknown[0].Framed(), "an unknown source leaves the rungs unframed")
	w, h = unknown[0].Frame()
	assert.Equal(t, [2]int{640, 360}, [2]int{w, h}, "an unframed rung reads as its box")
}

// The symmetric case of the source cap: a scope source must keep every box it
// fills at or under its own height, exactly as the widest-side rule kept them.
func TestCapLadderToSource_ScopeSourceKeepsItsLadder(t *testing.T) {
	got := CapLadderToSource(LadderFilmHighMotion, 1920, 800)
	names := make([]string, 0, len(got))
	for _, p := range got {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"480p", "720p", "1080p"}, names)
}

// A shrunk fallback rung drops any frame it carried: the frame belonged to the
// box it no longer has.
func TestCapLadderToHeight_ShrunkFallbackIsUnframed(t *testing.T) {
	framed := FrameLadderToSource(LadderFilmHighMotion, SourceFacts{Width: 640, Height: 480})
	got := CapLadderToHeight(framed, 360)
	require.Len(t, got, 1)
	assert.False(t, got[0].Framed())
	assert.Equal(t, 640, got[0].Width)
	assert.Equal(t, 360, got[0].Height)
	assert.Equal(t, "360p", got[0].Name)
}
