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
	// when the stream was probed by a post-migration-000110 ffprobe — the same
	// probe that reads the Dolby Vision configuration record. nil on an HEVC
	// stream means "DoVi status unknown" and gates copy off (a DoVi profile-5
	// title raw-served as HDR10 renders green/purple).
	ColorTransfer *string
	// DoViProfile / DoViBLCompat mirror the DOVI configuration record. Only
	// profile 8 with a compatible base layer (1=HDR10, 2=SDR, 4=HLG) stays
	// copy-eligible: the base renders correctly, the RPU is ignored.
	DoViProfile  *int
	DoViBLCompat *int
}

// CopyRungEligible reports whether a source video stream may be served as a
// video-COPY (remux) rung.
//
//   - H.264: 8-bit with a KNOWN level — without it the master cannot advertise
//     an accurate avc1.PPCCLL, and a wrong CODECS makes Safari/hls.js reject
//     the rendition. So an unknown level gates copy OFF; the title falls back
//     to transcode until a re-probe backfills the level.
//   - HEVC: known level AND probed HDR ground truth (ColorTransfer non-nil,
//     see CopySource) AND a Main/Main 10 profile (the two with a wired CODECS
//     string) AND not Dolby Vision unless profile 8 with a compatible base
//     layer. 10-bit is allowed — HEVC Main 10 HDR served raw to HDR-capable
//     clients (Safari/TV) is the entire point: no tonemap, no encode.
//
// The decision layer has already verified the CLIENT side (client.VideoCodecs
// contains the codec, SupportsHDR for HDR sources) before VideoDecision lands
// on "copy"; this guard is about the SOURCE being advertisable and colour-safe.
func CopyRungEligible(src CopySource) bool {
	if src.Level == nil {
		return false // unknown level → no accurate CODECS → no copy
	}
	switch strings.ToLower(strings.TrimSpace(src.Codec)) {
	case "h264":
		// 10-bit H.264 is out of the copy scope (no meaningful client decode).
		return !src.TenBit
	case "hevc":
		if src.ColorTransfer == nil {
			return false // HDR/DoVi ground truth never probed → not colour-safe
		}
		if hevcProfileIdx(src.Profile) == 0 {
			return false // only Main / Main 10 have a wired CODECS string
		}
		if src.DoViProfile != nil {
			if *src.DoViProfile != 8 || src.DoViBLCompat == nil {
				return false // P5/P7/unknown: base layer not raw-renderable
			}
			switch *src.DoViBLCompat {
			case 1, 2, 4: // HDR10 / SDR / HLG-compatible base layer
			default:
				return false
			}
		}
		return true
	}
	return false
}

// CopyVideoCodecString returns the RFC-6381 codec string to advertise for a
// video-COPY rung of this source stream ("avc1.PPCCLL" / "hvc1.P.C.Lxxx.B0"),
// or "" when the stream is not copy-eligible. One call combining the guard
// with the codec string, so the manifest builder cannot advertise a copy rung
// without the eligibility gates having passed.
func CopyVideoCodecString(src CopySource) string {
	if !CopyRungEligible(src) {
		return ""
	}
	if strings.ToLower(strings.TrimSpace(src.Codec)) == "hevc" {
		return hevcCodecString(src.Profile, *src.Level)
	}
	return avcCodecString(src.Profile, *src.Level)
}
