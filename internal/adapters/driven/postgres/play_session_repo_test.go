package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlaySession(id, profileID, mediaItemID string) *playback.PlaySession {
	return &playback.PlaySession{
		ID:           id,
		ProfileID:    profileID,
		MediaItemID:  mediaItemID,
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

	session := newTestPlaySession("ps1", p.ID, item.ID)
	require.NoError(t, sessionRepo.Create(ctx, session))

	got, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ProfileID, got.ProfileID)
	assert.Equal(t, session.DeviceName, got.DeviceName)
	assert.Equal(t, session.PlayMethod, got.PlayMethod)
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

	session := newTestPlaySession("ps2", p.ID, item.ID)
	require.NoError(t, sessionRepo.Create(ctx, session))

	endedAt := time.Now().UTC().Truncate(time.Millisecond)
	durationMs := int64(120000)
	posMs := int64(119000)
	session.EndedAt = &endedAt
	session.DurationMs = &durationMs
	session.PositionMs = &posMs
	require.NoError(t, sessionRepo.Update(ctx, session))

	got, err := sessionRepo.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, got.EndedAt)
	require.NotNil(t, got.DurationMs)
	assert.Equal(t, durationMs, *got.DurationMs)
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

	session := newTestPlaySession("ps3", p.ID, item.ID)
	require.NoError(t, sessionRepo.Create(ctx, session))

	active, err := sessionRepo.GetActiveByProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, session.ID, active[0].ID)
}

func TestPlaySessionRepo_GetHistory(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	ctx := context.Background()

	u := newTestUser("psu4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("psp4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pslib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("psmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("ps4a", p.ID, item.ID)))
	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("ps4b", p.ID, item.ID)))

	pid := p.ID
	list, err := sessionRepo.GetHistory(ctx, playback.PlaySessionFilter{ProfileID: &pid, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
