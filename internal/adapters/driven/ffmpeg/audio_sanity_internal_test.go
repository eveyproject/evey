package ffmpeg

import (
	"math"
	"testing"
)

// The sample points must land inside the film, never on its first frame (logos
// and silent idents) nor on its last (a dub's credits are often silent) - both
// are where a healthy track looks silent for innocent reasons.
func TestProbeOffsets_SpreadInsideTheRuntime(t *testing.T) {
	const dur = 7200.0
	got := probeOffsets(dur)
	if len(got) != sanityProbePoints {
		t.Fatalf("got %d points, want %d", len(got), sanityProbePoints)
	}
	for i, at := range got {
		if at <= 0 {
			t.Errorf("point %d at %.0fs starts on the head", i, at)
		}
		if at+sanityWindowSec >= dur {
			t.Errorf("point %d at %.0fs runs into the tail", i, at)
		}
		if i > 0 && at <= got[i-1] {
			t.Errorf("points are not spread: %.0f after %.0f", at, got[i-1])
		}
	}
}

// A very short file, or one whose runtime could not be read, still gets looked
// at - a narrow look beats declaring the file unmeasurable and repairing it
// unguarded.
func TestProbeOffsets_ShortOrUnknownRuntimeStillProbes(t *testing.T) {
	for _, dur := range []float64{0, -1, 5, 3 * sanityWindowSec} {
		got := probeOffsets(dur)
		if len(got) == 0 {
			t.Fatalf("runtime %.0f: no probe point at all", dur)
		}
		for _, at := range got {
			if at < 0 || math.IsNaN(at) {
				t.Errorf("runtime %.0f: nonsense offset %v", dur, at)
			}
		}
	}
}

// The floor must call digital silence silent and ordinary room tone audible.
// Getting this backwards would either roll back every healthy repair or let
// the incident through again.
func TestSanityFloor_SeparatesSilenceFromRoomTone(t *testing.T) {
	if coherenceSilenceDB > sanityFloorDB {
		t.Fatalf("digital silence (%.0f dB) reads as sound under the floor %.0f dB",
			coherenceSilenceDB, sanityFloorDB)
	}
	const quietestRealRoomTone = -70.0
	if quietestRealRoomTone <= sanityFloorDB {
		t.Fatalf("room tone (%.0f dB) reads as silence under the floor %.0f dB",
			quietestRealRoomTone, sanityFloorDB)
	}
}
