package playback

import (
	"context"
	"time"
)

// WatchHistoryCursor keysets watch-history responses by the item's
// last_played_at timestamp, DESC. MediaItemID is the tiebreaker when
// two items share the same timestamp (rare but possible on bulk
// imports).
type WatchHistoryCursor struct {
	LastPlayedAt time.Time
	MediaItemID  string
}

type ProgressQueryRepo interface {
	GetByProfileAndMedia(ctx context.Context, profileID, mediaItemID string) (*WatchProgress, error)
	GetByProfile(ctx context.Context, profileID string, limit, offset int) ([]*WatchProgress, error)
	// GetByProfileCursor returns up to `limit` items whose last_watched_at
	// is strictly older than the cursor (DESC). nil cursor = most recent
	// page.
	GetByProfileCursor(ctx context.Context, profileID string, cursor *WatchHistoryCursor, limit int) ([]*WatchProgress, error)
	GetResumeItems(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
	GetNextUp(ctx context.Context, profileID string, limit int) ([]*WatchProgress, error)
	// ListSeriesEpisodes is every episode of a series in viewing order, with
	// the profile's progress on each (see PickSeriesEpisode).
	ListSeriesEpisodes(ctx context.Context, profileID, seriesID string) ([]SeriesEpisode, error)
}

type SessionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*PlaySession, error)
	CountActiveByUser(ctx context.Context, userID string) (int, error)
	GetActiveByProfile(ctx context.Context, profileID string) ([]*PlaySession, error)
	ListActiveSessions(ctx context.Context) ([]*ActiveSessionInfo, error)
}
