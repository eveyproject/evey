package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscodeProfileRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	ctx := context.Background()

	p := &transcoding.TranscodeProfile{
		ID:           "tp1",
		Name:         "720p Fast",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     1280,
		MaxHeight:    720,
		VideoBitrate: 2800,
		AudioBitrate: 128,
		Preset:       "fast",
	}
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, "tp1")
	require.NoError(t, err)
	assert.Equal(t, "720p Fast", got.Name)
	assert.Equal(t, "h264", got.VideoCodec)
	assert.Equal(t, 1280, got.MaxWidth)
}

func TestTranscodeProfileRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &transcoding.TranscodeProfile{ID: "tp2a", Name: "AAA", VideoCodec: "h264", Container: "mp4"}))
	require.NoError(t, repo.Create(ctx, &transcoding.TranscodeProfile{ID: "tp2b", Name: "BBB", VideoCodec: "h265", Container: "mkv"}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestTranscodeProfileRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	ctx := context.Background()

	p := &transcoding.TranscodeProfile{ID: "tp3", Name: "Old", VideoCodec: "h264", Container: "mp4"}
	require.NoError(t, repo.Create(ctx, p))

	p.Name = "Updated"
	p.MaxWidth = 1920
	require.NoError(t, repo.Update(ctx, p))

	got, err := repo.GetByID(ctx, "tp3")
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, 1920, got.MaxWidth)
}

func TestTranscodeProfileRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	ctx := context.Background()

	p := &transcoding.TranscodeProfile{ID: "tp4", Name: "ToDelete", VideoCodec: "h264", Container: "mp4"}
	require.NoError(t, repo.Create(ctx, p))
	require.NoError(t, repo.Delete(ctx, "tp4"))

	_, err := repo.GetByID(ctx, "tp4")
	assert.Error(t, err)
}

func TestTranscodeProfileRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	err := repo.Delete(context.Background(), "tp-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestTranscodeProfileRepo_Update_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	p := &transcoding.TranscodeProfile{ID: "tp-missing", Name: "X", VideoCodec: "h264", Container: "mp4"}
	err := repo.Update(context.Background(), p)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

// Completes the set: this is the last of the repo's hand-enumerated UPDATE
// statements without a full round-trip. Its risky run is max_width /
// max_height / video_bitrate / audio_bitrate - four numeric columns in a row,
// two of them the same units apart by a factor of a thousand. A shift there
// swaps a height into a bitrate and the ladder silently encodes at the wrong
// size for every session that picks this profile.
//
// The existing Update test asserts two fields; distinct values here so no
// pair can hide a swap.
func TestTranscodeProfileRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewTranscodeProfileRepository(db)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &transcoding.TranscodeProfile{
		ID: "tp-rt", Name: "Before", VideoCodec: "h264", Container: "mp4",
	}))

	require.NoError(t, repo.Update(ctx, &transcoding.TranscodeProfile{
		ID:           "tp-rt",
		Name:         "After",
		VideoCodec:   "hevc",
		AudioCodec:   "eac3",
		Container:    "mkv",
		MaxWidth:     1280,
		MaxHeight:    720,
		VideoBitrate: 3_500_000,
		AudioBitrate: 192_000,
		Preset:       "veryfast",
	}))

	got, err := repo.GetByID(ctx, "tp-rt")
	require.NoError(t, err)
	assert.Equal(t, "After", got.Name)
	assert.Equal(t, "hevc", got.VideoCodec)
	assert.Equal(t, "eac3", got.AudioCodec)
	assert.Equal(t, "mkv", got.Container)
	// The four adjacent numeric columns.
	assert.Equal(t, 1280, got.MaxWidth)
	assert.Equal(t, 720, got.MaxHeight)
	assert.Equal(t, int64(3_500_000), got.VideoBitrate)
	assert.Equal(t, int64(192_000), got.AudioBitrate)
	assert.Equal(t, "veryfast", got.Preset)
}
