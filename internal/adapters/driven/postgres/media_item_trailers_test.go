package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end round trip against a real DB for the multi-row trailer INSERT
// (buildTrailerInsert is unit-tested for the SQL shape; this exercises it
// through Create/Update/GetByID against Postgres).
func TestMediaItemRepo_Trailers_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("trlib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("trmi1", lib.ID)
	item.Trailers = []library.Trailer{
		{Site: "YouTube", Key: "k0", Language: "en", Official: true, Type: "Trailer"},
		{Site: "YouTube", Key: "k1", Language: "fr", Official: false, Type: "Teaser", UseSubtitles: true, SubtitleLanguage: "fr"},
		{Site: "YouTube", Key: "k2", Language: "de", Official: false, Type: "Clip"},
	}
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got.Trailers, 3)
	assert.Equal(t, "k0", got.Trailers[0].Key)
	assert.Equal(t, "k1", got.Trailers[1].Key)
	assert.Equal(t, "k2", got.Trailers[2].Key)
	assert.True(t, got.Trailers[0].Official)
	assert.True(t, got.Trailers[1].UseSubtitles)
	assert.Equal(t, "fr", got.Trailers[1].SubtitleLanguage)

	// Replace with a shorter list - Update must delete the stale rows too,
	// not just re-insert (the DELETE+multi-row-INSERT pair in one call).
	item.Trailers = []library.Trailer{{Site: "YouTube", Key: "only", Language: "en", Type: "Trailer"}}
	require.NoError(t, itemRepo.Update(ctx, item))
	got, err = itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got.Trailers, 1)
	assert.Equal(t, "only", got.Trailers[0].Key)

	// Replace with none - must not attempt an empty INSERT (would be invalid SQL).
	item.Trailers = nil
	require.NoError(t, itemRepo.Update(ctx, item))
	got, err = itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Trailers)
}
