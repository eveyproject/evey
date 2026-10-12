package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// hw_gpu_repro_test.go - end-to-end reproductions on a real NVIDIA GPU, run
// through the SAME Go path production takes (BuildFFmpegArgs -> HLSTranscoder
// .Start -> handle.Failure()). Skipped unless EVEY_GPU_TEST=nvenc; meant to be
// compiled with `go test -c` and executed inside the runtime image with the
// GPU attached (the ffmpeg on PATH must be the bundled jellyfin build).
//
// Reproduced 2026-09-12/13 on an RTX 4070 Ti, image sha-21830e0-full:
//   - a progressive HDR10 title on the tone-map bridge failed with the chain
//     the builder emitted then (GPU deinterlacer in front of hwdownload);
//   - Hi10P and HEVC 4:2:2 sources on the zero-copy chain fail and used to be
//     classified as a host-wide "unsupported option".

func gpuTestFFmpeg(t *testing.T) string {
	t.Helper()
	if os.Getenv("EVEY_GPU_TEST") != "nvenc" {
		t.Skip("EVEY_GPU_TEST=nvenc not set")
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	return p
}

// gpuMire encodes a 1 s progressive clip of the given class.
func gpuMire(t *testing.T, ffmpeg, dst, encoder, pixFmt string, extra ...string) string {
	t.Helper()
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=1,format=" + pixFmt,
		"-c:v", encoder, "-pix_fmt", pixFmt}
	args = append(args, extra...)
	args = append(args, "-an", dst)
	out, err := exec.Command(ffmpeg, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("mire %s: %v\n%s", dst, err, out)
	}
	return dst
}

var gpuReproFeatures = []string{"rc_lookahead", "spatial_aq", "temporal_aq", "b_ref_mode", "bframes"}

func gpuSessionOpts(src, outDir string) transcoding.HLSProcessOpts {
	return transcoding.HLSProcessOpts{
		OutputDir: outDir, FilePath: src,
		Profiles:      []transcoding.HLSProfile{{Name: "360p", Width: 640, Height: 360, VideoBitrate: 1500, AudioBitrate: 96}},
		VideoDecision: "transcode", AudioDecision: "transcode", AudioTrackIndex: -1, SubtitleTrackIndex: -1,
		VideoCodec: "h264", HWAccel: transcoding.NVENC, HWFeatures: gpuReproFeatures, UseFMP4: true,
	}
}

// runSession starts the session exactly as the orchestrator does and returns
// its failure class once ffmpeg exited, plus whether a media segment landed.
func runSession(t *testing.T, opts transcoding.HLSProcessOpts) (transcoding.FailureClass, bool, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	h, err := NewHLSTranscoder().Start(ctx, opts)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-h.Done():
	case <-ctx.Done():
		_ = h.Stop()
		t.Fatal("session did not exit in time")
	}
	segs, _ := filepath.Glob(filepath.Join(opts.OutputDir, "stream_0_*.m4s"))
	hh := h.(*hlsHandle)
	return h.Failure(), len(segs) > 0, hh.stderr.String()
}

// The zero-copy chain on a source NVDEC does not decode: the session fails
// and MUST classify as a media/route incompatibility, never as a host verdict.
func TestGPU_UndecodableSourcesAreRouteIncompatible(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	dir := t.TempDir()
	cases := map[string]string{
		"hi10p":   gpuMire(t, ffmpeg, filepath.Join(dir, "hi10p.mkv"), "libx264", "yuv420p10le", "-profile:v", "high10"),
		"hevc422": gpuMire(t, ffmpeg, filepath.Join(dir, "hevc422.mkv"), "libx265", "yuv422p10le", "-profile:v", "main422-10"),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			class, produced, stderr := runSession(t, gpuSessionOpts(src, t.TempDir()))
			if produced {
				t.Fatalf("%s: the zero-copy chain unexpectedly produced a segment", name)
			}
			if class != transcoding.FailureRouteIncompatible {
				t.Fatalf("%s: class = %q, want %q\nstderr:\n%s", name, class, transcoding.FailureRouteIncompatible, stderr)
			}
			if class.IsHWCapability() {
				t.Fatalf("%s: a route incompatibility must never be a host capability failure", name)
			}
		})
	}
}

// The legacy/web source classes the extended decode matrix lists ride the
// builder's zero-copy chain end-to-end: NVDEC decodes them straight into the
// CUDA pool the scaler and encoder consume, like an H.264/HEVC source.
func TestGPU_LegacyAndWebSourcesRideTheZeroCopyChain(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	dir := t.TempDir()
	cases := map[string]string{
		"mpeg2":  gpuMire(t, ffmpeg, filepath.Join(dir, "mpeg2.mkv"), "mpeg2video", "yuv420p"),
		"vp8":    gpuMire(t, ffmpeg, filepath.Join(dir, "vp8.webm"), "libvpx", "yuv420p", "-deadline", "realtime", "-cpu-used", "8"),
		"vp9":    gpuMire(t, ffmpeg, filepath.Join(dir, "vp9.webm"), "libvpx-vp9", "yuv420p", "-deadline", "realtime", "-cpu-used", "8"),
		"vp9_10": gpuMire(t, ffmpeg, filepath.Join(dir, "vp9_10.webm"), "libvpx-vp9", "yuv420p10le", "-profile:v", "2", "-deadline", "realtime", "-cpu-used", "8"),
		"av1":    gpuMire(t, ffmpeg, filepath.Join(dir, "av1.mkv"), "libsvtav1", "yuv420p", "-preset", "12"),
		"av1_10": gpuMire(t, ffmpeg, filepath.Join(dir, "av1_10.mkv"), "libsvtav1", "yuv420p10le", "-preset", "12"),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			class, produced, stderr := runSession(t, gpuSessionOpts(src, t.TempDir()))
			if !produced || class != "" {
				t.Fatalf("%s: zero-copy session produced=%v class=%q\nstderr:\n%s", name, produced, class, stderr)
			}
		})
	}
}

// A progressive HDR10 title on the CPU tone-map bridge: the chain the builder
// emitted until 2026-09-12 (GPU deinterlacer, then hwdownload) fails on the
// real GPU and classifies as route-incompatible; the chain it emits now
// produces the segment.
func TestGPU_ProgressiveHDRBridge(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	dir := t.TempDir()
	src := gpuMire(t, ffmpeg, filepath.Join(dir, "hdr10.hevc"), "libx265", "yuv420p10le",
		"-x265-params", "hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")

	opts := gpuSessionOpts(src, filepath.Join(dir, "out"))
	opts.ToneMap, opts.HWTonemap = true, false
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	args := BuildFFmpegArgs(opts)
	vf := args[slices.Index(args, "-vf")+1]
	if !strings.HasPrefix(vf, "hwdownload,format=p010le,bwdif=") {
		t.Fatalf("builder no longer emits the fixed bridge chain: %s", vf)
	}

	// 1. the previous chain, byte-for-byte what the builder used to emit.
	oldVF := "yadif_cuda=deint=interlaced," + strings.Replace(vf, "bwdif=mode=send_frame:deint=interlaced,", "", 1)
	oldArgs := slices.Clone(args)
	oldArgs[slices.Index(oldArgs, "-vf")+1] = oldVF
	oldDir := filepath.Join(dir, "old")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, a := range oldArgs {
		oldArgs[i] = strings.ReplaceAll(a, opts.OutputDir, oldDir)
	}
	out, err := exec.Command(ffmpeg, append([]string{"-hide_banner", "-nostdin"}, oldArgs...)...).CombinedOutput()
	if err == nil {
		t.Fatalf("the pre-fix bridge chain unexpectedly succeeded on a progressive source:\n%s", oldVF)
	}
	if class := ClassifyHWFailure(err, string(out)); class != transcoding.FailureRouteIncompatible {
		t.Fatalf("pre-fix chain classified %q, want route_incompatible\n%s", class, tailString(string(out), 600))
	}

	// 2. the chain the builder emits now, through the real session path.
	class, produced, stderr := runSession(t, opts)
	if class != transcoding.FailureNone || !produced {
		t.Fatalf("fixed bridge: class=%q produced=%v\n%s", class, produced, stderr)
	}
}

// The calibration probes on the real GPU: decode classes come back only with
// a frame pulled off the device, and every route the builder can emit for
// NVENC runs end-to-end on a clip of its class.
func TestGPU_ProbesAreFailClosedAndRoutesRunThroughTheBuilder(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	ctx := context.Background()
	got := probeHWDecodeCodecs(ctx, ffmpeg, "nvenc", "")
	// Ada NVDEC: H.264, HEVC 8/10, and the legacy/web codecs (MPEG-2, VP8,
	// VP9 8/10, AV1 8/10) - each listed from a frame pulled off the device.
	for _, want := range []string{"h264", "hevc", transcoding.HWDecodeHEVC10,
		"mpeg2video", "vp8", "vp9", "vp9:yuv420p10le", "av1", "av1:yuv420p10le"} {
		if !slices.Contains(got, want) {
			t.Errorf("decode classes %v lack %s", got, want)
		}
	}
	// Measured on this rig (Ada): no 10-bit H.264, no 4:2:2 - and the matrix
	// says so from frames, not from a table.
	for _, absent := range []string{"h264:yuv420p10le", "h264:yuv422p10le", "hevc:yuv422p10le"} {
		if slices.Contains(got, absent) {
			t.Errorf("decode classes %v unexpectedly list %s", got, absent)
		}
	}
	t.Logf("decode classes: %v", got)
	// Ada encodes AV1: the real-encode probe lists av1_nvenc next to h264/hevc.
	encoders := probeHWCodecs(ctx, ffmpeg, "nvenc", "")
	if !slices.Contains(encoders, "av1_nvenc") {
		t.Errorf("encoders %v lack av1_nvenc on an Ada GPU", encoders)
	}
	// The bundled jellyfin-ffmpeg carries tonemap_cuda: the GPU tone-map probe
	// passes and the feature drives the builder's tonemap route (on a
	// PROGRESSIVE HDR10 mire, the class the bridge once failed on).
	tonemap, features := resolveTonemapCaps("nvenc", slices.Clone(gpuReproFeatures), probeHWTonemap(ctx, ffmpeg, "nvenc", ""), false, false)
	if !tonemap || !slices.Contains(features, transcoding.FeatureTonemapCUDA) {
		t.Errorf("tonemap_cuda probe failed on a jellyfin-ffmpeg + NVIDIA rig: tonemap=%v features=%v", tonemap, features)
	}
	r := NewBenchmarkRunner(BenchmarkOpts{FfmpegPath: ffmpeg, SamplePath: filepath.Join(t.TempDir(), "sample.hevc")}, nil)
	routes := r.probeHWRoutes(ctx, "nvenc", "", hwDomainCodecs(encoders), features, tonemap, true, true, false, got)
	t.Logf("routes: %v", routes)
	if slices.Contains(got, "hevc:yuv444p") && !slices.Contains(routes, transcoding.HWRouteSourceFormat("yuv444p")) {
		t.Errorf("4:4:4 decodes on this GPU but its zero-copy route did not prove: %v", routes)
	}
	for _, want := range []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteAV1, transcoding.HWRouteSource10Bit, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRoutePassthrough} {
		if !slices.Contains(routes, want) {
			t.Errorf("routes %v lack %s", routes, want)
		}
	}
	if !r.HWEncodeUsable(ctx, "nvenc", "", "h264", gpuReproFeatures) {
		t.Error("HWEncodeUsable with the probed features must pass on a working GPU")
	}
	if kept := r.ConfirmHWFeatures(ctx, "nvenc", "", "h264", gpuReproFeatures); !slices.Equal(kept, gpuReproFeatures) {
		t.Errorf("ConfirmHWFeatures kept %v, want all of %v", kept, gpuReproFeatures)
	}
}

// The detector enumerates every device with its identity: on this rig one
// NVIDIA GPU (index 0 = the empty default device, labelled by nvidia-smi's
// model) - and every certification probe binds that device explicitly.
func TestGPU_DetectDevicesEnumeratesAndBinds(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	ctx := context.Background()
	devices, err := NewHWDetector(ffmpeg).DetectDevices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var nv *transcoding.HWDevice
	for i := range devices {
		t.Logf("device: %+v", devices[i])
		if devices[i].Type == "nvenc" && nv == nil {
			nv = &devices[i]
		}
	}
	if nv == nil || nv.Name != "" || !strings.Contains(nv.Label, "NVIDIA") || !slices.Contains(nv.SupportedCodecs, "av1_nvenc") {
		t.Fatalf("NVIDIA GPU 0 not enumerated as the default device with its model: %+v", nv)
	}
	// An explicit CUDA index binds the same GPU through every probe shape.
	if !probeHWEncode(ctx, ffmpeg, "h264_nvenc", "nvenc", "0") || !probeHW10BitTranscode(ctx, ffmpeg, "nvenc", "0") ||
		!probeHWTonemap(ctx, ffmpeg, "nvenc", "0") || !probeHWOverlay(ctx, ffmpeg, "nvenc", "0") {
		t.Fatal("probes bound to CUDA index 0 must pass like the default device")
	}
	if probeHWEncode(ctx, ffmpeg, "h264_nvenc", "nvenc", "7") {
		t.Fatal("a CUDA index that does not exist must fail closed")
	}
}

// The NVENC GPU burn-in (overlay_cuda) and tone-map (tonemap_cuda) on a REAL
// HDR10 title with a PGS track, through the production builder, bounded to a
// few seconds. Needs EVEY_GPU_PGS_SAMPLE=<file> and EVEY_GPU_PGS_TRACK=<n>
// (the n-th subtitle stream, 0-based) - measured 2026-09-13 on Avatar 2160p.
func TestGPU_OverlayAndTonemapOnRealTitle(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	src, track := os.Getenv("EVEY_GPU_PGS_SAMPLE"), os.Getenv("EVEY_GPU_PGS_TRACK")
	if src == "" || track == "" {
		t.Skip("EVEY_GPU_PGS_SAMPLE / EVEY_GPU_PGS_TRACK not set")
	}
	var n int
	if _, err := fmt.Sscanf(track, "%d", &n); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	opts := gpuSessionOpts(src, out)
	opts.StartSec = 600
	opts.SubtitleTrackIndex, opts.SubtitleCodec, opts.HWOverlay = n, "hdmv_pgs_subtitle", true
	opts.ToneMap, opts.HWTonemap = true, true
	opts.HWFeatures = append(slices.Clone(gpuReproFeatures), transcoding.FeatureTonemapCUDA)
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	args := BuildFFmpegArgs(opts)
	// Bound the run: -t before the output (the last arg is the playlist path).
	args = append(args[:len(args)-1], "-t", "4", args[len(args)-1])
	argv := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-nostats", "-y"}, args...)
	t.Logf("argv: ffmpeg %s", strings.Join(argv, " "))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stderr, err := exec.CommandContext(ctx, ffmpeg, argv...).CombinedOutput()
	if err != nil {
		t.Fatalf("overlay+tonemap session failed: %v\n%s", err, stderr)
	}
	segs, _ := filepath.Glob(filepath.Join(out, "stream_0_*.m4s"))
	if len(segs) == 0 {
		t.Fatalf("no segment written\n%s", stderr)
	}
	t.Logf("segments: %d", len(segs))
}

// The tone-map correctness gate on the real GPU: the production tonemap_cuda
// chain agrees with the software reference on the gate mire (within the
// measured bounds), a chain that does not tone-map at all is refused, and the
// backend-level gate keeps CUDA certified.
func TestGPU_TonemapGateMeasuresCUDA(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	ctx := context.Background()
	r := NewBenchmarkRunner(BenchmarkOpts{FfmpegPath: ffmpeg, SamplePath: filepath.Join(t.TempDir(), "sample.hevc")}, nil)
	v, err := r.validateTonemapImpl(ctx, "nvenc", "", hwCUDATonemapChain)
	if err != nil {
		t.Fatalf("gate inconclusive: %v", err)
	}
	t.Logf("tonemap_cuda vs tonemapx: %s", v)
	if !v.OK || !v.SatChecked {
		t.Fatalf("the production CUDA curve must pass the gate: %s", v)
	}
	naive, err := r.validateTonemapImpl(ctx, "nvenc", "", "scale_cuda=format=nv12")
	if err != nil {
		t.Fatalf("naive gate inconclusive: %v", err)
	}
	t.Logf("no tone-map (10->8 bit fold only) vs tonemapx: %s", naive)
	if naive.OK {
		t.Fatalf("a chain that does not tone-map must be refused: %s", naive)
	}
	if native, _, _ := r.gateTonemapImpls(ctx, "nvenc", "", true, false, false); !native {
		t.Fatal("gateTonemapImpls dropped a correct CUDA tone-mapper")
	}
}

// The fixture path of the decode matrix, end to end on NVDEC: a row whose
// sample comes from the operator's fixture directory decodes on the GPU with
// the same hwdownload proof as a synthesised one; a missing fixture is simply
// not probed.
func TestGPU_DecodeFixtureRowsProbeLikeTheOthers(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	ctx := context.Background()
	dir := t.TempDir()
	gpuMire(t, ffmpeg, filepath.Join(dir, "h264-fixture.h264"), "libx264", "yuv420p")
	t.Setenv(decodeFixtureEnv, dir)
	row := hwDecodeProbe{codec: "h264", format: "h264", pixFmt: "yuv420p", download: "nv12", fixture: "h264-fixture.h264"}
	if !probeHWDecode(ctx, ffmpeg, row, "nvenc", "") {
		t.Fatal("a fixture-driven row must decode like a synthesised one")
	}
	row.fixture = "absent.h264"
	if probeHWDecode(ctx, ffmpeg, row, "nvenc", "") {
		t.Fatal("a missing fixture must not list the class")
	}
	// The real VC-1 row on this rig: no fixture bundled, so it is not listed
	// even though NVDEC decodes VC-1 - the proof is the fixture, not the GPU.
	vc1, _ := decodeProbeFor("vc1")
	if probeHWDecode(ctx, ffmpeg, vc1, "nvenc", "") {
		t.Fatal("VC-1 without a fixture must stay unlisted")
	}
}

// The Vulkan/libplacebo interop probe on a host without a usable Vulkan ICD
// for the render node (this WSL rig): fail-closed, classified so the boot
// does not spend the readiness schedule retrying it.
func TestGPU_VulkanTonemapProbeFailsClosedWithoutICD(t *testing.T) {
	ffmpeg := gpuTestFFmpeg(t)
	ok, argv, tail, err := probeHWTonemapVulkanE(context.Background(), ffmpeg, "vaapi", "/dev/dri/renderD128")
	t.Logf("argv: %s\nclass: %s\nstderr: %s", argv, classifyVulkanProbe(err, tail), tail)
	if ok {
		t.Skip("this rig has a working VAAPI->Vulkan interop: nothing to assert here")
	}
}
