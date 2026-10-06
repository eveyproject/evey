package loadtest

import (
	"fmt"
	"testing"
	"time"
)

// A library-shaped set of titles whose bitrate quartile and encode class vary
// independently: 4 bitrate bands x 4 cost classes, 10 titles each.
func lanedCandidates() []MediaCandidate {
	classes := []struct {
		codec  string
		height int
		rng    string
	}{
		{"h264", 1080, RangeSDR}, {"hevc", 1080, RangeSDR}, {"hevc", 2160, RangeHDR10}, {"hevc", 2160, RangeDoVi},
	}
	out := make([]MediaCandidate, 0, 160)
	for b := range 4 {
		for c, cl := range classes {
			for i := range 10 {
				out = append(out, MediaCandidate{
					ID: fmt.Sprintf("b%dc%d-%d", b, c, i), DurationSec: 7200,
					BitrateBps: int64(b+1) * 5_000_000, VideoCodec: cl.codec, VideoHeight: cl.height, VideoRange: cl.rng,
				})
			}
		}
	}
	return out
}

// Under the 50/50 pattern each type watches the library's own mix: the direct
// viewers every bitrate band, the encoded ones every cost class, each within
// one title of its share at every audience size. One shared draw cycling four
// bands handed the encodes bands 2 and 4 only.
func TestLanedPool_BalancedPatternGivesEachTypeTheLibrarysMix(t *testing.T) {
	in := lanedCandidates()
	direct, _, _ := representativeDraw(in, 3)
	encoded, _ := costDraw(in, 4)
	pool := newLanedPool(in, map[viewerType][]MediaCandidate{viewerTypeMkvgo: direct, viewerTypeTranscode: encoded}, time.Hour)

	band := func(m MediaCandidate) string { return fmt.Sprint(m.BitrateBps) }
	got := map[viewerType]map[string]int{viewerTypeMkvgo: {}, viewerTypeTranscode: {}}
	n := map[viewerType]int{}
	for slot := 1; slot <= 120; slot++ {
		vt := scenarioViewerType(ScenarioBalanced, slot)
		asg, ok := pool.takeFor(vt)
		if !ok {
			t.Fatalf("slot %d: pool exhausted", slot)
		}
		key := band(asg.media)
		if vt == viewerTypeTranscode {
			key = costClass(asg.media)
		}
		got[vt][key]++
		n[vt]++
		for k, c := range got[vt] {
			if d := float64(c) - 0.25*float64(n[vt]); d > 1 || d < -1 {
				t.Fatalf("slot %d: %s viewers hold %d of %q after %d, %.1f expected", slot, vt, c, k, n[vt], 0.25*float64(n[vt]))
			}
		}
	}
	if len(got[viewerTypeMkvgo]) != 4 || len(got[viewerTypeTranscode]) != 4 {
		t.Fatalf("direct saw %d bands and encoded %d classes, want 4 and 4", len(got[viewerTypeMkvgo]), len(got[viewerTypeTranscode]))
	}
}

// A title mkvgo cannot package is retired from the direct lane only: an
// encoded viewer may still be handed it.
func TestLanedPool_RetiringATitleForOneTypeKeepsItForTheOthers(t *testing.T) {
	in := []MediaCandidate{{ID: "x", DurationSec: 7200}}
	pool := newLanedPool(in, map[viewerType][]MediaCandidate{viewerTypeMkvgo: in, viewerTypeTranscode: in}, time.Hour)
	pool.markIneligibleFor(viewerTypeMkvgo, "x")
	if asg, ok := pool.takeFor(viewerTypeMkvgo); ok {
		t.Fatalf("direct lane handed out %q after retiring it", asg.media.ID)
	}
	if asg, ok := pool.takeFor(viewerTypeTranscode); !ok || asg.media.ID != "x" {
		t.Fatalf("encoded lane = %v %v, want x", asg.media.ID, ok)
	}
}

// The encode class reads the codec, the height rung and the dynamic range;
// an unprobed source is a class of its own, an unknown range counts as SDR.
func TestCostClass(t *testing.T) {
	cases := []struct {
		in   MediaCandidate
		want string
	}{
		{MediaCandidate{VideoCodec: "hevc", VideoHeight: 2160, VideoRange: RangeHDR10}, "hevc 2160p hdr10"},
		{MediaCandidate{VideoCodec: "h264", VideoHeight: 800}, "h264 1080p sdr"},
		{MediaCandidate{VideoCodec: "mpeg2video", VideoHeight: 576, VideoRange: RangeSDR}, "mpeg2video sd sdr"},
		{MediaCandidate{VideoCodec: "hevc", VideoHeight: 1608, VideoRange: RangeDoVi5}, "hevc 2160p dovi5"},
		{MediaCandidate{}, "unknown"},
	}
	for _, c := range cases {
		if got := costClass(c.in); got != c.want {
			t.Errorf("costClass(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
