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

// U+0000 is valid UTF-8 that Postgres refuses: a provider title carrying it
// failed the write on every enrichment pass. It is dropped like a bad byte.
func TestMediaItemRepo_StripsNulBytes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	lib := newTestLibrary("nul-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("nul-item", lib.ID)
	item.Title = "Ti\x00tle"
	item.Overview = "Over\x00view"
	require.NoError(t, itemRepo.Create(ctx, item))
	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Title", got.Title)
	assert.Equal(t, "Overview", got.Overview)

	person := &library.Person{ID: "nul-person", Name: "Na\x00me"}
	require.NoError(t, personRepo.Create(ctx, person))
	gotPerson, err := personRepo.GetByID(ctx, "nul-person")
	require.NoError(t, err)
	assert.Equal(t, "Name", gotPerson.Name)
}
