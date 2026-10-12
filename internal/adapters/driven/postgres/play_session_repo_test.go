package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlaySession(id, profileID, mediaItemID string) *playback.PlaySession {
	return &playback.PlaySession{
		ID:           id,
		ProfileID:    shared.ProfileID(profileID),
		MediaItemID:  shared.MediaItemID(mediaItemID),
		DeviceName:   "TestDevice",
		DeviceType:   "browser",
		ClientIP:     "127.0.0.1",
		PlayMethod:   string(playback.DirectPlay),
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Resolution:   "1920x1080",
		BandwidthBps: 8000000,
		StartedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestPlaySessionRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps1", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	got, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ProfileID, got.ProfileID)
	assert.Equal(t, session.DeviceName, got.DeviceName)
	assert.Equal(t, session.PlayMethod, got.PlayMethod)
}

func TestPlaySessionRepo_PurgeEndedBefore(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu_purge")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp_purge", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib_purge")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi_purge", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC()
	mkEnded := func(id string, endedAt time.Time) {
		s := newTestPlaySession(id, string(p.ID), string(item.ID))
		s.EndedAt = &endedAt
		require.NoError(t, sessionRepo.Create(ctx, s))
	}
	mkEnded("ps_old_ended", now.Add(-40*24*time.Hour))   // 40 days ago → purged
	mkEnded("ps_recent_ended", now.Add(-2*24*time.Hour)) // 2 days ago → kept
	// An active session (ended_at NULL) must never be purged regardless of age.
	active := newTestPlaySession("ps_active", string(p.ID), string(item.ID))
	active.StartedAt = now.Add(-100 * 24 * time.Hour)
	require.NoError(t, sessionRepo.Create(ctx, active))

	n, err := sessionRepo.PurgeEndedBefore(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the 40-day-old FINISHED session should be purged")

	_, err = sessionRepo.GetByID(ctx, "ps_old_ended")
	require.Error(t, err, "purged session must be gone")
	_, err = sessionRepo.GetByID(ctx, "ps_recent_ended")
	require.NoError(t, err, "recently-ended session must remain")
	_, err = sessionRepo.GetByID(ctx, "ps_active")
	require.NoError(t, err, "active session must remain even though it is old")
}

func TestPlaySessionRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps2", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	endedAt := time.Now().UTC().Truncate(time.Millisecond)
	dur, durErr := shared.NewDurationMs(120000)
	require.NoError(t, durErr)
	pos, err := shared.NewDurationMs(119000)
	require.NoError(t, err)
	session.EndedAt = &endedAt
	session.Duration = &dur
	session.Position = &pos
	require.NoError(t, sessionRepo.Update(ctx, session))

	got, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, got.EndedAt)
	require.NotNil(t, got.Duration)
	assert.Equal(t, int64(120000), got.Duration.Milliseconds())
}

func TestPlaySessionRepo_GetActiveByProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps3", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	// The list reads must carry the SAME columns as GetByID: the StartStream
	// dedup path copies a GetActiveByProfile row verbatim into the response,
	// so a partial SELECT silently blanks decisions/pause/cast on retries.
	session.VideoDecision = "copy"
	session.AudioDecision = "transcode"
	session.IsPaused = true
	session.CastDeviceName = "Living room TV"
	require.NoError(t, sessionRepo.Update(ctx, session))

	active, err := sessionRepo.GetActiveByProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, session.ID, active[0].ID)
	assert.Equal(t, "copy", active[0].VideoDecision)
	assert.Equal(t, "transcode", active[0].AudioDecision)
	assert.True(t, active[0].IsPaused)
	assert.Equal(t, "Living room TV", active[0].CastDeviceName)
}

func TestPlaySessionRepo_CloseStale(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps5", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	// with very short duration, the session should be "stale"
	closed, err := sessionRepo.CloseStale(ctx, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, closed, 1)
}

func TestPlaySessionRepo_CountActiveByUser(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu6")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp6", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi6", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps6", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	count, err := sessionRepo.CountActiveByUser(ctx, string(u.ID))
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestPlaySessionRepo_ListActiveSessions(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu7")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp7", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib7")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi7", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps7", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	active, err := sessionRepo.ListActiveSessions(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(active), 1)
}

// UpdateProgress is guarded on ended_at IS NULL: it updates a live session but
// is a no-op on an already-ended one, and NEVER clears ended_at. This is the
// property that stops a progress tick racing an in-flight StopStream from
// resurrecting the stopped session (the "Now Playing ghost + phantom quota"
// lost-update the audit traced on the normal player-close path).
func TestPlaySessionRepo_UpdateProgress_GuardedOnEndedAt(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu8")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp8", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib8")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi8", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("ps8", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	// Live session: the progress write lands and reports a row updated.
	updated, err := sessionRepo.UpdateProgress(ctx, session.ID, 42000, true, "Living room TV")
	require.NoError(t, err)
	assert.True(t, updated, "a live session must accept the progress tick")
	got, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Position)
	assert.Equal(t, int64(42000), got.Position.Milliseconds())
	assert.True(t, got.IsPaused)
	assert.Equal(t, "Living room TV", got.CastDeviceName)
	assert.Nil(t, got.EndedAt)

	// End the session the way StopStream does: read the FRESH row (carrying
	// the 42000 position UpdateProgress just wrote), set ended_at, Update.
	fresh, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	endedAt := time.Now().UTC().Truncate(time.Millisecond)
	fresh.EndedAt = &endedAt
	require.NoError(t, sessionRepo.Update(ctx, fresh))

	updated, err = sessionRepo.UpdateProgress(ctx, session.ID, 99000, false, "Someone Else TV")
	require.NoError(t, err)
	assert.False(t, updated, "a tick on an ended session must not update any row")

	after, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, after.EndedAt, "ended_at must stay set - no resurrection")
	assert.Equal(t, int64(42000), after.Position.Milliseconds(), "the ended session's fields must be untouched by the late tick")
	assert.Equal(t, "Living room TV", after.CastDeviceName)
}

// A draining server ends every open session in one statement; a remote
// command (admin "stop"/"pause") only lands on a session that is still open
// and is consumed exactly once.
func TestPlaySessionRepo_CloseAllActive_AndRemoteCommand(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu9")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp9", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib9")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi9", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	require.NoError(t, repo.Create(ctx, newTestPlaySession("ps9a", string(p.ID), string(item.ID))))
	require.NoError(t, repo.Create(ctx, newTestPlaySession("ps9b", string(p.ID), string(item.ID))))

	require.NoError(t, repo.SetRemoteCommand(ctx, "ps9a", "pause"))
	cmd, err := repo.ConsumeRemoteCommand(ctx, "ps9a")
	require.NoError(t, err)
	assert.Equal(t, "pause", cmd)
	cmd, err = repo.ConsumeRemoteCommand(ctx, "ps9a")
	require.NoError(t, err)
	assert.Empty(t, cmd, "a command is delivered once")

	closed, err := repo.CloseAllActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, closed)
	closed, err = repo.CloseAllActive(ctx)
	require.NoError(t, err)
	assert.Zero(t, closed, "nothing left open")
	active, err := repo.CountActiveByUser(ctx, string(u.ID))
	require.NoError(t, err)
	assert.Zero(t, active)

	require.NoError(t, repo.SetRemoteCommand(ctx, "ps9b", "stop"))
	cmd, err = repo.ConsumeRemoteCommand(ctx, "ps9b")
	require.NoError(t, err)
	assert.Empty(t, cmd, "an ended session never receives a command")
}

// The dashboard's theoretical outgoing rate: the source bitrates of the OPEN
// sessions only.
func TestPlaySessionStatsReader_GetActiveSessionsBitrate(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu-bw")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp-bw", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib-bw")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi-bw", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)
	stats := postgres.NewPlaySessionStatsReader(db)

	bps, err := stats.GetActiveSessionsBitrate(ctx)
	require.NoError(t, err)
	assert.Zero(t, bps)

	a := newTestPlaySession("ps-bw-a", string(p.ID), string(item.ID))
	a.BandwidthBps = 8_000_000
	b := newTestPlaySession("ps-bw-b", string(p.ID), string(item.ID))
	b.BandwidthBps = 12_000_000
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))
	bps, err = stats.GetActiveSessionsBitrate(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 20_000_000, bps)

	_, err = repo.CloseAllActive(ctx)
	require.NoError(t, err)
	bps, err = stats.GetActiveSessionsBitrate(ctx)
	require.NoError(t, err)
	assert.Zero(t, bps, "ended sessions no longer count")
}

// The cast receiver's audio_mode beacon rewrites the LIVE audio decision;
// a session that has ended is never rewritten, and the caller learns which.
func TestPlaySessionRepo_SetSessionAudioDecision_ActiveOnly(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu9")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp9", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib9")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi9", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	live := newTestPlaySession("ps9-live", string(p.ID), string(item.ID))
	live.AudioDecision = "copy"
	require.NoError(t, repo.Create(ctx, live))
	done := newTestPlaySession("ps9-done", string(p.ID), string(item.ID))
	done.AudioDecision = "copy"
	ended := time.Now().UTC()
	done.EndedAt = &ended
	require.NoError(t, repo.Create(ctx, done))

	ok, err := repo.SetSessionAudioDecision(ctx, live.ID, "transcode")
	require.NoError(t, err)
	assert.True(t, ok)
	got, err := repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision)

	ok, err = repo.SetSessionAudioDecision(ctx, done.ID, "transcode")
	require.NoError(t, err)
	assert.False(t, ok, "an ended session is left as it was")
	got, err = repo.GetByID(ctx, done.ID)
	require.NoError(t, err)
	assert.Equal(t, "copy", got.AudioDecision)

	ok, err = repo.SetSessionAudioDecision(ctx, "ps9-ghost", "transcode")
	require.NoError(t, err)
	assert.False(t, ok)
}

// SetSessionServing writes the audio codec label ONLY when the caller supplies
// one: a master render knows the engines but not the output codec, and an
// unconditional write would blank the label the session started with.
func TestPlaySessionRepo_SetSessionServing_KeepsTheCodecLabelUnlessGiven(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu10")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp10", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib10")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi10", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	live := newTestPlaySession("ps10-live", string(p.ID), string(item.ID))
	live.AudioCodec = "eac3"
	require.NoError(t, repo.Create(ctx, live))

	ok, err := repo.SetSessionServing(ctx, live.ID, playback.ServingPath{
		VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "wasm", AudioEngine: "client",
	})
	require.NoError(t, err)
	assert.True(t, ok)
	got, err := repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, "eac3", got.AudioCodec, "no codec supplied: the label stands")

	ok, err = repo.SetSessionServing(ctx, live.ID, playback.ServingPath{
		VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg", AudioCodec: "eac3→aac",
	})
	require.NoError(t, err)
	assert.True(t, ok)
	got, err = repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, "eac3→aac", got.AudioCodec)
	assert.Equal(t, "transcode", got.AudioDecision)
}

// A session ended by the previous process's shutdown comes back on its
// client's next tick, all the way to the now-playing list; one ended after
// the current process started (a StopStream) does not.
func TestPlaySessionRepo_ReopenEndedBefore(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu-ro")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp-ro", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib-ro")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi-ro", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	require.NoError(t, repo.Create(ctx, newTestPlaySession("ps-ro", string(p.ID), string(item.ID))))
	_, err := repo.CloseAllActive(ctx)
	require.NoError(t, err)

	// Before the shutdown: nothing to reopen.
	reopened, err := repo.ReopenEndedBefore(ctx, "ps-ro", time.Now().Add(-time.Hour), 1000, false, "")
	require.NoError(t, err)
	assert.False(t, reopened)

	restartedAt := time.Now().Add(time.Second)
	reopened, err = repo.ReopenEndedBefore(ctx, "ps-ro", restartedAt, 2_100_000, true, "")
	require.NoError(t, err)
	assert.True(t, reopened)
	active, err := repo.ListActiveSessions(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "ps-ro", active[0].SessionID)
	assert.True(t, active[0].IsPaused)
	assert.EqualValues(t, 2_100_000, active[0].Position.Milliseconds())

	// An open session is UpdateProgress's, never this statement's.
	reopened, err = repo.ReopenEndedBefore(ctx, "ps-ro", restartedAt, 3000, false, "")
	require.NoError(t, err)
	assert.False(t, reopened)
}
