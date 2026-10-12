package library

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

const thumbSize = 32 * 18

func texturedFrame(r *rand.Rand) []byte {
	f := make([]byte, thumbSize)
	for i := range f {
		f[i] = byte(r.Intn(256))
	}
	return f
}

func flatFrame(v byte) []byte {
	f := make([]byte, thumbSize)
	for i := range f {
		f[i] = v
	}
	return f
}

// timeline builds one episode's frames (4 fps) over [0, totalSec): the shared title
// frames over [titleStart, titleStart+len(title)/4), unique content elsewhere.
func timeline(r *rand.Rand, totalSec, titleStart float64, title [][]byte) [][]byte {
	n := int(totalSec * edgeFPS)
	out := make([][]byte, n)
	at := int(titleStart * edgeFPS)
	for i := range out {
		if k := i - at; k >= 0 && k < len(title) {
			out[i] = title[k]
		} else {
			out[i] = texturedFrame(r)
		}
	}
	return out
}

func sharedTitle(r *rand.Rand, sec float64) [][]byte {
	t := make([][]byte, int(sec*edgeFPS))
	for i := range t {
		t[i] = texturedFrame(r)
	}
	return t
}

// Billions S06 shape: the audio matched the middle 12s of a 20s title sequence.
func TestWidenFrames_ExtendsToTheSharedTitleBothWays(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	title := sharedTitle(r, 20)
	a := timeline(r, 200, 82, title) // title 82-102
	b := timeline(r, 200, 75, title) // title 75-95
	inner := int(edgeInnerSec * edgeFPS)
	// audio window 85-97 in a, 78-90 in b
	n, diff, _, judged := widenFrames(a, b, 85*4, 78*4, -1, inner)
	require.True(t, judged)
	require.LessOrEqual(t, diff, edgeMatchMaxDiff)
	require.Equal(t, 12, n, "start: 3s back to 82")
	n, _, _, _ = widenFrames(a, b, 97*4, 90*4, +1, inner)
	require.Equal(t, 20, n, "end: 5s on to 102")
}

func TestWidenFrames_SharedBlackDoesNotWiden(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(5))
	title := sharedTitle(r, 12)
	for range 12 { // 3s of black right after the title, in both episodes
		title = append(title, flatFrame(16))
	}
	a := timeline(r, 200, 85, title)
	b := timeline(r, 200, 78, title)
	n, _, _, _ := widenFrames(a, b, 97*4, 90*4, +1, int(edgeInnerSec*edgeFPS))
	require.Equal(t, 0, n)
}

func TestWidenFrames_NoMatchInsideSaysNothing(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(9))
	a := timeline(r, 200, 0, nil)
	b := timeline(r, 200, 0, nil)
	n, _, corr, judged := widenFrames(a, b, 85*4, 78*4, -1, int(edgeInnerSec*edgeFPS))
	require.True(t, judged)
	require.Less(t, corr, edgeRejectMaxCorr, "different pictures inside the edge: the neighbour contradicts the intro")
	require.Equal(t, 0, n)
}

// The service decodes each edge window through the detector and keeps the largest
// neighbour extension: c's missing first second does not hold a and b back.
func TestWidenHintsByFrames_KeepsTheLargestNeighbourExtension(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(13))
	title := sharedTitle(r, 20)
	full := map[string][][]byte{
		"a": timeline(r, 300, 82, title),     // title 82-102
		"b": timeline(r, 300, 75, title),     // title 75-95
		"c": timeline(r, 300, 90, title[4:]), // title 89-109 without its first second: 90-109
	}
	det := &fakeDetector{thumbs: func(path string, startSec, durSec, fps float64) [][]byte {
		f := full[path]
		lo, hi := int(startSec*fps), min(len(f), int((startSec+durSec)*fps))
		return f[lo:hi]
	}}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, det, "/tmp/trickplay")
	hints := map[string]domainlibrary.MediaSegment{
		"a": {Type: "intro", StartMs: 85_000, EndMs: 97_000},
		"b": {Type: "intro", StartMs: 78_000, EndMs: 90_000},
		"c": {Type: "intro", StartMs: 92_000, EndMs: 104_000},
	}
	svc.widenHintsByFrames(context.Background(), []string{"a", "b", "c"}, map[string]string{"a": "a", "b": "b", "c": "c"}, hints, true)
	require.Equal(t, domainlibrary.MediaSegment{Type: "intro", StartMs: 82_000, EndMs: 102_000}, hints["a"])
	require.Equal(t, domainlibrary.MediaSegment{Type: "intro", StartMs: 75_000, EndMs: 95_000}, hints["b"])
	require.Equal(t, domainlibrary.MediaSegment{Type: "intro", StartMs: 90_000, EndMs: 109_000}, hints["c"])
}

// A dark title sequence has little contrast at 32x18 (std ~5 on Billions S06) yet is
// not black: it must widen, where a truly flat frame must not.
func TestWidenFrames_DarkTitleStillWidens(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(21))
	title := make([][]byte, 20*4)
	for i := range title {
		f := make([]byte, thumbSize)
		for p := range f {
			f[p] = byte(10 + r.Intn(18)) // std ~5, mean ~19
		}
		title[i] = f
	}
	require.Greater(t, frameStd(title[0]), edgeFlatMaxStd)
	a := timeline(r, 200, 82, title)
	b := timeline(r, 200, 75, title)
	n, _, _, _ := widenFrames(a, b, 85*4, 78*4, -1, int(edgeInnerSec*edgeFPS))
	require.Equal(t, 12, n)
}

// A recurring music cue under different scenes clusters on audio; its pictures match
// no neighbour, so the hint is dropped. A carrier whose neighbours fail to decode keeps
// its hint: a decode error must never delete an intro.
func TestWidenHintsByFrames_DropsAHintNoNeighbourPictureConfirms(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(17))
	full := map[string][][]byte{"a": timeline(r, 300, 0, nil), "b": timeline(r, 300, 0, nil), "c": timeline(r, 300, 0, nil)}
	det := &fakeDetector{thumbs: func(path string, startSec, durSec, fps float64) [][]byte {
		f, ok := full[path]
		if !ok {
			return nil
		}
		lo, hi := int(startSec*fps), min(len(f), int((startSec+durSec)*fps))
		return f[lo:hi]
	}}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, det, "/tmp/trickplay")
	hints := map[string]domainlibrary.MediaSegment{
		"a": {Type: "intro", StartMs: 100_000, EndMs: 115_000},
		"b": {Type: "intro", StartMs: 200_000, EndMs: 215_000},
		"c": {Type: "intro", StartMs: 150_000, EndMs: 165_000},
		"x": {Type: "intro", StartMs: 150_000, EndMs: 165_000}, // no decodable frames anywhere
	}
	svc.widenHintsByFrames(context.Background(), []string{"a", "b", "c"}, map[string]string{"a": "a", "b": "b", "c": "c"}, hints, true)
	require.NotContains(t, hints, "a")
	require.NotContains(t, hints, "b")
	require.NotContains(t, hints, "c")

	lone := map[string]domainlibrary.MediaSegment{"x": {Type: "intro", StartMs: 150_000, EndMs: 165_000}, "y": {Type: "intro", StartMs: 150_000, EndMs: 165_000}}
	svc.widenHintsByFrames(context.Background(), []string{"x", "y"}, map[string]string{"x": "missing", "y": "missing2"}, lone, true)
	require.Contains(t, lone, "x", "nothing decoded, nothing compared: the audio verdict stands")
}

// Two dark passages always look alike: a dark edge neither confirms nor contradicts.
func TestWidenFrames_DarkInsideIsNoVerdict(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(23))
	a := timeline(r, 200, 0, nil)
	b := timeline(r, 200, 0, nil)
	for i := 85 * 4; i < 90*4; i++ {
		a[i], b[i] = flatFrame(18), flatFrame(20)
	}
	n, _, _, judged := widenFrames(a, b, 85*4, 85*4, -1, int(edgeInnerSec*edgeFPS))
	require.False(t, judged)
	require.Equal(t, 0, n)
}

// An edge that is a transition (fade, per-episode credit names) differs a little from
// every neighbour: too much to widen, not enough to reject (Neagley S01E03 scored 12.9).
func TestWidenHintsByFrames_KeepsAnIntroWhoseEdgesDifferSlightly(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(29))
	title := sharedTitle(r, 20)
	noisy := func(src [][]byte) [][]byte {
		out := make([][]byte, len(src))
		for i, f := range src {
			g := make([]byte, len(f))
			for p, v := range f {
				d := int(v) + r.Intn(61) - 30 // uniform +-30: mean |diff| ~15, above 12, below 18
				g[p] = byte(max(0, min(255, d)))
			}
			out[i] = g
		}
		return out
	}
	full := map[string][][]byte{"a": timeline(r, 300, 82, title), "b": timeline(r, 300, 75, noisy(title))}
	det := &fakeDetector{thumbs: func(path string, startSec, durSec, fps float64) [][]byte {
		f := full[path]
		lo, hi := int(startSec*fps), min(len(f), int((startSec+durSec)*fps))
		return f[lo:hi]
	}}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, det, "/tmp/trickplay")
	hints := map[string]domainlibrary.MediaSegment{
		"a": {Type: "intro", StartMs: 85_000, EndMs: 97_000},
		"b": {Type: "intro", StartMs: 78_000, EndMs: 90_000},
	}
	svc.widenHintsByFrames(context.Background(), []string{"a", "b"}, map[string]string{"a": "a", "b": "b"}, hints, true)
	require.Equal(t, domainlibrary.MediaSegment{Type: "intro", StartMs: 85_000, EndMs: 97_000}, hints["a"], "kept, not widened")
}

// A long or stable signature is trusted whatever the pictures say: only the ambiguous
// audio profile can be vetoed (a series' S05, 62s, correlates 0.44 at its edges).
func TestWidenHintsByFrames_NeverRejectsAnUnambiguousSignature(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(31))
	full := map[string][][]byte{"a": timeline(r, 300, 0, nil), "b": timeline(r, 300, 0, nil)}
	det := &fakeDetector{thumbs: func(path string, startSec, durSec, fps float64) [][]byte {
		f := full[path]
		lo, hi := int(startSec*fps), min(len(f), int((startSec+durSec)*fps))
		return f[lo:hi]
	}}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, det, "/tmp/trickplay")
	hints := map[string]domainlibrary.MediaSegment{
		"a": {Type: "intro", StartMs: 100_000, EndMs: 160_000},
		"b": {Type: "intro", StartMs: 200_000, EndMs: 260_000},
	}
	svc.widenHintsByFrames(context.Background(), []string{"a", "b"}, map[string]string{"a": "a", "b": "b"}, hints, false)
	require.Contains(t, hints, "a")
	require.Contains(t, hints, "b")
}

func TestAmbiguousSignature(t *testing.T) {
	t.Parallel()
	require.True(t, ambiguousSignature(13.9, []float64{640, 631, 505, 480, 118, 508, 570}), "City of Blood: short, scattered")
	require.False(t, ambiguousSignature(13.9, []float64{85, 67, 74, 84, 62, 78, 71, 75, 44, 78, 64}), "Billions: short but stable")
	require.False(t, ambiguousSignature(61.7, []float64{221, 589, 300, 450}), "series S05: long")
}

func TestFrameCorr_IgnoresContrast(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(37))
	a := texturedFrame(r)
	dim := make([]byte, len(a))
	for i, v := range a {
		dim[i] = v / 8 // the same picture, much darker
	}
	require.Greater(t, frameCorr(a, dim), 0.99)
	require.Less(t, frameCorr(a, texturedFrame(r)), 0.2)
}
