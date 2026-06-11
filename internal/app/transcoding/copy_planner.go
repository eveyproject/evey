package transcoding

import (
	"context"
	"fmt"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/cache"
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
// keyframe start), so the two can never drift. Plans are cached per media id —
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
	// expiry is checked lazily on Get — the planner has no Close lifecycle to
	// stop a sweeper from.
	cache *cache.TTLCache[[]CopySegment]
}

const (
	copyPlanCacheMax = 512
	copyPlanCacheTTL = 6 * time.Hour
)

// Density guard bounds for a usable copy plan. The every-keyframe contract
// means a pathological source (all-intra, or sub-second GOPs over hours)
// would explode into tens of thousands of tiny segments — worse for the
// client than transcoding. Mean gap < 1 s (e.g. all-intra ≈ one frame per
// segment) or an absurd segment count → Plan errors and the handler falls
// back to the transcode ladder. Normal film encodes (1–10 s GOPs) pass.
const (
	minCopyMeanGapMs    = 1000
	maxCopyPlanSegments = 20000
)

// NewCopyPlanner wires the planner to the stream + item readers.
func NewCopyPlanner(streams CopyStreamReader, items MediaItemReader) *CopyPlanner {
	c, _ := cache.NewWithMaxSize[[]CopySegment](0, copyPlanCacheMax) // interval 0 → no sweeper goroutine, stop func is a no-op
	return &CopyPlanner{
		streams: streams,
		items:   items,
		cache:   c,
	}
}

// Plan returns the copy segment boundaries for mediaID. Errors when the keyframe
// index or duration is missing — the caller must then NOT serve a copy rung.
func (c *CopyPlanner) Plan(ctx context.Context, mediaID string) ([]CopySegment, error) {
	if segs, ok := c.cache.Get(mediaID); ok {
		return segs, nil
	}

	streams, err := c.streams.GetByMediaItemID(ctx, shared.MediaItemID(mediaID))
	if err != nil {
		return nil, fmt.Errorf("copy planner: load streams for %s: %w", mediaID, err)
	}
	var keyframes []int64
	for _, s := range streams {
		if s != nil && s.StreamType == domainlibrary.StreamTypeVideo && len(s.Keyframes) > 0 {
			keyframes = s.Keyframes
			break
		}
	}
	if len(keyframes) == 0 {
		return nil, fmt.Errorf("copy planner: no keyframe index for media %s", mediaID)
	}

	item, err := c.items.GetByID(ctx, shared.MediaItemID(mediaID))
	if err != nil {
		return nil, fmt.Errorf("copy planner: load item %s: %w", mediaID, err)
	}
	if item == nil || item.File == nil {
		return nil, fmt.Errorf("copy planner: no file for media %s", mediaID)
	}
	durationMs := item.File.Duration.Milliseconds()
	if durationMs <= 0 {
		return nil, fmt.Errorf("copy planner: no duration for media %s", mediaID)
	}

	segs := KeyframeSegments(keyframes, durationMs)
	if len(segs) == 0 {
		return nil, fmt.Errorf("copy planner: empty plan for media %s", mediaID)
	}
	if len(segs) > maxCopyPlanSegments {
		return nil, fmt.Errorf("copy planner: %d segments exceeds the plan cap for media %s", len(segs), mediaID)
	}
	if durationMs/int64(len(segs)) < minCopyMeanGapMs {
		return nil, fmt.Errorf("copy planner: keyframes too dense for a copy rung (mean gap %dms) for media %s",
			durationMs/int64(len(segs)), mediaID)
	}

	c.cache.Set(mediaID, segs, copyPlanCacheTTL)
	return segs, nil
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
