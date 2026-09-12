package ffmpeg

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// title_probe.go - the per-title probe (plan P3). One scene, one point on the
// CPU reference at the static policy's CRF; another scene only while the
// prediction is out of its margin; a second CRF point on the first scene only
// when the cost is still out after the scenes, to record the title's own
// fidelity slope. Hard cap 3 scenes x 2 CRF. Every ffmpeg runs at scan
// priority (nice, ionice idle, bounded threads) and is SIGSTOPped while a
// playback is being served; every point lands in the bench cache under the
// bench's own key, so a probe and a calibration never measure the same point
// twice.

// ProbeOptions drive one probe.
type ProbeOptions struct {
	// Model and Cost are the cost model and its prediction for this title:
	// the probe confirms when the measured cost stays within Model.CeilingMargin
	// of the model's price for the scene it encodes.
	Model     transcoding.CostModel
	Cost      transcoding.TitleCost
	Height    int
	CRF       int
	SecondCRF int
	WindowSec float64
	// Busy pauses the running ffmpeg while it reports true.
	Busy     func() bool
	CacheDir string
	WorkDir  string
	FFmpeg   string
	FFprobe  string
	Progress func(string)
}

// DefaultProbeOptions: the reference height and the static CRF, a CRF +5
// second point, 8 s scenes.
func DefaultProbeOptions() ProbeOptions {
	return ProbeOptions{Model: transcoding.CostModelV2, Height: transcoding.CostModelV2.RefHeight, CRF: transcoding.StaticSoftwareCRF, SecondCRF: transcoding.StaticSoftwareCRF + 5,
		WindowSec: 8, FFmpeg: "ffmpeg", FFprobe: "ffprobe"}
}

// ProbeReport is what a probe measured, with its cost.
type ProbeReport struct {
	Probe     transcoding.TitleProbe
	Points    int
	CacheHits int
	Wall      time.Duration
	Source    BenchSource
}

// ProbeTitle runs the protocol on one title.
func ProbeTitle(ctx context.Context, path string, opts ProbeOptions) (ProbeReport, error) {
	t0 := time.Now()
	benchOpts := DefaultBenchOptions()
	benchOpts.WindowSec, benchOpts.FFmpeg, benchOpts.FFprobe, benchOpts.WorkDir, benchOpts.CacheDir = opts.WindowSec, opts.FFmpeg, opts.FFprobe, opts.WorkDir, opts.CacheDir
	src, err := benchProbe(ctx, path, benchOpts)
	if err != nil {
		return ProbeReport{}, err
	}
	if src.DurationSec < opts.WindowSec*2 {
		return ProbeReport{}, fmt.Errorf("title probe: %s is too short (%.0f s)", path, src.DurationSec)
	}
	// The model's mean price for the scene the probe encodes (probe height,
	// probe CRF, over the frame really encoded): what confirmation is judged
	// against.
	predicted := float64(opts.Model.RungKbps(opts.Cost, benchRungWidth(opts.Height), framedHeight(src, opts.Height), src.FPS, opts.CRF))
	margin := opts.Model.CeilingMargin
	rep := ProbeReport{Source: src}
	rep.Probe = transcoding.TitleProbe{Protocol: transcoding.TitleProbeProtocol, ModelVersion: opts.Model.Version, FFmpeg: ffmpegBuildID(opts.FFmpeg),
		Encoder: transcoding.TitleProbeEncoderID, Height: opts.Height, PredictedKbps: predicted, MeasuredAt: time.Now().UTC()}
	cache := newBenchCache(opts.CacheDir, src, benchOpts)
	starts := benchSceneStarts(src.DurationSec, transcoding.TitleProbeMaxScenes, opts.WindowSec)
	var mezz *mezzanine
	measure := func(start float64, crf int) (transcoding.ProbePoint, error) {
		enc := BenchEncoder{Name: fmt.Sprintf("ref-x264-crf%d", crf), Codec: "h264", Preset: "veryfast"}
		if pt, ok := cache.point(start, opts.Height, enc, 0); ok {
			rep.CacheHits++
			return transcoding.ProbePoint{StartSec: start, CRF: crf, Kbps: pt.ActualKbps, XPSNRY: pt.XPSNRY}, nil
		}
		if mezz == nil || mezz.path == "" || int(mezz.startSec) != int(start) {
			if mezz != nil && mezz.path != "" {
				os.Remove(mezz.path) //nolint:errcheck // scratch file
			}
			if opts.Progress != nil {
				opts.Progress(fmt.Sprintf("mezzanine %s @ %.0fs", filepath.Base(path), start))
			}
			m, err := probeMezzanine(ctx, src, start, opts, benchOpts)
			if err != nil {
				return transcoding.ProbePoint{}, err
			}
			mezz = &m
		}
		if opts.Progress != nil {
			opts.Progress(fmt.Sprintf("encode %s @ %.0fs crf %d", filepath.Base(path), start, crf))
		}
		pt, err := probeEncode(ctx, *mezz, crf, opts, benchOpts)
		if err != nil {
			return transcoding.ProbePoint{}, err
		}
		rep.Points++
		cache.putPoint(start, opts.Height, enc, 0, pt)
		return transcoding.ProbePoint{StartSec: start, CRF: crf, Kbps: pt.ActualKbps, XPSNRY: pt.XPSNRY}, nil
	}
	defer func() {
		if mezz != nil && mezz.path != "" {
			os.Remove(mezz.path) //nolint:errcheck // scratch file
		}
	}()

	// Scenes, while the prediction is out of its margin.
	var costs []float64
	for i, start := range starts {
		pt, err := measure(start, opts.CRF)
		if err != nil {
			return rep, err
		}
		rep.Probe.Points = append(rep.Probe.Points, pt)
		costs = append(costs, pt.Kbps)
		rep.Probe.MeasuredKbps = mean(costs)
		if predicted > 0 {
			rep.Probe.Ratio = rep.Probe.MeasuredKbps / predicted
			rep.Probe.Confirmed = math.Abs(math.Log(rep.Probe.Ratio)) <= math.Log(margin)
		}
		if rep.Probe.Confirmed || i == transcoding.TitleProbeMaxScenes-1 {
			break
		}
	}
	rep.Probe.Spread = 1
	if len(costs) > 1 {
		lo, hi := costs[0], costs[0]
		for _, c := range costs {
			lo, hi = math.Min(lo, c), math.Max(hi, c)
		}
		if lo > 0 {
			rep.Probe.Spread = hi / lo
		}
	}
	// Bits per pixel per frame over the frame really encoded (the model's unit).
	if w, h := benchRungWidth(opts.Height), framedHeight(src, opts.Height); src.FPS > 0 && w > 0 && h > 0 {
		rep.Probe.MeasuredBPP = rep.Probe.MeasuredKbps * 1000 / (float64(w) * float64(h) * src.FPS)
	}
	// A second CRF point on the first scene, only when the cost is still out
	// of its margin: the title's own fidelity slope, for the quality policy.
	if !rep.Probe.Confirmed && opts.SecondCRF > 0 && opts.SecondCRF != opts.CRF && transcoding.TitleProbeMaxCRFPoints > 1 {
		pt, err := measure(starts[0], opts.SecondCRF)
		if err != nil {
			return rep, err
		}
		rep.Probe.Points = append(rep.Probe.Points, pt)
	}
	rep.Wall = time.Since(t0)
	return rep, nil
}

func framedHeight(src BenchSource, height int) int {
	if src.Width <= 0 || src.Height <= 0 {
		return height
	}
	return int(math.Round(float64(benchRungWidth(height))*float64(src.Height)/float64(src.Width))) / 2 * 2
}

// probeMezzanine is the bench's mezzanine at scan priority, paused while busy.
func probeMezzanine(ctx context.Context, src BenchSource, start float64, opts ProbeOptions, benchOpts BenchOptions) (mezzanine, error) {
	dir := opts.WorkDir
	if dir == "" {
		dir = os.TempDir()
	}
	fps := src.FPS
	if fps <= 0 {
		fps = 24
	}
	m := mezzanine{path: filepath.Join(dir, fmt.Sprintf("title-probe-%d-%d.yuv", os.Getpid(), int(start))), width: benchRungWidth(opts.Height), height: framedHeight(src, opts.Height), fps: fps, frames: int(math.Round(opts.WindowSec * fps)), startSec: start}
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-ss", fmt.Sprintf("%.3f", start), "-i", src.Path, "-map", "0:v:0",
		"-vf", benchPreFilter(src, opts.Height), "-frames:v", strconv.Itoa(m.frames), "-f", "rawvideo", m.path}
	if err := runScanPriority(ctx, opts, "title_probe.mezzanine", args); err != nil {
		return mezzanine{}, fmt.Errorf("title probe: mezzanine @ %.0fs: %w", start, err)
	}
	return m, nil
}

// probeEncode is the reference encode at crf, scored against the mezzanine.
func probeEncode(ctx context.Context, m mezzanine, crf int, opts ProbeOptions, benchOpts BenchOptions) (EffPoint, error) {
	out := filepath.Join(filepath.Dir(m.path), fmt.Sprintf("title-probe-%d-%d-%d.mp4", os.Getpid(), int(m.startSec), crf))
	defer os.Remove(out) //nolint:errcheck // scratch file
	enc := resolveVideoEncoder("h264")
	args := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y"}, m.inputArgs()...)
	args = append(args, "-c:v", enc, "-preset", "veryfast", "-crf", strconv.Itoa(crf))
	args = append(args, softwareX264TuningArgs(enc, -1)...)
	args = append(args, "-pix_fmt", "yuv420p")
	args = append(args, hlsForceKeyframesArgs...)
	args = append(args, "-an", "-f", "mp4", out)
	if err := runScanPriority(ctx, opts, "title_probe.encode", args); err != nil {
		return EffPoint{}, fmt.Errorf("title probe: encode crf %d: %w", crf, err)
	}
	fi, err := os.Stat(out)
	if err != nil {
		return EffPoint{}, err
	}
	pt := EffPoint{ActualKbps: float64(fi.Size()) * 8 / opts.WindowSec / 1000}
	stats := out + ".xpsnr"
	defer os.Remove(stats) //nolint:errcheck // scratch file
	q := append([]string{"-hide_banner", "-v", "error", "-nostdin"}, m.inputArgs()...)
	q = append(q, "-i", out, "-lavfi", fmt.Sprintf("[1:v]format=yuv420p[e];[e][0:v]xpsnr=stats_file=%s", stats), "-f", "null", "-")
	if err := runScanPriority(ctx, opts, "title_probe.xpsnr", q); err != nil {
		return EffPoint{}, fmt.Errorf("title probe: xpsnr crf %d: %w", crf, err)
	}
	raw, rerr := os.ReadFile(stats)
	if rerr != nil {
		return EffPoint{}, rerr
	}
	pt.XPSNRY = parseXPSNRMeanY(string(raw))
	return pt, nil
}

// runScanPriority runs ffmpeg as background work: the scan class (nice 19,
// ionice idle, bounded threads, memory cap) and SIGSTOP while Busy says a
// playback is being served.
func runScanPriority(ctx context.Context, opts ProbeOptions, site string, args []string) error {
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Scan, Site: site}, opts.FFmpeg, args...)
	if err != nil {
		return err
	}
	stderr := newRingBuffer(4 * 1024)
	cmd.Stderr = stderr
	runErr := runPausableCmd(cmd, opts.Busy)
	if ferr := finish(runErr, []byte(stderr.String())); ferr != nil {
		return ferr
	}
	if runErr != nil {
		return fmt.Errorf("%w: %s", runErr, tailString(stderr.String(), 400))
	}
	return nil
}

// TitleProberAdapter adapts ProbeTitle to the app-layer TitleProber: it holds
// the shared point cache, the busy predicate (SIGSTOP under playback) and the
// scratch dir, so the worker passes only the source and the model prediction.
type TitleProberAdapter struct {
	cacheDir string
	workDir  string
	busy     func() bool
}

// NewTitleProber builds the adapter. cacheDir is the shared bench/probe point
// cache (resume for free); busy pauses every probe ffmpeg while it reports a
// live playback.
func NewTitleProber(cacheDir, workDir string, busy func() bool) *TitleProberAdapter {
	return &TitleProberAdapter{cacheDir: cacheDir, workDir: workDir, busy: busy}
}

// Probe runs the P3 protocol for one title against the model's prediction.
// Probe runs the P3 protocol; the returned ProbeReport carries the points,
// cache hits and wall time (the wire layer maps them onto the app interface).
func (a *TitleProberAdapter) Probe(ctx context.Context, sourcePath string, cost transcoding.TitleCost) (ProbeReport, error) {
	opts := DefaultProbeOptions()
	opts.Cost = cost
	opts.CacheDir, opts.WorkDir, opts.Busy = a.cacheDir, a.workDir, a.busy
	return ProbeTitle(ctx, sourcePath, opts)
}
