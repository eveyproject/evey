package mkvgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"
	"golang.org/x/sync/singleflight"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ondemand.go - the mkvgo implementation of apptranscoding.OnDemandPackager:
// zero-storage HLS/DASH packaging via mp4.PlanHLS. A plan is a few bounded
// reads (metadata head + Cues for Matroska, the moov for MP4/MOV) and is then
// immutable and concurrency-safe, so we cache one per source and serve every
// segment/playlist/manifest request straight off it - no ffmpeg, no session
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
	// audioDelays measures each Matroska audio track's start delay (native
	// mkvgo, no ffprobe) so Plan can cancel it in presentation without
	// touching the source. Injectable for tests; production passes
	// matroska.AudioStartDelays.
	audioDelays delaysFunc
	// trackList lists a Matroska source's tracks in container order (ID+type),
	// the cheapest possible read (header-only, no Cues/Chapters/Tags), so Plan
	// can resolve KeepAudioTrack's ordinal to a real track ID. Injectable for
	// tests; production passes reader.OpenMeta.
	trackList func(ctx context.Context, path string) ([]extmkv.Track, error)
	// group collapses concurrent cache misses for the SAME key into one build:
	// plan building can cluster-walk the whole source (SynthesizeIndex), and N
	// players hitting a cold file at stream start would walk it N times in
	// parallel - the weak-NAS I/O-starvation shape of the 2026-07-19 incident.
	group singleflight.Group
	// build is the uncached plan construction; a field (like audioDelays /
	// trackList above) so the stampede tests can count builds without a
	// real source.
	build func(ctx context.Context, srcPath string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error)
	// ttl is how long a plan outlives its last use (onDemandPlanTTL; a field
	// so the sliding test need not wait half an hour).
	ttl time.Duration
}

// NewOnDemandPackager returns a packager with an internal plan cache. Call
// Close to stop the cache's background janitor.
func NewOnDemandPackager() *OnDemandPackager {
	c, stop := pkgcache.NewWithMaxSize[apptranscoding.OnDemandPlan](onDemandCleanup, onDemandPlanMaxSize)
	p := &OnDemandPackager{
		plans: c, stopper: stop, ttl: onDemandPlanTTL,
		audioDelays: func(ctx context.Context, path string) (map[uint64]int64, error) {
			return extmatroska.AudioStartDelays(ctx, path)
		},
		trackList: func(ctx context.Context, path string) ([]extmkv.Track, error) {
			c, err := reader.OpenMeta(ctx, path)
			if err != nil {
				return nil, err
			}
			return c.Tracks, nil
		},
	}
	p.build = p.buildPlan
	return p
}

// Close stops the plan cache janitor.
func (p *OnDemandPackager) Close() {
	if p != nil && p.stopper != nil {
		p.stopper()
	}
}

// Plan returns a cached-or-freshly-built plan for srcPath. It fails (and the
// caller falls back to ffmpeg) when the source cannot be packaged verbatim -
// an unsupported codec. A Matroska with a missing or misskeyed Cues index is
// now served too (SynthesizeIndex), not refused.
func (p *OnDemandPackager) Plan(ctx context.Context, srcPath string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.OnDemandPlan")
	defer span.End()

	key := onDemandCacheKey(srcPath, opts)
	if pl, ok := p.plans.Get(key); ok {
		// Sliding: a plan in use stays. mkvgo learns where each track's
		// blocks lie as it serves (a block-ordered source reads ~2x instead of
		// a traversal per window); a fixed expiry dropped that mid-film.
		p.plans.Set(key, pl, p.ttl)
		return pl, nil
	}

	// Concurrent misses for the same key share ONE build (see p.group). The
	// winner builds under WithoutCancel so the first caller bailing (seek
	// away, tab close) can't abort the plan every follower is waiting on -
	// same contract as the remux singleflight. Errors are shared with the
	// waiters (they all fall back to ffmpeg together) but never cached, so
	// the next request retries.
	v, err, _ := p.group.Do(key, func() (any, error) {
		// Re-check under the flight: a prior winner may have cached it while
		// this caller queued.
		if pl, ok := p.plans.Get(key); ok {
			return pl, nil
		}
		pl, err := p.build(context.WithoutCancel(ctx), srcPath, opts)
		if err != nil {
			return nil, err
		}
		p.plans.Set(key, pl, p.ttl)
		return pl, nil
	})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return v.(apptranscoding.OnDemandPlan), nil
}

// buildPlan is the uncached construction Plan's singleflight runs: options
// assembly, best-effort Matroska probes, then mp4.PlanHLS.
func (p *OnDemandPackager) buildPlan(ctx context.Context, srcPath string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	// ChapterMarkers is hardcoded on: per mp4.Options docs, it is additive and
	// byte-identical for a chapterless source (no re-cut, no decode either way),
	// so there is no reason to gate it behind an option.
	//
	// SynthesizeIndex serves a source whose Cues are missing or key the wrong
	// tracks (the Highwaymen/Eli defect) by walking its clusters ONCE
	// (structure-only) to build the video index in memory - instead of
	// refusing the plan and falling back to a full ffmpeg transcode. It costs
	// nothing on a healthy file (the walk only fires when no usable video cue
	// exists) and the resulting plan is cached, so the walk is paid once. This
	// is the fast-seek path for un-indexed files AND for read-only media where
	// the on-disk reindex cannot write.
	// SkipUnsupported: a codec MP4 cannot carry (TrueHD, LPCM, Vorbis - routine in
	// a Blu-ray remux) makes PlanHLS fail HARD, which took the whole plan down and
	// sent a perfectly copy-eligible VIDEO to a full ffmpeg re-encode. Dropping the
	// track is exactly right here: Evey re-encodes any audio it can't carry anyway
	// (the hybrid rung), so the packager never needed to carry it. The admin remux
	// path has always passed this (adapter.go); the serving path had not.
	o := mp4.Options{SegmentMs: opts.SegmentMs, ChapterMarkers: true, SynthesizeIndex: true, SkipUnsupported: true}
	// Audio start-delay resync, in presentation only, nothing written: the
	// same per-track defect the admin retimer repairs on disk (audio_retime.go)
	// is cancelled here at packaging time for a source the operator hasn't
	// repaired yet (or can't - a read-only mount). AudioStartDelays is
	// Matroska-only; best-effort like SynthesizeIndex above - a probe failure
	// must not refuse the plan, it just serves without the resync.
	if IsEBMLContainer(srcPath) && p.audioDelays != nil {
		if delays, derr := p.audioDelays(ctx, srcPath); derr == nil {
			o.AudioPresentationShift = audioPresentationShift(delays)
		} else {
			slog.WarnContext(ctx, "mkvgo on-demand: audio-delay probe failed, serving without resync",
				slog.String("path", srcPath), slog.Any("err", derr))
		}
	}
	// KeepAudioTrack: a Virtual Edit Layer trim (mp4.Options.KeepTracks) to a
	// single audio track - video and every subtitle stay. Off by default (nil,
	// the operator flag gates it), and best-effort like the resync above: a
	// track-listing failure serves the untrimmed presentation rather than
	// refusing the plan.
	if opts.KeepAudioTrack != nil && IsEBMLContainer(srcPath) && p.trackList != nil {
		if tracks, terr := p.trackList(ctx, srcPath); terr == nil {
			o.KeepTracks = keepTracksFor(tracks, *opts.KeepAudioTrack)
		} else {
			slog.WarnContext(ctx, "mkvgo on-demand: track list failed, serving every track",
				slog.String("path", srcPath), slog.Any("err", terr))
		}
	}
	if opts.URLPrefix != "" {
		prefix := opts.URLPrefix
		o.RewriteURL = func(name string) string { return prefix + name }
	}
	if opts.AESRotateSegments > 0 && len(opts.AESKeys) >= 2 {
		keys := make([]mp4.HLSKey, len(opts.AESKeys))
		for i, k := range opts.AESKeys {
			keys[i] = mp4.HLSKey{Key: k.Key, KeyURI: k.URI}
		}
		o.Encrypt = &mp4.HLSEncryption{RotateEverySegments: opts.AESRotateSegments, Keys: keys}
	} else if len(opts.AESKey) > 0 {
		o.Encrypt = &mp4.HLSEncryption{Key: opts.AESKey, KeyURI: opts.AESKeyURI}
	}

	hp, err := mp4.PlanHLS(ctx, srcPath, o)
	if err != nil {
		return nil, fmt.Errorf("mkvgo PlanHLS %q: %w", srcPath, err)
	}

	pl := &ondemandPlan{plan: hp}
	// The DASH manifest is pre-built at plan time; probe it once (a field read,
	// not a segment build) so HasDASH is O(1) on the hot path.
	if _, _, derr := hp.Resource(ctx, "manifest.mpd"); derr == nil {
		pl.hasDASH = true
	}
	return pl, nil
}

// onDemandCacheKey keys a plan by the source identity (path + size + mtime, the
// scanner's own change-detection fingerprint) and the packaging options that
// change the bytes. A replaced/re-scanned source yields a new key, so a stale
// plan is never served. The URLPrefix is constant in practice, but keying on it
// keeps prefixed and unprefixed plans distinct if that ever changes.
//
// The AES key is hashed into the key (not just its length): the encrypted
// segment bytes depend on the exact key, so a rotated secret - same file, new
// derived key - must not alias onto a plan encrypted with the old key. The
// KeyURI rides along since it changes the playlists' EXT-X-KEY line.
func onDemandCacheKey(srcPath string, opts apptranscoding.OnDemandOptions) string {
	var mtime, size int64
	if fi, err := os.Stat(srcPath); err == nil {
		mtime = fi.ModTime().UnixNano()
		size = fi.Size()
	}
	keyTag := ""
	if opts.AESRotateSegments > 0 && len(opts.AESKeys) >= 2 {
		// Rotation: hash every key + URI and the period so a rotated presentation
		// never aliases onto the single-key plan (or a different rotation).
		h := sha256.New()
		fmt.Fprintf(h, "rot:%d:", opts.AESRotateSegments)
		for _, k := range opts.AESKeys {
			h.Write(k.Key)
			h.Write([]byte{0})
			h.Write([]byte(k.URI))
			h.Write([]byte{0})
		}
		keyTag = "rot" + hex.EncodeToString(h.Sum(nil)[:8])
	} else if len(opts.AESKey) > 0 {
		sum := sha256.Sum256(opts.AESKey)
		keyTag = hex.EncodeToString(sum[:8]) + ":" + opts.AESKeyURI
	}
	audioTrackTag := "-"
	if opts.KeepAudioTrack != nil {
		audioTrackTag = fmt.Sprintf("a%d", *opts.KeepAudioTrack)
	}
	return fmt.Sprintf("%s|%d|%d|%d|%s|%s|%s", srcPath, size, mtime, opts.SegmentMs, opts.URLPrefix, keyTag, audioTrackTag)
}

// keepTracksFor resolves mp4.Options.KeepTracks for a trimmed plan: the
// (first) video track and every subtitle track are always kept; the audio
// set narrows to the ordinal-th audio track in container order (0-based,
// matching resolveAudioTrack's numbering). An out-of-range ordinal keeps no
// audio track rather than falling back to "every audio track" - a stale
// request must not silently defeat the trim.
func keepTracksFor(tracks []extmkv.Track, audioOrdinal int) []uint64 {
	var kept []uint64
	videoKept := false
	audioIdx := 0
	for _, t := range tracks {
		switch t.Type {
		case extmkv.VideoTrack:
			if !videoKept {
				kept = append(kept, t.ID)
				videoKept = true
			}
		case extmkv.AudioTrack:
			if audioIdx == audioOrdinal {
				kept = append(kept, t.ID)
			}
			audioIdx++
		case extmkv.SubtitleTrack:
			kept = append(kept, t.ID)
		}
	}
	return kept
}

// delaysFunc measures each audio track's start delay (Matroska track number ->
// nanoseconds, positive = late). Injectable for tests; production passes
// matroska.AudioStartDelays. It lived beside the persistent retime adapter
// until that was removed - the retime that rewrote 98 files on 2026-07-13 -
// and stayed here because CANCELLING a delay in presentation touches nothing
// on disk and is a different thing entirely.
type delaysFunc func(ctx context.Context, path string) (map[uint64]int64, error)

// audioDelayFloorNs is the delay below which a track is considered aligned:
// codec priming territory (tens of ms), never a defect. Mirrors
// maintenance.AudioDelayFlagSec.
const audioDelayFloorNs = 100_000_000

// audioPresentationShift floors AudioStartDelays' raw map to mp4.Options.
// AudioPresentationShift: only tracks past audioDelayFloorNs are shifted, so
// codec priming (tens of ms) never moves a healthy file's presentation. The
// value is passed
// through UNNEGATED - unlike RetimeTracks' persistent shift, which takes the
// negated delay, AudioPresentationShift takes AudioStartDelays' sign
// directly (positive = presented earlier).
func audioPresentationShift(delays map[uint64]int64) map[uint64]int64 {
	var shift map[uint64]int64
	for track, ns := range delays {
		if ns > audioDelayFloorNs {
			if shift == nil {
				shift = make(map[uint64]int64, len(delays))
			}
			shift[track] = ns
		}
	}
	return shift
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
