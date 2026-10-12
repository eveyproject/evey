package metadata

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingProvider tracks Search / GetByProviderID invocations.
type countingProvider struct {
	fakeProvider
	searchCalls atomic.Int32
	detailCalls atomic.Int32
}

func (c *countingProvider) Search(ctx context.Context, title string, year *int, mediaType string) ([]domain.MetadataResult, error) {
	c.searchCalls.Add(1)
	return c.fakeProvider.Search(ctx, title, year, mediaType)
}
func (c *countingProvider) GetByProviderID(ctx context.Context, id, mediaType string) (*domain.MetadataResult, error) {
	c.detailCalls.Add(1)
	return c.fakeProvider.GetByProviderID(ctx, id, mediaType)
}

// fakeTranslationProvider: fakeProvider + TranslationProvider interface.
type fakeTranslationProvider struct {
	fakeProvider
	fetchCalls atomic.Int32
	result     map[string]domain.MetadataResult
	fetchErr   error
}

func (f *fakeTranslationProvider) FetchTranslations(_ context.Context, _ string, _ string, _ []string) (map[string]domain.MetadataResult, error) {
	f.fetchCalls.Add(1)
	return f.result, f.fetchErr
}
func (f *fakeTranslationProvider) FetchPersonTranslations(_ context.Context, _ string, _ []string) (map[string]string, error) {
	return nil, nil
}

// fakeMediaTranslationRepo counts Upsert.
type fakeMediaTranslationRepo struct {
	upserts atomic.Int32
}

func (r *fakeMediaTranslationRepo) Upsert(_ context.Context, _ *domainlib.MediaItemTranslation) error {
	r.upserts.Add(1)
	return nil
}
func (r *fakeMediaTranslationRepo) UpsertBatch(_ context.Context, ts []*domainlib.MediaItemTranslation) error {
	r.upserts.Add(int32(len(ts)))
	return nil
}
func (r *fakeMediaTranslationRepo) GetByMediaItemAndLocale(_ context.Context, _, _ string) (*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *fakeMediaTranslationRepo) GetByMediaItem(_ context.Context, _ string) ([]*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *fakeMediaTranslationRepo) GetByMediaItemsAndLocale(_ context.Context, _ []string, _ string) (map[string]*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *fakeMediaTranslationRepo) DeleteByMediaItem(_ context.Context, _ string) error { return nil }

func captureLog() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

// newTestEnricherMulti builds an Enricher with arbitrary []MetadataProvider supps.
func newTestEnricherMulti(primary domain.MetadataProvider, supps []domain.MetadataProvider) (*Enricher, *libfakes.FakeMediaItemStore) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemStore()
	searchIdx := libfakes.NewFakeSearchIndex()
	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, miRepo, searchIdx, primary, supps, nil, nil)
	return e, miRepo
}

// TestMeta_KillMutant_SupplementaryFallback_BreaksOnFirstSuccess kills
// enricher_auto.go:56 NEGATION `if primary != nil { break }`.
func TestMeta_KillMutant_SupplementaryFallback_BreaksOnFirstSuccess(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"} // search returns nothing → fallback

	supp1 := &countingProvider{fakeProvider: fakeProvider{
		name: "supp1",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"supp1": "10"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Movie",
			Overview:    "From supp1",
			ProviderIDs: map[string]string{"supp1": "10"},
		},
	}}
	supp2 := &countingProvider{fakeProvider: fakeProvider{name: "supp2"}}

	e, miRepo := newTestEnricherMulti(primary, []domain.MetadataProvider{supp1, supp2})
	item := &domainlib.MediaItem{ID: "m1", Title: "Movie", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())

	assert.Equal(t, "From supp1", item.Overview)
	assert.Equal(t, int32(1), supp1.searchCalls.Load())
	assert.Equal(t, int32(0), supp2.searchCalls.Load(), "supp2 must NOT run after supp1 succeeds")
}

// TestMeta_KillMutant_TranslationsFetched_WhenTmdbIDPresent kills
// enricher_auto.go:73 NEGATION `if tmdbID != ""` (translation path).
func TestMeta_KillMutant_TranslationsFetched_WhenTmdbIDPresent(t *testing.T) {
	tp := &fakeTranslationProvider{
		fakeProvider: fakeProvider{
			name: "tmdb",
			searchResults: []domain.MetadataResult{
				{Title: "Movie", ProviderIDs: map[string]string{"tmdb": "42"}},
			},
			detailResult: &domain.MetadataResult{
				Title:       "Movie",
				Overview:    "ov",
				ProviderIDs: map[string]string{"tmdb": "42"},
			},
		},
		result: map[string]domain.MetadataResult{
			"fr": {Title: "Film", Overview: "ov-fr"},
		},
	}
	transRepo := &fakeMediaTranslationRepo{}
	e, miRepo := newTestEnricherMulti(tp, nil)
	EnricherWithTranslations(transRepo, nil, []string{"fr"})(e)

	item := &domainlib.MediaItem{ID: "m1", Title: "Movie", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())

	assert.GreaterOrEqual(t, tp.fetchCalls.Load(), int32(1), "translations must fetch when tmdb id present")
	assert.GreaterOrEqual(t, transRepo.upserts.Load(), int32(1))
}

// TestMeta_KillMutant_SkipItemWithOverviewAndTrailers kills
// enricher_auto.go:473 NEGATION `Overview != "" && len(Trailers) > 0 { continue }`.
func TestMeta_KillMutant_SkipItemWithOverviewAndTrailers(t *testing.T) {
	primary := &countingProvider{fakeProvider: fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "X", ProviderIDs: map[string]string{"tmdb": "1"}},
		},
		detailResult: &domain.MetadataResult{Title: "X", Overview: "new"},
	}}
	e, miRepo := newTestEnricherMulti(primary, nil)

	already := &domainlib.MediaItem{
		ID:             "m1",
		Title:          "X",
		Type:           "movie",
		Overview:       "already enriched",
		Trailers:       []domainlib.Trailer{{Key: "abc"}},
		Classification: &domainlib.MediaClassification{CertificationsByCountry: map[string]string{"US": "PG-13"}},
	}
	miRepo.Create(context.Background(), already)

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{already}, nil)
	assert.Equal(t, int32(0), primary.searchCalls.Load(), "must skip items with overview+trailers+certs")
	assert.Equal(t, "already enriched", already.Overview)
}

// TestMeta_KillMutant_EnrichItemUpdateError_LogsWarn kills NEGATION at
// enricher_auto.go:67 `if err := EnrichItem; err != nil && log != nil`.
func TestMeta_KillMutant_EnrichItemUpdateError_LogsWarn(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "X", ProviderIDs: map[string]string{"tmdb": "1"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "X",
			Overview:    "ov",
			ProviderIDs: map[string]string{"tmdb": "1"},
		},
	}
	e, miRepo := newTestEnricherMulti(primary, nil)
	miRepo.ErrUpdate = errors.New("boom")

	item := &domainlib.MediaItem{ID: "m1", Title: "X", Type: "movie"}
	miRepo.Create(context.Background(), item)

	log, buf := captureLog()
	e.AutoEnrichItem(context.Background(), item, log)

	require.Contains(t, buf.String(), "auto-enrich failed", "must log warn on EnrichItem error when log != nil")
}

// TestMeta_KillMutant_EpisodeCountMatches_Tolerance kills boundary + arithmetic
// mutants on episodeCountMatches (enricher_auto.go:292-305).
func TestMeta_KillMutant_EpisodeCountMatches_Tolerance(t *testing.T) {
	assert.True(t, episodeCountMatches(10, 10))
	assert.True(t, episodeCountMatches(8, 10))
	assert.True(t, episodeCountMatches(12, 10))
	assert.False(t, episodeCountMatches(7, 10))
	assert.False(t, episodeCountMatches(13, 10))
	assert.False(t, episodeCountMatches(0, 10))
	assert.False(t, episodeCountMatches(10, 0))
	assert.True(t, episodeCountMatches(2, 3))
	assert.False(t, episodeCountMatches(1, 3))
}

// TestMeta_KillMutant_CountDiskEpisodes_NilMediaItems kills
// enricher_auto.go:164 NEGATION `if e.mediaItems == nil`.
func TestMeta_KillMutant_CountDiskEpisodes_NilMediaItems(t *testing.T) {
	e := &Enricher{}
	got, err := e.countDiskEpisodesForSeason(context.Background(), "series-1", 1)
	require.NoError(t, err, "no repository is not a failure - it is a caller with nothing to count")
	assert.Equal(t, 0, got)
}
