package transcoding

import (
	"fmt"
	"math"
)

// SegmentTargetSeconds is the segment length every grid aims at. It is the
// one number the HLS/DASH manifests, the encoders' keyframe cadence and the
// seek arithmetic share; a grid turns it into a whole number of frames.
const SegmentTargetSeconds = 3

// SegmentGrid is the segmentation of one transcoded presentation: every
// segment holds the same whole number of frames, so its length is exact in
// the video track's ticks and identical across the rungs of a ladder. A
// segment of "3 seconds" at 23.976 fps is 71.93 frames; the encoder can only
// cut at 72, which is 3.003 s, and a manifest that declared 3.000 drifted a
// frame per segment. hls.js reads the fragments' own timestamps; Shaka in
// sequence mode (its HLS default, the cast receiver's engine) places each
// segment at the sum of the EXTINF before it, and a DASH timeline is
// addressed by segment number. Every playlist of a presentation, its audio
// renditions included, declares what the encoder really cuts.
//
// The zero value is the legacy time-based grid: 3.000 s segments cut by
// -force_key_frames on the clock, no fixed frame count. It is what a source
// whose frame rate is unknown or variable still gets, and it never offers a
// DASH manifest.
type SegmentGrid struct {
	// FPSNum/FPSDen is the source frame rate as a reduced rational
	// (24000/1001, 25/1 ...). Zero: legacy grid.
	FPSNum, FPSDen int
	// FramesPerSegment is the whole number of frames one segment holds.
	FramesPerSegment int
	// Timescale is the video track timescale the encoder is told to write
	// (-video_track_timescale), chosen so a frame is a whole number of ticks.
	Timescale int
}

// canonicalRates maps the fractional broadcast rates ffprobe and mkvgo report
// as floats back to their exact rationals.
var canonicalRates = []struct {
	num, den int
}{
	{24000, 1001}, {30000, 1001}, {48000, 1001}, {60000, 1001}, {120000, 1001},
}

// GridForFrameRate builds the frame-exact grid for a constant frame rate given
// as a float (the probes carry one, not a rational). A rate within a
// thousandth of a canonical fraction or of an integer is taken as that rate;
// anything else (unknown, variable, exotic) yields the legacy grid.
func GridForFrameRate(fps float64) SegmentGrid {
	if fps <= 0 || math.IsNaN(fps) || math.IsInf(fps, 0) || fps > 240 {
		return SegmentGrid{}
	}
	const tol = 0.001
	for _, r := range canonicalRates {
		if math.Abs(fps-float64(r.num)/float64(r.den)) < tol {
			return gridFor(r.num, r.den)
		}
	}
	if n := math.Round(fps); math.Abs(fps-n) < tol && n >= 1 {
		return gridFor(int(n), 1)
	}
	return SegmentGrid{}
}

func gridFor(num, den int) SegmentGrid {
	frames := max(int(math.Round(float64(SegmentTargetSeconds)*float64(num)/float64(den))), 1)
	// Whole ticks per frame: the numerator itself for /1001 rates (one frame
	// = 1001 ticks), 512 ticks a frame for integer rates (ffmpeg's own
	// default for those, 25 fps -> 12800).
	ts := num
	if den == 1 {
		ts = num * 512
	}
	return SegmentGrid{FPSNum: num, FPSDen: den, FramesPerSegment: frames, Timescale: ts}
}

// Exact reports whether the grid is frame-exact (a DASH timeline can be
// declared from it).
func (g SegmentGrid) Exact() bool {
	return g.FPSNum > 0 && g.FPSDen > 0 && g.FramesPerSegment > 0 && g.Timescale > 0
}

// FrameTicks is one frame in the grid's timescale.
func (g SegmentGrid) FrameTicks() int64 {
	if !g.Exact() {
		return 0
	}
	return int64(g.Timescale) * int64(g.FPSDen) / int64(g.FPSNum)
}

// SegmentTicks is one full segment in the grid's timescale.
func (g SegmentGrid) SegmentTicks() int64 {
	return g.FrameTicks() * int64(g.FramesPerSegment)
}

// SegmentSeconds is one full segment as a float, for the arguments that take
// seconds (-ss, -hls_time, pacing). 3.003 at 23.976 fps, 3 on the legacy grid.
func (g SegmentGrid) SegmentSeconds() float64 {
	if !g.Exact() {
		return SegmentTargetSeconds
	}
	return float64(g.FramesPerSegment) * float64(g.FPSDen) / float64(g.FPSNum)
}

// SegmentStartSeconds is where segment n begins.
func (g SegmentGrid) SegmentStartSeconds(n int) float64 {
	if !g.Exact() {
		return float64(n) * SegmentTargetSeconds
	}
	return float64(n) * float64(g.FramesPerSegment) * float64(g.FPSDen) / float64(g.FPSNum)
}

// SegmentStartTicks is where segment n begins, exactly.
func (g SegmentGrid) SegmentStartTicks(n int) int64 {
	return int64(n) * g.SegmentTicks()
}

// SegmentIndexAt is the segment holding time sec (the seek arithmetic).
func (g SegmentGrid) SegmentIndexAt(sec float64) int {
	if sec <= 0 {
		return 0
	}
	return int(math.Floor(sec / g.SegmentSeconds()))
}

// FrameCount is how many frames a media of durationMs holds on this grid.
func (g SegmentGrid) FrameCount(durationMs int64) int64 {
	if !g.Exact() || durationMs <= 0 {
		return 0
	}
	// Round to the nearest frame: a declared duration is a millisecond
	// figure, the picture has a whole number of frames.
	return int64(math.Round(float64(durationMs) * float64(g.FPSNum) / (1000 * float64(g.FPSDen))))
}

// SegmentCount is how many segments a media of durationMs takes: the last one
// holds the leftover frames. Legacy grid: ceil(duration / 3 s), as before.
func (g SegmentGrid) SegmentCount(durationMs int64) int {
	if durationMs <= 0 {
		return 0
	}
	if !g.Exact() {
		return int(math.Ceil(float64(durationMs) / 1000 / SegmentTargetSeconds))
	}
	frames := g.FrameCount(durationMs)
	if frames <= 0 {
		return 0
	}
	return int((frames + int64(g.FramesPerSegment) - 1) / int64(g.FramesPerSegment))
}

// SegmentDurationTicks is the exact length of segment n for a media of
// durationMs: a full segment, or the leftover frames for the last one.
func (g SegmentGrid) SegmentDurationTicks(n int, durationMs int64) int64 {
	if !g.Exact() {
		return 0
	}
	count := g.SegmentCount(durationMs)
	if n < 0 || n >= count {
		return 0
	}
	if n < count-1 {
		return g.SegmentTicks()
	}
	frames := g.FrameCount(durationMs) - int64(n)*int64(g.FramesPerSegment)
	return frames * g.FrameTicks()
}

// SegmentDurationSeconds is SegmentDurationTicks as the seconds a playlist
// declares (EXTINF), rounded to the millisecond as ffmpeg formats it. The
// legacy grid keeps its former arithmetic: 3.000 and a float leftover.
func (g SegmentGrid) SegmentDurationSeconds(n int, durationMs int64) float64 {
	if g.Exact() {
		return math.Round(float64(g.SegmentDurationTicks(n, durationMs))/float64(g.Timescale)*1000) / 1000
	}
	count := g.SegmentCount(durationMs)
	if n == count-1 {
		leftover := float64(durationMs)/1000 - float64(n)*SegmentTargetSeconds
		if leftover > 0 && leftover < SegmentTargetSeconds {
			return math.Round(leftover*1000) / 1000
		}
	}
	return SegmentTargetSeconds
}

// HLSTime is the -hls_time the muxer is given: the exact segment length. The
// muxer closes a segment at the first keyframe whose time reaches n times
// this cadence (a rational comparison, no float slack), and on an exact grid
// the forced keyframe sits exactly there. Anything shorter would let a
// keyframe the encoder adds on its own (a scene cut) close a segment early;
// equal, only the forced ones qualify. Every canonical rate is exact in six
// decimals (72 frames at 24000/1001 = 3.003000).
func (g SegmentGrid) HLSTime() string {
	if !g.Exact() {
		return fmt.Sprintf("%d", SegmentTargetSeconds)
	}
	return fmt.Sprintf("%.6f", g.SegmentSeconds())
}

// ForceKeyframesExpr is the -force_key_frames expression: one keyframe every
// FramesPerSegment frames on an exact grid, one every 3 s of clock otherwise.
func (g SegmentGrid) ForceKeyframesExpr() string {
	if !g.Exact() {
		return fmt.Sprintf("expr:gte(t,n_forced*%d)", SegmentTargetSeconds)
	}
	return fmt.Sprintf("expr:eq(mod(n,%d),0)", g.FramesPerSegment)
}

// FrameRateString is the DASH @frameRate value ("24000/1001", "25").
func (g SegmentGrid) FrameRateString() string {
	if !g.Exact() {
		return ""
	}
	if g.FPSDen == 1 {
		return fmt.Sprintf("%d", g.FPSNum)
	}
	return fmt.Sprintf("%d/%d", g.FPSNum, g.FPSDen)
}

// String is the grid as a hash dimension: "24000/1001:72@24000" or "" for
// the legacy grid, so a legacy fingerprint hashes exactly as before.
func (g SegmentGrid) String() string {
	if !g.Exact() {
		return ""
	}
	return fmt.Sprintf("%d/%d:%d@%d", g.FPSNum, g.FPSDen, g.FramesPerSegment, g.Timescale)
}
