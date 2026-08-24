package metadata

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domain "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackingMediaTranslationRepo records every Upsert call.
type trackingMediaTranslationRepo struct {
	upserts []*domainlib.MediaItemTranslation
}

func (r *trackingMediaTranslationRepo) Upsert(_ context.Context, t *domainlib.MediaItemTranslation) error {
	r.upserts = append(r.upserts, t)
	return nil
}
func (r *trackingMediaTranslationRepo) UpsertBatch(_ context.Context, ts []*domainlib.MediaItemTranslation) error {
	r.upserts = append(r.upserts, ts...)
	return nil
}
func (r *trackingMediaTranslationRepo) GetByMediaItemAndLocale(_ context.Context, _, _ string) (*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *trackingMediaTranslationRepo) GetByMediaItem(_ context.Context, _ string) ([]*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *trackingMediaTranslationRepo) GetByMediaItemsAndLocale(_ context.Context, _ []string, _ string) (map[string]*domainlib.MediaItemTranslation, error) {
	return nil, nil
}
func (r *trackingMediaTranslationRepo) DeleteByMediaItem(_ context.Context, _ string) error {
	return nil
}

// trackingPersonTranslationRepo records every Upsert call.
type trackingPersonTranslationRepo struct {
	upserts []*domainlib.PersonTranslation
}

func (r *trackingPersonTranslationRepo) Upsert(_ context.Context, t *domainlib.PersonTranslation) error {
	r.upserts = append(r.upserts, t)
	return nil
}
func (r *trackingPersonTranslationRepo) GetByPersonAndLocale(_ context.Context, _, _ string) (*domainlib.PersonTranslation, error) {
	return nil, nil
}
func (r *trackingPersonTranslationRepo) GetByPerson(_ context.Context, _ string) ([]*domainlib.PersonTranslation, error) {
	return nil, nil
}
func (r *trackingPersonTranslationRepo) DeleteByPerson(_ context.Context, _ string) error {
	return nil
}

// translationProviderStub implements MetadataProvider + TranslationProvider.
type translationProviderStub struct {
	fakeProvider
	mediaResult     map[string]domain.MetadataResult
	mediaErr        error
	personResult    map[string]string
	personErr       error
	mediaCalls      atomic.Int32
	personCalls     atomic.Int32
	lastMediaType   string
	lastLocales     []string
	lastProviderID  string
	lastPersonID    string
	lastPersonScope []string
}

func (s *translationProviderStub) FetchTranslations(_ context.Context, providerID, mediaType string, locales []string) (map[string]domain.MetadataResult, error) {
	s.mediaCalls.Add(1)
	s.lastProviderID = providerID
	s.lastMediaType = mediaType
	s.lastLocales = locales
	return s.mediaResult, s.mediaErr
}
func (s *translationProviderStub) FetchPersonTranslations(_ context.Context, personID string, locales []string) (map[string]string, error) {
	s.personCalls.Add(1)
	s.lastPersonID = personID
	s.lastPersonScope = locales
	return s.personResult, s.personErr
}

// personDetailProviderStub implements MetadataProvider + TranslationProvider + personDetailFetcher.
// Used for fetchPersonBio coverage via findOrCreatePerson public path (upsertPeople -> EnrichItem).
type personDetailProviderStub struct {
	translationProviderStub
	details    *domainlib.PersonDetailsResult
	detailsErr error
	detailHits atomic.Int32
}

func (s *personDetailProviderStub) GetPersonDetails(_ context.Context, _ string) (*domainlib.PersonDetailsResult, error) {
	s.detailHits.Add(1)
	return s.details, s.detailsErr
}

func newTransEnricher(primary domain.MetadataProvider) (*Enricher, *libfakes.FakeMediaItemRepository, *libfakes.FakePersonRepository) {
	artRepo := metafakes.NewFakeArtworkRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mpRepo := libfakes.NewFakeMediaPersonRepository()
	miRepo := libfakes.NewFakeMediaItemRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, searchIdx, primary, nil, nil)
	return e, miRepo, personRepo
}

// --- EnrichTranslationsForItem ---

func TestEnrichTranslationsForItem_NoRepo_NoOp(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		mediaResult:  map[string]domain.MetadataResult{"fr": {Title: "X"}},
	}
	e, _, _ := newTransEnricher(stub)
	// no EnricherWithTranslations applied → mediaTranslations == nil

	item := &domainlib.MediaItem{ID: "m1", Type: "movie"}
	e.EnrichTranslationsForItem(context.Background(), item, "42", "movie")

	assert.Equal(t, int32(0), stub.mediaCalls.Load(), "must not call provider when repo nil")
}

func TestEnrichTranslationsForItem_EmptyLocales_NoOp(t *testing.T) {
	stub := &translationProviderStub{fakeProvider: fakeProvider{name: "tmdb"}}
	repo := &trackingMediaTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(repo, nil, []string{})(e)

	item := &domainlib.MediaItem{ID: "m1", Type: "movie"}
	e.EnrichTranslationsForItem(context.Background(), item, "42", "movie")

	assert.Equal(t, int32(0), stub.mediaCalls.Load())
	assert.Empty(t, repo.upserts)
}

func TestEnrichTranslationsForItem_ProviderNotTranslationProvider_NoOp(t *testing.T) {
	// fakeProvider does NOT implement TranslationProvider.
	primary := &fakeProvider{name: "tmdb"}
	repo := &trackingMediaTranslationRepo{}
	e, _, _ := newTransEnricher(primary)
	EnricherWithTranslations(repo, nil, []string{"fr"})(e)

	item := &domainlib.MediaItem{ID: "m1", Type: "movie"}
	e.EnrichTranslationsForItem(context.Background(), item, "42", "movie")

	assert.Empty(t, repo.upserts)
}

func TestEnrichTranslationsForItem_FetchError_LogsAndNoUpsert(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		mediaErr:     errors.New("boom"),
	}
	repo := &trackingMediaTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(repo, nil, []string{"fr", "es"})(e)

	item := &domainlib.MediaItem{ID: "m1", Type: "movie"}
	e.EnrichTranslationsForItem(context.Background(), item, "42", "movie")

	assert.Equal(t, int32(1), stub.mediaCalls.Load())
	assert.Empty(t, repo.upserts, "no Upsert when fetch fails")
}

func TestEnrichTranslationsForItem_HappyPath_UpsertsEachLocale(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		mediaResult: map[string]domain.MetadataResult{
			"fr": {Title: "Film", Overview: "ov-fr", Tagline: "tag-fr"},
			"es": {Title: "Pelicula", Overview: "ov-es", Tagline: "tag-es"},
		},
	}
	repo := &trackingMediaTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(repo, nil, []string{"fr", "es"})(e)

	item := &domainlib.MediaItem{ID: "m1", Type: "movie"}
	e.EnrichTranslationsForItem(context.Background(), item, "42", "movie")

	assert.Equal(t, int32(1), stub.mediaCalls.Load())
	assert.Equal(t, "42", stub.lastProviderID)
	assert.Equal(t, "movie", stub.lastMediaType)
	assert.Equal(t, []string{"fr", "es"}, stub.lastLocales)
	require.Len(t, repo.upserts, 2)

	byLocale := map[string]*domainlib.MediaItemTranslation{}
	for _, u := range repo.upserts {
		byLocale[u.Locale] = u
		assert.Equal(t, shared.MediaItemID("m1"), u.MediaItemID)
	}
	assert.Equal(t, "Film", byLocale["fr"].Title)
	assert.Equal(t, "ov-fr", byLocale["fr"].Overview)
	assert.Equal(t, "tag-fr", byLocale["fr"].Tagline)
	assert.Equal(t, "Pelicula", byLocale["es"].Title)
}

// --- EnrichPersonTranslations ---

func TestEnrichPersonTranslations_NoRepo_NoOp(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		personResult: map[string]string{"fr": "bio-fr"},
	}
	e, _, _ := newTransEnricher(stub)

	p := &domainlib.Person{ID: "p1", Name: "Alice"}
	e.EnrichPersonTranslations(context.Background(), p, 99)

	assert.Equal(t, int32(0), stub.personCalls.Load())
}

func TestEnrichPersonTranslations_EmptyLocales_NoOp(t *testing.T) {
	stub := &translationProviderStub{fakeProvider: fakeProvider{name: "tmdb"}}
	repo := &trackingPersonTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(nil, repo, []string{})(e)

	p := &domainlib.Person{ID: "p1", Name: "Alice"}
	e.EnrichPersonTranslations(context.Background(), p, 99)

	assert.Equal(t, int32(0), stub.personCalls.Load())
	assert.Empty(t, repo.upserts)
}

func TestEnrichPersonTranslations_ProviderNotTranslationProvider_NoOp(t *testing.T) {
	primary := &fakeProvider{name: "tmdb"}
	repo := &trackingPersonTranslationRepo{}
	e, _, _ := newTransEnricher(primary)
	EnricherWithTranslations(nil, repo, []string{"fr"})(e)

	p := &domainlib.Person{ID: "p1", Name: "Alice"}
	e.EnrichPersonTranslations(context.Background(), p, 99)

	assert.Empty(t, repo.upserts)
}

func TestEnrichPersonTranslations_FetchError_LogsAndNoUpsert(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		personErr:    errors.New("boom"),
	}
	repo := &trackingPersonTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(nil, repo, []string{"fr"})(e)

	p := &domainlib.Person{ID: "p1", Name: "Alice"}
	e.EnrichPersonTranslations(context.Background(), p, 99)

	assert.Equal(t, int32(1), stub.personCalls.Load())
	assert.Empty(t, repo.upserts)
}

func TestEnrichPersonTranslations_HappyPath_UpsertsEachLocale(t *testing.T) {
	stub := &translationProviderStub{
		fakeProvider: fakeProvider{name: "tmdb"},
		personResult: map[string]string{
			"fr": "bio-fr",
			"es": "bio-es",
		},
	}
	repo := &trackingPersonTranslationRepo{}
	e, _, _ := newTransEnricher(stub)
	EnricherWithTranslations(nil, repo, []string{"fr", "es"})(e)

	p := &domainlib.Person{ID: "p1", Name: "Alice"}
	e.EnrichPersonTranslations(context.Background(), p, 1234)

	assert.Equal(t, int32(1), stub.personCalls.Load())
	assert.Equal(t, "1234", stub.lastPersonID, "tmdbID must be stringified")
	assert.Equal(t, []string{"fr", "es"}, stub.lastPersonScope)
	require.Len(t, repo.upserts, 2)

	byLocale := map[string]*domainlib.PersonTranslation{}
	for _, u := range repo.upserts {
		byLocale[u.Locale] = u
		assert.Equal(t, "p1", u.PersonID)
	}
	assert.Equal(t, "bio-fr", byLocale["fr"].Biography)
	assert.Equal(t, "bio-es", byLocale["es"].Biography)
}

// --- EnricherWithPersonFetcher + fetchPersonBio (via EnrichItem path) ---

func TestEnricherWithPersonFetcher_SetsFetcher_AndBioPopulated(t *testing.T) {
	// translationProviderStub satisfies TranslationProvider (needed by fetchPersonBio).
	// personDetailProviderStub embeds it AND implements personDetailFetcher.
	stub := &personDetailProviderStub{
		translationProviderStub: translationProviderStub{
			fakeProvider: fakeProvider{name: "tmdb"},
		},
		details: &domainlib.PersonDetailsResult{
			Biography:    "an actor's life",
			Birthday:     "1970-01-01",
			Deathday:     "",
			PlaceOfBirth: "Paris",
			KnownFor:     "Acting",
		},
	}

	e, miRepo, personRepo := newTransEnricher(stub)
	// Apply the option (this is what we want to cover).
	EnricherWithPersonFetcher(stub)(e)
	assert.NotNil(t, e.personFetcher, "EnricherWithPersonFetcher must assign personFetcher")

	// Drive fetchPersonBio via EnrichItem -> upsertPeople -> findOrCreatePerson.
	item := &domainlib.MediaItem{ID: "m1", Title: "Movie", Type: "movie"}
	require.NoError(t, miRepo.Create(context.Background(), item))

	result := domainmetaResult("Movie", "Alice", "42")
	require.NoError(t, e.EnrichItem(context.Background(), item, result))

	// fetchPersonBio should have run via the e.primary cast (TranslationProvider + personDetailFetcher).
	assert.GreaterOrEqual(t, stub.detailHits.Load(), int32(1), "GetPersonDetails must be invoked")

	// Verify person row updated.
	stored, err := personRepo.GetByName(context.Background(), "Alice")
	require.NoError(t, err)
	assert.Equal(t, "an actor's life", stored.Biography)
	assert.Equal(t, "Paris", stored.PlaceOfBirth)
	assert.Equal(t, "Acting", stored.KnownFor)
	assert.Equal(t, "1970-01-01", stored.Birthday)
}

// helper to build a MetadataResult featuring one cast person with a TMDB ID.
func domainmetaResult(title, personName, personTMDB string) *domain.MetadataResult {
	return &domain.MetadataResult{
		Title:       title,
		ProviderIDs: map[string]string{"tmdb": "1"},
		People: []domain.PersonResult{
			{Name: personName, Role: "Actor", ProviderID: personTMDB},
		},
	}
}

// fetchPersonBio opens with FetchPersonTranslations(id, []string{""}), a call
// whose own comment says it "returns nothing useful" and whose result is
// discarded on the next line (`_ = bios`). Its only remaining effect was to
// early-return on error, so one rate-limited useless call was enough to skip
// the GetPersonDetails that actually fetches the biography - and the empty bio
// then reads as "the provider has none".
func TestFetchPersonBio_UselessTranslationCallDoesNotBlockTheBio(t *testing.T) {
	stub := &personDetailProviderStub{
		translationProviderStub: translationProviderStub{
			fakeProvider: fakeProvider{name: "tmdb"},
			personErr:    errors.New("429 rate limited"),
		},
		details: &domainlib.PersonDetailsResult{
			Biography:    "an actor's life",
			PlaceOfBirth: "Paris",
		},
	}

	e, miRepo, personRepo := newTransEnricher(stub)
	EnricherWithPersonFetcher(stub)(e)

	item := &domainlib.MediaItem{ID: "m1", Title: "Movie", Type: "movie"}
	require.NoError(t, miRepo.Create(context.Background(), item))
	require.NoError(t, e.EnrichItem(context.Background(), item, domainmetaResult("Movie", "Alice", "42")))

	assert.GreaterOrEqual(t, stub.detailHits.Load(), int32(1),
		"the biography fetch must not depend on a call whose result is thrown away")
	stored, err := personRepo.GetByName(context.Background(), "Alice")
	require.NoError(t, err)
	assert.Equal(t, "an actor's life", stored.Biography)
}
