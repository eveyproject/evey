package mkvgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	// onDemandFailureTTL is how long a source that could not be planned is
	// refused without trying again. A failed build is not cheap: a file mkvgo
	// cannot read may still cost a long read before saying so (a mislabelled
	// MPEG-TS read 341 MiB and held up to 512 MiB per attempt, 2026-10-05), and
	// every playlist and segment request of its viewers retried it. The key
	// carries the file's size and mtime, so a repaired file is planned at once.
	onDemandFailureTTL     = 5 * time.Minute
	onDemandFailureMaxSize = 1024
	// onDemandMaxBuilds caps the plans built at once. A build is a bounded read
	// of the head, but a cold disk makes it slow and an audience starting
	// together stacks them up: the cap keeps their memory and their reads from
	// growing with the audience while the ones queued wait their turn. 4 was too
	// few (2026-10-06 run, 91 viewers): three builds stuck 5-7 s behind a busy
	// disk (each a hundred-odd seeks, 0.1-1 s alone) held other titles queued up
	// to 7.8 s, past the 4 s start budget. A build allocates up to ~60 MB in
	// passing, so 16 at once stays under 1 GB.
	onDemandMaxBuilds = 16
	// onDemandSlowBuild is when a build gets named in the log. A plan is a
	// head read of milliseconds; one still running after this is reading far
	// more than a head, and the line naming its file is the only way to know
	// which source a memory peak belongs to (the heap profile names the code,
	// not the file).
	onDemandSlowBuild = 5 * time.Second
)

// OnDemandPackager builds and caches mp4.PlanHLS plans. Safe for concurrent use.
type OnDemandPackager struct {
	plans   *pkgcache.TTLCache[apptranscoding.OnDemandPlan]
	stopper func()
	// failures remembers, per key, the error of a build that failed, for
	// onDemandFailureTTL.
	failures    *pkgcache.TTLCache[error]
	stopFailure func()
	// builds is the onDemandMaxBuilds semaphore.
	builds chan struct{}
	// subtitleIndex finds the stored subtitle block index of a source, if any
	// (see WithSubtitleIndex).
	subtitleIndex func(path string) (*extmatroska.SubtitleIndex, bool)
	// slowAfter is onDemandSlowBuild, and logger where slow builds are named;
	// fields so a test can shorten the one and read the other.
	slowAfter time.Duration
	logger    *slog.Logger
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
	f, stopF := pkgcache.NewWithMaxSize[error](onDemandCleanup, onDemandFailureMaxSize)
	p := &OnDemandPackager{
		plans: c, stopper: stop, ttl: onDemandPlanTTL,
		failures: f, stopFailure: stopF,
		builds:    make(chan struct{}, onDemandMaxBuilds),
		slowAfter: onDemandSlowBuild, logger: slog.Default(),
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

// WithSubtitleIndex hands each Matroska plan the subtitle block index already
// stored for its source, so its subtitle segments read those blocks at their
// position instead of walking the video around them. A source without an index
// is planned as before; an index that no longer matches is set aside by mkvgo
// itself and the walk serves. A plan built before its source's index existed
// keeps walking until the plan cache lets it go.
func (p *OnDemandPackager) WithSubtitleIndex(lookup func(path string) (*extmatroska.SubtitleIndex, bool)) *OnDemandPackager {
	p.subtitleIndex = lookup
	return p
}

// subtitleIndexFor is the stored subtitle index of a Matroska source, or nil
// (no lookup wired, not Matroska, no index stored).
func (p *OnDemandPackager) subtitleIndexFor(srcPath string) mp4.SubtitleBlockIndex {
	if p.subtitleIndex == nil || !IsEBMLContainer(srcPath) {
		return nil
	}
	if ix, ok := p.subtitleIndex(srcPath); ok && ix != nil {
		return ix
	}
	return nil
}

// Close stops the plan cache janitor.
func (p *OnDemandPackager) Close() {
	if p != nil && p.stopper != nil {
		p.stopper()
	}
	if p != nil && p.stopFailure != nil {
		p.stopFailure()
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

	if err, ok := p.failures.Get(key); ok {
		span.RecordError(err)
		return nil, err
	}

	// Concurrent misses for the same key share ONE build (see p.group). The
	// winner builds under WithoutCancel so the first caller bailing (seek
	// away, tab close) can't abort the plan every follower is waiting on -
	// same contract as the remux singleflight. Errors are shared with the
	// waiters (they all fall back to ffmpeg together) and remembered for
	// onDemandFailureTTL, so the next requests fall back without rebuilding.
	v, err, _ := p.group.Do(key, func() (any, error) {
		// Re-check under the flight: a prior winner may have cached it while
		// this caller queued.
		if pl, ok := p.plans.Get(key); ok {
			return pl, nil
		}
		if err, ok := p.failures.Get(key); ok {
			return nil, err
		}
		pl, err := p.buildNamingTheSlowOnes(context.WithoutCancel(ctx), srcPath, opts)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				p.failures.Set(key, err, onDemandFailureTTL)
			}
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

// WindowStats sums the window counters of every plan the cache holds
// (mkvgo HLSPlan.Stats: each window build is a read of the source, a rebuild
// one that read the same window again), plus "plans", how many were summed.
// The counters run from each plan's construction, and a plan the cache let go
// takes its own with it: a caller wanting a run's share diffs two snapshots.
func (p *OnDemandPackager) WindowStats() map[string]int64 {
	out := map[string]int64{}
	for _, v := range p.plans.Values() {
		pl, ok := v.(interface{ Stats() mp4.HLSPlanStats })
		if !ok {
			continue
		}
		st := pl.Stats()
		out["plans"]++
		out["window_builds"] += st.WindowBuilds
		out["window_rebuilds"] += st.WindowRebuilds
		out["shared_renditions"] += st.SharedRenditions
		out["waited_builds"] += st.WaitedBuilds
		out["evictions"] += st.Evictions
		out["built_bytes"] += st.BuiltBytes
		out["served_bytes"] += st.ServedBytes
		out["dropped_bytes"] += st.DroppedBytes
		out["subtitle_walks"] += st.SubtitleWalks
		out["subtitle_indexed_blocks"] += st.SubtitleIndexedBlocks
		// mkvgo 0.42: the walk cap, the streaming path and the Cues-derived
		// subtitle index each say how often they engaged.
		out["slot_waits"] += st.SlotWaits
		out["streamed_segments"] += st.StreamedSegments
		out["stream_fallbacks"] += st.StreamFallbacks
		out["table_builds"] += st.TableBuilds
		out["table_evictions"] += st.TableEvictions
		out["undeclared_zlib_blocks"] += st.UndeclaredZlibBlocks
		out["subtitle_cue_indexed"] += st.SubtitleCueIndexed
		out["subtitle_cue_unverified"] += st.SubtitleCueUnverified
		out["spread_laced_tracks"] += st.SpreadLacedTracks
	}
	return out
}

// WalksInFlight is how many source windows the process is reading right now,
// across every plan (mp4.WalksInFlight). The gauge behind the walk cap.
func WalksInFlight() int64 { return mp4.WalksInFlight() }

// buildNamingTheSlowOnes runs one build under the onDemandMaxBuilds cap and
// names any that outlasts slowAfter: once while it is still running, because a
// build stuck behind a saturated disk only ends after the peak it caused, and
// once at the end with what it cost.
func (p *OnDemandPackager) buildNamingTheSlowOnes(ctx context.Context, srcPath string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	queued := time.Now()
	p.builds <- struct{}{}
	defer func() { <-p.builds }()
	started := time.Now()
	stillRunning := time.AfterFunc(p.slowAfter, func() {
		p.logger.WarnContext(ctx, "mkvgo on-demand: plan build still running",
			slog.String("path", srcPath),
			slog.Int64("running_ms", time.Since(started).Milliseconds()),
			slog.Int("builds_running", len(p.builds)))
	})
	pl, err := p.build(ctx, srcPath, opts)
	if !stillRunning.Stop() {
		args := []any{
			slog.String("path", srcPath),
			slog.Int64("build_ms", time.Since(started).Milliseconds()),
			slog.Int64("queued_ms", started.Sub(queued).Milliseconds()),
		}
		if err != nil {
			args = append(args, slog.Any("err", err))
		}
		p.logger.WarnContext(ctx, "mkvgo on-demand: slow plan build", args...)
	}
	return pl, err
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
	if ix := p.subtitleIndexFor(srcPath); ix != nil {
		o.SubtitleIndex = ix
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

// Open is apptranscoding.OnDemandOpener: a media segment of a local Matroska
// source streams from the file as it is written (mkvgo 0.42); everything else
// - playlists, inits, subtitles, an encrypted or converted segment - comes back
// whole, as Resource would return it (Stats().StreamFallbacks counts those).
func (p *ondemandPlan) Open(ctx context.Context, name string) (apptranscoding.OnDemandResource, error) {
	h, err := p.plan.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (p *ondemandPlan) Resources() []string { return p.plan.Resources() }

func (p *ondemandPlan) NumSegments() int { return p.plan.NumSegments() }

func (p *ondemandPlan) HasDASH() bool { return p.hasDASH }

// Stats is the plan's window counters (see OnDemandPackager.WindowStats).
func (p *ondemandPlan) Stats() mp4.HLSPlanStats { return p.plan.Stats() }
