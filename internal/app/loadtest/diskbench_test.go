package loadtest

import "testing"

// A spinning disk peaks at a handful of sequential readers and gets WORSE as
// viewers pile on. Quoting that peak next to a 21-viewer run is how a saturated
// disk gets read as 40% busy: the operator sees "1151 of 2872 Mb/s", concludes
// he has 60% headroom, and goes looking for the bottleneck somewhere it isn't.
//
// The figure an audience is up against is the one measured at the width that
// audience imposes - never the one measured at the width the hardware likes.
func TestDiskProbe_QuotesTheCurveAtTheWidthTheAudienceImposes(t *testing.T) {
	t.Parallel()

	// One disk. It loves 4 readers (peak) and falls apart past that.
	probe := DiskProbe{
		Measured: true,
		Disks: []DiskThroughput{{
			Levels: []DiskLevel{
				{Readers: 1, BytesPerSec: 100},
				{Readers: 4, BytesPerSec: 400}, // the peak
				{Readers: 8, BytesPerSec: 200},
				{Readers: 16, BytesPerSec: 90},
				{Readers: 32, BytesPerSec: 50},
			},
			BytesPerSec: 400,
			BestReaders: 4,
		}},
		TotalBytesPerSec: 400,
	}

	if got := probe.bestWidth(); got != 4 {
		t.Fatalf("bestWidth() = %d, want 4", got)
	}
	// Sixteen viewers on one disk means sixteen readers on that disk: the honest
	// answer is 90, not the 400 the drive gives when barely bothered.
	if got := probe.atWidth(16); got != 90 {
		t.Errorf("atWidth(16) = %d, want 90 (the measured curve at 16 readers, not the 400 peak)", got)
	}
	// Never extrapolate past what was measured: 64 viewers reads the widest
	// sampled level rather than inventing a number below it.
	if got := probe.atWidth(64); got != 50 {
		t.Errorf("atWidth(64) = %d, want 50 (the widest level actually sampled)", got)
	}
}

// Between sampled widths, the probe must round DOWN to a level it actually
// measured. Interpolating would be inventing a datapoint, and the whole reason
// this exists is that invented capacity is what misled the operator in the first
// place.
func TestDiskProbe_NeverInventsAWidthItDidNotSample(t *testing.T) {
	t.Parallel()

	probe := DiskProbe{
		Measured: true,
		Disks: []DiskThroughput{{
			Levels: []DiskLevel{
				{Readers: 1, BytesPerSec: 100},
				{Readers: 4, BytesPerSec: 400},
				{Readers: 8, BytesPerSec: 200},
			},
			BytesPerSec: 400,
			BestReaders: 4,
		}},
	}

	// 6 readers: the widest level at or below is 4.
	if got := probe.atWidth(6); got != 400 {
		t.Errorf("atWidth(6) = %d, want 400 (the level at 4, the widest not exceeding 6)", got)
	}
	if got := probe.atWidth(0); got != 0 {
		t.Errorf("atWidth(0) = %d, want 0", got)
	}
}

// Spreading a library over several disks is the ONE thing that genuinely buys
// concurrency, and the report must not erase it: 22 viewers over 2 drives is
// about 11 readers per spindle, not 22. Reading each drive's curve at 22 would
// invent a collapse that the hardware never suffers - the mirror image of the
// bug this whole split exists to kill.
func TestDiskProbe_SplitsTheAudienceAcrossTheDrivesItIsSpreadOver(t *testing.T) {
	t.Parallel()

	curve := []DiskLevel{
		{Readers: 1, BytesPerSec: 100},
		{Readers: 4, BytesPerSec: 400},
		{Readers: 8, BytesPerSec: 300}, // 11 readers/disk lands here
		{Readers: 16, BytesPerSec: 80},
		{Readers: 32, BytesPerSec: 40},
	}
	two := DiskProbe{
		Measured: true,
		Disks: []DiskThroughput{
			{Levels: curve, BytesPerSec: 400, BestReaders: 4},
			{Levels: curve, BytesPerSec: 400, BestReaders: 4},
		},
	}

	// 22 viewers / 2 disks = 11 readers each -> the level at 8, summed over both.
	if got := two.atWidth(22); got != 600 {
		t.Errorf("atWidth(22) over 2 disks = %d, want 600 (each drive at 11 readers => the 8-reader level, 300, twice)", got)
	}

	// The same audience on ONE disk hammers that single spindle with all 22, and
	// must read far worse. This is exactly the capacity a second disk buys.
	one := DiskProbe{
		Measured: true,
		Disks:    []DiskThroughput{{Levels: curve, BytesPerSec: 400, BestReaders: 4}},
	}
	if got := one.atWidth(22); got != 80 {
		t.Errorf("atWidth(22) over 1 disk = %d, want 80 (the 16-reader level)", got)
	}
	if two.atWidth(22) <= one.atWidth(22) {
		t.Error("spreading the same audience over two disks must measure better than crowding it onto one")
	}
}

// A probe that could not read anything must report nothing rather than a zero
// dressed up as a measurement.
func TestDiskProbe_UnmeasuredStaysSilent(t *testing.T) {
	t.Parallel()

	var empty DiskProbe
	if got := empty.atWidth(21); got != 0 {
		t.Errorf("atWidth on an unmeasured probe = %d, want 0", got)
	}
	if got := empty.bestWidth(); got != 0 {
		t.Errorf("bestWidth on an unmeasured probe = %d, want 0", got)
	}
}
