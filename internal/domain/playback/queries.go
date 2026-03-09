package playback

import "context"

type ProgressQueryRepo interface {
	GetByProfileAndMedia(ctx context.Context, profileID, mediaItemID string) (*WatchProgress, error)
	GetByProfile(ctx context.Context, profileID string, limit, offset int) ([]*WatchProgress, error)
	GetResumeItems(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
	GetNextUp(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
}

type SessionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*PlaySession, error)
	CountActiveByUser(ctx context.Context, userID string) (int, error)
}
