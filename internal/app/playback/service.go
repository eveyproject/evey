package playback

import (
	"context"
	"errors"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

var (
	ErrMaxStreams       = errors.New("max concurrent streams reached")
	ErrSessionNotFound  = errors.New("session not found")
	ErrProfileNotFound  = errors.New("profile not found")
)

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(
	progressCmd playback.ProgressCommandRepo,
	progressQuery playback.ProgressQueryRepo,
	sessionCmd playback.SessionCommandRepo,
	sessionQuery playback.SessionQueryRepo,
	users user.UserRepository,
	profiles user.ProfileRepository,
	events shared.EventBus,
) *Service {
	cmd := NewCommandService(progressCmd, sessionCmd, sessionQuery, users, profiles, events)
	q := NewQueryService(progressQuery, sessionQuery)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }

func (s *Service) StartStream(ctx context.Context, session *playback.PlaySession, userID string) error {
	return s.cmd.StartStream(ctx, session, userID)
}

func (s *Service) StopStream(ctx context.Context, sessionID string) error {
	return s.cmd.StopStream(ctx, sessionID)
}

func (s *Service) GetActiveSessions(ctx context.Context) ([]*playback.ActiveSessionInfo, error) {
	return s.query.GetActiveSessions(ctx)
}

func (s *Service) ReportProgressBySession(ctx context.Context, sessionID string, positionMs int64, isPaused bool) error {
	return s.cmd.ReportProgressBySession(ctx, sessionID, positionMs, isPaused)
}

func (s *Service) ReportProgress(ctx context.Context, profileID, mediaItemID string, positionMs int64, completed bool) error {
	return s.cmd.ReportProgress(ctx, profileID, mediaItemID, positionMs, completed)
}

func (s *Service) GetWatchProgress(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	return s.query.GetWatchProgress(ctx, profileID, mediaItemID)
}

func (s *Service) GetWatchHistory(ctx context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	return s.query.GetWatchHistory(ctx, profileID, limit, offset)
}

func (s *Service) GetWatchHistoryCursor(ctx context.Context, profileID string, cursor *playback.WatchHistoryCursor, limit int) ([]*playback.WatchProgress, error) {
	return s.query.GetWatchHistoryCursor(ctx, profileID, cursor, limit)
}

func (s *Service) GetResumeItems(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.query.GetResumeItems(ctx, profileID, limit)
}

func (s *Service) GetNextUp(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.query.GetNextUp(ctx, profileID, limit)
}

func (s *Service) MarkAsWatched(ctx context.Context, profileID, mediaItemID string) error {
	return s.cmd.MarkAsWatched(ctx, profileID, mediaItemID)
}

func (s *Service) MarkAsUnwatched(ctx context.Context, profileID, mediaItemID string) error {
	return s.cmd.MarkAsUnwatched(ctx, profileID, mediaItemID)
}
