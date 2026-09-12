package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
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
	return fmt.Sprintf("signalstats,scdet,entropy,bitplanenoise=bitplane=%d,blurdetect,blockdetect,siti,metadata=print:file=-", plane)
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
