package ffmpeg

import (
	"context"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveHWEncoder(t *testing.T) {
	tests := []struct {
		device string
		codec  string
		want   string
	}{
		{string(transcoding.VAAPI), "h264", "h264_vaapi"},
		{string(transcoding.VAAPI), "hevc", "hevc_vaapi"},
		{string(transcoding.NVENC), "h264", "h264_nvenc"},
		{string(transcoding.NVENC), "hevc", "hevc_nvenc"},
		{string(transcoding.QSV), "h264", "h264_qsv"},
		{string(transcoding.QSV), "hevc", "hevc_qsv"},
		{"unknown", "h264", ""},
		{string(transcoding.VAAPI), "H264", "h264_vaapi"},
	}

	for _, tt := range tests {
		t.Run(tt.device+"_"+tt.codec, func(t *testing.T) {
			got := resolveHWEncoder(tt.device, tt.codec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckEncoderAvailable_InvalidPath(t *testing.T) {
	ok := checkEncoderAvailable(t.Context(), "/nonexistent/ffmpeg", "libx264")
	assert.False(t, ok)
}

func TestNewHWDetector(t *testing.T) {
	d := NewHWDetector("/usr/bin/ffmpeg")
	assert.NotNil(t, d)
	assert.Equal(t, "/usr/bin/ffmpeg", d.ffmpegPath)
}

func TestDetectDevices_ReturnsNoError(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	devices, err := d.DetectDevices(t.Context())
	assert.NoError(t, err)
	// may or may not find devices, but should not error
	_ = devices
}

func TestSupportsCodec_UnknownDevice(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	supported, err := d.SupportsCodec(t.Context(), "unknown_device", "h264")
	require.NoError(t, err)
	assert.False(t, supported)
}

func TestSupportsCodec_ValidDevice_InvalidPath(t *testing.T) {
	d := NewHWDetector("/nonexistent/ffmpeg")
	supported, err := d.SupportsCodec(t.Context(), string(transcoding.VAAPI), "h264")
	require.NoError(t, err)
	assert.False(t, supported, "invalid ffmpeg path means encoder not available")
}

func TestProbeHWCodecs_UnknownType(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "unknown", "")
	assert.Nil(t, codecs)
}

func TestProbeHWCodecs_VAAPI_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "vaapi", "/dev/dri/renderD128")
	assert.Empty(t, codecs)
}

func TestProbeHWCodecs_NVENC_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "nvenc", "")
	assert.Empty(t, codecs)
}

func TestProbeHWCodecs_QSV_InvalidPath(t *testing.T) {
	codecs := probeHWCodecs(t.Context(), "/nonexistent/ffmpeg", "qsv", "/dev/dri/renderD128")
	assert.Empty(t, codecs)
}

// A present encoder whose real-encode probe fails must NOT be reported usable -
// this is the broken/absent-GPU guard (encoder compiled in but device dead).
func TestProbeHWEncode_NonexistentFfmpeg_FailsClosed(t *testing.T) {
	if probeHWEncode(t.Context(), "/nonexistent/ffmpeg", "h264_nvenc", "nvenc", "") {
		t.Fatal("probe must fail closed when ffmpeg cannot run")
	}
}

// TestDecodeProbeMatrix_CoversTheLibraryCodecs pins the matrix shape: one row
// per (codec, pix_fmt) class, every class key unique and round-tripping through
// decodeProbeFor, and the legacy/web codecs a library holds all present at the
// depths they ship in. VC-1 is deliberately absent (no ffmpeg encoder).
func TestDecodeProbeMatrix_CoversTheLibraryCodecs(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range hwDecodeProbes {
		key := transcoding.DecodeClassKey(c.codec, c.pixFmt)
		if seen[key] {
			t.Fatalf("duplicate decode class %q", key)
		}
		seen[key] = true
		if c.swEncoder == "" || c.format == "" || c.download == "" {
			t.Fatalf("row %q lacks encoder/format/download", key)
		}
		if transcoding.CanonicalVideoCodec(c.codec) != c.codec {
			t.Fatalf("row codec %q is not the canonical ffprobe name", c.codec)
		}
		row, ok := decodeProbeFor(key)
		if !ok || row.pixFmt != c.pixFmt {
			t.Fatalf("decodeProbeFor(%q) does not round-trip", key)
		}
	}
	for _, want := range []string{"h264", "hevc", transcoding.HWDecodeHEVC10, "mpeg2video", "vp8", "vp9", "vp9:yuv420p10le", "av1", "av1:yuv420p10le"} {
		if !seen[want] {
			t.Errorf("decode class %q missing from the probe matrix", want)
		}
	}
	if seen["vc1"] {
		t.Error("VC-1 cannot be probed (no encoder): it must not be listed as if it were")
	}
}

// The deinterlace probe reads idet's LAST multi-frame line and judges on the
// share of determined frames, with the thresholds measured on the woven mire
// (raw 25/25 TFF; bwdif / yadif_cuda leave 4-5 of 25 judged TFF).
func TestParseIdetCounts_DeinterlaceVerdict(t *testing.T) {
	raw := "[Parsed_idet_0 @ 0x1] Multi frame detection: TFF:     0 BFF:     0 Progressive:     0 Undetermined:     0\n" +
		"[Parsed_idet_0 @ 0x1] Multi frame detection: TFF:    25 BFF:     0 Progressive:     0 Undetermined:     0\n"
	v, ok := parseIdetCounts(raw)
	require.True(t, ok)
	assert.True(t, v.interlaced())
	assert.False(t, v.progressive())

	deint := "[Parsed_idet_1 @ 0x2] Multi frame detection: TFF:     5 BFF:     0 Progressive:    20 Undetermined:     0\n"
	v, ok = parseIdetCounts(deint)
	require.True(t, ok)
	assert.False(t, v.interlaced())
	assert.True(t, v.progressive())

	half := "Multi frame detection: TFF:    12 BFF:     0 Progressive:    13 Undetermined:     0"
	v, _ = parseIdetCounts(half)
	assert.False(t, v.progressive(), "a deinterlacer that left half the comb is not correct")

	_, ok = parseIdetCounts("frame=25\nprogress=end")
	assert.False(t, ok, "no idet line = the graph never ran")

	if probeHWDeinterlace(context.Background(), "/nonexistent/ffmpeg", "qsv", "/dev/dri/renderD128") {
		t.Fatal("no ffmpeg must fail closed")
	}
	if probeHWDeinterlace(context.Background(), "/nonexistent/ffmpeg", "vaapi", "/dev/dri/renderD128") {
		t.Fatal("per-frame backends never claim the session-level deinterlacer")
	}
}

// Render-node identification: QSV candidates are the Intel nodes by THEIR
// driver (i915/xe) or vendor id, never card0's; the admin label carries vendor
// + driver + node; a device row reports the default node when none was bound.
func TestRenderNodeIdentity(t *testing.T) {
	intel := drmNode{path: "/dev/dri/renderD129", vendor: "0x8086", driver: "i915"}
	amd := drmNode{path: "/dev/dri/renderD128", vendor: "0x1002", driver: "amdgpu"}
	xe := drmNode{path: "/dev/dri/renderD130", driver: "xe"}
	unknownIntel := drmNode{path: "/dev/dri/renderD131", vendor: "0x8086"}
	wsl := drmNode{path: "/dev/dri/renderD128"}
	assert.True(t, intel.intel())
	assert.True(t, xe.intel())
	assert.True(t, unknownIntel.intel(), "vendor id decides when the driver link is unreadable")
	assert.False(t, amd.intel())
	assert.False(t, wsl.intel())
	assert.Equal(t, "Intel i915 (renderD129)", intel.label())
	assert.Equal(t, "AMD amdgpu (renderD128)", amd.label())
	assert.Equal(t, "renderD128", wsl.label())

	infos := detectedDeviceInfos([]transcoding.HWDevice{
		{Type: "vaapi", Name: "/dev/dri/renderD129", Label: intel.label(), SupportedCodecs: []string{"h264_vaapi"}},
		{Type: "nvenc", Name: "", Label: "NVIDIA GeForce RTX 4070 Ti", SupportedCodecs: []string{"h264_nvenc", "av1_nvenc"}},
		{Type: "nvenc", Name: "1", Label: "NVIDIA T400"},
	})
	assert.Equal(t, "/dev/dri/renderD129", infos[0].Device)
	assert.Empty(t, infos[1].Device, "the first NVIDIA GPU is ffmpeg's default: no index")
	assert.Equal(t, "1", infos[2].Device)
	assert.Equal(t, []string{"nvenc", "vaapi"}, detectedBackends([]transcoding.HWDevice{{Type: "nvenc"}, {Type: "vaapi"}, {Type: "nvenc", Name: "1"}}))
	// Nodes are listed even when sysfs is unreadable (a container without /sys/class/drm).
	for _, n := range enumerateRenderNodes() {
		assert.True(t, strings.HasPrefix(n.path, "/dev/dri/renderD"), n.path)
	}
}
