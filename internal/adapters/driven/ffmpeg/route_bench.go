package ffmpeg

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/procfork"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// route_bench.go - the instrument that measures what a SESSION of each
// transcode route costs on this host, against what the boot calibration and
// the admission cost model believe it costs.
//
// One row per (route, source class, source fps, rung): the session is built by
// the production builder (BuildFFmpegArgs, single-variant, the orchestrator's
// options, the real HLS muxer to disk) on a clip of the route's class at that
// frame rate, and measured for throughput (frames / wall), CPU actually burnt
// (rusage) and GPU engine utilisation (nvidia-smi, when present). Next to it:
// the bench rung the model prices this session on (the cumulative bare-preset
// ladder of benchmark.go, re-run here on the same clip so shape and arguments
// are compared on equal footing) and the model's cost for it.
//
// It writes rows; it changes nothing. VideoCostModel is read, never edited.

// RouteBenchOptions configures one run.
type RouteBenchOptions struct {
	FfmpegPath string
	// Backend pins the accelerator ("nvenc"/"vaapi"/"qsv"); "" = auto-detect,
	// "none" = CPU routes only.
	Backend string
	Device  string
	// Heights are the rungs measured (one session each); FPS the source frame
	// rates the synthetic clips are generated at.
	Heights []int
	FPS     []int
	// Routes filters the route names measured (empty = every applicable one).
	Routes []string
	// Classes are the synthetic clip classes generated per fps: sdr8 (2160p
	// HEVC 8-bit), hdr10 (2160p HEVC Main10 PQ), h264 (1080p H.264 8-bit - the
	// other half of a real library, and the source class the decode term of
	// the cost model prices differently). Default sdr8,hdr10.
	Classes []string
	// DurationSec is the clip length; a session encodes all of it.
	DurationSec int
	// WorkDir holds the generated clips (cached across runs).
	WorkDir string
	// Sources are real files measured on top of the synthetic clips, at their
	// own frame rate and class.
	Sources []string
	// Capability is the calibration the model is built from; nil runs the
	// production BenchmarkRunner first (the honest comparison: same host, same
	// moment).
	Capability *transcoding.EncodeCapability
	// Margin is the realtime margin the model applies (DefaultDerivePolicy).
	Margin   float64
	Progress func(string)
}

// RouteBenchRow is one measured session.
type RouteBenchRow struct {
	Backend      string  `json:"backend"`
	Route        string  `json:"route"`
	Class        string  `json:"class"` // the LadderClass the session is billed as
	Source       string  `json:"source"`
	SourceCodec  string  `json:"source_codec"`
	SourcePixFmt string  `json:"source_pix_fmt"`
	SourceHeight int     `json:"source_height"`
	SourceFPS    float64 `json:"source_fps"`
	Height       int     `json:"height"`
	Frames       int     `json:"frames"`
	WallSec      float64 `json:"wall_sec"`
	FPS          float64 `json:"fps"`
	SpeedX       float64 `json:"speed_x"`       // fps / source fps
	CPUSec       float64 `json:"cpu_sec"`       // user+sys of the ffmpeg process
	CPUCores     float64 `json:"cpu_cores"`     // cpu_sec / wall
	CPUShare     float64 `json:"cpu_share"`     // cpu_cores / host CPU budget
	GPUUtil      float64 `json:"gpu_util"`      // avg %, -1 when not sampled
	GPUEnc       float64 `json:"gpu_enc"`       // avg encoder %, -1 when not sampled
	GPUDec       float64 `json:"gpu_dec"`       // avg decoder %, -1 when not sampled
	BenchRung    int     `json:"bench_rung"`    // the bench rung the model priced this on (0 = none)
	BenchFPS     float64 `json:"bench_fps"`     // that rung's calibrated fps
	PredCost     float64 `json:"pred_cost"`     // VideoCostModel.CostForSource(class, height, source)
	ActualCost   float64 `json:"actual_cost"`   // source fps x margin / measured fps
	CostRatio    float64 `json:"cost_ratio"`    // actual / predicted (>1 = under-priced)
	PoolCPUCost  float64 `json:"pool_cpu_cost"` // cpu_share: what this session really takes from the CPU pool
	Argv         string  `json:"argv"`
	Error        string  `json:"error,omitempty"`
}

// routeBenchSource is one clip the routes are measured on.
type routeBenchSource struct {
	name, path, codec, pixFmt string
	height                    int
	fps                       float64
	hdr, tenBit               bool
}

// routeBenchRung is the rung a session encodes: production class-table shapes
// at typical raised bitrates.
var routeBenchRungs = map[int]transcoding.HLSProfile{
	720:  {Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3500, AudioBitrate: 192},
	1080: {Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192},
	1440: {Name: "1440p", Width: 2560, Height: 1440, VideoBitrate: 16000, AudioBitrate: 192},
	2160: {Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 40000, AudioBitrate: 256},
}

// routeBenchRoute is one production shape, with the class it is billed as.
type routeBenchRoute struct {
	name   string
	class  string // LadderClass label: "hw", "software", "software-hdr", "software-av1"
	hw     bool
	source string // synthetic clip class it runs on: sdr8 / hdr10
	opts   func(o *transcoding.HLSProcessOpts)
}

// routeBenchRoutes lists the routes the host can serve. GPU routes are the
// same shapes routeProbesFor proves; the CPU routes are the software producer's.
func routeBenchRoutes(cap transcoding.EncodeCapability, hwOn bool) []routeBenchRoute {
	var routes []routeBenchRoute
	accel := transcoding.HWAccelType(cap.HWBackend)
	if hwOn && cap.HWBackend != "" {
		hevc := slices.Contains(cap.HWCodecs, "hevc")
		routes = append(routes, routeBenchRoute{"hw-sdr-h264", "hw", true, "sdr8", func(o *transcoding.HLSProcessOpts) {}})
		if hevc {
			routes = append(routes, routeBenchRoute{"hw-sdr-hevc", "hw", true, "sdr8", func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "hevc" }})
		}
		if cap.HW10BitTranscode {
			routes = append(routes, routeBenchRoute{"hw-src10-h264", "hw", true, "hdr10", func(o *transcoding.HLSProcessOpts) {}})
		}
		if cap.HWTonemap && hwTonemapFilter(accel, cap.HWFeatures) != "" {
			routes = append(routes, routeBenchRoute{"hw-tonemap-h264", "hw", true, "hdr10", func(o *transcoding.HLSProcessOpts) { o.ToneMap, o.HWTonemap = true, true }})
		}
		routes = append(routes, routeBenchRoute{"hw-bridge-h264", "hw", true, "hdr10", func(o *transcoding.HLSProcessOpts) { o.ToneMap, o.HWTonemap = true, false }})
		if hevc && cap.HWHDR10Encode {
			routes = append(routes, routeBenchRoute{"hw-passthrough-hevc", "hw", true, "hdr10", func(o *transcoding.HLSProcessOpts) {
				o.VideoCodec, o.HDRPassthrough, o.HDRTransfer = "hevc", true, "pq"
			}})
		}
		if hwUploadFilter(accel) != "" {
			routes = append(routes, routeBenchRoute{"hw-decoupled-h264", "hw", true, "sdr8", func(o *transcoding.HLSProcessOpts) { o.HWSoftwareDecode = true }})
		}
	}
	routes = append(routes,
		routeBenchRoute{"cpu-sdr-h264", "software", false, "sdr8", func(o *transcoding.HLSProcessOpts) {}},
		routeBenchRoute{"cpu-hdr-h264", "software-hdr", false, "hdr10", func(o *transcoding.HLSProcessOpts) { o.ToneMap = true }},
		routeBenchRoute{"cpu-sdr-hevc", "software", false, "sdr8", func(o *transcoding.HLSProcessOpts) { o.VideoCodec = "hevc" }},
	)
	return routes
}

// RunRouteBench measures every applicable route and returns the rows in the
// order they were measured, plus the capability the model was built from.
func RunRouteBench(ctx context.Context, opts RouteBenchOptions) ([]RouteBenchRow, transcoding.EncodeCapability, error) {
	if opts.FfmpegPath == "" {
		opts.FfmpegPath = "ffmpeg"
	}
	if opts.DurationSec <= 0 {
		opts.DurationSec = 10
	}
	if opts.Margin <= 0 {
		opts.Margin = transcoding.DefaultDerivePolicy().Margin
	}
	if len(opts.Heights) == 0 {
		opts.Heights = []int{1080, 2160}
	}
	if len(opts.FPS) == 0 {
		opts.FPS = []int{24, 30, 50, 60}
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(os.TempDir(), "evey-route-bench")
	}
	if len(opts.Classes) == 0 {
		opts.Classes = []string{"sdr8", "hdr10"}
	}
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return nil, transcoding.EncodeCapability{}, err
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(string) {}
	}

	// 1. The calibration the model prices on: the production runner, here and
	// now, unless a persisted capability was handed in.
	var cap transcoding.EncodeCapability
	hwOn := opts.Backend != "none"
	if opts.Capability != nil {
		cap = *opts.Capability
		// A pinned backend re-targets a persisted capability of another one (a
		// prod-host calibrated on VAAPI measured on QSV): same host facts, the
		// encoder family swapped, its render node, the tuning features that
		// carry over (low-power), the ones that do not dropped.
		if hwOn && opts.Backend != "" && opts.Backend != cap.HWBackend {
			cap.HWBackend, cap.HWDevice = opts.Backend, hwDevicePath(opts.Backend)
			var kept []string
			for _, f := range cap.HWFeatures {
				if strings.HasPrefix(f, "low_power_") {
					kept = append(kept, f)
				}
			}
			cap.HWFeatures = kept
		}
	} else {
		progress("calibrating with the production BenchmarkRunner (this is what the model prices on)")
		runner := NewBenchmarkRunner(BenchmarkOpts{FfmpegPath: opts.FfmpegPath, SamplePath: filepath.Join(opts.WorkDir, "bench", "sample.hevc"), SingleVariant: true}, nil)
		pref := transcoding.HWPreference{DisableHW: !hwOn}
		if hwOn && opts.Backend != "" {
			pref.Backend = opts.Backend
		}
		c, err := runner.Run(ctx, pref)
		if err != nil {
			return nil, cap, fmt.Errorf("calibration: %w", err)
		}
		cap = transcoding.Derive(c.Report(), transcoding.DefaultDerivePolicy())
	}
	if opts.Device == "" {
		opts.Device = cap.HWDevice
	}
	model := newRouteBenchModel(cap)
	cpuBudget := sysinfo.CPUBudget()

	// 2. The clips: one synthetic per (class, fps), plus the real files.
	var sources []routeBenchSource
	for _, fps := range opts.FPS {
		for _, class := range opts.Classes {
			src, err := routeBenchSynthetic(ctx, opts, class, fps, progress)
			if err != nil {
				return nil, cap, err
			}
			sources = append(sources, src)
		}
	}
	for _, path := range opts.Sources {
		src, err := routeBenchProbeSource(ctx, opts.FfmpegPath, path)
		if err != nil {
			return nil, cap, err
		}
		sources = append(sources, src)
	}

	// 3. Measure. The GPU is warmed first: measured 2026-09-13 on an RTX 4070
	// Ti, the FIRST encode after idle runs 2.5-6x slower for its whole 10 s
	// (clock ramp, context creation) - the boot bench's 720p hw rung read 49 fps
	// against 304 fps warm, and any first row here would be as wrong.
	var rows []RouteBenchRow
	sampler := newGPUSampler(cap.HWBackend)
	if hwOn && cap.HWBackend != "" && len(sources) > 0 {
		progress("warming the GPU (one discarded session)")
		warm := routeBenchRoutes(cap, true)[0]
		_ = measureRouteSession(ctx, opts, cap, model, cpuBudget, nil, warm, sources[0], opts.Heights[0])
	}
	for _, route := range routeBenchRoutes(cap, hwOn) {
		if len(opts.Routes) > 0 && !slices.Contains(opts.Routes, route.name) {
			continue
		}
		for _, src := range sources {
			if !routeTakesSource(route, src) {
				continue
			}
			for _, h := range opts.Heights {
				if h > src.height {
					continue
				}
				row := measureRouteSession(ctx, opts, cap, model, cpuBudget, sampler, route, src, h)
				rows = append(rows, row)
				progress(fmt.Sprintf("%-20s %-14s %4dp src %3.0f fps: %6.1f fps (%.2fx) cpu %.2f cores gpu %.0f%% enc %.0f%%  cost pred %.3f actual %.3f ratio %.2f %s",
					route.name, src.name, h, src.fps, row.FPS, row.SpeedX, row.CPUCores, row.GPUUtil, row.GPUEnc, row.PredCost, row.ActualCost, row.CostRatio, row.Error))
			}
		}
	}
	// 4. The bench shape itself, on the same clips, so shape and arguments are
	// compared on equal footing: benchmark.go's cumulative bare ladder for the
	// HW backend and its single-variant software rung.
	if hwOn && cap.HWBackend != "" {
		for _, src := range sources {
			if src.hdr || src.tenBit {
				continue
			}
			for _, h := range opts.Heights {
				if h > src.height {
					continue
				}
				rows = append(rows, measureBenchShape(ctx, opts, cap, model, cpuBudget, sampler, src, h))
				progress(fmt.Sprintf("bench-shape %4dp src %3.0f fps: %6.1f fps", h, src.fps, rows[len(rows)-1].FPS))
			}
		}
	}
	return rows, cap, nil
}

// routeTakesSource: HDR routes run on HDR clips, SDR routes on the 8-bit ones
// (the synthetic sdr8 / h264 classes, or an SDR file).
func routeTakesSource(route routeBenchRoute, src routeBenchSource) bool {
	if route.source == "hdr10" {
		return src.hdr
	}
	return !src.hdr && !src.tenBit
}

// routeBenchModel wraps the admission cost model exactly as wire.go builds it.
type routeBenchModel struct {
	samples []transcoding.BenchRungSample
	cost    routeCostFn
}

// routeCostFn prices (class, height, source) like VideoSessionLimiter does.
// The app package owns VideoCostModel; the bench re-implements its two rules
// (smallest rung >= h, decode term) so this driven package never imports app.
type routeCostFn func(class string, h int, src transcoding.SourceClass) (cost float64, rung int, rungFPS float64)

func newRouteBenchModel(cap transcoding.EncodeCapability) routeBenchModel {
	m := routeBenchModel{samples: cap.BenchSamples}
	decode := map[string]transcoding.DecodeRungSample{}
	for _, d := range cap.DecodeSamples {
		if d.FPS > 0 && d.Height > 0 && d.Codec != "" {
			decode[transcoding.CanonicalVideoCodec(d.Codec)] = d
		}
	}
	decodeFPS := func(src transcoding.SourceClass) (float64, bool) {
		d, ok := decode[transcoding.CanonicalVideoCodec(src.Codec)]
		if !ok || !src.Known() {
			return 0, false
		}
		return d.FPS * float64(d.Height*d.Height) / float64(src.Pixels()), true
	}
	rungsOf := func(class string) []transcoding.BenchRungSample {
		var out []transcoding.BenchRungSample
		for _, s := range cap.BenchSamples {
			if s.Ladder == class && s.Passing && s.FPS > 0 {
				out = append(out, s)
			}
		}
		slices.SortFunc(out, func(a, b transcoding.BenchRungSample) int { return a.Height - b.Height })
		return out
	}
	m.cost = func(class string, h int, src transcoding.SourceClass) (float64, int, float64) {
		rungs := rungsOf(class)
		if len(rungs) == 0 && class != transcoding.LadderSoftware {
			rungs = rungsOf(transcoding.LadderSoftware)
		}
		if len(rungs) == 0 {
			return 1.0, 0, 0
		}
		r := rungs[len(rungs)-1]
		for _, s := range rungs {
			if s.Height >= h {
				r = s
				break
			}
		}
		base := r.RequiredFPS / r.FPS
		benchDec, ok1 := decodeFPS(cap.BenchSource)
		srcDec, ok2 := decodeFPS(src)
		if ok1 && ok2 {
			if encodeTime := 1/r.FPS - 1/benchDec; encodeTime > 0 {
				return r.RequiredFPS * (encodeTime + 1/srcDec), r.Height, r.FPS
			}
		}
		return base, r.Height, r.FPS
	}
	return m
}

// routeBenchSynthetic builds (once) the synthetic clip of a class at fps:
// the boot bench's own recipe (testsrc2 + blur + grain, x265 medium, 5 Mb/s,
// 3840x2160) with a 5.1 audio bed, so the session encodes audio like a real
// title. HDR10: Main10 + PQ/bt2020 tags.
func routeBenchSynthetic(ctx context.Context, opts RouteBenchOptions, class string, fps int, progress func(string)) (routeBenchSource, error) {
	src := routeBenchSource{name: fmt.Sprintf("%s-%dfps", class, fps), codec: "hevc", height: 2160, fps: float64(fps)}
	src.pixFmt = "yuv420p"
	encoder, size, rate := "libx265", "3840x2160", "5M"
	switch class {
	case "hdr10":
		src.pixFmt, src.hdr, src.tenBit = "yuv420p10le", true, true
	case "h264":
		src.codec, src.height = "h264", 1080
		encoder, size, rate = "libx264", "1920x1080", "8M"
	case "sdr8":
	default:
		return src, fmt.Errorf("unknown clip class %q (sdr8, hdr10, h264)", class)
	}
	src.path = filepath.Join(opts.WorkDir, fmt.Sprintf("route-%s-%dfps-%ds.mkv", class, fps, opts.DurationSec))
	if st, err := os.Stat(src.path); err == nil && st.Size() > 0 {
		return src, nil
	}
	progress("generating " + src.path)
	genCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	args := []string{"-hide_banner", "-nostats", "-v", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=duration=%d:size=%s:rate=%d,gblur=sigma=1.5,noise=alls=4:allf=t+u,format=%s", opts.DurationSec, size, fps, src.pixFmt),
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=d=%d:c=pink:a=0.1,aformat=channel_layouts=5.1", opts.DurationSec),
		"-map", "0:v", "-map", "1:a",
		"-c:v", encoder, "-preset", "medium", "-pix_fmt", src.pixFmt,
		"-b:v", rate, "-maxrate", rate, "-bufsize", rate,
		"-c:a", "ac3", "-b:a", "448k"}
	if encoder == "libx265" {
		x265 := "bframes=4:ref=3:b-adapt=2:rc-lookahead=20"
		if src.hdr {
			x265 += ":hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc"
			args = append(args, "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
		}
		args = append(args, "-tag:v", "hvc1", "-x265-params", x265)
	} else {
		args = append(args, "-x264-params", "bframes=4:ref=3:rc-lookahead=20")
	}
	args = append(args, src.path)
	out, err := procfork.CombinedOutput(genCtx, procfork.Options{Class: procfork.Production, Site: "route_bench.generate", UnboundedThreads: true}, opts.FfmpegPath, args...)
	if err != nil {
		return src, fmt.Errorf("generate %s: %w (%s)", src.path, err, tailString(string(out), 300))
	}
	return src, nil
}

// routeBenchProbeSource reads a real file's video class with ffprobe.
func routeBenchProbeSource(ctx context.Context, ffmpegPath, path string) (routeBenchSource, error) {
	src := routeBenchSource{name: "file:" + filepath.Base(path), path: path}
	probe := filepath.Join(filepath.Dir(ffmpegPath), strings.Replace(filepath.Base(ffmpegPath), "ffmpeg", "ffprobe", 1))
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "route_bench.probe"}, probe,
		"-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,height,pix_fmt,r_frame_rate,color_transfer,bits_per_raw_sample", "-of", "json", path)
	if err != nil {
		return src, fmt.Errorf("probe %s: %w", path, err)
	}
	var parsed struct {
		Streams []struct {
			Codec  string `json:"codec_name"`
			Height int    `json:"height"`
			PixFmt string `json:"pix_fmt"`
			Rate   string `json:"r_frame_rate"`
			Trc    string `json:"color_transfer"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Streams) == 0 {
		return src, fmt.Errorf("probe %s: unreadable", path)
	}
	s := parsed.Streams[0]
	src.codec, src.height, src.pixFmt = s.Codec, s.Height, s.PixFmt
	if n, d, ok := strings.Cut(s.Rate, "/"); ok {
		num, _ := strconv.ParseFloat(n, 64)
		den, _ := strconv.ParseFloat(d, 64)
		if den > 0 {
			src.fps = num / den
		}
	}
	src.tenBit = strings.Contains(s.PixFmt, "10")
	src.hdr = s.Trc == "smpte2084" || s.Trc == "arib-std-b67"
	return src, nil
}

// measureRouteSession runs one production session of route on src at rung h.
func measureRouteSession(ctx context.Context, opts RouteBenchOptions, cap transcoding.EncodeCapability, model routeBenchModel, cpuBudget int, sampler *gpuSampler, route routeBenchRoute, src routeBenchSource, h int) RouteBenchRow {
	row := RouteBenchRow{Backend: cap.HWBackend, Route: route.name, Class: route.class, Source: src.name, SourceCodec: src.codec,
		SourcePixFmt: src.pixFmt, SourceHeight: src.height, SourceFPS: src.fps, Height: h, GPUUtil: -1, GPUEnc: -1, GPUDec: -1}
	if !route.hw {
		row.Backend = "cpu"
	}
	rung := routeBenchRungs[h]
	if rung.Height == 0 {
		rung = transcoding.HLSProfile{Name: fmt.Sprintf("%dp", h), Width: h * 16 / 9, Height: h, VideoBitrate: int64(h) * 4, AudioBitrate: 192}
	}
	dir, err := os.MkdirTemp("", "evey-routebench-*")
	if err != nil {
		row.Error = err.Error()
		return row
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// The orchestrator's session, field for field (segment_orchestrator.spawnSession
	// / software_producer.startSession): one rung, muxed AAC stereo audio, fMP4,
	// the probed features, the software producer's pacing on the CPU route (its
	// 60 s burst covers the whole clip, so it does not bound the measurement).
	o := transcoding.HLSProcessOpts{
		OutputDir: dir, FilePath: src.path,
		Profiles:      []transcoding.HLSProfile{rung},
		VariantIndex:  0,
		VideoDecision: "transcode", AudioDecision: "transcode", AudioCodec: "aac", AudioTrackIndex: 0, MaxAudioChannels: 2,
		SubtitleTrackIndex: -1, VideoCodec: "h264", UseFMP4: true, HDRSupported: false,
	}
	if route.hw {
		o.HWAccel, o.HWDevice, o.HWFeatures = transcoding.HWAccelType(cap.HWBackend), opts.Device, cap.HWFeatures
	} else {
		o.ReadRate = 1.5
	}
	route.opts(&o)
	if o.HWAccel != "" && o.HDRPassthrough {
		o.HDRTransfer = "pq"
	}
	args := BuildFFmpegArgs(o)
	row.Argv = strings.Join(args, " ")
	res := runMeasured(ctx, opts.FfmpegPath, args, sampler, time.Duration(opts.DurationSec*6+30)*time.Second)
	row.Frames, row.WallSec, row.CPUSec, row.Error = res.frames, res.wall, res.cpu, res.err
	row.GPUUtil, row.GPUEnc, row.GPUDec = res.gpu, res.enc, res.dec
	if segs, gerr := filepath.Glob(filepath.Join(dir, "stream_0_*.m4s")); (gerr != nil || len(segs) == 0) && row.Error == "" {
		row.Error = "no segment written"
	}
	fillRouteCosts(&row, model, cpuBudget, opts.Margin, src, h)
	return row
}

// measureBenchShape runs benchmark.go's OWN probe shape on the clip: the
// cumulative bare-preset ladder up to h for the HW backend (runMultiVariant,
// hwInputArgs) - what HWMaxHeight and the hw cost table were derived from.
func measureBenchShape(ctx context.Context, opts RouteBenchOptions, cap transcoding.EncodeCapability, model routeBenchModel, cpuBudget int, sampler *gpuSampler, src routeBenchSource, h int) RouteBenchRow {
	row := RouteBenchRow{Backend: cap.HWBackend, Route: "bench-shape-hw", Class: "hw", Source: src.name, SourceCodec: src.codec,
		SourcePixFmt: src.pixFmt, SourceHeight: src.height, SourceFPS: src.fps, Height: h, GPUUtil: -1, GPUEnc: -1, GPUDec: -1}
	encoder := resolveHWEncoder(cap.HWBackend, "h264")
	var heights []int
	for _, r := range hwBenchLadder {
		if r <= h {
			heights = append(heights, r)
		}
	}
	// Same argv builder as the bench (runMultiVariant), assembled here so the
	// process can be measured for rusage/GPU like the sessions are.
	args := []string{"-hide_banner", "-nostats"}
	args = append(args, hwInputArgs(cap.HWBackend)...)
	args = append(args, "-t", strconv.Itoa(opts.DurationSec), "-i", src.path)
	hwScale, hwPreset := "", ""
	switch {
	case strings.HasSuffix(encoder, "_nvenc"):
		hwScale, hwPreset = "scale_cuda", "p4"
	case strings.HasSuffix(encoder, "_vaapi"):
		hwScale = "scale_vaapi"
	case strings.HasSuffix(encoder, "_qsv"):
		hwScale, hwPreset = "scale_qsv", "veryfast"
	}
	var fc strings.Builder
	fmt.Fprintf(&fc, "[0:v]split=%d", len(heights))
	for i := range heights {
		fmt.Fprintf(&fc, "[v%d]", i)
	}
	for i, hh := range heights {
		fmt.Fprintf(&fc, ";[v%d]%s[o%d]", i, hwScaleToHeight(hwScale, hh), i)
	}
	args = append(args, "-filter_complex", fc.String())
	for i := range heights {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), fmt.Sprintf("-c:v:%d", i), encoder)
		if hwPreset != "" {
			args = append(args, "-preset", hwPreset)
		}
	}
	args = append(args, "-an", "-sn", "-f", "null", "-")
	row.Argv = strings.Join(args, " ")
	res := runMeasured(ctx, opts.FfmpegPath, args, sampler, time.Duration(opts.DurationSec*6+30)*time.Second)
	row.Frames, row.WallSec, row.CPUSec, row.Error = res.frames, res.wall, res.cpu, res.err
	row.GPUUtil, row.GPUEnc, row.GPUDec = res.gpu, res.enc, res.dec
	fillRouteCosts(&row, model, cpuBudget, opts.Margin, src, h)
	return row
}

func fillRouteCosts(row *RouteBenchRow, model routeBenchModel, cpuBudget int, margin float64, src routeBenchSource, h int) {
	if row.WallSec > 0 {
		row.FPS = float64(row.Frames) / row.WallSec
		row.CPUCores = row.CPUSec / row.WallSec
	}
	if src.fps > 0 {
		row.SpeedX = row.FPS / src.fps
	}
	if cpuBudget > 0 {
		row.CPUShare = row.CPUCores / float64(cpuBudget)
	}
	row.PoolCPUCost = row.CPUShare
	row.PredCost, row.BenchRung, row.BenchFPS = model.cost(row.Class, h, transcoding.SourceClass{Codec: src.codec, Height: src.height})
	if row.FPS > 0 {
		row.ActualCost = src.fps * margin / row.FPS
	}
	if row.PredCost > 0 && row.ActualCost > 0 {
		row.CostRatio = row.ActualCost / row.PredCost
	}
}

// measured is what one ffmpeg run yielded.
type measured struct {
	frames        int
	wall, cpu     float64
	gpu, enc, dec float64
	err           string
}

// runMeasured runs ffmpeg with the given argv, counting frames from its
// -progress stream and the CPU it burnt from rusage, with the GPU sampler
// running alongside.
func runMeasured(ctx context.Context, ffmpegPath string, args []string, sampler *gpuSampler, timeout time.Duration) measured {
	m := measured{gpu: -1, enc: -1, dec: -1}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append([]string{"-progress", "pipe:1", "-nostdin", "-y"}, args...)
	// Through procfork like every fork, but with the session's own conditions:
	// no nice, no address-space cap, no thread cap - a production HLS session
	// (hls_transcoder.go) runs with none of them, and the point is to measure
	// that session, not a background job.
	cmd, finish, err := procfork.Command(runCtx, procfork.Options{Class: procfork.Production, Site: "route_bench.session", NoNice: true, NoMemCap: true, UnboundedThreads: true}, ffmpegPath, full...)
	if err != nil {
		m.err = err.Error()
		return m
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.err = err.Error()
		return m
	}
	stop := sampler.start()
	started := time.Now()
	if err := cmd.Start(); err != nil {
		stop()
		m.err = err.Error()
		return m
	}
	frames := 0
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "frame="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				frames = n
			}
		}
	}
	waitErr := finish(cmd.Wait(), []byte(stderr.String()))
	m.wall = time.Since(started).Seconds()
	m.gpu, m.enc, m.dec = stop()
	m.frames = frames
	if ps := cmd.ProcessState; ps != nil {
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
			m.cpu = float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 + float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
		}
	}
	if waitErr != nil {
		m.err = tailString(strings.TrimSpace(stderr.String()), 300)
	}
	return m
}

// gpuSampler averages nvidia-smi's engine utilisation while a run lasts.
// nil (no nvidia-smi, non-NVIDIA backend) samples nothing and reports -1.
type gpuSampler struct {
	bin string
}

func newGPUSampler(backend string) *gpuSampler {
	if backend != "nvenc" {
		return nil
	}
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	return &gpuSampler{bin: bin}
}

// start launches the sampler and returns the function that stops it and
// yields the averages (gpu, encoder, decoder %).
func (s *gpuSampler) start() func() (float64, float64, float64) {
	if s == nil {
		return func() (float64, float64, float64) { return -1, -1, -1 }
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Probe, Site: "route_bench.nvidia_smi"}, s.bin,
		"--query-gpu=utilization.gpu,utilization.encoder,utilization.decoder", "--format=csv,noheader,nounits", "-lms", "250")
	if err != nil {
		cancel()
		return func() (float64, float64, float64) { return -1, -1, -1 }
	}
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		cancel()
		return func() (float64, float64, float64) { return -1, -1, -1 }
	}
	var mu sync.Mutex
	var n int
	var g, e, d float64
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			f := strings.Split(sc.Text(), ",")
			if len(f) < 3 {
				continue
			}
			gv, _ := strconv.ParseFloat(strings.TrimSpace(f[0]), 64)
			ev, _ := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
			dv, _ := strconv.ParseFloat(strings.TrimSpace(f[2]), 64)
			mu.Lock()
			n++
			g += gv
			e += ev
			d += dv
			mu.Unlock()
		}
	}()
	return func() (float64, float64, float64) {
		cancel()
		_ = finish(cmd.Wait(), nil) //nolint:errcheck // the sampler is cancelled on purpose; its exit is not a result
		<-done
		mu.Lock()
		defer mu.Unlock()
		if n == 0 {
			return -1, -1, -1
		}
		return g / float64(n), e / float64(n), d / float64(n)
	}
}
