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
//
// The ratio is read on the walks alone (WalkReadBytes, mkvgo 0.58). Since
// mkvgo 0.57 a local file's walk keeps nothing and its segments are copied
// from the file again, out of the page cache the walk just filled: the total
// SourceReadBytes counts that copy too and read 3.35 times what a test file
// served against 1.37 before, while the disk read the same. An MP4 plan has no
// walk (WalkReadBytes 0) and is never named.

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

// servedWatch is the clock that reads a plan's counters while it serves: it
// counts what Evey handed to clients from the plan's resources other than its
// playlists (media segments, inits, subtitle windows), and the ratio itself is
// read on mkvgo's own count of what the plan served (planServed).
type servedWatch struct {
	bytes     atomic.Int64
	resources atomic.Int64
	// flagged is set once the plan has been named while still in use; its
	// final figures are logged again when it leaves the cache.
	flagged atomic.Bool
	// damaged is set once the plan has named the damaged regions it passed over.
	damaged atomic.Bool
	// counted is set the first time WindowStats counts the plan over the bar,
	// see countedOver.
	counted atomic.Bool
}

// countedOver latches the plan's place in "plans_read_over_ratio": a plan
// counted once stays counted, though its ratio may fall back under the bar as
// it serves more. WindowStats promises counters that only grow, and the load
// test diffs two snapshots of them: a live state there read negative.
func (w *servedWatch) countedOver(nowOver bool) bool {
	if nowOver || w.flagged.Load() {
		w.counted.Store(true)
	}
	return w.counted.Load()
}

// planReport is what the watch hands its callers: the plan's counters, and
// how many resources Evey handed out from it.
type planReport func(st mp4.HLSPlanStats, resources int64)

// count adds n bytes handed out and, each time the total crosses a multiple
// of readRatioMinServed, reads the plan's counters (stats): report is called
// the first time the plan reads over the bar, damaged the first time it has
// passed over a damaged region of its source.
func (w *servedWatch) count(n int64, stats func() mp4.HLSPlanStats, report, damaged planReport) {
	resources := w.resources.Add(1)
	after := w.bytes.Add(n)
	if n <= 0 || (after-n)/readRatioMinServed == after/readRatioMinServed {
		return
	}
	st := stats()
	if overRead(st) && w.flagged.CompareAndSwap(false, true) {
		report(st, resources)
	}
	if st.DamageSkips+st.DamageRefusals > 0 && w.damaged.CompareAndSwap(false, true) {
		damaged(st, resources)
	}
}

// final reads the counters of a plan leaving the cache and calls report when
// it read over the bar, or had been named while in use, with its last figures;
// damaged when it met a damaged region not named yet.
func (w *servedWatch) final(stats func() mp4.HLSPlanStats, report, damaged planReport) {
	if w.bytes.Load() == 0 {
		return
	}
	st := stats()
	if overRead(st) || w.flagged.Load() {
		report(st, w.resources.Load())
	}
	if st.DamageSkips+st.DamageRefusals > 0 && w.damaged.CompareAndSwap(false, true) {
		damaged(st, w.resources.Load())
	}
}

// planServed is what the plan served through Open, by mkvgo's count (0.55):
// segments handed out whole by Resource, and everything Open handed out
// beside them, streamed or kept.
func planServed(st mp4.HLSPlanStats) int64 { return st.ServedBytes + st.StreamedBytes }

// overRead reports whether a plan that served at least readRatioMinServed
// walked more than readRatioFlag times what it served from its source.
func overRead(st mp4.HLSPlanStats) bool {
	served := planServed(st)
	return served >= readRatioMinServed && st.WalkReadBytes > readRatioFlag*served
}

// logOverRead names the file and what mkvgo needs to take the case up: the
// bytes read and served, the lookups in the uncued regions, the segments
// served from a kept window and the tables that fell back to the source, and
// the windows built. final says the plan has left the cache.
func logOverRead(logger *slog.Logger, path string, st mp4.HLSPlanStats, resources int64, final bool) {
	served := planServed(st)
	logger.Warn("mkvgo: plan read its source far more than it served",
		slog.String("path", path),
		slog.Int64("walk_read_bytes", st.WalkReadBytes),
		slog.Int64("copy_read_bytes", st.CopyReadBytes),
		slog.Int64("served_bytes", served),
		slog.Int64("read_ratio_pct", st.WalkReadBytes*100/max(served, 1)),
		slog.Int64("resources_served", resources),
		slog.Int64("segments_opened", st.ServedSegments),
		slog.Int64("window_builds", st.WindowBuilds),
		slog.Int64("window_rebuilds", st.WindowRebuilds),
		slog.Int64("far_lookups", st.FarLookups),
		slog.Int64("far_hits", st.FarHits),
		slog.Int64("kept_segments", st.KeptSegments),
		slog.Int64("kept_fallbacks", st.KeptFallbacks),
		slog.Int64("streamed_segments", st.StreamedSegments),
		slog.Bool("final", final))
}

// logDamaged names a file whose plan met a damaged region (zeroed bytes, an
// element that does not decode): passed over, the stretch it held played as a
// still image; refused, the windows inside it are not served. Either way the
// file wants replacing or `mkvgo reindex --resync`.
func logDamaged(logger *slog.Logger, path string, st mp4.HLSPlanStats) {
	logger.Warn("mkvgo: plan met a damaged region of its source, replace or repair the file",
		slog.String("path", path),
		slog.Int64("damage_skips", st.DamageSkips),
		slog.Int64("damage_refusals", st.DamageRefusals))
}

// served counts n bytes handed out (see servedWatch.count).
func (p *ondemandPlan) served(n int64) {
	p.watch.count(n, p.Stats, func(st mp4.HLSPlanStats, resources int64) {
		logOverRead(p.logger(), p.srcPath, st, resources, false)
	}, p.logDamaged)
}

// retireReadRatio logs a plan leaving the cache (see servedWatch.final).
func (p *ondemandPlan) retireReadRatio() {
	p.watch.final(p.Stats, func(st mp4.HLSPlanStats, resources int64) {
		logOverRead(p.logger(), p.srcPath, st, resources, true)
	}, p.logDamaged)
}

func (p *ondemandPlan) logDamaged(st mp4.HLSPlanStats, _ int64) {
	logDamaged(p.logger(), p.srcPath, st)
}

func (p *ondemandPlan) logger() *slog.Logger {
	if p.pkg != nil && p.pkg.logger != nil {
		return p.pkg.logger
	}
	return slog.Default()
}

// servedStats is what a plan adds to WindowStats beside mkvgo's counters: what
// it served, and whether it read over the bar.
func (p *ondemandPlan) servedStats() (served int64, overBar bool) {
	st := p.Stats()
	return planServed(st), p.watch.countedOver(overRead(st))
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
