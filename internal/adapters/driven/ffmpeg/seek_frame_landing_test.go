package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A 4K film, 24000/1001, Matroska millisecond timestamps around a
// `-ss 279.000` seek: packets from the keyframe before the target, in decode
// order (B-frames out of presentation order), as ffprobe lists them.
var remuxPacketsAt279 = []float64{278.945, 279.154, 279.070, 279.029, 278.987, 279.112, 279.279, 279.196, 279.237, 279.404}

func TestSeekLandingFromPackets_FirstFrameAtOrAfterTarget_SnappedToGrid(t *testing.T) {
	us, ok := seekLandingFromPackets(remuxPacketsAt279, 279.0, 24000, 1001)
	if !ok {
		t.Fatal("expected a landing")
	}
	// Frame 6690 of the grid: 6690 × 1001/24000 = 279.028750 s. The stored
	// 279.029 (ms rounded) is 250 µs off the grid and snaps onto it - the
	// cold rafale's CFR output places that same frame at exactly this time.
	if us != 279_028_750 {
		t.Fatalf("landing = %d µs, want 279028750 (frame 6690 on the 24000/1001 grid)", us)
	}
}

func TestSeekLandingFromPackets_ExcludesFramesBeforeTarget(t *testing.T) {
	// 278.987 is the last frame before the target and must not win even
	// though it is the closest packet to 279.000.
	us, ok := seekLandingFromPackets(remuxPacketsAt279, 279.0, 0, 0)
	if !ok || us != 279_029_000 {
		t.Fatalf("landing = %d µs ok=%v, want 279029000 (raw pts, no grid)", us, ok)
	}
}

func TestSeekLandingFromPackets_TargetOnAFrameKeepsThatFrame(t *testing.T) {
	// ffmpeg keeps a frame whose pts equals -ss (trim is inclusive).
	us, ok := seekLandingFromPackets([]float64{35.953, 35.994, 36.036, 36.078}, 36.036, 24000, 1001)
	if !ok || us != 36_036_000 {
		t.Fatalf("landing = %d µs ok=%v, want 36036000 (frame 864 = 36.036000 on the grid)", us, ok)
	}
}

func TestSeekLandingFromPackets_OffGridPtsIsNotSnapped(t *testing.T) {
	// 2.3 ms away from any 24000/1001 grid point: a real irregularity, the
	// stored pts is what the frame will be presented at.
	us, ok := seekLandingFromPackets([]float64{10.000, 10.044}, 10.010, 24000, 1001)
	if !ok || us != 10_044_000 {
		t.Fatalf("landing = %d µs ok=%v, want the raw 10044000", us, ok)
	}
}

func TestSeekLandingFromPackets_NoCandidate(t *testing.T) {
	if _, ok := seekLandingFromPackets([]float64{278.945, 278.987}, 279.0, 24000, 1001); ok {
		t.Fatal("no packet at or after the target must report ok=false")
	}
	if _, ok := seekLandingFromPackets(nil, 279.0, 24000, 1001); ok {
		t.Fatal("no packets at all must report ok=false")
	}
}

func TestParseRational(t *testing.T) {
	for _, tc := range []struct {
		in       string
		num, den int64
	}{
		{"24000/1001", 24000, 1001},
		{"25/1", 25, 1},
		{"0/0", 0, 0},
		{"", 0, 0},
		{"abc", 0, 0},
		{"-1/1", 0, 0},
	} {
		n, d := parseRational(tc.in)
		if n != tc.num || d != tc.den {
			t.Errorf("parseRational(%q) = %d/%d, want %d/%d", tc.in, n, d, tc.num, tc.den)
		}
	}
}

// The pump's tick conversion must keep a frame-grid landing exact at the
// timescales the transcode path writes: 24000/1001 × k is an integer number
// of ticks at timescale 24000, and a millisecond rounding would not be.
func TestScaleDelta_FrameGridIsExactAtVideoTimescale(t *testing.T) {
	if got := scaleDelta(279_028_750, 24000); got != 6_696_690 {
		t.Fatalf("scaleDelta(279.02875 s @24000) = %d, want 6696690 (= 6690 × 1001)", got)
	}
	if got := scaleDelta(2_500_000, 1000); got != 2500 {
		t.Fatalf("scaleDelta(2.5 s @1000) = %d, want 2500", got)
	}
	if got := scaleDelta(36_036_000, 90000); got != 3_243_240 {
		t.Fatalf("scaleDelta(36.036 s @90000) = %d, want 3243240", got)
	}
}

// The interval must end at an ABSOLUTE time. ffprobe seeks to the keyframe
// before the target; a relative "+1" counted from that keyframe and, on a
// 2 s GOP, a seek 1.6 s past its keyframe read no packet at or after the
// target (measured 2026-09-26: 3.600%+1 empty, 3.600%4.600 found 3.629).
func TestSeekLandingReadInterval_AbsoluteEnd(t *testing.T) {
	if got := seekLandingReadInterval(3.6); got != "3.600%4.600" {
		t.Fatalf("interval = %q, want 3.600%%4.600", got)
	}
	if got := seekLandingReadInterval(504); got != "504.000%505.000" {
		t.Fatalf("interval = %q, want 504.000%%505.000", got)
	}
}

// Every rung of a per-variant seek asks for the same landing: one probe answers
// them all, a changed file is measured again, and a failure is not remembered.
func TestTranscodeSeekLandingUs_ProbesOncePerFileAndTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.mkv")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := false
	orig := seekLandingProbe
	t.Cleanup(func() { seekLandingProbe = orig })
	seekLandingProbe = func(_ context.Context, _ string, startSec float64) (int64, error) {
		calls++
		if fail {
			return 0, errors.New("probe failed")
		}
		return int64(startSec*1e6) + 7, nil
	}

	for range 4 {
		us, err := transcodeSeekLandingUs(context.Background(), path, 36)
		if err != nil || us != 36_000_007 {
			t.Fatalf("landing = %d, %v", us, err)
		}
	}
	if calls != 1 {
		t.Fatalf("4 rungs at the same target probed %d times, want 1", calls)
	}
	if _, err := transcodeSeekLandingUs(context.Background(), path, 39); err != nil || calls != 2 {
		t.Fatalf("another target: calls=%d err=%v, want a new probe", calls, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := transcodeSeekLandingUs(context.Background(), path, 36); err != nil || calls != 3 {
		t.Fatalf("changed file: calls=%d err=%v, want a new probe", calls, err)
	}
	fail = true
	for range 2 {
		if _, err := transcodeSeekLandingUs(context.Background(), path, 42); err == nil {
			t.Fatal("a failed probe answered from the cache")
		}
	}
	if calls != 5 {
		t.Fatalf("failures probed %d times, want each one retried", calls-3)
	}
}
