package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The level of a rung is the lowest whose limits hold it as encoded: its
// picture, its frame rate and its rate ceiling (Tables A-1 / A.8, High
// profile factor 1.25 for H.264, Main tier for HEVC).
func TestCodecLevel_HoldsThePictureTheRateAndTheCeiling(t *testing.T) {
	sw := EncodeRoute{}
	hw := EncodeRoute{HWUsable: true}
	target := func(p HLSProfile, r EncodeRoute) QualityTarget { return StaticQualityPolicy{}.TargetFor(p, r) }
	rung := func(w, h int, kbps int64) HLSProfile { return HLSProfile{Width: w, Height: h, VideoBitrate: kbps} }
	for _, tc := range []struct {
		name  string
		codec string
		p     HLSProfile
		fps   float64
		route EncodeRoute
		want  int
	}{
		{"720p60 h264: 216000 MB/s is 3.2", "h264", rung(1280, 720, 2800), 60, sw, 32},
		{"1080p24 h264", "h264", rung(1920, 1080, 5000), 23.976, sw, 40},
		{"1080p60 h264: 489600 MB/s is 4.2", "h264", rung(1920, 1080, 5000), 60, sw, 42},
		{"1080p boosted to 15 Mb/s, software VBV (24000/48000): 4.0's CPB cannot", "h264", rung(1920, 1080, 15000), 23.976, sw, 41},
		{"1080p boosted to 15 Mb/s, GPU VBV (15000/30000): 4.0 holds it", "h264", rung(1920, 1080, 15000), 23.976, hw, 40},
		{"4K24 h264", "h264", rung(3840, 2160, 16000), 23.976, sw, 51},
		{"4K60 h264", "h264", rung(3840, 2160, 16000), 60, sw, 52},
		{"720p60 hevc: 55 M luma/s is 4.0", "hevc", rung(1280, 720, 1867), 60, sw, 120},
		{"1080p60 hevc", "hevc", rung(1920, 1080, 3333), 60, sw, 123},
		{"1080p boosted to 15 Mb/s hevc: the Main tier's CPB takes 5.2", "hevc", rung(1920, 1080, 15000), 23.976, sw, 156},
		{"4K24 hevc: 5.0's CPB cannot hold 34 Mbit", "hevc", rung(3840, 2160, 10667), 23.976, sw, 153},
		{"unknown rate counts as 60", "h264", rung(1920, 1080, 5000), 0, sw, 42},
	} {
		assert.Equal(t, tc.want, CodecLevel(tc.codec, tc.p, tc.fps, target(tc.p, tc.route)), tc.name)
	}
	assert.Equal(t, 0, CodecLevel("av1", rung(1920, 1080, 5000), 24, target(rung(1920, 1080, 5000), sw)), "AV1 levels itself")
	assert.Equal(t, 4, H264MaxDpbFrames(42, 1920, 1080), "1080p at 4.2 holds four reference frames")
	assert.Equal(t, "avc1.64002a", H264CodecString(42))
	assert.Equal(t, "hvc1.1.6.L156.90", HEVCCodecString(156, false))
	assert.Equal(t, "hvc1.2.4.L153.90", HEVCCodecString(153, true))
}

// A client's H.264 cap removes the rungs above it - it never relabels one.
// When not even the lowest fits, that one stays at its true level.
func TestAssignCodecLevels_TheCapDropsNeverRelabels(t *testing.T) {
	ladder := []HLSProfile{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000},
	}
	target := func(p HLSProfile) QualityTarget { return StaticQualityPolicy{}.TargetFor(p, EncodeRoute{}) }
	names := func(l []HLSProfile) []string {
		var out []string
		for _, p := range l {
			out = append(out, p.Name)
		}
		return out
	}
	all := AssignCodecLevels(ladder, "h264", 23.976, target, 0)
	require.Len(t, all, 4)
	for _, p := range all {
		assert.Positive(t, p.CodecLevel, p.Name)
	}
	assert.Equal(t, []string{"360p", "480p", "720p", "1080p"}, names(AssignCodecLevels(ladder, "h264", 23.976, target, 40)))
	assert.Equal(t, []string{"360p", "480p", "720p"}, names(AssignCodecLevels(ladder, "h264", 23.976, target, 31)))
	capped := AssignCodecLevels(ladder, "h264", 60, target, 31)
	for _, p := range capped {
		assert.LessOrEqual(t, p.CodecLevel, 31, p.Name)
	}
	lowest := AssignCodecLevels(ladder, "h264", 60, target, 10)
	require.Len(t, lowest, 1, "nothing fits level 1: the lowest rung stays")
	assert.Greater(t, lowest[0].CodecLevel, 10, "at its true level, not relabelled")
	assert.Len(t, AssignCodecLevels(ladder, "hevc", 23.976, target, 31), 4, "the H.264 cap does not touch HEVC")
}
