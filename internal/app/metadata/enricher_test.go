package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domain "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes local to this test file ---

type fakeProvider struct {
	name          string
	searchResults []domain.MetadataResult
	searchErr     error
	detailResult  *domain.MetadataResult
	detailErr     error
	seasonResult  *domain.MetadataResult
	seasonErr     error
	episodeResult *domain.MetadataResult
	episodeErr    error
	lookupResult  *domain.MetadataResult
	lookupErr     error
	altTitles     map[string][]string // key: providerID
	altTitlesErr  error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Search(_ context.Context, _ string, _ *int, _ string) ([]domain.MetadataResult, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeProvider) GetByProviderID(_ context.Context, _ string, _ string) (*domain.MetadataResult, error) {
	return f.detailResult, f.detailErr
}
func (f *fakeProvider) GetSeasonDetails(_ context.Context, _ string, _ int) (*domain.MetadataResult, error) {
	return f.seasonResult, f.seasonErr
}
func (f *fakeProvider) GetEpisodeDetails(_ context.Context, _ string, _, _ int) (*domain.MetadataResult, error) {
	return f.episodeResult, f.episodeErr
}
func (f *fakeProvider) LookupByExternalID(_ context.Context, _, _ string) (*domain.MetadataResult, error) {
	return f.lookupResult, f.lookupErr
}
func (f *fakeProvider) AlternativeTitles(_ context.Context, providerID string, _ string) ([]string, error) {
	if f.altTitlesErr != nil {
		return nil, f.altTitlesErr
	}
	if f.altTitles != nil {
		return f.altTitles[providerID], nil
	}
	return nil, nil
}

type fakeCollectionLinker struct {
	col     *domainlib.Collection
	err     error
	linkErr error
}

func (f *fakeCollectionLinker) Upsert(_ context.Context, _ int, _, _, _, _ string) (*domainlib.Collection, error) {
	return f.col, f.err
}
func (f *fakeCollectionLinker) LinkMediaItem(_ context.Context, _, _ string, _ int) error {
	return f.linkErr
}

// collectionFake is what the test linkers implement: both sides the enricher
// takes separately.
type collectionFake interface {
	CollectionLinker
	UnlinkedMovieLister
}

func newTestEnricher(primary *fakeProvider, supplementary []*fakeProvider, collections collectionFake) (*Enricher, *metafakes.FakeArtworkRepository, *libfakes.FakePersonRepository, *libfakes.FakeMediaPersonRepository, *libfakes.FakeMediaItemStore, *libfakes.FakeSearchIndex) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemStore()
	searchIdx := libfakes.NewFakeSearchIndex()

	var supps []domain.MetadataProvider
	for _, s := range supplementary {
		supps = append(supps, s)
	}

	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, miRepo, searchIdx, primary, supps, collections, collections)
	return e, artRepo, personRepo, mpRepo, miRepo, searchIdx
}

// --- pickBestMatch tests (existing) ---

func TestPickBestMatch_ExactTitleBeatsTMDBOrder(t *testing.T) {
	// When exactly one candidate's title equals the query (case-folded +
	// diacritic-stripped), it wins even if TMDB's popularity ranking put
	// another word-sharing candidate first. Fixes the real-world bug where
	// a localized title matched an unrelated, differently-named show just
	// because it shared the same "Le Flic de <ville>" localisation pattern
	// and TMDB ranked it higher - the exact-title result was sitting right
	// there in the list.
	results := []domain.MetadataResult{
		{Title: "The Detective : Le Flic de Paris", OriginalTitle: "The Detective"},
		{Title: "Le flic de Marseille", OriginalTitle: "City Patrol"},
	}
	best := pickBestMatch(results, "Le flic de marseille", nil)
	assert.Equal(t, "Le flic de Marseille", best.Title)
}

func TestPickBestMatch_ExactTitleBeatsSubstringPopularity(t *testing.T) {
	// "Some Series" is outranked by "The Foods that Make Some Series" by
	// popularity on a generic query, and both share the word "series".
	// Exact-title shortcut rescues the right one.
	results := []domain.MetadataResult{
		{Title: "The Foods that Make Some Series", OriginalTitle: "The Foods that Make Some Series"},
		{Title: "Some Series", OriginalTitle: "Some Series"},
	}
	best := pickBestMatch(results, "Some Series", nil)
	assert.Equal(t, "Some Series", best.Title)
}

func TestPickBestMatch_ExactTitleDiacriticInsensitive(t *testing.T) {
	// Query without diacritics ("metro show"), TMDB title with diacritics →
	// exact match via the fold table.
	results := []domain.MetadataResult{
		{Title: "Some Other Show"},
		{Title: "Métro Show"},
	}
	best := pickBestMatch(results, "metro show", nil)
	assert.Equal(t, "Métro Show", best.Title)
}

func TestPickBestMatch_SkipsIncoherentCandidates(t *testing.T) {
	// First TMDB result shares no word with the query - skip it and take
	// the next coherent one.
	results := []domain.MetadataResult{
		{Title: "Completely Unrelated"},
		{Title: "Some Series"},
	}
	best := pickBestMatch(results, "Some Series", nil)
	assert.Equal(t, "Some Series", best.Title)
}

func TestPickBestMatch_ExactTitle(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "Some Series"},
		{Title: "Series Some"},
	}
	best := pickBestMatch(results, "Some Series", nil)
	assert.Equal(t, "Some Series", best.Title)
}

func TestPickBestMatch_YearBonus(t *testing.T) {
	y2020 := 2020
	y2005 := 2005
	results := []domain.MetadataResult{
		{Title: "The Movie", Year: &y2020},
		{Title: "The Movie", Year: &y2005},
	}
	best := pickBestMatch(results, "The Movie", &y2005)
	assert.Equal(t, &y2005, best.Year)
}

func TestPickBestMatch_ExactYearWinsOverOrder(t *testing.T) {
	// When year is given and a candidate matches exactly, it wins even if it
	// comes later in TMDB's order.
	y1990 := 1990
	y2017 := 2017
	results := []domain.MetadataResult{
		{Title: "Some Legend", Year: &y1990},
		{Title: "Some Legend", Year: &y2017},
	}
	best := pickBestMatch(results, "Some Legend", &y2017)
	assert.Equal(t, &y2017, best.Year)

	// No year hint → trust TMDB order.
	best = pickBestMatch(results, "Some Legend", nil)
	assert.Equal(t, &y1990, best.Year)
}

func TestPickBestMatch_OriginalTitleUsed(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "La Loi de la Force", OriginalTitle: "The Force of Law"},
		{Title: "La Force de la Loi", OriginalTitle: "Other Show"},
	}
	best := pickBestMatch(results, "The Force of Law", nil)
	assert.Equal(t, "The Force of Law", best.OriginalTitle)
}

func TestPickBestMatch_SingleResult(t *testing.T) {
	results := []domain.MetadataResult{{Title: "Only One"}}
	best := pickBestMatch(results, "anything", nil)
	assert.Equal(t, "Only One", best.Title)
}

// --- roleType ---

func TestRoleType(t *testing.T) {
	assert.Equal(t, "actor", roleType("Actor"))
	assert.Equal(t, "director", roleType("Director"))
	assert.Equal(t, "writer", roleType("Writer"))
	assert.Equal(t, "producer", roleType("Producer"))
	assert.Equal(t, "actor", roleType("Unknown"))
	assert.Equal(t, "actor", roleType(""))
}

// --- mergeProviderIDs ---

func TestMergeProviderIDs(t *testing.T) {
	item := &domainlib.MediaItem{Catalog: &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "123"}}}
	changed := mergeProviderIDs(item, map[string]string{"imdb": "tt123", "tmdb": "999"})
	assert.True(t, changed)
	assert.Equal(t, "tt123", item.Catalog.ProviderIDs["imdb"])
	assert.Equal(t, "123", item.Catalog.ProviderIDs["tmdb"]) // not overwritten

	changed = mergeProviderIDs(item, map[string]string{"tmdb": "999"})
	assert.False(t, changed)
}

func TestMergeProviderIDs_NilMap(t *testing.T) {
	item := &domainlib.MediaItem{}
	changed := mergeProviderIDs(item, map[string]string{"imdb": "tt1"})
	assert.True(t, changed)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, "tt1", item.Catalog.ProviderIDs["imdb"])
}

// --- mergeRatings ---

func TestMergeRatings(t *testing.T) {
	item := &domainlib.MediaItem{
		Classification: &domainlib.MediaClassification{ProviderRatings: map[string]float64{"tmdb": 8.0}},
	}
	changed := mergeRatings(item, map[string]float64{"imdb": 7.5, "tmdb": 9.0})
	assert.True(t, changed)
	assert.InDelta(t, 7.5, item.Classification.ProviderRatings["imdb"], 0.01)
	assert.InDelta(t, 8.0, item.Classification.ProviderRatings["tmdb"], 0.01) // not overwritten

	changed = mergeRatings(item, map[string]float64{"tmdb": 9.0})
	assert.False(t, changed)
}

func TestMergeRatings_NilMap(t *testing.T) {
	item := &domainlib.MediaItem{}
	changed := mergeRatings(item, map[string]float64{"rt": 85})
	assert.True(t, changed)
	require.NotNil(t, item.Classification)
	assert.InDelta(t, 85.0, item.Classification.ProviderRatings["rt"], 0.01)
}

// --- buildArtworkRecord ---

func TestBuildArtworkRecord(t *testing.T) {
	art := domain.ArtworkResult{
		Type: "poster", URL: "http://img.com/p.jpg",
		Width: 500, Height: 750, Language: "en", Provider: "tmdb",
	}
	rec := buildArtworkRecord("media-1", 0, art)
	assert.NotEmpty(t, rec.ID)
	assert.Equal(t, shared.MediaItemID("media-1"), rec.MediaItemID)
	assert.Equal(t, "poster", rec.Type)
	assert.Equal(t, "tmdb", rec.Provider)
	require.NotNil(t, rec.SourceURL)
	assert.Equal(t, "http://img.com/p.jpg", *rec.SourceURL)
	require.NotNil(t, rec.Language)
	assert.Equal(t, "en", *rec.Language)
	require.NotNil(t, rec.Width)
	assert.Equal(t, 500, *rec.Width)
	require.NotNil(t, rec.Height)
	assert.Equal(t, 750, *rec.Height)
	assert.Equal(t, 0, rec.SortOrder)
}

func TestBuildArtworkRecord_NoOptionalFields(t *testing.T) {
	art := domain.ArtworkResult{Type: "backdrop", URL: "http://img.com/b.jpg"}
	rec := buildArtworkRecord("media-2", 1, art)
	assert.Equal(t, "unknown", rec.Provider)
	assert.Nil(t, rec.Language)
	assert.Nil(t, rec.Width)
	assert.Nil(t, rec.Height)
	assert.Equal(t, 1, rec.SortOrder)
}

// --- applyBasicFields ---

func TestApplyBasicFields(t *testing.T) {
	item := &domainlib.MediaItem{ID: "item-1"}
	y := 2020
	rating := 8.5
	premiere := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)
	result := &domain.MetadataResult{
		Title:           "Test Movie",
		OriginalTitle:   "Test Original",
		Overview:        "A great movie",
		Tagline:         "Be great",
		Genres:          []string{"Drama"},
		Studios:         []string{"Studio A"},
		PremiereDate:    &premiere,
		Year:            &y,
		CommunityRating: &rating,
		OfficialRating:  "PG-13",
		ProviderIDs:     map[string]string{"tmdb": "123"},
		Ratings:         map[string]float64{"tmdb": 8.5},
		Trailers: []domain.TrailerResult{
			{Site: "YouTube", Key: "abc", Language: "en", Official: true, Type: "Trailer"},
		},
	}

	applyBasicFields(item, result)

	assert.Equal(t, "Test Movie", item.Title)
	assert.Equal(t, "Test Original", item.OriginalTitle)
	assert.Equal(t, "A great movie", item.Overview)
	assert.Equal(t, "Be great", item.Tagline)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, []string{"Drama"}, item.Catalog.Genres)
	assert.Equal(t, []string{"Studio A"}, item.Catalog.Studios)
	assert.Equal(t, &premiere, item.Catalog.PremiereDate)
	assert.Equal(t, &y, item.Catalog.Year)
	require.NotNil(t, item.Classification)
	assert.Equal(t, &rating, item.Classification.CommunityRating)
	assert.Equal(t, "PG-13", item.Classification.OfficialRating)
	assert.Equal(t, "123", item.Catalog.ProviderIDs["tmdb"])
	assert.InDelta(t, 8.5, item.Classification.ProviderRatings["tmdb"], 0.01)
	require.Len(t, item.Trailers, 1)
	assert.Equal(t, "YouTube", item.Trailers[0].Site)
	assert.Equal(t, "abc", item.Trailers[0].Key)
}

// A provider that returns no OriginalTitle/Tagline/Genres (TMDB season/episode
// lookups) must not blank values an earlier pass already set on the item.
func TestApplyBasicFields_PreservesExistingWhenResultEmpty(t *testing.T) {
	item := &domainlib.MediaItem{
		ID:            "ep-1",
		OriginalTitle: "Kept Original",
		Tagline:       "Kept Tagline",
		Catalog:       &domainlib.MediaCatalog{Genres: []string{"Sci-Fi", "Drama"}},
	}
	// Episode-shaped result: title + overview only, nothing else.
	result := &domain.MetadataResult{
		Title:    "Episode 1",
		Overview: "Pilot",
	}

	applyBasicFields(item, result)

	assert.Equal(t, "Episode 1", item.Title, "title still updates")
	assert.Equal(t, "Pilot", item.Overview, "overview still updates")
	assert.Equal(t, "Kept Original", item.OriginalTitle, "empty result must not blank OriginalTitle")
	assert.Equal(t, "Kept Tagline", item.Tagline, "empty result must not blank Tagline")
	assert.Equal(t, []string{"Sci-Fi", "Drama"}, item.Catalog.Genres, "empty result must not clear genres")
}

func TestApplyBasicFields_MergesProviderIDs(t *testing.T) {
	item := &domainlib.MediaItem{
		Catalog:        &domainlib.MediaCatalog{ProviderIDs: map[string]string{"imdb": "tt1"}},
		Classification: &domainlib.MediaClassification{ProviderRatings: map[string]float64{"imdb": 7.0}},
	}
	result := &domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": "123"},
		Ratings:     map[string]float64{"tmdb": 8.0},
	}
	applyBasicFields(item, result)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, "tt1", item.Catalog.ProviderIDs["imdb"])
	assert.Equal(t, "123", item.Catalog.ProviderIDs["tmdb"])
	require.NotNil(t, item.Classification)
	assert.InDelta(t, 7.0, item.Classification.ProviderRatings["imdb"], 0.01)
	assert.InDelta(t, 8.0, item.Classification.ProviderRatings["tmdb"], 0.01)
}

// --- EnrichItem ---

func TestEnrichItem(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, artRepo, personRepo, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "item-1"}
	miRepo.Create(context.Background(), item)

	result := &domain.MetadataResult{
		Title:   "Enriched Title",
		Artwork: []domain.ArtworkResult{{Type: "poster", URL: "http://img.com/p.jpg", Provider: "tmdb"}},
		People:  []domain.PersonResult{{Name: "Actor A", Role: "Actor", Character: "Hero"}},
	}

	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err)
	assert.Equal(t, "Enriched Title", item.Title)

	arts, _ := artRepo.GetByMediaItem(context.Background(), "item-1")
	assert.Len(t, arts, 1)

	persons, _ := personRepo.List(context.Background())
	assert.Len(t, persons, 1)
	assert.Equal(t, "Actor A", persons[0].Name)
}

func TestEnrichItem_UpdateError(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	miRepo.ErrUpdate = errors.New("db down")

	item := &domainlib.MediaItem{ID: "item-1"}
	err := e.EnrichItem(context.Background(), item, &domain.MetadataResult{Title: "X"})
	assert.ErrorContains(t, err, "update media item")
}

// createBatchFailMP fails ONLY the cast/crew link CreateBatch (Delete + preload
// still succeed), to isolate the previously-swallowed media_person batch error.
type createBatchFailMP struct {
	*libfakes.FakeMediaPersonRepository
}

func (r *createBatchFailMP) CreateBatch(_ context.Context, _ []*domainlib.MediaPerson) error {
	return errors.New("db down on batch")
}

func TestEnrichItem_PersonLinkBatchError_Propagates(t *testing.T) {
	// Regression: a failed cast/crew link write was warn-and-continue, so a
	// per-item "Refresh metadata" reported success while persisting no links.
	// It must now surface as an error (mirroring the artwork/item-update siblings).
	primary := &fakeProvider{name: "tmdb"}
	mp := &createBatchFailMP{FakeMediaPersonRepository: libfakes.NewFakeMediaPersonRepository()}
	miRepo := libfakes.NewFakeMediaItemStore()
	e := NewEnricher(
		metafakes.NewFakeArtworkRepository(),
		libfakes.NewFakePersonRepository(),
		mp,
		miRepo,
		miRepo,
		libfakes.NewFakeSearchIndex(),
		primary, nil, nil, nil,
	)
	item := &domainlib.MediaItem{ID: "item-1"}
	miRepo.Create(context.Background(), item)
	result := &domain.MetadataResult{
		Title:  "X",
		People: []domain.PersonResult{{Name: "Actor A", Role: "Actor"}},
	}
	err := e.EnrichItem(context.Background(), item, result)
	assert.ErrorContains(t, err, "media_person batch")
}

func TestEnrichItem_EmptyArtwork(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, artRepo, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	err := e.EnrichItem(context.Background(), item, &domain.MetadataResult{Title: "X"})
	require.NoError(t, err)

	arts, _ := artRepo.GetByMediaItem(context.Background(), "item-1")
	assert.Empty(t, arts)
}

// TestEnrichItem_EmptyArtwork_KeepsExisting: a re-enrich that returns NO artwork
// (transient provider miss / match without images) must NOT wipe artwork the
// item already has - otherwise posters vanish until the next pass re-adds them.
func TestEnrichItem_EmptyArtwork_KeepsExisting(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, artRepo, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	ctx := context.Background()
	item := &domainlib.MediaItem{ID: "item-1", Title: "X", Type: "movie"}
	require.NoError(t, miRepo.Create(ctx, item))

	require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{
		Title:   "X",
		Artwork: []domain.ArtworkResult{{Type: "poster", URL: "https://img/p.jpg", Provider: "tmdb"}},
	}))
	before, _ := artRepo.GetByMediaItem(ctx, "item-1")
	require.Len(t, before, 1)

	// Re-enrich with empty artwork → existing must survive.
	require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{Title: "X"}))
	after, _ := artRepo.GetByMediaItem(ctx, "item-1")
	require.Len(t, after, 1, "empty provider artwork must not wipe existing")
	assert.Equal(t, before[0].ID, after[0].ID)
}

// TestEnrichItem_EmptyPeople_KeepsExisting: a re-enrich that returns NO people
// (transient provider miss / a supplementary provider with no cast) must NOT
// wipe the cast/crew the item already has - same data-loss class as the empty-
// artwork no-wipe guard. upsertPeople used to DELETE before building the batch,
// so an empty result committed the wipe.
func TestEnrichItem_EmptyPeople_KeepsExisting(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, mpRepo, miRepo, _ := newTestEnricher(primary, nil, nil)
	ctx := context.Background()
	item := &domainlib.MediaItem{ID: "item-1", Title: "X", Type: "movie"}
	require.NoError(t, miRepo.Create(ctx, item))

	require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{
		Title:  "X",
		People: []domain.PersonResult{{Name: "Actor A", Role: "Actor", Character: "Hero"}},
	}))
	before, _ := mpRepo.GetByMediaItemID(ctx, "item-1")
	require.Len(t, before, 1)

	// Re-enrich with empty people → existing cast must survive.
	require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{Title: "X"}))
	after, _ := mpRepo.GetByMediaItemID(ctx, "item-1")
	require.Len(t, after, 1, "empty provider people must not wipe existing cast")
}

// --- upsertPeople: dedup + character ---

func TestUpsertPeople_Dedup(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, mpRepo, miRepo, _ := newTestEnricher(primary, nil, nil)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title: "X",
		People: []domain.PersonResult{
			{Name: "Same Person", Role: "Actor"},
			{Name: "Same Person", Role: "Director"},
			{Name: "Other Person", Role: "Writer"},
		},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err)

	mps, _ := mpRepo.GetByMediaItemID(context.Background(), "item-1")
	assert.Len(t, mps, 2) // "Same Person" deduped
}

// --- indexSearch ---

func TestIndexSearch_NilSearch(t *testing.T) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemStore()
	primary := &fakeProvider{name: "tmdb"}

	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, miRepo, nil, primary, nil, nil, nil)
	// should not panic
	e.indexSearch(context.Background(), &domainlib.MediaItem{ID: "x"}, nil)
}

// --- linkCollection ---

func TestLinkCollection(t *testing.T) {
	col := &domainlib.Collection{ID: "col-1"}
	linker := &fakeCollectionLinker{col: col}
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, linker)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title: "X",
		Collection: &domain.CollectionRef{
			TmdbID: 42, Name: "My Collection",
			PosterURL: "http://img.com/p.jpg", BackdropURL: "http://img.com/b.jpg",
		},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err)
}

func TestLinkCollection_NilLinker(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title:      "X",
		Collection: &domain.CollectionRef{TmdbID: 42, Name: "C"},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err)
}

func TestLinkCollection_UpsertError(t *testing.T) {
	linker := &fakeCollectionLinker{err: errors.New("upsert fail")}
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, linker)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title:      "X",
		Collection: &domain.CollectionRef{TmdbID: 1, Name: "C"},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err) // linkCollection logs, doesn't fail
}

func TestLinkCollection_LinkError(t *testing.T) {
	col := &domainlib.Collection{ID: "col-1"}
	linker := &fakeCollectionLinker{col: col, linkErr: errors.New("link fail")}
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, linker)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title:      "X",
		Collection: &domain.CollectionRef{TmdbID: 1, Name: "C"},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err) // logs warning, no error
}

func TestLinkCollection_NilCollectionReturned(t *testing.T) {
	linker := &fakeCollectionLinker{col: nil}
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, linker)
	miRepo.Create(context.Background(), &domainlib.MediaItem{ID: "item-1"})

	item := &domainlib.MediaItem{ID: "item-1"}
	result := &domain.MetadataResult{
		Title:      "X",
		Collection: &domain.CollectionRef{TmdbID: 1, Name: "C"},
	}
	err := e.EnrichItem(context.Background(), item, result)
	require.NoError(t, err)
}

// --- lookupSupplement ---

func TestLookupSupplement_EmptyIMDB(t *testing.T) {
	sp := &fakeProvider{name: "omdb"}
	result := lookupSupplement(context.Background(), sp, "", "movie")
	assert.Nil(t, result)
}

func TestLookupSupplement_OMDb(t *testing.T) {
	expected := &domain.MetadataResult{Title: "Found"}
	sp := &fakeProvider{name: "omdb", detailResult: expected}
	result := lookupSupplement(context.Background(), sp, "tt123", "movie")
	assert.Equal(t, expected, result)
}

func TestLookupSupplement_ExternalIDLookup(t *testing.T) {
	expected := &domain.MetadataResult{Title: "Found via lookup"}
	sp := &fakeProvider{name: "tvmaze", lookupResult: expected}
	result := lookupSupplement(context.Background(), sp, "tt123", "tv")
	assert.Equal(t, expected, result)
}

func TestLookupSupplement_OMDbError(t *testing.T) {
	sp := &fakeProvider{name: "omdb", detailErr: errors.New("api error")}
	result := lookupSupplement(context.Background(), sp, "tt123", "movie")
	assert.Nil(t, result) // falls through, no ExternalIDLookup on omdb fake
}

// --- searchAndFetchBest (diskS01Episodes=0, matching the removed searchAndFetch wrapper) ---

func TestSearchAndFetchBest_Success(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"tmdb": "100"}},
		},
		detailResult: &domain.MetadataResult{Title: "Movie Full", Overview: "Details"},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)
	log := slog.Default()

	result := e.searchAndFetchBest(context.Background(), primary, "Movie", nil, "movie", 0, log)
	require.NotNil(t, result)
	assert.Equal(t, "Movie Full", result.Title)
}

func TestSearchAndFetchBest_SearchError(t *testing.T) {
	primary := &fakeProvider{name: "tmdb", searchErr: errors.New("fail")}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)
	log := slog.Default()

	result := e.searchAndFetchBest(context.Background(), primary, "X", nil, "movie", 0, log)
	assert.Nil(t, result)
}

func TestSearchAndFetchBest_EmptyResults(t *testing.T) {
	primary := &fakeProvider{name: "tmdb", searchResults: []domain.MetadataResult{}}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetchBest(context.Background(), primary, "X", nil, "movie", 0, nil)
	assert.Nil(t, result)
}

func TestSearchAndFetchBest_NoProviderID(t *testing.T) {
	primary := &fakeProvider{
		name:          "tmdb",
		searchResults: []domain.MetadataResult{{Title: "Movie", ProviderIDs: map[string]string{}}},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetchBest(context.Background(), primary, "Movie", nil, "movie", 0, nil)
	require.NotNil(t, result)
	assert.Equal(t, "Movie", result.Title) // returns search result as-is
}

func TestSearchAndFetchBest_OMDbFallbackToIMDB(t *testing.T) {
	primary := &fakeProvider{
		name: "omdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"imdb": "tt123"}},
		},
		detailResult: &domain.MetadataResult{Title: "Movie Full"},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetchBest(context.Background(), primary, "Movie", nil, "movie", 0, nil)
	require.NotNil(t, result)
	assert.Equal(t, "Movie Full", result.Title)
}

func TestSearchAndFetchBest_DetailError(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"tmdb": "100"}},
		},
		detailErr: errors.New("detail fail"),
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetchBest(context.Background(), primary, "Movie", nil, "movie", 0, nil)
	// On a detail-fetch error we now SKIP the provider (return nil) instead of
	// persisting the search-level skeleton: a non-empty overview with no
	// artwork/cast would trip the non-force re-enrich guard and strand the item
	// in a degraded state forever. nil lets the chain fall through / retry next scan.
	assert.Nil(t, result)
}

// --- AutoEnrichItem ---

func TestAutoEnrichItem_EmptyTitle(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: ""}
	e.AutoEnrichItem(context.Background(), item, nil)
	assert.Empty(t, item.Overview) // no enrichment
}

func TestAutoEnrichItem_AlreadyEnriched(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Movie", Overview: "Already there"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, nil)
	assert.Equal(t, "Already there", item.Overview)
}

// TestAutoEnrichItem_OrphanWithTitleOverride_UsesOverrideForSearch is the
// pré-prod regression: an admin manually cleans a parser-polluted title via
// MediaDetailEdit, expecting the next match-orphans sweep to pick it up.
// Before the fix, the enricher early-returned on "item has overrides" and
// the orphan stayed orphan forever. After the fix, an orphan's overrides
// are NOT a skip signal - they're additional input for the search query.
func TestAutoEnrichItem_OrphanWithTitleOverride_UsesOverrideForSearch(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Quand l'hôpital retient son souffle", ProviderIDs: map[string]string{"tmdb": "999"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Quand l'hôpital retient son souffle",
			Overview:    "Documentaries santé.",
			ProviderIDs: map[string]string{"tmdb": "999"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(overrideRepo)(e)

	// Orphan: media_items.title still holds the parser-polluted form,
	// Catalog is nil → no provider IDs yet.
	item := &domainlib.MediaItem{ID: "item-orphan", Title: "Quand l'hôpital retient son souffle 720", Type: "movie"}
	require.NoError(t, miRepo.Create(context.Background(), item))

	// Admin cleaned the title via UI - override row exists.
	ov, err := domain.NewMetadataOverride("mo-1", "item-orphan", "title",
		domain.WithOverrideValue("Quand l'hôpital retient son souffle"))
	require.NoError(t, err)
	require.NoError(t, overrideRepo.Set(context.Background(), ov))

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Equal(t, "Documentaries santé.", item.Overview, "orphan with title override must still enrich")
}

func TestAutoEnrichAll_OrphanWithOverride_NotSkipped(t *testing.T) {
	// Bulk path mirror of TestAutoEnrichItem_OrphanWithTitleOverride_UsesOverrideForSearch.
	// drainOrphans → AutoEnrichAll(items, ...) had its own override-skip at
	// the bulk gate; without it, the single-item fix never gets reached.
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Quand l'hôpital retient son souffle", ProviderIDs: map[string]string{"tmdb": "999"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Quand l'hôpital retient son souffle",
			Overview:    "Documentaries santé.",
			ProviderIDs: map[string]string{"tmdb": "999"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(overrideRepo)(e)

	item := &domainlib.MediaItem{ID: "orphan-bulk", Title: "Quand l'hôpital retient son souffle 720", Type: "movie"}
	require.NoError(t, miRepo.Create(context.Background(), item))

	ov, err := domain.NewMetadataOverride("mo-bulk", "orphan-bulk", "title",
		domain.WithOverrideValue("Quand l'hôpital retient son souffle"))
	require.NoError(t, err)
	require.NoError(t, overrideRepo.Set(context.Background(), ov))

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{item}, slog.Default())
	stored, err := miRepo.GetByID(context.Background(), "orphan-bulk")
	require.NoError(t, err)
	assert.Equal(t, "Documentaries santé.", stored.Overview, "bulk path must NOT skip orphan with override")
}

func TestAutoEnrichAll_MatchedItemWithOverride_StillSkipped(t *testing.T) {
	primary := &fakeProvider{
		name:         "tmdb",
		detailResult: &domain.MetadataResult{Overview: "REPLACED"},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(overrideRepo)(e)

	item := &domainlib.MediaItem{
		ID: "matched-bulk", Title: "Some Movie", Type: "movie",
		Catalog: &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "603"}},
	}
	require.NoError(t, miRepo.Create(context.Background(), item))

	ov, err := domain.NewMetadataOverride("mo-bulk2", "matched-bulk", "title",
		domain.WithOverrideValue("Custom"))
	require.NoError(t, err)
	require.NoError(t, overrideRepo.Set(context.Background(), ov))

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{item}, slog.Default())
	stored, err := miRepo.GetByID(context.Background(), "matched-bulk")
	require.NoError(t, err)
	assert.Empty(t, stored.Overview, "bulk path matched-with-override stays skipped")
}

func TestAutoEnrichItem_MatchedItemWithOverride_StillSkips(t *testing.T) {
	// Matched item's overrides are display preferences - must not retrigger
	// a TMDB search that would overwrite them. Asserts the historical
	// behaviour stays for the non-orphan case.
	primary := &fakeProvider{
		name: "tmdb",
		detailResult: &domain.MetadataResult{
			Overview: "REPLACED",
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(overrideRepo)(e)

	// Matched item: has provider IDs.
	item := &domainlib.MediaItem{
		ID: "item-matched", Title: "Some Movie", Type: "movie",
		Catalog: &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "603"}},
	}
	require.NoError(t, miRepo.Create(context.Background(), item))

	ov, err := domain.NewMetadataOverride("mo-2", "item-matched", "title",
		domain.WithOverrideValue("Custom Display Title"))
	require.NoError(t, err)
	require.NoError(t, overrideRepo.Set(context.Background(), ov))

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Empty(t, item.Overview, "matched item with override must NOT be re-enriched")
}

// TestEnrichItem_ArtworkIdempotent is the regression guard for the runaway
// artwork duplication: re-enriching an item whose provider artwork is unchanged
// must NOT delete+recreate the rows (which minted fresh records → fresh files →
// duplicate downloads). The stored record keeps its identity across passes.
func TestEnrichItem_ArtworkIdempotent(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, artRepo, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	ctx := context.Background()

	item := &domainlib.MediaItem{ID: "m1", Title: "X", Type: "movie"}
	require.NoError(t, miRepo.Create(ctx, item))

	result := &domain.MetadataResult{
		Title:    "X",
		Overview: "o",
		Artwork: []domain.ArtworkResult{
			{Type: "poster", URL: "https://img/p.jpg", Provider: "tmdb"},
			{Type: "backdrop", URL: "https://img/b.jpg", Provider: "tmdb"},
		},
	}

	require.NoError(t, e.EnrichItem(ctx, item, result))
	first, err := artRepo.GetByMediaItem(ctx, "m1")
	require.NoError(t, err)
	require.Len(t, first, 2)
	firstIDs := map[string]bool{first[0].ID: true, first[1].ID: true}

	// Re-enrich with the SAME artwork set → must be a no-op (records unchanged).
	require.NoError(t, e.EnrichItem(ctx, item, result))
	second, err := artRepo.GetByMediaItem(ctx, "m1")
	require.NoError(t, err)
	require.Len(t, second, 2, "unchanged artwork must not be duplicated")
	for _, r := range second {
		assert.True(t, firstIDs[r.ID], "record %s was recreated - replaceArtwork is not idempotent", r.ID)
	}

	// A genuine change (new poster URL) IS applied.
	result.Artwork[0].URL = "https://img/p2.jpg"
	require.NoError(t, e.EnrichItem(ctx, item, result))
	third, err := artRepo.GetByMediaItem(ctx, "m1")
	require.NoError(t, err)
	require.Len(t, third, 2)
	urls := map[string]bool{}
	for _, r := range third {
		if r.SourceURL != nil {
			urls[*r.SourceURL] = true
		}
	}
	assert.True(t, urls["https://img/p2.jpg"], "changed artwork URL must be applied")
}

func TestAutoEnrichItem_Movie(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Test Movie", ProviderIDs: map[string]string{"tmdb": "100"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Test Movie",
			Overview:    "Great movie",
			ProviderIDs: map[string]string{"tmdb": "100", "imdb": "tt100"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Test Movie", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Equal(t, "Great movie", item.Overview)
}

func TestAutoEnrichItem_FallbackToSupplementary(t *testing.T) {
	primary := &fakeProvider{name: "tmdb", searchResults: nil}
	supp := &fakeProvider{
		name: "omdb",
		searchResults: []domain.MetadataResult{
			{Title: "Fallback Movie", ProviderIDs: map[string]string{"imdb": "tt200"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Fallback Movie",
			Overview:    "Fallback overview",
			ProviderIDs: map[string]string{"imdb": "tt200"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, []*fakeProvider{supp}, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Fallback Movie", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Equal(t, "Fallback overview", item.Overview)
}

func TestAutoEnrichItem_RecoversTMDBMatchViaIMDB(t *testing.T) {
	// TMDB title-search misses, so OMDb wins the match with only an IMDB id and
	// an English plot. The IMDB id maps back to a TMDB record via /find, so the
	// enricher must re-fetch the localized TMDB detail and use it as the
	// canonical result - otherwise the item is stuck in English with no TMDB id
	// (hence no translations) forever.
	primary := &fakeProvider{
		name:          "tmdb",
		searchResults: nil, // title search misses
		lookupResult:  &domain.MetadataResult{ProviderIDs: map[string]string{"tmdb": "100"}},
		detailResult: &domain.MetadataResult{
			Title:       "Le Film",
			Overview:    "Synopsis français",
			ProviderIDs: map[string]string{"tmdb": "100", "imdb": "tt200"},
		},
	}
	supp := &fakeProvider{
		name: "omdb",
		searchResults: []domain.MetadataResult{
			{Title: "The Film", ProviderIDs: map[string]string{"imdb": "tt200"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "The Film",
			Overview:    "English plot",
			ProviderIDs: map[string]string{"imdb": "tt200"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, []*fakeProvider{supp}, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Le Film", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())

	assert.Equal(t, "Synopsis français", item.Overview, "localized TMDB overview must replace OMDb's English plot")
	require.NotNil(t, item.Catalog)
	assert.Equal(t, "100", item.Catalog.ProviderIDs["tmdb"], "recovered TMDB id must be persisted so translations can populate")
	assert.Equal(t, "tt200", item.Catalog.ProviderIDs["imdb"], "IMDB anchor preserved for supplementary lookups")
}

func TestAutoEnrichItem_NoTMDBMappingKeepsSupplementaryMatch(t *testing.T) {
	// When TMDB's /find has no record for the IMDB id, the OMDb match stands -
	// the upgrade must not blank out a real (if English) result.
	primary := &fakeProvider{
		name:          "tmdb",
		searchResults: nil,
		lookupResult:  nil, // /find yields nothing
	}
	supp := &fakeProvider{
		name: "omdb",
		searchResults: []domain.MetadataResult{
			{Title: "Obscure Film", ProviderIDs: map[string]string{"imdb": "tt999"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Obscure Film",
			Overview:    "English plot",
			ProviderIDs: map[string]string{"imdb": "tt999"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, []*fakeProvider{supp}, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Obscure Film", Type: "movie"}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())

	assert.Equal(t, "English plot", item.Overview)
	require.NotNil(t, item.Catalog)
	assert.Empty(t, item.Catalog.ProviderIDs["tmdb"], "no TMDB mapping -> no spurious TMDB id")
	assert.Equal(t, "tt999", item.Catalog.ProviderIDs["imdb"])
}

func TestAutoEnrichItem_TVSeries(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Test Show", ProviderIDs: map[string]string{"tmdb": "200"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Test Show",
			Overview:    "Great show",
			ProviderIDs: map[string]string{"tmdb": "200"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "item-1", Title: "Test Show", Type: domainlib.Series}
	miRepo.Create(context.Background(), item)

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Equal(t, "Great show", item.Overview)
}

// --- supplementItem ---

func TestSupplementItem_MergesExtraData(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	supp := &fakeProvider{
		name:         "omdb",
		detailResult: &domain.MetadataResult{ProviderIDs: map[string]string{"imdb": "tt1"}, Ratings: map[string]float64{"imdb": 8.5}},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, []*fakeProvider{supp}, nil)

	item := &domainlib.MediaItem{
		ID:             "item-1",
		Catalog:        &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "100"}},
		Classification: &domainlib.MediaClassification{ProviderRatings: map[string]float64{"tmdb": 7.0}},
	}
	miRepo.Create(context.Background(), item)

	primaryResult := &domain.MetadataResult{ProviderIDs: map[string]string{"imdb": "tt1"}}
	e.supplementItem(context.Background(), item, primaryResult, "movie", slog.Default())

	require.NotNil(t, item.Catalog)
	assert.Equal(t, "tt1", item.Catalog.ProviderIDs["imdb"])
	require.NotNil(t, item.Classification)
	assert.InDelta(t, 8.5, item.Classification.ProviderRatings["imdb"], 0.01)
}

func TestSupplementItem_SkipsSameProvider(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	supp := &fakeProvider{
		name:         "tmdb", // same as primary
		detailResult: &domain.MetadataResult{Ratings: map[string]float64{"extra": 9.0}},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, []*fakeProvider{supp}, nil)

	item := &domainlib.MediaItem{
		ID:             "item-1",
		Catalog:        &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "100"}},
		Classification: &domainlib.MediaClassification{ProviderRatings: map[string]float64{}},
	}
	miRepo.Create(context.Background(), item)

	primaryResult := &domain.MetadataResult{ProviderIDs: map[string]string{"imdb": "tt1"}}
	e.supplementItem(context.Background(), item, primaryResult, "movie", nil)
	require.NotNil(t, item.Classification)
	assert.Empty(t, item.Classification.ProviderRatings) // nothing merged
}

// --- autoEnrichSeason ---

func TestAutoEnrichSeason(t *testing.T) {
	seasonNum := 1
	primary := &fakeProvider{
		name: "tmdb",
		seasonResult: &domain.MetadataResult{
			Title:    "Season 1",
			Overview: "First season",
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	series := &domainlib.MediaItem{
		ID:       "series-1",
		Title:    "Show",
		Overview: "Series overview",
		Type:     domainlib.Series,
		Catalog:  &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "500"}},
	}
	miRepo.Create(context.Background(), series)

	season := &domainlib.MediaItem{
		ID:          "season-1",
		ParentID:    "series-1",
		Type:        domainlib.Season,
		Title:       "Season 1",
		IndexNumber: &seasonNum,
	}
	miRepo.Create(context.Background(), season)

	e.AutoEnrichItem(context.Background(), season, slog.Default())
	assert.Equal(t, "First season", season.Overview)
}

func TestAutoEnrichSeason_NoParentOrIndex(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	season := &domainlib.MediaItem{ID: "s-1", Title: "S1", Type: domainlib.Season}
	e.AutoEnrichItem(context.Background(), season, nil)
	assert.Empty(t, season.Overview) // no-op
}

// --- autoEnrichEpisode ---

func TestAutoEnrichEpisode(t *testing.T) {
	seasonNum := 1
	episodeNum := 3
	primary := &fakeProvider{
		name: "tmdb",
		episodeResult: &domain.MetadataResult{
			Title:    "Pilot",
			Overview: "First episode",
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	series := &domainlib.MediaItem{
		ID:       "series-1",
		Title:    "Show",
		Overview: "Series overview",
		Type:     domainlib.Series,
		Catalog:  &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "500"}},
	}
	miRepo.Create(context.Background(), series)

	season := &domainlib.MediaItem{
		ID:          "season-1",
		ParentID:    "series-1",
		Type:        domainlib.Season,
		IndexNumber: &seasonNum,
	}
	miRepo.Create(context.Background(), season)

	episode := &domainlib.MediaItem{
		ID:          "ep-1",
		ParentID:    "season-1",
		Title:       "Pilot",
		Type:        domainlib.Episode,
		IndexNumber: &episodeNum,
	}
	miRepo.Create(context.Background(), episode)

	e.AutoEnrichItem(context.Background(), episode, slog.Default())
	assert.Equal(t, "First episode", episode.Overview)
}

func TestAutoEnrichEpisode_NoParentOrIndex(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	ep := &domainlib.MediaItem{ID: "ep-1", Title: "Ep", Type: domainlib.Episode}
	e.AutoEnrichItem(context.Background(), ep, nil)
	assert.Empty(t, ep.Overview)
}

// --- EpisodeCapable ---

func TestEpisodeProviders(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	supp1 := &fakeProvider{name: "tvmaze"}
	supp2 := &fakeProvider{name: "omdb"} // omdb doesn't implement EpisodeDetailProvider via fakeProvider but our fake does
	e, _, _, _, _, _ := newTestEnricher(primary, []*fakeProvider{supp1, supp2}, nil)

	eps := e.chain.EpisodeCapable()
	assert.Len(t, eps, 3) // all fakeProviders implement EpisodeDetailProvider
}

// --- AutoEnrichAll ---

func TestAutoEnrichAll(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie 1", ProviderIDs: map[string]string{"tmdb": "1"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Movie 1",
			Overview:    "Overview 1",
			ProviderIDs: map[string]string{"tmdb": "1"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item1 := &domainlib.MediaItem{ID: "m1", Title: "Movie 1", Type: "movie"}
	item2 := &domainlib.MediaItem{ID: "m2", Title: "Movie 2", Overview: "Exists", Trailers: []domainlib.Trailer{{Key: "x"}}}
	miRepo.Create(context.Background(), item1)
	miRepo.Create(context.Background(), item2)

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{item1, item2}, slog.Default())
	assert.Equal(t, "Overview 1", item1.Overview)
}

func TestAutoEnrichAll_ContextCanceled(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	item := &domainlib.MediaItem{ID: "m1", Title: "X", Type: "movie"}
	e.AutoEnrichAll(ctx, []*domainlib.MediaItem{item}, nil)
	assert.Empty(t, item.Overview) // limiter.Wait returns err immediately
}

// countingOverrideRepo wraps the fake override repo to record how many times
// GetByMediaItem vs GetByMediaItems is called, proving a bulk pass batches
// the admin-override preload into one call instead of one per item.
type countingOverrideRepo struct {
	*metafakes.FakeMetadataOverrideRepository
	mu               sync.Mutex
	getByMediaItemN  int
	getByMediaItemsN int
}

func (r *countingOverrideRepo) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domain.MetadataOverride, error) {
	r.mu.Lock()
	r.getByMediaItemN++
	r.mu.Unlock()
	return r.FakeMetadataOverrideRepository.GetByMediaItem(ctx, mediaItemID)
}

func (r *countingOverrideRepo) GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*domain.MetadataOverride, error) {
	r.mu.Lock()
	r.getByMediaItemsN++
	r.mu.Unlock()
	return r.FakeMetadataOverrideRepository.GetByMediaItems(ctx, mediaItemIDs)
}

// TestAutoEnrichAll_BatchesOverridePreload proves the N+1 fix: a bulk
// "Refresh all metadata" pass over many items must issue a single
// GetByMediaItems call, never a per-item GetByMediaItem. Before the fix,
// autoEnrichItem re-fetched overrides for every already-enriched item that
// autoEnrichAll's skip filter let through, and every to-be-enriched item.
func TestAutoEnrichAll_BatchesOverridePreload(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)
	overrideRepo := &countingOverrideRepo{FakeMetadataOverrideRepository: metafakes.NewFakeMetadataOverrideRepository()}
	EnricherWithOverrides(overrideRepo)(e)

	// Overview intentionally left empty so autoEnrichAll's skip filter lets
	// every item through into autoEnrichItem (where the override lookup this
	// test cares about happens). apiLimiter is 1 req/250ms, so keep n small.
	const n = 5
	items := make([]*domainlib.MediaItem, 0, n)
	for i := range n {
		it := &domainlib.MediaItem{
			ID: shared.MediaItemID(fmt.Sprintf("bulk-%d", i)), Title: fmt.Sprintf("Movie %d", i),
			Type: "movie",
		}
		require.NoError(t, miRepo.Create(context.Background(), it))
		items = append(items, it)
	}

	e.AutoEnrichAll(context.Background(), items, slog.Default())

	assert.Equal(t, 1, overrideRepo.getByMediaItemsN, "must batch the preload into one GetByMediaItems call")
	assert.Equal(t, 0, overrideRepo.getByMediaItemN, "must not fall back to a per-item GetByMediaItem when the caller preloaded")
}

// --- Alternative-titles matching (Fix A) ---

// TestPickBestMatch_AltTitleWins is the localised-alt-title case: TMDB
// search (fr) returns three results; the correct film has a localised
// French title as its primary Title but carries the English title in its
// AlternativeTitles field. pickBestMatch must pick it over a word-sharing
// wrong candidate that appears first.
func TestPickBestMatch_AltTitleWins(t *testing.T) {
	results := []domain.MetadataResult{
		// Wrong film (first in TMDB popularity order) - shares word "train"
		{Title: "The Last Train to Newhaven", OriginalTitle: "The Last Train to Newhaven",
			ProviderIDs: map[string]string{"tmdb": "900002"}},
		// Correct film - French title is primary; English alt-title populated
		{Title: "Dernier train pour Rivemont", OriginalTitle: "리버몬트행",
			AlternativeTitles: []string{"Train to Rivemont"},
			ProviderIDs:       map[string]string{"tmdb": "900001"}},
		// Unrelated sequel
		{Title: "Rivemont Island", OriginalTitle: "리버몬트섬",
			ProviderIDs: map[string]string{"tmdb": "900003"}},
	}
	best := pickBestMatch(results, "Train to Rivemont", nil)
	assert.Equal(t, "Dernier train pour Rivemont", best.Title,
		"alt-title exact match must win over word-share candidate")
	assert.Equal(t, "900001", best.ProviderIDs["tmdb"])
}

// TestFetchBestCandidate_AltTitleLazyFetch verifies that when no candidate
// exactly matches on Title/OriginalTitle, fetchBestCandidate lazily calls
// AlternativeTitles for top candidates and returns the correct one.
func TestFetchBestCandidate_AltTitleLazyFetch(t *testing.T) {
	// Provider returns TMDB search results with localised titles.
	// Alt-titles call for "396535" returns the English title.
	primary := &fakeProvider{
		name: "tmdb",
		altTitles: map[string][]string{
			"900002": {"The Last Train to Newhaven"},
			"900001": {"Train to Rivemont", "리버몬트행"},
		},
		detailResult: &domain.MetadataResult{
			Title:         "Dernier train pour Rivemont",
			OriginalTitle: "리버몬트행",
			Overview:      "A zombie film set on a train to Rivemont.",
			ProviderIDs:   map[string]string{"tmdb": "900001"},
		},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	results := []domain.MetadataResult{
		{Title: "The Last Train to Newhaven", OriginalTitle: "The Last Train to Newhaven",
			ProviderIDs: map[string]string{"tmdb": "900002"}},
		{Title: "Dernier train pour Rivemont", OriginalTitle: "리버몬트행",
			ProviderIDs: map[string]string{"tmdb": "900001"}},
		{Title: "Rivemont Island", OriginalTitle: "리버몬트섬",
			ProviderIDs: map[string]string{"tmdb": "900003"}},
	}

	result := e.fetchBestCandidate(context.Background(), primary, results, "Train to Rivemont", nil, "movie")
	require.NotNil(t, result)
	assert.Equal(t, "900001", result.ProviderIDs["tmdb"],
		"lazy alt-title fetch must identify and return the correct candidate")
	assert.Equal(t, "A zombie film set on a train to Rivemont.", result.Overview,
		"full details must be fetched for the alt-title match")
}

// --- Cascade query retry (Fix B) ---

// TestSearchAndFetchBest_CascadeRetryFindsResult is the "subtitle trimmed
// away" case: the full query returns 0 results, but successive
// word-trimming eventually hits a non-empty result set. The enricher must
// return the best candidate from the trimmed query.
func TestSearchAndFetchBest_CascadeRetryFindsResult(t *testing.T) {
	// The fake provider returns nothing for the full query and for
	// intermediate trims, but returns a result for "Zorblax".
	primary := &fakeProvider{
		name: "tmdb",
		// Search is keyed on exact query string; anything not "Zorblax" → nil
		detailResult: &domain.MetadataResult{
			Title:       "Zorblax: Take your road",
			Overview:    "Action thriller.",
			ProviderIDs: map[string]string{"tmdb": "800001"},
		},
	}
	// Manually wire search results only for the trimmed query.
	primary.searchResults = []domain.MetadataResult{
		{Title: "Zorblax: Take your road", ProviderIDs: map[string]string{"tmdb": "800001"}},
	}

	// We need a provider whose Search returns nothing for the long query
	// but something for the short one. Override with a custom fakeProvider.
	shortQueryProvider := &cascadeTestProvider{
		wantQuery: "Zorblax",
		result: []domain.MetadataResult{
			{Title: "Zorblax: Take your road", ProviderIDs: map[string]string{"tmdb": "800001"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "Zorblax: Take your road",
			Overview:    "Action thriller.",
			ProviderIDs: map[string]string{"tmdb": "800001"},
		},
	}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)

	result := e.searchAndFetchBest(context.Background(), shortQueryProvider,
		"Zorblax Take Your Road", nil, "movie", 0, slog.Default())
	require.NotNil(t, result, "cascade retry must return a result")
	assert.Equal(t, "Zorblax: Take your road", result.Title)
}

// cascadeTestProvider returns results only when the query equals wantQuery.
type cascadeTestProvider struct {
	wantQuery    string
	result       []domain.MetadataResult
	detailResult *domain.MetadataResult
}

func (p *cascadeTestProvider) Name() string { return "tmdb" }
func (p *cascadeTestProvider) Search(_ context.Context, query string, _ *int, _ string) ([]domain.MetadataResult, error) {
	if query == p.wantQuery {
		return p.result, nil
	}
	return nil, nil
}
func (p *cascadeTestProvider) GetByProviderID(_ context.Context, _ string, _ string) (*domain.MetadataResult, error) {
	return p.detailResult, nil
}
func (p *cascadeTestProvider) AlternativeTitles(_ context.Context, _ string, _ string) ([]string, error) {
	return nil, nil
}

// --- EnrichItem atomicity (unit of work + post-commit side effects) ---

// orderingUOW records whether the artwork hook fired while the "transaction"
// function was still running (i.e. before commit).
type orderingUOW struct {
	inFn  bool
	calls int
}

func (u *orderingUOW) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	u.calls++
	u.inFn = true
	err := fn(ctx)
	u.inFn = false
	return err
}

// The artwork post-save hook triggers an auto-DOWNLOAD: it must fire after
// the enrichment transaction commits, never inside it - a mid-tx hook starts
// a download for rows a later failure rolls back (phantom artwork).
func TestEnrichItem_ArtworkHookFiresAfterCommit(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	artRepo := metafakes.NewFakeArtworkRepository()
	miRepo := libfakes.NewFakeMediaItemStore()
	uow := &orderingUOW{}
	hookCalls := 0
	hookDuringTx := false
	e := NewEnricher(
		artRepo,
		libfakes.NewFakePersonRepository(),
		libfakes.NewFakeMediaPersonRepository(),
		miRepo,
		miRepo,
		libfakes.NewFakeSearchIndex(),
		primary, nil, nil, nil,
		EnricherWithUnitOfWork(uow),
		EnricherWithArtworkPostSaveHook(func(_ context.Context, rec *domain.ArtworkRecord) {
			hookCalls++
			if uow.inFn {
				hookDuringTx = true
			}
		}),
	)
	item := &domainlib.MediaItem{ID: "item-1"}
	miRepo.Create(context.Background(), item)
	result := &domain.MetadataResult{
		Title:   "X",
		Artwork: []domain.ArtworkResult{{Type: "poster", URL: "http://img/p.jpg", Provider: "tmdb"}},
		People:  []domain.PersonResult{{Name: "Actor A", Role: "Actor"}},
	}

	require.NoError(t, e.EnrichItem(context.Background(), item, result))
	assert.Equal(t, 1, uow.calls, "the three writes run under one unit of work")
	assert.Equal(t, 1, hookCalls, "hook fires once per persisted artwork record")
	assert.False(t, hookDuringTx, "hook must fire AFTER the transaction, not inside it")
}

// When a later step fails, the hook must NOT have fired at all: previously
// replaceArtwork fired it inline, so a subsequent upsertPeople failure left a
// download running for artwork rows that (with a uow) get rolled back.
func TestEnrichItem_ArtworkHookNotFiredOnFailure(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	miRepo := libfakes.NewFakeMediaItemStore()
	hookCalls := 0
	e := NewEnricher(
		metafakes.NewFakeArtworkRepository(),
		libfakes.NewFakePersonRepository(),
		&createBatchFailMP{FakeMediaPersonRepository: libfakes.NewFakeMediaPersonRepository()},
		miRepo,
		miRepo,
		libfakes.NewFakeSearchIndex(),
		primary, nil, nil, nil,
		EnricherWithUnitOfWork(&orderingUOW{}),
		EnricherWithArtworkPostSaveHook(func(_ context.Context, _ *domain.ArtworkRecord) { hookCalls++ }),
	)
	item := &domainlib.MediaItem{ID: "item-1"}
	miRepo.Create(context.Background(), item)
	result := &domain.MetadataResult{
		Title:   "X",
		Artwork: []domain.ArtworkResult{{Type: "poster", URL: "http://img/p.jpg", Provider: "tmdb"}},
		People:  []domain.PersonResult{{Name: "Actor A", Role: "Actor"}},
	}

	err := e.EnrichItem(context.Background(), item, result)
	require.Error(t, err)
	assert.Zero(t, hookCalls, "no auto-download for artwork rows the failed enrichment rolls back")
}

func (f *fakeCollectionLinker) ListMoviesWithoutCollection(_ context.Context, _ string, _ int) ([]domainlib.UnlinkedMovie, error) {
	return nil, nil
}

// The twin of TestAutoEnrichItem_MatchedItemWithOverride_StillSkips, on the
// read that decides it. `if err == nil && len(overrides) > 0` reads a DB
// failure as "this item has no overrides", so a transient blip is enough to
// re-run the provider chain over an item an admin deliberately corrected. The
// override rows survive and the grid/detail still overlay them, but the stored
// title and overview are replaced underneath - and the search index, which
// serves the provider title directly, then disagrees with every other surface.
func TestAutoEnrichItem_OverrideReadErrorProtectsTheItem(t *testing.T) {
	// The provider must actually be able to answer, otherwise the assertion
	// holds for the wrong reason - a chain that finds nothing leaves Overview
	// empty whether or not the item was protected.
	newProvider := func() *fakeProvider {
		return &fakeProvider{
			name: "tmdb",
			searchResults: []domain.MetadataResult{
				{Title: "Some Movie", ProviderIDs: map[string]string{"tmdb": "603"}},
			},
			detailResult: &domain.MetadataResult{
				Title: "Some Movie", Overview: "REPLACED",
				ProviderIDs: map[string]string{"tmdb": "603"},
			},
		}
	}
	newItem := func() *domainlib.MediaItem {
		return &domainlib.MediaItem{
			ID: "item-matched", Title: "Some Movie", Type: "movie",
			Catalog: &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "603"}},
		}
	}

	// Control: a readable override table protects the item, and the chain is
	// otherwise willing to overwrite it.
	guard, _, _, _, guardRepo, _ := newTestEnricher(newProvider(), nil, nil)
	guardOverrides := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(guardOverrides)(guard)
	guardItem := newItem()
	require.NoError(t, guardRepo.Create(context.Background(), guardItem))
	ov, err := domain.NewMetadataOverride("mo-3", "item-matched", "title",
		domain.WithOverrideValue("Custom Display Title"))
	require.NoError(t, err)
	require.NoError(t, guardOverrides.Set(context.Background(), ov))
	guard.AutoEnrichItem(context.Background(), guardItem, slog.Default())
	require.Empty(t, guardItem.Overview, "control: a readable override must protect the item")

	bare, _, _, _, bareRepo, _ := newTestEnricher(newProvider(), nil, nil)
	EnricherWithOverrides(metafakes.NewFakeMetadataOverrideRepository())(bare)
	bareItem := newItem()
	require.NoError(t, bareRepo.Create(context.Background(), bareItem))
	bare.AutoEnrichItem(context.Background(), bareItem, slog.Default())
	require.Equal(t, "REPLACED", bareItem.Overview,
		"control: with no override the chain does overwrite, so the assertion below is not vacuous")

	e, _, _, _, miRepo, _ := newTestEnricher(newProvider(), nil, nil)
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	EnricherWithOverrides(overrideRepo)(e)
	item := newItem()
	require.NoError(t, miRepo.Create(context.Background(), item))
	overrideRepo.Err = errors.New("db down")

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Empty(t, item.Overview,
		"an unreadable override table must protect the item, not expose it: the safe answer to "+
			"\"does an admin edit exist here?\" is yes")
}

// diskS01Episodes is the guard that rejects a provider match whose season 1
// does not match what is on disk. It is an int, and 0 legitimately means "no
// episodes there" - so returning 0 on a failed read hands the matcher the one
// value that switches the guard OFF, on the very pass where it is most needed.
func TestAutoEnrichItem_DiskCountReadErrorDoesNotDisableTheMatchGuard(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Some Series", ProviderIDs: map[string]string{"tmdb": "900"}},
		},
		detailResult: &domain.MetadataResult{
			Title: "Some Series", Overview: "REPLACED",
			ProviderIDs: map[string]string{"tmdb": "900"},
		},
	}
	e, _, _, _, miRepo, _ := newTestEnricher(primary, nil, nil)

	item := &domainlib.MediaItem{ID: "series-1", Title: "Some Series", Type: domainlib.Series}
	require.NoError(t, miRepo.Create(context.Background(), item))
	miRepo.ErrList = errors.New("db down")

	e.AutoEnrichItem(context.Background(), item, slog.Default())
	assert.Empty(t, item.Overview,
		"a series whose disk structure cannot be read must not be matched against a provider "+
			"with the validation silently disabled")
}
