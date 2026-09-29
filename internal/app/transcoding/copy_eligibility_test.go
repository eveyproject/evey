package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyRungEligible_H264(t *testing.T) {
	l40 := 40
	assert.True(t, CopyRungEligible(CopySource{Codec: "h264", Level: &l40}), "H.264 8-bit with a known level is copyable")
	assert.True(t, CopyRungEligible(CopySource{Codec: "H264", Level: &l40}), "codec match is case-insensitive")

	// The guard: no level ⟹ no copy (can't advertise an accurate avc1.PPCCLL).
	assert.False(t, CopyRungEligible(CopySource{Codec: "h264"}), "unknown level must gate copy off")
	// Scope guards.
	assert.False(t, CopyRungEligible(CopySource{Codec: "h264", Level: &l40, TenBit: true}), "10-bit H.264 is out of copy scope")
	assert.False(t, CopyRungEligible(CopySource{Codec: "mpeg2video", Level: &l40}), "legacy codecs are out of copy scope")
}

func TestCopyRungEligible_HEVC(t *testing.T) {
	l153 := 153
	pq := "smpte2084"
	bt709 := "bt709"

	// The happy paths: probed HDR ground truth + Main/Main 10 profile.
	assert.True(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153, TenBit: true, ColorTransfer: &pq}),
		"HEVC Main 10 HDR with probed ground truth is copyable - the whole point of the rung")
	assert.True(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main", Level: &l153, ColorTransfer: &bt709}),
		"HEVC Main 8-bit SDR with probed transfer is copyable")

	// Gates.
	assert.False(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main 10", ColorTransfer: &pq}),
		"unknown level must gate copy off")
	assert.False(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153}),
		"unprobed color_transfer (DoVi status unknown) must gate copy off")
	assert.False(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main Still Picture", Level: &l153, ColorTransfer: &pq}),
		"profiles without a wired CODECS string must gate copy off")
}

func TestCopyRungEligible_DolbyVision(t *testing.T) {
	l153 := 153
	pq := "smpte2084"
	base := CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153, TenBit: true, ColorTransfer: &pq}

	p5 := base
	p5.DoViProfile = new(5)
	assert.False(t, CopyRungEligible(p5), "DoVi profile 5 (IPTC2) must never be raw-served")

	p7 := base
	p7.DoViProfile = new(7)
	p7.DoViBLCompat = new(1)
	assert.False(t, CopyRungEligible(p7), "DoVi profile 7 (dual layer) stays transcoded")

	p8 := base
	p8.DoViProfile = new(8)
	assert.False(t, CopyRungEligible(p8), "DoVi profile 8 without a known bl compat stays transcoded")

	for _, compat := range []int{1, 2, 4} {
		ok := base
		ok.DoViProfile = new(8)
		ok.DoViBLCompat = new(compat)
		assert.True(t, CopyRungEligible(ok), "DoVi 8 with compatible base layer %d is copyable as its base", compat)
	}

	weird := base
	weird.DoViProfile = new(8)
	weird.DoViBLCompat = new(0)
	assert.False(t, CopyRungEligible(weird), "DoVi 8 with bl compat 0 (none) stays transcoded")
}

func TestCopyRungEligible_AV1VP9(t *testing.T) {
	pq := "smpte2084"
	bt709 := "bt709"

	av1 := CopySource{Codec: "av1", Profile: "Main", Level: new(8), ColorTransfer: &bt709}
	assert.True(t, CopyRungEligible(av1), "AV1 Main with probed ground truth is copyable")

	av1HDR := CopySource{Codec: "av1", Profile: "Main", Level: new(13), TenBit: true, ColorTransfer: &pq}
	assert.True(t, CopyRungEligible(av1HDR), "AV1 10-bit HDR with probed truth is copyable")

	assert.False(t, CopyRungEligible(CopySource{Codec: "av1", Profile: "Main", Level: new(8)}),
		"unprobed color_transfer gates AV1 copy off (AV1 is HDR-capable, DoVi P10 exists)")
	assert.False(t, CopyRungEligible(CopySource{Codec: "av1", Profile: "Professional", Level: new(8), ColorTransfer: &bt709}),
		"AV1 Professional has no browser decode - not wired")

	av1DoVi := av1HDR
	av1DoVi.DoViProfile = new(10)
	assert.False(t, CopyRungEligible(av1DoVi), "DoVi profile 10 (AV1) never raw-serves")

	vp9 := CopySource{Codec: "vp9", Profile: "Profile 0", Level: new(41), ColorTransfer: &bt709}
	assert.True(t, CopyRungEligible(vp9), "VP9 Profile 0 with probed truth is copyable")
	vp9p2 := CopySource{Codec: "vp9", Profile: "Profile 2", Level: new(41), TenBit: true, ColorTransfer: &pq}
	assert.True(t, CopyRungEligible(vp9p2), "VP9 Profile 2 (10-bit) is copyable")
	assert.False(t, CopyRungEligible(CopySource{Codec: "vp9", Profile: "Profile 1", Level: new(41), ColorTransfer: &bt709}),
		"VP9 Profile 1 (4:2:2) has no browser decode - not wired")
	assert.False(t, CopyRungEligible(CopySource{Codec: "vp9", Profile: "Profile 0", ColorTransfer: &bt709}),
		"unknown level AND unknown size gates VP9 copy off")
	// The VP9 level is never in the file: with the picture size known it is
	// derived (Spring, 2048x858 → level 4.0), as mkvgo derives it for its vpcC.
	spring := CopySource{Codec: "vp9", Profile: "Profile 0", Width: 2048, Height: 858, ColorTransfer: &bt709}
	assert.True(t, CopyRungEligible(spring), "VP9 with a known picture size derives its level")
	assert.Equal(t, "vp09.00.40.08", CopyVideoCodecString(spring))
	assert.Empty(t, CopyBlocker(spring))
	// A probed level always wins over the derivation.
	springL := spring
	springL.Level = new(41)
	assert.Equal(t, "vp09.00.41.08", CopyVideoCodecString(springL))
	// VP9 Profile 0 is 8-bit: no HDR can hide behind an unprobed transfer, the
	// colour gate does not apply (Sprite Fright through ffprobe: colour space
	// known, transfer not). Profile 2 (10-bit) keeps the gate.
	sf := CopySource{Codec: "vp9", Profile: "Profile 0", Width: 2048, Height: 858}
	assert.Empty(t, CopyBlocker(sf))
	assert.Equal(t, "vp09.00.40.08", CopyVideoCodecString(sf))
	assert.Equal(t, "hdr_truth_unknown", CopyBlocker(CopySource{Codec: "vp9", Profile: "Profile 2", TenBit: true, Width: 3840, Height: 2160}))
	// Only VP9 derives: an H.264 without a level stays blocked.
	assert.Equal(t, "level_unknown", CopyBlocker(CopySource{Codec: "h264", Profile: "High", Width: 1920, Height: 1080, ColorTransfer: &bt709}))
}

// The VP9 level table (Annex A of the bitstream spec), by maximum luma
// picture size: the same one mkvgo writes into a vpcC. Levels 6.0 and 6.1
// share a picture size and differ by sample rate, so 8K derives as 6.0.
func TestVP9LevelForSize(t *testing.T) {
	for _, c := range []struct{ w, h, want int }{
		{256, 144, 10}, {640, 360, 21}, {1280, 720, 31}, {1920, 1080, 40}, {2048, 858, 40},
		{3840, 2160, 50}, {4096, 2160, 50}, {7680, 4320, 60}, {8192, 4352, 60}, {16384, 8704, 62},
	} {
		got, ok := VP9LevelForSize(c.w, c.h)
		assert.True(t, ok)
		assert.Equal(t, c.want, got, "%dx%d", c.w, c.h)
	}
	_, ok := VP9LevelForSize(0, 1080)
	assert.False(t, ok, "unknown size: no level")
}

func TestCopyRungEligible_VFR(t *testing.T) {
	l40 := 40
	l153 := 153
	pq := "smpte2084"

	// A VFR source must never reach the copy rung: the HLS copy segmenter
	// cuts segments at keyframe boundaries assuming a constant frame period,
	// and a variable rate drifts audio/video sync.
	assert.False(t, CopyRungEligible(CopySource{Codec: "h264", Level: &l40, FrameRateMode: "vfr"}),
		"a VFR H.264 source must gate the copy rung off")
	assert.False(t, CopyRungEligible(CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153, TenBit: true, ColorTransfer: &pq, FrameRateMode: "VFR"}),
		"the FrameRateMode check is case-insensitive")

	// "" (never analyzed) and "cfr" must not change today's behaviour.
	assert.True(t, CopyRungEligible(CopySource{Codec: "h264", Level: &l40, FrameRateMode: ""}),
		"unknown frame-rate mode leaves copy eligibility unchanged")
	assert.True(t, CopyRungEligible(CopySource{Codec: "h264", Level: &l40, FrameRateMode: "cfr"}),
		"a CFR source stays copy-eligible")
}

func TestCopyVideoCodecString(t *testing.T) {
	l40 := 40
	l153 := 153
	pq := "smpte2084"

	assert.Equal(t, "avc1.640028",
		CopyVideoCodecString(CopySource{Codec: "h264", Profile: "High", Level: &l40}),
		"H.264 High L4.0")
	assert.Equal(t, "hvc1.2.4.L153.B0",
		CopyVideoCodecString(CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153, TenBit: true, ColorTransfer: &pq}),
		"HEVC Main 10 L5.1")
	assert.Equal(t, "hvc1.1.6.L120.B0",
		CopyVideoCodecString(CopySource{Codec: "hevc", Profile: "Main", Level: new(120), ColorTransfer: &pq}),
		"HEVC Main L4.0")
	assert.Empty(t,
		CopyVideoCodecString(CopySource{Codec: "hevc", Profile: "Main 10", Level: &l153}),
		"ineligible source yields no codec string")

	bt709 := "bt709"
	assert.Equal(t, "av01.0.08M.08",
		CopyVideoCodecString(CopySource{Codec: "av1", Profile: "Main", Level: new(8), ColorTransfer: &bt709}),
		"AV1 Main L4.0 8-bit")
	assert.Equal(t, "av01.0.13M.10",
		CopyVideoCodecString(CopySource{Codec: "av1", Profile: "Main", Level: new(13), TenBit: true, ColorTransfer: &pq}),
		"AV1 Main L5.1 10-bit")
	assert.Equal(t, "vp09.00.41.08",
		CopyVideoCodecString(CopySource{Codec: "vp9", Profile: "Profile 0", Level: new(41), ColorTransfer: &bt709}),
		"VP9 Profile 0 L4.1")
	assert.Equal(t, "vp09.02.41.10",
		CopyVideoCodecString(CopySource{Codec: "vp9", Profile: "Profile 2", Level: new(41), ColorTransfer: &pq}),
		"VP9 Profile 2 is the 10-bit variant even when bit depth wasn't probed")
}
