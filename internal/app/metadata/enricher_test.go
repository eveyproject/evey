package metadata

import (
	"context"
	"errors"
	"log/slog"
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

func newTestEnricher(primary *fakeProvider, supplementary []*fakeProvider, collections CollectionLinker) (*Enricher, *metafakes.FakeArtworkRepository, *libfakes.FakePersonRepository, *libfakes.FakeMediaPersonRepository, *libfakes.FakeMediaItemRepository, *libfakes.FakeSearchIndex) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemRepository()
	searchIdx := libfakes.NewFakeSearchIndex()

	var supps []domain.MetadataProvider
	for _, s := range supplementary {
		supps = append(supps, s)
	}

	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, searchIdx, primary, supps, collections)
	return e, artRepo, personRepo, mpRepo, miRepo, searchIdx
}

// --- pickBestMatch tests (existing) ---

func TestPickBestMatch_TrustsTMDBOrderWhenBothCoherent(t *testing.T) {
	// TMDB ranks the most relevant result first; we trust that order rather
	// than re-ranking with our own heuristic. Both candidates share words
	// with the query here, so we return the first one verbatim.
	results := []domain.MetadataResult{
		{Title: "The Rookie : Le Flic de Los Angeles", OriginalTitle: "The Rookie"},
		{Title: "Le flic de Shanghai", OriginalTitle: "Martial Law"},
	}
	best := pickBestMatch(results, "Le flic de shanghai", nil)
	assert.Equal(t, "The Rookie : Le Flic de Los Angeles", best.Title)
}

func TestPickBestMatch_SkipsIncoherentCandidates(t *testing.T) {
	// First TMDB result shares no word with the query — skip it and take
	// the next coherent one.
	results := []domain.MetadataResult{
		{Title: "Completely Unrelated"},
		{Title: "Breaking Bad"},
	}
	best := pickBestMatch(results, "Breaking Bad", nil)
	assert.Equal(t, "Breaking Bad", best.Title)
}

func TestPickBestMatch_ExactTitle(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "Breaking Bad"},
		{Title: "Bad Breaking"},
	}
	best := pickBestMatch(results, "Breaking Bad", nil)
	assert.Equal(t, "Breaking Bad", best.Title)
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
		{Title: "Robin Hood", Year: &y1990},
		{Title: "Robin Hood", Year: &y2017},
	}
	best := pickBestMatch(results, "Robin Hood", &y2017)
	assert.Equal(t, &y2017, best.Year)

	// No year hint → trust TMDB order.
	best = pickBestMatch(results, "Robin Hood", nil)
	assert.Equal(t, &y1990, best.Year)
}

func TestPickBestMatch_OriginalTitleUsed(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "La Loi de la Force", OriginalTitle: "Martial Law"},
		{Title: "La Force de la Loi", OriginalTitle: "Other Show"},
	}
	best := pickBestMatch(results, "Martial Law", nil)
	assert.Equal(t, "Martial Law", best.OriginalTitle)
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
	miRepo := libfakes.NewFakeMediaItemRepository()
	primary := &fakeProvider{name: "tmdb"}

	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, nil, primary, nil, nil)
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

// --- searchAndFetch ---

func TestSearchAndFetch_Success(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"tmdb": "100"}},
		},
		detailResult: &domain.MetadataResult{Title: "Movie Full", Overview: "Details"},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)
	log := slog.Default()

	result := e.searchAndFetch(context.Background(), primary, "Movie", nil, "movie", log)
	require.NotNil(t, result)
	assert.Equal(t, "Movie Full", result.Title)
}

func TestSearchAndFetch_SearchError(t *testing.T) {
	primary := &fakeProvider{name: "tmdb", searchErr: errors.New("fail")}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)
	log := slog.Default()

	result := e.searchAndFetch(context.Background(), primary, "X", nil, "movie", log)
	assert.Nil(t, result)
}

func TestSearchAndFetch_EmptyResults(t *testing.T) {
	primary := &fakeProvider{name: "tmdb", searchResults: []domain.MetadataResult{}}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetch(context.Background(), primary, "X", nil, "movie", nil)
	assert.Nil(t, result)
}

func TestSearchAndFetch_NoProviderID(t *testing.T) {
	primary := &fakeProvider{
		name:          "tmdb",
		searchResults: []domain.MetadataResult{{Title: "Movie", ProviderIDs: map[string]string{}}},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetch(context.Background(), primary, "Movie", nil, "movie", nil)
	require.NotNil(t, result)
	assert.Equal(t, "Movie", result.Title) // returns search result as-is
}

func TestSearchAndFetch_OMDbFallbackToIMDB(t *testing.T) {
	primary := &fakeProvider{
		name: "omdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"imdb": "tt123"}},
		},
		detailResult: &domain.MetadataResult{Title: "Movie Full"},
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetch(context.Background(), primary, "Movie", nil, "movie", nil)
	require.NotNil(t, result)
	assert.Equal(t, "Movie Full", result.Title)
}

func TestSearchAndFetch_DetailError(t *testing.T) {
	primary := &fakeProvider{
		name: "tmdb",
		searchResults: []domain.MetadataResult{
			{Title: "Movie", ProviderIDs: map[string]string{"tmdb": "100"}},
		},
		detailErr: errors.New("detail fail"),
	}
	e, _, _, _, _, _ := newTestEnricher(primary, nil, nil)

	result := e.searchAndFetch(context.Background(), primary, "Movie", nil, "movie", nil)
	require.NotNil(t, result)
	assert.Equal(t, "Movie", result.Title) // falls back to search result
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

// --- episodeProviders ---

func TestEpisodeProviders(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	supp1 := &fakeProvider{name: "tvmaze"}
	supp2 := &fakeProvider{name: "omdb"} // omdb doesn't implement EpisodeDetailProvider via fakeProvider but our fake does
	e, _, _, _, _, _ := newTestEnricher(primary, []*fakeProvider{supp1, supp2}, nil)

	eps := e.episodeProviders()
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
