package playback

import (
	"context"
	"time"
)

type PlaySessionFilter struct {
	ProfileID *string
	StartFrom *time.Time
	StartTo   *time.Time
	Limit     int
	Offset    int
}

// PlaySessionRepository composes command+query + extra methods not split.
type PlaySessionRepository interface {
	SessionCommandRepo
	SessionQueryRepo
	GetActiveByProfile(ctx context.Context, profileID string) ([]*PlaySession, error)
	ListActiveSessions(ctx context.Context) ([]*ActiveSessionInfo, error)
	GetHistory(ctx context.Context, filter PlaySessionFilter) ([]*PlaySession, error)
}
