package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
// which a media item flips to completed=true. 9/10 = 90%, the de-facto
// standard threshold and the watch_history analyzer's existing assumption.
const (
	CompletedThresholdNumerator   = 9
	CompletedThresholdDenominator = 10
)

// MinAutoResumeDurationMs is the floor below which a mid-stream progress
// report is not persisted. Items shorter than 5 minutes (trailers, music
// videos, intros, sound effects) flooding "Continue Watching" with a few
// seconds of progress is noise, not signal - 300 s is the classic floor
// for this exact reason. Completion at 90% still writes
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
	// sessionRetention bounds play_sessions: finished sessions older than the
	// returned number of days are purged. 0 (or nil getter) = keep forever.
	// A GETTER rather than a snapshot so the admin panel's change applies at
	// the next sweep without a restart (it reads config under its lock).
	// lastSessionPurge gates the delete to ~once/day inside the 30s
	// stale-session sweep (single-goroutine, no lock).
	sessionRetention func() int
	lastSessionPurge time.Time
	// orphanCastCommands reaches a Chromecast that is still beating but whose
	// session has ended - see cast_orphan_commands.go.
	orphanCastCommands *castOrphanCommands
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
// WithSessionRetention enables the daily purge of finished play_sessions older
// than the number of days the getter returns. 0 keeps every session. Re-read
// at every sweep so a panel change needs no restart.
func WithSessionRetention(days func() int) CommandServiceOption {
	return func(s *CommandService) { s.sessionRetention = days }
}

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
		// Always built, never optional: a nil store would make the control
		// silently do nothing, which is the failure it exists to end.
		orphanCastCommands: newCastOrphanCommands(),
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
		slog.DebugContext(ctx, "auto-complete: duration lookup failed",
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

func (s *CommandService) StartStream(ctx context.Context, session *playback.PlaySession, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.StartStream")
	defer span.End()
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
		if err := s.sessions.AcquireUserStreamLock(ctx, string(userID)); err != nil {
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
			count, err := s.sessionQueries.CountActiveByUser(ctx, string(userID))
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtStopped.EventName()), slog.Any("err", err))
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
	// A session that has never reported a positive position has not played
	// yet: a player stuck before its first frame (codec refused, network gone)
	// ticks at 0, and the freshest write wins, so that 0 erased the resume
	// point. Measured in prod on 2026-09-23. The tick still updates the
	// session below; only the watch-progress write waits for a real position.
	neverPlayed := session.Position == nil || session.Position.Milliseconds() <= 0
	pos, dErr := shared.NewDurationMs(positionMs)
	safePositionMs := positionMs
	if dErr != nil {
		slog.WarnContext(ctx, "invalid report progress position", slog.String("sessionID", sessionID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
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
	if decision.skipPersist || (neverPlayed && safePositionMs <= 0) {
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
		slog.WarnContext(ctx, "invalid report progress position", slog.String("profileID", profileID), slog.Int64("ms", positionMs), slog.Any("err", dErr))
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
		// Short items don't get a mid-stream progress row (see the floor above).
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtWatched.EventName()), slog.Any("err", err))
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

// CastBeacon is one liveness beat from a cast receiver, as TouchCastSession
// reads it.
//
// A struct rather than a parameter list because the two booleans mean opposite
// kinds of thing and sit next to each other: swapping them at a call site would
// compile, and would silently turn "a paused television" into "any beat may
// resurrect a session".
type CastBeacon struct {
	ProfileID   string
	MediaItemID string
	PositionMs  int64
	// AllowCreate permits creating a session when none is active. True ONLY for
	// cast_receiver_loaded, which fires once per media load; an alive beat that
	// could create would let a stalled receiver rebuild the phantom row the
	// reaper has just cleared.
	AllowCreate bool
	// Paused is what the receiver says its player state is. A pause is a human
	// act and therefore proof of life on its own, even though the playhead is
	// frozen - which is what separates it from a stall, where the receiver
	// still believes it is playing. A receiver too old to report it leaves this
	// false, which falls back to judging on the playhead alone.
	Paused bool
}

// castBeaconAdvanceMs is how far the playhead must move between two receiver
// beacons to count as proof of life.
//
// The beat is every 30s (ALIVE_BEACON_MS), so real playback advances ~30000ms;
// one second is a floor far below any genuine progress and above the float
// noise a stalled currentTime can jitter by (the beacon sends seconds as a
// float and we scale by 1000).
const castBeaconAdvanceMs = 1000

// castBeaconIsAlive reports whether a beacon's position proves the picture is
// still moving. A backwards jump counts: only a human seeks.
func castBeaconIsAlive(stored *shared.Duration, beaconMs int64) bool {
	if stored == nil {
		// Nothing recorded yet: the first positioned beacon is the only
		// evidence there is, and refusing it would strand a session that never
		// got a progress tick.
		return true
	}
	delta := beaconMs - stored.Milliseconds()
	if delta < 0 {
		delta = -delta
	}
	return delta >= castBeaconAdvanceMs
}

// TouchCastSession refreshes the ACTIVE session of (profile, media) from a cast
// receiver's liveness beacon: position + updated_at, through the same guarded
// UpdateProgress the phone's ticks use (a racing StopStream can never be
// resurrected). During a cast the PHONE owns ReportProgress, and an OEM freeze
// silences it - without this, the stale reaper ends the session ~2min later
// while the TV plays on, and the now-playing panel goes blind. The device name
// the phone set at cast start is preserved.
//
// ⚠️ A BEAT IS NOT PROOF OF LIFE - ADVANCEMENT IS.
//
// This used to refresh unconditionally, on the belief (stated in this comment,
// and wrong) that "the receiver beats only while playing". It does not:
// createAliveHeartbeat is a plain setInterval started at PLAYER_LOAD_COMPLETE
// and stopped only at MEDIA_FINISHED / SHUTDOWN / pagehide, so it keeps beating
// through a stall - and a television that is never really switched off keeps
// the receiver page loaded indefinitely.
//
// Because UpdateProgress writes updated_at=NOW() whether or not the position
// moved, one beat every 30s made the 2-minute stale reaper structurally
// unreachable. Measured on prod 2026-09-04: a session held open 3h26, playhead
// frozen at 19min16 across every sample, is_paused false, videoSessions.active
// 0 - no encode, no viewer, an immortal row claiming to be playing.
//
// So a beacon whose playhead has not moved writes NOTHING. updated_at stays
// where the picture last advanced, and the ordinary reaper ends the session
// ~2min later. That 2-minute window IS the stall tolerance; it needs no state
// and no second timer.
//
// Consequence to know: a cast PAUSED with the phone gone also ages out, because
// the server then holds no evidence at all that anyone is still there (the
// beacon carries no pause flag; the phone's own ticks do, and they keep a
// paused session alive as before). Resuming re-creates it - see allowCreate.
//
// Returns whether an active session was found and refreshed.
func (s *CommandService) TouchCastSession(ctx context.Context, b CastBeacon) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.TouchCastSession")
	defer span.End()
	if b.ProfileID == "" || b.MediaItemID == "" || b.PositionMs < 0 {
		return false, nil
	}
	sessions, err := s.sessionQueries.GetActiveByProfile(ctx, b.ProfileID)
	if err != nil {
		return false, err
	}
	for _, sess := range sessions {
		if sess == nil || string(sess.MediaItemID) != b.MediaItemID {
			continue
		}
		// Two things count as life, and a stall is neither: the playhead moved,
		// or a human paused it. Everything else writes nothing.
		if !castBeaconIsAlive(sess.Position, b.PositionMs) && !b.Paused {
			// Deliberately no write at all: touching the row would bump
			// updated_at and re-arm the phantom.
			return false, nil
		}
		// IsPaused is now REPORTED by the receiver rather than assumed false.
		// A receiver too old to send it says nothing, which decodes to false -
		// and then only an advancing playhead got us here, so false is true.
		return s.sessions.UpdateProgress(ctx, sess.ID, b.PositionMs, b.Paused, sess.CastDeviceName)
	}
	if !b.AllowCreate {
		// A stalled receiver must not be able to re-create what the reaper just
		// closed. Only cast_receiver_loaded reaches here, and it fires once per
		// media load - which is exactly the event the resurrection exists for
		// (a re-cast from an already mounted player). An endless alive beat is
		// not a new playback and can no longer masquerade as one.
		return false, nil
	}
	// Resurrection: a positioned beacon with NO active session is a real
	// playback the panel cannot see - the phone re-cast from an already
	// mounted player (no new StartStream) after its previous session was
	// reaped (prod 2026-07-29). The beacon's token pins (profile, media), so
	// a minimal row makes the cast visible and pilotable again; the stale
	// reaper ends it when the beacons stop.
	pos, dErr := shared.NewDurationMs(b.PositionMs)
	if dErr != nil {
		return false, nil
	}
	// Built through the canonical constructor (ADR-0008) rather than a struct
	// literal, so the row is Validate()d like every other PlaySession. The
	// empty-identity cases are already refused at the top of this function;
	// what this adds is that the invariant travels with the type instead of
	// depending on that guard staying there, and that a future option carrying
	// an invalid PlayMethod cannot slip into the store.
	sess, cErr := playback.NewPlaySession(
		id.New("ps-"),
		shared.ProfileID(b.ProfileID),
		shared.MediaItemID(b.MediaItemID),
		playback.WithPlaySessionDevice("Chromecast", ""),
		playback.WithPlaySessionCastDevice("Chromecast"),
		playback.WithPlaySessionPosition(pos),
	)
	if cErr != nil {
		return false, nil
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return false, fmt.Errorf("TouchCastSession resurrect: %w", err)
	}
	return true, nil
}

// EndCastSession ends the ACTIVE session of (profile, media) on a receiver
// end-of-life beacon (cast_receiver_shutdown / cast_receiver_finished).
//
// Those beacons used to touch nothing, on the reasoning that "aging out is the
// point" - but aging out is exactly what the alive beat prevented, so the one
// moment the TV explicitly announced the end was the one moment the server
// ignored. The receiver stops its heartbeat on both events (MEDIA_FINISHED,
// SHUTDOWN, pagehide), so this is the earliest and most reliable end signal
// there is: it beats waiting 2 minutes for silence, and it is the only signal
// at all when the sender phone is gone and no StopStream will ever arrive.
//
// Reuses StopStream so an ended cast is indistinguishable from any other ended
// session - same ended_at, same duration, same PlaybackStopped event, so watch
// progress and the now-playing panel behave identically. Missing or already
// ended sessions are a no-op: a receiver may legitimately send shutdown twice
// (SHUTDOWN then pagehide), and the second must not error.
func (s *CommandService) EndCastSession(ctx context.Context, profileID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.EndCastSession")
	defer span.End()
	if profileID == "" || mediaItemID == "" {
		return nil
	}
	sessions, err := s.sessionQueries.GetActiveByProfile(ctx, profileID)
	if err != nil {
		return err
	}
	for _, sess := range sessions {
		if sess == nil || string(sess.MediaItemID) != mediaItemID {
			continue
		}
		if err := s.StopStream(ctx, sess.ID); err != nil && !errors.Is(err, ErrSessionNotFound) {
			return fmt.Errorf("EndCastSession: %w", err)
		}
		return nil
	}
	return nil
}

// SetCastAudioPath mirrors a player's audio_mode beacon (cast receiver or web
// wasm player) onto the active session so the admin badge follows the REAL
// audio path: "aac" (and the aac-latched fallback) IS a server transcode;
// "wasm" means the player decodes and the server's ffmpeg dies at idle seconds
// later. Unknown modes are ignored - a future string must not corrupt the column.
func (s *CommandService) SetCastAudioPath(ctx context.Context, profileID, mediaItemID, mode string) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.SetCastAudioPath")
	defer span.End()
	var decision, engine string
	switch mode {
	case "aac", "aac-latched":
		decision, engine = "transcode", "ffmpeg"
	case "wasm":
		decision, engine = "wasm", "client"
	default:
		return nil
	}
	sess, err := s.activeSessionFor(ctx, profileID, mediaItemID)
	if err != nil || sess == nil {
		return err
	}
	// The audio leg only: the video columns are the master render's, and a
	// read-modify-write of the whole path let this beacon blank video_engine.
	_, err = s.sessions.SetSessionAudioPath(ctx, sess.ID, decision, engine, relayAudioCodecLabel(sess.AudioCodec, decision))
	return err
}

// relayAudioCodecLabel keeps the session's audio codec label pointed at what
// the viewer is actually fed when the relay changes leg. The label is written
// once at start-up from the client's own decision ("eac3" while the track is
// carried untouched), and SetCastAudioPath used to flip the DECISION without
// touching it: prod, 2026-09-20, a starved wasm leg left the dashboard reading
// "transcodé (eac3)" - a transcode whose output codec is its input.
//
// The relay's native leg is always Evey's AAC rendition, so the target is known
// here and nowhere else; returning "" means "leave the label alone".
func relayAudioCodecLabel(current, decision string) string {
	source, _, _ := strings.Cut(current, "→")
	if source == "" || source == relayNativeAudioCodec {
		return ""
	}
	if decision == "transcode" {
		return source + "→" + relayNativeAudioCodec
	}
	return source // back on the untouched track: the source IS what is served
}

// relayNativeAudioCodec is the codec of the relay's native leg (Evey's audio
// rendition re-encodes to AAC).
const relayNativeAudioCodec = "aac"

// SetSessionServing records what the master playlist just handed the
// (profile, media) session: which packager/encoder serves each track. Called
// on every master render, so a mid-session reshape (fallback rung, wasm
// latch-off) keeps the admin panel truthful.
func (s *CommandService) SetSessionServing(ctx context.Context, profileID, mediaItemID string, p playback.ServingPath) error {
	ctx, span := tracing.StartSpan(ctx, "app.playback.SetSessionServing")
	defer span.End()
	sess, err := s.activeSessionFor(ctx, profileID, mediaItemID)
	if err != nil || sess == nil {
		return err
	}
	_, err = s.sessions.SetSessionServing(ctx, sess.ID, p)
	return err
}

// activeSessionFor is the (profile, media) → live session lookup the beacon
// paths share; nil when nothing matches.
func (s *CommandService) activeSessionFor(ctx context.Context, profileID, mediaItemID string) (*playback.PlaySession, error) {
	if profileID == "" || mediaItemID == "" {
		return nil, nil
	}
	sessions, err := s.sessionQueries.GetActiveByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	for _, sess := range sessions {
		if sess != nil && string(sess.MediaItemID) == mediaItemID {
			return sess, nil
		}
	}
	return nil, nil
}

// ConsumeCastCommand hands the pending remote command of the ACTIVE
// (profile, media) session to the cast receiver polling via its alive beacon.
// During a cast the phone used to be the only consumer (ReportProgress), so a
// frozen phone took the admin's remote control down with it. Destructive
// read: whichever consumer polls first gets the command, the other sees none.
func (s *CommandService) ConsumeCastCommand(ctx context.Context, profileID, mediaItemID string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.ConsumeCastCommand")
	defer span.End()
	if profileID == "" || mediaItemID == "" {
		return "", nil
	}
	sessions, err := s.sessionQueries.GetActiveByProfile(ctx, profileID)
	if err != nil {
		return "", err
	}
	for _, sess := range sessions {
		if sess == nil || string(sess.MediaItemID) != mediaItemID {
			continue
		}
		if cmd, cErr := s.sessions.ConsumeRemoteCommand(ctx, sess.ID); cErr != nil || cmd != "" {
			return cmd, cErr
		}
		break
	}
	// No session, or a session with nothing pending: fall back to the command
	// parked for the profile itself. That is the only channel left to a receiver
	// still beating on a television that never switches off, whose session ended
	// when its picture stopped moving.
	return s.orphanCastCommands.take(profileID), nil
}

// SetCastReceiverCommand parks a command for the next cast beacon of this
// profile, whether or not it still has a play session.
//
// The session-bound SendSessionCommand cannot reach that receiver: there is no
// row to write on. And the receiver is not gone - it is listed in online
// devices, beating every 30s, holding the television. Before this the only way
// to release it was to unplug it.
//
// Delivery is best-effort and unacknowledged by design, exactly like the
// session path: the command rides the answer to the next beacon, so it lands
// within one beacon interval or expires unseen.
func (s *CommandService) SetCastReceiverCommand(ctx context.Context, profileID, command string) error {
	_, span := tracing.StartSpan(ctx, "app.playback.SetCastReceiverCommand")
	defer span.End()
	if profileID == "" || command == "" {
		return errors.New("SetCastReceiverCommand: profile id and command are required")
	}
	s.orphanCastCommands.set(profileID, command)
	return nil
}

// EndAllSessions closes every open play session. Called once while the server
// drains.
//
// Everything that made those sessions real dies with the process: the ffmpeg
// behind each transcode, the HLS orchestrators, the in-memory limiter. What
// survives is a row in play_sessions with ended_at NULL, which the admin panel
// reads as somebody watching. The stale sweep does eventually clear them, but
// only 2 minutes after their last heartbeat - so a restart handed the operator
// a now-playing list full of viewers who had been disconnected by the restart
// itself. Clients re-negotiate a fresh session on their next request; there is
// nothing here worth carrying across the exit.
func (s *CommandService) EndAllSessions(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.EndAllSessions")
	defer span.End()
	return s.sessions.CloseAllActive(ctx)
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
				slog.ErrorContext(ctx, "panic in playback cleanup", slog.Any("recover", r))
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
		slog.WarnContext(ctx, "stale session cleanup", slog.Any("err", err))
	} else if n > 0 {
		slog.InfoContext(ctx, "cleaned stale sessions", slog.Int("count", n))
	}

	// Retention purge of finished sessions, gated to ~once/day (the stale sweep
	// runs every 30s). lastSessionPurge is zero on boot, so the first sweep
	// purges immediately. The window is re-read here so a panel change applies
	// on the next sweep, not the next restart.
	retentionDays := 0
	if s.sessionRetention != nil {
		retentionDays = s.sessionRetention()
	}
	if retentionDays > 0 && time.Since(s.lastSessionPurge) >= 24*time.Hour {
		s.lastSessionPurge = time.Now()
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		if n, err := s.sessions.PurgeEndedBefore(ctx, cutoff); err != nil {
			slog.WarnContext(ctx, "play_session retention purge", slog.Any("err", err))
		} else if n > 0 {
			slog.InfoContext(ctx, "purged old play sessions", slog.Int("count", n), slog.Int("retention_days", retentionDays))
		}
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
	// Deleting the row makes the item appear never-watched - the semantics
	// callers expect from "mark as unwatched".
	if err := s.progress.Delete(ctx, profileID, mediaItemID); err != nil {
		return fmt.Errorf("CommandService.MarkAsUnwatched: %w", err)
	}
	return nil
}

// ClearWatchHistory deletes every watch-progress row for the profile and
// returns how many were removed.
func (s *CommandService) ClearWatchHistory(ctx context.Context, profileID shared.ProfileID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playback.ClearWatchHistory")
	defer span.End()
	if err := s.validateProfile(ctx, profileID); err != nil {
		return 0, err
	}
	n, err := s.progress.DeleteByProfile(ctx, string(profileID))
	if err != nil {
		return 0, fmt.Errorf("CommandService.ClearWatchHistory: %w", err)
	}
	return n, nil
}
