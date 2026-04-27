package library

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// MediaSegment is a labelled time range within a media item (e.g. intro, outro, recap).
type MediaSegment struct {
	ID          string
	MediaItemID shared.MediaItemID
	Type        string // "intro", "outro", "recap", …
	StartMs     int64
	EndMs       int64
	Confidence  float64
	Source      string // "phash", "audio", "subtitle"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MediaSegmentRepository persists and retrieves media segments.
type MediaSegmentRepository interface {
	Upsert(ctx context.Context, seg *MediaSegment) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaSegment, error)
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
}
