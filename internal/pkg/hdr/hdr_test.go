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

// A Dolby Vision profile of 0 is no profile (an unread or blank DoVi record):
// an 8-bit SDR stream carrying it stays SDR instead of being sent down the
// always-transcode Dolby Vision path (mutation 2026-09-28).
func TestDetectHDR_DoViProfileZeroIsNotDolbyVision(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", BitDepth: new(8), DoViProfile: new(0)},
	}
	if info := DetectHDR(toVS(streams)); info.IsHDR {
		t.Fatalf("a zero DoVi profile must not make the stream HDR, got %+v", info)
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
	// 10-bit signal here - must still be detected as HDR.
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

func TestBuildToneMapFilter_Tonemapx_HDR10(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hdr10"}
	opts := DefaultToneMapOptions()
	opts.DstWidth = 1920
	opts.UseTonemapx = true
	filter := BuildToneMapFilter(info, opts)

	// The fast SIMD path: tonemapx (bt2390 EETF), scale-before-tonemap at delivery
	// resolution, PQ setparams, yuv420p out - and NONE of the slow zscale chain.
	for _, want := range []string{
		"setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc",
		"scale=1920:-2",
		"tonemapx=tonemap=bt2390:desat=0:peak=100:t=bt709:m=bt709:p=bt709:format=yuv420p",
	} {
		if !strings.Contains(filter, want) {
			t.Fatalf("tonemapx filter missing %q, got %s", want, filter)
		}
	}
	if strings.Contains(filter, "zscale") || strings.Contains(filter, "gbrpf32le") {
		t.Fatalf("tonemapx path must NOT use the zscale float32 chain, got %s", filter)
	}
}

func TestBuildToneMapFilter_Tonemapx_HLG(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hlg"}
	opts := DefaultToneMapOptions()
	opts.DstWidth = 1280
	opts.UseTonemapx = true
	filter := BuildToneMapFilter(info, opts)
	if !strings.Contains(filter, "color_trc=arib-std-b67") {
		t.Fatalf("HLG tonemapx must tag the arib-std-b67 transfer, got %s", filter)
	}
}

func TestBuildToneMapFilter_Tonemapx_FallsBackToZscaleWhenOff(t *testing.T) {
	info := &HDRInfo{IsHDR: true, Type: "hdr10"}
	opts := DefaultToneMapOptions()
	opts.DstWidth = 1920
	opts.UseTonemapx = false // vanilla ffmpeg → portable path
	filter := BuildToneMapFilter(info, opts)
	if strings.Contains(filter, "tonemapx") {
		t.Fatalf("UseTonemapx=false must emit the zscale fallback, got %s", filter)
	}
	if !strings.Contains(filter, "zscale=w=1920:h=-2:t=linear") {
		t.Fatalf("fallback must be the folded zscale chain, got %s", filter)
	}
}

// --- HDR ground truth (color_transfer + DoVi configuration record) ---

func TestDetectHDR_TransferPQ(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("smpte2084")},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR || info.Type != "hdr10" {
		t.Fatalf("PQ transfer must detect hdr10, got IsHDR=%v Type=%q", info.IsHDR, info.Type)
	}
}

func TestDetectHDR_TransferHLG(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("arib-std-b67"), ColorSpace: new("bt2020nc"), BitDepth: new(10)},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR || info.Type != "hlg" {
		t.Fatalf("HLG transfer must win over the bt2020 hdr10 fallback, got IsHDR=%v Type=%q", info.IsHDR, info.Type)
	}
}

func TestDetectHDR_TransferSDR(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("bt709"), ColorSpace: new("bt709")},
	}
	info := DetectHDR(toVS(streams))
	if info.IsHDR {
		t.Fatalf("bt709 transfer is SDR, got Type=%q", info.Type)
	}
}

// DoVi profile 5 (IPTC2 base layer) must always be reported dolby_vision -
// even though it also carries a PQ transfer - so the playback decision
// transcodes it (raw-serving renders green/purple on non-DoVi pipelines).
func TestDetectHDR_DoViProfile5_AlwaysDolbyVision(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("smpte2084"), DoViProfile: new(5)},
	}
	info := DetectHDR(toVS(streams))
	if !info.IsHDR || info.Type != "dolby_vision" {
		t.Fatalf("DoVi P5 must be dolby_vision, got IsHDR=%v Type=%q", info.IsHDR, info.Type)
	}
	if info.DoViProfile != 5 {
		t.Fatalf("DoViProfile must be surfaced, got %d", info.DoViProfile)
	}
}

// DoVi profile 7 (Blu-ray dual layer, possibly FEL) has no single-layer
// compatible rendering contract → conservative dolby_vision (transcode).
func TestDetectHDR_DoViProfile7_DolbyVision(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("smpte2084"), DoViProfile: new(7), DoViBLCompat: new(1)},
	}
	info := DetectHDR(toVS(streams))
	if info.Type != "dolby_vision" {
		t.Fatalf("DoVi P7 must stay dolby_vision, got %q", info.Type)
	}
}

// DoVi profile 8 with a compatible base layer is served as that base:
// bl_compat 1/2 ⇒ hdr10 (PQ base), 4 ⇒ hlg. The DoVi presence stays surfaced
// via DoViProfile.
func TestDetectHDR_DoViProfile8_CompatibleBase(t *testing.T) {
	cases := []struct {
		compat int
		want   string
	}{
		{1, "hdr10"},
		{2, "hdr10"},
		{4, "hlg"},
	}
	for _, c := range cases {
		streams := []*library.MediaStream{
			{StreamType: "video", ColorTransfer: new("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(c.compat)},
		}
		info := DetectHDR(toVS(streams))
		if !info.IsHDR || info.Type != c.want {
			t.Fatalf("DoVi P8 compat=%d: want %q, got IsHDR=%v Type=%q", c.compat, c.want, info.IsHDR, info.Type)
		}
		if info.DoViProfile != 8 {
			t.Fatalf("DoVi P8 compat=%d: DoViProfile must be surfaced, got %d", c.compat, info.DoViProfile)
		}
	}
}

// Profile 8 with an unknown/absent compatibility id has no proven base layer
// → conservative dolby_vision.
func TestDetectHDR_DoViProfile8_UnknownCompat(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", ColorTransfer: new("smpte2084"), DoViProfile: new(8)},
	}
	info := DetectHDR(toVS(streams))
	if info.Type != "dolby_vision" {
		t.Fatalf("DoVi P8 without bl_compat must stay dolby_vision, got %q", info.Type)
	}
	streams[0].DoViBLCompat = new(0)
	info = DetectHDR(toVS(streams))
	if info.Type != "dolby_vision" {
		t.Fatalf("DoVi P8 bl_compat=0 must stay dolby_vision, got %q", info.Type)
	}
}
