package metadata

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domain "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bioCacheProviderStub implements MetadataProvider + personDetailFetcher with
// a configurable answer (biography, no biography, or an error) and a hit
// counter so tests can assert whether the provider was reached at all.
type bioCacheProviderStub struct {
	fakeProvider
	details    *domainlib.PersonDetailsResult
	detailsErr error
	detailHits atomic.Int32
}

func (s *bioCacheProviderStub) GetPersonDetails(_ context.Context, _ string) (*domainlib.PersonDetailsResult, error) {
	s.detailHits.Add(1)
	return s.details, s.detailsErr
}

// newBioCacheEnricher builds an Enricher wired with the given provider and a
// fixed clock, on top of fakes, for the personNeedsBio / fetchPersonBio tests.
func newBioCacheEnricher(primary domain.MetadataProvider, now time.Time) (*Enricher, *libfakes.FakePersonRepository, *libfakes.FakeMediaItemRepository) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, searchIdx, primary, nil, nil,
		EnricherWithClock(func() time.Time { return now }))
	return e, personRepo, miRepo
}

func bioCacheItem(personName string) (*domainlib.MediaItem, *domain.MetadataResult) {
	item := &domainlib.MediaItem{ID: "m-bio-cache", Title: "Movie", Type: "movie"}
	result := &domain.MetadataResult{
		Title:       "Movie",
		ProviderIDs: map[string]string{"tmdb": "1"},
		People: []domain.PersonResult{
			{Name: personName, Role: "Actor", ProviderID: "42"},
		},
	}
	return item, result
}

func TestPersonNeedsBio_RecentNegativeCheck_ReturnsFalse(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Enricher{now: func() time.Time { return now }}
	checkedAt := now.Add(-24 * time.Hour)
	p := &domainlib.Person{Biography: "", BioCheckedAt: &checkedAt}
	assert.False(t, e.personNeedsBio(p), "a bio checked 1 day ago is within bioRecheckTTL")
}

func TestPersonNeedsBio_StaleNegativeCheck_ReturnsTrue(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Enricher{now: func() time.Time { return now }}
	checkedAt := now.Add(-31 * 24 * time.Hour)
	p := &domainlib.Person{Biography: "", BioCheckedAt: &checkedAt}
	assert.True(t, e.personNeedsBio(p), "a bio check older than bioRecheckTTL must be retried")
}

func TestPersonNeedsBio_NeverChecked_ReturnsTrue(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Enricher{now: func() time.Time { return now }}
	p := &domainlib.Person{Biography: ""}
	assert.True(t, e.personNeedsBio(p), "nil BioCheckedAt means never asked - a fresh person still needs its bio")
}

func TestPersonNeedsBio_HasBiography_ReturnsFalseRegardlessOfStamp(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Enricher{now: func() time.Time { return now }}
	p := &domainlib.Person{Biography: "already known"}
	assert.False(t, e.personNeedsBio(p))
}

// TestEnrichItem_RecentNegativeCheck_SkipsTheProvider is the whole point of
// this backlog item: a person with no bio and a recent negative check must
// NOT trigger a provider round-trip on every re-enrichment pass.
func TestEnrichItem_RecentNegativeCheck_SkipsTheProvider(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stub := &bioCacheProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		details:      &domainlib.PersonDetailsResult{Biography: "a late bio"},
	}
	e, personRepo, miRepo := newBioCacheEnricher(stub, now)

	checkedAt := now.Add(-24 * time.Hour)
	require.NoError(t, personRepo.Create(context.Background(), &domainlib.Person{
		ID: "p-recent", Name: "Alice", BioCheckedAt: &checkedAt,
	}))

	item, result := bioCacheItem("Alice")
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	assert.Equal(t, int32(0), stub.detailHits.Load(), "provider must not be called within bioRecheckTTL")
	stored, err := personRepo.GetByName(context.Background(), "Alice")
	require.NoError(t, err)
	assert.Empty(t, stored.Biography)
}

// TestEnrichItem_StaleNegativeCheck_CallsTheProviderAgain is the TTL expiry
// side of the same guarantee: after bioRecheckTTL, the person is fair game
// again.
func TestEnrichItem_StaleNegativeCheck_CallsTheProviderAgain(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stub := &bioCacheProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		details:      &domainlib.PersonDetailsResult{Biography: ""},
	}
	e, personRepo, miRepo := newBioCacheEnricher(stub, now)

	checkedAt := now.Add(-31 * 24 * time.Hour)
	require.NoError(t, personRepo.Create(context.Background(), &domainlib.Person{
		ID: "p-stale", Name: "Bob", BioCheckedAt: &checkedAt,
	}))

	item, result := bioCacheItem("Bob")
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	assert.GreaterOrEqual(t, stub.detailHits.Load(), int32(1), "a stale negative check must be retried")
}

// TestFetchPersonBio_NoBiographyFromProvider_StampsNegativeCache covers the
// bug this item exists for: the provider answering "no bio" must be
// remembered, not silently dropped.
func TestFetchPersonBio_NoBiographyFromProvider_StampsNegativeCache(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	stub := &bioCacheProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		details:      &domainlib.PersonDetailsResult{Biography: ""},
	}
	e, personRepo, miRepo := newBioCacheEnricher(stub, now)

	item, result := bioCacheItem("Carol")
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	stored, err := personRepo.GetByName(context.Background(), "Carol")
	require.NoError(t, err)
	assert.Empty(t, stored.Biography)
	require.NotNil(t, stored.BioCheckedAt, "a negative answer must be stamped so it is not re-fetched daily")
	assert.True(t, now.Equal(*stored.BioCheckedAt))
	assert.GreaterOrEqual(t, personRepo.UpdateCalls.Load(), int64(1))
}

// TestFetchPersonBio_ProviderError_DoesNotStampTheCache ensures a transient
// failure (429/timeout) never silences a person for bioRecheckTTL.
func TestFetchPersonBio_ProviderError_DoesNotStampTheCache(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	stub := &bioCacheProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		detailsErr:   errors.New("429 rate limited"),
	}
	e, personRepo, miRepo := newBioCacheEnricher(stub, now)

	item, result := bioCacheItem("Dave")
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	updatesBeforeCheck := personRepo.UpdateCalls.Load()

	stored, err := personRepo.GetByName(context.Background(), "Dave")
	require.NoError(t, err)
	assert.Nil(t, stored.BioCheckedAt, "a provider error must not stamp the negative cache")
	assert.Empty(t, stored.Biography)
	assert.Equal(t, int64(0), updatesBeforeCheck, "a provider error must not persist anything")
}

// TestFetchPersonBio_BiographyFound_PersistsBioAndStamp covers the happy
// path: a found biography is persisted AND the negative-cache stamp is set
// too (harmless once Biography != "" makes personNeedsBio always false).
func TestFetchPersonBio_BiographyFound_PersistsBioAndStamp(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	stub := &bioCacheProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		details:      &domainlib.PersonDetailsResult{Biography: "a life story"},
	}
	e, personRepo, miRepo := newBioCacheEnricher(stub, now)

	item, result := bioCacheItem("Eve")
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	stored, err := personRepo.GetByName(context.Background(), "Eve")
	require.NoError(t, err)
	assert.Equal(t, "a life story", stored.Biography)
	require.NotNil(t, stored.BioCheckedAt)
	assert.True(t, now.Equal(*stored.BioCheckedAt))
}
