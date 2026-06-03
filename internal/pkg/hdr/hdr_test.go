package hdr

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
)

func ptr[T any](v T) *T { return &v }

func toVS(ms []*library.MediaStream) []VideoStream {
	vs := make([]VideoStream, len(ms))
	for i, s := range ms {
		vs[i] = s
	}
	return vs
}

func TestDetectHDR_HDR10(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", HDRType: ptr("HDR10"), BitDepth: ptr(10), ColorSpace: ptr("bt2020nc")},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR {
		t.Fatal("expected HDR")
	}
	if info.Type != "hdr10" {
		t.Fatalf("expected hdr10, got %s", info.Type)
	}
}

func TestDetectHDR_HLG(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", HDRType: ptr("HLG"), BitDepth: ptr(10)},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR {
		t.Fatal("expected HDR")
	}
	if info.Type != "hlg" {
		t.Fatalf("expected hlg, got %s", info.Type)
	}
}

func TestDetectHDR_DolbyVision(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", HDRType: ptr("Dolby Vision"), BitDepth: ptr(10)},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR {
		t.Fatal("expected HDR")
	}
	if info.Type != "dolby_vision" {
		t.Fatalf("expected dolby_vision, got %s", info.Type)
	}
}

func TestDetectHDR_SDR(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", BitDepth: ptr(8)},
	}
	info := DetectHDR(toVS(streams))
	if info.IsHDR {
		t.Fatal("expected SDR")
	}
}

func TestDetectHDR_BT2020ColorSpace(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorSpace: ptr("bt2020nc"), BitDepth: ptr(10)},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR {
		t.Fatal("expected HDR from bt2020 color space + 10-bit")
	}
	if info.Type != "hdr10" {
		t.Fatalf("expected hdr10, got %s", info.Type)
	}
}

func TestDetectHDR_BT2020_8bit_NotHDR(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorSpace: ptr("bt2020nc"), BitDepth: ptr(8)},
	}
	info := DetectHDR(toVS(streams))
	if info.IsHDR {
		t.Fatal("bt2020 with 8-bit should not be HDR")
	}
}

func TestDetectHDR_BT2020_ProfileMain10_NoBitDepth(t *testing.T) {
	// Real-world 2160p HDR: bt2020 + HEVC "Main 10" profile, but ffprobe left
	// bits_per_raw_sample empty (BitDepth nil). The profile is the reliable
	// 10-bit signal here — must still be detected as HDR.
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", ColorSpace: new("bt2020nc"), Profile: "Main 10"},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR {
		t.Fatal("bt2020 + Main 10 profile (no bit_depth) must be detected as HDR")
	}
	if info.Type != "hdr10" {
		t.Fatalf("expected hdr10, got %s", info.Type)
	}
}

func TestDetectHDR_BT2020_NoTenBitSignal_NotHDR(t *testing.T) {
	// bt2020 but with no 10-bit evidence at all (no bit_depth, 8-bit profile) →
	// don't guess HDR (avoid false-positives on rare wide-gamut SDR).
	streams := []*library.MediaStream{
		{StreamType: "video", ColorSpace: new("bt2020nc"), Profile: "Main"},
	}
	info := DetectHDR(toVS(streams))
	if info.IsHDR {
		t.Fatal("bt2020 with no 10-bit signal must not be assumed HDR")
	}
}

func TestDetectHDR_NoVideoStreams(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "audio", Codec: "aac"},
	}
	info := DetectHDR(toVS(streams))
	if info.IsHDR {
		t.Fatal("audio-only should not be HDR")
	}
}

func TestDetectHDR_NilStreams(t *testing.T) {
	info := DetectHDR(nil)
	if info.IsHDR {
		t.Fatal("nil streams should not be HDR")
	}
}

func TestBuildToneMapFilter_HDR10(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hdr10"}
	filter := BuildToneMapFilter(info, DefaultToneMapOptions())
	if !strings.Contains(filter, "tonemap=hable") {
		t.Fatalf("expected tonemap=hable, got %s", filter)
	}
	if !strings.Contains(filter, "zscale=t=linear:npl=100") {
		t.Fatalf("expected zscale npl=100, got %s", filter)
	}
	if !strings.Contains(filter, "format=yuv420p") {
		t.Fatalf("expected format=yuv420p, got %s", filter)
	}
}

func TestBuildToneMapFilter_HLG(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hlg"}
	filter := BuildToneMapFilter(info, DefaultToneMapOptions())
	if !strings.Contains(filter, "tonemap=hable") {
		t.Fatalf("expected tonemap=hable, got %s", filter)
	}
	if !strings.Contains(filter, "zscale=t=linear:npl=100") {
		t.Fatalf("expected zscale npl=100, got %s", filter)
	}
}

func TestBuildToneMapFilter_SDR(t *testing.T) {
	info := &HDRInfo{IsHDR: false}
	filter := BuildToneMapFilter(info, DefaultToneMapOptions())
	if filter != "" {
		t.Fatalf("expected empty filter for SDR, got %s", filter)
	}
}

func TestBuildToneMapFilter_CustomOptions(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hdr10"}
	opts := ToneMapOptions{Algorithm: "reinhard", Desat: 2, Peak: 200}
	filter := BuildToneMapFilter(info, opts)
	if !strings.Contains(filter, "tonemap=reinhard:desat=2") {
		t.Fatalf("expected reinhard with desat=2, got %s", filter)
	}
	if !strings.Contains(filter, "npl=200") {
		t.Fatalf("expected npl=200, got %s", filter)
	}
}

func TestBuildToneMapFilter_DstWidthFoldsScale(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hdr10"}
	opts := DefaultToneMapOptions()
	opts.DstWidth = 1920
	filter := BuildToneMapFilter(info, opts)
	// The downscale is folded into the first (linearizing) zscale so the float
	// tonemap pipeline runs at delivery resolution, not the 4K source.
	if !strings.Contains(filter, "zscale=w=1920:h=-2:t=linear:npl=100") {
		t.Fatalf("expected folded downscale in the linearizing zscale, got %s", filter)
	}
	// DstWidth<=0 keeps the legacy full-resolution chain (no resize folded).
	opts.DstWidth = 0
	if f := BuildToneMapFilter(info, opts); strings.Contains(f, "w=") {
		t.Fatalf("DstWidth=0 must not fold a scale, got %s", f)
	}
}
