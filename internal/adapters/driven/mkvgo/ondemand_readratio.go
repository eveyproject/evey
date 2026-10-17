package mkvgo

// ondemand_readratio.go - how much of its source a plan read for what it
// served, and a log line for the files that read far more.
//
// A plan normally reads its source a little more than once for what goes out
// (the window kept for serving, mkvgo 0.54). A file laid out against the
// packager's walk reads several times that: the 2026-10-16 case, a UHD episode
// whose audio sits apart from the video past 240 s, read 2.96 times what it
// served until mkvgo looked for the audio in the uncued regions. Such a file
// shows nowhere but in the disk's load, so each plan says when its ratio
// crosses readRatioFlag, with what mkvgo needs to take the case up. Nothing is
// done to the plan.

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"

	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

const (
	// readRatioFlag is the source read, as a multiple of what the plan served,
	// past which the file is named in the log.
	readRatioFlag = 3
	// readRatioMinServed is what a plan must have served before its ratio is
	// read: the first windows carry the head and the cold start, and a plan
	// that served a few segments says nothing about the file. The ratio is
	// read again each time the plan serves this much more.
	readRatioMinServed = 64 << 20
)

// servedWatch counts what Evey handed to clients from a plan's resources other
// than its playlists: media segments, inits, subtitle windows.
type servedWatch struct {
	bytes     atomic.Int64
	resources atomic.Int64
	// flagged is set once the plan has been named while still in use; its
	// final figures are logged again when it leaves the cache.
	flagged atomic.Bool
}

// count adds n bytes handed out and, each time the total crosses a multiple
// of readRatioMinServed, reads the plan's counters (stats) and calls report
// the first time the plan reads over the bar.
func (w *servedWatch) count(n int64, stats func() mp4.HLSPlanStats, report func(st mp4.HLSPlanStats, served, resources int64)) {
	resources := w.resources.Add(1)
	after := w.bytes.Add(n)
	if n <= 0 || (after-n)/readRatioMinServed == after/readRatioMinServed {
		return
	}
	if st := stats(); overRead(st, after) && w.flagged.CompareAndSwap(false, true) {
		report(st, after, resources)
	}
}

// final reads the counters of a plan leaving the cache and calls report when
// it read over the bar, or had been named while in use, with its last figures.
func (w *servedWatch) final(stats func() mp4.HLSPlanStats, report func(st mp4.HLSPlanStats, served, resources int64)) {
	served := w.bytes.Load()
	if served == 0 {
		return
	}
	if st := stats(); overRead(st, served) || w.flagged.Load() {
		report(st, served, w.resources.Load())
	}
}

// overRead reports whether a plan that served served bytes read more than
// readRatioFlag times that from its source.
func overRead(st mp4.HLSPlanStats, served int64) bool {
	return served >= readRatioMinServed && st.SourceReadBytes > readRatioFlag*served
}

// logOverRead names the file and what mkvgo needs to take the case up: the
// bytes read and served, the lookups in the uncued regions, the segments
// served from a kept window and the tables that fell back to the source, and
// the windows built. final says the plan has left the cache.
func logOverRead(logger *slog.Logger, path string, st mp4.HLSPlanStats, served, resources int64, final bool) {
	logger.Warn("mkvgo: plan read its source far more than it served",
		slog.String("path", path),
		slog.Int64("read_bytes", st.SourceReadBytes),
		slog.Int64("served_bytes", served),
		slog.Int64("read_ratio_pct", st.SourceReadBytes*100/served),
		slog.Int64("resources_served", resources),
		// Every handle Open returns counts in exactly one of the three.
		slog.Int64("segments_opened", st.StreamedSegments+st.KeptSegments+st.StreamFallbacks),
		slog.Int64("window_builds", st.WindowBuilds),
		slog.Int64("window_rebuilds", st.WindowRebuilds),
		slog.Int64("far_lookups", st.FarLookups),
		slog.Int64("far_hits", st.FarHits),
		slog.Int64("kept_segments", st.KeptSegments),
		slog.Int64("kept_fallbacks", st.KeptFallbacks),
		slog.Int64("streamed_segments", st.StreamedSegments),
		slog.Bool("final", final))
}

// served counts n bytes handed out (see servedWatch.count).
func (p *ondemandPlan) served(n int64) {
	p.watch.count(n, p.Stats, func(st mp4.HLSPlanStats, served, resources int64) {
		logOverRead(p.logger(), p.srcPath, st, served, resources, false)
	})
}

// retireReadRatio logs a plan leaving the cache (see servedWatch.final).
func (p *ondemandPlan) retireReadRatio() {
	p.watch.final(p.Stats, func(st mp4.HLSPlanStats, served, resources int64) {
		logOverRead(p.logger(), p.srcPath, st, served, resources, true)
	})
}

func (p *ondemandPlan) logger() *slog.Logger {
	if p.pkg != nil && p.pkg.logger != nil {
		return p.pkg.logger
	}
	return slog.Default()
}

// servedStats is what a plan adds to WindowStats beside mkvgo's counters: the
// bytes Evey handed out from it, and whether it read over the bar.
func (p *ondemandPlan) servedStats() (served int64, overBar bool) {
	served = p.watch.bytes.Load()
	return served, overRead(p.Stats(), served)
}

// countedResource counts what a streamed resource wrote to its client.
type countedResource struct {
	apptranscoding.OnDemandResource
	plan *ondemandPlan
}

func (c *countedResource) WriteRange(ctx context.Context, w io.Writer, off, n int64) (int64, error) {
	written, err := c.OnDemandResource.WriteRange(ctx, w, off, n)
	c.plan.served(written)
	return written, err
}
