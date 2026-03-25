package postgres_test

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
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
