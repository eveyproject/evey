package hdr

import (
	"fmt"
	"strings"
)

type VideoStream interface {
	GetStreamType() string
	GetBitDepth() *int
	GetColorSpace() *string
	GetHDRType() *string
	GetProfile() string
}

type HDRInfo struct {
	IsHDR      bool
	Type       string // "hdr10", "hdr10+", "dolby_vision", "hlg", ""
	ColorSpace string
	BitDepth   int
}

type ToneMapOptions struct {
	Algorithm string // "hable", "reinhard", "mobius"
	Desat     float64
	Peak      float64
	// DstWidth, when > 0, folds a downscale to this width into the first
	// (linearizing) zscale so the expensive float32 tonemap pipeline runs at the
	// DELIVERY resolution instead of the source. A 4K→1080p title otherwise pays
	// 4× the per-pixel float math + memory bandwidth on the dominant stage.
	// zscale resizes in linear light (t=linear) → colour-correct AND ~2.1× faster
	// (empirically validated on a 4K HDR10 sample). 0 = no resize: the caller
	// scales separately (e.g. the subtitle burn-in path, which must overlay at a
	// fixed size before scaling).
	DstWidth int

	// UseTonemapx selects jellyfin-ffmpeg's `tonemapx` filter (one AVX2/SIMD pass)
	// instead of the portable zscale float32 chain. Measured on an old box, same
	// file/encoder/1080p: 1.56x realtime (tonemapx) vs 0.89x (zscale) — the gap
	// that flips 4K HDR from buffering to playable. Only set when the active
	// ffmpeg actually has the filter (vanilla ffmpeg lacks it); the builder falls
	// back to zscale when false, so the code is portable across both builds.
	UseTonemapx bool
}

func DefaultToneMapOptions() ToneMapOptions {
	return ToneMapOptions{
		Algorithm: "hable",
		Desat:     0,
		Peak:      100,
	}
}

func DetectHDR(streams []VideoStream) *HDRInfo {
	for _, s := range streams {
		if s.GetStreamType() != "video" {
			continue
		}

		info := &HDRInfo{}

		if bd := s.GetBitDepth(); bd != nil {
			info.BitDepth = *bd
		}
		if cs := s.GetColorSpace(); cs != nil {
			info.ColorSpace = *cs
		}

		if ht := s.GetHDRType(); ht != nil && *ht != "" {
			info.IsHDR = true
			info.Type = normalizeHDRType(*ht)
			return info
		}

		// BT.2020 colour space + 10-bit ⇒ HDR (HDR10/HLG can't be told apart
		// without the transfer characteristics, which Evey doesn't store yet, so
		// we report hdr10 as the conservative default). ffprobe leaves
		// bits_per_raw_sample empty for many HEVC streams, so the codec profile
		// ("Main 10" / "High 10") is the reliable 10-bit signal when bit depth is
		// absent — without this fallback, real 2160p HDR (e.g. bt2020nc + Main 10
		// with no bits_per_raw_sample) is misread as SDR.
		if cs := s.GetColorSpace(); cs != nil && strings.Contains(strings.ToLower(*cs), "bt2020") {
			tenBit := false
			if bd := s.GetBitDepth(); bd != nil && *bd >= 10 {
				tenBit = true
			} else if strings.Contains(strings.ToLower(s.GetProfile()), "10") {
				tenBit = true
			}
			if tenBit {
				info.IsHDR = true
				info.Type = "hdr10"
				return info
			}
		}

		return info
	}

	return &HDRInfo{}
}

// BuildToneMapFilter generates ffmpeg filter string for HDR→SDR conversion.
// Returns empty string for SDR content.
func BuildToneMapFilter(info *HDRInfo, opts ToneMapOptions) string {
	if !info.IsHDR {
		return ""
	}

	npl := opts.Peak
	if npl <= 0 {
		npl = 100
	}

	if opts.UseTonemapx {
		// jellyfin-ffmpeg's `tonemapx`: linearize + tonemap + convert in a single
		// AVX2/SIMD pass, ~1.8x faster than the zscale float32 round-trips on the
		// same CPU (old box, 1080p, same file/encoder: 1.56x vs 0.89x realtime).
		// bt2390 is the ITU-R EETF (tonemapx's recommended HDR10 curve). Scale
		// BEFORE the tonemap so it runs at the delivery resolution, and tag the
		// frame's colour metadata first (mirrors Jellyfin — some rips under-tag).
		scale := ""
		if opts.DstWidth > 0 {
			scale = fmt.Sprintf("scale=%d:-2,", opts.DstWidth)
		}
		return fmt.Sprintf(
			"%s%stonemapx=tonemap=bt2390:desat=%g:peak=%g:t=bt709:m=bt709:p=bt709:format=yuv420p",
			setParamsFor(info), scale, opts.Desat, npl,
		)
	}

	algo := opts.Algorithm
	if algo == "" {
		algo = "hable"
	}

	// Fold the downscale into the first zscale (which already runs at t=linear)
	// so tonemap operates on the delivery-resolution frame, not the 4K source.
	// h=-2 keeps the aspect ratio at an even height (valid yuv420p chroma).
	scale := ""
	if opts.DstWidth > 0 {
		scale = fmt.Sprintf("w=%d:h=-2:", opts.DstWidth)
	}

	return fmt.Sprintf(
		"zscale=%st=linear:npl=%g,format=gbrpf32le,zscale=p=bt709,tonemap=%s:desat=%g,zscale=t=bt709:m=bt709:r=tv,format=yuv420p",
		scale, npl, algo, opts.Desat,
	)
}

// setParamsFor tags the decoded HDR frame's colour properties before tonemapx,
// mirroring jellyfin-ffmpeg's defensive setparams (some rips carry incomplete
// metadata). HLG uses the arib-std-b67 transfer; everything else (HDR10/HDR10+,
// and the Dolby Vision base layer) is treated as PQ (smpte2084).
func setParamsFor(info *HDRInfo) string {
	trc := "smpte2084" // PQ — HDR10 / HDR10+ / DoVi base layer
	if info != nil && info.Type == "hlg" {
		trc = "arib-std-b67"
	}
	return fmt.Sprintf("setparams=color_primaries=bt2020:color_trc=%s:colorspace=bt2020nc,", trc)
}

func normalizeHDRType(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "dolby vision", "dolbyvision", "dovi":
		return "dolby_vision"
	case "hdr10+", "hdr10plus":
		return "hdr10+"
	case "hdr10", "hdr":
		return "hdr10"
	case "hlg":
		return "hlg"
	default:
		return lower
	}
}
