package library_test

import (
	"context"
	"errors"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCachingRepo(
	inner library.PersonRepository,
	fetcher library.PersonDetailsFetcher,
	transFetcher library.PersonTranslationFetcher,
	transRepo library.PersonTranslationRepository,
	locales []string,
) library.PersonRepository {
	return applib.NewCachingPersonRepo(inner, fetcher, transFetcher, transRepo, locales)
}

func TestCachingPersonRepo_GetByID_NoBioFetch_WhenAlreadyPopulated(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: "Already has bio"})
	fetcher := &fakePersonDetailsFetcher{Result: &library.PersonDetailsResult{Biography: "Should not be used"}}

	repo := newCachingRepo(inner, fetcher, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "Already has bio", person.Biography)
}

func TestCachingPersonRepo_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	repo := newCachingRepo(inner, nil, nil, nil, nil)

	_, err := repo.GetByID(context.Background(), "unknown")
	require.Error(t, err)
}

func TestCachingPersonRepo_GetByID_LazyFetchBio(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	thumbURL := "http://img.example.com/alice.jpg"
	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{
			Biography:    "Fetched bio",
			Birthday:     "1990-01-01",
			PlaceOfBirth: "Paris",
			KnownFor:     "Acting",
			ThumbURL:     thumbURL,
		},
	}
	repo := newCachingRepo(inner, fetcher, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Fetched bio", person.Biography)
	assert.Equal(t, "1990-01-01", person.Birthday)
	assert.Equal(t, "Paris", person.PlaceOfBirth)
	assert.Equal(t, "Acting", person.KnownFor)
	require.NotNil(t, person.ThumbPath)
	assert.Equal(t, thumbURL, *person.ThumbPath)

	persisted, _ := inner.GetByID(context.Background(), "p1")
	assert.Equal(t, "Fetched bio", persisted.Biography)
}

func TestCachingPersonRepo_GetByID_LazyFetchBio_NoThumbOverwrite(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	existingThumb := "/existing/thumb.jpg"
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: "", ThumbPath: &existingThumb})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio", ThumbURL: "http://new-thumb.jpg"},
	}
	repo := newCachingRepo(inner, fetcher, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "/existing/thumb.jpg", *person.ThumbPath)
}

func TestCachingPersonRepo_GetByID_FetcherError(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{Err: errors.New("tmdb down")}
	repo := newCachingRepo(inner, fetcher, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "", person.Biography)
}

func TestCachingPersonRepo_GetByID_NilFetcher(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})
	repo := newCachingRepo(inner, nil, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "", person.Biography)
}

func TestCachingPersonRepo_GetByID_WithTranslations(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{Result: &library.PersonDetailsResult{Biography: "Bio EN"}}
	transRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{Result: map[string]string{"fr": "Bio FR", "de": "Bio DE"}}

	repo := newCachingRepo(inner, fetcher, transFetcher, transRepo, []string{"fr", "de"})

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Bio EN", person.Biography)

	frTrans, _ := transRepo.GetByPersonAndLocale(context.Background(), "p1", "fr")
	require.NotNil(t, frTrans)
	assert.Equal(t, "Bio FR", frTrans.Biography)

	deTrans, _ := transRepo.GetByPersonAndLocale(context.Background(), "p1", "de")
	require.NotNil(t, deTrans)
	assert.Equal(t, "Bio DE", deTrans.Biography)
}

func TestCachingPersonRepo_GetByID_TranslationFetchError(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{Result: &library.PersonDetailsResult{Biography: "Bio EN"}}
	transRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{Err: errors.New("fetch failed")}

	repo := newCachingRepo(inner, fetcher, transFetcher, transRepo, []string{"fr"})

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Bio EN", person.Biography)

	frTrans, _ := transRepo.GetByPersonAndLocale(context.Background(), "p1", "fr")
	assert.Nil(t, frTrans)
}

// Kills mutation at person_cache_repo.go len(r.locales) > 0.
// With an empty locale list, FetchPersonTranslations must NOT be invoked.
func TestCachingPersonRepo_GetByID_EmptyLocales_FetcherNotCalled(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	detailsFetcher := &fakePersonDetailsFetcher{Result: &library.PersonDetailsResult{Biography: "Bio EN"}}
	transRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{}

	repo := newCachingRepo(inner, detailsFetcher, transFetcher, transRepo, []string{})

	_, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, 0, transFetcher.Calls, "translation fetcher must not be invoked when locales list is empty")
}

// Update-only failure: person should still be returned with enriched bio.
type updateFailPersonRepo struct {
	inner     *libfakes.FakePersonRepository
	updateErr error
}

func (r *updateFailPersonRepo) Create(ctx context.Context, p *library.Person) error {
	return r.inner.Create(ctx, p)
}
func (r *updateFailPersonRepo) Update(_ context.Context, _ *library.Person) error {
	return r.updateErr
}
func (r *updateFailPersonRepo) GetByID(ctx context.Context, id string) (*library.Person, error) {
	return r.inner.GetByID(ctx, id)
}
func (r *updateFailPersonRepo) GetByName(ctx context.Context, name string) (*library.Person, error) {
	return r.inner.GetByName(ctx, name)
}
func (r *updateFailPersonRepo) GetByNamesBatch(ctx context.Context, names []string) (map[string]*library.Person, error) {
	return r.inner.GetByNamesBatch(ctx, names)
}
func (r *updateFailPersonRepo) List(ctx context.Context) ([]*library.Person, error) {
	return r.inner.List(ctx)
}

func TestCachingPersonRepo_GetByID_UpdateFails_BioStillReturned(t *testing.T) {
	t.Parallel()
	fake := libfakes.NewFakePersonRepository()
	fake.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})
	inner := &updateFailPersonRepo{inner: fake, updateErr: errors.New("update boom")}

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "From TMDB", Birthday: "1970-01-01"},
	}
	repo := newCachingRepo(inner, fetcher, nil, nil, nil)

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "From TMDB", person.Biography)
	assert.Equal(t, "1970-01-01", person.Birthday)
}

// Upsert-only failure: person still returned, error swallowed with warn.
type upsertFailPersonTransRepo struct {
	Err error
}

func (r *upsertFailPersonTransRepo) Upsert(_ context.Context, _ *library.PersonTranslation) error {
	return r.Err
}
func (r *upsertFailPersonTransRepo) GetByPersonAndLocale(_ context.Context, _, _ string) (*library.PersonTranslation, error) {
	return nil, nil
}
func (r *upsertFailPersonTransRepo) GetByPerson(_ context.Context, _ string) ([]*library.PersonTranslation, error) {
	return nil, nil
}
func (r *upsertFailPersonTransRepo) DeleteByPerson(_ context.Context, _ string) error { return nil }

func TestCachingPersonRepo_GetByID_TranslationUpsertFails_StillReturns(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	detailsFetcher := &fakePersonDetailsFetcher{Result: &library.PersonDetailsResult{Biography: "Bio EN"}}
	transFetcher := &fakePersonTranslationFetcher{Result: map[string]string{"fr": "Bio FR"}}
	transRepo := &upsertFailPersonTransRepo{Err: errors.New("upsert boom")}

	repo := newCachingRepo(inner, detailsFetcher, transFetcher, transRepo, []string{"fr"})

	person, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "Bio EN", person.Biography)
	assert.Equal(t, 1, transFetcher.Calls, "translation fetcher was invoked")
}
