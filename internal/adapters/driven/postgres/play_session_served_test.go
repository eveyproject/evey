package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/playback"
)

// What the witnesses write reads back whole through every read of a session
// (the one-row read, the per-profile list the witness loads from, the admin
// list), and the summary columns follow the served path in the same write.
func TestPlaySessionRepo_ServedAndPlayedReadBack(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu-sv")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp-sv", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib-sv")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi-sv", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	live := newTestPlaySession("ps-sv-live", string(p.ID), string(item.ID))
	require.NoError(t, repo.Create(ctx, live))
	got, err := repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Served.Video, "a new session has been served nothing")
	assert.Nil(t, got.Played)

	at := time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC)
	served := playback.Served{
		Video: &playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: -1, Codec: "h264", Width: 1920, Height: 1080, At: at},
		Audio: &playback.ServedLeg{Engine: playback.EngineFFmpeg, Track: 1, Codec: "aac", SourceCodec: "mp3", Channels: 2, At: at},
	}
	played := playback.Played{Source: playback.PlayedHLS, VideoCodec: "avc1.640028", Height: 1080, AudioTrack: 1, AudioCodec: "mp4a.40.2", AudioDecoder: playback.DecoderNative, At: at}
	ok, err := repo.SetServed(ctx, live.ID, served)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = repo.SetPlayed(ctx, live.ID, played)
	require.NoError(t, err)
	require.True(t, ok)

	got, err = repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.True(t, served.Video.SameBytes(*got.Served.Video))
	assert.True(t, served.Audio.SameBytes(*got.Served.Audio))
	assert.True(t, got.Served.Audio.At.Equal(at))
	require.NotNil(t, got.Played)
	assert.True(t, played.SameState(*got.Played))
	assert.Equal(t, string(playback.Transcode), got.PlayMethod)
	assert.Equal(t, "h264", got.VideoCodec)
	assert.Equal(t, "aac", got.AudioCodec)
	assert.Equal(t, "1080p", got.Resolution)

	byProfile, err := repo.GetActiveByProfile(ctx, string(p.ID))
	require.NoError(t, err)
	require.Len(t, byProfile, 1)
	assert.Equal(t, got.Served, byProfile[0].Served)

	// The admin list only shows a session heartbeated within its window.
	_, err = repo.UpdateProgress(ctx, live.ID, 1000, false, "")
	require.NoError(t, err)
	listed, err := repo.ListActiveSessions(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, got.Served, listed[0].Served)
	require.NotNil(t, listed[0].Played)
	assert.Equal(t, playback.DecoderNative, listed[0].Played.AudioDecoder)

	// A new start reusing the row empties what it was seen served and playing.
	require.NoError(t, repo.ResetWitness(ctx, live.ID))
	got, err = repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Served.Video)
	assert.Nil(t, got.Served.Audio)
	assert.Nil(t, got.Played)
	assert.Empty(t, got.PlayMethod)
	assert.Empty(t, got.AudioCodec)
}

// An ended session is never rewritten, and the caller learns which.
func TestPlaySessionRepo_SetServed_ActiveOnly(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu-sv2")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp-sv2", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib-sv2")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi-sv2", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	done := newTestPlaySession("ps-sv2-done", string(p.ID), string(item.ID))
	ended := time.Now().UTC()
	done.EndedAt = &ended
	require.NoError(t, repo.Create(ctx, done))

	served := playback.Served{Video: &playback.ServedLeg{Engine: playback.EngineFile, Copy: true, Track: -1, Codec: "h264"}}
	ok, err := repo.SetServed(ctx, done.ID, served)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.SetPlayed(ctx, done.ID, playback.Played{Source: playback.PlayedFile, AudioTrack: -1})
	require.NoError(t, err)
	assert.False(t, ok)
	got, err := repo.GetByID(ctx, done.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Served.Video)
	assert.Nil(t, got.Played)

	ok, err = repo.SetServed(ctx, "ps-ghost", served)
	require.NoError(t, err)
	assert.False(t, ok)
}
