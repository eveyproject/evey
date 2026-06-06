package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestOverride(id, mediaItemID, fieldName string, value any) *metadata.MetadataOverride {
	return &metadata.MetadataOverride{
		ID:          id,
		MediaItemID: shared.MediaItemID(mediaItemID),
		FieldName:   fieldName,
		CustomValue: value,
		EditedBy:    "user-admin",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestMetadataOverrideRepo_Set_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("ovmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	o := newTestOverride("ov1", string(item.ID), "title", "Custom Title")
	require.NoError(t, overrideRepo.Set(ctx, o))

	got, err := overrideRepo.Get(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, o.ID, got.ID)
	assert.Equal(t, o.MediaItemID, got.MediaItemID)
	assert.Equal(t, o.FieldName, got.FieldName)
	assert.Equal(t, "Custom Title", got.CustomValue)
	assert.Equal(t, o.EditedBy, got.EditedBy)
	assert.WithinDuration(t, o.CreatedAt, got.CreatedAt, time.Second)
}

func TestMetadataOverrideRepo_Set_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("ovmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	o := newTestOverride("ov2", string(item.ID), "overview", "Original overview")
	require.NoError(t, overrideRepo.Set(ctx, o))
	first, err := overrideRepo.Get(ctx, o.ID)
	require.NoError(t, err)

	// Same id, same (media_item_id, field_name), updated value + a LATER created_at
	// (simulating a re-edit) — upsert updates the value but must PRESERVE the
	// original created_at, not clobber it with the edit timestamp.
	o.CustomValue = "Updated overview"
	o.CreatedAt = first.CreatedAt.Add(time.Hour)
	require.NoError(t, overrideRepo.Set(ctx, o))

	got, err := overrideRepo.Get(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated overview", got.CustomValue)
	assert.WithinDuration(t, first.CreatedAt, got.CreatedAt, time.Second, "created_at must be preserved on update, not clobbered by the edit time")

	// Only one row for (media_item_id, field_name)
	list, err := overrideRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestMetadataOverrideRepo_GetByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("ovmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov3a", string(item.ID), "title", "T")))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov3b", string(item.ID), "overview", "O")))

	list, err := overrideRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, list, 2)
	// ORDER BY field_name: "overview" < "title"
	assert.Equal(t, "overview", list[0].FieldName)
	assert.Equal(t, "title", list[1].FieldName)
}

func TestMetadataOverrideRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("ovmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	o := newTestOverride("ov4", string(item.ID), "title", "To delete")
	require.NoError(t, overrideRepo.Set(ctx, o))
	require.NoError(t, overrideRepo.Delete(ctx, o.ID))

	_, err := overrideRepo.Get(ctx, o.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMetadataOverrideRepo_DeleteByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("ovmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov5a", string(item.ID), "title", "T")))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov5b", string(item.ID), "tagline", "S")))

	require.NoError(t, overrideRepo.DeleteByMediaItemID(ctx, string(item.ID)))

	list, err := overrideRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Empty(t, list)
}
