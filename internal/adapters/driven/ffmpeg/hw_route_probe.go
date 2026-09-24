package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// hw_route_probe.go - the route probes: every GPU shape the production builder
// can emit is run ONCE at calibration, on a real clip of the class the route
// serves, with the exact argv BuildFFmpegArgs produces and the real HLS muxer
// writing to disk. A route is proven only when ffmpeg exits 0 AND wrote its
// init + a media segment: "exit 0" alone is how a broken chain gets certified
// (the tone-map bridge measured on 2026-09-12: the hand-written probe chain
// passed, the builder's chain failed on every progressive title).
//
// Kept apart from the per-filter probes in hwdetect.go on purpose: those say
// whether a FILTER runs, these say whether a SESSION runs. The resolver takes a
// route only when its name is in EncodeCapability.HWRoutes.

// routeMireWidth/Height is the route mires' frame: above every decoder's
// minimum (NVDEC HEVC needs 144 rows) and one ladder rung the builder scales to.
const (
	routeMireWidth  = 384
	routeMireHeight = 216
)

// routeMire is one source class a route is probed on.
type routeMire struct {
	name       string   // file suffix
	pixFmt     string   // yuv420p (8-bit), yuv420p10le (Main10), or an exotic layout
	hdr        bool     // tag bt2020/PQ metadata (HDR10 class)
	encoder    string   // "" = libx265
	extra      []string // encoder args the class needs (a profile)
	interlaced bool     // a woven TFF H.264 clip (interlacedSampleArgs), the deinterlace route's class
}

// ext is the raw elementary-stream container the mire is written in.
func (m routeMire) ext() string {
	if m.interlaced {
		return "h264"
	}
	return "hevc"
}

// routeMireForDecodeClass builds the mire of a persisted decode class outside
// the canonical 4:2:0 ones, from its probe matrix row.
func routeMireForDecodeClass(class string) (routeMire, bool) {
	c, ok := decodeProbeFor(class)
	if !ok || transcoding.CanonicalDecodeFormat(c.pixFmt) {
		return routeMire{}, false
	}
	return routeMire{name: c.codec + "-" + c.pixFmt, pixFmt: c.pixFmt, encoder: c.swEncoder, extra: c.encoderArgs}, true
}

var (
	routeMireSDR8  = routeMire{name: "sdr8", pixFmt: "yuv420p"}
	routeMireSDR10 = routeMire{name: "sdr10", pixFmt: "yuv420p10le"}
	routeMireHDR10 = routeMire{name: "hdr10", pixFmt: "yuv420p10le", hdr: true}
	routeMireTFF   = routeMire{name: "tff", pixFmt: "yuv420p", interlaced: true}
)

// routeProbe is one production shape: the route name persisted in HWRoutes,
// the mire class it plays, and the opts the builder reads for it.
type routeProbe struct {
	route string
	mire  routeMire
	opts  func(o *transcoding.HLSProcessOpts)
}

// routeProbesFor lists the routes worth proving on this backend, given the
// filter-level facts already probed. Pure so the selection is unit-tested.
func routeProbesFor(accel transcoding.HWAccelType, codecs, features []string, hwTonemap, hw10Bit, hdr10, hwDeint bool, decodeClasses []string) []routeProbe {
	hevc := slices.Contains(codecs, "hevc")
	probes := []routeProbe{
		{route: transcoding.HWRouteSDR, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {}},
	}
	if hevc {
		probes = append(probes, routeProbe{route: transcoding.HWRouteHEVC, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {
			o.VideoCodec = "hevc"
		}})
	}
	if slices.Contains(codecs, "av1") {
		probes = append(probes, routeProbe{route: transcoding.HWRouteAV1, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {
			o.VideoCodec = "av1"
		}})
	}
	if hw10Bit {
		probes = append(probes, routeProbe{route: transcoding.HWRouteSource10Bit, mire: routeMireSDR10, opts: func(o *transcoding.HLSProcessOpts) {}})
	}
	if hwTonemap && hwTonemapFilter(accel, features) != "" {
		probes = append(probes, routeProbe{route: transcoding.HWRouteTonemap, mire: routeMireHDR10, opts: func(o *transcoding.HLSProcessOpts) {
			o.ToneMap, o.HWTonemap = true, true
		}})
	}
	if hwTonemapBridge(accel, "tonemapx", "") != "" {
		probes = append(probes, routeProbe{route: transcoding.HWRouteBridge, mire: routeMireHDR10, opts: func(o *transcoding.HLSProcessOpts) {
			o.ToneMap, o.HWTonemap = true, false
		}})
	}
	if hdr10 && hevc {
		probes = append(probes, routeProbe{route: transcoding.HWRoutePassthrough, mire: routeMireHDR10, opts: func(o *transcoding.HLSProcessOpts) {
			o.VideoCodec, o.HDRPassthrough, o.HDRTransfer = "hevc", true, "pq"
		}})
	}
	if hwUploadFilter(accel) != "" {
		probes = append(probes, routeProbe{route: transcoding.HWRouteDecoupled, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {
			o.HWSoftwareDecode = true
		}})
	}
	// The session-level GPU deinterlacer (QSV only - the per-frame backends
	// emit theirs on every route already), on a genuinely interlaced clip.
	if hwDeint && accel == transcoding.QSV && hwDeinterlaceFilter(accel, false, true) != "" {
		probes = append(probes, routeProbe{route: transcoding.HWRouteDeinterlace, mire: routeMireTFF, opts: func(o *transcoding.HLSProcessOpts) {
			o.HWDeinterlace = true
		}})
	}
	// Every decode class outside 4:2:0 8/10-bit the GPU measured (4:4:4, 12-bit):
	// decodable is not servable until the builder's scaler folded that layout.
	for _, class := range decodeClasses {
		if m, ok := routeMireForDecodeClass(class); ok {
			probes = append(probes, routeProbe{route: transcoding.HWRouteSourceFormat(m.pixFmt), mire: m, opts: func(o *transcoding.HLSProcessOpts) {}})
		}
	}
	return probes
}

// routeProbeOpts is the session the probe runs: one ladder rung at the mire's
// own size, no audio (the mires carry none), fMP4 like production, every HW
// fact threaded exactly as the orchestrator threads it. Exported to the tests
// so the argv the probe runs can be pinned next to the production goldens.
func routeProbeOpts(accel transcoding.HWAccelType, device string, features []string, mirePath, outDir string, p routeProbe) transcoding.HLSProcessOpts {
	o := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           mirePath,
		Profiles:           []transcoding.HLSProfile{{Name: "216p", Width: routeMireWidth, Height: routeMireHeight, VideoBitrate: 600, AudioBitrate: 96}},
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		AudioTrackIndex:    -1,
		SubtitleTrackIndex: -1,
		VideoCodec:         "h264",
		HWAccel:            accel,
		HWDevice:           device,
		HWFeatures:         features,
		UseFMP4:            true,
	}
	p.opts(&o)
	return o
}

// routeMirePath sits next to the bench samples, class in the name.
func (r *BenchmarkRunner) routeMirePath(m routeMire) string {
	return filepath.Join(filepath.Dir(r.samplePath), "route-"+m.name+"."+m.ext())
}

// ensureRouteMire encodes the class's clip once (1 s, 24 fps, HEVC, the
// production 2160p container codec) and caches it with the bench samples.
// Progressive on purpose: that is what every film is, and what the bridge
// failed on. Fail-closed: no mire, no proof, no route.
func (r *BenchmarkRunner) ensureRouteMire(ctx context.Context, m routeMire) (string, bool) {
	dst := r.routeMirePath(m)
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst, true
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", false
	}
	genCtx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	if m.interlaced {
		args := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y"}, interlacedSampleArgs(fmt.Sprintf("%dx%d", routeMireWidth, routeMireHeight), 1)...)
		args = append(args, "-f", "h264", dst)
		if out, err := procfork.CombinedOutput(genCtx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.route_mire"}, r.ffmpegBin(), args...); err != nil {
			r.logger.Warn("benchmark: route mire generation failed",
				slog.String("mire", m.name), slog.Any("err", err), slog.String("tail", tailString(string(out), 200)))
			return "", false
		}
		return dst, true
	}
	enc := m.encoder
	if enc == "" {
		enc = "libx265"
	}
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=24:duration=1,format=%s", routeMireWidth, routeMireHeight, m.pixFmt),
		"-c:v", enc, "-preset", "ultrafast", "-pix_fmt", m.pixFmt, "-an"}
	if enc == "libx265" {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args, m.extra...)
	if m.hdr {
		args = append(args,
			"-x265-params", "hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
			"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
	}
	args = append(args, dst)
	if out, err := procfork.CombinedOutput(genCtx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.route_mire"}, r.ffmpegBin(), args...); err != nil {
		r.logger.Warn("benchmark: route mire generation failed",
			slog.String("mire", m.name), slog.Any("err", err), slog.String("tail", tailString(string(out), 200)))
		return "", false
	}
	return dst, true
}

// probeHWRoutes runs every candidate route through the production builder and
// returns the names that produced a segment. Never nil once the backend is
// probed - an empty list is the fail-closed verdict "no route proven", which
// the resolver honours; nil is reserved for rows benched before this existed.
func (r *BenchmarkRunner) probeHWRoutes(ctx context.Context, backend, device string, codecs, features []string, hwTonemap, hw10Bit, hdr10, hwDeint bool, decodeClasses []string) []string {
	accel := transcoding.HWAccelType(backend)
	proven := []string{}
	for _, p := range routeProbesFor(accel, codecs, features, hwTonemap, hw10Bit, hdr10, hwDeint, decodeClasses) {
		mire, ok := r.ensureRouteMire(ctx, p.mire)
		if !ok {
			continue
		}
		ok, tail := r.runRouteProbe(ctx, routeProbeOpts(accel, device, features, mire, "", p))
		if ok {
			proven = append(proven, p.route)
			r.logger.Info("benchmark: HW route proven through the production builder",
				slog.String("backend", backend), slog.String("route", p.route))
			continue
		}
		// Loud on purpose: a route the host cannot run is the single most useful
		// line of a boot log for whoever wonders why a title plays on the CPU.
		r.logger.Warn("benchmark: HW route FAILED through the production builder - not routed to the GPU",
			slog.String("backend", backend), slog.String("route", p.route), slog.String("stderr_tail", tail))
	}
	return proven
}

// HWRouteUsable re-proves one route on backend for the output codec that
// failed, at runtime, when a session of that shape failed while the plain
// encode still passes. A route the builder cannot express here is not usable.
func (r *BenchmarkRunner) HWRouteUsable(ctx context.Context, backend, device, route, codec string, features []string) bool {
	accel := transcoding.HWAccelType(backend)
	if device == "" {
		device = hwDevicePath(backend)
	}
	for _, p := range routeProbesFor(accel, []string{"h264", "hevc", "av1"}, features, true, true, true, true, nil) {
		if p.route != route {
			continue
		}
		mire, ok := r.ensureRouteMire(ctx, p.mire)
		if !ok {
			return false
		}
		opts := routeReproofOpts(accel, device, features, mire, p, codec)
		ok, tail := r.runRouteProbe(ctx, opts)
		if !ok {
			r.logger.Warn("benchmark: HW route failed its runtime re-proof",
				slog.String("backend", backend), slog.String("route", route), slog.String("codec", opts.VideoCodec), slog.String("stderr_tail", tail))
		}
		return ok
	}
	return false
}

// routeReproofOpts is the calibration's session for p, encoding the codec
// that failed ("" keeps the probe's own).
func routeReproofOpts(accel transcoding.HWAccelType, device string, features []string, mirePath string, p routeProbe, codec string) transcoding.HLSProcessOpts {
	o := routeProbeOpts(accel, device, features, mirePath, "", p)
	if c := transcoding.CanonicalVideoCodec(codec); c != "" {
		o.VideoCodec = c
	}
	return o
}

// runRouteProbe runs one builder-produced session into a scratch directory
// and reports whether it wrote its init and a media segment. The tail of
// ffmpeg's stderr is returned for the log on failure.
func (r *BenchmarkRunner) runRouteProbe(ctx context.Context, opts transcoding.HLSProcessOpts) (bool, string) {
	dir, err := os.MkdirTemp("", "evey-routeprobe-*")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _ = os.RemoveAll(dir) }()
	opts.OutputDir = dir
	if verr := opts.Validate(); verr != nil {
		return false, verr.Error()
	}
	runCtx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-nostats", "-y"}, BuildFFmpegArgs(opts)...)
	out, err := procfork.CombinedOutput(runCtx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.route"}, r.ffmpegBin(), args...)
	if err != nil {
		return false, tailString(strings.TrimSpace(string(out)), 400)
	}
	// The proof of use: the init and at least one media segment on disk. A
	// stat/glob error reads as "not there", which is the fail-closed answer.
	init, serr := os.Stat(filepath.Join(dir, "stream_0_init.mp4"))
	segs, gerr := filepath.Glob(filepath.Join(dir, "stream_0_*.m4s"))
	if serr != nil || gerr != nil || init.Size() == 0 || len(segs) == 0 {
		return false, "exit 0 but no init/segment written: " + tailString(strings.TrimSpace(string(out)), 300)
	}
	return true, ""
}
