package metadata_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTranslationsEnricher records EnrichTranslationsForItem calls.
type fakeTranslationsEnricher struct {
	mu      sync.Mutex
	calls   []translationCall
	singleN atomic.Int32
}

type translationCall struct {
	item      *library.MediaItem
	tmdbID    string
	mediaType string
}

func (f *fakeTranslationsEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (f *fakeTranslationsEnricher) EnrichTranslationsForItem(_ context.Context, item *library.MediaItem, tmdbID, mediaType string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, translationCall{item: item, tmdbID: tmdbID, mediaType: mediaType})
}
func (f *fakeTranslationsEnricher) AutoEnrichItem(_ context.Context, _ *library.MediaItem, _ *slog.Logger) {
	f.singleN.Add(1)
}

func newCmdSvcNoEnricher() *appmetadata.CommandService {
	return appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
}

// --- RefreshAllTranslations ---

func TestRefreshAllTranslations_NoEnricher(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	err := cmdSvc.RefreshAllTranslations(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enricher not configured")
}

func TestRefreshAllTranslations_ListError(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	mediaRepo.ErrList = errors.New("db down")
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	err := cmdSvc.RefreshAllTranslations(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list all items")
}

func TestRefreshAllTranslations_SkipsItemsWithoutTMDB(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Type: library.MovieItem, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "42"}}}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Type: library.MovieItem, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"imdb": "tt1"}}}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "m3", Type: library.MovieItem, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": ""}}}))

	var progressCalls int
	err := cmdSvc.RefreshAllTranslations(ctx, func(done, total int) {
		progressCalls++
		assert.Equal(t, 3, total)
		assert.LessOrEqual(t, done, total)
	})
	require.NoError(t, err)
	// Only m1 has tmdb with non-empty value
	require.Len(t, enricher.calls, 1)
	assert.Equal(t, "42", enricher.calls[0].tmdbID)
	assert.Equal(t, "movie", enricher.calls[0].mediaType)
	// progress only fires for items that pass the skip (m1 only)
	assert.Equal(t, 1, progressCalls)
}

func TestRefreshAllTranslations_TVMediaType(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "s1", Type: library.Series, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "100"}}}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "se1", Type: library.Season, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "101"}}}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "e1", Type: library.Episode, Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "102"}}}))

	err := cmdSvc.RefreshAllTranslations(ctx, nil)
	require.NoError(t, err)
	require.Len(t, enricher.calls, 3)
	for _, c := range enricher.calls {
		assert.Equal(t, "tv", c.mediaType)
	}
}

// --- GetMediaItem ---

func TestGetMediaItem_NotConfigured(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	_, err := cmdSvc.GetMediaItem(context.Background(), "item-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "media items repo not configured")
}

func TestGetMediaItem_Success(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Matrix"}))

	got, err := cmdSvc.GetMediaItem(ctx, "item-1")
	require.NoError(t, err)
	assert.Equal(t, "Matrix", got.Title)
}

func TestGetMediaItem_NotFound(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	_, err := cmdSvc.GetMediaItem(context.Background(), "missing")
	require.Error(t, err)
}

// --- ResetItemMetadata ---

func TestResetItemMetadata_NoRepo(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	err := cmdSvc.ResetItemMetadata(context.Background(), "item-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "media item repository not configured")
}

func TestResetItemMetadata_ItemNotFound(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	err := cmdSvc.ResetItemMetadata(context.Background(), "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestResetItemMetadata_MovieClearsFields(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo,
		artworkRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	personRepo := libfakes.NewFakeMediaPersonRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)
	appmetadata.WithMediaPersons(personRepo)(cmdSvc)

	ctx := context.Background()
	year := 2023
	item := &library.MediaItem{
		ID:             "m1",
		Type:           library.MovieItem,
		Title:          "Old Title",
		File:           &library.MediaFile{Path: "/media/Inception (2010).mkv"},
		Overview:       "some overview",
		Tagline:        "tag",
		OriginalTitle:  "orig",
		Catalog:        &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "27205"}, Year: &year},
		Classification: &library.MediaClassification{OfficialRating: "PG-13"},
	}
	require.NoError(t, mediaRepo.Create(ctx, item))
	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "m1", FieldName: "title", CustomValue: "x",
	}))
	require.NoError(t, personRepo.Create(ctx, &library.MediaPerson{MediaItemID: "m1", PersonID: "p1"}))

	err := cmdSvc.ResetItemMetadata(ctx, "m1")
	require.NoError(t, err)

	reset, err := mediaRepo.GetByID(ctx, shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, "Inception (2010)", reset.Title)
	assert.Empty(t, reset.Overview)
	assert.Empty(t, reset.Tagline)
	assert.Empty(t, reset.OriginalTitle)
	assert.Nil(t, reset.Catalog)
	assert.Nil(t, reset.Classification)

	// overrides removed
	ovs, _ := overrideRepo.GetByMediaItem(ctx, "m1")
	assert.Empty(t, ovs)

	// persons removed
	persons, _ := personRepo.GetByMediaItemID(ctx, shared.MediaItemID("m1"))
	assert.Empty(t, persons)
}

// Series with no FilePath falls back to deriveTitleFromChildren: the top-level
// directory extracted from the first episode's file path.
func TestResetItemMetadata_Series_TitleFromChildren(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo,
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	// Series: no FilePath
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "s-1", Type: library.Series, Title: "Old Series Title",
	}))
	// Season (parent = series)
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "se-1", Type: library.Season, ParentID: "s-1", Title: "S01",
	}))
	// Episode with FilePath → extract directory name = top-level dir above season dir
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "ep-1", Type: library.Episode, ParentID: "se-1",
		File: &library.MediaFile{Path: "/media/Wednesday Addams/S01/E01.mkv"},
	}))

	err := cmdSvc.ResetItemMetadata(ctx, "s-1")
	require.NoError(t, err)

	reset, err := mediaRepo.GetByID(ctx, shared.MediaItemID("s-1"))
	require.NoError(t, err)
	assert.Equal(t, "Wednesday Addams", reset.Title)
}

func TestResetItemMetadata_SeriesCascadesToChildren(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo,
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "s1", Type: library.Series, Title: "Old Series",
		File:     &library.MediaFile{Path: "/media/Breaking Bad"},
		Overview: "o",
		Catalog:  &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "1"}},
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "se1", Type: library.Season, ParentID: "s1", Title: "Season 1",
		File:     &library.MediaFile{Path: "/media/Breaking Bad/S01"},
		Overview: "s-overview",
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "e1", Type: library.Episode, ParentID: "se1", Title: "Ep",
		File:     &library.MediaFile{Path: "/media/Breaking Bad/S01/ep.mkv"},
		Overview: "e-overview",
	}))

	err := cmdSvc.ResetItemMetadata(ctx, "s1")
	require.NoError(t, err)

	series, _ := mediaRepo.GetByID(ctx, shared.MediaItemID("s1"))
	assert.Empty(t, series.Overview)
	assert.Nil(t, series.Catalog)

	season, _ := mediaRepo.GetByID(ctx, shared.MediaItemID("se1"))
	assert.Empty(t, season.Overview)

	ep, _ := mediaRepo.GetByID(ctx, shared.MediaItemID("e1"))
	assert.Empty(t, ep.Overview)
	assert.Equal(t, "ep", ep.Title)
}

// --- WaitWorkers ---

func TestWaitWorkers_NoWorkersReturnsImmediately(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	done := make(chan struct{})
	go func() {
		cmdSvc.WaitWorkers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitWorkers did not return immediately with no workers started")
	}
}

// --- ResearchMetadata ---

// fakeForceEnricher implements ForceSingleItemEnricher and writes a known
// post-enrichment state so the test can verify the post-enrich GetByID path.
type fakeForceEnricher struct {
	mediaRepo *libfakes.FakeMediaItemRepository
	newTitle  string
	overview  string
	tmdbID    string
}

func (f *fakeForceEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (f *fakeForceEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}
func (f *fakeForceEnricher) ForceAutoEnrichItem(ctx context.Context, item *library.MediaItem, _ *slog.Logger) {
	stored, err := f.mediaRepo.GetByID(ctx, item.ID)
	if err != nil {
		return
	}
	stored.Title = f.newTitle
	stored.Overview = f.overview
	stored.Catalog = &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": f.tmdbID}}
	_ = f.mediaRepo.Update(ctx, stored)
}

func TestResearchMetadata_NoRepo(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	_, err := cmdSvc.ResearchMetadata(context.Background(), "any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "media item repository not configured")
}

func TestResearchMetadata_ItemNotFound(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	_, err := cmdSvc.ResearchMetadata(context.Background(), "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestResearchMetadata_ResetsAndReEnrichesFromScanTitle(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo,
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeForceEnricher{mediaRepo: mediaRepo, newTitle: "Billions", overview: "Wall Street drama", tmdbID: "1419"}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	year := 2018
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID:            "m1",
		Type:          library.Series,
		Title:         "The Foods that Make Billions",
		ScanTitle:     "Billions",
		OriginalTitle: "Billions (orig)",
		Overview:      "wrong overview",
		Tagline:       "wrong tagline",
		Catalog: &library.MediaCatalog{
			ProviderIDs: map[string]string{"tmdb": "wrong-id", "imdb": "tt0000000", "thetvdb": "999", "wikidata": "Q123"},
			Year:        &year,
		},
	}))
	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "m1", FieldName: "title", CustomValue: "stale",
	}))

	res, err := cmdSvc.ResearchMetadata(ctx, "m1")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Matched, "title moved + overview present → matched true")
	assert.Equal(t, "Billions", res.NewTitle)

	// overrides removed
	ovs, _ := overrideRepo.GetByMediaItem(ctx, "m1")
	assert.Empty(t, ovs)

	// final state in repo
	final, err := mediaRepo.GetByID(ctx, shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, "Billions", final.Title)
	assert.Equal(t, "Wall Street drama", final.Overview)
	require.NotNil(t, final.Catalog)
	assert.Equal(t, "1419", final.Catalog.ProviderIDs["tmdb"])
	// non-targeted provider IDs (wikidata) were also dropped because the
	// enricher overwrote the catalog wholesale — that's fine; what matters
	// here is that the bad tmdb/imdb/thetvdb didn't survive.
}

func TestResearchMetadata_NoEnricherStillResetsState(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo,
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	// Enricher that doesn't satisfy ForceSingleItemEnricher / SingleItemEnricher
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	year := 2010
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID:            "m1",
		Type:          library.MovieItem,
		Title:         "Wrong Title",
		ScanTitle:     "Inception",
		OriginalTitle: "x",
		Overview:      "y",
		Tagline:       "z",
		Catalog:       &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "00000"}, Year: &year},
	}))

	res, err := cmdSvc.ResearchMetadata(ctx, "m1")
	require.NoError(t, err)
	require.NotNil(t, res)
	// fakeTranslationsEnricher implements SingleItemEnricher (AutoEnrichItem)
	// but doesn't write anything → overview remains empty → matched=false.
	assert.False(t, res.Matched)
	assert.Equal(t, "Inception", res.NewTitle)

	final, err := mediaRepo.GetByID(ctx, shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, "Inception", final.Title)
	assert.Empty(t, final.Overview)
	assert.Empty(t, final.Tagline)
	assert.Empty(t, final.OriginalTitle)
	require.NotNil(t, final.Catalog)
	assert.Empty(t, final.Catalog.ProviderIDs["tmdb"])
	assert.Nil(t, final.Catalog.Year)
}

// --- StartEnrichmentWorker ---

func TestStartEnrichmentWorker_WaitWorkersCompletesAfterCancel(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	cmdSvc.StartEnrichmentWorker(ctx)
	cancel()

	done := make(chan struct{})
	go func() {
		cmdSvc.WaitWorkers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("workers did not exit after ctx cancel")
	}
}
