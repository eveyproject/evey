package mkvgo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"strings"
	"time"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	pkgcache "github.com/eveyproject/evey/internal/pkg/cache"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// concat.go - the mkvgo implementation of apptranscoding.ConcatPackager:
// gapless multi-episode HLS via mp4.PlanConcat. Same zero-storage contract as
// ondemand.go's OnDemandPackager, one cache entry per distinct source chain.

const (
	concatPlanTTL     = 30 * time.Minute
	concatPlanMaxSize = 64
	concatCleanup     = 5 * time.Minute
	// A chain refused over its audio delays is refused again for as long as
	// its sources stand: the refusal is remembered for concatRefusalTTL so
	// every episode start of the series does not probe every file again.
	concatRefusalTTL     = 5 * time.Minute
	concatRefusalMaxSize = 64
	// concatPathSep replaces "/" in a part resource's canonical name
	// ("p0/seg00001.m4s") on the wire: the shared HLS route's {file} wildcard
	// is single-segment, so a slash would 404 before reaching the handler.
	// Encoded here, decoded back in Resource below - entirely internal to
	// this adapter, the HTTP layer only strips its own prefix.
	concatPathSep = "__"
)

// ErrConcatAudioShiftMismatch refuses a chain whose sources do not all call
// for the same audio resync: mp4.PlanConcat applies one option set to every
// source, so one shift would be right for some episodes and wrong for the
// others. The caller falls back to per-episode playback, where each file is
// resynced on its own.
var ErrConcatAudioShiftMismatch = errors.New("mkvgo concat: audio start delays differ across the sources")

// ConcatPackager builds and caches mp4.PlanConcat plans. Safe for concurrent use.
type ConcatPackager struct {
	plans    *pkgcache.TTLCache[apptranscoding.ConcatPlan]
	refusals *pkgcache.TTLCache[error]
	stopper  func()
	// audioDelays measures each Matroska audio track's start delay, as the
	// single-episode packager does (ondemand.go), so the chain is served with
	// the same resync as each episode on its own. Injectable for tests;
	// production passes matroska.AudioStartDelays.
	audioDelays delaysFunc
}

// NewConcatPackager returns a packager with an internal plan cache. Call
// Close to stop the cache's background janitor.
func NewConcatPackager() *ConcatPackager {
	c, stop := pkgcache.NewWithMaxSize[apptranscoding.ConcatPlan](concatCleanup, concatPlanMaxSize)
	f, stopF := pkgcache.NewWithMaxSize[error](concatCleanup, concatRefusalMaxSize)
	return &ConcatPackager{
		plans: c, refusals: f,
		stopper: func() { stop(); stopF() },
		audioDelays: func(ctx context.Context, path string) (map[uint64]int64, error) {
			return extmatroska.AudioStartDelays(ctx, path)
		},
	}
}

// Close stops the cache janitors.
func (p *ConcatPackager) Close() {
	if p != nil && p.stopper != nil {
		p.stopper()
	}
}

// Plan returns a cached-or-freshly-built concatenated plan for sources
// (ordered, at least two). It fails - the caller's signal to fall back to
// per-episode playback - when the sources are incompatible (video codec or
// audio layout mismatch) or any one of them can't be packaged verbatim.
func (p *ConcatPackager) Plan(ctx context.Context, sources []string, opts apptranscoding.ConcatOptions) (apptranscoding.ConcatPlan, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ConcatPlan")
	defer span.End()
	if len(sources) < 2 {
		return nil, fmt.Errorf("mkvgo PlanConcat: a chain needs at least two sources (got %d)", len(sources))
	}

	key := concatCacheKey(sources, opts)
	if pl, ok := p.plans.Get(key); ok {
		return pl, nil
	}
	if err, ok := p.refusals.Get(key); ok {
		return nil, err
	}

	// Same hardcoded-on additions as the single-episode packager (ondemand.go):
	// ChapterMarkers is additive/byte-identical on a chapterless part, and
	// SynthesizeIndex lets an un-indexed part join the session instead of
	// refusing the whole chain over one file's missing Cues.
	// SkipUnsupported: same reason as the on-demand packager (ondemand.go) - one
	// TrueHD/LPCM/Vorbis track must not fail the whole chain's plan.
	// NoIFrames: same reason as the on-demand packager (ondemand.go).
	o := mp4.Options{SegmentMs: opts.SegmentMs, ChapterMarkers: true, SynthesizeIndex: true, SkipUnsupported: true, NoIFrames: true}
	// The same audio start-delay resync the single-episode packager applies
	// (ondemand.go): an episode played in a chain must not drift where it
	// played in sync on its own. One option set serves every source, so the
	// chain is planned only when every source calls for the same shift.
	shift, probed, err := concatAudioShift(ctx, sources, p.audioDelays)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, ErrConcatAudioShiftMismatch) {
			p.refusals.Set(key, err, concatRefusalTTL)
		}
		return nil, err
	}
	o.AudioPresentationShift = shift
	if opts.URLPrefix != "" {
		prefix := opts.URLPrefix
		o.RewriteURL = func(name string) string {
			return prefix + strings.ReplaceAll(name, "/", concatPathSep)
		}
	}

	cp, err := mp4.PlanConcat(ctx, sources, o)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("mkvgo PlanConcat (%d sources): %w", len(sources), err)
	}

	pl := &concatPlan{plan: cp}
	// A plan built while a probe failed is served, not kept: the next
	// request probes again rather than serving a part without its resync
	// for the cache's whole life.
	if probed {
		p.plans.Set(key, apptranscoding.ConcatPlan(pl), concatPlanTTL)
	}
	return pl, nil
}

// concatAudioShift probes every Matroska source's audio start delays and
// returns the one presentation shift the chain is served with, nil when no
// source needs one; probed is false when a probe failed. Shifts agree when
// they round to the same millisecond, the grain mkvgo writes in the edit
// list. A probe failure leaves that source without a resync, as the
// single-episode packager does; sources that do not agree refuse the chain
// with ErrConcatAudioShiftMismatch.
func concatAudioShift(ctx context.Context, sources []string, probe delaysFunc) (shift map[uint64]int64, probed bool, err error) {
	probed = true
	if probe == nil {
		return nil, probed, nil
	}
	var first map[uint64]int64
	for k, src := range sources {
		var cur map[uint64]int64
		if IsEBMLContainer(src) {
			delays, perr := probe(ctx, src)
			if perr != nil {
				probed = false
				slog.WarnContext(ctx, "mkvgo concat: audio-delay probe failed, part planned without resync",
					slog.Int("part", k+1), slog.String("path", src), slog.Any("err", perr))
			} else {
				cur = shiftMs(audioPresentationShift(delays))
			}
		}
		if k == 0 {
			first = cur
			shift = audioPresentationShiftFromMs(cur)
			continue
		}
		if !maps.Equal(first, cur) {
			return nil, probed, fmt.Errorf("%w: part %d wants %v, part 1 wants %v (ms)", ErrConcatAudioShiftMismatch, k+1, cur, first)
		}
	}
	return shift, probed, nil
}

// shiftMs rounds a shift map to milliseconds, half away from zero, as mkvgo
// does when it writes the edit list.
func shiftMs(ns map[uint64]int64) map[uint64]int64 {
	if len(ns) == 0 {
		return nil
	}
	ms := make(map[uint64]int64, len(ns))
	for track, v := range ns {
		if v >= 0 {
			ms[track] = (v + 500_000) / 1_000_000
		} else {
			ms[track] = -((-v + 500_000) / 1_000_000)
		}
	}
	return ms
}

// audioPresentationShiftFromMs is the shift mkvgo is handed, in nanoseconds.
func audioPresentationShiftFromMs(ms map[uint64]int64) map[uint64]int64 {
	if len(ms) == 0 {
		return nil
	}
	ns := make(map[uint64]int64, len(ms))
	for track, v := range ms {
		ns[track] = v * 1_000_000
	}
	return ns
}

// concatCacheKey keys a plan by every source's identity (path+size+mtime, the
// scanner's change-detection fingerprint) in order, plus the packaging
// options - a replaced source or a different chain yields a new key.
func concatCacheKey(sources []string, opts apptranscoding.ConcatOptions) string {
	var b strings.Builder
	for _, s := range sources {
		var mtime, size int64
		if fi, err := os.Stat(s); err == nil {
			mtime = fi.ModTime().UnixNano()
			size = fi.Size()
		}
		fmt.Fprintf(&b, "%s:%d:%d|", s, size, mtime)
	}
	fmt.Fprintf(&b, "%d|%s", opts.SegmentMs, opts.URLPrefix)
	return b.String()
}

// concatPlan wraps *mp4.ConcatPlan to satisfy apptranscoding.ConcatPlan.
type concatPlan struct{ plan *mp4.ConcatPlan }

func (p *concatPlan) Resource(ctx context.Context, name string) ([]byte, string, error) {
	data, ct, err := p.plan.Resource(ctx, strings.ReplaceAll(name, concatPathSep, "/"))
	return data, ct, sourceChangedErr(err)
}

// Open is apptranscoding.OnDemandOpener (mkvgo 0.51): a part's media segment
// streams from its source as it is written; the concatenated playlists and the
// shifted subtitles come back whole, as Resource returns them.
func (p *concatPlan) Open(ctx context.Context, name string) (apptranscoding.OnDemandResource, error) {
	h, err := p.plan.Open(ctx, strings.ReplaceAll(name, concatPathSep, "/"))
	if err != nil {
		return nil, sourceChangedErr(err)
	}
	return h, nil
}

// Stat is apptranscoding.OnDemandStater (mkvgo 0.56).
func (p *concatPlan) Stat(ctx context.Context, name string) (size int64, etag, contentType string, ok bool, err error) {
	size, etag, contentType, ok, err = p.plan.Stat(ctx, strings.ReplaceAll(name, concatPathSep, "/"))
	return size, etag, contentType, ok, sourceChangedErr(err)
}

func (p *concatPlan) Resources() []string { return p.plan.Resources() }

func (p *concatPlan) NumParts() int { return p.plan.NumParts() }
