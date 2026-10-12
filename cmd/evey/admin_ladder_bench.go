package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
)

// adminLadderBench is the ladder engine's measuring instrument (plan P1): it
// produces the corpus rows the complexity and quality models are calibrated
// on. It only reads the titles and writes to --out; nothing touches the
// library or the caches.
func adminLadderBench(args []string) error {
	fset := adminFlags("ladder-bench")
	scenes := fset.Int("scenes", 3, "scenes sampled per title")
	window := fset.Float64("window", 8, "seconds per scene")
	heights := fset.String("heights", "1080,720", "encode heights, comma-separated (a height above the source is skipped)")
	crfs := fset.String("crf", "18,23,28", "x264 CRFs to encode at")
	preset := fset.String("preset", "veryfast", "x264 preset (the production one)")
	out := fset.String("out", "", "CSV file for the rows (default: stdout)")
	refHeight := fset.Int("ref-height", 1080, "height the correlations are computed at")
	refCRF := fset.Int("ref-crf", 23, "CRF the correlations are computed at")
	efficiency := fset.Bool("efficiency", false, "codec/encoder efficiency mode: fixed-bitrate curves per production encoder against the x264 CRF 23 reference")
	mode := fset.String("mode", "fast", "efficiency mode: full (3 scenes, 5 points, the scientific reference), fast (2 scenes, bracketed points, CPU and GPU lanes in parallel) or incremental (fast, and only the points the cache does not hold)")
	cacheDir := fset.String("cache", "", "efficiency mode: point cache directory (default <tmp>/ladder-bench-cache; incremental mode requires it)")
	encoderList := fset.String("encoders", "", "efficiency mode: comma-separated encoder names to measure (default: every production encoder this host runs)")
	device := fset.String("device", "/dev/dri/renderD128", "efficiency mode: VAAPI/QSV render node")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() == 0 {
		return fmt.Errorf("ladder-bench: at least one media file is required")
	}
	opts := ffmpeg.DefaultBenchOptions()
	opts.Scenes, opts.WindowSec, opts.Preset = *scenes, *window, *preset
	var err error
	if opts.Heights, err = parseIntList(*heights); err != nil {
		return fmt.Errorf("ladder-bench: --heights: %w", err)
	}
	if opts.CRFs, err = parseIntList(*crfs); err != nil {
		return fmt.Errorf("ladder-bench: --crf: %w", err)
	}
	opts.Progress = func(line string) { fmt.Fprintln(os.Stderr, "  "+line) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *efficiency {
		switch *mode {
		case "full":
			opts.Scenes, opts.Adaptive, opts.Parallel = 3, false, false
		case "fast", "incremental":
			opts.Scenes, opts.Adaptive, opts.Parallel = 2, true, true
		default:
			return fmt.Errorf("ladder-bench: --mode must be full, fast or incremental")
		}
		opts.CacheDir = *cacheDir
		if opts.CacheDir == "" {
			opts.CacheDir = filepath.Join(os.TempDir(), "ladder-bench-cache")
		}
		fmt.Fprintf(os.Stderr, "mode %s: %d scenes, %s points, %s lanes, cache %s\n", *mode, opts.Scenes,
			map[bool]string{true: "bracketed", false: "all five"}[opts.Adaptive], map[bool]string{true: "cpu+gpu", false: "one"}[opts.Parallel], opts.CacheDir)
		return runEfficiencyBench(ctx, fset.Args(), opts, *encoderList, *device, *out)
	}

	var results []*ffmpeg.BenchResult
	for _, path := range fset.Args() {
		fmt.Fprintf(os.Stderr, "→ %s\n", path)
		r, err := ffmpeg.RunLadderBench(ctx, path, opts)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "  skipped: %v\n", err)
			continue
		}
		results = append(results, r)
	}
	if len(results) == 0 {
		return fmt.Errorf("ladder-bench: nothing measured")
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	cw := csv.NewWriter(w)
	header, rows := ffmpeg.BenchRows(results)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}

	for _, perTitle := range []bool{false, true} {
		corr := ffmpeg.FeatureCorrelations(results, *refHeight, *refCRF, perTitle)
		if len(corr) == 0 {
			fmt.Fprintf(os.Stderr, "\n(no correlation: fewer than 3 samples at %dp crf %d)\n", *refHeight, *refCRF)
			continue
		}
		fmt.Fprintf(os.Stderr, "\nSpearman rank correlation at %s - |rho| > 0.5 is worth a rule, < 0.3 is noise:\n", corr[0].Reference)
		fmt.Fprintf(os.Stderr, "  %-12s %9s %9s\n", "feature", "vs bpp", "vs xpsnr")
		for _, c := range corr {
			fmt.Fprintf(os.Stderr, "  %-12s %+9.2f %+9.2f\n", c.Feature, c.VsBPP, c.VsXPSNR)
		}
	}
	return nil
}

func parseIntList(s string) ([]int, error) {
	var out []int
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%q is not a positive integer", part)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty list")
	}
	return out, nil
}

// runEfficiencyBench is the P2 calibration: which production encoders this
// host runs, their bitrate/XPSNR curves on every scene against the x264
// reference, the CSV, and the per-encoder summary the CodecEfficiencyModel is
// calibrated from.
func runEfficiencyBench(ctx context.Context, paths []string, opts ffmpeg.BenchOptions, encoderList, device, out string) error {
	wanted := map[string]bool{}
	for part := range strings.SplitSeq(encoderList, ",") {
		if p := strings.TrimSpace(part); p != "" {
			wanted[p] = true
		}
	}
	var encoders []ffmpeg.BenchEncoder
	var skipped []string
	for _, e := range ffmpeg.BenchEncoders(device) {
		if len(wanted) > 0 && !wanted[e.Name] {
			continue
		}
		if ffmpeg.BenchEncoderAvailable(ctx, opts, e) {
			encoders = append(encoders, e)
		} else {
			skipped = append(skipped, e.Name)
		}
	}
	fmt.Fprintf(os.Stderr, "encoders on this host: %s\n", strings.Join(encoderNames(encoders), ", "))
	if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr, "not on this host (skipped): %s\n", strings.Join(skipped, ", "))
	}
	if len(encoders) == 0 {
		return fmt.Errorf("ladder-bench: no encoder to measure")
	}

	// The CSV grows title by title: hours of encodes are not lost to an
	// interruption, and a run can be read while it is still going.
	w := os.Stdout
	if out != "" {
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	cw := csv.NewWriter(w)
	header, _ := ffmpeg.EffRows(nil)
	if err := cw.Write(header); err != nil {
		return err
	}
	cw.Flush()
	var results []*ffmpeg.EffResult
	for _, path := range paths {
		fmt.Fprintf(os.Stderr, "→ %s\n", path)
		r, err := ffmpeg.RunEfficiencyBench(ctx, path, opts, encoders)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "  skipped: %v\n", err)
			continue
		}
		results = append(results, r)
		_, rows := ffmpeg.EffRows([]*ffmpeg.EffResult{r})
		for _, row := range rows {
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
	}
	if len(results) == 0 {
		return fmt.Errorf("ladder-bench: nothing measured")
	}

	fmt.Fprintf(os.Stderr, "\nEfficiency = bitrate the encoder needs to match the x264 CRF 23 reference's XPSNR, over the reference's bitrate (< 1: cheaper).\n")
	fmt.Fprintf(os.Stderr, "  %-11s %5s %6s %6s %4s %7s %8s %8s %8s %8s %8s %8s %8s\n", "encoder", "rung", "scenes", "titles", "oor", "factor", "dispP50", "dispP90", "dispMax", "looP50", "looP90", "looMax", "fps@1.0")
	for _, s := range ffmpeg.EffSummaries(results) {
		fmt.Fprintf(os.Stderr, "  %-11s %4dp %6d %6d %4d %7.3f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.0f\n",
			s.Encoder, s.Height, s.Scenes, s.Titles, s.OutOfRange, s.MedianFactor, s.DispP50, s.DispP90, s.DispWorst, s.LOOP50, s.LOOP90, s.LOOWorst, s.MedianFPS)
	}
	return nil
}

func encoderNames(es []ffmpeg.BenchEncoder) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}
