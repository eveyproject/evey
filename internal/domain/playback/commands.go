package playback

import (
	"context"
	"time"
)

type ProgressCommandRepo interface {
	// Upsert writes the progress entry. Completed is sticky on conflict: a
	// row already marked completed=true cannot regress to false via Upsert -
	// only Delete (used by MarkAsUnwatched) clears the row. This protects
	// auto-completion at 90% and explicit MarkAsWatched from being undone
	// by a subsequent ReportProgressBySession tick.
	Upsert(ctx context.Context, wp *WatchProgress) error
	// UpsertMany is the batch form of Upsert - identical conflict semantics,
	// one statement for the whole flush instead of one round trip per viewer.
	// Entries MUST have distinct (profile, media) keys (ON CONFLICT cannot
	// touch a row twice in one statement); the flusher's dedup map guarantees
	// that. Empty input is a no-op.
	UpsertMany(ctx context.Context, entries []*WatchProgress) error
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
	// SetSessionAudioDecision rewrites ONLY the live audio_decision of an
	// ACTIVE session (ended_at IS NULL guard, like UpdateProgress). The cast
	// receiver's audio_mode beacon drives it so the admin badge follows the
	// REAL audio path (server AAC transcode vs TV-side wasm decode).
	SetSessionAudioDecision(ctx context.Context, sessionID, decision string) (bool, error)
	// SetSessionServing records what the server serves (per-track decision,
	// engine, backend) on an ACTIVE session; same ended_at guard.
	SetSessionServing(ctx context.Context, sessionID string, p ServingPath) (bool, error)
	// SetSessionAudioPath rewrites ONLY the audio leg (decision, engine and, when
	// audioCodec is non-empty, the codec label) of an ACTIVE session, in one
	// statement. The audio_mode beacon owns that leg alone; going through
	// SetSessionServing instead meant reading the whole path, swapping the audio
	// and writing it all back, so a master recorded in between (or a read that
	// missed a column) blanked video_engine. The video columns are never named.
	SetSessionAudioPath(ctx context.Context, sessionID, decision, engine, audioCodec string) (bool, error)
	CloseStale(ctx context.Context, olderThan time.Duration) (int, error)
	// CloseAllActive ends every session still open, regardless of age. Called
	// once as the server drains: the process is going away and every encode
	// with it, so no open session can survive the exit as anything but a
	// phantom - and the stale sweep would otherwise leave the youngest of them
	// claiming to be playing for two more minutes after the restart.
	// Returns the number of sessions ended.
	CloseAllActive(ctx context.Context) (int, error)
	// ReopenEndedBefore revives a session ended before `before` (the current
	// process start, so by the previous process's shutdown) with this tick's
	// state. A session ended since, by a StopStream, stays ended.
	ReopenEndedBefore(ctx context.Context, sessionID string, before time.Time, positionMs int64, isPaused bool, castDeviceName string) (bool, error)
	// PurgeEndedBefore deletes finished sessions (ended_at set) whose ended_at
	// is older than the cutoff, bounding the table's growth. Active sessions
	// (ended_at NULL) are never touched. Returns the number of rows removed.
	PurgeEndedBefore(ctx context.Context, cutoff time.Time) (int, error)
	SetRemoteCommand(ctx context.Context, sessionID, command string) error
	ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error)
	// AcquireUserStreamLock takes a tx-scoped advisory lock keyed by userID
	// so the quota check+Create pair is serialized. No-op without an ambient tx.
	AcquireUserStreamLock(ctx context.Context, userID string) error
}
