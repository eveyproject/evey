package transcoding

import "strings"

// CopySource is the probed video-stream metadata the copy-rung eligibility
// and the RFC-6381 CODECS advertise are computed from. A plain value object
// so the handler can project it from library.MediaStream without this package
// depending on the persistence shape of every field.
type CopySource struct {
	Codec   string
	Profile string
	// Level is the codec level_idc (H.264: 40 = L4.0; HEVC: 153 = L5.1), nil
	// when never probed. Without it the master cannot advertise an accurate
	// CODECS string, so nil gates copy off ("no copy without level").
	Level *int
	// TenBit only matters for H.264 (10-bit H.264 has no browser decode
	// support worth advertising); HEVC Main 10 is the whole point of the HEVC
	// copy rung.
	TenBit bool
	// ColorTransfer is the HDR ground-truth sentinel: the column only fills
	// when the stream was probed by a post-migration-000110 ffprobe - the same
	// probe that reads the Dolby Vision configuration record. nil on an HEVC
	// stream means "DoVi status unknown" and gates copy off (a DoVi profile-5
	// title raw-served as HDR10 renders green/purple).
	ColorTransfer *string
	// DoViProfile / DoViBLCompat mirror the DOVI configuration record. Only
	// profile 8 with a compatible base layer (1=HDR10, 2=SDR, 4=HLG) stays
	// copy-eligible: the base renders correctly, the RPU is ignored.
	DoViProfile  *int
	DoViBLCompat *int
	// FrameRateMode is "cfr" / "vfr" / "" (unknown), from mkvgo's structural
	// ops.Analyze (see the mkvgo adapter's AnalyzeFrameRateMode). A vfr source
	// gates copy off: the HLS copy segmenter cuts at keyframe boundaries
	// assuming a constant frame period, and a variable rate drifts A/V sync.
	FrameRateMode string
}

// CopyRungEligible reports whether a source video stream may be served as a
// video-COPY (remux) rung.
//
//   - H.264: 8-bit with a KNOWN level - without it the master cannot advertise
//     an accurate avc1.PPCCLL, and a wrong CODECS makes Safari/hls.js reject
//     the rendition. So an unknown level gates copy OFF; the title falls back
//     to transcode until a re-probe backfills the level.
//   - HEVC / AV1 / VP9 (the HDR-capable codecs): known level AND probed HDR
//     ground truth (ColorTransfer non-nil, see CopySource) AND a profile with
//     a wired CODECS string AND not Dolby Vision unless profile 8 with a
//     compatible base layer (DoVi rides HEVC profiles 4-8 and AV1 profile 10;
//     the nil check is a no-op for VP9). 10-bit is allowed - 4K HDR served
//     raw to capable clients with no tonemap and no encode is the point.
//   - Any codec: FrameRateMode != "vfr" - the HLS copy segmenter cuts
//     -c:v copy segments at keyframe boundaries assuming a constant frame
//     period; a variable-rate source drifts audio/video sync over the
//     segment. "" (never analyzed) does not gate, only a confirmed "vfr" does.
//
// The decision layer has already verified the CLIENT side (client.VideoCodecs
// contains the codec, SupportsHDR for HDR sources) before VideoDecision lands
// on "copy"; this guard is about the SOURCE being advertisable and colour-safe.
func CopyRungEligible(src CopySource) bool {
	return CopyBlocker(src) == ""
}

// CopyBlocker names the gate that keeps this source off the copy rung, or ""
// when it is copy-eligible. The names are metric labels (see
// SourceCopyBlocker): a closed set, one per gate above.
func CopyBlocker(src CopySource) string {
	if src.Level == nil {
		return "level_unknown" // no accurate CODECS without it
	}
	if strings.EqualFold(src.FrameRateMode, "vfr") {
		return "vfr" // keyframe-cut copy segments drift A/V
	}
	codec := strings.ToLower(strings.TrimSpace(src.Codec))
	if codec == "h264" {
		if src.TenBit {
			return "h264_10bit" // no meaningful client decode
		}
		return ""
	}

	var profileWired bool
	switch codec {
	case "hevc":
		profileWired = hevcProfileIdx(src.Profile) != 0
	case "av1":
		profileWired = av1ProfileIdx(src.Profile) >= 0
	case "vp9":
		profileWired = vp9ProfileIdx(src.Profile) >= 0
	default:
		return "codec_unwired"
	}
	if !profileWired {
		return "profile_unwired" // no wired CODECS string for this profile
	}
	if src.ColorTransfer == nil {
		return "hdr_truth_unknown" // HDR/DoVi never probed → not colour-safe
	}
	if src.DoViProfile != nil {
		if *src.DoViProfile != 8 || src.DoViBLCompat == nil {
			return "dovi_profile" // P5/P7/P10/unknown: base layer not raw-renderable
		}
		switch *src.DoViBLCompat {
		case 1, 2, 4: // HDR10 / SDR / HLG-compatible base layer
		default:
			return "dovi_profile"
		}
	}
	return ""
}

// CopyVideoCodecString returns the RFC-6381 codec string to advertise for a
// video-COPY rung of this source stream (avc1.PPCCLL / hvc1.P.C.Lxxx.B0 /
// av01.P.LLT.DD / vp09.PP.LL.DD), or "" when the stream is not copy-eligible.
// One call combining the guard with the codec string, so the manifest builder
// cannot advertise a copy rung without the eligibility gates having passed.
func CopyVideoCodecString(src CopySource) string {
	if !CopyRungEligible(src) {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(src.Codec)) {
	case "hevc":
		return hevcCodecString(src.Profile, *src.Level)
	case "av1":
		return av1CodecString(src.Profile, *src.Level, src.TenBit)
	case "vp9":
		return vp9CodecString(src.Profile, *src.Level, src.TenBit)
	}
	return avcCodecString(src.Profile, *src.Level)
}
