package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
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
	require.NoError(t, repo.UpdateFilePath(ctx, "ph8", "/photos/new_path.jpg", "Voyage", newMod))

	got, err := repo.GetByID(ctx, "ph8")
	require.NoError(t, err)
	assert.Equal(t, "/photos/new_path.jpg", got.FilePath)
	assert.Equal(t, "Voyage", got.Album, "the album moves with the file")
}

// The keyset ORDER BY is (taken_at DESC NULLS LAST, created_at DESC, id DESC).
// taken_at and created_at deliberately repeat across several rows here
// (including a NULL taken_at group) - id is the only thing that makes those
// tuples unique. A cursor built from anything less than the full tuple would
// skip or repeat a row exactly at a page boundary that lands inside one of
// these ties; a limit of 2 forces every boundary in this 7-row set to land
// inside a tie.
func TestPhotoRepo_ListByLibraryCursor_PageBoundary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-cursor-lib")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	c0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c1 := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	c2 := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)

	photos := []photo.Photo{
		newTestPhoto("pcur1", lib.ID, ""),
		newTestPhoto("pcur2", lib.ID, ""),
		newTestPhoto("pcur3", lib.ID, ""),
		newTestPhoto("pcur4", lib.ID, ""),
		newTestPhoto("pcur5", lib.ID, ""),
		newTestPhoto("pcur6", lib.ID, ""),
		newTestPhoto("pcur7", lib.ID, ""),
	}
	photos[0].TakenAt, photos[0].CreatedAt = &t0, c0 // exact tie with pcur2 on (taken_at, created_at)
	photos[1].TakenAt, photos[1].CreatedAt = &t0, c0
	photos[2].TakenAt, photos[2].CreatedAt = &t0, c1
	photos[3].TakenAt, photos[3].CreatedAt = nil, c2 // exact tie with pcur5, both NULL taken_at
	photos[4].TakenAt, photos[4].CreatedAt = nil, c2
	photos[5].TakenAt, photos[5].CreatedAt = &t1, c0
	photos[6].TakenAt, photos[6].CreatedAt = nil, c1

	ids := make([]string, 0, len(photos))
	for _, p := range photos {
		p.FilePath = "/photos/" + p.ID + ".jpg"
		require.NoError(t, repo.Upsert(ctx, p))
		ids = append(ids, p.ID)
	}

	var collected []string
	var cursor *photo.PhotoCursor
	// A page count cap, not a break-on-empty-page loop: a seek predicate bug
	// that fails to exclude an already-returned row turns this into an
	// infinite loop against a live DB connection instead of a fast failure -
	// exactly what happened with the equivalent music query during development.
	for range len(photos) + 2 {
		page, err := repo.ListByLibraryCursor(ctx, lib.ID, cursor, 2)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, p := range page {
			collected = append(collected, p.ID)
		}
		last := page[len(page)-1]
		cursor = &photo.PhotoCursor{TakenAt: last.TakenAt, CreatedAt: last.CreatedAt, ID: last.ID}
		if len(page) < 2 {
			break
		}
	}

	assert.ElementsMatch(t, ids, collected, "keyset pagination must return every row exactly once")
	assert.Len(t, collected, len(ids), "no row should be skipped or repeated at a page boundary")

	// The exact order must match taken_at DESC NULLS LAST, created_at DESC, id
	// DESC - and must agree with the offset-based ListByLibrary, since both
	// queries claim to sort the same way.
	want := []string{"pcur6", "pcur3", "pcur2", "pcur1", "pcur5", "pcur4", "pcur7"}
	assert.Equal(t, want, collected)

	offsetPhotos, total, err := repo.ListByLibrary(ctx, lib.ID, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, len(ids), total)
	var offsetIDs []string
	for _, p := range offsetPhotos {
		offsetIDs = append(offsetIDs, p.ID)
	}
	assert.Equal(t, want, offsetIDs, "keyset and offset queries must agree on ordering")
}

func TestPhotoRepo_ListByAlbumCursor_PageBoundary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-cursor-album-lib")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))

	same := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	inAlbum := []photo.Photo{
		newTestPhoto("pcal1", lib.ID, "Trip"),
		newTestPhoto("pcal2", lib.ID, "Trip"),
		newTestPhoto("pcal3", lib.ID, "Trip"),
	}
	for i, p := range inAlbum {
		p.TakenAt, p.CreatedAt = &same, same // every row ties on (taken_at, created_at)
		p.FilePath = "/photos/" + p.ID + ".jpg"
		require.NoError(t, repo.Upsert(ctx, p))
		inAlbum[i] = p
	}
	// A photo in a different album must never surface in the album-scoped page.
	other := newTestPhoto("pcal-other", lib.ID, "OtherAlbum")
	other.FilePath = "/photos/pcal-other.jpg"
	require.NoError(t, repo.Upsert(ctx, other))

	var collected []string
	var cursor *photo.PhotoCursor
	for range len(inAlbum) + 2 { // page count cap, see ListByLibraryCursor's test for why
		page, err := repo.ListByAlbumCursor(ctx, lib.ID, "Trip", cursor, 1)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, p := range page {
			collected = append(collected, p.ID)
		}
		last := page[len(page)-1]
		cursor = &photo.PhotoCursor{TakenAt: last.TakenAt, CreatedAt: last.CreatedAt, ID: last.ID}
	}

	assert.ElementsMatch(t, []string{"pcal1", "pcal2", "pcal3"}, collected)
	assert.NotContains(t, collected, "pcal-other")
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

func TestPhotoRepo_DeleteByIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewPhotoRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ph-lib-batch")
	lib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"phb1", "phb2", "phb3"} {
		require.NoError(t, repo.Upsert(ctx, newTestPhoto(id, lib.ID, "")))
	}

	require.NoError(t, repo.DeleteByIDs(ctx, nil))
	require.NoError(t, repo.DeleteByIDs(ctx, []string{"phb1", "phb2", "no-such-id"}))

	for _, id := range []string{"phb1", "phb2"} {
		_, err := repo.GetByID(ctx, id)
		require.ErrorIs(t, err, shared.ErrNotFound)
	}
	kept, err := repo.GetByID(ctx, "phb3")
	require.NoError(t, err)
	assert.Equal(t, "phb3", kept.ID)
}
