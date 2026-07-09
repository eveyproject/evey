package mkvgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ondemand.go — the mkvgo implementation of apptranscoding.OnDemandPackager:
// zero-storage HLS/DASH packaging via mp4.PlanHLS. A plan is a few bounded
// reads (metadata head + Cues for Matroska, the moov for MP4/MOV) and is then
// immutable and concurrency-safe, so we cache one per source and serve every
// segment/playlist/manifest request straight off it — no ffmpeg, no session
// state, no on-disk segments.

const (
	onDemandPlanTTL     = 30 * time.Minute
	onDemandPlanMaxSize = 256
	onDemandCleanup     = 5 * time.Minute
)

// OnDemandPackager builds and caches mp4.PlanHLS plans. Safe for concurrent use.
type OnDemandPackager struct {
	plans   *pkgcache.TTLCache[apptranscoding.OnDemandPlan]
	stopper func()
}

// NewOnDemandPackager returns a packager with an internal plan cache. Call
// Close to stop the cache's background janitor.
func NewOnDemandPackager() *OnDemandPackager {
	c, stop := pkgcache.NewWithMaxSize[apptranscoding.OnDemandPlan](onDemandCleanup, onDemandPlanMaxSize)
	return &OnDemandPackager{plans: c, stopper: stop}
}

// Close stops the plan cache janitor.
func (p *OnDemandPackager) Close() {
	if p != nil && p.stopper != nil {
		p.stopper()
	}
}

// Plan returns a cached-or-freshly-built plan for srcPath. It fails (and the
// caller falls back to ffmpeg) when the source cannot be packaged verbatim —
// an unsupported codec, or a Matroska with no Cues seek index.
func (p *OnDemandPackager) Plan(ctx context.Context, srcPath string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.OnDemandPlan")
	defer span.End()

	key := onDemandCacheKey(srcPath, opts)
	if pl, ok := p.plans.Get(key); ok {
		return pl, nil
	}

	// ChapterMarkers is hardcoded on: per mp4.Options docs, it is additive and
	// byte-identical for a chapterless source (no re-cut, no decode either way),
	// so there is no reason to gate it behind an option.
	o := mp4.Options{SegmentMs: opts.SegmentMs, ChapterMarkers: true}
	if opts.URLPrefix != "" {
		prefix := opts.URLPrefix
		o.RewriteURL = func(name string) string { return prefix + name }
	}
	if len(opts.AESKey) > 0 {
		o.Encrypt = &mp4.HLSEncryption{Key: opts.AESKey, KeyURI: opts.AESKeyURI}
	}

	hp, err := mp4.PlanHLS(ctx, srcPath, o)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("mkvgo PlanHLS %q: %w", srcPath, err)
	}

	pl := &ondemandPlan{plan: hp}
	// The DASH manifest is pre-built at plan time; probe it once (a field read,
	// not a segment build) so HasDASH is O(1) on the hot path.
	if _, _, derr := hp.Resource(ctx, "manifest.mpd"); derr == nil {
		pl.hasDASH = true
	}
	p.plans.Set(key, apptranscoding.OnDemandPlan(pl), onDemandPlanTTL)
	return pl, nil
}

// onDemandCacheKey keys a plan by the source identity (path + size + mtime, the
// scanner's own change-detection fingerprint) and the packaging options that
// change the bytes. A replaced/re-scanned source yields a new key, so a stale
// plan is never served. The URLPrefix is constant in practice, but keying on it
// keeps prefixed and unprefixed plans distinct if that ever changes.
//
// The AES key is hashed into the key (not just its length): the encrypted
// segment bytes depend on the exact key, so a rotated secret — same file, new
// derived key — must not alias onto a plan encrypted with the old key. The
// KeyURI rides along since it changes the playlists' EXT-X-KEY line.
func onDemandCacheKey(srcPath string, opts apptranscoding.OnDemandOptions) string {
	var mtime, size int64
	if fi, err := os.Stat(srcPath); err == nil {
		mtime = fi.ModTime().UnixNano()
		size = fi.Size()
	}
	keyTag := ""
	if len(opts.AESKey) > 0 {
		sum := sha256.Sum256(opts.AESKey)
		keyTag = hex.EncodeToString(sum[:8]) + ":" + opts.AESKeyURI
	}
	return fmt.Sprintf("%s|%d|%d|%d|%s|%s", srcPath, size, mtime, opts.SegmentMs, opts.URLPrefix, keyTag)
}

// ondemandPlan wraps *mp4.HLSPlan to satisfy apptranscoding.OnDemandPlan.
type ondemandPlan struct {
	plan    *mp4.HLSPlan
	hasDASH bool
}

func (p *ondemandPlan) Resource(ctx context.Context, name string) ([]byte, string, error) {
	return p.plan.Resource(ctx, name)
}

func (p *ondemandPlan) Resources() []string { return p.plan.Resources() }

func (p *ondemandPlan) NumSegments() int { return p.plan.NumSegments() }

func (p *ondemandPlan) HasDASH() bool { return p.hasDASH }
