package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPhoto(id string, libID shared.LibraryID, album string) photo.Photo {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return photo.Photo{
		ID:          id,
		LibraryID:   shared.LibraryID(libID),
		Title:       "Photo " + id,
		FilePath:    "/photos/" + id + ".jpg",
		FileSize:    2000000,
		FileHash:    "phash-" + id,
		ModTime:     now,
		MimeType:    "image/jpeg",
		Width:       1920,
		Height:      1080,
		CameraModel: "Canon",
		Album:       album,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestPhotoRepo_Upsert_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib1")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	p := newTestPhoto("ph1", lib.ID, "Vacation")
	require.NoError(t, repo.Upsert(ctx, p))

	got, err := repo.GetByID(ctx, "ph1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Photo ph1", got.Title)
	assert.Equal(t, "Vacation", got.Album)
}

func TestPhotoRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib2")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	p := newTestPhoto("ph2", lib.ID, "")
	require.NoError(t, repo.Upsert(ctx, p))
	require.NoError(t, repo.Delete(ctx, "ph2"))

	got, err := repo.GetByID(ctx, "ph2")
	assert.ErrorIs(t, err, shared.ErrNotFound)
	assert.Nil(t, got)
}

func TestPhotoRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPhotoRepository(db)
	err := repo.Delete(context.Background(), "photo-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPhotoRepo_ListByLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib3")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestPhoto("ph3a", lib.ID, "")))
	p2 := newTestPhoto("ph3b", lib.ID, "")
	p2.FilePath = "/photos/ph3b.jpg"
	require.NoError(t, repo.Upsert(ctx, p2))

	photos, total, err := repo.ListByLibrary(ctx, lib.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, photos, 2)
}

func TestPhotoRepo_ListByAlbum(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib4")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestPhoto("ph4a", lib.ID, "Trip")))
	p2 := newTestPhoto("ph4b", lib.ID, "Trip")
	p2.FilePath = "/photos/ph4b.jpg"
	require.NoError(t, repo.Upsert(ctx, p2))

	photos, total, err := repo.ListByAlbum(ctx, lib.ID, "Trip", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, photos, 2)
}

func TestPhotoRepo_GetAlbums(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib5")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestPhoto("ph5a", lib.ID, "Beach")))
	p2 := newTestPhoto("ph5b", lib.ID, "Beach")
	p2.FilePath = "/photos/ph5b.jpg"
	require.NoError(t, repo.Upsert(ctx, p2))

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, albums, 1)
	assert.Equal(t, "Beach", albums[0].Name)
	assert.Equal(t, 2, albums[0].PhotoCount)
}

func TestPhotoRepo_Search(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib6")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	p := newTestPhoto("ph6", lib.ID, "SearchableAlbum")
	require.NoError(t, repo.Upsert(ctx, p))

	photos, err := repo.Search(ctx, lib.ID, "Searchable", 10)
	require.NoError(t, err)
	assert.Len(t, photos, 1)
}

func TestPhotoRepo_ListAllByLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib7")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestPhoto("ph7a", lib.ID, "")))
	p2 := newTestPhoto("ph7b", lib.ID, "")
	p2.FilePath = "/photos/ph7b.jpg"
	require.NoError(t, repo.Upsert(ctx, p2))

	photos, err := repo.ListAllByLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, photos, 2)
}

func TestPhotoRepo_UpdateFilePath(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib8")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestPhoto("ph8", lib.ID, "")))

	newMod := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.UpdateFilePath(ctx, "ph8", "/photos/new_path.jpg", newMod))

	got, err := repo.GetByID(ctx, "ph8")
	require.NoError(t, err)
	assert.Equal(t, "/photos/new_path.jpg", got.FilePath)
}

// The stats endpoint used to obtain this number by calling ListByLibrary with
// limit=1 and throwing the rows away - which still paid the list query's
// COUNT(*) OVER() window over the entire library, once per library. The
// dedicated counter must agree with the list total exactly, and must count only
// the library it was asked about.
func TestPhotoRepo_CountByLibrary_AgreesWithListTotal(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-count-a")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))
	other := newTestLibrary("ph-count-b")
	other.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, other))

	for _, id := range []string{"pc1", "pc2", "pc3"} {
		p := newTestPhoto(id, lib.ID, "")
		p.FilePath = "/photos/" + id + ".jpg"
		require.NoError(t, repo.Upsert(ctx, p))
	}
	stray := newTestPhoto("pc-other", other.ID, "")
	stray.FilePath = "/photos/pc-other.jpg"
	require.NoError(t, repo.Upsert(ctx, stray))

	_, total, err := repo.ListByLibrary(ctx, lib.ID, 1, 0)
	require.NoError(t, err)

	n, err := repo.CountByLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, total, n, "the counter disagrees with the list total it replaced")
	assert.Equal(t, 3, n)

	// A photo in another library must not be counted, or every library would
	// report the whole server's total.
	nOther, err := repo.CountByLibrary(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, nOther)

	// An empty library is 0, not an error - the stats page renders it.
	empty := newTestLibrary("ph-count-empty")
	empty.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, empty))
	nEmpty, err := repo.CountByLibrary(ctx, empty.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, nEmpty)
}
