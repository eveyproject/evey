package mkvgo

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// concat.go - the mkvgo implementation of apptranscoding.ConcatPackager:
// gapless multi-episode HLS via mp4.PlanConcat. Same zero-storage contract as
// ondemand.go's OnDemandPackager, one cache entry per distinct source chain.

const (
	concatPlanTTL     = 30 * time.Minute
	concatPlanMaxSize = 64
	concatCleanup     = 5 * time.Minute
	// concatPathSep replaces "/" in a part resource's canonical name
	// ("p0/seg00001.m4s") on the wire: the shared HLS route's {file} wildcard
	// is single-segment, so a slash would 404 before reaching the handler.
	// Encoded here, decoded back in Resource below - entirely internal to
	// this adapter, the HTTP layer only strips its own prefix.
	concatPathSep = "__"
)

// ConcatPackager builds and caches mp4.PlanConcat plans. Safe for concurrent use.
type ConcatPackager struct {
	plans   *pkgcache.TTLCache[apptranscoding.ConcatPlan]
	stopper func()
}

// NewConcatPackager returns a packager with an internal plan cache. Call
// Close to stop the cache's background janitor.
func NewConcatPackager() *ConcatPackager {
	c, stop := pkgcache.NewWithMaxSize[apptranscoding.ConcatPlan](concatCleanup, concatPlanMaxSize)
	return &ConcatPackager{plans: c, stopper: stop}
}

// Close stops the plan cache janitor.
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

	key := concatCacheKey(sources, opts)
	if pl, ok := p.plans.Get(key); ok {
		return pl, nil
	}

	// Same hardcoded-on additions as the single-episode packager (ondemand.go):
	// ChapterMarkers is additive/byte-identical on a chapterless part, and
	// SynthesizeIndex lets an un-indexed part join the session instead of
	// refusing the whole chain over one file's missing Cues.
	// SkipUnsupported: same reason as the on-demand packager (ondemand.go) - one
	// TrueHD/LPCM/Vorbis track must not fail the whole chain's plan.
	// NoIFrames: same reason as the on-demand packager (ondemand.go).
	o := mp4.Options{SegmentMs: opts.SegmentMs, ChapterMarkers: true, SynthesizeIndex: true, SkipUnsupported: true, NoIFrames: true}
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
	p.plans.Set(key, apptranscoding.ConcatPlan(pl), concatPlanTTL)
	return pl, nil
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
	return p.plan.Resource(ctx, strings.ReplaceAll(name, concatPathSep, "/"))
}

func (p *concatPlan) Resources() []string { return p.plan.Resources() }

func (p *concatPlan) NumParts() int { return p.plan.NumParts() }
