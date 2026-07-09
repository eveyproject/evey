package playback

import (
	"context"
	"time"
)

type ProgressCommandRepo interface {
	// Upsert writes the progress entry. Completed is sticky on conflict: a
	// row already marked completed=true cannot regress to false via Upsert —
	// only Delete (used by MarkAsUnwatched) clears the row. This protects
	// auto-completion at 90% and explicit MarkAsWatched from being undone
	// by a subsequent ReportProgressBySession tick.
	Upsert(ctx context.Context, wp *WatchProgress) error
	// Delete removes the (profile, media) progress entry entirely, so the
	// item appears never-watched again. Used by MarkAsUnwatched. No-op
	// when no row exists.
	Delete(ctx context.Context, profileID, mediaItemID string) error
	// DeleteByProfile removes every watch-progress row for the profile (clear
	// watch history). Returns the number of rows deleted.
	DeleteByProfile(ctx context.Context, profileID string) (int, error)
}

type SessionCommandRepo interface {
	Create(ctx context.Context, session *PlaySession) error
	Update(ctx context.Context, session *PlaySession) error
	// UpdateProgress writes ONLY the live-playback fields (position, pause,
	// cast relay) and is guarded on the session still being active
	// (ended_at IS NULL). A progress tick that races an in-flight StopStream
	// must never be able to rewrite ended_at back to NULL from a stale
	// snapshot and resurrect a stopped session - so progress reports go
	// through this targeted update instead of a full read-modify-write.
	// Returns whether a row was updated (false = the session already ended;
	// the tick is a no-op for the live session, watch-progress still records).
	UpdateProgress(ctx context.Context, sessionID string, positionMs int64, isPaused bool, castDeviceName string) (bool, error)
	CloseStale(ctx context.Context, olderThan time.Duration) (int, error)
	SetRemoteCommand(ctx context.Context, sessionID, command string) error
	ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error)
	// AcquireUserStreamLock takes a tx-scoped advisory lock keyed by userID
	// so the quota check+Create pair is serialized. No-op without an ambient tx.
	AcquireUserStreamLock(ctx context.Context, userID string) error
}
