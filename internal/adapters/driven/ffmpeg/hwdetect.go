package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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

// DetectDevices enumerates EVERY accelerator the host exposes: each DRM render
// node (renderD128, renderD129, ...) identified by its PCI vendor and kernel
// driver, probed for VAAPI and - on an Intel node - for QSV, and each NVIDIA
// GPU nvidia-smi lists, by CUDA index. One HWDevice per (backend, device): a
// box with an Intel iGPU and an AMD dGPU yields vaapi@renderD128, qsv@renderD128
// and vaapi@renderD129, each certified on its own by the calibration.
func (d *HWDetector) DetectDevices(ctx context.Context) ([]transcoding.HWDevice, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.hwdetect.DetectDevices")
	defer span.End()
	var devices []transcoding.HWDevice

	nodes := enumerateRenderNodes()
	for _, n := range nodes {
		slog.Info("hwdetect: render node", slog.String("node", n.path), slog.String("vendor", n.vendor), slog.String("driver", n.driver))
	}
	devices = append(devices, detectVAAPI(ctx, d.ffmpegPath, nodes)...)
	devices = append(devices, detectNVENC(ctx, d.ffmpegPath)...)
	devices = append(devices, detectQSV(ctx, d.ffmpegPath, nodes)...)
	if dev := detectRKMPP(ctx, d.ffmpegPath); dev != nil {
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

// drmNode is one /dev/dri render node with what sysfs says about the GPU
// behind it: the PCI vendor id and the bound kernel driver.
type drmNode struct {
	path   string // /dev/dri/renderD128
	vendor string // 0x8086 Intel, 0x1002 AMD, 0x10de NVIDIA, "" unknown (WSL d3d12)
	driver string // i915, xe, amdgpu, radeon, nouveau, nvidia, "" unknown
}

// intel reports an Intel GPU node (the QSV candidates): the i915/xe driver, or
// the Intel vendor id when the driver cannot be read.
func (n drmNode) intel() bool {
	switch n.driver {
	case "i915", "xe":
		return true
	}
	return n.driver == "" && n.vendor == "0x8086"
}

// label is the human identification for the admin.
func (n drmNode) label() string {
	vendor := map[string]string{"0x8086": "Intel", "0x1002": "AMD", "0x10de": "NVIDIA"}[n.vendor]
	if vendor == "" {
		vendor = n.vendor
	}
	parts := []string{}
	if vendor != "" {
		parts = append(parts, vendor)
	}
	if n.driver != "" {
		parts = append(parts, n.driver)
	}
	if len(parts) == 0 {
		return filepath.Base(n.path)
	}
	return strings.Join(parts, " ") + " (" + filepath.Base(n.path) + ")"
}

// enumerateRenderNodes lists /dev/dri/renderD* in node order with their sysfs
// identity. Empty when /dev/dri is absent. sysfs fields are best-effort: a
// node whose vendor/driver cannot be read is still listed (and probed).
func enumerateRenderNodes() []drmNode {
	entries, err := os.ReadDir("/dev/dri")
	if err != nil {
		return nil
	}
	var nodes []drmNode
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "renderD") {
			continue
		}
		n := drmNode{path: filepath.Join("/dev/dri", e.Name())}
		if b, rerr := os.ReadFile("/sys/class/drm/" + e.Name() + "/device/vendor"); rerr == nil {
			n.vendor = strings.ToLower(strings.TrimSpace(string(b)))
		}
		if link, rerr := os.Readlink("/sys/class/drm/" + e.Name() + "/device/driver"); rerr == nil {
			n.driver = filepath.Base(link)
		}
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(a, b drmNode) int { return strings.Compare(a.path, b.path) })
	return nodes
}

// detectVAAPI probes every render node for VAAPI; a node with no working
// encoder is not a device (a WSL d3d12 node, an NVIDIA node without the VA
// bridge, a display-only card).
func detectVAAPI(ctx context.Context, ffmpegPath string, nodes []drmNode) []transcoding.HWDevice {
	var out []transcoding.HWDevice
	for _, n := range nodes {
		codecs := probeHWCodecs(ctx, ffmpegPath, "vaapi", n.path)
		if len(codecs) == 0 {
			continue
		}
		out = append(out, transcoding.HWDevice{Type: string(transcoding.VAAPI), Name: n.path, Label: n.label(), SupportedCodecs: codecs})
	}
	return out
}

// detectNVENC lists every NVIDIA GPU by CUDA index. The first GPU keeps the
// empty device name (ffmpeg's default, byte-identical to every argv this
// engine emitted on a single-GPU host); the others carry their index, which
// hwInitArgs/-gpu bind explicitly.
func detectNVENC(ctx context.Context, ffmpegPath string) []transcoding.HWDevice {
	smiPath, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.nvidia_smi"}, smiPath, "--query-gpu=index,name", "--format=csv,noheader")
	if err != nil {
		return nil
	}
	var devices []transcoding.HWDevice
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		idx, name, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		idx, name = strings.TrimSpace(idx), strings.TrimSpace(name)
		if name == "" {
			continue
		}
		device := idx
		if idx == "0" {
			device = ""
		}
		codecs := probeHWCodecs(ctx, ffmpegPath, "nvenc", device)
		if len(codecs) == 0 {
			continue
		}
		devices = append(devices, transcoding.HWDevice{Type: string(transcoding.NVENC), Name: device, Label: name, SupportedCodecs: codecs})
	}
	return devices
}

// detectRKMPP: a Rockchip SoC exposes the MPP service node (/dev/mpp_service,
// older kernels /dev/mpp-service); the real-encode probe of h264_rkmpp then
// decides, as everywhere. No device selection: MPP is one per SoC. The label
// carries the SoC from the device tree (rockchip,rk3588). Every RKMPP shape
// beyond the encode probe (drm_prime decode, RGA scale/overlay, OpenCL
// tone-map) is certified by the calibration's own probes and route proofs -
// NONE of it is assumed: this engine has not run on the hardware yet.
func detectRKMPP(ctx context.Context, ffmpegPath string) *transcoding.HWDevice {
	present := false
	for _, n := range []string{"/dev/mpp_service", "/dev/mpp-service"} {
		if _, err := os.Stat(n); err == nil {
			present = true
			break
		}
	}
	if !present {
		return nil
	}
	codecs := probeHWCodecs(ctx, ffmpegPath, "rkmpp", "")
	if len(codecs) == 0 {
		return nil
	}
	label := "Rockchip MPP"
	if b, err := os.ReadFile("/proc/device-tree/compatible"); err == nil {
		for c := range strings.SplitSeq(string(b), "\x00") {
			if soc, ok := strings.CutPrefix(c, "rockchip,"); ok {
				label = "Rockchip MPP " + strings.ToUpper(soc)
				break
			}
		}
	}
	return &transcoding.HWDevice{Type: string(transcoding.RKMPP), Label: label, SupportedCodecs: codecs}
}

// detectQSV probes QSV on every INTEL render node - the node's own driver,
// not card0's (a box whose card0 is the AMD dGPU still has QSV on renderD129).
func detectQSV(ctx context.Context, ffmpegPath string, nodes []drmNode) []transcoding.HWDevice {
	var out []transcoding.HWDevice
	for _, n := range nodes {
		if !n.intel() {
			continue
		}
		codecs := probeHWCodecs(ctx, ffmpegPath, "qsv", n.path)
		if len(codecs) == 0 {
			continue
		}
		out = append(out, transcoding.HWDevice{Type: string(transcoding.QSV), Name: n.path, Label: "Intel Quick Sync " + n.label(), SupportedCodecs: codecs})
	}
	return out
}

func probeHWCodecs(ctx context.Context, ffmpegPath, hwType, device string) []string {
	// AV1 is probed like the others: the encoder is compiled into every build
	// but only an Ada+ NVENC, an Arc / Meteor Lake+ QSV or an RDNA3+ / Arc
	// VAAPI actually encodes it - the real-encode probe decides, never a table.
	var encoderSuffixes []string
	switch hwType {
	case "vaapi":
		encoderSuffixes = []string{"h264_vaapi", "hevc_vaapi", "av1_vaapi"}
	case "nvenc":
		encoderSuffixes = []string{"h264_nvenc", "hevc_nvenc", "av1_nvenc"}
	case "qsv":
		encoderSuffixes = []string{"h264_qsv", "hevc_qsv", "av1_qsv"}
	case "rkmpp":
		encoderSuffixes = []string{"h264_rkmpp", "hevc_rkmpp"}
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
// holds (MPEG-2 TV/DVD rips, VC-1 early Blu-rays, VP8/VP9 web rips, AV1
// 8/10-bit), each through the same fail-closed hwdownload proof - the matrix
// is codec x pix_fmt, never a per-vendor support table. The web encoders run
// at their fastest speed: the sample only has to exist, not look good.
//
// A row either names the software encoder that makes its sample, or - when
// ffmpeg has no encoder for the codec - a FIXTURE: a real bitstream shipped
// with the binary (decodeFixtures) or dropped by the operator in
// EVEY_HW_DECODE_FIXTURES_DIR. VC-1 is such a row: no VC-1 encoder exists in
// ffmpeg (wmv3/vc1 are decode-only), and no synthetic, clearly redistributable
// VC-1 clip is bundled yet (see decodeFixtures), so until one is provided the
// class is simply never listed - a VC-1 source software-decodes. The probe
// itself is the same as every other class the moment the fixture exists.
//
// download is the software layout(s) the device pool hands back for the class
// (hwdownload output must be pinned: an open-ended list fails negotiation).
var hwDecodeProbes = []hwDecodeProbe{
	{"h264", "libx264", "h264", "yuv420p", nil, "nv12", ""},
	{"h264", "libx264", "h264", "yuv420p10le", []string{"-profile:v", "high10"}, "p010le", ""},
	{"h264", "libx264", "h264", "yuv422p10le", []string{"-profile:v", "high422"}, "y210le|p210le|yuv422p10le", ""},
	{"hevc", "libx265", "hevc", "yuv420p", nil, "nv12", ""},
	{"hevc", "libx265", "hevc", "yuv420p10le", nil, "p010le", ""},
	{"hevc", "libx265", "hevc", "yuv422p10le", []string{"-profile:v", "main422-10"}, "y210le|p210le|yuv422p10le", ""},
	{"hevc", "libx265", "hevc", "yuv444p", []string{"-profile:v", "main444-8"}, "yuv444p", ""},
	{"hevc", "libx265", "hevc", "yuv444p10le", []string{"-profile:v", "main444-10"}, "yuv444p16le", ""},
	{"hevc", "libx265", "hevc", "yuv420p12le", []string{"-profile:v", "main12"}, "p016le|p012le", ""},
	{"mpeg2video", "mpeg2video", "mpeg2video", "yuv420p", nil, "nv12", ""},
	{codec: "vc1", format: "vc1", pixFmt: "yuv420p", download: "nv12", fixture: "vc1-yuv420p.vc1"},
	{"vp8", "libvpx", "ivf", "yuv420p", []string{"-deadline", "realtime", "-cpu-used", "8"}, "nv12", ""},
	{"vp9", "libvpx-vp9", "ivf", "yuv420p", []string{"-deadline", "realtime", "-cpu-used", "8"}, "nv12", ""},
	{"vp9", "libvpx-vp9", "ivf", "yuv420p10le", []string{"-profile:v", "2", "-deadline", "realtime", "-cpu-used", "8"}, "p010le", ""},
	{"av1", "libsvtav1", "ivf", "yuv420p", []string{"-preset", "12"}, "nv12", ""},
	{"av1", "libsvtav1", "ivf", "yuv420p10le", []string{"-preset", "12"}, "p010le", ""},
}

type hwDecodeProbe struct {
	codec, swEncoder, format, pixFmt string
	encoderArgs                      []string
	download                         string
	// fixture names a real bitstream to decode instead of encoding a sample
	// (swEncoder empty): looked up in the embedded decodeFixtures, then in
	// EVEY_HW_DECODE_FIXTURES_DIR. Absent fixture = the class is not probed.
	fixture string
}

// decodeFixtures holds the bitstreams the probe matrix cannot synthesise. The
// directory ships empty but for its README: the one class that needs a
// fixture, VC-1, has no encoder in ffmpeg (only decoders), and the VC-1 clips
// that exist publicly (the SMPTE conformance suite, the FATE samples, rips of
// commercial discs) carry no licence that makes bundling them in a
// redistributable image clearly safe. A synthetic, freely-licensed VC-1
// elementary stream dropped at fixtures/decode/vc1-yuv420p.vc1 (raw Advanced
// Profile, 256x144 is enough) turns the row on with no code change; an
// operator can do the same at runtime with EVEY_HW_DECODE_FIXTURES_DIR.
//
//go:embed fixtures/decode/*
var decodeFixtures embed.FS

// decodeFixtureEnv is the operator's directory of extra decode fixtures.
const decodeFixtureEnv = "EVEY_HW_DECODE_FIXTURES_DIR"

// decodeFixtureBytes returns the fixture's bitstream: the embedded copy first,
// then the operator's directory. ok=false when neither has it.
func decodeFixtureBytes(name string) ([]byte, bool) {
	if b, err := decodeFixtures.ReadFile("fixtures/decode/" + name); err == nil && len(b) > 0 {
		return b, true
	}
	if dir := strings.TrimSpace(os.Getenv(decodeFixtureEnv)); dir != "" {
		// #nosec G703 -- dir is the operator's own directory (env), name a
		// matrix constant reduced to its base name: no request-controlled input.
		if b, err := os.ReadFile(filepath.Join(dir, filepath.Base(name))); err == nil && len(b) > 0 {
			return b, true
		}
	}
	return nil, false
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

	// 1. a tiny real sample of the class: software-encoded, or the shipped /
	// operator fixture for a codec ffmpeg cannot encode. A build whose encoder
	// cannot make the class (no 12-bit x265), or a fixture nobody provided,
	// simply leaves it unprobed: unlisted, so routed to the CPU - the
	// fail-closed answer.
	if c.fixture != "" {
		b, ok := decodeFixtureBytes(c.fixture)
		if !ok {
			slog.Debug("hwdetect: decode class has no fixture, not probed",
				slog.String("class", transcoding.DecodeClassKey(c.codec, c.pixFmt)), slog.String("fixture", c.fixture), slog.String("env", decodeFixtureEnv))
			return false
		}
		if werr := os.WriteFile(tmpPath, b, 0o600); werr != nil {
			return false
		}
	} else {
		sampleArgs := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=" + hwProbeMire + ":rate=5:duration=0.4",
			"-pix_fmt", pixFmt, "-c:v", swEncoder}
		sampleArgs = append(sampleArgs, encoderArgs...)
		sampleArgs = append(sampleArgs, "-f", format, tmpPath)
		if procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.decode_sample"}, ffmpegPath, sampleArgs...) != nil {
			return false
		}
	}

	// 2. HW-decode it, download the frames, count them.
	args, ok := hwDecodeProbeArgs(hwType, device, tmpPath, download)
	if !ok {
		return false
	}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_decode"}, ffmpegPath, args...)
	if err != nil {
		return false
	}
	return progressFrames(string(out)) > 0
}

// hwDecodeProbeArgs is the decode half of the probe: the backend's production
// hwaccel prologue on the named device, the sample, and the MANDATORY
// hwdownload - it refuses a frame that never reached the device, which is what
// turns ffmpeg's advisory -hwaccel into a proof. ok=false for a backend the
// probe cannot express (or VAAPI without a node).
func hwDecodeProbeArgs(hwType, device, sample, download string) ([]string, bool) {
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-nostats", "-progress", "pipe:1"}
	switch hwType {
	case "vaapi":
		if device == "" {
			return nil, false
		}
		args = append(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-hwaccel_device", device)
	case "qsv", "nvenc", "rkmpp":
		args = append(args, hwInitArgs(transcoding.HWAccelType(hwType), device)...)
	default:
		return nil, false
	}
	// The format behind hwdownload is the layout the device pool holds for
	// this class (pinned: an open-ended list fails negotiation).
	args = append(args, "-i", sample, "-vf", "hwdownload,format="+download, "-f", "null", "-")
	return args, true
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
		args = append(args, qsvInitDeviceArgs(device)...)
		filter = []string{"-vf", "hwupload=extra_hw_frames=16,format=qsv"}
	case "nvenc":
		encoderArgs = append(nvencGPUArgs(device), encoderArgs...)
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
		args = append(args, qsvInitDeviceArgs(device)...)
		vf, enc = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=w=256:h=-1:format=p010", "hevc_qsv"
	case "nvenc":
		args = append(args, cudaInitDeviceArgs(device)...)
		vf, enc = "format=p010le,hwupload_cuda,scale_cuda=256:-2:format=p010", "hevc_nvenc"
	case "rkmpp":
		args = append(args, rkmppInitDeviceArgs()...)
		vf, enc = "format=p010le,hwupload,scale_rkrga=w=256:h=-2:format=p010", "hevc_rkmpp"
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
		args = append(args, qsvInitDeviceArgs(device)...)
		vf = "format=p010le,hwupload=extra_hw_frames=16,scale_qsv=w=64:h=-1:format=nv12"
	case "nvenc":
		args = append(args, cudaInitDeviceArgs(device)...)
		vf = "format=p010le,hwupload_cuda,scale_cuda=64:-2:format=nv12"
	case "rkmpp":
		args = append(args, rkmppInitDeviceArgs()...)
		vf = "format=p010le,hwupload,scale_rkrga=w=64:h=-2:format=nv12"
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
// Gen9+ Intel (tonemap_vaapi / vpp_qsv) and an NVIDIA GPU on jellyfin-ffmpeg
// (tonemap_cuda) pass; old-iGPU-class VEBOX and a stock ffmpeg fail.
// Fail-closed: any error/abort/timeout ⇒ false ⇒ HDR tone-maps in software.
//
// hwCUDATonemapChain is the NVENC GPU tone-map: tonemap_cuda on the decoder's
// CUDA frames, the same bt2390 / peak=100 / desat=0 curve as the OpenCL and
// software paths, output nv12 for the 8-bit encoder. apply_dovi defaults on,
// so a Dolby Vision RPU is applied rather than ignored. Correctness measured
// against the production tonemapx chain on real HDR10 titles (RTX 4070 Ti,
// 2026-09-13): PSNR 41-47 dB, SSIM >= 0.993 across three scenes - the same
// picture, unlike the QSV VPP curve (washed highlights) that stays ineligible.
//
// hwOpenCLTonemapChain maps the VAAPI surface into OpenCL, tone-maps with the
// same bt2390/peak=100 curve as the software path, and maps back for the encoder.
// The trailing format=vaapi pins the reverse map's output: without it the
// next filter can negotiate 'opencl' and the VAAPI pool refuses it (ENOSYS).
const hwCUDATonemapChain = "tonemap_cuda=format=nv12:p=bt709:t=bt709:m=bt709:tonemap=bt2390:peak=100:desat=0"

// hwRKMPPOpenCLTonemapChain is the same OpenCL tone-map mapped from and back
// to Rockchip drm_prime frames (RK3588's Mali-G610 ICD); the trailing
// format=drm_prime pins the reverse map like format=vaapi does.
const hwRKMPPOpenCLTonemapChain = "hwmap=derive_device=opencl:mode=read,tonemap_opencl=format=nv12:primaries=bt709:transfer=bt709:matrix=bt709:tonemap=bt2390:peak=100:desat=0,hwmap=derive_device=rkmpp:reverse=1:extra_hw_frames=16,format=drm_prime"

// hwVulkanTonemapChain is the VAAPI -> Vulkan/libplacebo -> VAAPI tone-map:
// the decoded VAAPI surface maps into a Vulkan device DERIVED from the same
// render node (DRM PRIME export, no copy), libplacebo applies the static
// bt.2390 EETF to bt709 SDR (peak_detect off: the OpenCL/CUDA/software chains
// use a fixed curve, dynamic peak detection would draw a different picture
// per scene), the frame maps back into the VAAPI pool for scale_vaapi and the
// encoder. Dolby Vision RPU and HLG are read from the frame by libplacebo.
// Pinned formats on both ends for the same reason as the OpenCL chain.
const hwVulkanTonemapChain = "hwmap=derive_device=vulkan:mode=read,libplacebo=tonemapping=bt.2390:peak_detect=0:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:format=nv12,hwmap=derive_device=vaapi:reverse=1:extra_hw_frames=16,format=vaapi"

const hwOpenCLTonemapChain = "hwmap=derive_device=opencl:mode=read,tonemap_opencl=format=nv12:primaries=bt709:transfer=bt709:matrix=bt709:tonemap=bt2390:peak=100:desat=0,hwmap=derive_device=vaapi:reverse=1:extra_hw_frames=16,format=vaapi"

// probeHWTonemapOpenCL: VAAPI only; the synthetic frame is tagged PQ because
// tonemap_opencl refuses an SDR input. Failure output is logged: a missing ICD
// or a bad option must be visible in the boot log, not a silent "no".
// probeHWTonemapOpenCLE returns whether the probe succeeded plus the argv it
// ran and the stderr tail, so the calibration readiness retry can classify and
// log it (a cold VAAPI->OpenCL derive is an init transient, not a hard "no").
func probeHWTonemapOpenCLE(ctx context.Context, ffmpegPath, hwType, device string) (bool, string, string, error) {
	var init []string
	var chain string
	switch {
	case hwType == "vaapi" && device != "":
		init, chain = []string{"-vaapi_device", device}, hwOpenCLTonemapChain
	case hwType == "rkmpp":
		init, chain = rkmppInitDeviceArgs(), hwRKMPPOpenCLTonemapChain
	default:
		return false, "", "", nil // not applicable: QSV/NVENC have no ->OpenCL derive route here
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := append([]string{"-hide_banner", "-v", "error", "-nostdin"}, init...)
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3",
		"-vf", "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload,"+chain+",hwdownload,format=nv12",
		"-f", "null", "-")
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap_opencl"}, ffmpegPath, args...)
	argv := strings.Join(args, " ")
	if err != nil {
		return false, argv, tailString(strings.TrimSpace(string(out)), 400), err
	}
	return true, argv, "", nil
}

// probeHWTonemapVulkanE runs the Vulkan/libplacebo tone-map interop on the
// named VAAPI render node - the same chain the builder emits, bracketed by an
// upload of a PQ-tagged p010 mire and a download of the result: the VAAPI
// device binds the node, the Vulkan device is DERIVED from it inside the graph
// (not a second device picked by index: a box with two GPUs must map on the
// one that decoded), libplacebo runs, the reverse map lands in the VAAPI pool.
// Any failure - no Vulkan loader, no ICD for this GPU, an interop the driver
// refuses, a libplacebo the build lacks - is "no". Returns the argv and the
// stderr tail so the readiness retry can classify a cold-init transient.
func probeHWTonemapVulkanE(ctx context.Context, ffmpegPath, hwType, device string) (bool, string, string, error) {
	if hwType != "vaapi" || device == "" {
		return false, "", "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-vaapi_device", device,
		"-f", "lavfi", "-i", "testsrc2=size=" + hwProbeMire + ":rate=5:duration=0.3",
		"-vf", "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload," + hwVulkanTonemapChain + ",hwdownload,format=nv12",
		"-f", "null", "-"}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap_vulkan"}, ffmpegPath, args...)
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
		args = append(args, qsvInitDeviceArgs(device)...)
		vf = "format=p010le,hwupload=extra_hw_frames=16,vpp_qsv=tonemap=1:format=nv12,hwdownload,format=nv12"
	case "nvenc":
		// tonemap_cuda (jellyfin-ffmpeg) on the CUDA frames themselves - the
		// chain hwTonemapFilter emits. Tagged PQ like the OpenCL probe: an SDR-
		// tagged input is refused. A stock ffmpeg without the filter fails here
		// and NVENC keeps the CPU bridge.
		args = append(args, cudaInitDeviceArgs(device)...)
		vf = "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload_cuda," + hwCUDATonemapChain + ",hwdownload,format=nv12"
	default:
		return false
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3")
	args = append(args, "-vf", vf, "-f", "null", "-")

	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_tonemap"}, ffmpegPath, args...) == nil
}

// interlacedSampleArgs renders a genuinely INTERLACED H.264 mire: testsrc2 at
// twice the rate, tinterlace weaves two frames into one TFF frame (real
// combing on the moving parts), setfield stamps it, and x264 codes it as
// fields so the decoder flags every frame interlaced - the same signal a
// PAL/NTSC DVD rip carries. Shared by the deinterlace probe and the route mire.
func interlacedSampleArgs(size string, seconds float64) []string {
	return []string{
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%s:rate=50:duration=%g,tinterlace=interleave_top,setfield=tff", size, seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-flags", "+ilme+ildct", "-x264opts", "tff=1", "-an",
	}
}

// probeHWDeinterlace reports whether the backend's SESSION-level GPU
// deinterlacer is CORRECT, not merely present: it decodes an interlaced mire
// on the GPU, runs the very filter the builder emits (hwQSVDeinterlaceFilter),
// pulls the frames back and asks idet. Correct means the output is
// progressive (no comb left) at the SOURCE frame rate (one frame per field
// pair - a bob that doubled the rate would desync the HLS cadence). A control
// pass without the deinterlacer must idet as interlaced, otherwise the mire
// proves nothing and the answer is "no". QSV only: NVENC/VAAPI deinterlace per
// frame off the decoder flag (see hwDeinterlaceFilter). Fail-closed.
func probeHWDeinterlace(ctx context.Context, ffmpegPath, hwType, device string) bool {
	if hwType != "qsv" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	tmp, err := os.CreateTemp("", "evey-deintprobe-*.h264")
	if err != nil {
		return false
	}
	tmpPath := tmp.Name()
	defer func() {
		if rerr := os.Remove(tmpPath); rerr != nil {
			slog.Debug("hwdetect: deinterlace-probe temp cleanup failed", slog.Any("err", rerr))
		}
	}()
	if cerr := tmp.Close(); cerr != nil {
		return false
	}
	sample := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y"}, interlacedSampleArgs(hwProbeMire, 1)...)
	sample = append(sample, "-f", "h264", tmpPath)
	if procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.deint_sample"}, ffmpegPath, sample...) != nil {
		return false
	}
	// Control: the mire itself idets interlaced (software decode).
	control := []string{"-hide_banner", "-nostdin", "-nostats", "-i", tmpPath, "-vf", "idet", "-f", "null", "-"}
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.deint_control"}, ffmpegPath, control...)
	if err != nil {
		return false
	}
	if v, ok := parseIdetCounts(string(out)); !ok || !v.interlaced() {
		slog.Warn("hwdetect: interlaced mire does not idet as interlaced - deinterlace probe inconclusive, treated as unsupported")
		return false
	}
	args := []string{"-hide_banner", "-nostdin", "-nostats", "-progress", "pipe:1",
		"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
	if device != "" {
		args = append(args, "-qsv_device", device)
	}
	args = append(args, "-i", tmpPath, "-vf", hwQSVDeinterlaceFilter+",hwdownload,format=nv12,idet", "-f", "null", "-")
	out, err = procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_deinterlace"}, ffmpegPath, args...)
	if err != nil {
		return false
	}
	v, ok := parseIdetCounts(string(out))
	// 25 frames in at 25 fps (50 fields/s woven); a bob would return ~50.
	if !ok || !v.progressive() || progressFrames(string(out)) == 0 || progressFrames(string(out)) > 30 {
		slog.Warn("hwdetect: GPU deinterlacer output is not progressive at the source rate - treated as unsupported",
			slog.String("hw", hwType), slog.Int("tff", v.tff), slog.Int("bff", v.bff), slog.Int("progressive", v.prog),
			slog.Int("undetermined", v.undet), slog.Int("frames", progressFrames(string(out))))
		return false
	}
	return true
}

// idetCounts is idet's multi-frame verdict ("Multi frame detection: TFF: a
// BFF: b Progressive: c Undetermined: d") with the counts kept - the prober's
// parseIdetVerdict folds them to a bool, the deinterlace probe needs the share.
type idetCounts struct{ tff, bff, prog, undet int }

// interlaced: more frames judged interlaced than progressive.
func (v idetCounts) interlaced() bool { return v.tff+v.bff > v.prog }

// progressive: at least 7 of 10 determined frames progressive. Measured on
// the woven testsrc2 mire (2026-09-13): the raw mire idets 25/25 TFF, bwdif
// and yadif_cuda leave 4-5 frames judged TFF out of 25 (idet's own
// uncertainty on residual motion) - a deinterlacer that did nothing stays
// near 100 % interlaced, so the two populations are far apart.
func (v idetCounts) progressive() bool {
	det := v.tff + v.bff + v.prog
	return det > 0 && v.prog*10 >= det*7
}

var idetCountsRE = regexp.MustCompile(`Multi frame detection:\s*TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)\s*Undetermined:\s*(\d+)`)

// parseIdetCounts reads the LAST multi-frame line of an idet stderr; ok=false
// when idet printed none (the graph never ran).
func parseIdetCounts(stderr string) (idetCounts, bool) {
	m := idetCountsRE.FindAllStringSubmatch(stderr, -1)
	if len(m) == 0 {
		return idetCounts{}, false
	}
	last := m[len(m)-1]
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	return idetCounts{tff: atoi(last[1]), bff: atoi(last[2]), prog: atoi(last[3]), undet: atoi(last[4])}, true
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
		args = append(args, qsvInitDeviceArgs(device)...)
		main = "format=nv12,hwupload=extra_hw_frames=16"
		sub = "format=bgra,hwupload=extra_hw_frames=16"
		blend = "overlay_qsv"
	case "nvenc":
		args = append(args, cudaInitDeviceArgs(device)...)
		// overlay_cuda takes the overlay as yuva420p, not bgra: it blends in the
		// planar YUV domain rather than through a VPP colour converter.
		main, sub, blend = "format=nv12,hwupload_cuda", "format=yuva420p,hwupload_cuda", "overlay_cuda"
	case "rkmpp":
		args = append(args, rkmppInitDeviceArgs()...)
		main, sub, blend = "format=nv12,hwupload", "format=bgra,hwupload", "overlay_rkrga"
	default:
		return false
	}
	// overlay_cuda has no w/h options ("Option not found", measured on an RTX
	// 4070 Ti 2026-09-13: the probe failed on every NVIDIA host and GPU burn-in
	// never certified there); the explicit geometry is the Intel VPP's need.
	if hwType == "vaapi" || hwType == "qsv" {
		blend += "=w=256:h=144"
	}
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=5:duration=0.3",
		"-f", "lavfi", "-i", "color=c=white@0.5:size="+hwProbeMire+":rate=5:duration=0.3",
		"-filter_complex", fmt.Sprintf("[0:v]%s[m];[1:v]%s[s];[m][s]%s", main, sub, blend),
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
	case "qsv", "rkmpp":
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
	case string(transcoding.RKMPP):
		return fmt.Sprintf("%s_rkmpp", codec)
	}
	return ""
}
