package playback

import (
	"context"
	"errors"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type QueryService struct {
	progress playback.ProgressQueryRepo
	sessions playback.SessionQueryRepo
	pending  pendingProgress
}

// pendingProgress is the write buffer the player's ticks wait in before they
// reach the database (WatchProgressBuffer).
type pendingProgress interface {
	Peek(profileID shared.ProfileID, mediaItemID shared.MediaItemID) (*playback.WatchProgress, bool)
}

// WithPendingProgress makes GetWatchProgress read through the tick buffer.
func (s *QueryService) WithPendingProgress(p pendingProgress) *QueryService {
	s.pending = p
	return s
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
	row, err := s.progress.GetByProfileAndMedia(ctx, profileID, mediaItemID)
	if s.pending == nil {
		return row, err
	}
	// The player's last ticks sit in the buffer for up to a flush interval.
	// Back on the media page a second after leaving the player, the database
	// alone answered with no row (a first viewing: Play instead of Resume) or
	// a position seconds behind.
	p, ok := s.pending.Peek(shared.ProfileID(profileID), shared.MediaItemID(mediaItemID))
	if !ok {
		return row, err
	}
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return p, nil
		}
		return nil, err
	}
	if p.LastPlayedAt.Before(row.LastPlayedAt) {
		return row, nil
	}
	// Same merge as the database upsert: the fresher position, a completion
	// that never goes back.
	merged := *row
	merged.Position = p.Position
	merged.LastPlayedAt = p.LastPlayedAt
	if p.Completed && !row.Completed {
		merged.Completed = true
		merged.PlayCount = row.PlayCount + 1
	}
	return &merged, nil
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

// GetSeriesPlayTarget is the episode "Play" on a series starts for this
// profile (playback.PickSeriesEpisode) and whether the profile has started
// the series at all; ok is false for a series without episodes.
func (s *QueryService) GetSeriesPlayTarget(ctx context.Context, profileID shared.ProfileID, seriesID shared.MediaItemID) (target playback.SeriesEpisode, started, ok bool, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetSeriesPlayTarget")
	defer span.End()
	episodes, err := s.progress.ListSeriesEpisodes(ctx, string(profileID), string(seriesID))
	if err != nil {
		return playback.SeriesEpisode{}, false, false, err
	}
	target, ok = playback.PickSeriesEpisode(episodes)
	return target, playback.SeriesStarted(episodes), ok, nil
}
