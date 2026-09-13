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
	"strconv"
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

// hwProbeMire is the synthetic frame size every probe encodes or decodes:
// above every accelerator's minimum (NVDEC refuses HEVC under 144x144; a
// 128x128 mire once made the decode probe fail for a reason that had nothing to
// do with the host's ability to decode) and small enough to stay instant.
const hwProbeMire = "256x144"

// hwDecodeProbes is the decode probe MATRIX: every (codec, chroma, depth)
// class the library can hold, each with the software encoder that makes its
// sample. The persisted class is transcoding.DecodeClassKey(codec, pixFmt).
// Nothing here is assumed decodable or undecodable: an Ada NVDEC lists no
// 4:2:2 class, a Blackwell one does, a Skylake iGPU lists no Main10, a
// pre-Ice-Lake iGPU no AV1 - each host answers from its own frames.
// Extending coverage is adding a row.
//
// Beyond H.264/HEVC the matrix covers the legacy and web codecs a library
// holds (MPEG-2 TV/DVD rips, VP8/VP9 web rips, AV1 8/10-bit), each through the
// same fail-closed hwdownload proof - the matrix is codec x pix_fmt, never a
// per-vendor support table. VC-1 is the one codec NOT probed: ffmpeg carries no
// VC-1 encoder, so no sample can be made and the class is never listed - a
// VC-1 source software-decodes (cheap at its 1080p ceiling). The web encoders
// run at their fastest speed: the sample only has to exist, not look good.
//
// download is the software layout(s) the device pool hands back for the class
// (hwdownload output must be pinned: an open-ended list fails negotiation).
var hwDecodeProbes = []hwDecodeProbe{
	{"h264", "libx264", "h264", "yuv420p", nil, "nv12"},
	{"h264", "libx264", "h264", "yuv420p10le", []string{"-profile:v", "high10"}, "p010le"},
	{"h264", "libx264", "h264", "yuv422p10le", []string{"-profile:v", "high422"}, "y210le|p210le|yuv422p10le"},
	{"hevc", "libx265", "hevc", "yuv420p", nil, "nv12"},
	{"hevc", "libx265", "hevc", "yuv420p10le", nil, "p010le"},
	{"hevc", "libx265", "hevc", "yuv422p10le", []string{"-profile:v", "main422-10"}, "y210le|p210le|yuv422p10le"},
	{"hevc", "libx265", "hevc", "yuv444p", []string{"-profile:v", "main444-8"}, "yuv444p"},
	{"hevc", "libx265", "hevc", "yuv444p10le", []string{"-profile:v", "main444-10"}, "yuv444p16le"},
	{"hevc", "libx265", "hevc", "yuv420p12le", []string{"-profile:v", "main12"}, "p016le|p012le"},
	{"mpeg2video", "mpeg2video", "mpeg2video", "yuv420p", nil, "nv12"},
	{"vp8", "libvpx", "ivf", "yuv420p", []string{"-deadline", "realtime", "-cpu-used", "8"}, "nv12"},
	{"vp9", "libvpx-vp9", "ivf", "yuv420p", []string{"-deadline", "realtime", "-cpu-used", "8"}, "nv12"},
	{"vp9", "libvpx-vp9", "ivf", "yuv420p10le", []string{"-profile:v", "2", "-deadline", "realtime", "-cpu-used", "8"}, "p010le"},
	{"av1", "libsvtav1", "ivf", "yuv420p", []string{"-preset", "12"}, "nv12"},
	{"av1", "libsvtav1", "ivf", "yuv420p10le", []string{"-preset", "12"}, "p010le"},
}

type hwDecodeProbe struct {
	codec, swEncoder, format, pixFmt string
	encoderArgs                      []string
	download                         string
}

// decodeProbeFor returns the matrix row of a persisted decode class, so a
// route probe can build a mire of exactly that class.
func decodeProbeFor(class string) (hwDecodeProbe, bool) {
	for _, c := range hwDecodeProbes {
		if transcoding.DecodeClassKey(c.codec, c.pixFmt) == class {
			return c, true
		}
	}
	return hwDecodeProbe{}, false
}

// probeHWDecodeCodecs returns the source classes the GPU HW-DECODES via hwType
// (see hwDecodeProbes). Encode capability does NOT imply decode capability: a
// pre-Skylake Intel iGPU encodes H.264 but cannot decode HEVC, so feeding such
// a source to `-hwaccel <type>` fails and the player stalls. Probing decode
// separately lets the transcode pipeline software-decode what the GPU can't,
// and the admin see an honest encode/decode support matrix.
func probeHWDecodeCodecs(ctx context.Context, ffmpegPath, hwType, device string) []string {
	var out []string
	for _, c := range hwDecodeProbes {
		if probeHWDecode(ctx, ffmpegPath, c, hwType, device) {
			out = append(out, transcoding.DecodeClassKey(c.codec, c.pixFmt))
		}
	}
	return out
}

// probeHWDecode software-encodes a tiny real clip of the class, then HW-decodes
// it through hwType and pulls the frames BACK from the device (hwdownload).
// The download is what makes the probe fail-closed: ffmpeg's `-hwaccel` is
// advisory - when the decoder cannot hand the stream to the GPU it logs
// "Failed setup for format" and quietly decodes in software, exit 0 - so a bare
// `-f null` run proved nothing (measured 2026-09-12: every host reported every
// class). hwdownload only accepts a frame that lives on the device, and the
// probe additionally requires ffmpeg to report the frames it decoded. Any
// error/timeout ⇒ false (worst case: software decode, a perf cost, never a stall).
func probeHWDecode(ctx context.Context, ffmpegPath string, c hwDecodeProbe, hwType, device string) bool {
	swEncoder, format, pixFmt, encoderArgs, download := c.swEncoder, c.format, c.pixFmt, c.encoderArgs, c.download
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

	// 1. software-encode a tiny real sample of the class. A build whose encoder
	// cannot make the class (no 12-bit x265) simply leaves it unprobed: unlisted,
	// so routed to the CPU - the fail-closed answer.
	sampleArgs := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=" + hwProbeMire + ":rate=5:duration=0.4",
		"-pix_fmt", pixFmt, "-c:v", swEncoder}
	sampleArgs = append(sampleArgs, encoderArgs...)
	sampleArgs = append(sampleArgs, "-f", format, tmpPath)
	if procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.decode_sample"}, ffmpegPath, sampleArgs...) != nil {
		return false
	}

	// 2. HW-decode it, download the frames, count them.
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-nostats", "-progress", "pipe:1"}
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
	// hwdownload refuses a frame that never reached the device; the format
	// behind it is the layout the device pool holds for this class.
	args = append(args, "-i", tmpPath, "-vf", "hwdownload,format="+download, "-f", "null", "-")
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_decode"}, ffmpegPath, args...)
	if err != nil {
		return false
	}
	return progressFrames(string(out)) > 0
}

// progressFrames reads the last `frame=N` of an ffmpeg -progress stream; 0 when
// none - a run that decoded nothing proved nothing.
func progressFrames(progress string) int {
	last := 0
	for line := range strings.SplitSeq(progress, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "frame="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				last = n
			}
		}
	}
	return last
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
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=10:duration=0.2")
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
		vf, enc = "format=p010le,hwupload,scale_vaapi=256:-2:format=p010", "hevc_vaapi"
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw")
		vf, enc = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=w=256:h=-1:format=p010", "hevc_qsv"
	case "nvenc":
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		vf, enc = "format=p010le,hwupload_cuda,scale_cuda=256:-2:format=p010", "hevc_nvenc"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3")
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
		vf = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=w=64:h=-1:format=nv12"
	case "nvenc":
		args = append(args, "-init_hw_device", "cuda=cu", "-filter_hw_device", "cu")
		vf = "format=p010le,hwupload_cuda,scale_cuda=64:-2:format=nv12"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3")
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
// The trailing format=vaapi pins the reverse map's output: without it the
// next filter can negotiate 'opencl' and the VAAPI pool refuses it (ENOSYS).
const hwOpenCLTonemapChain = "hwmap=derive_device=opencl:mode=read,tonemap_opencl=format=nv12:primaries=bt709:transfer=bt709:matrix=bt709:tonemap=bt2390:peak=100:desat=0,hwmap=derive_device=vaapi:reverse=1:extra_hw_frames=16,format=vaapi"

// probeHWTonemapOpenCL: VAAPI only; the synthetic frame is tagged PQ because
// tonemap_opencl refuses an SDR input. Failure output is logged: a missing ICD
// or a bad option must be visible in the boot log, not a silent "no".
// probeHWTonemapOpenCLE returns whether the probe succeeded plus the argv it
// ran and the stderr tail, so the calibration readiness retry can classify and
// log it (a cold VAAPI->OpenCL derive is an init transient, not a hard "no").
func probeHWTonemapOpenCLE(ctx context.Context, ffmpegPath, hwType, device string) (bool, string, string, error) {
	if hwType != "vaapi" || device == "" {
		return false, "", "", nil // not applicable: QSV/NVENC have no VAAPI->OpenCL derive
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-vaapi_device", device,
		"-f", "lavfi", "-i", "testsrc2=size=" + hwProbeMire + ":rate=5:duration=0.3",
		"-vf", "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload," + hwOpenCLTonemapChain + ",hwdownload,format=nv12",
		"-f", "null", "-"}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap_opencl"}, ffmpegPath, args...)
	argv := strings.Join(args, " ")
	if err != nil {
		return false, argv, tailString(strings.TrimSpace(string(out)), 400), err
	}
	return true, argv, "", nil
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
		"-f", "lavfi", "-i", "testsrc2=size=" + hwProbeMire + ":rate=10:duration=0.5",
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
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3")
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
		"-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3",
		"-f", "lavfi", "-i", "color=c=white@0.5:size="+hwProbeMire+":rate=5:duration=0.3",
		"-filter_complex", fmt.Sprintf("[0:v]%s[m];[1:v]%s[s];[m][s]%s=w=256:h=144", main, sub, blend),
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
