package reduceddl

import (
	"strings"
	"testing"
)

func TestWidthFor(t *testing.T) {
	tests := []struct {
		name       string
		w, h, want int
	}{
		{"4K 16:9 scales to 1280", 3840, 2160, 1280},
		{"1080p 16:9 scales to 1280", 1920, 1080, 1280},
		// A source already at or under the target is never re-encoded: the
		// "lighter" copy would cost an hour of NAS and save nothing.
		{"720p is already small enough", 1280, 720, 0},
		{"sub-720p is already small enough", 640, 360, 0},
		{"unknown dimensions refuse", 0, 0, 0},
		{"negative dimensions refuse", -1, 1080, 0},
		// yuv420p cannot do odd: 2.39:1 at 720 lands on 1721.6 -> 1720.
		{"cinemascope width stays even", 3840, 1606, 1720},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WidthFor(tc.w, tc.h); got != tc.want {
				t.Errorf("WidthFor(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.want)
			}
			if got := WidthFor(tc.w, tc.h); got%2 != 0 {
				t.Errorf("WidthFor(%d, %d) = %d, must be even (yuv420p chroma)", tc.w, tc.h, got)
			}
		})
	}
}

func TestSliceCount(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		want       int
	}{
		{"nothing to slice", 0, 0},
		{"under one slice still needs one", 1_000, 1},
		{"exactly one slice", int64(SliceSec) * 1000, 1},
		// The remainder gets its own (short) last slice - never dropped.
		{"one second over rolls into a second slice", int64(SliceSec)*1000 + 1000, 2},
		{"two hours", 2 * 3600 * 1000, 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SliceCount(tc.durationMs); got != tc.want {
				t.Errorf("SliceCount(%d) = %d, want %d", tc.durationMs, got, tc.want)
			}
		})
	}
}

// The estimate is what the user commits an hour of NAS time on, so it must
// track the bitrates the encoder is actually capped at - not a separate guess.
func TestEstimatedBytesMatchesTheCappedBitrates(t *testing.T) {
	const twoHoursMs = 2 * 3600 * 1000
	want := int64(twoHoursMs) * (VideoKbps + AudioKbps) * 1000 / 8 / 1000
	if got := EstimatedBytes(twoHoursMs); got != want {
		t.Errorf("EstimatedBytes(2h) = %d, want %d", got, want)
	}
	if got := EstimatedBytes(0); got != 0 {
		t.Errorf("EstimatedBytes(0) = %d, want 0", got)
	}
	if got := EstimatedBytes(-5); got != 0 {
		t.Errorf("EstimatedBytes(negative) = %d, want 0", got)
	}
}

func TestListNameRoundTrip(t *testing.T) {
	for _, start := range []int{0, 1, 7, 12345} {
		got, ok := ListStart(ListName(start))
		if !ok || got != start {
			t.Errorf("ListStart(ListName(%d)) = %d, %v; want %d, true", start, got, ok, start)
		}
	}
	// Anything else in the job dir must not be mistaken for a list, or a stray
	// file would inflate the readiness count and hand out an unwritten slice.
	for _, name := range []string{"", "slice-00000.mkv", "slices-.csv", "slices-abc.csv", "slices-00001.txt", "notes.csv"} {
		if _, ok := ListStart(name); ok {
			t.Errorf("ListStart(%q) accepted a non-list name", name)
		}
	}
}

func TestClosedSlicesCountsOnlyCompleteLines(t *testing.T) {
	// The list is read WHILE the muxer appends to it, so the tail can be a
	// half-flushed line. Counting it would advertise a slice ffmpeg is still
	// writing into, and the phone would join a truncated Matroska.
	const list = "slice-00000.mkv,0.000000,600.000000\n" +
		"slice-00001.mkv,600.000000,1200.000000\n" +
		"slice-00002.mkv,1200.0000"
	got, end := ClosedSlices(strings.NewReader(list))
	if got != 2 {
		t.Errorf("ClosedSlices = %d, want 2 (the partial third line is not a slice)", got)
	}
	// The end time of the LAST complete slice is where a resume seeks to;
	// picking up the partial line's 1200 would skip a whole slice of film.
	if end != 1200 {
		t.Errorf("ClosedSlices endSec = %v, want 1200 (the end of the last COMPLETE slice)", end)
	}
	if got, end := ClosedSlices(strings.NewReader("")); got != 0 || end != 0 {
		t.Errorf("ClosedSlices(empty) = %d, %v; want 0, 0", got, end)
	}
	if got, end := ClosedSlices(nil); got != 0 || end != 0 {
		t.Errorf("ClosedSlices(nil) = %d, %v; want 0, 0", got, end)
	}
}

// The slice name is what the segment muxer writes and what the assembler
// reads back: five zero-padded digits, so lexical order is index order.
func TestSliceName(t *testing.T) {
	if got := SliceName(0); got != "slice-00000.mkv" {
		t.Errorf("SliceName(0) = %q", got)
	}
	if got := SliceName(1234); got != "slice-01234.mkv" {
		t.Errorf("SliceName(1234) = %q", got)
	}
	if SliceName(9) >= SliceName(10) {
		t.Error("zero padding must keep lexical order equal to index order")
	}
}
