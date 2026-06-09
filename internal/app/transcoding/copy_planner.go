package transcoding

import (
	"context"
	"fmt"
	"sync"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// CopyStreamReader loads a media item's streams, which carry the keyframe index
// (media_streams.keyframes_ms) the copy rung cuts on. Implemented by the
// media-stream repository (GetByMediaItemID already scans Keyframes).
type CopyStreamReader interface {
	GetByMediaItemID(ctx context.Context, id shared.MediaItemID) ([]*domainlibrary.MediaStream, error)
}

// CopyPlanner resolves the deterministic, keyframe-aligned segment plan for a
// copy-rung media item: the video stream's keyframes_ms + the file duration →
// KeyframeSegments. The plan is the SINGLE source of truth for both the copy
// manifest (#EXTINF list) and the session StartSec mapping (segment N → its
// keyframe start), so the two can never drift. Plans are cached per media id —
// keyframes are immutable for a fixed file.
//
// A copy rung is only usable when a plan exists: without a keyframe index there
// is no way to cut a -c:v copy stream, so Plan returns an error and the caller
// falls back to the transcode ladder.
type CopyPlanner struct {
	streams  CopyStreamReader
	items    MediaItemReader
	targetMs int64

	mu    sync.Mutex
	cache map[string][]CopySegment
}

// NewCopyPlanner wires the planner to the stream + item readers. targetMs is the
// minimum segment length fed to KeyframeSegments (a long-GOP source yields longer
// segments rather than splitting mid-GOP); 0 falls back to 6000.
func NewCopyPlanner(streams CopyStreamReader, items MediaItemReader, targetMs int64) *CopyPlanner {
	if targetMs <= 0 {
		targetMs = 6000
	}
	return &CopyPlanner{
		streams:  streams,
		items:    items,
		targetMs: targetMs,
		cache:    make(map[string][]CopySegment),
	}
}

// Plan returns the copy segment boundaries for mediaID. Errors when the keyframe
// index or duration is missing — the caller must then NOT serve a copy rung.
func (c *CopyPlanner) Plan(ctx context.Context, mediaID string) ([]CopySegment, error) {
	c.mu.Lock()
	if segs, ok := c.cache[mediaID]; ok {
		c.mu.Unlock()
		return segs, nil
	}
	c.mu.Unlock()

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

	segs := KeyframeSegments(keyframes, c.targetMs, durationMs)
	if len(segs) == 0 {
		return nil, fmt.Errorf("copy planner: empty plan for media %s", mediaID)
	}

	c.mu.Lock()
	c.cache[mediaID] = segs
	c.mu.Unlock()
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

// SegmentSeconds is the keyframe-segment target in seconds. The copy ffmpeg
// session MUST run -hls_time at this value so its split points land on the same
// keyframes the precalculated manifest (KeyframeSegments at the same target)
// addresses — otherwise the produced segments drift from the #EXTINF indices.
func (c *CopyPlanner) SegmentSeconds() float64 {
	return float64(c.targetMs) / 1000.0
}
