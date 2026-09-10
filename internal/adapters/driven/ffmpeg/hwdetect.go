package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// hwProbeTimeout bounds the real-encode capability probe. A working HW encoder
// finishes a 0.2s clip near-instantly; a broken/absent device either errors out
// fast or hangs, so a tight timeout keeps boot snappy.
const hwProbeTimeout = 12 * time.Second

type HWDetector struct {
	ffmpegPath string
}

func NewHWDetector(ffmpegPath string) *HWDetector {
	return &HWDetector{ffmpegPath: ffmpegPath}
}

func (d *HWDetector) DetectDevices(ctx context.Context) ([]transcoding.HWDevice, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.hwdetect.DetectDevices")
	defer span.End()
	var devices []transcoding.HWDevice

	if dev := detectVAAPI(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}
	if dev := detectNVENC(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}
	if dev := detectQSV(ctx, d.ffmpegPath); dev != nil {
		devices = append(devices, *dev)
	}

	return devices, nil
}

func (d *HWDetector) SupportsCodec(ctx context.Context, deviceType, codec string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.hwdetect.SupportsCodec")
	defer span.End()
	encoder := resolveHWEncoder(deviceType, codec)
	if encoder == "" {
		return false, nil
	}
	return checkEncoderAvailable(ctx, d.ffmpegPath, encoder), nil
}

func detectVAAPI(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	driPath := "/dev/dri"
	entries, err := os.ReadDir(driPath)
	if err != nil {
		return nil
	}

	hasRender := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "renderD") {
			hasRender = true
			break
		}
	}
	if !hasRender {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "vaapi", filepath.Join(driPath, "renderD128"))
	return &transcoding.HWDevice{
		Type:            string(transcoding.VAAPI),
		Name:            filepath.Join(driPath, "renderD128"),
		SupportedCodecs: codecs,
	}
}

func detectNVENC(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	// Check nvidia-smi
	smiPath, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}

	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.nvidia_smi"}, smiPath, "--query-gpu=name", "--format=csv,noheader")
	if err != nil {
		return nil
	}

	gpuName := strings.TrimSpace(string(out))
	if gpuName == "" {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "nvenc", "")
	return &transcoding.HWDevice{
		Type:            string(transcoding.NVENC),
		Name:            gpuName,
		SupportedCodecs: codecs,
	}
}

func detectQSV(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	// Check for /dev/dri/renderD128 with Intel GPU
	if _, err := os.Stat("/dev/dri/renderD128"); err != nil {
		return nil
	}

	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.drm_driver"}, "ls", "/sys/class/drm/card0/device/driver/module/drivers/")
	if err != nil {
		return nil
	}

	if !strings.Contains(string(out), "i915") {
		return nil
	}

	codecs := probeHWCodecs(ctx, ffmpegPath, "qsv", "/dev/dri/renderD128")
	return &transcoding.HWDevice{
		Type:            string(transcoding.QSV),
		Name:            "Intel Quick Sync",
		SupportedCodecs: codecs,
	}
}

func probeHWCodecs(ctx context.Context, ffmpegPath, hwType, device string) []string {
	var encoderSuffixes []string
	switch hwType {
	case "vaapi":
		encoderSuffixes = []string{"h264_vaapi", "hevc_vaapi"}
	case "nvenc":
		encoderSuffixes = []string{"h264_nvenc", "hevc_nvenc"}
	case "qsv":
		encoderSuffixes = []string{"h264_qsv", "hevc_qsv"}
	default:
		return nil
	}

	var codecs []string
	for _, enc := range encoderSuffixes {
		if !checkEncoderAvailable(ctx, ffmpegPath, enc) {
			continue
		}
		// Presence in `ffmpeg -encoders` is necessary but NOT sufficient: the
		// encoder is compiled in even when the GPU/driver is absent or broken,
		// and selecting it then fails at playtime (hw_unsupported_option) and
		// stalls the player ~30s before the level-2 demotion can react. Confirm
		// the device actually encodes with a tiny real probe; fail closed.
		if !probeHWEncode(ctx, ffmpegPath, enc, hwType, device) {
			slog.Warn("hwdetect: encoder present but real-encode probe failed, treating as unavailable",
				slog.String("encoder", enc), slog.String("hw", hwType))
			continue
		}
		codecs = append(codecs, enc)
	}
	return codecs
}

// probeHWDecodeCodecs returns the domain video codecs the GPU can HW-DECODE via
// hwType. Encode capability does NOT imply decode capability: a pre-Skylake Intel
// iGPU encodes H.264 but cannot decode HEVC, so feeding such a source to
// `-hwaccel <type>` fails (ENOSYS) and stalls the player. Probing decode
// separately lets the transcode pipeline software-decode what the GPU can't, and
// the admin see an honest encode/decode support matrix.
func probeHWDecodeCodecs(ctx context.Context, ffmpegPath, hwType, device string) []string {
	var out []string
	for _, c := range []struct{ name, swEncoder, format string }{
		{"h264", "libx264", "h264"},
		{"hevc", "libx265", "hevc"},
	} {
		if probeHWDecode(ctx, ffmpegPath, c.swEncoder, c.format, hwType, device) {
			out = append(out, c.name)
		}
	}
	return out
}

// probeHWDecode software-encodes a tiny real clip in the target codec, then tries
// to HW-DECODE it through hwType. Exit 0 → the GPU decodes that codec. Fail-closed
// on any error/timeout (worst case: software decode, a perf cost, never a stall).
func probeHWDecode(ctx context.Context, ffmpegPath, swEncoder, format, hwType, device string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	tmp, err := os.CreateTemp("", "evey-decprobe-*."+format)
	if err != nil {
		return false
	}
	tmpPath := tmp.Name()
	// Register cleanup before the Close check so an early return on Close failure
	// still removes the temp file (no leak).
	defer func() {
		if rerr := os.Remove(tmpPath); rerr != nil {
			slog.Debug("hwdetect: decode-probe temp cleanup failed", slog.Any("err", rerr))
		}
	}()
	if cerr := tmp.Close(); cerr != nil {
		return false
	}

	// 1. software-encode a tiny real sample of the codec.
	if procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.decode_sample"}, ffmpegPath, "-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.4",
		"-c:v", swEncoder, "-f", format, tmpPath) != nil {
		return false
	}

	// 2. HW-decode it through the backend; -hwaccel_output_format forces the GPU
	// decode path (no silent software fallback), so success means real HW decode.
	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-hwaccel_device", device)
	case "qsv":
		args = append(args, "-hwaccel", "qsv", "-hwaccel_output_format", "qsv")
	case "nvenc":
		args = append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	default:
		return false
	}
	args = append(args, "-i", tmpPath, "-f", "null", "-")
	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_decode"}, ffmpegPath, args...) == nil
}

// probeHWEncode runs a tiny real encode through the hardware encoder to confirm
// THIS host's GPU/driver can actually use it - not merely that ffmpeg was built
// with it. Fail-closed: returns false on any error/timeout, so a dead backend is
// never reported as usable. Worst case we fall back to software (a perf cost),
// which is far better than the broken-NVENC playtime loop it replaces.
func probeHWEncode(ctx context.Context, ffmpegPath, encoder, hwType, device string) bool {
	return probeHWEncodeWith(ctx, ffmpegPath, encoder, hwType, device, nil)
}

// probeHWEncodeWith is probeHWEncode with encoder options appended (feature probes).
func probeHWEncodeWith(ctx context.Context, ffmpegPath, encoder, hwType, device string, encoderArgs []string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	var filter []string
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-vaapi_device", device)
		filter = []string{"-vf", "format=nv12,hwupload"}
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		filter = []string{"-vf", "hwupload=extra_hw_frames=16,format=qsv"}
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=256x144:rate=10:duration=0.2")
	args = append(args, filter...)
	args = append(args, "-c:v", encoder)
	args = append(args, encoderArgs...)
	args = append(args, "-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_encode"}, ffmpegPath, args...) == nil
}

// probeHWHDR10Encode: HEVC Main10 from a p010 surface with the passthrough args. Fail-closed.
func probeHWHDR10Encode(ctx context.Context, ffmpegPath, hwType, device string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	var vf, enc string
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-vaapi_device", device)
		vf, enc = "format=p010le,hwupload,scale_vaapi=128:-2:format=p010", "hevc_vaapi"
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		vf, enc = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=128:-2:format=p010", "hevc_qsv"
	case "nvenc":
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		vf, enc = "format=p010le,hwupload_cuda,scale_cuda=128:-2:format=p010", "hevc_nvenc"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.3")
	args = append(args, "-vf", vf, "-c:v", enc)
	args = append(args, hwHDRPassthroughArgs(transcoding.HWAccelType(hwType), "pq", -1)...)
	args = append(args, "-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_hdr10_encode"}, ffmpegPath, args...) == nil
}

// probeHW10BitTranscode reports whether the HW VPP can scale a 10-bit (p010)
// surface down to 8-bit nv12 - the conversion every transcode of an HEVC Main10
// / yuv420p10le source needs before the 8-bit HW encoder (hwScaleFilter emits
// scale_<backend>=W:-2:format=nv12). Encode AND decode capability imply nothing
// about it: the Intel i965 VEBOX aborts (SIGABRT, fourcc assertion) the
// instant a 10-bit surface reaches scale_vaapi, so without this probe every
// 2160p HDR source crashes the HW pipeline and churns through a demotion. It
// uploads a synthetic p010 frame (no HEVC decode - a pre-Skylake iGPU can't do
// that anyway) and runs the real per-backend scaler. Fail-closed: any
// error/abort/timeout ⇒ false ⇒ the source software-transcodes.
func probeHW10BitTranscode(ctx context.Context, ffmpegPath, hwType, device string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	var vf string
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-vaapi_device", device)
		vf = "format=p010le,hwupload,scale_vaapi=64:-2:format=nv12"
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		vf = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=64:-2:format=nv12"
	case "nvenc":
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		vf = "format=p010le,hwupload_cuda,scale_cuda=64:-2:format=nv12"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.3")
	args = append(args, "-vf", vf, "-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_10bit"}, ffmpegPath, args...) == nil
}

// probeHWTonemap reports whether the HW backend can run an HDR->SDR tone-map on
// the GPU. It runs the real per-backend tone-map filter on a synthetic p010
// surface; success means the filter graph initialises and processes a frame.
// Gen9+ Intel (tonemap_vaapi / vpp_qsv) and a Vulkan libplacebo build pass;
// old-iGPU-class VEBOX and hosts without a Vulkan/OpenCL ICD (incl. WSL) fail.
// Fail-closed: any error/abort/timeout ⇒ false ⇒ HDR tone-maps in software.
// hwOpenCLTonemapChain maps the VAAPI surface into OpenCL, tone-maps with the
// same bt2390/peak=100 curve as the software path, and maps back for the encoder.
const hwOpenCLTonemapChain = "hwmap=derive_device=opencl,tonemap_opencl=format=nv12:primaries=bt709:transfer=bt709:matrix=bt709:tonemap=bt2390:peak=100:desat=0,hwmap=derive_device=vaapi:reverse=1:extra_hw_frames=16"

// probeHWTonemapOpenCL: VAAPI only; the synthetic frame is tagged PQ because
// tonemap_opencl refuses an SDR input. Failure output is logged: a missing ICD
// or a bad option must be visible in the boot log, not a silent "no".
func probeHWTonemapOpenCL(ctx context.Context, ffmpegPath, hwType, device string) bool {
	if hwType != "vaapi" || device == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-vaapi_device", device,
		"-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.3",
		"-vf", "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload," + hwOpenCLTonemapChain + ",hwdownload,format=nv12",
		"-f", "null", "-"}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap_opencl"}, ffmpegPath, args...)
	if err != nil {
		text := string(out)
		if len(text) > 1200 {
			text = text[:1200]
		}
		slog.Info("hwdetect: OpenCL tone-map probe failed", slog.String("head", text), slog.String("tail", tailString(string(out), 300)))
		return false
	}
	return true
}

// probeHWBFrames runs a short encode at -v verbose: vaapi_encode reports the
// GOP it could build, and a driver without B support silently degrades to P.
func probeHWBFrames(ctx context.Context, ffmpegPath, encoder, hwType, device string, mode []string) bool {
	if hwType != "vaapi" || device == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-v", "verbose", "-nostdin", "-vaapi_device", device,
		"-f", "lavfi", "-i", "testsrc2=size=256x144:rate=10:duration=0.5",
		"-vf", "format=nv12,hwupload", "-c:v", encoder}
	args = append(args, mode...)
	args = append(args, "-bf", "3", "-f", "null", "-")
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_bframes"}, ffmpegPath, args...)
	return err == nil && strings.Contains(string(out), "P- and B-frames")
}

func probeHWTonemap(ctx context.Context, ffmpegPath, hwType, device string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	var vf string
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-vaapi_device", device)
		vf = "format=p010le,hwupload,tonemap_vaapi=format=nv12:matrix=bt709:primaries=bt709:transfer=bt709,hwdownload,format=nv12"
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		vf = "format=p010le,hwupload=extra_hw_frames=16,vpp_qsv=tonemap=1:format=nv12,hwdownload,format=nv12"
	case "nvenc":
		// NVIDIA has no native tonemap filter; the GPU path is libplacebo (Vulkan).
		args = append(args, "-init_hw_device", "vulkan=vk", "-filter_hw_device", "vk")
		vf = "format=p010le,hwupload,libplacebo=tonemapping=bt.2390:colorspace=bt709:color_primaries=bt709:color_trc=bt709:format=nv12,hwdownload,format=nv12"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.3")
	args = append(args, "-vf", vf, "-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap"}, ffmpegPath, args...) == nil
}

// probeHWOverlay reports whether the HW backend can composite a bitmap subtitle
// onto decoded frames on the GPU. It runs the real per-backend overlay filter on
// two synthetic surfaces - an opaque "video" and a translucent "subtitle" - so
// success means the blend actually configures and processes a frame.
//
// Both inputs are uploaded and the overlay is given an EXPLICIT w/h matching the
// main surface. That is not decoration: with the sizes left implicit the Intel
// VPP answers "Error querying VPP params: unsupported" (measured on UHD 630,
// 2026-09-09), so a probe without them would report a false negative on hardware
// that composites perfectly well once told the geometry. The production builder
// scales the subtitle to the rung and passes the same explicit w/h.
//
// Fail-closed: any error/abort/timeout ⇒ false ⇒ the burn-in keeps the software
// path, which is what every host did before this capability existed.
func probeHWOverlay(ctx context.Context, ffmpegPath, hwType, device string) bool {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	var main, sub, blend string
	switch hwType {
	case "vaapi":
		if device == "" {
			return false
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va")
		main, sub, blend = "format=nv12,hwupload", "format=bgra,hwupload", "overlay_vaapi"
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		main = "format=nv12,hwupload=extra_hw_frames=16"
		sub = "format=bgra,hwupload=extra_hw_frames=16"
		blend = "overlay_qsv"
	case "nvenc":
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		// overlay_cuda takes the overlay as yuva420p, not bgra: it blends in the
		// planar YUV domain rather than through a VPP colour converter.
		main, sub, blend = "format=nv12,hwupload_cuda", "format=yuva420p,hwupload_cuda", "overlay_cuda"
	default:
		return false
	}
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size=128x128:rate=5:duration=0.3",
		"-f", "lavfi", "-i", "color=c=white@0.5:size=128x128:rate=5:duration=0.3",
		"-filter_complex", fmt.Sprintf("[0:v]%s[m];[1:v]%s[s];[m][s]%s=w=128:h=128", main, sub, blend),
		"-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_overlay"}, ffmpegPath, args...) == nil
}

// classifyGPU labels the calibrated accelerator integrated vs discrete for the
// admin panel. NVENC is always a discrete NVIDIA card; QSV is always an Intel
// iGPU. For VAAPI the PCI vendor at /sys/class/drm/<node>/device/vendor decides:
// Intel (0x8086) integrated, NVIDIA (0x10de) discrete. AMD (0x1002) shares one
// vendor across APUs and Radeon dGPUs, so it's left unclassified ("") rather
// than guessed; "" is also returned on hosts without the sysfs node (a WSL/d3d12
// virtual render node exposes no PCI vendor).
func classifyGPU(backend, device string) string {
	switch backend {
	case "nvenc":
		return "discrete"
	case "qsv":
		return "integrated"
	case "vaapi":
		node := filepath.Base(device)
		if node == "" || node == "." || node == "/" {
			return ""
		}
		b, err := os.ReadFile("/sys/class/drm/" + node + "/device/vendor")
		if err != nil {
			return ""
		}
		switch strings.TrimSpace(string(b)) {
		case "0x8086":
			return "integrated"
		case "0x10de":
			return "discrete"
		}
	}
	return ""
}

func checkEncoderAvailable(ctx context.Context, ffmpegPath, encoder string) bool {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.encoders"}, ffmpegPath, "-encoders")
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == encoder {
			return true
		}
	}
	return false
}

func resolveHWEncoder(deviceType, codec string) string {
	codec = strings.ToLower(codec)
	switch deviceType {
	case string(transcoding.VAAPI):
		return fmt.Sprintf("%s_vaapi", codec)
	case string(transcoding.NVENC):
		return fmt.Sprintf("%s_nvenc", codec)
	case string(transcoding.QSV):
		return fmt.Sprintf("%s_qsv", codec)
	}
	return ""
}
