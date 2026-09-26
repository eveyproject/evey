package transcoding

import (
	"context"
	"slices"
	"strings"
	"time"
)

// EncodeCapability is the calibrated upper bound on what the host can encode
// in realtime (software, and hardware when a GPU is available), computed by the
// BenchmarkRunner and persisted so boots do not re-bench. Ceilings default to
// a safe 720p until a benchmark ran.
type EncodeCapability struct {
	// SoftwareMaxHeight is the tallest H.264 software encode the host sustains
	// at >= 1.2× realtime. One of {720, 1080, 1440, 2160}.
	SoftwareMaxHeight int `json:"software_max_height"`

	// SoftwareHDRMaxHeight is the same for HDR->SDR content (10-bit decode +
	// tonemap, the costs the SDR bench cannot see). 0 until benched.
	SoftwareHDRMaxHeight int `json:"software_hdr_max_height"`

	// SoftwareAV1MaxHeight is the libsvtav1 ceiling. 0 is meaningful: AV1
	// software is not viable, so AV1 output stays hardware-only.
	SoftwareAV1MaxHeight int `json:"software_av1_max_height"`

	// HWMaxHeight is the hardware encode ceiling; 0 when no HW passed the bench.
	HWMaxHeight int `json:"hw_max_height"`

	// HWBridgeMaxHeight is the ceiling of the CPU tone-map bridged between a GPU
	// decode and a GPU encode (LadderHWBridge). 0 = not measured: the HDR ladder
	// then caps at SoftwareHDRMaxHeight, the conservative bound.
	HWBridgeMaxHeight int `json:"hw_bridge_max_height,omitempty"`

	// HWBackend names the accelerator behind HWMaxHeight (nvenc/qsv/vaapi, "").
	HWBackend string `json:"hw_backend,omitempty"`

	// HWCodecs lists the domain codecs the backend encodes ("h264", "hevc",
	// "av1"): an old QSV box encodes H.264 only and software-encodes HEVC.
	HWCodecs []string `json:"hw_codecs,omitempty"`

	// HWDecodeCodecs lists what the backend DECODES, per source class the
	// probe exercised (DecodeClassKey): "h264", "hevc", "mpeg2video", "vc1",
	// "vp8", "vp9", "av1" (8-bit 4:2:0), "hevc10" (HEVC Main10, the 2160p HDR class)
	// and "<codec>:<pix_fmt>" for every other chroma/depth the probe matrix
	// covers (10-bit H.264/VP9/AV1, 4:2:2, 4:4:4, 12-bit). Probed fail-closed:
	// a frame has to come back from the GPU (hwdownload) for a class to be
	// listed. Measured, not assumed: a Skylake iGPU decodes HEVC but not
	// Main10, an Ada NVDEC no 4:2:2, a Blackwell one yes, a Gen9 iGPU no AV1.
	// VC-1 is probed from a shipped/operator fixture (ffmpeg has no encoder to
	// synthesise one) and is listed only when that fixture decoded.
	HWDecodeCodecs []string `json:"hw_decode_codecs,omitempty"`

	// Backends is the per-accelerator capability of EVERY certified backend
	// (not just the primary): VAAPI and QSV on a box that has both, each with
	// its own device, codecs, decode classes, features and proven routes. The
	// resolver picks the backend PER ROUTE from this (the cheapest correct,
	// proven one) instead of one global "best backend" - so HDR->SDR can take
	// VAAPI's OpenCL tone-map while HEVC/SDR take a faster QSV, on the same host.
	// The scalar HWBackend/HWDevice/... below are the PRIMARY (the cheapest SDR
	// backend), kept for backward compatibility and non-route reads; a row from
	// before this field is read as a single-backend host (Backends synthesised
	// from the scalars by PrimaryAsBackend at read time).
	Backends []HWBackendCap `json:"backends,omitempty"`

	// RouteSamples is the per-route single-session cost evidence (see
	// RouteSample): what a warm, route-faithful transcode of each proven route
	// actually cost here. The admission cost model prices a route on this
	// instead of the cumulative HW rung. Empty on a row benched before it
	// existed - CapabilityProbeVersion re-benches such a row at boot.
	RouteSamples []RouteSample `json:"route_samples,omitempty"`

	// HWRoutes lists the ENCODE ROUTES the calibration ran end-to-end through
	// the production builder (BuildFFmpegArgs on a mire of the route's class,
	// real HLS output on disk): HWRouteSDR, HWRouteHEVC, HWRouteAV1,
	// HWRouteSource10Bit, HWRouteTonemap, HWRouteBridge, HWRoutePassthrough,
	// HWRouteDecoupled, HWRouteDeinterlace.
	// The resolver only takes a route named here. nil (a row benched before
	// the route probes existed) is trusted as before - CapabilityProbeVersion
	// re-benches such a row at boot, so nil never survives a deploy.
	HWRoutes []string `json:"hw_routes,omitempty"`

	// HW10BitTranscode: the VPP folds a 10-bit source (p010 -> nv12). The Intel
	// i965 driver aborts the moment a p010 surface reaches scale_vaapi, so false
	// routes 10-bit sources to software. No omitempty: the admin view must show
	// the gate is active.
	HW10BitTranscode bool `json:"hw_10bit_transcode"`

	// HWTonemap: the backend tone-maps HDR->SDR on the GPU (tonemap_vaapi /
	// tonemap_opencl / tonemap_cuda). False -> software tonemap. No omitempty.
	HWTonemap bool `json:"hw_tonemap"`

	// HWOverlay: the backend composites a bitmap subtitle on the GPU; false
	// keeps a burn-in on the software path (a forced-subtitle rip used to send
	// a whole 4K HDR session to the CPU, prod 2026-09-09). No omitempty.
	HWOverlay bool `json:"hw_overlay"`

	// HWHDR10Encode: the backend encodes HEVC Main10 from a 10-bit surface, HDR metadata kept.
	HWHDR10Encode bool `json:"hw_hdr10_encode"`

	// ProbeVersion is the CapabilityProbeVersion the HW probes ran under; a
	// persisted capability below it is re-benched at boot, and served interim
	// through Snapshot until then. The calibration lifecycle is its ONLY
	// reader: no decision path branches on it.
	ProbeVersion int `json:"probe_version,omitempty"`

	// GPUClass labels the accelerator for the admin panel: "integrated",
	// "discrete", or "" when it cannot be told (AMD VAAPI, a WSL render node).
	GPUClass string `json:"gpu_class,omitempty"`

	// HWDevice is the DRM render node for VAAPI/QSV, empty for NVENC.
	HWDevice string `json:"hw_device,omitempty"`

	// HWFeatures lists the encoder tuning options the probe confirmed the
	// backend accepts; the arg builder emits a flag only when named here.
	// Demoted to empty on a runtime unsupported-option failure.
	HWFeatures []string `json:"hw_features,omitempty"`

	// DetectedBackends lists every accelerator the host advertises, recorded
	// before the mode's pin: informational, for the admin picker.
	DetectedBackends []string `json:"detected_backends,omitempty"`

	// DetectedDevices lists every (backend, device) the host advertises - each
	// DRM render node and each NVIDIA GPU identified - before any pin, so an
	// operator sees the second render node a multi-GPU box exposes and can pin
	// it (HWPreference.Device). Informational; the certified set is Backends.
	DetectedDevices []HWDeviceInfo `json:"detected_devices,omitempty"`

	// CPUSignature identifies the host CPU; a changed signature (container
	// restored elsewhere) invalidates the calibration at boot.
	CPUSignature string `json:"cpu_signature"`

	BenchmarkedAt       time.Time `json:"benchmarked_at"`
	BenchmarkDurationMs int64     `json:"benchmark_duration_ms"`

	// BenchSamples is every rung the benchmark probed (fps vs threshold), so
	// the operator sees WHY a ceiling landed where it did. Empty on rows
	// persisted before it existed.
	BenchSamples []BenchRungSample `json:"bench_samples,omitempty"`

	// HWDemotedAt is when a runtime failure last disabled the HW backend, so
	// the demotion expires instead of outliving its cause.
	HWDemotedAt time.Time `json:"hw_demoted_at,omitzero"`

	// DecodeSamples is DECODE-only throughput per source codec (a 4K HEVC and a
	// 1080p H.264 source measured 2.12x vs 8.43x at the same output rung).
	DecodeSamples []DecodeRungSample `json:"decode_samples,omitempty"`

	// BenchSource is the codec+height BenchSamples were decoded from.
	BenchSource SourceClass `json:"bench_source,omitzero"`

	// SourceRate is the fps the samples were measured against, so Derive can
	// re-derive pass/fail under a different margin. 0 on old rows: the stored
	// Passing flag is trusted.
	SourceRate int `json:"source_rate,omitempty"`

	// PolicyVersion is the Derive policy that produced the ceilings; an older
	// one is re-derived from the proof at boot without re-benching.
	PolicyVersion int `json:"policy_version,omitempty"`

	// BenchContended records that the host was already busy when the benchmark
	// ran (a first boot during a library import benched 720p@5fps): the ceilings
	// are provisional and the service re-benches once idle.
	BenchContended bool `json:"bench_contended,omitempty"`

	// BenchHostBusyPercent is the host CPU busy share sampled before the bench.
	BenchHostBusyPercent int `json:"bench_host_busy_percent,omitempty"`
}

// BenchRungSample is one resolution rung the benchmark measured.
type BenchRungSample struct {
	Ladder          string  `json:"ladder"` // "software" | "software-hdr" | "hw" | "hw-bridge"
	Height          int     `json:"height"`
	FPS             float64 `json:"fps"`          // measured encode fps for this rung
	RequiredFPS     float64 `json:"required_fps"` // sourceRate × realtime margin it had to beat
	Passing         bool    `json:"passing"`      // FPS >= RequiredFPS
	ParallelEncodes int     `json:"parallel_encodes"`
}

// SourceClass is a decode workload: codec + pixel height. The zero value means
// "unknown source", which callers price the old way rather than as free.
type SourceClass struct {
	Codec  string `json:"codec"`
	Height int    `json:"height"`
}

// Known reports whether c carries enough information to price a decode.
func (c SourceClass) Known() bool {
	return c.Codec != "" && c.Height > 0
}

// Pixels is the frame area decode throughput scales against (height alone).
func (c SourceClass) Pixels() int {
	if c.Height <= 0 {
		return 0
	}
	return c.Height * c.Height
}

// DecodeRungSample is measured DECODE-only throughput for one codec at one height.
type DecodeRungSample struct {
	Codec  string  `json:"codec"`
	Height int     `json:"height"`
	FPS    float64 `json:"fps"`
}

// CapabilityProbeVersion bumps whenever a new HW probe is added (1: HDR10
// encode + Intel low-power features; 2: OpenCL tone-map + per-codec B-frames;
// 3: the OpenCL chain fixed, 2026-09-10; 4: the tone-map bridge ladder;
// 5: fail-closed decode probes incl. HEVC Main10 + the builder route probes;
// 6: the decode probe matrix over chroma/depth; 7: per-route single-session
// cost evidence, source-fps required rate, QSV scaler fix; 8: per-route
// backend evidence for EVERY certified accelerator (Backends[] of VAAPI AND
// QSV) - the code landed at v7 but a host already benched at v7 kept its
// single-primary calibration, so the bump forces the multi-backend re-bench;
// and the independent VAAPI OpenCL tone-map probe; 9: the decode matrix
// extended to MPEG-2, VP8, VP9 8/10-bit and AV1 8/10-bit - a v8 row lists
// none of them and would software-decode every such source; 10: the
// VAAPI->Vulkan/libplacebo tone-map probe (FeatureTonemapVulkan) and the
// tone-map correctness gate - a v9 row never ran either, so an AMD/Mesa host
// would keep bridging on the CPU with a working Vulkan interop; 11: the hw
// ladder benched in the runtime's pipeline shape (single-variant: one rung per
// probe, not the cumulative parallel ladder that under-certified a small GPU),
// each backend's OWN ceiling recorded (HWBackendCap.MaxHeight) and route
// evidence measured at the route's ceiling rung too - a v10 row certified a
// 720p-only backend with NO route sample, which admission then refused);
// 12: route evidence derived from the runtime's own shape matrix
// (RuntimeRouteShapes): the tone-map, bridge and decoupled routes measured
// for EVERY codec the backend encodes and x265 measured on the tone-map
// route - a v11 row priced them for H.264 only, so admission refused the
// HEVC variant the resolver upgrades to and spilled it to software.
const CapabilityProbeVersion = 12

// Route names the calibration proves through the production builder (see
// EncodeCapability.HWRoutes). Each is one shape BuildFFmpegArgs can emit for a
// GPU session; a shape that is not proven is not routed to the GPU.
const (
	HWRouteSDR         = "sdr"         // zero-copy decode -> scale -> h264
	HWRouteHEVC        = "hevc"        // same, hevc encoder
	HWRouteAV1         = "av1"         // same, av1 encoder (Ada+ / Arc / RDNA3+)
	HWRouteSource10Bit = "src10"       // 10-bit SDR source folded to nv12 on the GPU
	HWRouteTonemap     = "tonemap"     // GPU HDR->SDR tone-map (OpenCL / VPP)
	HWRouteBridge      = "bridge"      // GPU decode, CPU tone-map, GPU encode
	HWRoutePassthrough = "passthrough" // HEVC Main10 with the HDR kept
	HWRouteDecoupled   = "decoupled"   // CPU decode, hwupload, GPU scale+encode
	HWRouteDeinterlace = "deint"       // QSV: the session-level GPU deinterlacer on an interlaced source
)

// HWRouteSourceFormat names the route of a source class outside 4:2:0 8/10-bit
// (4:4:4, 12-bit...): decoded on the GPU (its decode class listed) AND run
// through the builder's zero-copy chain, whose scaler must fold that layout.
// Only such a route proven end-to-end takes the source to the GPU.
func HWRouteSourceFormat(pixFmt string) string {
	return "src:" + strings.ToLower(strings.TrimSpace(pixFmt))
}

// CanonicalDecodeFormat reports whether pixFmt is one of the 4:2:0 8/10-bit
// layouts the SDR / 10-bit routes already cover ("" = unknown, treated as such).
func CanonicalDecodeFormat(pixFmt string) bool {
	switch strings.ToLower(strings.TrimSpace(pixFmt)) {
	case "", "yuv420p", "yuvj420p", "nv12", "yuv420p10le", "p010le":
		return true
	}
	return false
}

// HWDecodeHEVC10 is the HWDecodeCodecs entry for HEVC Main10 sources.
const HWDecodeHEVC10 = "hevc10"

// DecodeClassKey names the decode class of a (codec, pix_fmt) source in
// HWDecodeCodecs: the three canonical 4:2:0 classes keep their short names,
// everything else is "<codec>:<pix_fmt>". Unknown pix_fmt ("") maps to the
// codec's 8-bit class.
func DecodeClassKey(codec, pixFmt string) string {
	c := normaliseSourceCodec(codec)
	switch f := strings.ToLower(strings.TrimSpace(pixFmt)); f {
	case "", "yuv420p", "yuvj420p", "nv12":
		return c
	case "yuv420p10le", "p010le":
		if c == "hevc" {
			return HWDecodeHEVC10
		}
		return c + ":yuv420p10le"
	default:
		return c + ":" + f
	}
}

// FeatureTonemapOpenCL: tonemap_opencl runs on this GPU (bt2390 curve, HLG and
// Dolby Vision RPU handled) - the VPP tone-mapper is the fallback.
const FeatureTonemapOpenCL = "tonemap_opencl"

// FeatureTonemapVulkan: libplacebo on a Vulkan device DERIVED from the VAAPI
// render node runs on this GPU - the VAAPI surface maps into Vulkan, libplacebo
// tone-maps (bt.2390, HLG and Dolby Vision RPU handled), the frame maps back to
// VAAPI for the scaler/encoder. The Mesa/RADV path of an AMD box, where no
// OpenCL ICD is usually available. Probed by running that exact interop on the
// node, never inferred from vendor, kernel or the filter's presence.
const FeatureTonemapVulkan = "tonemap_vulkan"

// FeatureTonemapCUDA: tonemap_cuda (the jellyfin-ffmpeg CUDA tone-mapper, same
// bt2390 curve, HLG and Dolby Vision RPU applied) runs on this NVIDIA GPU
// directly on the decoder's CUDA frames. NVENC has no other GPU tone-mapper:
// without it HDR->SDR takes the CPU bridge.
const FeatureTonemapCUDA = "tonemap_cuda"

// BFramesFeature names the probed B-frame support of a codec; "_lp" suffixed
// for the low-power encoder, whose support differs (VDEnc has none on Gen9).
func BFramesFeature(codec string, lowPower bool) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	if lowPower {
		return "bframes_" + codec + "_lp"
	}
	return "bframes_" + codec
}

// SafeFallback is the capability used before any calibration ran: 720p
// software, the universal floor.
func SafeFallback() EncodeCapability {
	return EncodeCapability{
		SoftwareMaxHeight: 720,
		HWMaxHeight:       0,
		HWBackend:         "",
	}
}

// MaxHeightFor returns the effective cap: HWMaxHeight when the request takes
// the HW path end-to-end (hwAccelUsable), else the software ceiling.
func (c EncodeCapability) MaxHeightFor(hwAccelUsable bool) int {
	if hwAccelUsable && c.HWMaxHeight > 0 {
		return c.HWMaxHeight
	}
	if c.SoftwareMaxHeight > 0 {
		return c.SoftwareMaxHeight
	}
	return 720
}

// AV1SoftwareViable reports whether the host has a calibrated AV1 software ceiling.
func (c EncodeCapability) AV1SoftwareViable() bool {
	return c.SoftwareAV1MaxHeight > 0
}

// SoftwareCeilingFor returns the software height ceiling for an OUTPUT codec:
// AV1 has its own (far heavier), every other codec uses SoftwareMaxHeight.
func (c EncodeCapability) SoftwareCeilingFor(codec string) int {
	if strings.EqualFold(strings.TrimSpace(codec), "av1") && c.SoftwareAV1MaxHeight > 0 {
		return c.SoftwareAV1MaxHeight
	}
	if c.SoftwareMaxHeight > 0 {
		return c.SoftwareMaxHeight
	}
	return 720
}

// HWSupports reports whether the calibrated HW backend can encode the given
// domain codec. An empty codec is treated as h264, matching resolveVideoEncoder.
func (c EncodeCapability) HWSupports(codec string) bool {
	if c.HWBackend == "" {
		return false
	}
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	return slices.Contains(c.HWCodecs, codec)
}

// decodeClassProbeVersion is the CapabilityProbeVersion from which the decode
// probe distinguishes HEVC Main10 (HWDecodeHEVC10) from 8-bit HEVC. A row
// probed before it never had the class: Snapshot reads "hevc" as covering
// Main10, as it always did, ONCE at publish - the decode gate itself knows no
// version (a re-bench at boot replaces such a row anyway).
const decodeClassProbeVersion = 5

// PersistedEncodeCapability is EncodeCapability in its persistence role: the
// row the CapabilityRepo stores and loads, written by whatever probe
// generation benched the host (ProbeVersion) and carrying the calibration's
// lifecycle (signature, timestamps, policy version). The planner never reads
// one: it reads a HostCapabilitySnapshot.
type PersistedEncodeCapability = EncodeCapability

// HostCapabilitySnapshot is the capability as the planner reads it: one
// vocabulary whatever probe version wrote the row, produced ONLY by
// EncodeCapability.Snapshot (the type is the proof that the row went through
// it). The calibration service publishes one on every store - persisted,
// benched or demoted - and PlanResolver.Capability yields one. Nothing
// downstream branches on ProbeVersion.
type HostCapabilitySnapshot struct {
	EncodeCapability
}

// Snapshot is the row as the planner reads it (normalized).
func (c EncodeCapability) Snapshot() HostCapabilitySnapshot {
	return HostCapabilitySnapshot{EncodeCapability: c.normalized()}
}

// normalized upgrades a row to the planner's vocabulary. A pre-class row
// (ProbeVersion < 5) listed "hevc" for 8-bit AND Main10 sources: it gains
// HWDecodeHEVC10 so the matrix reading answers the same. The row's
// ProbeVersion is kept - the calibration's staleness check reads it.
func (c EncodeCapability) normalized() EncodeCapability {
	if c.ProbeVersion >= decodeClassProbeVersion {
		return c.withRouteVerdicts()
	}
	c.HWDecodeCodecs = withImpliedMain10(c.HWDecodeCodecs)
	if len(c.Backends) > 0 {
		backends := make([]HWBackendCap, len(c.Backends))
		copy(backends, c.Backends)
		for i := range backends {
			backends[i].DecodeCodecs = withImpliedMain10(backends[i].DecodeCodecs)
		}
		c.Backends = backends
	}
	return c
}

// withRouteVerdicts makes a route-probed row (ProbeVersion >= 5) say what it
// proved: a backend whose Routes is nil gets the empty list, the fail-closed
// verdict the probe wrote. Rows persisted while Routes was omitted when empty
// came back nil, and Proves read nil as "every route trusted" - a backend
// with no working route was offered every one of them.
func (c EncodeCapability) withRouteVerdicts() EncodeCapability {
	fix := false
	for _, b := range c.Backends {
		if b.Routes == nil {
			fix = true
			break
		}
	}
	if !fix {
		return c
	}
	backends := make([]HWBackendCap, len(c.Backends))
	copy(backends, c.Backends)
	for i := range backends {
		if backends[i].Routes == nil {
			backends[i].Routes = []string{}
		}
	}
	c.Backends = backends
	return c
}

// withImpliedMain10 adds the Main10 class to a pre-class decode list that
// names hevc (a copy: the persisted row is never edited in place).
func withImpliedMain10(codecs []string) []string {
	if !slices.Contains(codecs, "hevc") || slices.Contains(codecs, HWDecodeHEVC10) {
		return codecs
	}
	out := make([]string, 0, len(codecs)+1)
	out = append(out, codecs...)
	return append(out, HWDecodeHEVC10)
}

// decodeClassOf names the decode class a source asks of a decode list, from
// its codec, bit depth and pixel format: the (codec, pix_fmt) matrix class
// when the format is known, else the codec's 8-bit class, or HEVC Main10
// for a 10-bit source of unknown format (the only 10-bit class the depth
// flag alone can name). ok=false when no class can name the source: it is
// not decodable, fail-closed.
func decodeClassOf(codec string, tenBit bool, pixFmt string) (key string, ok bool) {
	if strings.TrimSpace(pixFmt) != "" {
		return DecodeClassKey(codec, pixFmt), true
	}
	c := normaliseSourceCodec(codec)
	if !tenBit {
		return c, true
	}
	if c != "hevc" {
		return "", false
	}
	return HWDecodeHEVC10, true
}

// decodable reports whether codecs (a decode list in the class vocabulary)
// names the class of this source.
func decodable(codecs []string, codec string, tenBit bool, pixFmt string) bool {
	key, ok := decodeClassOf(codec, tenBit, pixFmt)
	return ok && slices.Contains(codecs, key)
}

// HWCanDecodeSource reports whether the GPU decodes a source of this codec,
// bit depth and pixel format, from MEASUREMENT: the (codec, pix_fmt) class is
// decodable iff the probe brought a frame back for it, and a class the list
// does not name is not decodable (conservative, and extensible by adding a
// probe, never by assuming). The row is read in one vocabulary whatever probe
// version wrote it (Snapshot).
func (c EncodeCapability) HWCanDecodeSource(codec string, tenBit bool, pixFmt string) bool {
	return c.HWBackend != "" && decodable(c.HWDecodeCodecs, codec, tenBit, pixFmt)
}

// LowPowerFeature names the probed Intel low-power (VDEnc) encoder feature of a codec.
// RuntimeHWFeatures is the feature list an encode session really runs with:
// the low-power features are dropped unless the operator opted in. The ONE
// derivation for the runtime (effectiveHWFeatures) and for the calibration's
// route proofs and cost evidence, so what is measured is what is emitted -
// the calibration used to prove and price every route with the low-power
// encoder while the runtime, opted out by default, ran the full one.
func RuntimeHWFeatures(features []string, lowPower bool) []string {
	if lowPower {
		return features
	}
	out := make([]string, 0, len(features))
	for _, f := range features {
		if !strings.HasPrefix(f, "low_power_") {
			out = append(out, f)
		}
	}
	return out
}

func LowPowerFeature(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	return "low_power_" + codec
}

// CapabilityRepo persists the calibration row across restarts
// (server_settings, one JSON value). It speaks the persisted role only: a
// row it returns is read by the planner through Snapshot, never as is.
type CapabilityRepo interface {
	Get(ctx context.Context) (PersistedEncodeCapability, bool, error)
	Set(ctx context.Context, cap PersistedEncodeCapability) error
}

// BenchmarkRunner measures the host's realtime encode ceilings on a bundled
// sample. Implementations MUST be cancellable (a weak host benches ~60 s).
type BenchmarkRunner interface {
	// Run benches under pref: Backend pins the HW probe to one accelerator
	// (empty = auto), DisableHW skips hardware probing entirely.
	Run(ctx context.Context, pref HWPreference) (EncodeCapability, error)
	// HWBackendUsable confirms the named backend can encode on this host right
	// now, on the device the calibration bound (empty = the backend's default):
	// a matching CPU signature does not guarantee the GPU is still there.
	HWBackendUsable(ctx context.Context, backend, device string) bool
	// HWEncodeUsable is the media-INDEPENDENT proof a runtime failure has to
	// clear before the host is demoted: a synthetic encode through backend's
	// encoder for codec (empty = h264) with the given probed tuning features.
	// Fail-closed; true means the host still encodes, so the failure was the
	// media's or the route's, not the GPU's.
	HWEncodeUsable(ctx context.Context, backend, device, codec string, features []string) bool
	// ConfirmHWFeatures re-probes each of features individually and returns
	// the subset the backend still accepts, so an "unsupported option" verdict
	// narrows the host to what it proved rather than wiping everything.
	ConfirmHWFeatures(ctx context.Context, backend, device, codec string, features []string) []string
	// HWRouteUsable re-runs one route proof (an HWRoutes name) on backend for
	// the output codec that failed, the same builder session the calibration
	// ran. Fail-closed.
	HWRouteUsable(ctx context.Context, backend, device, route, codec string, features []string) bool
}
