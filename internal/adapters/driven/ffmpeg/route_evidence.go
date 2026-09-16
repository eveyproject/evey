package ffmpeg

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// route_evidence.go - the calibration's per-route cost proof. For every route
// this host actually serves, it runs the PRODUCTION session (BuildFFmpegArgs,
// single-variant, real HLS output) on a warm accelerator, twice, and keeps the
// faster run's throughput and the CPU cores it burnt. That is what admission
// prices on (RouteCostModel), replacing the cumulative bare-preset HW rung the
// runtime never emits. Reuses the bench samples already resolved for the
// software/HW ladders - no extra clip generation.

// routeEvidenceRungs are the reference heights measured per route. Two points
// are enough for the cost model (smallest rung >= h), and boot stays bounded.
// Capped to the route's own ceiling by routeEvidenceRungsFor, which ALWAYS
// adds that ceiling: a route whose ceiling is under 1080 (a 720p-only
// accelerator, the 720p software floor, the 480p HDR floor) used to get no
// sample at all, and admission - which prices a named route ONLY on evidence -
// refused it outright. It is that host's whole capacity; it must be priced.
var routeEvidenceRungs = []int{1080, 2160}

// routeEvidenceRungsFor is the ascending list of heights to measure for a route
// capped at ceiling: every reference rung at or under it, plus the ceiling
// itself when it is not one of them. Empty for a route with no ceiling.
func routeEvidenceRungsFor(ceiling int) []int {
	if ceiling <= 0 {
		return nil
	}
	var out []int
	for _, h := range routeEvidenceRungs {
		if h <= ceiling {
			out = append(out, h)
		}
	}
	if len(out) == 0 || out[len(out)-1] != ceiling {
		out = append(out, ceiling)
	}
	return out
}

// evidenceRoute is one route to measure: its cost key parts and the opts
// mutation that makes BuildFFmpegArgs emit that pipeline.
type evidenceRoute struct {
	route          transcoding.RouteCostClass
	codec          string // domain codec the encoder is for
	backend        string // "" = software; else the specific accelerator
	device         string
	features       []string
	hdrSample      bool // measure on the 10-bit HDR sample
	softwareDecode bool
	opts           func(o *transcoding.HLSProcessOpts)
}

func (er evidenceRoute) hw() bool { return er.backend != "" }

// measureRouteEvidence measures every route the calibrated capability can
// serve and returns the per-route samples. Warm-up + best-of-two per point.
// Best-effort: a route that fails to produce a segment is skipped (it should
// not have been in HWRoutes), never fatal.
func (r *BenchmarkRunner) measureRouteEvidence(ctx context.Context, sdrSample string, sdrFallback bool, cap *transcoding.EncodeCapability) []transcoding.RouteSample {
	if sdrFallback || sdrSample == "" {
		return nil // the synthetic lavfi fallback is not a real decode workload
	}
	hdrSample, hdrFallback := r.resolveHDRSample(ctx)
	sourceFPS := float64(r.sourceRate())

	var routes []evidenceRoute
	// Software routes (always, on every host): x264 SDR, x264 HDR tone-map, x265.
	swFn := func(*transcoding.HLSProcessOpts) {}
	routes = append(routes,
		evidenceRoute{route: transcoding.RouteCostSDR, codec: "h264", opts: swFn},
		evidenceRoute{route: transcoding.RouteCostSoftwareHDR, codec: "h264", hdrSample: true, opts: func(o *transcoding.HLSProcessOpts) { o.ToneMap = true }},
		evidenceRoute{route: transcoding.RouteCostSDR, codec: "hevc", opts: func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "hevc" }},
	)
	// Hardware routes, PER certified backend (VAAPI and QSV both, if both proved):
	// only the routes that backend ran end-to-end, with ITS device and features.
	for _, bc := range cap.BackendCaps() {
		accel := transcoding.HWAccelType(bc.Backend)
		b, dev, feat := bc.Backend, bc.Device, bc.Features
		if bc.Proves(transcoding.HWRouteSDR) {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostSDR, codec: "h264", backend: b, device: dev, features: feat, opts: swFn})
		}
		if bc.Proves(transcoding.HWRouteHEVC) {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostSDR, codec: "hevc", backend: b, device: dev, features: feat, opts: func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "hevc" }})
		}
		if bc.Proves(transcoding.HWRouteAV1) {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostSDR, codec: "av1", backend: b, device: dev, features: feat, opts: func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "av1" }})
		}
		if bc.Proves(transcoding.HWRouteTonemap) && hwTonemapFilter(accel, feat) != "" {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostTonemap, codec: "h264", backend: b, device: dev, features: feat, hdrSample: true, opts: func(o *transcoding.HLSProcessOpts) { o.ToneMap, o.HWTonemap = true, true }})
		}
		if bc.Proves(transcoding.HWRouteBridge) {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostBridge, codec: "h264", backend: b, device: dev, features: feat, hdrSample: true, opts: func(o *transcoding.HLSProcessOpts) { o.ToneMap, o.HWTonemap = true, false }})
		}
		if bc.Proves(transcoding.HWRoutePassthrough) {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostPassthrough, codec: "hevc", backend: b, device: dev, features: feat, hdrSample: true, opts: func(o *transcoding.HLSProcessOpts) {
				o.VideoCodec, o.HDRPassthrough, o.HDRTransfer = "hevc", true, "pq"
			}})
		}
		if bc.Proves(transcoding.HWRouteDecoupled) && hwUploadFilter(accel) != "" {
			routes = append(routes, evidenceRoute{route: transcoding.RouteCostDecoupled, codec: "h264", backend: b, device: dev, features: feat, softwareDecode: true, opts: func(o *transcoding.HLSProcessOpts) { o.HWSoftwareDecode = true }})
		}
	}

	warmed := map[string]bool{}
	var samples []transcoding.RouteSample
	for _, er := range routes {
		sample, fallback := sdrSample, sdrFallback
		if er.hdrSample {
			sample, fallback = hdrSample, hdrFallback
		}
		if fallback || sample == "" {
			continue
		}
		enc := videoEncoderFor(codecOrH264(er), transcoding.HWAccelType(er.backend))
		key := transcoding.RouteCostKeyFor(transcoding.HWAccelType(er.backend), enc, er.route)
		for _, h := range routeEvidenceRungsFor(r.routeCeiling(er, cap)) {
			// Warm each accelerator once before its first measurement: the first
			// encode after idle runs far slower for its whole window (2.5-6x on
			// NVENC), the artefact this evidence exists to avoid.
			if er.hw() && !warmed[er.backend] {
				r.measureRoutePoint(ctx, sample, fallback, er, cap, h)
				warmed[er.backend] = true
			}
			best := r.conservativeRoutePoint(ctx, sample, fallback, er, cap, h)
			if best.fps <= 0 {
				r.logger.Warn("benchmark: route evidence point produced nothing, skipping",
					slog.String("route", string(er.route)), slog.String("encoder", enc), slog.Int("height", h))
				continue
			}
			samples = append(samples, transcoding.RouteSample{
				Key: key, Height: h, FPS: best.fps, SourceFPS: sourceFPS,
				CPUCores: best.cpuCores, SoftwareDecode: er.softwareDecode,
			})
			r.logger.Info("benchmark: route evidence",
				slog.String("backend", key.Backend), slog.String("encoder", key.Encoder), slog.String("route", string(er.route)),
				slog.Int("height", h), slog.Float64("fps", best.fps), slog.Float64("cpu_cores", best.cpuCores))
		}
	}
	return samples
}

func codecOrH264(er evidenceRoute) string {
	if er.codec == "" {
		return "h264"
	}
	return er.codec
}

// routeCeiling caps the measured rungs to the route's own max height so we do
// not try 2160 on a host that tops out at 1440 (software) for this route. A
// hardware route is capped at ITS backend's certified ceiling (a 1080p-only
// secondary next to a 2160p primary measures to 1080), the primary scalar
// being the fallback for a backend that recorded none.
func (r *BenchmarkRunner) routeCeiling(er evidenceRoute, cap *transcoding.EncodeCapability) int {
	if er.hw() {
		// The bridge route measures up to the backend's OWN bridge ceiling, the
		// software HDR ceiling when its bridge cleared no rung - never the
		// backend's plain encode ceiling.
		if er.route == transcoding.RouteCostBridge {
			if h := cap.EffectiveBridgeCeilingFor(er.backend); h > 0 {
				return h
			}
		}
		if h := cap.HWCeilingFor(er.backend); h > 0 {
			return h
		}
		return 2160
	}
	if er.hdrSample && cap.SoftwareHDRMaxHeight > 0 {
		return cap.SoftwareHDRMaxHeight
	}
	if strings.EqualFold(er.codec, "hevc") {
		return cap.SoftwareMaxHeight // no x265 ceiling of its own; bounded by the sw cap
	}
	if cap.SoftwareMaxHeight > 0 {
		return cap.SoftwareMaxHeight
	}
	return 1080
}

type routePoint struct {
	fps      float64
	cpuCores float64
}

// conservativeRoutePoint measures a route point twice and keeps the WORST of
// each dimension: the lower throughput and the higher CPU. A best-of-two would
// flatter the host (the least-contended run of two), pinning admission on a
// number it cannot always sustain; the pessimistic combination is what a pool
// budget should be sized against. fps<=0 (both runs failed) is dropped upstream.
func (r *BenchmarkRunner) conservativeRoutePoint(ctx context.Context, sample string, fallback bool, er evidenceRoute, cap *transcoding.EncodeCapability, h int) routePoint {
	var got routePoint
	n := 0
	for range 2 {
		if ctx.Err() != nil {
			break
		}
		p := r.measureRoutePoint(ctx, sample, fallback, er, cap, h)
		if p.fps <= 0 {
			continue
		}
		n++
		if n == 1 || p.fps < got.fps {
			got.fps = p.fps
		}
		if p.cpuCores > got.cpuCores {
			got.cpuCores = p.cpuCores
		}
	}
	return got
}

// measureRoutePoint runs one production session of the route at rung h on the
// sample and returns its throughput + CPU cores. Video-only (the sample has no
// audio): a faithful production shape, the muxed-AAC ~0.05 core apart (the
// route-bench CLI with an audio bed measures that fuller figure).
func (r *BenchmarkRunner) measureRoutePoint(ctx context.Context, sample string, fallback bool, er evidenceRoute, cap *transcoding.EncodeCapability, h int) routePoint {
	dir, err := os.MkdirTemp("", "evey-routeev-*")
	if err != nil {
		return routePoint{}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	o := transcoding.HLSProcessOpts{
		OutputDir:          dir,
		FilePath:           sample,
		Profiles:           []transcoding.HLSProfile{{Name: "r", Width: h * 16 / 9, Height: h, VideoBitrate: int64(h) * 6, AudioBitrate: 192}},
		VideoDecision:      "transcode",
		AudioTrackIndex:    -1,
		SubtitleTrackIndex: -1,
		VideoCodec:         "h264",
		UseFMP4:            true,
	}
	if er.hw() {
		dev := er.device
		if dev == "" {
			dev = hwDevicePath(er.backend)
		}
		o.HWAccel, o.HWDevice, o.HWFeatures = transcoding.HWAccelType(er.backend), dev, er.features
	}
	er.opts(&o)
	if fallback {
		return routePoint{}
	}
	args := append([]string{"-t", "5"}, BuildFFmpegArgs(o)...)
	res := runMeasured(ctx, r.ffmpegBin(), args, nil, 60_000_000_000)
	if res.err != "" || res.wall <= 0 {
		return routePoint{}
	}
	if segs, gerr := filepath.Glob(filepath.Join(dir, "stream_0_*.m4s")); gerr != nil || len(segs) == 0 {
		return routePoint{}
	}
	p := routePoint{fps: float64(res.frames) / res.wall}
	if res.cpu > 0 {
		p.cpuCores = res.cpu / res.wall
	}
	return p
}

// sortRouteSamples keeps the persisted evidence stable (backend, encoder,
// route, height) so a re-bench diff is readable.
func sortRouteSamples(s []transcoding.RouteSample) {
	sort.Slice(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.Key.Backend != b.Key.Backend {
			return a.Key.Backend < b.Key.Backend
		}
		if a.Key.Encoder != b.Key.Encoder {
			return a.Key.Encoder < b.Key.Encoder
		}
		if a.Key.Route != b.Key.Route {
			return a.Key.Route < b.Key.Route
		}
		return a.Height < b.Height
	})
}
