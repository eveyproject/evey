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

		if cs := s.GetColorSpace(); cs != nil && strings.Contains(strings.ToLower(*cs), "bt2020") {
			if bd := s.GetBitDepth(); bd != nil && *bd >= 10 {
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
