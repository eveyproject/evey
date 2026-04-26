package playback

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type CommandService struct {
	progress         playback.ProgressCommandRepo
	progressBuffer   *WatchProgressBuffer
	sessions         playback.SessionCommandRepo
	sessionQueries   playback.SessionQueryRepo
	users            user.UserRepository
	profiles         user.ProfileRepository
	events           shared.EventBus
	uow              shared.TransactionalUnit
}

type CommandServiceOption func(*CommandService)

// WithUnitOfWork wires a transactional unit so StartStream can serialize
// the quota-check + session-create via a Postgres advisory lock,
// preventing the TOCTOU race where N concurrent requests bypass MaxStreams.
func WithUnitOfWork(uow shared.TransactionalUnit) CommandServiceOption {
	return func(s *CommandService) { s.uow = uow }
}

// WithWatchProgressBuffer routes ReportProgressBySession through an in-memory
// buffer flushed periodically. Without it, every player tick (~5s/stream)
// upserts the DB directly. Pass nil to keep the synchronous path (tests).
func WithWatchProgressBuffer(buf *WatchProgressBuffer) CommandServiceOption {
	return func(s *CommandService) { s.progressBuffer = buf }
}

func NewCommandService(
	progress playback.ProgressCommandRepo,
	sessions playback.SessionCommandRepo,
	sessionQueries playback.SessionQueryRepo,
	users user.UserRepository,
	profiles user.ProfileRepository,
	events shared.EventBus,
	opts ...CommandServiceOption,
) *CommandService {
	s := &CommandService{
		progress:       progress,
		sessions:       sessions,
		sessionQueries: sessionQueries,
		users:          users,
		profiles:       profiles,
		events:         events,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *CommandService) validateProfile(ctx context.Context, profileID shared.ProfileID) error {
	if _, err := s.profiles.GetByID(ctx, profileID); err != nil {
		return ErrProfileNotFound
	}
	return nil
}

func (s *CommandService) StartStream(ctx context.Context, session *playback.PlaySession, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.StartStream")
	defer span.End()
	if err := session.Validate(); err != nil {
		return fmt.Errorf("invalid play session: %w", err)
	}
	if err := s.validateProfile(ctx, session.ProfileID); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	session.StartedAt = time.Now()

	claim := func(ctx context.Context) error {
		if u.HasStreamsLimit() {
			if err := s.sessions.AcquireUserStreamLock(ctx, userID); err != nil {
				return fmt.Errorf("acquire stream lock: %w", err)
			}
			count, err := s.sessionQueries.CountActiveByUser(ctx, userID)
			if err != nil {
				return fmt.Errorf("count active: %w", err)
			}
			if u.StreamQuotaExceeded(count) {
				return ErrMaxStreams
			}
		}
		if err := s.sessions.Create(ctx, session); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	}

	if s.uow != nil {
		if err := s.uow.Do(ctx, claim); err != nil {
			return err
		}
	} else {
		if err := claim(ctx); err != nil {
			return err
		}
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
	ctx, span := tracing.StartSpan(ctx, "app.playback.StopStream")
	defer span.End()
	session, err := s.sessionQueries.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	now := time.Now()
	session.EndedAt = &now
	if dur, dErr := shared.NewDurationMs(now.Sub(session.StartedAt).Milliseconds()); dErr == nil {
		session.Duration = &dur
	}
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
	ctx, span := tracing.StartSpan(ctx, "app.playback.ReportProgressBySession")
	defer span.End()
	session, err := s.sessionQueries.GetByID(ctx, sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	// Negative positions from a misbehaving client get logged and dropped;
	// IsPaused still gets recorded so pause UI doesn't desync. shared.Duration
	// rejects negatives by construction.
	pos, dErr := shared.NewDurationMs(positionMs)
	if dErr != nil {
		slog.Warn("invalid report progress position", slog.String("sessionID", sessionID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
	} else {
		session.Position = &pos
	}
	session.IsPaused = isPaused
	if err := s.sessions.Update(ctx, session); err != nil {
		return fmt.Errorf("update session position: %w", err)
	}
	// WatchProgress.Position is non-negative by construction; on a corrupt
	// input we already logged above and pos is zero — keep buffering forward
	// so the player UI can still record "session is alive" via IsPaused.
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    session.ProfileID,
		MediaItemID:  session.MediaItemID,
		Position:     pos,
		Completed:    false,
		LastPlayedAt: time.Now(),
	}
	// Buffer-mode keeps the player tick off the DB hot path; the periodic
	// flusher absorbs N ticks per (profile, media) into a single upsert.
	// Crash window = flush interval. Synchronous fallback preserves the
	// old behaviour for tests and any caller that opts out of buffering.
	if s.progressBuffer != nil {
		s.progressBuffer.Enqueue(wp)
		return nil
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.ReportProgressBySession: %w", err)
	}
	return nil
}

func (s *CommandService) ReportProgress(ctx context.Context, profileID, mediaItemID string, positionMs int64, completed bool) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.ReportProgress")
	defer span.End()
	if err := s.validateProfile(ctx, shared.ProfileID(profileID)); err != nil {
		return err
	}
	pos, dErr := shared.NewDurationMs(positionMs)
	if dErr != nil {
		slog.Warn("invalid report progress position", slog.String("profileID", profileID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    shared.ProfileID(profileID),
		MediaItemID:  shared.MediaItemID(mediaItemID),
		Position:     pos,
		Completed:    completed,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.ReportProgress: %w", err)
	}
	return nil
}

func (s *CommandService) MarkAsWatched(ctx context.Context, profileID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.MarkAsWatched")
	defer span.End()
	if err := s.validateProfile(ctx, shared.ProfileID(profileID)); err != nil {
		return err
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    shared.ProfileID(profileID),
		MediaItemID:  shared.MediaItemID(mediaItemID),
		Completed:    true,
		PlayCount:    1,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.MarkAsWatched: %w", err)
	}
	evtWatched := playback.MediaMarkedWatched{
		ProfileID:   shared.ProfileID(profileID),
		MediaItemID: shared.MediaItemID(mediaItemID),
		OccurredAt:  time.Now(),
	}
	if err := s.events.Publish(ctx, evtWatched); err != nil {
		slog.Warn("publish event", slog.String("event", evtWatched.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) GetSession(ctx context.Context, sessionID string) (*playback.PlaySession, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.GetSession")
	defer span.End()
	return s.sessionQueries.GetByID(ctx, sessionID)
}

func (s *CommandService) SetRemoteCommand(ctx context.Context, sessionID, command string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.SetRemoteCommand")
	defer span.End()
	return s.sessions.SetRemoteCommand(ctx, sessionID, command)
}

func (s *CommandService) ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.ConsumeRemoteCommand")
	defer span.End()
	return s.sessions.ConsumeRemoteCommand(ctx, sessionID)
}

func (s *CommandService) CleanupStaleSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.CleanupStaleSessions")
	defer span.End()
	return s.sessions.CloseStale(ctx, maxAge)
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	s.StartCleanupWithInterval(ctx, 2*time.Minute, 5*time.Minute)
}

// StartProgressFlusher launches the watch-progress flusher when one is wired.
// No-op when WithWatchProgressBuffer wasn't used. Safe to call from main.
func (s *CommandService) StartProgressFlusher(ctx context.Context) {
	if s.progressBuffer == nil {
		return
	}
	s.progressBuffer.Start(ctx)
}

// StopProgressFlusher drains and stops the flusher if any. Call during
// graceful shutdown so the last 0-5s of progress reaches the DB before exit.
func (s *CommandService) StopProgressFlusher() {
	if s.progressBuffer == nil {
		return
	}
	s.progressBuffer.Stop()
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
	ctx, span := tracing.StartSpan(ctx, "app.playback.MarkAsUnwatched")
	defer span.End()
	if err := s.validateProfile(ctx, shared.ProfileID(profileID)); err != nil {
		return err
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    shared.ProfileID(profileID),
		MediaItemID:  shared.MediaItemID(mediaItemID),
		Completed:    false,
		PlayCount:    0,
		LastPlayedAt: time.Now(),
	}
	if err := s.progress.Upsert(ctx, wp); err != nil {
		return fmt.Errorf("CommandService.MarkAsUnwatched: %w", err)
	}
	return nil
}
