package hdr

import (
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

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
}

func DefaultToneMapOptions() ToneMapOptions {
	return ToneMapOptions{
		Algorithm: "hable",
		Desat:     0,
		Peak:      100,
	}
}

// DetectHDR analyzes video streams for HDR content
func DetectHDR(streams []*library.MediaStream) *HDRInfo {
	for _, s := range streams {
		if s.StreamType != "video" {
			continue
		}

		info := &HDRInfo{}

		if s.BitDepth != nil {
			info.BitDepth = *s.BitDepth
		}
		if s.ColorSpace != nil {
			info.ColorSpace = *s.ColorSpace
		}

		if s.HDRType != nil && *s.HDRType != "" {
			info.IsHDR = true
			info.Type = normalizeHDRType(*s.HDRType)
			return info
		}

		if s.ColorSpace != nil && strings.Contains(strings.ToLower(*s.ColorSpace), "bt2020") {
			if s.BitDepth != nil && *s.BitDepth >= 10 {
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

	algo := opts.Algorithm
	if algo == "" {
		algo = "hable"
	}

	npl := opts.Peak
	if npl <= 0 {
		npl = 100
	}

	return fmt.Sprintf(
		"zscale=t=linear:npl=%g,format=gbrpf32le,zscale=p=bt709,tonemap=%s:desat=%g,zscale=t=bt709:m=bt709:r=tv,format=yuv420p",
		npl, algo, opts.Desat,
	)
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
