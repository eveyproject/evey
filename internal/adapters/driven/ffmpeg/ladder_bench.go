package ffmpeg

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// ladder_bench.go - the ladder engine's measuring instrument (plan P1). For
// a title it samples a few scenes, extracts the candidate complexity features
// on each, encodes each scene at fixed quality targets and measures what that
// quality COST (bitrate) and BOUGHT (XPSNR against the source). The engine's
// models are calibrated on these rows, never on taste: a feature only earns a
// rule once it predicts the cost or the quality across a corpus.

// BenchFeatures are the per-scene candidate features. Every one is a
// hypothesis until the corpus says otherwise.
type BenchFeatures struct {
	// MotionYDIF is the mean frame-to-frame luma difference (the classifier's
	// motion proxy). SI/TI are ITU-T P.910 spatial and temporal information.
	MotionYDIF float64
	SI, TI     float64
	// SceneCutsPerMin at the classifier's 0.3 threshold and at scdet's own
	// default of 10: the two disagree by design, the corpus says which counts.
	SceneCutsPerMin03 float64
	SceneCutsPerMin10 float64
	// EntropyY is the normalised luma entropy (texture). Grain is the noise of
	// the lowest luma bit plane. Blur and Block come from the detectors of the
	// same names (source softness, source compression artefacts).
	EntropyY float64
	Grain    float64
	Blur     float64
	Block    float64
	Frames   int
}

// BenchEncode is one (height, quality) encode of a scene.
type BenchEncode struct {
	Height   int
	CRF      int
	Kbps     float64
	XPSNRY   float64
	EncodeFP float64 // encode speed, frames per second of wall clock
}

// BenchScene is one sampled window of a title.
type BenchScene struct {
	StartSec float64
	Features BenchFeatures
	Encodes  []BenchEncode
}

// BenchSource is what the probe says about the title.
type BenchSource struct {
	Path        string
	Codec       string
	Width       int
	Height      int
	PixelFormat string
	BitDepth    int
	HDR         bool
	FPS         float64
	DurationSec float64
	Kbps        float64
	// BitsPerPixel is the source's bits per pixel per frame: how the source
	// encoder valued the content, resolution and rate factored out.
	BitsPerPixel float64
}

// BenchResult is one title's rows.
type BenchResult struct {
	Source BenchSource
	Scenes []BenchScene
}

// BenchOptions drive one run.
type BenchOptions struct {
	Scenes    int
	WindowSec float64
	Heights   []int
	CRFs      []int
	Preset    string
	FFmpeg    string
	FFprobe   string
	WorkDir   string
	// Progress receives one line per step (nil = silent).
	Progress func(string)
	// Adaptive: bracket the reference fidelity from the 1.0 point instead of
	// measuring every ratio - the same interpolation pair, 2-3 encodes instead
	// of 5 (identical factor on 164/165 curves of the FULL corpus).
	Adaptive bool
	// Parallel: run the GPU encoders' curves alongside the CPU ones (the GPU
	// lane costs the CPU little more than its XPSNR reads).
	Parallel bool
	// CacheDir memoises every measured point by everything it depends on
	// (file identity, scene, height, filter chain, encoder configuration,
	// target, ffmpeg build): a point already known is never encoded again.
	// Empty = no cache.
	CacheDir string
}

// DefaultBenchOptions: three 8 s scenes, the delivery heights the ladders use
// most, three CRFs around the static policy's 20, the production preset.
func DefaultBenchOptions() BenchOptions {
	return BenchOptions{Scenes: 3, WindowSec: 8, Heights: []int{1080, 720}, CRFs: []int{18, 23, 28}, Preset: "veryfast", FFmpeg: "ffmpeg", FFprobe: "ffprobe"}
}

// RunLadderBench measures one title.
func RunLadderBench(ctx context.Context, path string, opts BenchOptions) (*BenchResult, error) {
	src, err := benchProbe(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	if src.DurationSec < opts.WindowSec*2 {
		return nil, fmt.Errorf("ladder-bench: %s is too short (%.0f s)", path, src.DurationSec)
	}
	res := &BenchResult{Source: src}
	for _, start := range benchSceneStarts(src.DurationSec, opts.Scenes, opts.WindowSec) {
		scene := BenchScene{StartSec: start}
		opts.log("features %s @ %.0fs", filepath.Base(path), start)
		f, err := benchFeatures(ctx, src, start, opts)
		if err != nil {
			return nil, err
		}
		scene.Features = f
		for _, h := range opts.Heights {
			if benchRungWidth(h) > src.Width {
				continue // never upscale: the ladder frames by width, like the builders
			}
			for _, crf := range opts.CRFs {
				opts.log("encode %s @ %.0fs %dp crf %d", filepath.Base(path), start, h, crf)
				e, err := benchEncode(ctx, src, start, h, crf, opts)
				if err != nil {
					return nil, err
				}
				scene.Encodes = append(scene.Encodes, e)
			}
		}
		res.Scenes = append(res.Scenes, scene)
	}
	return res, nil
}

func (o BenchOptions) log(format string, args ...any) {
	if o.Progress != nil {
		o.Progress(fmt.Sprintf(format, args...))
	}
}

// benchSceneStarts spreads n windows between 10 % and 90 % of the title: the
// opening and the credits are not the film.
func benchSceneStarts(duration float64, n int, window float64) []float64 {
	if n <= 0 {
		n = 1
	}
	lo, hi := duration*0.10, duration*0.90-window
	if hi <= lo {
		return []float64{lo}
	}
	out := make([]float64, 0, n)
	for i := range n {
		frac := 0.5
		if n > 1 {
			frac = float64(i) / float64(n-1)
		}
		out = append(out, math.Round(lo+(hi-lo)*frac))
	}
	return out
}

func benchProbe(ctx context.Context, path string, opts BenchOptions) (BenchSource, error) {
	pr, err := NewProber(opts.FFprobe).ProbeStreams(ctx, path)
	if err != nil {
		return BenchSource{}, fmt.Errorf("ladder-bench: probe %s: %w", path, err)
	}
	var v *library.MediaStream
	for _, s := range pr.Streams {
		if s != nil && s.StreamType == library.StreamTypeVideo && !library.IsStillImageCodec(s.Codec) {
			v = s
			break
		}
	}
	if v == nil || v.Width == nil || v.Height == nil {
		return BenchSource{}, fmt.Errorf("ladder-bench: %s has no probed video stream", path)
	}
	src := BenchSource{Path: path, Codec: v.Codec, Width: *v.Width, Height: *v.Height, DurationSec: pr.Duration.Seconds()}
	if v.PixelFormat != nil {
		src.PixelFormat = *v.PixelFormat
	}
	if v.BitDepth != nil {
		src.BitDepth = *v.BitDepth
	}
	if v.Framerate != nil {
		src.FPS = *v.Framerate
	}
	src.HDR = hdr.DetectHDR([]hdr.VideoStream{v}).IsHDR
	if v.Bitrate != nil && *v.Bitrate > 0 {
		src.Kbps = float64(*v.Bitrate) / 1000
	} else if fi, err := os.Stat(path); err == nil && src.DurationSec > 0 {
		src.Kbps = float64(fi.Size()) * 8 / src.DurationSec / 1000
	}
	if src.FPS > 0 && src.Width > 0 {
		src.BitsPerPixel = src.Kbps * 1000 / (float64(src.Width) * float64(src.Height) * src.FPS)
	}
	return src, nil
}

// benchRungWidth is the frame width of a rung named by its height (the
// ladder tables' 16:9 widths); a letterboxed source keeps its own aspect.
func benchRungWidth(height int) int {
	switch height {
	case 2160:
		return 3840
	case 1440:
		return 2560
	case 1080:
		return 1920
	case 720:
		return 1280
	case 480:
		return 854
	case 360:
		return 640
	}
	return height * 16 / 9
}

// benchFeatureFilter is the one decode pass that yields every candidate
// feature. The grain plane follows the depth: plane 1 of a 10-bit picture is
// noise by construction, plane 3 is what plane 1 measures on 8-bit.
func benchFeatureFilter(src BenchSource) string {
	plane := 1
	if src.BitDepth > 8 {
		plane += src.BitDepth - 8
	}
	return fmt.Sprintf("format=yuv420p,signalstats,scdet,entropy,bitplanenoise=bitplane=%d,blurdetect,blockdetect,siti,metadata=print:file=-", plane)
}

func benchFeatures(ctx context.Context, src BenchSource, start float64, opts BenchOptions) (BenchFeatures, error) {
	args := []string{"-hide_banner", "-v", "error", "-nostdin",
		"-ss", fmt.Sprintf("%.3f", start), "-t", fmt.Sprintf("%.3f", opts.WindowSec),
		"-i", src.Path, "-map", "0:v:0", "-vf", benchFeatureFilter(src), "-f", "null", "-"}
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.features"}, opts.FFmpeg, args...)
	if err != nil {
		return BenchFeatures{}, fmt.Errorf("ladder-bench: features: %w", err)
	}
	f := parseBenchFeatures(string(out), opts.WindowSec)
	if f.Frames == 0 {
		return f, fmt.Errorf("ladder-bench: no frame analysed at %.0fs of %s", start, src.Path)
	}
	return f, nil
}

var benchKeyRE = regexp.MustCompile(`^(lavfi\.[A-Za-z0-9_.]+)=(-?[0-9]+\.?[0-9]*(?:e[-+]?[0-9]+)?)$`)

// parseBenchFeatures averages the per-frame metadata over the window.
func parseBenchFeatures(text string, windowSec float64) BenchFeatures {
	var f BenchFeatures
	var sum = map[string]float64{}
	var n = map[string]int{}
	cuts03, cuts10 := 0, 0
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "frame:") {
			f.Frames++
			continue
		}
		m := benchKeyRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		sum[m[1]] += v
		n[m[1]]++
		if m[1] == "lavfi.scd.score" {
			if v > 0.3 {
				cuts03++
			}
			if v > 10 {
				cuts10++
			}
		}
	}
	mean := func(k string) float64 {
		if n[k] == 0 {
			return 0
		}
		return sum[k] / float64(n[k])
	}
	f.MotionYDIF = mean("lavfi.signalstats.YDIF")
	f.SI = mean("lavfi.siti.si")
	f.TI = mean("lavfi.siti.ti")
	f.EntropyY = mean("lavfi.entropy.normalized_entropy.normal.Y")
	for k, v := range sum {
		if strings.HasPrefix(k, "lavfi.bitplanenoise.0.") && n[k] > 0 {
			f.Grain = v / float64(n[k]) // luma plane, whichever bit plane was measured
		}
	}
	f.Blur = mean("lavfi.blur")
	f.Block = mean("lavfi.block")
	if windowSec > 0 {
		f.SceneCutsPerMin03 = float64(cuts03) * 60 / windowSec
		f.SceneCutsPerMin10 = float64(cuts10) * 60 / windowSec
	}
	return f
}

// benchPreFilter is what the software pipeline does to the picture before
// the encoder: tone-map an HDR source, scale to the rung, 8-bit 4:2:0. The
// reference for XPSNR goes through the same chain so the metric judges the
// encoder alone.
func benchPreFilter(src BenchSource, height int) string {
	parts := []string{}
	if src.HDR {
		tm := hdr.DefaultToneMapOptions()
		tm.UseTonemapx = tonemapxAvailable("ffmpeg")
		parts = append(parts, hdr.BuildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, tm))
	}
	parts = append(parts, fmt.Sprintf("scale=%d:-2", benchRungWidth(height)), "format=yuv420p")
	return strings.Join(parts, ",")
}

func benchEncode(ctx context.Context, src BenchSource, start float64, height, crf int, opts BenchOptions) (BenchEncode, error) {
	dir := opts.WorkDir
	if dir == "" {
		dir = os.TempDir()
	}
	out := filepath.Join(dir, fmt.Sprintf("ladder-bench-%d-%d-%d.mp4", os.Getpid(), height, crf))
	defer os.Remove(out) //nolint:errcheck // scratch file, best-effort cleanup
	enc := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-ss", fmt.Sprintf("%.3f", start), "-t", fmt.Sprintf("%.3f", opts.WindowSec),
		"-i", src.Path, "-map", "0:v:0", "-vf", benchPreFilter(src, height),
		"-c:v", "libx264", "-preset", opts.Preset, "-crf", strconv.Itoa(crf)}
	enc = append(enc, softwareX264TuningArgs("libx264", -1)...)
	enc = append(enc, "-pix_fmt", "yuv420p", "-an", "-f", "mp4", out)
	t0 := time.Now()
	if err := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.encode"}, opts.FFmpeg, enc...); err != nil {
		return BenchEncode{}, fmt.Errorf("ladder-bench: encode %dp crf %d: %w", height, crf, err)
	}
	wall := time.Since(t0).Seconds()
	fi, err := os.Stat(out)
	if err != nil {
		return BenchEncode{}, err
	}
	e := BenchEncode{Height: height, CRF: crf, Kbps: float64(fi.Size()) * 8 / opts.WindowSec / 1000}
	if wall > 0 && src.FPS > 0 {
		e.EncodeFP = opts.WindowSec * src.FPS / wall
	}
	stats := out + ".xpsnr"
	defer os.Remove(stats) //nolint:errcheck // scratch file, best-effort cleanup
	q := []string{"-hide_banner", "-v", "error", "-nostdin",
		"-ss", fmt.Sprintf("%.3f", start), "-t", fmt.Sprintf("%.3f", opts.WindowSec), "-i", src.Path,
		"-i", out,
		"-lavfi", fmt.Sprintf("[0:v]%s[ref];[1:v][ref]xpsnr=stats_file=%s", benchPreFilter(src, height), stats),
		"-f", "null", "-"}
	if qerr := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.xpsnr"}, opts.FFmpeg, q...); qerr != nil {
		return BenchEncode{}, fmt.Errorf("ladder-bench: xpsnr %dp crf %d: %w", height, crf, qerr)
	}
	raw, err := os.ReadFile(stats)
	if err != nil {
		return BenchEncode{}, err
	}
	e.XPSNRY = parseXPSNRMeanY(string(raw))
	return e, nil
}

var xpsnrYRE = regexp.MustCompile(`XPSNR y:\s*([0-9]+\.?[0-9]*|inf)`)

// parseXPSNRMeanY averages the per-frame luma XPSNR; "inf" frames (identical
// picture) are clamped to 100 dB so they weigh without dominating.
func parseXPSNRMeanY(text string) float64 {
	sum, n := 0.0, 0
	for _, m := range xpsnrYRE.FindAllStringSubmatch(text, -1) {
		v := 100.0
		if m[1] != "inf" {
			p, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			v = math.Min(p, 100)
		}
		sum += v
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// --- the corpus view ---

// BenchRow is one scene flattened for a CSV, with the encodes as columns.
func BenchRows(results []*BenchResult) (header []string, rows [][]string) {
	header = []string{"file", "codec", "width", "height", "pixfmt", "depth", "hdr", "fps", "src_kbps", "src_bpp", "scene_s",
		"ydif", "si", "ti", "cuts03_pm", "cuts10_pm", "entropy_y", "grain", "blur", "block", "frames"}
	var encCols []string
	seen := map[string]bool{}
	for _, r := range results {
		for _, s := range r.Scenes {
			for _, e := range s.Encodes {
				k := fmt.Sprintf("%dp_crf%d", e.Height, e.CRF)
				if !seen[k] {
					seen[k] = true
					encCols = append(encCols, k)
				}
			}
		}
	}
	sort.Strings(encCols)
	for _, c := range encCols {
		header = append(header, c+"_kbps", c+"_xpsnr", c+"_fps")
	}
	for _, r := range results {
		s := r.Source
		for _, sc := range r.Scenes {
			f := sc.Features
			row := []string{filepath.Base(s.Path), s.Codec, strconv.Itoa(s.Width), strconv.Itoa(s.Height), s.PixelFormat, strconv.Itoa(s.BitDepth),
				strconv.FormatBool(s.HDR), fmtF(s.FPS), fmtF(s.Kbps), fmtF(s.BitsPerPixel), fmtF(sc.StartSec),
				fmtF(f.MotionYDIF), fmtF(f.SI), fmtF(f.TI), fmtF(f.SceneCutsPerMin03), fmtF(f.SceneCutsPerMin10), fmtF(f.EntropyY), fmtF(f.Grain), fmtF(f.Blur), fmtF(f.Block), strconv.Itoa(f.Frames)}
			byKey := map[string]BenchEncode{}
			for _, e := range sc.Encodes {
				byKey[fmt.Sprintf("%dp_crf%d", e.Height, e.CRF)] = e
			}
			for _, c := range encCols {
				if e, ok := byKey[c]; ok {
					row = append(row, fmtF(e.Kbps), fmtF(e.XPSNRY), fmtF(e.EncodeFP))
				} else {
					row = append(row, "", "", "")
				}
			}
			rows = append(rows, row)
		}
	}
	return header, rows
}

func fmtF(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

// FeatureCorrelations ranks the candidate features by how well they predict
// the cost of a fixed quality: the Spearman rank correlation of each feature
// with the bits per pixel the encoder spent at (height, crf), and with the
// XPSNR it reached. Rank correlation because none of the relations is
// expected to be linear, and because a single 4K remux must not drag a
// Pearson coefficient on its own.
type FeatureCorrelation struct {
	Feature   string
	VsBPP     float64
	VsXPSNR   float64
	N         int
	Reference string
}

// perTitle averages the scenes of each title first: the ladder is shaped per
// title, so that is the granularity a rule will be applied at; per scene shows
// whether the feature tracks the content at all.
func FeatureCorrelations(results []*BenchResult, height, crf int, perTitle bool) []FeatureCorrelation {
	type sample struct {
		feats map[string]float64
		bpp   float64
		xpsnr float64
	}
	var samples []sample
	for _, r := range results {
		s := r.Source
		var titleSamples []sample
		for _, sc := range r.Scenes {
			for _, e := range sc.Encodes {
				if e.Height != height || e.CRF != crf || s.FPS <= 0 {
					continue
				}
				w := float64(benchRungWidth(e.Height))
				h := w * float64(s.Height) / float64(s.Width)
				bpp := e.Kbps * 1000 / (w * h * s.FPS)
				f := sc.Features
				titleSamples = append(titleSamples, sample{
					feats: map[string]float64{
						"ydif": f.MotionYDIF, "si": f.SI, "ti": f.TI, "cuts03_pm": f.SceneCutsPerMin03, "cuts10_pm": f.SceneCutsPerMin10,
						"entropy_y": f.EntropyY, "grain": f.Grain, "blur": f.Blur, "block": f.Block,
						"src_bpp": s.BitsPerPixel, "src_kbps": s.Kbps, "src_height": float64(s.Height), "src_depth": float64(s.BitDepth),
					},
					bpp: bpp, xpsnr: e.XPSNRY,
				})
			}
		}
		if !perTitle || len(titleSamples) == 0 {
			samples = append(samples, titleSamples...)
			continue
		}
		agg := sample{feats: map[string]float64{}}
		for _, ts := range titleSamples {
			for k, v := range ts.feats {
				agg.feats[k] += v / float64(len(titleSamples))
			}
			agg.bpp += ts.bpp / float64(len(titleSamples))
			agg.xpsnr += ts.xpsnr / float64(len(titleSamples))
		}
		samples = append(samples, agg)
	}
	if len(samples) < 3 {
		return nil
	}
	names := []string{"ydif", "si", "ti", "cuts03_pm", "cuts10_pm", "entropy_y", "grain", "blur", "block", "src_bpp", "src_kbps", "src_height", "src_depth"}
	var out []FeatureCorrelation
	for _, name := range names {
		xs := make([]float64, len(samples))
		bpps := make([]float64, len(samples))
		qs := make([]float64, len(samples))
		for i, s := range samples {
			xs[i], bpps[i], qs[i] = s.feats[name], s.bpp, s.xpsnr
		}
		unit := "scenes"
		if perTitle {
			unit = "titles"
		}
		out = append(out, FeatureCorrelation{Feature: name, VsBPP: spearman(xs, bpps), VsXPSNR: spearman(xs, qs), N: len(samples), Reference: fmt.Sprintf("%dp crf %d, %d %s", height, crf, len(samples), unit)})
	}
	sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].VsBPP) > math.Abs(out[j].VsBPP) })
	return out
}

// spearman is the rank correlation of two equal-length series (ties share
// the mean rank). 0 when either series is constant.
func spearman(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0
	}
	rx, ry := ranks(x), ranks(y)
	mx, my := mean(rx), mean(ry)
	var sxy, sxx, syy float64
	for i := range rx {
		dx, dy := rx[i]-mx, ry[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

func ranks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, len(v))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			r[idx[k]] = avg
		}
		i = j + 1
	}
	return r
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// --- codec / encoder efficiency (plan P2) ---
//
// Efficiency is measured, never assumed: for a scene and a rung height the
// reference (libx264 veryfast, the production tuning, CRF 23) sets an
// operating point (B_ref, Q_ref). Every production encoder then encodes the
// same scene with ITS production rate-control arguments at 0.5, 0.7, 1.0, 1.4
// and 2.0 x B_ref; the bitrate it needs to reach Q_ref, interpolated on
// (log bitrate, XPSNR), divided by B_ref, is its efficiency factor. Below 1
// the encoder buys the reference's fidelity for less; above 1 it needs more.

// BenchEncoder is one production encoder configuration.
type BenchEncoder struct {
	Name     string // "x264-abr", "x265", "nvenc-h264", "vaapi-hevc"...
	Codec    string // domain codec
	Accel    transcoding.HWAccelType
	Preset   string
	Features []string
	Device   string
}

// EffRatios are the bitrate points around the reference cost.
var EffRatios = []float64{0.5, 0.7, 1.0, 1.4, 2.0}

// BenchEncoders lists the production encoders to calibrate on this host; the
// ones the host cannot run are skipped by the availability probe.
func BenchEncoders(device string) []BenchEncoder {
	nv := []string{"rc_lookahead", "spatial_aq", "temporal_aq", "b_ref_mode", "bframes"}
	return []BenchEncoder{
		{Name: "x264-abr", Codec: "h264", Preset: "veryfast"},
		{Name: "x265", Codec: "hevc", Preset: "veryfast"},
		{Name: "nvenc-h264", Codec: "h264", Accel: transcoding.NVENC, Preset: "p4", Features: nv},
		{Name: "nvenc-hevc", Codec: "hevc", Accel: transcoding.NVENC, Preset: "p4", Features: nv},
		{Name: "vaapi-h264", Codec: "h264", Accel: transcoding.VAAPI, Preset: "driver", Features: []string{"bframes_h264"}, Device: device},
		{Name: "vaapi-hevc", Codec: "hevc", Accel: transcoding.VAAPI, Preset: "driver", Features: []string{"bframes_hevc"}, Device: device},
		{Name: "qsv-h264", Codec: "h264", Accel: transcoding.QSV, Preset: "veryfast", Features: []string{"forced_idr"}, Device: device},
		{Name: "qsv-hevc", Codec: "hevc", Accel: transcoding.QSV, Preset: "veryfast", Features: []string{"forced_idr"}, Device: device},
	}
}

// benchHWInput is the device binding and upload chain of an encoder whose
// input is decoded on the CPU (the reference chain, then an upload).
func benchHWInput(e BenchEncoder) (pre []string, upload string) {
	switch e.Accel {
	case transcoding.NVENC:
		return []string{"-init_hw_device", "cuda=cu", "-filter_hw_device", "cu"}, "format=nv12,hwupload_cuda"
	case transcoding.VAAPI:
		return []string{"-vaapi_device", e.Device}, "format=nv12,hwupload"
	case transcoding.QSV:
		return []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}, "hwupload=extra_hw_frames=16,format=qsv"
	}
	return nil, ""
}

// benchEncoderArgs renders the production encoder arguments for a bitrate
// target: the same helpers the HLS builders use, so the calibration measures
// what a session gets.
func benchEncoderArgs(e BenchEncoder, kbps int64) []string {
	qt := transcoding.QualityTarget{Mode: transcoding.RateControlVBR, TargetKbps: kbps, MaxKbps: kbps, BufferKbps: kbps * 2}
	if e.Accel != "" && e.Accel != transcoding.None {
		args := []string{"-c:v", videoEncoderFor(e.Codec, e.Accel)}
		args = append(args, hwEncoderQualityArgs(e.Accel, e.Codec, e.Features)...)
		args = append(args, hwRateControlArgs(e.Accel, qt, -1)...)
		return args
	}
	enc := resolveVideoEncoder(e.Codec)
	args := []string{"-c:v", enc, "-preset", e.Preset}
	args = append(args, softwareProfileLevelArgs(enc, -1, transcoding.H264LevelIdcFor(1080))...)
	args = append(args, softwareX264TuningArgs(enc, -1)...)
	args = append(args, softwareRateControlArgs(qt, "")...)
	args = append(args, "-pix_fmt", "yuv420p")
	return args
}

// BenchEncoderAvailable runs one second of test pattern through the encoder
// with its production arguments; a failure means "not on this host".
func BenchEncoderAvailable(ctx context.Context, opts BenchOptions, e BenchEncoder) bool {
	pre, upload := benchHWInput(e)
	args := append([]string{"-hide_banner", "-v", "error", "-nostdin"}, pre...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=1")
	if upload != "" {
		args = append(args, "-vf", "format=yuv420p,"+upload)
	}
	args = append(args, benchEncoderArgs(e, 1000)...)
	args = append(args, "-f", "null", "-")
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.encoder_probe"}, opts.FFmpeg, args...) == nil
}

// EffPoint is one fixed-bitrate encode of a scene by an encoder.
type EffPoint struct {
	Ratio      float64
	TargetKbps int64
	ActualKbps float64
	XPSNRY     float64
	EncodeFP   float64
}

// EffMeasure is one encoder's curve on one scene at one height, and the
// factor read off it.
type EffMeasure struct {
	Encoder    BenchEncoder
	Height     int
	RefKbps    float64
	RefXPSNR   float64
	Points     []EffPoint
	Factor     float64
	OutOfRange bool
}

type EffScene struct {
	StartSec float64
	Measures []EffMeasure
}

type EffResult struct {
	Source BenchSource
	Scenes []EffScene
	// Encoders the host could not run, by name.
	Skipped []string
}

// RunEfficiencyBench measures one title's encoder efficiencies.
//
// Per (scene, height) the source is decoded, tone-mapped and framed ONCE into
// a raw mezzanine; the reference encode, every encoder and every XPSNR read
// those exact frames. Reading the mezzanine costs a fraction of decoding a
// 4K HEVC 10-bit source, which used to be paid twice per point; the pixels
// every encoder sees are identical to the source-fed chain by construction.
func RunEfficiencyBench(ctx context.Context, path string, opts BenchOptions, encoders []BenchEncoder) (*EffResult, error) {
	src, err := benchProbe(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	if src.DurationSec < opts.WindowSec*2 {
		return nil, fmt.Errorf("ladder-bench: %s is too short (%.0f s)", path, src.DurationSec)
	}
	res := &EffResult{Source: src}
	// Scene positions are those of the FULL protocol (three, spread over the
	// title); a shorter run takes the first ones, so its points are the same
	// points and the cache serves both.
	starts := benchSceneStarts(src.DurationSec, 3, opts.WindowSec)
	if opts.Scenes > 0 && opts.Scenes < len(starts) {
		starts = starts[:opts.Scenes]
	}
	cache := newBenchCache(opts.CacheDir, src, opts)
	for _, start := range starts {
		scene := EffScene{StartSec: start}
		for _, h := range opts.Heights {
			if benchRungWidth(h) > src.Width {
				continue
			}
			measures, err := benchSceneHeight(ctx, src, start, h, opts, encoders, cache)
			if err != nil {
				return nil, err
			}
			scene.Measures = append(scene.Measures, measures...)
		}
		res.Scenes = append(res.Scenes, scene)
	}
	return res, nil
}

// mezzanine is the decoded, tone-mapped, framed scene on disk.
type mezzanine struct {
	path     string
	width    int
	height   int
	fps      float64
	frames   int
	startSec float64
}

func (m mezzanine) inputArgs() []string {
	return []string{"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", fmt.Sprintf("%dx%d", m.width, m.height), "-r", fmt.Sprintf("%.6f", m.fps), "-i", m.path}
}

// benchMezzanine writes the scene's frames after the production pre-filter.
func benchMezzanine(ctx context.Context, src BenchSource, start float64, height int, opts BenchOptions) (mezzanine, error) {
	dir := opts.WorkDir
	if dir == "" {
		dir = os.TempDir()
	}
	fps := src.FPS
	if fps <= 0 {
		fps = 24
	}
	w := benchRungWidth(height)
	h := int(math.Round(float64(w)*float64(src.Height)/float64(src.Width))) / 2 * 2
	if src.Width <= 0 || src.Height <= 0 {
		h = height
	}
	m := mezzanine{path: filepath.Join(dir, fmt.Sprintf("ladder-mezz-%d-%d-%d.yuv", os.Getpid(), int(start), height)), width: w, height: h, fps: fps, frames: int(math.Round(opts.WindowSec * fps)), startSec: start}
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-ss", fmt.Sprintf("%.3f", start), "-i", src.Path, "-map", "0:v:0",
		"-vf", benchPreFilter(src, height), "-frames:v", strconv.Itoa(m.frames), "-f", "rawvideo", m.path}
	if err := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.mezzanine"}, opts.FFmpeg, args...); err != nil {
		return mezzanine{}, fmt.Errorf("ladder-bench: mezzanine %s @ %.0fs %dp: %w", src.Path, start, height, err)
	}
	return m, nil
}

// benchSceneHeight measures every encoder on one (scene, height): the
// mezzanine, the reference point, then each encoder's curve - the GPU ones in
// a second lane when Parallel is set.
func benchSceneHeight(ctx context.Context, src BenchSource, start float64, height int, opts BenchOptions, encoders []BenchEncoder, cache *benchCache) ([]EffMeasure, error) {
	name := filepath.Base(src.Path)
	ref, cached := cache.reference(start, height)
	var mezz mezzanine
	var mezzErr error
	mezzOnce := sync.OnceFunc(func() {
		opts.log("mezzanine %s @ %.0fs %dp", name, start, height)
		mezz, mezzErr = benchMezzanine(ctx, src, start, height, opts)
	})
	if !cached {
		mezzOnce()
		if mezzErr != nil {
			return nil, mezzErr
		}
		opts.log("reference %s @ %.0fs %dp", name, start, height)
		var err error
		ref, err = benchEncodeMezz(ctx, mezz, BenchEncoder{Name: refEncoderName, Codec: "h264", Preset: "veryfast"}, 0, 23, opts)
		if err != nil {
			return nil, err
		}
		cache.putReference(start, height, ref)
	}
	defer func() {
		if mezz.path != "" {
			os.Remove(mezz.path) //nolint:errcheck // scratch file
		}
	}()

	curve := func(e BenchEncoder) (EffMeasure, error) {
		m := EffMeasure{Encoder: e, Height: height, RefKbps: ref.ActualKbps, RefXPSNR: ref.XPSNRY}
		measure := func(ratio float64) (EffPoint, error) {
			target := int64(math.Round(ref.ActualKbps * ratio))
			if pt, ok := cache.point(start, height, e, target); ok {
				pt.Ratio = ratio
				return pt, nil
			}
			mezzOnce()
			if mezzErr != nil {
				return EffPoint{}, mezzErr
			}
			opts.log("encode %s @ %.0fs %dp %s %.1fx (%d kbps)", name, start, height, e.Name, ratio, target)
			pt, err := benchEncodeMezz(ctx, mezz, e, target, 0, opts)
			if err != nil {
				return EffPoint{}, err
			}
			pt.Ratio = ratio
			cache.putPoint(start, height, e, target, pt)
			return pt, nil
		}
		ratios := EffRatios
		if opts.Adaptive {
			ratios = nil
		}
		for _, r := range ratios {
			pt, err := measure(r)
			if err != nil {
				return m, err
			}
			m.Points = append(m.Points, pt)
		}
		if opts.Adaptive {
			pts, err := bracketPoints(ref.XPSNRY, measure)
			if err != nil {
				return m, err
			}
			m.Points = pts
		}
		m.Factor, m.OutOfRange = efficiencyFactor(m.Points, ref.ActualKbps, ref.XPSNRY)
		return m, nil
	}

	var cpu, gpu []BenchEncoder
	for _, e := range encoders {
		if e.Name == "x265" && height > 1080 {
			continue // libx265 above 1080p is not a production route on the boxes this targets
		}
		if e.Accel != "" && e.Accel != transcoding.None && opts.Parallel {
			gpu = append(gpu, e)
		} else {
			cpu = append(cpu, e)
		}
	}
	var gpuMeasures []EffMeasure
	var gpuErr error
	var wg sync.WaitGroup
	if len(gpu) > 0 {
		// The mezzanine must exist before the lanes fork.
		mezzOnce()
		if mezzErr != nil {
			return nil, mezzErr
		}
		wg.Go(func() {
			for _, e := range gpu {
				m, err := curve(e)
				if err != nil {
					gpuErr = err
					return
				}
				gpuMeasures = append(gpuMeasures, m)
			}
		})
	}
	var out []EffMeasure
	for _, e := range cpu {
		m, err := curve(e)
		if err != nil {
			wg.Wait()
			return nil, err
		}
		out = append(out, m)
	}
	wg.Wait()
	if gpuErr != nil {
		return nil, gpuErr
	}
	return append(out, gpuMeasures...), nil
}

const refEncoderName = "ref-x264-crf23"

// bracketPoints measures from the reference bitrate and walks toward the
// reference fidelity until it is bracketed: the interpolation only ever reads
// the two points around it, so the factor is the FULL curve's whenever the
// curve is monotone (164 of the 165 FULL corpus curves; all were monotone).
func bracketPoints(refXPSNR float64, measure func(ratio float64) (EffPoint, error)) ([]EffPoint, error) {
	first, err := measure(1.0)
	if err != nil {
		return nil, err
	}
	pts := []EffPoint{first}
	seq := []float64{1.4, 2.0}
	if first.XPSNRY > refXPSNR {
		seq = []float64{0.7, 0.5}
	}
	for _, r := range seq {
		pt, err := measure(r)
		if err != nil {
			return nil, err
		}
		pts = append(pts, pt)
		lo, hi := pts[0].XPSNRY, pts[0].XPSNRY
		for _, p := range pts {
			lo, hi = math.Min(lo, p.XPSNRY), math.Max(hi, p.XPSNRY)
		}
		if lo < refXPSNR && refXPSNR <= hi {
			break
		}
	}
	return pts, nil
}

// benchEncodeMezz encodes the mezzanine with an encoder at a bitrate target
// (or, for the reference, at crf) and scores it against the same frames.
func benchEncodeMezz(ctx context.Context, m mezzanine, e BenchEncoder, kbps int64, crf int, opts BenchOptions) (EffPoint, error) {
	dir := filepath.Dir(m.path)
	out := filepath.Join(dir, fmt.Sprintf("ladder-eff-%d-%s-%d-%d.mp4", os.Getpid(), e.Name, m.height, kbps))
	defer os.Remove(out) //nolint:errcheck // scratch file
	pre, upload := benchHWInput(e)
	args := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y"}, pre...)
	args = append(args, m.inputArgs()...)
	if upload != "" {
		args = append(args, "-vf", upload)
	}
	if crf > 0 {
		enc := resolveVideoEncoder(e.Codec)
		args = append(args, "-c:v", enc, "-preset", e.Preset, "-crf", strconv.Itoa(crf))
		args = append(args, softwareX264TuningArgs(enc, -1)...)
		args = append(args, "-pix_fmt", "yuv420p")
	} else {
		args = append(args, benchEncoderArgs(e, kbps)...)
	}
	args = append(args, hlsForceKeyframesArgs...)
	args = append(args, "-an", "-f", "mp4", out)
	t0 := time.Now()
	if err := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.eff_encode"}, opts.FFmpeg, args...); err != nil {
		return EffPoint{}, fmt.Errorf("ladder-bench: %s %dp at %d kbps: %w", e.Name, m.height, kbps, err)
	}
	wall := math.Max(time.Since(t0).Seconds(), 0.05)
	fi, err := os.Stat(out)
	if err != nil {
		return EffPoint{}, err
	}
	pt := EffPoint{TargetKbps: kbps, ActualKbps: float64(fi.Size()) * 8 / opts.WindowSec / 1000, EncodeFP: float64(m.frames) / wall}
	stats := out + ".xpsnr"
	defer os.Remove(stats) //nolint:errcheck // scratch file
	q := append([]string{"-hide_banner", "-v", "error", "-nostdin"}, m.inputArgs()...)
	q = append(q, "-i", out, "-lavfi", fmt.Sprintf("[1:v]format=yuv420p[e];[e][0:v]xpsnr=stats_file=%s", stats), "-f", "null", "-")
	if qerr := procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.eff_xpsnr"}, opts.FFmpeg, q...); qerr != nil {
		return EffPoint{}, fmt.Errorf("ladder-bench: xpsnr %s %dp: %w", e.Name, m.height, qerr)
	}
	raw, err := os.ReadFile(stats)
	if err != nil {
		return EffPoint{}, err
	}
	pt.XPSNRY = parseXPSNRMeanY(string(raw))
	return pt, nil
}

// efficiencyFactor interpolates, on (log actual bitrate, XPSNR), the bitrate
// at which the curve reaches refXPSNR, over refKbps. Outside the measured
// range the factor is the nearest end and outOfRange is set.
func efficiencyFactor(points []EffPoint, refKbps, refXPSNR float64) (factor float64, outOfRange bool) {
	pts := make([]EffPoint, 0, len(points))
	for _, p := range points {
		if p.ActualKbps > 0 && p.XPSNRY > 0 {
			pts = append(pts, p)
		}
	}
	if len(pts) < 2 || refKbps <= 0 {
		return 0, true
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].ActualKbps < pts[j].ActualKbps })
	if refXPSNR <= pts[0].XPSNRY {
		return pts[0].ActualKbps / refKbps, true
	}
	for i := 1; i < len(pts); i++ {
		lo, hi := pts[i-1], pts[i]
		if hi.XPSNRY >= refXPSNR && lo.XPSNRY < refXPSNR {
			t := (refXPSNR - lo.XPSNRY) / (hi.XPSNRY - lo.XPSNRY)
			lb := math.Log(lo.ActualKbps) + t*(math.Log(hi.ActualKbps)-math.Log(lo.ActualKbps))
			return math.Exp(lb) / refKbps, false
		}
	}
	return pts[len(pts)-1].ActualKbps / refKbps, true
}

// EffRows flattens every point for the CSV.
func EffRows(results []*EffResult) (header []string, rows [][]string) {
	header = []string{"file", "codec_src", "width", "height", "hdr", "fps", "scene_s", "rung", "encoder", "codec", "backend", "preset",
		"ref_kbps", "ref_xpsnr", "ratio", "target_kbps", "actual_kbps", "xpsnr", "enc_fps", "factor", "out_of_range"}
	for _, r := range results {
		s := r.Source
		for _, sc := range r.Scenes {
			for _, m := range sc.Measures {
				for _, p := range m.Points {
					rows = append(rows, []string{filepath.Base(s.Path), s.Codec, strconv.Itoa(s.Width), strconv.Itoa(s.Height), strconv.FormatBool(s.HDR), fmtF(s.FPS),
						fmtF(sc.StartSec), strconv.Itoa(m.Height), m.Encoder.Name, m.Encoder.Codec, string(m.Encoder.Accel), m.Encoder.Preset,
						fmtF(m.RefKbps), fmtF(m.RefXPSNR), fmtF(p.Ratio), strconv.FormatInt(p.TargetKbps, 10), fmtF(p.ActualKbps), fmtF(p.XPSNRY), fmtF(p.EncodeFP),
						fmtF(m.Factor), strconv.FormatBool(m.OutOfRange)})
				}
			}
		}
	}
	return header, rows
}

// EffSummary is one (encoder, height) cell of the calibration.
type EffSummary struct {
	Encoder      string
	Height       int
	Scenes       int
	Titles       int
	OutOfRange   int
	MedianFactor float64
	// Dispersion of the per-scene factors around the median, as factors.
	DispP50, DispP90, DispWorst float64
	// Leave-one-title-out error of the median, as factors.
	LOOP50, LOOP90, LOOWorst float64
	MedianFPS                float64
}

// EffSummaries aggregates the corpus per (encoder, height).
func EffSummaries(results []*EffResult) []EffSummary {
	type key struct {
		enc string
		h   int
	}
	type obs struct {
		title  string
		factor float64
		fps    float64
		oor    bool
	}
	byKey := map[key][]obs{}
	for _, r := range results {
		for _, sc := range r.Scenes {
			for _, m := range sc.Measures {
				o := obs{title: r.Source.Path, factor: m.Factor, oor: m.OutOfRange}
				for _, p := range m.Points {
					if p.Ratio == 1.0 {
						o.fps = p.EncodeFP
					}
				}
				k := key{m.Encoder.Name, m.Height}
				byKey[k] = append(byKey[k], o)
			}
		}
	}
	var out []EffSummary
	for k, os := range byKey {
		s := EffSummary{Encoder: k.enc, Height: k.h, Scenes: len(os)}
		titles := map[string]bool{}
		var valid []obs
		var fps []float64
		for _, o := range os {
			titles[o.title] = true
			if o.fps > 0 {
				fps = append(fps, o.fps)
			}
			if o.oor || o.factor <= 0 {
				s.OutOfRange++
				continue
			}
			valid = append(valid, o)
		}
		s.Titles = len(titles)
		s.MedianFPS = median(fps)
		if len(valid) == 0 {
			out = append(out, s)
			continue
		}
		factors := make([]float64, len(valid))
		for i, o := range valid {
			factors[i] = o.factor
		}
		s.MedianFactor = median(factors)
		var disp []float64
		for _, f := range factors {
			disp = append(disp, math.Abs(math.Log(f)-math.Log(s.MedianFactor)))
		}
		s.DispP50, s.DispP90, s.DispWorst = expPct(disp, 0.5), expPct(disp, 0.9), expPct(disp, 1)
		var loo []float64
		for t := range titles {
			var train, held []float64
			for _, o := range valid {
				if o.title == t {
					held = append(held, o.factor)
				} else {
					train = append(train, o.factor)
				}
			}
			if len(train) == 0 || len(held) == 0 {
				continue
			}
			pred := median(train)
			loo = append(loo, math.Abs(math.Log(median(held))-math.Log(pred)))
		}
		s.LOOP50, s.LOOP90, s.LOOWorst = expPct(loo, 0.5), expPct(loo, 0.9), expPct(loo, 1)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Encoder != out[j].Encoder {
			return out[i].Encoder < out[j].Encoder
		}
		return out[i].Height < out[j].Height
	})
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// expPct is exp(percentile p of v) - a log-space error read back as a factor.
func expPct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(math.Round(p * float64(len(s)-1)))
	return math.Exp(s[i])
}

// --- the point cache (INCREMENTAL mode) ---
//
// Every measured point is memoised under a key made of everything the
// measurement depends on: the protocol version, the file's identity (path,
// size, mtime), the scene, the window, the height and the pre-filter chain,
// the encoder configuration (name, codec, backend, preset, features, device),
// the bitrate target and the ffmpeg build. A point is recomputed only when one
// of those changed; a run over known titles and encoders costs nothing, and
// a new encoder or title adds only its own points.

// benchCacheProtocol bumps when the measurement itself changes meaning.
const benchCacheProtocol = 1

type benchCache struct {
	dir     string
	src     BenchSource
	opts    BenchOptions
	ident   string
	version string
	mu      sync.Mutex
}

type benchCachedPoint struct {
	Point    EffPoint `json:"point"`
	Measured string   `json:"measured"`
}

func newBenchCache(dir string, src BenchSource, opts BenchOptions) *benchCache {
	c := &benchCache{dir: dir, src: src, opts: opts}
	if dir == "" {
		return c
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.dir = ""
		return c
	}
	fi, err := os.Stat(src.Path)
	if err != nil {
		c.dir = ""
		return c
	}
	c.ident = fmt.Sprintf("%s|%d|%d", src.Path, fi.Size(), fi.ModTime().UnixNano())
	c.version = ffmpegBuildID(opts.FFmpeg)
	return c
}

var (
	ffmpegBuildOnce sync.Once
	ffmpegBuild     string
)

// ffmpegBuildID is the first line of `ffmpeg -version` - the build the points
// were measured with; a different binary is a different cache.
func ffmpegBuildID(bin string) string {
	ffmpegBuildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "ladder_bench.version"}, bin, "-version")
		if err == nil {
			ffmpegBuild = strings.SplitN(string(out), "\n", 2)[0]
		}
	})
	return ffmpegBuild
}

func (c *benchCache) key(start float64, height int, e BenchEncoder, target int64) string {
	h := sha256.New()
	fmt.Fprintf(h, "p%d|%s|%.3f|%.3f|%d|%s|%s|%s|%s|%s|%s|%s|%d|%s",
		benchCacheProtocol, c.ident, start, c.opts.WindowSec, height, benchPreFilter(c.src, height),
		e.Name, e.Codec, e.Accel, e.Preset, strings.Join(e.Features, ","), e.Device, target, c.version)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func (c *benchCache) path(k string) string { return filepath.Join(c.dir, k+".json") }

func (c *benchCache) get(k string) (EffPoint, bool) {
	if c.dir == "" {
		return EffPoint{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, err := os.ReadFile(c.path(k))
	if err != nil {
		return EffPoint{}, false
	}
	var p benchCachedPoint
	if json.Unmarshal(raw, &p) != nil || p.Point.ActualKbps <= 0 {
		return EffPoint{}, false
	}
	return p.Point, true
}

func (c *benchCache) put(k string, pt EffPoint) {
	if c.dir == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, err := json.Marshal(benchCachedPoint{Point: pt, Measured: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return
	}
	_ = os.WriteFile(c.path(k), raw, 0o644) //nolint:errcheck // a point that fails to persist is simply measured again next time
}

func (c *benchCache) reference(start float64, height int) (EffPoint, bool) {
	return c.get(c.key(start, height, BenchEncoder{Name: refEncoderName, Codec: "h264", Preset: "veryfast"}, 0))
}

func (c *benchCache) putReference(start float64, height int, pt EffPoint) {
	c.put(c.key(start, height, BenchEncoder{Name: refEncoderName, Codec: "h264", Preset: "veryfast"}, 0), pt)
}

func (c *benchCache) point(start float64, height int, e BenchEncoder, target int64) (EffPoint, bool) {
	return c.get(c.key(start, height, e, target))
}

func (c *benchCache) putPoint(start float64, height int, e BenchEncoder, target int64, pt EffPoint) {
	c.put(c.key(start, height, e, target), pt)
}
