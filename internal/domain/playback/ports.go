package playback

import (
	"context"
	"time"
)

type WatchProgressRepository interface {
	Upsert(ctx context.Context, wp *WatchProgress) error
	GetByProfileAndMedia(ctx context.Context, profileID, mediaItemID string) (*WatchProgress, error)
	GetByProfile(ctx context.Context, profileID string, limit, offset int) ([]*WatchProgress, error)
	GetResumeItems(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
	GetNextUp(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
}

type PlaySessionFilter struct {
	ProfileID *string
	StartFrom *time.Time
	StartTo   *time.Time
	Limit     int
	Offset    int
}

type PlaySessionRepository interface {
	Create(ctx context.Context, session *PlaySession) error
	Update(ctx context.Context, session *PlaySession) error
	GetByID(ctx context.Context, id string) (*PlaySession, error)
	GetActiveByProfile(ctx context.Context, profileID string) ([]*PlaySession, error)
	GetHistory(ctx context.Context, filter PlaySessionFilter) ([]*PlaySession, error)
	CountActiveByUser(ctx context.Context, userID string) (int, error)
}
