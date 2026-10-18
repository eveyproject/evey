package transcoding

// pixel_format.go - the pixel formats a client declares it decodes
// (ClientProfile.PixelFormats), in one vocabulary the source's probed pix_fmt
// folds into: chroma layout plus bit depth, the way ffmpeg names them, without
// what does not change the decoder's answer (endianness, full range,
// semi-planar packing).

import (
	"fmt"
	"slices"
	"strings"
)

// pixelFormatAliases folds the semi-planar / GPU spellings onto the planar
// family a decoder is asked for: an nv12 surface is 8-bit 4:2:0, p010 is
// 10-bit 4:2:0.
var pixelFormatAliases = map[string]string{
	"nv12": "yuv420p", "nv21": "yuv420p",
	"p010": "yuv420p10", "p012": "yuv420p12", "p016": "yuv420p16",
	"nv16": "yuv422p", "p210": "yuv422p10", "p212": "yuv422p12", "p216": "yuv422p16",
	"nv24": "yuv444p", "nv42": "yuv444p", "p410": "yuv444p10", "p412": "yuv444p12", "p416": "yuv444p16",
}

// CanonicalPixelFormat folds a pix_fmt onto the family vocabulary:
// "yuv420p10le" and "p010le" are both "yuv420p10", "yuvj420p" and "nv12" are
// "yuv420p". A name outside the table is returned lower-cased and stripped of
// its endianness, so it can still match a client that declares it verbatim.
func CanonicalPixelFormat(pixFmt string) string {
	f := strings.ToLower(strings.TrimSpace(pixFmt))
	if n := len(f); n > 2 && (strings.HasSuffix(f, "le") || strings.HasSuffix(f, "be")) && f[n-3] >= '0' && f[n-3] <= '9' {
		f = f[:n-2]
	}
	if alias, ok := pixelFormatAliases[f]; ok {
		return alias
	}
	return strings.Replace(f, "yuvj", "yuv", 1)
}

// SourcePixelFormat is the family a source asks of the client's decoder: its
// pix_fmt when probed, else 4:2:0 at its bit depth (the layout of every
// release that carries no pix_fmt). "" when neither is known.
func SourcePixelFormat(pixFmt string, bitDepth int) string {
	if f := CanonicalPixelFormat(pixFmt); f != "" {
		return f
	}
	switch {
	case bitDepth <= 0:
		return ""
	case bitDepth <= 8:
		return "yuv420p"
	}
	return fmt.Sprintf("yuv420p%d", bitDepth)
}

// NormalizePixelFormats reads a client's declaration: each entry is a family
// the client decodes for every video codec ("yuv420p10") or for one codec
// only ("hevc:yuv420p10" - a TV whose HEVC decoder takes Main10 while its
// H.264 one stops at 8 bits). Entries are canonicalized and deduplicated;
// nil when nothing usable was declared, which leaves the decision on its
// built-in rule.
func NormalizePixelFormats(raw []string) []string {
	var out []string
	for _, entry := range raw {
		codec, format, qualified := strings.Cut(strings.TrimSpace(entry), ":")
		if !qualified {
			codec, format = "", codec
		}
		f := CanonicalPixelFormat(format)
		if f == "" {
			continue
		}
		if qualified {
			c := pixelFormatCodec(codec)
			if c == "" {
				continue
			}
			f = c + ":" + f
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// DecodesPixelFormat reports whether the client declared family for codec,
// unqualified or qualified with that codec. Only meaningful when the client
// declared a list (len(PixelFormats) > 0).
func (c ClientProfile) DecodesPixelFormat(codec, family string) bool {
	return slices.Contains(c.PixelFormats, family) ||
		slices.Contains(c.PixelFormats, pixelFormatCodec(codec)+":"+family)
}

func pixelFormatCodec(codec string) string {
	if c := CanonicalVideoCodec(codec); c != "" {
		return c
	}
	return strings.ToLower(strings.TrimSpace(codec))
}
