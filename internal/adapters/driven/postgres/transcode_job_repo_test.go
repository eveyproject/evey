package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTranscodeJob(id, mediaItemID, status string) *transcoding.TranscodeJob {
	return &transcoding.TranscodeJob{
		ID:            id,
		MediaItemID:   shared.MediaItemID(mediaItemID),
		InputCodec:    "h265",
		OutputCodec:   "h264",
		HWAccelType:   "none",
		Preset:        "fast",
		ResolutionIn:  "1920x1080",
		ResolutionOut: "1280x720",
		BitrateIn:     5000000,
		BitrateOut:    2800000,
		Speed:         1.5,
		Status:        transcoding.JobStatus(status),
		StartedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestTranscodeJobRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewTranscodeJobRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tj-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tj-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	job := newTestTranscodeJob("tj1", string(item.ID), "queued")
	require.NoError(t, repo.Create(ctx, job))

	got, err := repo.GetByID(ctx, "tj1")
	require.NoError(t, err)
	assert.Equal(t, transcoding.Queued, got.Status)
	assert.Equal(t, "h265", got.InputCodec)
}

func TestTranscodeJobRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewTranscodeJobRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tj-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tj-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	job := newTestTranscodeJob("tj2", string(item.ID), "queued")
	require.NoError(t, repo.Create(ctx, job))

	job.Status = transcoding.Running
	job.Speed = 2.0
	require.NoError(t, repo.Update(ctx, job))

	got, err := repo.GetByID(ctx, "tj2")
	require.NoError(t, err)
	assert.Equal(t, transcoding.Running, got.Status)
}

func TestTranscodeJobRepo_ListActive(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewTranscodeJobRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tj-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tj-mi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, repo.Create(ctx, newTestTranscodeJob("tj3a", string(item.ID), "running")))
	completedJob := newTestTranscodeJob("tj3b", string(item.ID), "completed")
	require.NoError(t, repo.Create(ctx, completedJob))

	active, err := repo.ListActive(ctx)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, "tj3a", active[0].ID)
}

func TestTranscodeJobRepo_ListByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewTranscodeJobRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tj-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tj-mi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, repo.Create(ctx, newTestTranscodeJob("tj4a", string(item.ID), "completed")))
	require.NoError(t, repo.Create(ctx, newTestTranscodeJob("tj4b", string(item.ID), "running")))

	jobs, err := repo.ListByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Len(t, jobs, 2)
}
