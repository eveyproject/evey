package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/playback"
)

// Prod, 2026-10-02: 58 non-direct sessions carried video_engine = ”. The
// beacon path (SetCastAudioPath) reads the live session, swaps the audio leg
// and writes the WHOLE serving path back; the read never selected the engine
// columns, so every beacon blanked video_engine and video_backend. The read
// must hand back exactly what SetSessionServing wrote, for every path shape.
func TestPlaySessionRepo_ServingPathReadsBack(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu11")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp11", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib11")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi11", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	cases := []struct {
		name string
		path playback.ServingPath
	}{
		{"direct play: nothing recorded", playback.ServingPath{}},
		{"mkvgo video + mkvgo audio", playback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "copy", AudioEngine: "mkvgo"}},
		{"mkvgo video + ffmpeg audio (hybrid)", playback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg"}},
		{"mkvgo video + client wasm audio", playback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "wasm", AudioEngine: "client"}},
		{"ffmpeg copy video + client wasm audio", playback.ServingPath{VideoDecision: "copy", VideoEngine: "ffmpeg", AudioDecision: "wasm", AudioEngine: "client"}},
		{"ffmpeg transcode video + wasm audio", playback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: "vaapi", AudioDecision: "wasm", AudioEngine: "client"}},
		{"ffmpeg video + ffmpeg audio", playback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: "software", AudioDecision: "transcode", AudioEngine: "ffmpeg"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestPlaySession("ps11-"+string(rune('a'+i)), string(p.ID), string(item.ID))
			require.NoError(t, repo.Create(ctx, s))
			ok, err := repo.SetSessionServing(ctx, s.ID, tc.path)
			require.NoError(t, err)
			require.True(t, ok)

			byID, err := repo.GetByID(ctx, s.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.path, byID.Serving, "GetByID")

			active, err := repo.GetActiveByProfile(ctx, string(p.ID))
			require.NoError(t, err)
			var live *playback.PlaySession
			for _, a := range active {
				if a.ID == s.ID {
					live = a
				}
			}
			require.NotNil(t, live)
			assert.Equal(t, tc.path, live.Serving, "GetActiveByProfile: the beacon path rewrites from this read")

			require.NoError(t, repo.Update(ctx, live))
			again, err := repo.GetByID(ctx, s.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.path, again.Serving, "a session Update never touches the serving columns")
		})
	}
}

// The beacon owns the audio leg alone. A master recorded AFTER the beacon's
// read (the web player fetches the master while StartStream is in flight) must
// survive the beacon's write: the statement names no video column.
func TestPlaySessionRepo_SetSessionAudioPath_NeverTouchesTheVideoLeg(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("psu12")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("psp12", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pslib12")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("psmi12", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewPlaySessionRepository(db)

	live := newTestPlaySession("ps12-live", string(p.ID), string(item.ID))
	live.AudioCodec = "eac3"
	require.NoError(t, repo.Create(ctx, live))
	done := newTestPlaySession("ps12-done", string(p.ID), string(item.ID))
	require.NoError(t, repo.Create(ctx, done))
	done.EndedAt = &done.StartedAt
	require.NoError(t, repo.Update(ctx, done))

	master := playback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg"}
	ok, err := repo.SetSessionServing(ctx, live.ID, master)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.SetSessionAudioPath(ctx, live.ID, "wasm", "client", "")
	require.NoError(t, err)
	require.True(t, ok)
	got, err := repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, playback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "wasm", AudioEngine: "client"}, got.Serving)
	assert.Equal(t, "eac3", got.AudioCodec, "no codec supplied: the label stands")

	ok, err = repo.SetSessionAudioPath(ctx, live.ID, "transcode", "ffmpeg", "eac3→aac")
	require.NoError(t, err)
	require.True(t, ok)
	got, err = repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, "mkvgo", got.Serving.VideoEngine)
	assert.Equal(t, "eac3→aac", got.AudioCodec)

	ok, err = repo.SetSessionAudioPath(ctx, done.ID, "wasm", "client", "")
	require.NoError(t, err)
	assert.False(t, ok, "an ended session is never rewritten")
	ok, err = repo.SetSessionAudioPath(ctx, "ps12-ghost", "wasm", "client", "")
	require.NoError(t, err)
	assert.False(t, ok)
}
