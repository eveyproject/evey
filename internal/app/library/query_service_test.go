package library_test

import (
	"context"
	"sync"
	"testing"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
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
	if r.items[string(t.MediaItemID)] == nil {
		r.items[string(t.MediaItemID)] = make(map[string]*library.MediaItemTranslation)
	}
	r.items[string(t.MediaItemID)][t.Locale] = t
	return nil
}

func (r *fakeMediaItemTranslationRepo) UpsertBatch(_ context.Context, ts []*library.MediaItemTranslation) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range ts {
		if r.items[string(t.MediaItemID)] == nil {
			r.items[string(t.MediaItemID)] = make(map[string]*library.MediaItemTranslation)
		}
		r.items[string(t.MediaItemID)][t.Locale] = t
	}
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

func (r *fakeMediaItemTranslationRepo) GetByMediaItemsAndLocale(_ context.Context, ids []string, locale string) (map[string]*library.MediaItemTranslation, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*library.MediaItemTranslation)
	for _, id := range ids {
		if m, ok := r.items[id]; ok {
			if t, ok2 := m[locale]; ok2 {
				out[id] = t
			}
		}
	}
	return out, nil
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
	// No translations repo wired -> ErrNotConfigured, not silent (nil, nil).
	require.ErrorIs(t, err, shared.ErrNotConfigured)
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
		applib.WithTranslations(mediaTransRepo, newFakePersonTranslationRepo()),
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
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), newFakePersonTranslationRepo()),
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
	require.ErrorIs(t, err, shared.ErrNotConfigured)
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
		applib.WithTranslations(newFakeMediaItemTranslationRepo(), personTransRepo),
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
	require.ErrorIs(t, err, shared.ErrNotConfigured)
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
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Falcon Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Falcon Returns"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m3", Title: "Skyward"}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "falcon", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, hits, 2)
}

func TestQueryService_SearchMedia_LimitApplied(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Falcon Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Falcon Returns"}, nil)

	hits, _, err := f.svc.SearchMedia(context.Background(), "falcon", nil, nil, 1)
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
		ID: "m1", Title: "Falcon Begins", Type: "movie",
		Catalog:        &library.MediaCatalog{Year: &year, Genres: []string{"action"}},
		Classification: &library.MediaClassification{OfficialRating: "PG-13"},
	}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "falcon", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, hits, 1)
	assert.Equal(t, "m1", hits[0].ID)
	assert.Equal(t, "Falcon Begins", hits[0].Title)
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
		ID: "m1", Title: "Some Movie", Type: "movie",
		LibraryID:      "lib-1",
		Catalog:        &library.MediaCatalog{Year: &year, Genres: []string{"sci-fi", "thriller"}},
		Classification: &library.MediaClassification{CommunityRating: &rating},
	})

	item, err := f.svc.GetMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("m1"), item.ID)
	assert.Equal(t, "Some Movie", item.Title)
	assert.Equal(t, library.MediaItemType("movie"), item.Type)
	assert.Equal(t, shared.LibraryID("lib-1"), item.LibraryID)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, 2020, *item.Catalog.Year)
	assert.InEpsilon(t, 8.5, *item.Classification.CommunityRating, 1e-9)
	assert.Equal(t, []string{"sci-fi", "thriller"}, item.Catalog.Genres)
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
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Old Film", Catalog: &library.MediaCatalog{Year: &y2000}})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "Mid Film", Catalog: &library.MediaCatalog{Year: &y2010}})
	f.media.Seed(&library.MediaItem{ID: "m3", Title: "New Film", Catalog: &library.MediaCatalog{Year: &y2020}})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{YearFrom: 2005, YearTo: 2015})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m2"), items[0].ID)
}

func TestQueryService_ListMediaItems_FilterByMinRating(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	low, high := 3.0, 8.5
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Low Rated", Classification: &library.MediaClassification{CommunityRating: &low}})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "High Rated", Classification: &library.MediaClassification{CommunityRating: &high}})

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
		applib.WithTranslations(mediaTransRepo, newFakePersonTranslationRepo()),
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
	assert.Empty(t, chs)
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
	assert.Equal(t, shared.MediaItemID("m1"), manifest.MediaItemID)
}
