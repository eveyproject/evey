package playback

import (
	"context"

	"github.com/evey-project/evey/internal/domain/playback"
)

type QueryService struct {
	progress playback.ProgressQueryRepo
	sessions playback.SessionQueryRepo
}

func NewQueryService(progress playback.ProgressQueryRepo, sessions ...playback.SessionQueryRepo) *QueryService {
	q := &QueryService{progress: progress}
	if len(sessions) > 0 {
		q.sessions = sessions[0]
	}
	return q
}

func (s *QueryService) GetActiveSessions(ctx context.Context) ([]*playback.ActiveSessionInfo, error) {
	if s.sessions == nil {
		return []*playback.ActiveSessionInfo{}, nil
	}
	return s.sessions.ListActiveSessions(ctx)
}

func (s *QueryService) GetWatchProgress(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	return s.progress.GetByProfileAndMedia(ctx, profileID, mediaItemID)
}

func (s *QueryService) GetWatchHistory(ctx context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	return s.progress.GetByProfile(ctx, profileID, limit, offset)
}

func (s *QueryService) GetWatchHistoryCursor(ctx context.Context, profileID string, cursor *playback.WatchHistoryCursor, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetByProfileCursor(ctx, profileID, cursor, limit)
}

func (s *QueryService) GetResumeItems(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetResumeItems(ctx, profileID, limit)
}

func (s *QueryService) GetNextUp(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetNextUp(ctx, profileID, limit)
}
