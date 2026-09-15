package library_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ListGenres folds the stored spellings onto one label each, rendered in the
// SERVER language; no language (or a config read that fails) folds without
// translating, and a repository error is the caller's to see.

func genresFixture(cfg shared.ServerConfigRepository) *applib.QueryService {
	media := libfakes.NewFakeMediaItemQueryRepo()
	media.Add(&library.MediaItem{ID: "m1", LibraryID: "lib1", Catalog: &library.MediaCatalog{Genres: []string{"Drama", "Action"}}})
	media.Add(&library.MediaItem{ID: "m2", LibraryID: "lib1", Catalog: &library.MediaCatalog{Genres: []string{"Drame", "Sci-Fi"}}})
	media.Add(&library.MediaItem{ID: "m3", LibraryID: "lib2", Catalog: &library.MediaCatalog{Genres: []string{"Horror"}}})
	opts := []applib.QueryServiceOption{}
	if cfg != nil {
		opts = append(opts, applib.WithServerConfig(cfg))
	}
	return applib.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), media, libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex(), opts...)
}

func TestQueryService_ListGenres_RendersInServerLanguage(t *testing.T) {
	t.Parallel()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfg.Set(context.Background(), "server.language", " FR "))
	got, err := genresFixture(cfg).ListGenres(context.Background(), "lib1")
	require.NoError(t, err)
	assert.Contains(t, got, "Drame", "Drama and Drame fold onto the French label")
	assert.NotContains(t, got, "Drama")
	assert.NotContains(t, got, "Horror", "another library's genres must not leak in")
	assert.True(t, slices.IsSorted(got), "genres are sorted: %v", got)
}

func TestQueryService_ListGenres_NoLanguageFoldsWithoutTranslating(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]shared.ServerConfigRepository{
		"no config repo": nil,
		"key unset":      sharedfakes.NewFakeServerConfigRepository(),
		"config read fails": func() shared.ServerConfigRepository {
			r := sharedfakes.NewFakeServerConfigRepository()
			r.Err = errors.New("db down")
			return r
		}(),
	} {
		got, err := genresFixture(cfg).ListGenres(context.Background(), "lib1")
		require.NoError(t, err, name)
		assert.Len(t, got, 3, "%s: Drama/Drame folded, Action and Sci-Fi kept: %v", name, got)
		assert.True(t, slices.Contains(got, "Drama") || slices.Contains(got, "Drame"), "%s: a stored spelling survives: %v", name, got)
	}
}

func TestQueryService_ListGenres_RepoErrorPropagates(t *testing.T) {
	t.Parallel()
	media := libfakes.NewFakeMediaItemQueryRepo()
	media.Err = errors.New("db down")
	svc := applib.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), media, libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	_, err := svc.ListGenres(context.Background(), "lib1")
	require.Error(t, err)
}
