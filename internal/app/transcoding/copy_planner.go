package transcoding

import (
	"context"
	"fmt"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/cache"
)

// CopyStreamReader loads a media item's streams, which carry the keyframe index
// (media_streams.keyframes_ms) the copy rung cuts on. Implemented by the
// media-stream repository (GetByMediaItemID already scans Keyframes).
type CopyStreamReader interface {
	GetByMediaItemID(ctx context.Context, id shared.MediaItemID) ([]*domainlibrary.MediaStream, error)
}

// SeekLandingProber reports where the demuxer ACTUALLY lands when seeking the
// video stream to targetSec (the first packet's presentation time, seconds).
// Real-world container seek indexes are sparse (mkvmerge: one MKV cue per
// ~5 s cluster, not per keyframe), so a pre-input -ss aimed at a known
// keyframe can land a GOP or more EARLIER on the preceding cue point. A copy
// session must start its plan bookkeeping (StartSec / -start_number / tfdt
// rebase) at the LANDED keyframe, not the requested one, or every video
// timestamp it emits is shifted by the gap (the post-deep-seek A/V desync
// measured in prod). Implemented by the ffmpeg Prober (ProbeSeekLanding).
type SeekLandingProber interface {
	ProbeSeekLanding(ctx context.Context, filePath string, targetSec float64) (float64, error)
}

// CopyPlanner resolves the deterministic, keyframe-aligned segment plan for a
// copy-rung media item: the video stream's keyframes_ms + the file duration →
// KeyframeSegments (one segment per keyframe gap, mirroring the session's
// `-hls_time 0`). The plan is the SINGLE source of truth for both the copy
// manifest (#EXTINF list) and the session StartSec mapping (segment N → its
// keyframe start), so the two can never drift. Plans are cached per media id -
// keyframes are immutable for a fixed file.
//
// A copy rung is only usable when a plan exists: without a keyframe index there
// is no way to cut a -c:v copy stream, so Plan returns an error and the caller
// falls back to the transcode ladder.
type CopyPlanner struct {
	streams CopyStreamReader
	items   MediaItemReader

	// cache holds plans keyed by media id, LRU-bounded + TTL'd: the old plain
	// map grew one entry per distinct copy-served title for the process
	// lifetime (plafond = library size). Plans are immutable for a fixed file
	// and cheap to rebuild (two repository reads), so eviction is harmless.
	// No cleanup goroutine (interval 0): the LRU cap alone bounds memory and
	// expiry is checked lazily on Get - the planner has no Close lifecycle to
	// stop a sweeper from.
	//
	// Each entry carries the file stamp (size|mtime|fingerprint) it was built
	// from and Plan revalidates it against the item on every hit: a replaced
	// file (re-download, repair) re-probes to different keyframes, and serving
	// the old boundaries would cut -c:v copy segments at offsets that are no
	// longer keyframes. Self-validation beats a remote invalidation hook - no
	// scan-event wiring to forget, no stale window. The item read it costs on
	// a hit is a light PK lookup on the paths that call Plan (manifest bake,
	// session spawn), never the per-segment hot path.
	cache *cache.TTLCache[cachedCopyPlan]
}

// cachedCopyPlan is a plan plus the identity of the file it was derived from.
type cachedCopyPlan struct {
	segs     []CopySegment
	originMs int64
	stamp    string
}

// copyPlanFileStamp captures what change-detection knows about a file. It
// deliberately mirrors the scanner's size+mtime+fingerprint notion of "same
// file": when the scanner re-probes (and rewrites keyframes), this stamp
// changes with it.
func copyPlanFileStamp(f *domainlibrary.MediaFile) string {
	return fmt.Sprintf("%d|%d|%s", f.Size, f.ModTime.UnixNano(), f.Fingerprint)
}

const (
	copyPlanCacheMax = 512
	copyPlanCacheTTL = 6 * time.Hour
)

// Density guard bounds for a usable copy plan. The every-keyframe contract
// means a pathological source (all-intra, or sub-second GOPs over hours)
// would explode into tens of thousands of tiny segments - worse for the
// client than transcoding. Mean gap < 1 s (e.g. all-intra ≈ one frame per
// segment) or an absurd segment count → Plan errors and the handler falls
// back to the transcode ladder. Normal film encodes (1-10 s GOPs) pass.
const (
	minCopyMeanGapMs    = 1000
	maxCopyPlanSegments = 20000
)

// NewCopyPlanner wires the planner to the stream + item readers.
func NewCopyPlanner(streams CopyStreamReader, items MediaItemReader) *CopyPlanner {
	c, _ := cache.NewWithMaxSize[cachedCopyPlan](0, copyPlanCacheMax) // interval 0 → no sweeper goroutine, stop func is a no-op
	return &CopyPlanner{
		streams: streams,
		items:   items,
		cache:   c,
	}
}

// Plan returns the copy segment boundaries for mediaID. Errors when the keyframe
// index or duration is missing - the caller must then NOT serve a copy rung.
func (c *CopyPlanner) Plan(ctx context.Context, mediaID string) ([]CopySegment, error) {
	segs, _, err := c.PlanWithOrigin(ctx, mediaID)
	return segs, err
}

// PlanWithOrigin is Plan plus the plan's origin: the source time (ms) of the
// first keyframe, which the plan counts from. A copy session re-zeroes its
// output on its first packet, so a source that starts late (an MPEG-TS at
// ~1.4 s) produces fragments from 0: the plan is laid on that same origin,
// and a seek aims at StartMs + origin in the source's own time.
func (c *CopyPlanner) PlanWithOrigin(ctx context.Context, mediaID string) ([]CopySegment, int64, error) {
	// The item is read on every call (hit included): its file stamp decides
	// whether a cached plan is still for the file on disk.
	item, err := c.items.GetByID(ctx, shared.MediaItemID(mediaID))
	if err != nil {
		return nil, 0, fmt.Errorf("copy planner: load item %s: %w", mediaID, err)
	}
	if item == nil || item.File == nil {
		return nil, 0, fmt.Errorf("copy planner: no file for media %s", mediaID)
	}
	stamp := copyPlanFileStamp(item.File)
	if entry, ok := c.cache.Get(mediaID); ok && entry.stamp == stamp {
		return entry.segs, entry.originMs, nil
	}

	streams, err := c.streams.GetByMediaItemID(ctx, shared.MediaItemID(mediaID))
	if err != nil {
		return nil, 0, fmt.Errorf("copy planner: load streams for %s: %w", mediaID, err)
	}
	var keyframes []int64
	for _, s := range streams {
		if s != nil && s.StreamType == domainlibrary.StreamTypeVideo && len(s.Keyframes) > 0 {
			keyframes = s.Keyframes
			break
		}
	}
	if len(keyframes) == 0 {
		return nil, 0, fmt.Errorf("copy planner: no keyframe index for media %s", mediaID)
	}

	durationMs := item.File.Duration.Milliseconds()
	if durationMs <= 0 {
		return nil, 0, fmt.Errorf("copy planner: no duration for media %s", mediaID)
	}

	originMs := max(keyframes[0], 0)
	segs := KeyframeSegments(shiftKeyframes(keyframes, originMs), durationMs)
	if len(segs) == 0 {
		return nil, 0, fmt.Errorf("copy planner: empty plan for media %s", mediaID)
	}
	if len(segs) > maxCopyPlanSegments {
		return nil, 0, fmt.Errorf("copy planner: %d segments exceeds the plan cap for media %s", len(segs), mediaID)
	}
	if durationMs/int64(len(segs)) < minCopyMeanGapMs {
		return nil, 0, fmt.Errorf("copy planner: keyframes too dense for a copy rung (mean gap %dms) for media %s",
			durationMs/int64(len(segs)), mediaID)
	}

	c.cache.Set(mediaID, cachedCopyPlan{segs: segs, originMs: originMs, stamp: stamp}, copyPlanCacheTTL)
	return segs, originMs, nil
}

// shiftKeyframes is keyframes counted from originMs.
func shiftKeyframes(keyframes []int64, originMs int64) []int64 {
	if originMs == 0 {
		return keyframes
	}
	out := make([]int64, len(keyframes))
	for i, kf := range keyframes {
		out[i] = kf - originMs
	}
	return out
}

// SegmentCount returns the number of copy segments in the plan, or 0 on error.
// Used by the manifest builder and the StartSec bounds check.
func (c *CopyPlanner) SegmentCount(ctx context.Context, mediaID string) int {
	segs, err := c.Plan(ctx, mediaID)
	if err != nil {
		return 0
	}
	return len(segs)
}
