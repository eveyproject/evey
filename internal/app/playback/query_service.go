package playback

import (
	"context"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetActiveSessions")
	defer span.End()
	if s.sessions == nil {
		return []*playback.ActiveSessionInfo{}, nil
	}
	return s.sessions.ListActiveSessions(ctx)
}

func (s *QueryService) GetWatchProgress(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetWatchProgress")
	defer span.End()
	return s.progress.GetByProfileAndMedia(ctx, profileID, mediaItemID)
}

func (s *QueryService) GetWatchHistory(ctx context.Context, profileID shared.ProfileID, limit, offset int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetWatchHistory")
	defer span.End()
	return s.progress.GetByProfile(ctx, string(profileID), limit, offset)
}

func (s *QueryService) GetWatchHistoryCursor(ctx context.Context, profileID shared.ProfileID, cursor *playback.WatchHistoryCursor, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetWatchHistoryCursor")
	defer span.End()
	return s.progress.GetByProfileCursor(ctx, string(profileID), cursor, limit)
}

func (s *QueryService) GetResumeItems(ctx context.Context, profileID shared.ProfileID, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetResumeItems")
	defer span.End()
	return s.progress.GetResumeItems(ctx, string(profileID), limit)
}

func (s *QueryService) GetNextUp(ctx context.Context, profileID shared.ProfileID, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetNextUp")
	defer span.End()
	return s.progress.GetNextUp(ctx, string(profileID), limit)
}
