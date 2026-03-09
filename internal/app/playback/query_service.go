package playback

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

type QueryService struct {
	progress playback.ProgressQueryRepo
}

func NewQueryService(progress playback.ProgressQueryRepo) *QueryService {
	return &QueryService{progress: progress}
}

func (s *QueryService) GetWatchProgress(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	return s.progress.GetByProfileAndMedia(ctx, profileID, mediaItemID)
}

func (s *QueryService) GetWatchHistory(ctx context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	return s.progress.GetByProfile(ctx, profileID, limit, offset)
}

func (s *QueryService) GetResumeItems(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetResumeItems(ctx, profileID, limit)
}

func (s *QueryService) GetNextUp(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetNextUp(ctx, profileID, limit)
}
