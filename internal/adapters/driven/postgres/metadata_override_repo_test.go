package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
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
	// (simulating a re-edit) - upsert updates the value but must PRESERVE the
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

func TestMetadataOverrideRepo_GetByMediaItems_Batch(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib-batch")
	require.NoError(t, libRepo.Create(ctx, lib))
	a := newTestMediaItem("ovmi-a", lib.ID)
	b := newTestMediaItem("ovmi-b", lib.ID)
	c := newTestMediaItem("ovmi-c", lib.ID) // no overrides
	require.NoError(t, itemRepo.Create(ctx, a))
	require.NoError(t, itemRepo.Create(ctx, b))
	require.NoError(t, itemRepo.Create(ctx, c))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ovb-a1", string(a.ID), "title", "Ta")))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ovb-a2", string(a.ID), "overview", "Oa")))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ovb-b1", string(b.ID), "title", "Tb")))

	got, err := overrideRepo.GetByMediaItems(ctx, []string{string(a.ID), string(b.ID), string(c.ID)})
	require.NoError(t, err)
	require.Len(t, got[string(a.ID)], 2, "item a has two overrides")
	require.Len(t, got[string(b.ID)], 1, "item b has one override")
	_, hasC := got[string(c.ID)]
	assert.False(t, hasC, "item without overrides must be absent from the map")
	// Grouped + ORDER BY field_name within a's bucket.
	assert.Equal(t, "overview", got[string(a.ID)][0].FieldName)
	assert.Equal(t, "title", got[string(a.ID)][1].FieldName)
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

func TestMetadataOverrideRepo_DeleteByMediaItemIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ovlib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("ovmi6a", lib.ID)
	item2 := newTestMediaItem("ovmi6b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov6a", string(item1.ID), "title", "T1")))
	require.NoError(t, overrideRepo.Set(ctx, newTestOverride("ov6b", string(item2.ID), "title", "T2")))

	require.NoError(t, overrideRepo.DeleteByMediaItemIDs(ctx, []string{string(item1.ID), string(item2.ID)}))

	list1, err := overrideRepo.GetByMediaItem(ctx, string(item1.ID))
	require.NoError(t, err)
	assert.Empty(t, list1)
	list2, err := overrideRepo.GetByMediaItem(ctx, string(item2.ID))
	require.NoError(t, err)
	assert.Empty(t, list2)
}

func TestMetadataOverrideRepo_DeleteByMediaItemIDs_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	overrideRepo := postgres.NewMetadataOverrideRepository(db)
	assert.NoError(t, overrideRepo.DeleteByMediaItemIDs(context.Background(), nil))
}

// The library grid asks in one query which of a page of items carry an
// override (the "edited" badge): one entry per item whatever the number of
// overridden fields, nothing for the untouched ones, empty in = empty out.
func TestMetadataOverrideRepo_ListOverriddenMediaItemIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("ovlistlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	edited, plain := newTestMediaItem("ovl-edited", lib.ID), newTestMediaItem("ovl-plain", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, edited))
	require.NoError(t, itemRepo.Create(ctx, plain))
	repo := postgres.NewMetadataOverrideRepository(db)
	require.NoError(t, repo.Set(ctx, newTestOverride("ovl-1", string(edited.ID), "title", "T")))
	require.NoError(t, repo.Set(ctx, newTestOverride("ovl-2", string(edited.ID), "overview", "O")))

	got, err := repo.ListOverriddenMediaItemIDs(ctx, []string{string(edited.ID), string(plain.ID), "ghost"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{string(edited.ID): true}, got)

	got, err = repo.ListOverriddenMediaItemIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got)
}
