package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// adminLadderProbe runs the P3 per-title probe on the given files and reports
// the numbers the plan asks for BEFORE any further tuning: probe time per
// title, early-stop rate after one point, mean points, and whether the model
// was confirmed. Reads and encodes only; nothing touches the library.
func adminLadderProbe(args []string) error {
	fset := flag.NewFlagSet("ladder-probe", flag.ExitOnError)
	window := fset.Float64("window", 8, "seconds per scene")
	cacheDir := fset.String("cache", "", "point cache directory (default <tmp>/ladder-bench-cache)")
	outDir := fset.String("out", "", "directory to write each title's probe JSON (optional)")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() == 0 {
		return fmt.Errorf("ladder-probe: at least one media file is required")
	}
	cache := *cacheDir
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "ladder-bench-cache")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	classifier := ffmpeg.NewComplexityClassifier("ffmpeg", "ffprobe")
	m := transcoding.CostModelV1

	type row struct {
		name              string
		ydif              float64
		probe             transcoding.TitleProbe
		points, cacheHits int
		wall              time.Duration
	}
	var rows []row
	for _, path := range fset.Args() {
		fmt.Fprintf(os.Stderr, "→ %s\n", path)
		dur := probeDurationSec(ctx, path)
		if dur <= 0 {
			fmt.Fprintf(os.Stderr, "  skipped: duration unknown\n")
			continue
		}
		metrics, cerr := classifier.Classify(ctx, path, dur, transcoding.KnownSourceFacts{})
		if cerr != nil && metrics.FramesSampled == 0 {
			fmt.Fprintf(os.Stderr, "  skipped: classify: %v\n", cerr)
			continue
		}
		cost, _ := m.Predict(metrics)
		opts := ffmpeg.DefaultProbeOptions()
		opts.Model, opts.Cost = m, cost
		opts.WindowSec, opts.CacheDir = *window, cache
		opts.Progress = func(s string) { fmt.Fprintln(os.Stderr, "  "+s) }
		rep, perr := ffmpeg.ProbeTitle(ctx, path, opts)
		if perr != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "  skipped: probe: %v\n", perr)
			continue
		}
		rows = append(rows, row{filepath.Base(path), metrics.MotionMean, rep.Probe, rep.Points, rep.CacheHits, rep.Wall})
		if *outDir != "" {
			if err := writeProbeJSON(*outDir, filepath.Base(path), rep.Probe); err != nil {
				fmt.Fprintf(os.Stderr, "  probe json: %v\n", err)
			}
		}
	}
	if len(rows) == 0 {
		return fmt.Errorf("ladder-probe: nothing measured")
	}

	fmt.Fprintf(os.Stderr, "\n%-40s %6s %8s %8s %6s %5s %4s %8s %7s\n", "title", "ydif", "pred", "measured", "ratio", "conf", "scn", "pts(hit)", "wall")
	stop1, ptsSum, confN := 0, 0, 0
	var wall time.Duration
	for _, r := range rows {
		p := r.probe
		scenes := p.Scenes(transcoding.StaticSoftwareCRF)
		if p.Confirmed && scenes == 1 {
			stop1++
		}
		if p.Confirmed {
			confN++
		}
		ptsSum += r.points
		wall += r.wall
		fmt.Fprintf(os.Stderr, "%-40s %6.2f %8.0f %8.0f %6.2f %5v %4d %5d(%d) %6.1fs\n",
			trunc(r.name, 40), r.ydif, p.PredictedKbps, p.MeasuredKbps, p.Ratio, p.Confirmed, scenes, r.points, r.cacheHits, r.wall.Seconds())
	}
	n := len(rows)
	fmt.Fprintf(os.Stderr, "\nprobed titles: %d\n", n)
	fmt.Fprintf(os.Stderr, "early-stop after 1 point (confirmed on scene 1): %d/%d (%.0f %%)\n", stop1, n, 100*float64(stop1)/float64(n))
	fmt.Fprintf(os.Stderr, "confirmed (model within margin): %d/%d\n", confN, n)
	fmt.Fprintf(os.Stderr, "points actually encoded: mean %.2f per title\n", float64(ptsSum)/float64(n))
	fmt.Fprintf(os.Stderr, "probe wall: mean %.1f s/title, total %.0f s\n", wall.Seconds()/float64(n), wall.Seconds())
	return nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// probeDurationSec reads the container duration with ffprobe; 0 on failure.
func probeDurationSec(ctx context.Context, path string) float64 {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d, err := ffmpeg.ProbeContainerDurationSec(c, "ffprobe", path)
	if err != nil {
		return 0
	}
	return d
}

func writeProbeJSON(dir, name string, probe transcoding.TitleProbe) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(probe, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".probe.json"), raw, 0o644)
}
