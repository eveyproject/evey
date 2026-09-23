package postgres_test

// Integration proof of EnrichItem's atomicity: real repositories + the real
// TxUnit against a real Postgres. A failure on the LAST write (cast/crew
// links) must roll back the item update AND the artwork delete+insert -
// previously each step auto-committed, so a mid-enrichment blip left the item
// with a fresh title but no artwork and no cast, unrepairable except by a
// manual "Refresh metadata" (the re-scan trigger keys on empty Overview).

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

// failingMediaPersons delegates everything to the real repository but fails
// CreateBatch - the last write of the enrichment transaction.
type failingMediaPersons struct {
	*postgres.MediaPersonRepository
}

func (r *failingMediaPersons) CreateBatch(_ context.Context, _ []*library.MediaPerson) error {
	return errors.New("simulated blip on the last enrichment write")
}

func TestEnrichItem_RollsBackAllStepsOnFailure(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := &failingMediaPersons{MediaPersonRepository: postgres.NewMediaPersonRepository(db)}

	lib := newTestLibrary("enrlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("enrich-atom", lib.ID)
	item.Title = "Old Title"
	require.NoError(t, itemRepo.Create(ctx, item))

	// Pre-existing artwork the enrichment's delete+insert would replace.
	oldArt, err := domainmetadata.NewArtworkRecord("art-old", item.ID, "poster", "tmdb",
		domainmetadata.WithArtworkSource("http://img/old.jpg"))
	require.NoError(t, err)
	require.NoError(t, artRepo.Create(ctx, oldArt))

	enricher := appmetadata.NewEnricher(
		artRepo, personRepo, mpRepo, postgres.NewMediaItemQueryRepoAdapter(db), itemRepo, nil,
		nil, nil, nil, nil,
		appmetadata.EnricherWithUnitOfWork(postgres.NewTxUnit(db)),
	)

	result := &domainmetadata.MetadataResult{
		Title:   "New Title",
		Artwork: []domainmetadata.ArtworkResult{{Type: "poster", URL: "http://img/new.jpg", Provider: "tmdb"}},
		People:  []domainmetadata.PersonResult{{Name: "Actor A", Role: "Actor"}},
	}
	err = enricher.EnrichItem(ctx, item, result)
	require.Error(t, err, "the cast/crew write fails by construction")

	// Every step must have rolled back, not just the failing one.
	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Old Title", got.Title, "item update rolled back")

	arts, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, arts, 1, "artwork delete+insert rolled back - the OLD record survives")
	assert.Equal(t, "art-old", arts[0].ID)

	persons, err := personRepo.GetByName(ctx, "Actor A")
	assert.True(t, persons == nil || errors.Is(err, shared.ErrNotFound),
		"person insert rolled back with the rest")
}

// The happy path through the SAME wiring: all three writes commit.
func TestEnrichItem_CommitsAllSteps(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)

	lib := newTestLibrary("enrlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("enrich-ok", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	enricher := appmetadata.NewEnricher(
		artRepo, personRepo, mpRepo, postgres.NewMediaItemQueryRepoAdapter(db), itemRepo, nil,
		nil, nil, nil, nil,
		appmetadata.EnricherWithUnitOfWork(postgres.NewTxUnit(db)),
	)

	result := &domainmetadata.MetadataResult{
		Title:   "New Title",
		Artwork: []domainmetadata.ArtworkResult{{Type: "poster", URL: "http://img/new.jpg", Provider: "tmdb"}},
		People:  []domainmetadata.PersonResult{{Name: "Actor B", Role: "Actor", Character: "Hero"}},
	}
	require.NoError(t, enricher.EnrichItem(ctx, item, result))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Title", got.Title)

	arts, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Len(t, arts, 1)

	links, err := mpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
}
