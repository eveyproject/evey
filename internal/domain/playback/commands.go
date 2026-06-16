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
	CloseStale(ctx context.Context, olderThan time.Duration) (int, error)
	SetRemoteCommand(ctx context.Context, sessionID, command string) error
	ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error)
	// AcquireUserStreamLock takes a tx-scoped advisory lock keyed by userID
	// so the quota check+Create pair is serialized. No-op without an ambient tx.
	AcquireUserStreamLock(ctx context.Context, userID string) error
}
