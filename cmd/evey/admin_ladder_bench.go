package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
)

// adminLadderBench is the ladder engine's measuring instrument (plan P1): it
// produces the corpus rows the complexity and quality models are calibrated
// on. It only reads the titles and writes to --out; nothing touches the
// library or the caches.
func adminLadderBench(args []string) error {
	fset := flag.NewFlagSet("ladder-bench", flag.ExitOnError)
	scenes := fset.Int("scenes", 3, "scenes sampled per title")
	window := fset.Float64("window", 8, "seconds per scene")
	heights := fset.String("heights", "1080,720", "encode heights, comma-separated (a height above the source is skipped)")
	crfs := fset.String("crf", "18,23,28", "x264 CRFs to encode at")
	preset := fset.String("preset", "veryfast", "x264 preset (the production one)")
	out := fset.String("out", "", "CSV file for the rows (default: stdout)")
	refHeight := fset.Int("ref-height", 1080, "height the correlations are computed at")
	refCRF := fset.Int("ref-crf", 23, "CRF the correlations are computed at")
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
