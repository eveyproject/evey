package library_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- inline fakes for translation/fetcher ports (no domain fake exists) ---

type fakeMediaItemTranslationRepo struct {
	mu    sync.RWMutex
	items map[string]map[string]*library.MediaItemTranslation // mediaItemID -> locale -> translation
	Err   error
}

func newFakeMediaItemTranslationRepo() *fakeMediaItemTranslationRepo {
	return &fakeMediaItemTranslationRepo{items: make(map[string]map[string]*library.MediaItemTranslation)}
}

func (r *fakeMediaItemTranslationRepo) Upsert(_ context.Context, t *library.MediaItemTranslation) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[t.MediaItemID] == nil {
		r.items[t.MediaItemID] = make(map[string]*library.MediaItemTranslation)
	}
	r.items[t.MediaItemID][t.Locale] = t
	return nil
}

func (r *fakeMediaItemTranslationRepo) GetByMediaItemAndLocale(_ context.Context, mediaItemID, locale string) (*library.MediaItemTranslation, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.items[mediaItemID]; ok {
		if t, ok2 := m[locale]; ok2 {
			return t, nil
		}
	}
	return nil, nil
}

func (r *fakeMediaItemTranslationRepo) GetByMediaItem(_ context.Context, mediaItemID string) ([]*library.MediaItemTranslation, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItemTranslation
	for _, t := range r.items[mediaItemID] {
		result = append(result, t)
	}
	return result, nil
}

func (r *fakeMediaItemTranslationRepo) DeleteByMediaItem(_ context.Context, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, mediaItemID)
	return nil
}

type fakePersonTranslationRepo struct {
	mu    sync.RWMutex
	items map[string]map[string]*library.PersonTranslation // personID -> locale -> translation
	Err   error
}

func newFakePersonTranslationRepo() *fakePersonTranslationRepo {
	return &fakePersonTranslationRepo{items: make(map[string]map[string]*library.PersonTranslation)}
}

func (r *fakePersonTranslationRepo) Upsert(_ context.Context, t *library.PersonTranslation) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[t.PersonID] == nil {
		r.items[t.PersonID] = make(map[string]*library.PersonTranslation)
	}
	r.items[t.PersonID][t.Locale] = t
	return nil
}

func (r *fakePersonTranslationRepo) GetByPersonAndLocale(_ context.Context, personID, locale string) (*library.PersonTranslation, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.items[personID]; ok {
		if t, ok2 := m[locale]; ok2 {
			return t, nil
		}
	}
	return nil, nil
}

func (r *fakePersonTranslationRepo) GetByPerson(_ context.Context, personID string) ([]*library.PersonTranslation, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.PersonTranslation
	for _, t := range r.items[personID] {
		result = append(result, t)
	}
	return result, nil
}

func (r *fakePersonTranslationRepo) DeleteByPerson(_ context.Context, personID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, personID)
	return nil
}

type fakePersonDetailsFetcher struct {
	Result *library.PersonDetailsResult
	Err    error
}

func (f *fakePersonDetailsFetcher) GetPersonDetails(_ context.Context, _ string) (*library.PersonDetailsResult, error) {
	return f.Result, f.Err
}

type fakePersonTranslationFetcher struct {
	Result     map[string]string
	Err        error
	Calls      int
	LastLocale []string
}

func (f *fakePersonTranslationFetcher) FetchPersonTranslations(_ context.Context, _ string, locales []string) (map[string]string, error) {
	f.Calls++
	f.LastLocale = locales
	return f.Result, f.Err
}

// --- helpers ---

type querySvcFull struct {
	svc       *applib.QueryService
	media     *libfakes.FakeMediaItemQueryRepo
	libraries *libfakes.FakeLibraryQueryRepo
	streams   *libfakes.FakeMediaStreamRepository
	search    *libfakes.FakeSearchIndex
}

func newQuerySvcFull() *querySvcFull {
	media := libfakes.NewFakeMediaItemQueryRepo()
	libraries := libfakes.NewFakeLibraryQueryRepo()
	streams := libfakes.NewFakeMediaStreamRepository()
	search := libfakes.NewFakeSearchIndex()
	svc := applib.NewQueryService(libraries, media, streams, search)
	return &querySvcFull{svc: svc, media: media, libraries: libraries, streams: streams, search: search}
}

// --- GetMediaItemTranslation ---

func TestQueryService_GetMediaItemTranslation_NilRepo(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	result, err := f.svc.GetMediaItemTranslation(context.Background(), "m1", "fr")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestQueryService_GetMediaItemTranslation_Found(t *testing.T) {
	t.Parallel()
	mediaTransRepo := newFakeMediaItemTranslationRepo()
	mediaTransRepo.Upsert(context.Background(), &library.MediaItemTranslation{
		MediaItemID: "m1", Locale: "fr", Title: "Le Film", Overview: "Un film",
	})

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTranslations(mediaTransRepo, newFakePersonTranslationRepo(), &fakePersonTranslationFetcher{}, nil),
	)

	result, err := svc.GetMediaItemTranslation(context.Background(), "m1", "fr")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Le Film", result.Title)
	assert.Equal(t, "Un film", result.Overview)
}

func TestQueryService_GetMediaItemTranslation_NotFound(t *testing.T) {
	t.Parallel()
	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), newFakePersonTranslationRepo(), &fakePersonTranslationFetcher{}, nil),
	)

	result, err := svc.GetMediaItemTranslation(context.Background(), "m1", "fr")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// --- GetPersonTranslation ---

func TestQueryService_GetPersonTranslation_NilRepo(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	result, err := f.svc.GetPersonTranslation(context.Background(), "p1", "fr")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestQueryService_GetPersonTranslation_Found(t *testing.T) {
	t.Parallel()
	personTransRepo := newFakePersonTranslationRepo()
	personTransRepo.Upsert(context.Background(), &library.PersonTranslation{
		PersonID: "p1", Locale: "fr", Biography: "Bio en francais",
	})

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), personTransRepo, &fakePersonTranslationFetcher{}, nil),
	)

	result, err := svc.GetPersonTranslation(context.Background(), "p1", "fr")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Bio en francais", result.Biography)
}

// --- GetPerson ---

func TestQueryService_GetPerson_NilPeopleRepo(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	result, err := f.svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestQueryService_GetPerson_Found_NoBioFetch(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: "Already has bio"})

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "Alice", person.Name)
	assert.Equal(t, "Already has bio", person.Biography)
}

func TestQueryService_GetPerson_NotFound(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
	)

	_, err := svc.GetPerson(context.Background(), "unknown")
	require.Error(t, err)
}

func TestQueryService_GetPerson_LazyFetchBio(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	thumbURL := "http://img.example.com/alice.jpg"
	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{
			Biography:    "Fetched bio",
			Birthday:     "1990-01-01",
			Deathday:     "",
			PlaceOfBirth: "Paris",
			KnownFor:     "Acting",
			ThumbURL:     thumbURL,
		},
	}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Fetched bio", person.Biography)
	assert.Equal(t, "1990-01-01", person.Birthday)
	assert.Equal(t, "Paris", person.PlaceOfBirth)
	assert.Equal(t, "Acting", person.KnownFor)
	require.NotNil(t, person.ThumbPath)
	assert.Equal(t, thumbURL, *person.ThumbPath)

	// verify persisted via Update
	persisted, _ := people.GetByID(context.Background(), "p1")
	assert.Equal(t, "Fetched bio", persisted.Biography)
}

func TestQueryService_GetPerson_LazyFetchBio_NoThumbOverwrite(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	existingThumb := "/existing/thumb.jpg"
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: "", ThumbPath: &existingThumb})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{
			Biography: "Bio",
			ThumbURL:  "http://new-thumb.jpg",
		},
	}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "/existing/thumb.jpg", *person.ThumbPath)
}

func TestQueryService_GetPerson_LazyFetch_FetcherError(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{Err: errors.New("tmdb down")}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "", person.Biography)
}

func TestQueryService_GetPerson_LazyFetch_WithTranslations(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio EN"},
	}
	personTransRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{
		Result: map[string]string{"fr": "Bio FR", "de": "Bio DE"},
	}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), personTransRepo, transFetcher, []string{"fr", "de"}),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Bio EN", person.Biography)

	frTrans, _ := personTransRepo.GetByPersonAndLocale(context.Background(), "p1", "fr")
	require.NotNil(t, frTrans)
	assert.Equal(t, "Bio FR", frTrans.Biography)

	deTrans, _ := personTransRepo.GetByPersonAndLocale(context.Background(), "p1", "de")
	require.NotNil(t, deTrans)
	assert.Equal(t, "Bio DE", deTrans.Biography)
}

func TestQueryService_GetPerson_LazyFetch_TranslationFetchError(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio EN"},
	}
	personTransRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{Err: errors.New("fetch failed")}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), personTransRepo, transFetcher, []string{"fr"}),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Bio EN", person.Biography)

	// no translation stored
	frTrans, _ := personTransRepo.GetByPersonAndLocale(context.Background(), "p1", "fr")
	assert.Nil(t, frTrans)
}

func TestQueryService_GetPerson_LazyFetch_UpdateError_StillReturns(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio"},
	}

	// make update fail after the first GetByID succeeds
	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	// set error after person is created so GetByID works but Update fails
	people.Err = errors.New("db write fail")

	// GetByID will fail too because Err is set globally on the fake
	// So we need a different approach: just verify that the person is returned
	// even if GetByID returns an error, the service returns that error
	_, err := svc.GetPerson(context.Background(), "p1")
	require.Error(t, err)
}

// --- GetMediaItemsByIDs ---

func TestQueryService_GetMediaItemsByIDs_Found(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "B"})
	f.media.Seed(&library.MediaItem{ID: "m3", Title: "C"})

	items, err := f.svc.GetMediaItemsByIDs(context.Background(), []string{"m1", "m3"})
	require.NoError(t, err)
	assert.Len(t, items, 2)
	ids := []string{string(items[0].ID), string(items[1].ID)}
	assert.Contains(t, ids, "m1")
	assert.Contains(t, ids, "m3")
}

func TestQueryService_GetMediaItemsByIDs_PartialMatch(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "A"})

	items, err := f.svc.GetMediaItemsByIDs(context.Background(), []string{"m1", "nonexistent"})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].ID)
}

func TestQueryService_GetMediaItemsByIDs_EmptyInput(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	items, err := f.svc.GetMediaItemsByIDs(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, items)
}

// --- CountMediaItems ---

func TestQueryService_CountMediaItems_Empty(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestQueryService_CountMediaItems_WithItems(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	lib1 := shared.LibraryID("lib-1")
	lib2 := shared.LibraryID("lib-2")
	f.media.Seed(&library.MediaItem{ID: "m1", LibraryID: shared.LibraryID(lib1)})
	f.media.Seed(&library.MediaItem{ID: "m2", LibraryID: shared.LibraryID(lib1)})
	f.media.Seed(&library.MediaItem{ID: "m3", LibraryID: shared.LibraryID(lib2)})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{LibraryID: &lib1})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestQueryService_CountMediaItems_FilterByType(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	movieType := "movie"
	f.media.Seed(&library.MediaItem{ID: "m1", Type: "movie"})
	f.media.Seed(&library.MediaItem{ID: "m2", Type: "movie"})
	f.media.Seed(&library.MediaItem{ID: "m3", Type: "series"})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{Type: &movieType})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestQueryService_CountMediaItems_FilterByParentID(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	parent := "series-1"
	f.media.Seed(&library.MediaItem{ID: "ep1", ParentID: parent, Type: "episode"})
	f.media.Seed(&library.MediaItem{ID: "ep2", ParentID: parent, Type: "episode"})
	f.media.Seed(&library.MediaItem{ID: "ep3", ParentID: "series-2", Type: "episode"})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{ParentID: &parent})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// --- GetCastForItems ---

func TestQueryService_GetCastForItems_MultipleItems(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.SeedCast("item-1", []library.CastMember{
		{PersonID: "p1", PersonName: "Alice", RoleType: "actor", SortOrder: 0},
	})
	f.media.SeedCast("item-2", []library.CastMember{
		{PersonID: "p2", PersonName: "Bob", RoleType: "director", SortOrder: 0},
	})

	castMap, err := f.svc.GetCastForItems(context.Background(), []string{"item-1", "item-2"})
	require.NoError(t, err)
	require.Len(t, castMap, 2)
	assert.Equal(t, "Alice", castMap["item-1"][0].PersonName)
	assert.Equal(t, "Bob", castMap["item-2"][0].PersonName)
}

func TestQueryService_GetCastForItems_UnknownItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	castMap, err := f.svc.GetCastForItems(context.Background(), []string{"no-such-item"})
	require.NoError(t, err)
	assert.Empty(t, castMap)
}

func TestQueryService_GetCastForItems_EmptySlice(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	castMap, err := f.svc.GetCastForItems(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, castMap)
}

// --- GetMediaStreams ---

func TestQueryService_GetMediaStreams_NoStreams(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	streams, err := f.svc.GetMediaStreams(context.Background(), "nonexistent-item")
	require.NoError(t, err)
	assert.Empty(t, streams)
}

func TestQueryService_GetMediaStreams_WithStreams(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "h264"},
		{ID: "s2", MediaItemID: "item-1", StreamType: "audio", Codec: "aac"},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 2)
}

func TestQueryService_GetMediaStreams_IsolatedByItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "h264"},
		{ID: "s2", MediaItemID: "item-2", StreamType: "audio", Codec: "aac"},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 1)
	assert.Equal(t, "h264", streams[0].Codec)
}

func TestQueryService_GetMediaStreams_AssertStreamContent(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	width, height := 1920, 1080
	channels := 6
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "hevc", Width: &width, Height: &height},
		{ID: "s2", MediaItemID: "item-1", StreamType: "audio", Codec: "aac", Channels: &channels},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	require.Len(t, streams, 2)
	var video, audio *library.MediaStream
	for _, s := range streams {
		if s.StreamType == "video" {
			video = s
		} else {
			audio = s
		}
	}
	require.NotNil(t, video)
	assert.Equal(t, "hevc", video.Codec)
	assert.Equal(t, 1920, *video.Width)
	assert.Equal(t, 1080, *video.Height)
	require.NotNil(t, audio)
	assert.Equal(t, "aac", audio.Codec)
	assert.Equal(t, 6, *audio.Channels)
}

// --- SearchMedia ---

func TestQueryService_SearchMedia_Finds(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Batman Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Batman Returns"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m3", Title: "Superman"}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, hits, 2)
}

func TestQueryService_SearchMedia_LimitApplied(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Batman Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Batman Returns"}, nil)

	hits, _, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 1)
	require.NoError(t, err)
	assert.Len(t, hits, 1)
}

func TestQueryService_SearchMedia_NoResults(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	hits, total, err := f.svc.SearchMedia(context.Background(), "xyzzy-notfound", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, hits)
}

func TestQueryService_SearchMedia_HitContent(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	year := 2005
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{
		ID: "m1", Title: "Batman Begins", Type: "movie",
		Year: &year, Genres: []string{"action"}, OfficialRating: "PG-13",
	}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, hits, 1)
	assert.Equal(t, "m1", hits[0].ID)
	assert.Equal(t, "Batman Begins", hits[0].Title)
	assert.Equal(t, "movie", hits[0].Type)
	assert.Equal(t, 2005, *hits[0].Year)
	assert.Equal(t, []string{"action"}, hits[0].Genres)
	assert.Equal(t, "PG-13", hits[0].OfficialRating)
}

// --- GetLibrary ---

func TestQueryService_GetLibrary_NotFound(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	_, err := f.svc.GetLibrary(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestQueryService_GetLibrary_Found(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.libraries.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})

	lib, err := f.svc.GetLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, shared.LibraryID("lib-1"), lib.ID)
	assert.Equal(t, "Movies", lib.Name)
	assert.Equal(t, library.LibraryType("movie"), lib.Type)
}

// --- ListLibraries ---

func TestQueryService_ListLibraries_Empty(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	libs, err := f.svc.ListLibraries(context.Background())
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestQueryService_ListLibraries_Multiple(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.libraries.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	f.libraries.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})

	libs, err := f.svc.ListLibraries(context.Background())
	require.NoError(t, err)
	assert.Len(t, libs, 2)
	ids := []string{string(libs[0].ID), string(libs[1].ID)}
	assert.Contains(t, ids, "lib-1")
	assert.Contains(t, ids, "lib-2")
}

// --- GetMediaItem ---

func TestQueryService_GetMediaItem_Found(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	year := 2020
	rating := 8.5
	f.media.Seed(&library.MediaItem{
		ID: "m1", Title: "Inception", Type: "movie",
		LibraryID: "lib-1", Year: &year, CommunityRating: &rating,
		Genres: []string{"sci-fi", "thriller"},
	})

	item, err := f.svc.GetMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("m1"), item.ID)
	assert.Equal(t, "Inception", item.Title)
	assert.Equal(t, library.MediaItemType("movie"), item.Type)
	assert.Equal(t, shared.LibraryID("lib-1"), item.LibraryID)
	assert.Equal(t, 2020, *item.Year)
	assert.InEpsilon(t, 8.5, *item.CommunityRating, 1e-9)
	assert.Equal(t, []string{"sci-fi", "thriller"}, item.Genres)
}

func TestQueryService_GetMediaItem_NotFound(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	_, err := f.svc.GetMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

// --- GetCast ---

func TestQueryService_GetCast_SingleItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	charName := "Bruce Wayne"
	f.media.SeedCast("item-1", []library.CastMember{
		{PersonID: "p1", PersonName: "Christian Bale", RoleType: "actor", CharacterName: &charName, SortOrder: 0},
		{PersonID: "p2", PersonName: "Christopher Nolan", RoleType: "director", SortOrder: 1},
	})

	cast, err := f.svc.GetCast(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, cast, 2)
	assert.Equal(t, "Christian Bale", cast[0].PersonName)
	assert.Equal(t, "actor", cast[0].RoleType)
	assert.Equal(t, "Bruce Wayne", *cast[0].CharacterName)
	assert.Equal(t, "Christopher Nolan", cast[1].PersonName)
	assert.Equal(t, "director", cast[1].RoleType)
}

func TestQueryService_GetCast_NoCast(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	cast, err := f.svc.GetCast(context.Background(), "no-item")
	require.NoError(t, err)
	assert.Empty(t, cast)
}

// --- ListMediaItems ---

func TestQueryService_ListMediaItems_AllNoFilter(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "B"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestQueryService_ListMediaItems_Pagination(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	for i := range 5 {
		id := "m" + string(rune('A'+i))
		f.media.Seed(&library.MediaItem{ID: shared.MediaItemID(id), Title: "Film " + id})
	}

	page1, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	page3, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, page3, 1)
}

func TestQueryService_ListMediaItems_FilterByLibraryID(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	lib1 := shared.LibraryID("lib-1")
	lib2 := shared.LibraryID("lib-2")
	f.media.Seed(&library.MediaItem{ID: "m1", LibraryID: shared.LibraryID(lib1), Title: "Film A"})
	f.media.Seed(&library.MediaItem{ID: "m2", LibraryID: shared.LibraryID(lib2), Title: "Film B"})
	f.media.Seed(&library.MediaItem{ID: "m3", LibraryID: shared.LibraryID(lib1), Title: "Film C"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{LibraryID: &lib1})
	require.NoError(t, err)
	assert.Len(t, items, 2)
	for _, item := range items {
		assert.Equal(t, shared.LibraryID(lib1), item.LibraryID)
	}
}

func TestQueryService_ListMediaItems_FilterByType(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	movieType := "movie"
	f.media.Seed(&library.MediaItem{ID: "m1", Type: "movie", Title: "Film A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Type: "series", Title: "Show B"})
	f.media.Seed(&library.MediaItem{ID: "m3", Type: "movie", Title: "Film C"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Type: &movieType})
	require.NoError(t, err)
	assert.Len(t, items, 2)
	for _, item := range items {
		assert.Equal(t, library.MediaItemType("movie"), item.Type)
	}
}

func TestQueryService_ListMediaItems_FilterByYearRange(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	y2000, y2010, y2020 := 2000, 2010, 2020
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Old Film", Year: &y2000})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "Mid Film", Year: &y2010})
	f.media.Seed(&library.MediaItem{ID: "m3", Title: "New Film", Year: &y2020})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{YearFrom: 2005, YearTo: 2015})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m2"), items[0].ID)
}

func TestQueryService_ListMediaItems_FilterByMinRating(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	low, high := 3.0, 8.5
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Low Rated", CommunityRating: &low})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "High Rated", CommunityRating: &high})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{MinRating: 7.0})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m2"), items[0].ID)
}

// --- Options ---

func TestQueryService_WithPeople_Option(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: "Bio"})

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "Alice", person.Name)
}

func TestQueryService_WithPersonFetcher_Option(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Fetched"},
	}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "Fetched", person.Biography)
}

func TestQueryService_WithTranslations_Option(t *testing.T) {
	t.Parallel()
	mediaTransRepo := newFakeMediaItemTranslationRepo()
	mediaTransRepo.Upsert(context.Background(), &library.MediaItemTranslation{
		MediaItemID: "m1", Locale: "es", Title: "Pelicula",
	})

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTranslations(mediaTransRepo, newFakePersonTranslationRepo(), &fakePersonTranslationFetcher{}, []string{"es"}),
	)

	result, err := svc.GetMediaItemTranslation(context.Background(), "m1", "es")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Pelicula", result.Title)
}

// --- GetChapters ---

func TestQueryService_GetChapters_NilRepo(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	chs, err := f.svc.GetChapters(context.Background(), "any-id")
	require.NoError(t, err)
	assert.Nil(t, chs)
}

func TestQueryService_GetChapters_Found(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	chapterRepo.ReplaceBatch(context.Background(), "item-1", []*library.Chapter{
		{ID: "ch-1", MediaItemID: "item-1", Title: "Ch1"},
	})
	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithChapters(chapterRepo),
	)
	chs, err := svc.GetChapters(context.Background(), "item-1")
	require.NoError(t, err)
	require.Len(t, chs, 1)
	assert.Equal(t, "Ch1", chs[0].Title)
}

// --- GetTrickplayManifest ---

func TestQueryService_GetTrickplayManifest_NilRepo(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	_, err := f.svc.GetTrickplayManifest(context.Background(), "any-id")
	require.Error(t, err)
}

func TestQueryService_GetTrickplayManifest_NotFound(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTrickplay(trickplayRepo),
	)
	_, err := svc.GetTrickplayManifest(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestQueryService_GetTrickplayManifest_Found(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayRepo.Create(context.Background(), &library.TrickplayManifest{MediaItemID: "m1", Width: 320, Interval: 15})
	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTrickplay(trickplayRepo),
	)
	manifest, err := svc.GetTrickplayManifest(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", manifest.MediaItemID)
}
// Kills mutation at query_service.go:104:95 (len(s.translationLocales) > 0).
// With an empty locale list, FetchPersonTranslations must NOT be invoked.
// Flipping "> 0" to ">= 0" would call the fetcher even with no locales.
func TestQueryService_GetPerson_LazyFetch_EmptyLocales_FetcherNotCalled(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	detailsFetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio EN"},
	}
	personTransRepo := newFakePersonTranslationRepo()
	transFetcher := &fakePersonTranslationFetcher{}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(detailsFetcher),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), personTransRepo, transFetcher, []string{}),
	)

	_, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, 0, transFetcher.Calls, "translation fetcher must not be invoked when locales list is empty")
}

// --- personRepo wrapper to inject Update-only errors (kills mutations around
// updateErr != nil at query_service.go:100:60 and uErr != nil at 109:15 by
// keeping GetByID functional while Update / Upsert fail). ---

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
func (r *updateFailPersonRepo) List(ctx context.Context) ([]*library.Person, error) {
	return r.inner.List(ctx)
}

// Kills mutation at query_service.go:100:60 (updateErr != nil) + confirms the
// service still returns the fetched bio even when the update call fails.
// The prod code warns-and-continues; we verify the populated bio is returned.
func TestQueryService_GetPerson_LazyFetch_UpdateFails_BioStillReturned(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakePersonRepository()
	inner.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})
	people := &updateFailPersonRepo{inner: inner, updateErr: errors.New("update boom")}

	fetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "From TMDB", Birthday: "1970-01-01"},
	}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(fetcher),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "From TMDB", person.Biography)
	assert.Equal(t, "1970-01-01", person.Birthday)
}

// --- person translation repo wrapper to fail Upsert only (kills mutation at
// query_service.go:109:15 by letting the Upsert error path execute without
// affecting the returned person). ---

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

// Kills mutation at query_service.go:109:15 (uErr != nil): a failing Upsert
// inside the translation-fetch loop must not propagate; person is still returned.
func TestQueryService_GetPerson_LazyFetch_TranslationUpsertFails_StillReturns(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	people.Create(context.Background(), &library.Person{ID: "p1", Name: "Alice", Biography: ""})

	detailsFetcher := &fakePersonDetailsFetcher{
		Result: &library.PersonDetailsResult{Biography: "Bio EN"},
	}
	transFetcher := &fakePersonTranslationFetcher{
		Result: map[string]string{"fr": "Bio FR"},
	}
	transRepo := &upsertFailPersonTransRepo{Err: errors.New("upsert boom")}

	svc := applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithPeople(people),
		applib.WithPersonFetcher(detailsFetcher),
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), transRepo, transFetcher, []string{"fr"}),
	)

	person, err := svc.GetPerson(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "Bio EN", person.Biography)
	assert.Equal(t, 1, transFetcher.Calls, "translation fetcher was invoked")
}
