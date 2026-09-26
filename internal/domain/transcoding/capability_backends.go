package transcoding

import (
	"slices"
	"strings"
)

// capability_backends.go - the per-accelerator half of EncodeCapability: what
// each certified backend (VAAPI, QSV, NVENC, RKMPP) decodes, encodes, tone-maps
// and has proven route by route, and how the resolver picks one backend for a
// route. The host-wide view (ceilings, samples, features, ports) stays in
// capability.go.

// HWDeviceInfo identifies one detected device for the admin: the backend, the
// device ffmpeg binds (render node / CUDA index) and a human label (vendor +
// driver, or GPU model), plus the encoders its real-encode probe passed.
type HWDeviceInfo struct {
	Backend  string   `json:"backend"`
	Device   string   `json:"device,omitempty"`
	Label    string   `json:"label,omitempty"`
	Encoders []string `json:"encoders,omitempty"`
}

// HWBackendCap is one accelerator's full capability - everything the resolver
// needs to BUILD and gate a route on that specific backend. One per certified
// backend in EncodeCapability.Backends.
type HWBackendCap struct {
	Backend      string   `json:"backend"`
	Device       string   `json:"device,omitempty"`
	Codecs       []string `json:"codecs,omitempty"`
	DecodeCodecs []string `json:"decode_codecs,omitempty"`
	Features     []string `json:"features,omitempty"`
	// Routes is written even when empty: an empty list is the fail-closed
	// verdict "no route proven" and must survive persistence - omitted, it
	// came back nil and read as "every route trusted".
	Routes []string `json:"routes"`
	// FailedRoutes are route+codec combinations (HWRouteCombo) that failed a
	// runtime re-proof: that topology no longer takes that output codec here,
	// the other codecs keep it. A calibration writes a fresh row without them.
	FailedRoutes  []string `json:"failed_routes,omitempty"`
	HW10Bit       bool     `json:"hw_10bit"`
	HWTonemap     bool     `json:"hw_tonemap"`
	HWOverlay     bool     `json:"hw_overlay"`
	HWHDR10Encode bool     `json:"hw_hdr10_encode"`
	// HWDeinterlace: the backend's SESSION-level GPU deinterlacer (QSV's
	// deinterlace_qsv, which has no only-if-flagged mode) proved CORRECT on an
	// interlaced mire: the output idets progressive at the source frame rate.
	// NVENC/VAAPI deinterlace per frame off the decoder flag and never need it.
	HWDeinterlace bool `json:"hw_deinterlace,omitempty"`
	// MaxHeight is THIS backend's own certified encode ceiling (the tallest hw
	// ladder rung it sustained under the realtime margin). The scalar
	// EncodeCapability.HWMaxHeight is the PRIMARY's; a secondary backend chosen
	// per route (a 1080p-only VAAPI next to a 2160p QSV) must cap its ladder at
	// its own ceiling, never the primary's. 0 on a row benched before it existed
	// (HWCeilingFor then falls back to the primary scalar).
	MaxHeight int `json:"max_height,omitempty"`
	// BridgeMaxHeight is THIS backend's certified ceiling for the CPU tone-map
	// bridge (GPU decode, CPU tone-map, GPU encode - LadderHWBridge), benched
	// per backend. 0 = not measured (the backend tone-maps on the GPU, or has
	// no H.264 encoder) or no rung passed: the bridge on it then caps at the
	// software HDR ceiling. The scalar EncodeCapability.HWBridgeMaxHeight is
	// the primary's.
	BridgeMaxHeight int `json:"bridge_max_height,omitempty"`
}

// DeinterlacesOnGPU reports whether an INTERLACED source may take this
// backend's zero-copy chain: the session-level deinterlacer probed correct and
// the builder route ran. Backends whose deinterlacer is per-frame (NVENC,
// VAAPI) always may - their filter is a no-op on progressive frames.
func (b HWBackendCap) DeinterlacesOnGPU() bool {
	switch b.Backend {
	case string(QSV):
		return b.HWDeinterlace && b.Proves(HWRouteDeinterlace)
	case string(RKMPP):
		return false // RGA has no deinterlacer: an interlaced source stays off the MPP zero-copy chain
	}
	return true
}

// Proves reports whether this backend ran route end-to-end through the builder.
// nil Routes is trusted: it only survives on a row benched before the route
// probes existed (ProbeVersion < 5) - normalized() turns nil into "no route
// proven" on every later row, so a persisted empty verdict cannot read as
// proof once the row is published.
func (b HWBackendCap) Proves(route string) bool {
	if b.Routes == nil {
		return true
	}
	return slices.Contains(b.Routes, route)
}

// ProvesFor is Proves for one output codec: the route is proven and that
// combination has not failed its runtime re-proof.
func (b HWBackendCap) ProvesFor(route, codec string) bool {
	return b.Proves(route) && !slices.Contains(b.FailedRoutes, HWRouteCombo(route, codec))
}

// HWRouteCombo names one route+output codec combination ("" codec = h264).
func HWRouteCombo(route, codec string) string {
	c := CanonicalVideoCodec(codec)
	if c == "" {
		c = "h264"
	}
	return route + "/" + c
}

// SupportsCodec reports whether this backend encodes the domain codec.
func (b HWBackendCap) SupportsCodec(codec string) bool {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	return slices.Contains(b.Codecs, codec)
}

// CanDecodeSource is EncodeCapability.HWCanDecodeSource for one backend.
func (b HWBackendCap) CanDecodeSource(codec string, tenBit bool, pixFmt string) bool {
	return decodable(b.DecodeCodecs, codec, tenBit, pixFmt)
}

// BackendCaps returns the per-backend capabilities, synthesising a single
// entry from the primary scalars when Backends is empty (a pre-Backends row):
// such a host behaves exactly as before - one backend for every route.
func (c EncodeCapability) BackendCaps() []HWBackendCap {
	if len(c.Backends) > 0 {
		return c.Backends
	}
	if c.HWBackend == "" {
		return nil
	}
	return []HWBackendCap{{
		Backend: c.HWBackend, Device: c.HWDevice, Codecs: c.HWCodecs, DecodeCodecs: c.HWDecodeCodecs,
		Features: c.HWFeatures, Routes: c.HWRoutes, HW10Bit: c.HW10BitTranscode, HWTonemap: c.HWTonemap,
		HWOverlay: c.HWOverlay, HWHDR10Encode: c.HWHDR10Encode, MaxHeight: c.HWMaxHeight,
		BridgeMaxHeight: c.HWBridgeMaxHeight,
	}}
}

// HWBridgeCeilingFor is the CPU tone-map bridge ceiling of the named backend:
// its own BridgeMaxHeight when it recorded one, the primary scalar for the
// primary (or an unnamed backend), 0 otherwise - a secondary whose bridge was
// never proven or failed every rung caps at the software HDR ceiling.
func (c EncodeCapability) HWBridgeCeilingFor(backend string) int {
	if b, ok := c.BackendCap(backend); ok && b.BridgeMaxHeight > 0 {
		return b.BridgeMaxHeight
	}
	if backend == "" || strings.EqualFold(backend, c.HWBackend) {
		return c.HWBridgeMaxHeight
	}
	return 0
}

// EffectiveBridgeCeilingFor is the ceiling a bridge session on the named
// backend is really capped at: its measured bridge ceiling when one rung
// passed, else the software HDR ceiling (the CPU tone-map is the bridge's
// bottleneck, so a bridge that clears no rung is bounded like the all-software
// HDR chain - 480p floor included). 0 only on an un-benched row: no bridge
// route is ever ranked, measured or shaped above this.
func (c EncodeCapability) EffectiveBridgeCeilingFor(backend string) int {
	if h := c.HWBridgeCeilingFor(backend); h > 0 {
		return h
	}
	return c.SoftwareHDRMaxHeight
}

// HWCeilingFor is the encode ceiling of the named certified backend: its own
// MaxHeight when it recorded one, else the primary scalar HWMaxHeight (a row
// benched before per-backend ceilings, or an unnamed backend). 0 = no hw
// ceiling. The ladder of a route picked on a secondary backend is capped here,
// so a 1080p-only accelerator next to a 2160p primary never gets a 2160p rung.
func (c EncodeCapability) HWCeilingFor(backend string) int {
	if b, ok := c.BackendCap(backend); ok && b.MaxHeight > 0 {
		return b.MaxHeight
	}
	return c.HWMaxHeight
}

// WithoutBackend drops one certified backend (a boot re-validation found it
// dead on this host) and every route sample measured on it, so the resolver
// can neither pick it per route nor price a session on its evidence. When it
// was the primary, the next certified backend is promoted to the scalar
// fields (with ITS ceiling); with none left the host is software-only.
func (c EncodeCapability) WithoutBackend(backend string) EncodeCapability {
	kept := make([]HWBackendCap, 0, len(c.Backends))
	for _, b := range c.Backends {
		if !strings.EqualFold(b.Backend, backend) {
			kept = append(kept, b)
		}
	}
	samples := make([]RouteSample, 0, len(c.RouteSamples))
	for _, s := range c.RouteSamples {
		if !strings.EqualFold(s.Key.Backend, backend) {
			samples = append(samples, s)
		}
	}
	c.Backends, c.RouteSamples = kept, samples
	if !strings.EqualFold(c.HWBackend, backend) {
		return c
	}
	if len(kept) == 0 {
		disableHW(&c)
		return c
	}
	// The hw ladder samples were the dead primary's: drop them so a re-derive
	// never reads the promoted backend's ceiling off another device's proof.
	bench := make([]BenchRungSample, 0, len(c.BenchSamples))
	for _, s := range c.BenchSamples {
		if s.Ladder != LadderHW && s.Ladder != LadderHWBridge {
			bench = append(bench, s)
		}
	}
	c.BenchSamples = bench
	p := kept[0]
	c.HWBackend, c.HWDevice, c.HWCodecs, c.HWDecodeCodecs = p.Backend, p.Device, p.Codecs, p.DecodeCodecs
	c.HWFeatures, c.HWRoutes = p.Features, p.Routes
	c.HW10BitTranscode, c.HWTonemap, c.HWOverlay, c.HWHDR10Encode = p.HW10Bit, p.HWTonemap, p.HWOverlay, p.HWHDR10Encode
	c.HWMaxHeight, c.HWBridgeMaxHeight = p.MaxHeight, p.BridgeMaxHeight
	return c
}

// AnyBackendSupports reports whether ANY certified backend encodes the codec.
func (c EncodeCapability) AnyBackendSupports(codec string) bool {
	for _, b := range c.BackendCaps() {
		if b.SupportsCodec(codec) {
			return true
		}
	}
	return false
}

// TonemapBackend returns a backend that tone-maps HDR->SDR on the GPU (proves
// the tone-map route and carries a usable tonemap for hdrRange): OpenCL and
// Vulkan/libplacebo (VAAPI) and CUDA (NVENC) handle any range - HLG and a
// Dolby Vision RPU included - the fixed VAAPI VPP only PQ. Empty ok=false when none - HDR then
// tone-maps in software. QSV's vpp is ineligible so it never proves the route.
// codec is the output codec ("" = h264): a combination that failed its
// runtime re-proof does not count.
func (c EncodeCapability) TonemapBackend(hdrRange, codec string) (HWBackendCap, bool) {
	for _, b := range c.BackendCaps() {
		// b.HWTonemap is the GPU tone-map probe; Proves gates the builder route
		// (QSV's vpp is ineligible so its route is never proven, even though its
		// probe may init). Both required so a pre-Backends row (nil Routes,
		// trusted) still respects the probe flag.
		if !b.HWTonemap || !b.ProvesFor(HWRouteTonemap, codec) || !slices.Contains([]string{"vaapi", "qsv", "nvenc", "rkmpp"}, b.Backend) {
			continue
		}
		// NVENC's only GPU tone-mapper is tonemap_cuda: no feature, no route
		// (a probed row from a stock ffmpeg, or a pre-CUDA row, bridges on the CPU).
		if b.Backend == "nvenc" && !slices.Contains(b.Features, FeatureTonemapCUDA) {
			continue
		}
		if hdrRange != "PQ" && !b.TonemapsAnyRange() {
			continue
		}
		return b, true
	}
	return HWBackendCap{}, false
}

// TonemapsAnyRange reports whether the backend's GPU tone-mapper reads the
// source's transfer and Dolby Vision RPU itself (OpenCL / CUDA), rather than
// the fixed PQ-only VPP curve.
func (b HWBackendCap) TonemapsAnyRange() bool {
	return slices.Contains(b.Features, FeatureTonemapOpenCL) || slices.Contains(b.Features, FeatureTonemapCUDA) ||
		slices.Contains(b.Features, FeatureTonemapVulkan)
}

// PassthroughBackend returns a backend that keeps HDR (HEVC Main10 passthrough):
// proves the route, encodes HEVC, HDR10 + 10-bit VPP. Empty ok=false when none.
func (c EncodeCapability) PassthroughBackend() (HWBackendCap, bool) {
	for _, b := range c.BackendCaps() {
		if b.ProvesFor(HWRoutePassthrough, "hevc") && b.SupportsCodec("hevc") && b.HWHDR10Encode && b.HW10Bit {
			return b, true
		}
	}
	return HWBackendCap{}, false
}

// OverlayBackend returns a backend that composites a bitmap subtitle on the GPU.
func (c EncodeCapability) OverlayBackend() (HWBackendCap, bool) {
	for _, b := range c.BackendCaps() {
		if b.HWOverlay {
			return b, true
		}
	}
	return HWBackendCap{}, false
}

// BackendCap returns the named backend's capability, ok=false when absent.
func (c EncodeCapability) BackendCap(name string) (HWBackendCap, bool) {
	for _, b := range c.BackendCaps() {
		if strings.EqualFold(b.Backend, name) {
			return b, true
		}
	}
	return HWBackendCap{}, false
}
