package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

var (
	ErrMaxStreams           = errors.New("max concurrent streams reached")
	ErrMaxDevicesPerProfile = errors.New("max concurrent devices per profile reached")
	ErrSessionNotFound      = errors.New("session not found")
	ErrProfileNotFound      = errors.New("profile not found")
)

// MediaItemDurationGetter returns the total duration of a media item in
// milliseconds. Wired from the library QueryService so the playback
// command service can auto-mark items as completed once playback crosses
// 90% without round-tripping client-side state - used by
// ReportProgressBySession. Returns (0, nil) when the duration is unknown
// (parent containers, songs scanned without probe data); the caller skips
// the auto-complete in that case.
type MediaItemDurationGetter interface {
	GetMediaItemDurationMs(ctx context.Context, mediaItemID string) (int64, error)
}

// CompletedThresholdNumerator/Denominator define the watched fraction at
// which a media item flips to completed=true. 9/10 = 90%, matching the
// Jellyfin default and the watch_history analyzer's existing assumption.
const (
	CompletedThresholdNumerator   = 9
	CompletedThresholdDenominator = 10
)

// MinAutoResumeDurationMs is the floor below which a mid-stream progress
// report is not persisted. Items shorter than 5 minutes (trailers, music
// videos, intros, sound effects) flooding "Continue Watching" with a few
// seconds of progress is noise, not signal - Jellyfin uses the same
// threshold (MinResumeDurationSec=300). Completion at 90% still writes
// through (so the row exists and "watched" counts), only intermediate
// position updates are dropped for short items.
const MinAutoResumeDurationMs int64 = 300_000

type CommandService struct {
	progress       playback.ProgressCommandRepo
	progressBuffer *WatchProgressBuffer
	sessions       playback.SessionCommandRepo
	sessionQueries playback.SessionQueryRepo
	users          user.UserRepository
	profiles       user.ProfileRepository
	events         shared.EventBus
	uow            shared.TransactionalUnit
	durations      MediaItemDurationGetter
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

// WithMediaItemDurationGetter enables auto-completion at 90% inside
// ReportProgressBySession. Without it, the call falls back to writing
// Completed=false on every tick (the historical behaviour - completion
// still happens via MarkAsWatched or the sticky-true Upsert semantic
// once any caller has set it).
func WithMediaItemDurationGetter(g MediaItemDurationGetter) CommandServiceOption {
	return func(s *CommandService) { s.durations = g }
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
		if errors.Is(err, shared.ErrNotFound) {
			return ErrProfileNotFound
		}
		// A transient lookup error must not masquerade as a permanent 404 - the
		// caller would map it to NotFound, the client would stop retrying, and the
		// progress/stop/mark write would be silently dropped.
		return fmt.Errorf("validate profile: %w", err)
	}
	return nil
}

// progressDecision bundles the per-tick judgments derived from the media
// duration: whether to flip Completed and whether the mid-stream tick
// should be persisted at all. Both fields are best-effort - the absence of
// a duration source defaults to "no completion, persist always" so the
// historical behaviour is preserved.
type progressDecision struct {
	completed bool
	// skipPersist is true when the media is too short to warrant a resume
	// entry (MinAutoResumeDurationMs floor). Completion still writes
	// through; only the "user paused at 0:15 of a 90s trailer" entries
	// are dropped to keep "Continue Watching" focused.
	skipPersist bool
}

// classifyProgress derives both decisions from a single duration lookup,
// avoiding two round-trips for what's a hot per-tick path.
func (s *CommandService) classifyProgress(ctx context.Context, mediaItemID string, positionMs int64) progressDecision {
	if s.durations == nil || positionMs <= 0 {
		return progressDecision{}
	}
	durMs, err := s.durations.GetMediaItemDurationMs(ctx, mediaItemID)
	if err != nil {
		slog.Debug("auto-complete: duration lookup failed",
			slog.String("mediaItemID", mediaItemID),
			slog.Any("err", err))
		return progressDecision{}
	}
	if durMs <= 0 {
		return progressDecision{}
	}
	// positionMs is client-controlled and reaches here unvalidated (only > 0 is
	// guaranteed above). A position at or beyond the real, server-side duration
	// is trivially complete - short-circuit it. This also prevents the
	// positionMs*Denominator multiply below from overflowing int64 on a forged
	// out-of-range positionMs: only an absurd value (≫ any real durMs) could
	// overflow, and such a value is necessarily >= durMs, so it's caught here.
	if positionMs >= durMs {
		return progressDecision{completed: true}
	}
	// Integer threshold check: positionMs / durMs >= 9/10  ⇔  positionMs*10 >= durMs*9
	// Safe from overflow now: positionMs < durMs (a real, bounded duration).
	completed := positionMs*int64(CompletedThresholdDenominator) >= durMs*int64(CompletedThresholdNumerator)
	return progressDecision{
		completed:   completed,
		skipPersist: !completed && durMs < MinAutoResumeDurationMs,
	}
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

	// deduped reports that the claim below resolved to an already-active session
	// (a double-click / retry) instead of creating a new row, so the caller can
	// skip re-publishing PlaybackStarted for a stream that already started.
	var deduped bool
	claim := func(ctx context.Context) error {
		// Take the per-user advisory lock unconditionally: it serializes the
		// idempotency dedup (so two concurrent double-click StartStream calls
		// can't both miss the existing session and both create a row) as well as
		// the count-then-create for the stream and device caps below. The lock
		// is tx-scoped and keyed per user, so the cost is negligible.
		if err := s.sessions.AcquireUserStreamLock(ctx, userID); err != nil {
			return fmt.Errorf("acquire stream lock: %w", err)
		}
		// One GetActiveByProfile feeds both the idempotency check and the device
		// cap (no second round-trip).
		active, err := s.sessionQueries.GetActiveByProfile(ctx, string(session.ProfileID))
		if err != nil {
			return fmt.Errorf("list active by profile: %w", err)
		}
		// Idempotency: at most one active session per (profile, media, device).
		// The player mints a fresh server-side ID on every StartStream, so a
		// retry or a double-click would otherwise spawn a phantom session that
		// inflates analytics and the device count. Reuse the live session and
		// return it verbatim to the caller instead of creating a duplicate.
		for _, a := range active {
			if a.MediaItemID == session.MediaItemID && a.DeviceName == session.DeviceName {
				*session = *a
				deduped = true
				return nil
			}
		}
		if u.HasStreamsLimit() {
			count, err := s.sessionQueries.CountActiveByUser(ctx, userID)
			if err != nil {
				return fmt.Errorf("count active: %w", err)
			}
			if u.StreamQuotaExceeded(count) {
				return ErrMaxStreams
			}
		}
		if u.HasDeviceLimit() && u.DeviceQuotaExceeded(len(active)) {
			return ErrMaxDevicesPerProfile
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

	// A deduped start replays an already-running session; PlaybackStarted was
	// already published when it was first created, so don't emit it twice.
	if deduped {
		return nil
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

// ReportProgressBySession records a playback tick. castDeviceName is the
// remote device the media actually plays on when the reporting client is a
// cast relay (web sender → Chromecast); empty = local playback (and clears a
// previous relay). The position/isPaused then describe that remote playback.
func (s *CommandService) ReportProgressBySession(ctx context.Context, sessionID string, positionMs int64, isPaused bool, castDeviceName string) error {
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
	safePositionMs := positionMs
	if dErr != nil {
		slog.Warn("invalid report progress position", slog.String("sessionID", sessionID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
		safePositionMs = 0
	} else {
		session.Position = &pos
	}
	session.IsPaused = isPaused
	session.CastDeviceName = castDeviceName
	// Targeted, ended_at-guarded write: a tick that races an in-flight
	// StopStream must not resurrect the stopped session (a full read-modify-
	// write would rewrite ended_at back to NULL from this stale snapshot). A
	// no-op here (session already ended) still records watch-progress below.
	if _, err := s.sessions.UpdateProgress(ctx, sessionID, safePositionMs, isPaused, castDeviceName); err != nil {
		return fmt.Errorf("update session position: %w", err)
	}
	// Auto-completion: when a duration source is wired and the position
	// crosses 90% of the media duration, flip Completed=true. The repo's
	// Upsert is sticky-true on conflict, so once we flip it the flag
	// survives any subsequent tick that drops below 90% (a user seeking
	// back). Skipped when the getter is absent or duration is unknown.
	decision := s.classifyProgress(ctx, string(session.MediaItemID), positionMs)
	completed := decision.completed
	// Short items (< 5 min) skip mid-stream progress writes - Continue
	// Watching should not surface a trailer at 0:15. Session position was
	// already updated above so the live UI still gets its tick; we just
	// don't materialise a watch_progress row until completion.
	if decision.skipPersist {
		return nil
	}
	// First-time completion seeds play_count=1. On INSERT (no prior row)
	// the value is used directly; on ON CONFLICT the SQL's transition
	// branch handles the actual increment from the previous play_count,
	// so this seed only matters for the fresh-insert path.
	playCount := 0
	if completed {
		playCount = 1
	}
	// WatchProgress.Position is non-negative by construction; on a corrupt
	// input we already logged above and pos is zero - keep buffering forward
	// so the player UI can still record "session is alive" via IsPaused.
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    session.ProfileID,
		MediaItemID:  session.MediaItemID,
		Position:     pos,
		Completed:    completed,
		PlayCount:    playCount,
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
		// Drop a malformed (e.g. negative) position rather than persisting a
		// zero-valued Duration, which would silently reset the user's resume
		// point to the start. ReportProgressBySession guards this the same way.
		slog.Warn("invalid report progress position", slog.String("profileID", profileID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
		return nil
	}
	// Caller-supplied completed is taken as-is when true (explicit
	// MarkAsWatched-style intent). When the caller says false we still
	// auto-complete at 90% via the duration getter, so CLI/non-web clients
	// that just stream raw position get the same Completed semantics as
	// the web/mobile players which compute completion client-side.
	if !completed {
		decision := s.classifyProgress(ctx, mediaItemID, positionMs)
		completed = decision.completed
		// Short items don't get a mid-stream progress row (Jellyfin parity).
		if decision.skipPersist {
			return nil
		}
	}
	playCount := 0
	if completed {
		playCount = 1
	}
	wp := &playback.WatchProgress{
		ID:           id.New("wp-"),
		ProfileID:    shared.ProfileID(profileID),
		MediaItemID:  shared.MediaItemID(mediaItemID),
		Position:     pos,
		Completed:    completed,
		PlayCount:    playCount,
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

// StartCleanup runs the stale-session cleanup loop with default parameters.
// The optional initDone channel(s) are closed after the first cleanup pass.
//
// The client stop (POST /playback/stop) is best-effort - a keepalive fetch that
// browsers can drop on navigation/unload - so the SERVER must be the authority
// on when an abandoned session ends. The reliable signal is the 3s progress
// heartbeat, which keeps firing even while paused; a session with no heartbeat
// for 2 min is genuinely gone (player closed, or the connection died), so reap
// it then rather than lingering for 5 min as a phantom "now playing". A live or
// merely-paused session beats every ~3s and is never at risk. Sweep every 30s so
// the end-to-end teardown lands within ~2-2.5 min of close even when the client
// beacon never arrives. CloseStale only sets ended_at - the resume position is
// preserved (last heartbeat / final report already persisted it).
func (s *CommandService) StartCleanup(ctx context.Context, initDone ...chan<- struct{}) {
	s.StartCleanupWithInterval(ctx, 30*time.Second, 2*time.Minute, initDone...)
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

func (s *CommandService) StartCleanupWithInterval(ctx context.Context, interval, maxAge time.Duration, initDone ...chan<- struct{}) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in playback cleanup", slog.Any("recover", r))
			}
		}()
		s.runCleanup(ctx, maxAge)
		for _, ch := range initDone {
			if ch != nil {
				close(ch)
			}
		}
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
	// Delete instead of upsert because Upsert keeps completed sticky-true:
	// writing a Completed=false row would not actually flip the flag back.
	// Deleting the row makes the item appear never-watched, matching the
	// Jellyfin semantics callers expect.
	if err := s.progress.Delete(ctx, profileID, mediaItemID); err != nil {
		return fmt.Errorf("CommandService.MarkAsUnwatched: %w", err)
	}
	return nil
}

// ClearWatchHistory deletes every watch-progress row for the profile and
// returns how many were removed.
func (s *CommandService) ClearWatchHistory(ctx context.Context, profileID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.ClearWatchHistory")
	defer span.End()
	if err := s.validateProfile(ctx, shared.ProfileID(profileID)); err != nil {
		return 0, err
	}
	n, err := s.progress.DeleteByProfile(ctx, profileID)
	if err != nil {
		return 0, fmt.Errorf("CommandService.ClearWatchHistory: %w", err)
	}
	return n, nil
}
