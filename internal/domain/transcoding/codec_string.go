package transcoding

import (
	"fmt"
	"math"
	"strings"
)

// The profile and level a transcoded rung is ENCODED at, and the RFC 6381
// codec string the manifests declare for it, come from here: one contract
// for the software and the GPU encoders, the HLS master and the DASH
// manifest. The level is the lowest one whose limits hold the rung as it is
// encoded - its picture size, its frame rate and its rate ceiling
// (-maxrate / -bufsize) - so what is declared is also what the stream
// conforms to. It is computed once per rung (AssignCodecLevels) and carried
// on the rung (HLSProfile.CodecLevel), hashed with it.

// h264Level is one row of the H.264 level limits (Table A-1).
type h264Level struct {
	idc    int
	mbps   float64 // MaxMBPS, macroblocks per second
	fs     int     // MaxFS, macroblocks per frame
	dpbMbs int     // MaxDpbMbs
	br     int64   // MaxBR, in 1000 bits/s of cpbBrVclFactor
	cpb    int64   // MaxCPB, same unit
}

var h264Levels = []h264Level{
	{10, 1485, 99, 396, 64, 175},
	{11, 3000, 396, 900, 192, 500},
	{12, 6000, 396, 2376, 384, 1000},
	{13, 11880, 396, 2376, 768, 2000},
	{20, 11880, 396, 2376, 2000, 2000},
	{21, 19800, 792, 4752, 4000, 4000},
	{22, 20250, 1620, 8100, 4000, 4000},
	{30, 40500, 1620, 8100, 10000, 10000},
	{31, 108000, 3600, 18000, 14000, 14000},
	{32, 216000, 5120, 20480, 20000, 20000},
	{40, 245760, 8192, 32768, 20000, 25000},
	{41, 245760, 8192, 32768, 50000, 62500},
	{42, 522240, 8704, 34816, 50000, 62500},
	{50, 589824, 22080, 110400, 135000, 135000},
	{51, 983040, 36864, 184320, 240000, 240000},
	{52, 2073600, 36864, 184320, 240000, 240000},
	{60, 4177920, 139264, 696320, 240000, 240000},
	{61, 8355840, 139264, 696320, 480000, 480000},
	{62, 16711680, 139264, 696320, 800000, 800000},
}

// h264HighBrFactor is the High profile's cpbBrVclFactor over the Main one
// (1250 / 1000, Table A-2): a High stream's MaxBR and MaxCPB are 1.25x.
const h264HighBrFactor = 1.25

func mbs(v int) int { return (v + 15) / 16 }

// H264Level is the lowest High profile level_idc that holds a width x height
// picture at fps with a VBV of maxKbps / bufKbps; the top level when none
// does. fps <= 0 (unknown) counts as 60.
func H264Level(width, height int, fps float64, maxKbps, bufKbps int64) int {
	fps = LevelFrameRate(fps)
	fs := mbs(width) * mbs(height)
	for _, l := range h264Levels {
		side := int(math.Sqrt(float64(l.fs) * 8))
		if fs > l.fs || mbs(width) > side || mbs(height) > side || float64(fs)*fps > l.mbps {
			continue
		}
		if float64(maxKbps) > float64(l.br)*h264HighBrFactor || float64(bufKbps) > float64(l.cpb)*h264HighBrFactor {
			continue
		}
		return l.idc
	}
	return h264Levels[len(h264Levels)-1].idc
}

// H264MaxDpbFrames is how many reference frames a width x height stream at
// level idc may hold (A.3.1: MaxDpbMbs / frame size, at most 16).
func H264MaxDpbFrames(idc, width, height int) int {
	for _, l := range h264Levels {
		if l.idc == idc {
			return min(l.dpbMbs/(mbs(width)*mbs(height)), 16)
		}
	}
	return 0
}

// H264LevelLimits returns MaxBR and MaxCPB (kbit/s, High profile) of idc.
func H264LevelLimits(idc int) (maxKbps, cpbKbits int64) {
	for _, l := range h264Levels {
		if l.idc == idc {
			return int64(float64(l.br) * h264HighBrFactor), int64(float64(l.cpb) * h264HighBrFactor)
		}
	}
	return 0, 0
}

// hevcLevel is one row of the H.265 level limits (Tables A.8/A.9), Main
// tier: picture size, luma rate, MaxBR and MaxCPB (kbit/s). The High tier is
// never used: hevc_metadata cannot write it and decoders support it less;
// a rate the Main tier cannot hold at a level takes the next level.
type hevcLevel struct {
	idc int
	ps  float64 // MaxLumaPs
	sr  float64 // MaxLumaSr
	br  int64   // MaxBR, Main tier
	cpb int64   // MaxCPB, Main tier
}

var hevcLevels = []hevcLevel{
	{30, 36864, 552960, 128, 350},
	{60, 122880, 3686400, 1500, 1500},
	{63, 245760, 7372800, 3000, 3000},
	{90, 552960, 16588800, 6000, 6000},
	{93, 983040, 33177600, 10000, 10000},
	{120, 2228224, 66846720, 12000, 12000},
	{123, 2228224, 133693440, 20000, 20000},
	{150, 8912896, 267386880, 25000, 25000},
	{153, 8912896, 534773760, 40000, 40000},
	{156, 8912896, 1069547520, 60000, 60000},
	{180, 35651584, 1069547520, 60000, 60000},
	{183, 35651584, 2139095040, 120000, 120000},
	{186, 35651584, 4278190080, 240000, 240000},
}

// HEVCLevel is the lowest Main-profile, Main-tier general_level_idc that
// holds a width x height picture at fps with a VBV of maxKbps / bufKbps;
// the top level when none does. fps <= 0 (unknown) counts as 60.
func HEVCLevel(width, height int, fps float64, maxKbps, bufKbps int64) int {
	fps = LevelFrameRate(fps)
	ps := float64(width * height)
	for _, l := range hevcLevels {
		side := math.Sqrt(l.ps * 8)
		if ps > l.ps || float64(width) > side || float64(height) > side || ps*fps > l.sr {
			continue
		}
		if maxKbps <= l.br && bufKbps <= l.cpb {
			return l.idc
		}
	}
	return hevcLevels[len(hevcLevels)-1].idc
}

// HEVCLevelLimits returns MaxBR and MaxCPB (kbit/s, Main tier) of idc.
func HEVCLevelLimits(idc int) (maxKbps, cpbKbits int64) {
	for _, l := range hevcLevels {
		if l.idc == idc {
			return l.br, l.cpb
		}
	}
	return 0, 0
}

// LevelFrameRate is the frame rate a level is computed at: the encode's,
// 60 when unknown (the highest a transcode rung plays at).
func LevelFrameRate(fps float64) float64 {
	if fps <= 0 || math.IsNaN(fps) {
		return 60
	}
	return fps
}

// CodecLevel is the level a rung of codec is encoded and declared at, for
// its rate ceiling qt: level_idc for H.264, general_level_idc (Main tier)
// for HEVC, 0 for a codec the encoders pick their own level for.
func CodecLevel(codec string, p HLSProfile, fps float64, qt QualityTarget) int {
	switch strings.ToLower(codec) {
	case "h264", "avc", "libx264", "":
		return H264Level(p.Width, p.Height, fps, qt.MaxKbps, qt.BufferKbps)
	case "hevc", "h265", "x265":
		return HEVCLevel(p.Width, p.Height, fps, qt.MaxKbps, qt.BufferKbps)
	}
	return 0
}

// AssignCodecLevels stamps every rung with its CodecLevel (the route's
// target for it, targetFor) and drops the H.264 rungs above the client's
// cap (maxH264Level > 0): a rung is removed, never relabelled with a level
// its stream exceeds. When not even the lowest rung fits, it is kept at its
// true level - declared honestly, the client decides.
func AssignCodecLevels(ladder []HLSProfile, codec string, fps float64, targetFor func(HLSProfile) QualityTarget, maxH264Level int) []HLSProfile {
	out := make([]HLSProfile, 0, len(ladder))
	for _, p := range ladder {
		if p.Copy {
			out = append(out, p)
			continue
		}
		p.CodecLevel = CodecLevel(codec, p, fps, targetFor(p))
		out = append(out, p)
	}
	if maxH264Level <= 0 || !strings.EqualFold(codec, "h264") || len(out) == 0 {
		return out
	}
	kept := out[:0:0]
	for _, p := range out {
		if p.Copy || p.CodecLevel <= maxH264Level {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return out[:1]
	}
	return kept
}

// H264CodecString is the codecs value of an H.264 transcode rung: High
// (profile_idc 100, no constraint flag) at level_idc.
func H264CodecString(level int) string {
	return fmt.Sprintf("avc1.6400%02x", level)
}

// HEVCCodecString is the codecs value of an HEVC transcode rung: Main (1.6)
// or Main 10 (2.4), Main tier at level, and the constraint byte the
// encoders write (progressive source, frame only: 0x90 - measured on
// libx265 and hevc_nvenc).
func HEVCCodecString(level int, main10 bool) string {
	profile := "1.6"
	if main10 {
		profile = "2.4"
	}
	return fmt.Sprintf("hvc1.%s.L%d.90", profile, level)
}

// FPS is the grid's frame rate, 0 on the legacy grid.
func (g SegmentGrid) FPS() float64 {
	if !g.Exact() {
		return 0
	}
	return float64(g.FPSNum) / float64(g.FPSDen)
}
