package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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
		if (c.swEncoder == "") == (c.fixture == "") || c.format == "" || c.download == "" {
			t.Fatalf("row %q needs exactly one sample source (encoder xor fixture), a format and a download layout", key)
		}
		if transcoding.CanonicalVideoCodec(c.codec) != c.codec {
			t.Fatalf("row codec %q is not the canonical ffprobe name", c.codec)
		}
		row, ok := decodeProbeFor(key)
		if !ok || row.pixFmt != c.pixFmt {
			t.Fatalf("decodeProbeFor(%q) does not round-trip", key)
		}
	}
	for _, want := range []string{"h264", "hevc", transcoding.HWDecodeHEVC10, "mpeg2video", "vc1", "vp8", "vp9", "vp9:yuv420p10le", "av1", "av1:yuv420p10le"} {
		if !seen[want] {
			t.Errorf("decode class %q missing from the probe matrix", want)
		}
	}
}

// VC-1 is a normal matrix row whose sample is a FIXTURE (ffmpeg has no VC-1
// encoder). No fixture - embedded or operator-provided - means the class is
// not probed at all (no ffmpeg run, no listing); a fixture that is not a
// decodable VC-1 stream fails closed; and the decode half always carries the
// production hwaccel prologue plus the mandatory hwdownload.
func TestDecodeProbe_VC1FixtureRow(t *testing.T) {
	row, ok := decodeProbeFor("vc1")
	require.True(t, ok)
	assert.Equal(t, "vc1-yuv420p.vc1", row.fixture)
	assert.Empty(t, row.swEncoder, "no encoder can synthesise VC-1")
	assert.Equal(t, "vc1", row.format, "raw VC-1 Advanced Profile elementary stream")

	t.Setenv(decodeFixtureEnv, "")
	if _, found := decodeFixtureBytes(row.fixture); found {
		t.Fatal("no VC-1 fixture is bundled (none is licence-clean): the embedded lookup must miss")
	}
	// No fixture: not probed, and ffmpeg is never even invoked (a bogus path
	// would otherwise be an error, which is also false - but this is cheaper).
	assert.False(t, probeHWDecode(context.Background(), "/nonexistent/ffmpeg", row, "nvenc", ""))

	// The operator's directory is consulted by file name; garbage bytes are a
	// fixture too, and must fail closed through the real decode.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, row.fixture), []byte("not a vc1 stream"), 0o600))
	t.Setenv(decodeFixtureEnv, dir)
	b, ok := decodeFixtureBytes(row.fixture)
	require.True(t, ok)
	assert.Equal(t, "not a vc1 stream", string(b))
	assert.False(t, probeHWDecode(context.Background(), "/nonexistent/ffmpeg", row, "nvenc", ""))

	args, ok := hwDecodeProbeArgs("nvenc", "", "/tmp/x.vc1", row.download)
	require.True(t, ok)
	argv := strings.Join(args, " ")
	assert.Contains(t, argv, "-hwaccel cuda -hwaccel_output_format cuda -i /tmp/x.vc1 -vf hwdownload,format=nv12")
	args, _ = hwDecodeProbeArgs("vaapi", "/dev/dri/renderD129", "/tmp/x.vc1", row.download)
	assert.Contains(t, strings.Join(args, " "), "-hwaccel_device /dev/dri/renderD129 -i /tmp/x.vc1 -vf hwdownload,format=nv12")
	_, ok = hwDecodeProbeArgs("vaapi", "", "/tmp/x.vc1", row.download)
	assert.False(t, ok, "VAAPI without a render node cannot prove anything")
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

// The tone-map gate's bounds against the populations measured on the RTX 4070
// Ti (see hw_tonemap_validation.go): the correct CUDA curve on the level-scaled
// mire passes, every failure shape is refused, a too-dark reference skips the
// saturation ratio, structure loss (garbage) is refused by the pHash distance.
func TestTonemapGate_MeasuredPopulations(t *testing.T) {
	ref := frameStats{meanY: 78.7, meanSat: 71.7}
	same := uint64(0x0f0f0f0f0f0f0f0f)
	pass := foldTonemapVerdict(tonemapVerdict{OK: true, LumaRatio: 1, SatRatio: 1}, ref, frameStats{meanY: 80.8, meanSat: 70.7}, same, same)
	assert.True(t, pass.OK, "tonemap_cuda on the lvl40 mire: %s", pass)
	assert.True(t, pass.SatChecked)

	cases := map[string]frameStats{
		"no tone-map at all (naive 10->8 bit)": {meanY: 63.5, meanSat: 51.2},
		"washed + half saturation (VPP shape)": {meanY: 95.7, meanSat: 35.7},
		"25 % desaturated":                     {meanY: 78.7, meanSat: 53.8},
		"black":                                {meanY: 4, meanSat: 1},
	}
	for name, got := range cases {
		v := foldTonemapVerdict(tonemapVerdict{OK: true, LumaRatio: 1, SatRatio: 1}, ref, got, same, same)
		assert.False(t, v.OK, "%s must be refused: %s", name, v)
	}
	// Structure: a picture that is not the reference (hash 32+ bits away).
	v := foldTonemapVerdict(tonemapVerdict{OK: true, LumaRatio: 1, SatRatio: 1}, ref, frameStats{meanY: 80, meanSat: 70}, 0, ^uint64(0))
	assert.False(t, v.OK)
	// A dark real scene: saturation too small to judge, luma still judged.
	dark := foldTonemapVerdict(tonemapVerdict{OK: true, LumaRatio: 1, SatRatio: 1}, frameStats{meanY: 27.5, meanSat: 2.8}, frameStats{meanY: 26.6, meanSat: 3.4}, same, same)
	assert.True(t, dark.OK)
	assert.False(t, dark.SatChecked)
	// Worst case is kept across frames.
	two := foldTonemapVerdict(pass, ref, frameStats{meanY: 70, meanSat: 72}, same, same)
	assert.InDelta(t, 70.0/78.7, two.LumaRatio, 1e-9)

	// Every probed implementation is gated with the feature set that selects
	// it in the builder; the new Vulkan route and the measured CUDA one are
	// enforced, the production-validated OpenCL/VPP curves run in shadow.
	impls := tonemapImplsToGate("vaapi", true, true, true)
	require.Len(t, impls, 3)
	assert.Equal(t, transcoding.FeatureTonemapOpenCL, impls[0].name)
	assert.Equal(t, hwOpenCLTonemapChain, hwTonemapFilter(transcoding.VAAPI, impls[0].features))
	assert.Equal(t, hwVulkanTonemapChain, hwTonemapFilter(transcoding.VAAPI, impls[1].features))
	assert.Contains(t, hwTonemapFilter(transcoding.VAAPI, impls[2].features), "tonemap_vaapi=")
	assert.True(t, tonemapGateEnforced[transcoding.FeatureTonemapVulkan])
	assert.True(t, tonemapGateEnforced[transcoding.FeatureTonemapCUDA])
	assert.False(t, tonemapGateEnforced[transcoding.FeatureTonemapOpenCL])
	assert.Empty(t, tonemapImplsToGate("qsv", true, false, false), "the QSV VPP curve is ineligible: never certified, so never gated")
}

// The Vulkan/libplacebo probe: VAAPI only, on the named node (never a
// hard-coded renderD128), the production chain bracketed by upload/download;
// no ffmpeg / no Vulkan = fail-closed with no feature.
func TestProbeHWTonemapVulkan_ShapeAndFailClosed(t *testing.T) {
	ok, argv, _, _ := probeHWTonemapVulkanE(context.Background(), "/nonexistent/ffmpeg", "vaapi", "/dev/dri/renderD129")
	assert.False(t, ok)
	assert.Contains(t, argv, "-vaapi_device /dev/dri/renderD129 ")
	assert.Contains(t, argv, "hwupload,"+hwVulkanTonemapChain+",hwdownload,format=nv12")
	assert.NotContains(t, argv, "renderD128")
	for _, hw := range []string{"nvenc", "qsv", "rkmpp"} {
		other, otherArgv, _, _ := probeHWTonemapVulkanE(context.Background(), "/nonexistent/ffmpeg", hw, "/dev/dri/renderD128")
		assert.False(t, other)
		assert.Empty(t, otherArgv, "%s has no VAAPI->Vulkan interop route", hw)
	}
	ok, _, _, _ = probeHWTonemapVulkanE(context.Background(), "/nonexistent/ffmpeg", "vaapi", "")
	assert.False(t, ok, "no render node, nothing to derive from")
	// A host without Vulkan answers deterministically: no retry burns boot time.
	assert.Equal(t, transcoding.FailureHWUnsupportedOption, classifyVulkanProbe(assert.AnError, "No such filter: 'libplacebo'"))
	assert.Equal(t, transcoding.FailureHWUnsupportedOption, classifyVulkanProbe(assert.AnError, "[AVHWDeviceContext] Failed to create Vulkan instance: -9"))
	assert.Equal(t, transcoding.FailureHWInit, classifyVulkanProbe(assert.AnError, "Failed to map frame: -38"))
}
