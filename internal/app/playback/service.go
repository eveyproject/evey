package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

var (
	ErrMaxStreams   = errors.New("max concurrent streams reached")
	ErrSessionNotFound = errors.New("session not found")
)

type Service struct {
	progress playback.WatchProgressRepository
	sessions playback.PlaySessionRepository
	users    user.UserRepository
	events   shared.EventBus
}

func NewService(
	progress playback.WatchProgressRepository,
	sessions playback.PlaySessionRepository,
	users user.UserRepository,
	events shared.EventBus,
) *Service {
	return &Service{
		progress: progress,
		sessions: sessions,
		users:    users,
		events:   events,
	}
}

func (s *Service) StartStream(ctx context.Context, session *playback.PlaySession, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if u.MaxStreams != nil && *u.MaxStreams > 0 {
		count, err := s.sessions.CountActiveByUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("count active: %w", err)
		}
		if count >= *u.MaxStreams {
			return ErrMaxStreams
		}
	}

	session.StartedAt = time.Now()
	if err := s.sessions.Create(ctx, session); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	s.events.Publish(ctx, playback.PlaybackStarted{
		SessionID:   session.ID,
		ProfileID:   session.ProfileID,
		MediaItemID: session.MediaItemID,
		PlayMethod:  session.PlayMethod,
		OccurredAt:  time.Now(),
	})
	return nil
}

func (s *Service) StopStream(ctx context.Context, sessionID string) error {
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	now := time.Now()
	session.EndedAt = &now
	if err := s.sessions.Update(ctx, session); err != nil {
		return err
	}
	s.events.Publish(ctx, playback.PlaybackStopped{
		SessionID:   sessionID,
		ProfileID:   session.ProfileID,
		MediaItemID: session.MediaItemID,
		OccurredAt:  now,
	})
	return nil
}

func (s *Service) GetActiveSessions(ctx context.Context) ([]*playback.ActiveSessionInfo, error) {
	return s.sessions.ListActiveSessions(ctx)
}

func (s *Service) ReportProgressBySession(ctx context.Context, sessionID string, positionMs int64) error {
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	return s.ReportProgress(ctx, session.ProfileID, session.MediaItemID, positionMs, false)
}

func (s *Service) ReportProgress(ctx context.Context, profileID, mediaItemID string, positionMs int64, completed bool) error {
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		PositionMs:   positionMs,
		Completed:    completed,
		LastPlayedAt: time.Now(),
	}
	return s.progress.Upsert(ctx, wp)
}

func (s *Service) GetWatchProgress(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	return s.progress.GetByProfileAndMedia(ctx, profileID, mediaItemID)
}

func (s *Service) GetWatchHistory(ctx context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	return s.progress.GetByProfile(ctx, profileID, limit, offset)
}

func (s *Service) GetResumeItems(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetResumeItems(ctx, profileID, limit)
}

func (s *Service) GetNextUp(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	return s.progress.GetNextUp(ctx, profileID, limit)
}

func (s *Service) MarkAsWatched(ctx context.Context, profileID, mediaItemID string) error {
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		Completed:    true,
		PlayCount:    1,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return err
	}
	s.events.Publish(ctx, playback.MediaMarkedWatched{
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		OccurredAt:  time.Now(),
	})
	return nil
}

func (s *Service) MarkAsUnwatched(ctx context.Context, profileID, mediaItemID string) error {
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		Completed:    false,
		PositionMs:   0,
		PlayCount:    0,
		LastPlayedAt: time.Now(),
	}
	return s.progress.Upsert(ctx, wp)
}
