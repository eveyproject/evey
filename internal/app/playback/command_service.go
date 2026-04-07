package playback

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type CommandService struct {
	progress       playback.ProgressCommandRepo
	sessions       playback.SessionCommandRepo
	sessionQueries playback.SessionQueryRepo
	users          user.UserRepository
	profiles       user.ProfileRepository
	events         shared.EventBus
}

func NewCommandService(
	progress playback.ProgressCommandRepo,
	sessions playback.SessionCommandRepo,
	sessionQueries playback.SessionQueryRepo,
	users user.UserRepository,
	profiles user.ProfileRepository,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		progress:       progress,
		sessions:       sessions,
		sessionQueries: sessionQueries,
		users:          users,
		profiles:       profiles,
		events:         events,
	}
}

func (s *CommandService) validateProfile(ctx context.Context, profileID string) error {
	if _, err := s.profiles.GetByID(ctx, profileID); err != nil {
		return ErrProfileNotFound
	}
	return nil
}

func (s *CommandService) StartStream(ctx context.Context, session *playback.PlaySession, userID string) error {
	if err := session.Validate(); err != nil {
		return fmt.Errorf("invalid play session: %w", err)
	}
	if err := s.validateProfile(ctx, session.ProfileID); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if u.HasStreamsLimit() {
		count, err := s.sessionQueries.CountActiveByUser(ctx, userID)
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
	evtStarted := playback.PlaybackStarted{
		SessionID:   session.ID,
		ProfileID:   session.ProfileID,
		MediaItemID: session.MediaItemID,
		PlayMethod:  session.PlayMethod,
		OccurredAt:  time.Now(),
	}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.Warn("publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) StopStream(ctx context.Context, sessionID string) error {
	session, err := s.sessionQueries.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	now := time.Now()
	session.EndedAt = &now
	durationMs := now.Sub(session.StartedAt).Milliseconds()
	session.DurationMs = &durationMs
	if err := s.sessions.Update(ctx, session); err != nil {
		return fmt.Errorf("CommandService.StopStream: %w", err)
	}
	evtStopped := playback.PlaybackStopped{
		SessionID:   sessionID,
		ProfileID:   session.ProfileID,
		MediaItemID: session.MediaItemID,
		OccurredAt:  now,
	}
	if err := s.events.Publish(ctx, evtStopped); err != nil {
		slog.Warn("publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) ReportProgressBySession(ctx context.Context, sessionID string, positionMs int64, isPaused bool) error {
	session, err := s.sessionQueries.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	session.PositionMs = &positionMs
	session.IsPaused = isPaused
	if err := s.sessions.Update(ctx, session); err != nil {
		return fmt.Errorf("update session position: %w", err)
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    session.ProfileID,
		MediaItemID:  session.MediaItemID,
		PositionMs:   positionMs,
		Completed:    false,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.ReportProgressBySession: %w", err)
	}
	return nil
}

func (s *CommandService) ReportProgress(ctx context.Context, profileID, mediaItemID string, positionMs int64, completed bool) error {
	if err := s.validateProfile(ctx, profileID); err != nil {
		return err
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		PositionMs:   positionMs,
		Completed:    completed,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.ReportProgress: %w", err)
	}
	return nil
}

func (s *CommandService) MarkAsWatched(ctx context.Context, profileID, mediaItemID string) error {
	if err := s.validateProfile(ctx, profileID); err != nil {
		return err
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		Completed:    true,
		PlayCount:    1,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.MarkAsWatched: %w", err)
	}
	evtWatched := playback.MediaMarkedWatched{
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		OccurredAt:  time.Now(),
	}
	if err := s.events.Publish(ctx, evtWatched); err != nil {
		slog.Warn("publish event", slog.String("event", evtWatched.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) GetSession(ctx context.Context, sessionID string) (*playback.PlaySession, error) {
	return s.sessionQueries.GetByID(ctx, sessionID)
}

func (s *CommandService) SetRemoteCommand(ctx context.Context, sessionID, command string) error {
	return s.sessions.SetRemoteCommand(ctx, sessionID, command)
}

func (s *CommandService) ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error) {
	return s.sessions.ConsumeRemoteCommand(ctx, sessionID)
}

func (s *CommandService) CleanupStaleSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	return s.sessions.CloseStale(ctx, maxAge)
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	s.StartCleanupWithInterval(ctx, 2*time.Minute, 5*time.Minute)
}

func (s *CommandService) StartCleanupWithInterval(ctx context.Context, interval, maxAge time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in playback cleanup", slog.Any("recover", r))
			}
		}()
		s.runCleanup(ctx, maxAge)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runCleanup(ctx, maxAge)
			}
		}
	}()
}

func (s *CommandService) runCleanup(ctx context.Context, maxAge time.Duration) {
	if n, err := s.sessions.CloseStale(ctx, maxAge); err != nil {
		slog.Warn("stale session cleanup", slog.Any("err", err))
	} else if n > 0 {
		slog.Info("cleaned stale sessions", slog.Int("count", n))
	}
}

func (s *CommandService) MarkAsUnwatched(ctx context.Context, profileID, mediaItemID string) error {
	if err := s.validateProfile(ctx, profileID); err != nil {
		return err
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
		Completed:    false,
		PositionMs:   0,
		PlayCount:    0,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.MarkAsUnwatched: %w", err)
	}
	return nil
}
