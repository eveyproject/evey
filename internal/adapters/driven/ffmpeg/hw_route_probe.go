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
	name   string // file suffix
	pixFmt string // yuv420p (8-bit) or yuv420p10le (Main10)
	hdr    bool   // tag bt2020/PQ metadata (HDR10 class)
}

var (
	routeMireSDR8  = routeMire{name: "sdr8", pixFmt: "yuv420p"}
	routeMireSDR10 = routeMire{name: "sdr10", pixFmt: "yuv420p10le"}
	routeMireHDR10 = routeMire{name: "hdr10", pixFmt: "yuv420p10le", hdr: true}
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
func routeProbesFor(accel transcoding.HWAccelType, codecs, features []string, hwTonemap, hw10Bit, hdr10 bool) []routeProbe {
	hevc := slices.Contains(codecs, "hevc")
	probes := []routeProbe{
		{route: transcoding.HWRouteSDR, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {}},
	}
	if hevc {
		probes = append(probes, routeProbe{route: transcoding.HWRouteHEVC, mire: routeMireSDR8, opts: func(o *transcoding.HLSProcessOpts) {
			o.VideoCodec = "hevc"
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
	return filepath.Join(filepath.Dir(r.samplePath), "route-"+m.name+".hevc")
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
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=24:duration=1,format=%s", routeMireWidth, routeMireHeight, m.pixFmt),
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", m.pixFmt, "-tag:v", "hvc1", "-an"}
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
func (r *BenchmarkRunner) probeHWRoutes(ctx context.Context, backend, device string, codecs, features []string, hwTonemap, hw10Bit, hdr10 bool) []string {
	accel := transcoding.HWAccelType(backend)
	proven := []string{}
	for _, p := range routeProbesFor(accel, codecs, features, hwTonemap, hw10Bit, hdr10) {
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
