package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// adminRouteBench measures what a session of each transcode route costs on
// this host (throughput, CPU, GPU) against what the calibration and the
// admission cost model believe it costs. It reads clips and writes rows;
// nothing it does touches the library, the caches or the persisted capability.
func adminRouteBench(args []string) error {
	fset := adminFlags("route-bench")
	backend := fset.String("backend", "", "accelerator to measure: nvenc, vaapi, qsv (default: auto-detect), none = CPU routes only")
	device := fset.String("device", "", "VAAPI/QSV render node (default: the calibration's)")
	heights := fset.String("heights", "1080,2160", "rungs measured, comma-separated")
	fpsList := fset.String("fps", "24,30,50,60", "source frame rates the synthetic clips are generated at")
	routes := fset.String("routes", "", "comma-separated route names to measure (default: all this host serves)")
	classes := fset.String("classes", "sdr8,hdr10", "synthetic clip classes per fps: sdr8 (2160p HEVC), hdr10 (2160p Main10 PQ), h264 (1080p H.264)")
	duration := fset.Int("duration", 10, "clip length in seconds")
	workdir := fset.String("workdir", "", "directory for the generated clips (default <tmp>/evey-route-bench)")
	capFile := fset.String("capability", "", "JSON of a persisted capability (the admin calibration GET) to price on, instead of running the production calibration here")
	out := fset.String("out", "", "CSV file for the rows (default: stdout)")
	jsonOut := fset.String("json", "", "JSON file for the rows + the capability used")
	if err := fset.Parse(args); err != nil {
		return err
	}
	opts := ffmpeg.RouteBenchOptions{Backend: *backend, Device: *device, DurationSec: *duration, WorkDir: *workdir, Sources: fset.Args()}
	var err error
	if opts.Heights, err = parseIntList(*heights); err != nil {
		return fmt.Errorf("route-bench: --heights: %w", err)
	}
	if opts.FPS, err = parseIntList(*fpsList); err != nil {
		return fmt.Errorf("route-bench: --fps: %w", err)
	}
	if *routes != "" {
		opts.Routes = strings.Split(*routes, ",")
	}
	opts.Classes = strings.Split(*classes, ",")
	if *capFile != "" {
		b, rerr := os.ReadFile(*capFile)
		if rerr != nil {
			return fmt.Errorf("route-bench: --capability: %w", rerr)
		}
		var cap transcoding.EncodeCapability
		if uerr := json.Unmarshal(b, &cap); uerr != nil {
			return fmt.Errorf("route-bench: --capability: %w", uerr)
		}
		opts.Capability = &cap
	}
	opts.Progress = func(line string) { fmt.Fprintln(os.Stderr, "  "+line) }
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rows, cap, err := ffmpeg.RunRouteBench(ctx, opts)
	if err != nil {
		return fmt.Errorf("route-bench: %w", err)
	}
	if *jsonOut != "" {
		b, merr := json.MarshalIndent(struct {
			Capability transcoding.EncodeCapability `json:"capability"`
			Rows       []ffmpeg.RouteBenchRow       `json:"rows"`
		}{cap, rows}, "", "  ")
		if merr != nil {
			return merr
		}
		if werr := os.WriteFile(*jsonOut, b, 0o644); werr != nil {
			return werr
		}
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
	if err := cw.Write([]string{"backend", "route", "class", "source", "src_codec", "src_pix_fmt", "src_height", "src_fps", "height",
		"frames", "wall_s", "fps", "speed_x", "cpu_s", "cpu_cores", "cpu_share", "gpu_util", "gpu_enc", "gpu_dec",
		"bench_rung", "bench_fps", "pred_cost", "actual_cost", "cost_ratio", "pool_cpu_cost", "error", "argv"}); err != nil {
		return err
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
	for _, r := range rows {
		if err := cw.Write([]string{r.Backend, r.Route, r.Class, r.Source, r.SourceCodec, r.SourcePixFmt, strconv.Itoa(r.SourceHeight), f(r.SourceFPS), strconv.Itoa(r.Height),
			strconv.Itoa(r.Frames), f(r.WallSec), f(r.FPS), f(r.SpeedX), f(r.CPUSec), f(r.CPUCores), f(r.CPUShare), f(r.GPUUtil), f(r.GPUEnc), f(r.GPUDec),
			strconv.Itoa(r.BenchRung), f(r.BenchFPS), f(r.PredCost), f(r.ActualCost), f(r.CostRatio), f(r.PoolCPUCost), r.Error, r.Argv}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
