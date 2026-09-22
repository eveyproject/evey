package metadata_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
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

// writeSolidPNG writes a tiny 32×32 magenta PNG to path. The blurhash
// encoder needs a real image; this is the smallest one that decodes cleanly
// via image/png without pulling in a fixture file.
func writeSolidPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	c := color.RGBA{R: 220, G: 50, B: 180, A: 255}
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, png.Encode(f, img))
}

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

// ForceAutoEnrichItem is a no-op: this fake exists to record translation
// calls and count non-force auto-enrich invocations. Some tests reuse it as
// the wired enricher for ResearchMetadata, which now always prefers the
// force variant (merged into SingleItemEnricher) - it must write nothing so
// those tests' "unmatched" assertions still hold.
func (f *fakeTranslationsEnricher) ForceAutoEnrichItem(_ context.Context, _ *library.MediaItem, _ *slog.Logger) {
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

// The sweep used to read the catalogue with a single unpaginated List. A zero
// Limit is not "everything" - postgres clamps it to DefaultListLimit (500) -
// so on any catalogue past 500 items the rest never had its translations
// refreshed, and the endpoint still logged "refresh all translations complete".
//
// This stayed invisible because the fake repository used to return the whole
// set for a zero Limit, unlike the database. The fake now clamps the same way,
// which is what lets this test fail against the old code.
func TestRefreshAllTranslations_PagesPastTheFirstFiveHundred(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	const catalogue = 640 // past the 500-row page, well within a real library
	for i := range catalogue {
		require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
			ID:      shared.MediaItemID(fmt.Sprintf("m%04d", i)),
			Type:    library.MovieItem,
			Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": fmt.Sprintf("%d", 1000+i)}},
		}))
	}

	var lastDone, lastTotal int
	err := cmdSvc.RefreshAllTranslations(ctx, func(done, total int) {
		lastDone, lastTotal = done, total
	})
	require.NoError(t, err)

	// Count AND identity: with the fake's ordering left to a map's iteration,
	// paging could return 640 calls while repeating some items and skipping
	// others. Asserting the set is what makes the count mean something.
	assert.Len(t, enricher.calls, catalogue, "every item must be refreshed, not just the first page")
	seenIDs := make(map[string]struct{}, len(enricher.calls))
	for _, c := range enricher.calls {
		seenIDs[c.tmdbID] = struct{}{}
	}
	assert.Len(t, seenIDs, catalogue, "each item must be refreshed exactly once - no repeats, no gaps")
	assert.Equal(t, catalogue, lastTotal, "the reported total must be the real catalogue size")
	assert.Equal(t, catalogue, lastDone, "progress must reach the end")
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
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Some Movie"}))

	got, err := cmdSvc.GetMediaItem(ctx, "item-1")
	require.NoError(t, err)
	assert.Equal(t, "Some Movie", got.Title)
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
		File:           &library.MediaFile{Path: "/media/Some Movie (2010).mkv"},
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
	assert.Equal(t, "Some Movie (2010)", reset.Title)
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
		File: &library.MediaFile{Path: "/media/Some Family Show/S01/E01.mkv"},
	}))

	err := cmdSvc.ResetItemMetadata(ctx, "s-1")
	require.NoError(t, err)

	reset, err := mediaRepo.GetByID(ctx, shared.MediaItemID("s-1"))
	require.NoError(t, err)
	assert.Equal(t, "Some Family Show", reset.Title)
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
		File:     &library.MediaFile{Path: "/media/Some Series"},
		Overview: "o",
		Catalog:  &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "1"}},
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "se1", Type: library.Season, ParentID: "s1", Title: "Season 1",
		File:     &library.MediaFile{Path: "/media/Some Series/S01"},
		Overview: "s-overview",
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "e1", Type: library.Episode, ParentID: "se1", Title: "Ep",
		File:     &library.MediaFile{Path: "/media/Some Series/S01/ep.mkv"},
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

// --- MatchOrphansInLibrary ---

// fakeOrphanBulkEnricher records every batch enrich call without doing the actual
// TMDB work. Satisfies BulkItemEnricher and the base ItemEnricher contract.
// finishedCh is closed exactly once, on the first AutoEnrichAll return - the
// drainer may call AutoEnrichAll multiple times so a per-call close would
// panic.
type fakeOrphanBulkEnricher struct {
	mu         sync.Mutex
	called     bool
	itemsSeen  []string
	releaseCh  chan struct{} // when non-nil, AutoEnrichAll blocks on it
	finishedCh chan struct{} // closed once on the first AutoEnrichAll return
	closeOnce  sync.Once
}

func (f *fakeOrphanBulkEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}

func (f *fakeOrphanBulkEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

func (f *fakeOrphanBulkEnricher) AutoEnrichAll(_ context.Context, items []*library.MediaItem, _ *slog.Logger) {
	f.mu.Lock()
	f.called = true
	for _, it := range items {
		f.itemsSeen = append(f.itemsSeen, string(it.ID))
	}
	rc := f.releaseCh
	fc := f.finishedCh
	f.mu.Unlock()
	if rc != nil {
		<-rc
	}
	if fc != nil {
		f.closeOnce.Do(func() { close(fc) })
	}
}

// fakeMatchingBulkEnricher simulates a working enricher: it writes a synthetic
// ProviderIDs map onto each item via the underlying repo so the drainer's
// stall detector sees progress and exits on an empty remainder.
type fakeMatchingBulkEnricher struct {
	mu        sync.Mutex
	repo      *libfakes.FakeMediaItemRepository
	itemsSeen []string
	doneCh    chan struct{}
	closeOnce sync.Once
}

func (f *fakeMatchingBulkEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}

func (f *fakeMatchingBulkEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

func (f *fakeMatchingBulkEnricher) AutoEnrichAll(ctx context.Context, items []*library.MediaItem, _ *slog.Logger) {
	for _, it := range items {
		f.mu.Lock()
		f.itemsSeen = append(f.itemsSeen, string(it.ID))
		f.mu.Unlock()
		stored, err := f.repo.GetByID(ctx, it.ID)
		if err != nil {
			continue
		}
		stored.Catalog = &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "matched"}}
		_ = f.repo.Update(ctx, stored)
	}
	f.closeOnce.Do(func() { close(f.doneCh) })
}

func TestMatchOrphansInLibrary_NoEnricher(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	_, err := cmdSvc.MatchOrphansInLibrary(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support bulk refresh")
}

func TestMatchOrphansInLibrary_SelectsOnlyOrphansInLibrary(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	// Enricher that "matches" each orphan by writing back ProviderIDs.
	// This is what makes the drainer's stall detector see progress and
	// eventually exit on an empty remainder.
	enricher := &fakeMatchingBulkEnricher{repo: mediaRepo, doneCh: make(chan struct{})}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	// orphan in target library
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan 1",
	}))
	// orphan with empty pid map in target library
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m2", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan 2",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{}},
	}))
	// matched item in target library - must be skipped
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m3", LibraryID: "lib-1", Type: library.MovieItem, Title: "Matched",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "42"}},
	}))
	// orphan in OTHER library - must be skipped
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m4", LibraryID: "lib-2", Type: library.MovieItem, Title: "Other lib orphan",
	}))

	total, err := cmdSvc.MatchOrphansInLibrary(ctx, "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 2, total, "should count only lib-1 orphans")

	// Drainer runs in a detached goroutine - wait for completion.
	select {
	case <-enricher.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("drainer never completed")
	}
	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2"}, enricher.itemsSeen)
	// The other-library orphan must be untouched.
	assert.NotContains(t, enricher.itemsSeen, "m4")
}

func TestMatchOrphansInLibrary_DrainCancelledByShutdown(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeOrphanBulkEnricher{finishedCh: make(chan struct{})}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	// Service ctx already cancelled: the detached drain must see it and stop
	// before any enrich work, AND WaitWorkers must await it (it is now
	// registered on workerWg, not a bare `go` on context.Background()).
	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	appmetadata.WithShutdownCtx(shutdownCtx)(cmdSvc)

	require.NoError(t, mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan",
	}))

	// The initial count runs on the request ctx (not cancelled) so total is real.
	total, err := cmdSvc.MatchOrphansInLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	done := make(chan struct{})
	go func() { cmdSvc.WaitWorkers(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitWorkers did not await the drain (not registered on workerWg?)")
	}

	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.False(t, enricher.called, "a cancelled service ctx must stop the drain before any enrich work")
}

func TestMatchOrphansInLibrary_StopsOnStall(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	// Enricher that does NOT write back ProviderIDs - orphan count stays
	// constant → drainer must detect the stall and exit.
	enricher := &fakeOrphanBulkEnricher{finishedCh: make(chan struct{})}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Stuck orphan",
	}))

	total, err := cmdSvc.MatchOrphansInLibrary(ctx, "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	// Wait for the first AutoEnrichAll call - drainer should then detect
	// the stall (Count unchanged) and exit instead of looping forever.
	select {
	case <-enricher.finishedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("drainer never invoked enricher")
	}

	// Sleep enough for a second drainer iteration if the stall guard misfires.
	time.Sleep(200 * time.Millisecond)
	enricher.mu.Lock()
	called := enricher.called
	itemsCount := len(enricher.itemsSeen)
	enricher.mu.Unlock()
	assert.True(t, called)
	assert.Equal(t, 1, itemsCount, "stall guard must stop after one iteration")
}

func TestMatchOrphansInLibrary_NoOrphansShortCircuits(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeOrphanBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem,
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "42"}},
	}))

	queued, err := cmdSvc.MatchOrphansInLibrary(ctx, "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, queued)
	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.False(t, enricher.called, "enricher must not be called when no orphans")
}

func TestMatchOrphansInLibrary_DetachesFromCallerContext(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	release := make(chan struct{})
	enricher := &fakeOrphanBulkEnricher{releaseCh: release, finishedCh: make(chan struct{})}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	require.NoError(t, mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan",
	}))

	callerCtx, cancel := context.WithCancel(context.Background())
	queued, err := cmdSvc.MatchOrphansInLibrary(callerCtx, "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, queued)

	cancel() // simulate HTTP client disconnect

	// The detached goroutine must still be alive - release it and confirm it
	// completes, proving cancellation of the caller ctx did not propagate.
	close(release)
	select {
	case <-enricher.finishedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("detached enricher was cancelled by caller ctx - regression")
	}
}

// --- ResearchMetadata ---

// fakeForceEnricher implements the merged SingleItemEnricher and writes a
// known post-enrichment state so the test can verify the post-enrich GetByID
// path. Only the force half is exercised (ResearchMetadata always prefers
// it); AutoEnrichItem is a no-op stub required to satisfy the interface.
type fakeForceEnricher struct {
	mediaRepo     *libfakes.FakeMediaItemRepository
	newTitle      string
	overview      string
	tmdbID        string
	clearedItemID string // records ClearTranslations(mediaItemID)
}

func (f *fakeForceEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (f *fakeForceEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}
func (f *fakeForceEnricher) ClearTranslations(_ context.Context, mediaItemID string) {
	f.clearedItemID = mediaItemID
}
func (f *fakeForceEnricher) AutoEnrichItem(_ context.Context, _ *library.MediaItem, _ *slog.Logger) {
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
	enricher := &fakeForceEnricher{mediaRepo: mediaRepo, newTitle: "Some Series", overview: "Wall Street drama", tmdbID: "1419"}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	year := 2018
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID:            "m1",
		Type:          library.Series,
		Title:         "The Foods that Make Some Series",
		ScanTitle:     "Some Series",
		OriginalTitle: "Some Series (orig)",
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
	assert.Equal(t, "Some Series", res.NewTitle)

	// overrides removed
	ovs, _ := overrideRepo.GetByMediaItem(ctx, "m1")
	assert.Empty(t, ovs)

	// stale locale translations from the bad match were cleared
	assert.Equal(t, "m1", enricher.clearedItemID, "ResearchMetadata must clear stale translations")

	// final state in repo
	final, err := mediaRepo.GetByID(ctx, shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, "Some Series", final.Title)
	assert.Equal(t, "Wall Street drama", final.Overview)
	require.NotNil(t, final.Catalog)
	assert.Equal(t, "1419", final.Catalog.ProviderIDs["tmdb"])
	// non-targeted provider IDs (wikidata) were also dropped because the
	// enricher overwrote the catalog wholesale - that's fine; what matters
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
		ScanTitle:     "Some Movie",
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
	assert.Equal(t, "Some Movie", res.NewTitle)

	final, err := mediaRepo.GetByID(ctx, shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, "Some Movie", final.Title)
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

// --- SetOverride: title-on-orphan propagation + auto-enrich trigger ---

func TestSetOverride_TitleOnOrphan_PropagatesAndTriggersEnrich(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem,
		Title: "Quand l'hôpital retient son souffle 720", // parser-polluted
	}))

	override, err := metadata.NewMetadataOverride("mo-1", "m1", "title",
		metadata.WithOverrideValue("Quand l'hôpital retient son souffle"))
	require.NoError(t, err)
	require.NoError(t, cmdSvc.SetOverride(ctx, override))

	// media_items.title should now hold the cleaned override value so the
	// background enricher's TMDB search uses the corrected query.
	updated, err := mediaRepo.GetByID(ctx, "m1")
	require.NoError(t, err)
	assert.Equal(t, "Quand l'hôpital retient son souffle", updated.Title)

	// Auto-enrich runs in a goroutine - give it a moment to fire.
	assert.Eventually(t, func() bool {
		return enricher.singleN.Load() == 1
	}, 2*time.Second, 20*time.Millisecond, "auto-enrich was not triggered")
}

func TestSetOverride_TitleOnMatched_DoesNotPropagate(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem,
		Title:   "Some Movie",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "603"}},
	}))

	override, err := metadata.NewMetadataOverride("mo-1", "m1", "title",
		metadata.WithOverrideValue("Custom display title"))
	require.NoError(t, err)
	require.NoError(t, cmdSvc.SetOverride(ctx, override))

	// Matched item: title in DB stays, no re-enrich (override is purely
	// presentation for the display layer in GetMediaItem).
	updated, err := mediaRepo.GetByID(ctx, "m1")
	require.NoError(t, err)
	assert.Equal(t, "Some Movie", updated.Title, "matched item title must NOT be overwritten")

	time.Sleep(150 * time.Millisecond) // give any errant goroutine a chance to fire
	assert.Equal(t, int32(0), enricher.singleN.Load(), "auto-enrich must NOT fire for matched items")
}

func TestSetOverride_NonTitleField_NoPropagation(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan",
	}))

	override, err := metadata.NewMetadataOverride("mo-1", "m1", "overview",
		metadata.WithOverrideValue("New overview"))
	require.NoError(t, err)
	require.NoError(t, cmdSvc.SetOverride(ctx, override))

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), enricher.singleN.Load(), "non-title fields don't drive TMDB matching, must NOT enrich")
}

func TestSetOverride_TitleAlreadyAligned_NoEnrich(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeTranslationsEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", LibraryID: "lib-1", Type: library.MovieItem,
		Title: "Quand l'hôpital retient son souffle",
	}))

	override, err := metadata.NewMetadataOverride("mo-1", "m1", "title",
		metadata.WithOverrideValue("Quand l'hôpital retient son souffle"))
	require.NoError(t, err)
	require.NoError(t, cmdSvc.SetOverride(ctx, override))

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), enricher.singleN.Load(), "no-op title override must NOT re-enrich")
}

// --- BackfillBlurhashes ---

func TestBackfillBlurhashes_EmptyRepo(t *testing.T) {
	cmdSvc := newCmdSvcNoEnricher()
	succ, fail := cmdSvc.BackfillBlurhashes(context.Background())
	assert.Equal(t, 0, succ)
	assert.Equal(t, 0, fail)
}

func TestBackfillBlurhashes_DecodesAndPersists(t *testing.T) {
	// Build a fake repo with a real PNG on disk so computeBlurhash succeeds.
	artRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)

	dir := t.TempDir()
	pngPath := dir + "/poster.png"
	writeSolidPNG(t, pngPath)

	id := "art-1"
	lp := pngPath
	require.NoError(t, artRepo.Create(context.Background(), &metadata.ArtworkRecord{
		ID:          id,
		MediaItemID: "m1",
		Type:        "poster",
		Provider:    "tmdb",
		LocalPath:   &lp,
	}))

	succ, fail := cmdSvc.BackfillBlurhashes(context.Background())
	assert.Equal(t, 1, succ)
	assert.Equal(t, 0, fail)

	got, err := artRepo.GetByMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Blurhash)
	assert.NotEmpty(t, *got[0].Blurhash)
}

func TestBackfillBlurhashes_SkipsAlreadyHashed(t *testing.T) {
	artRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)

	already := "L9Pj0^IU00%M~qWBM{j["
	lp := "/some/path.jpg"
	require.NoError(t, artRepo.Create(context.Background(), &metadata.ArtworkRecord{
		ID:          "art-already",
		MediaItemID: "m1",
		Type:        "poster",
		Provider:    "tmdb",
		LocalPath:   &lp,
		Blurhash:    &already,
	}))

	succ, fail := cmdSvc.BackfillBlurhashes(context.Background())
	// already-hashed row is filtered by the repo query → not even processed.
	assert.Equal(t, 0, succ)
	assert.Equal(t, 0, fail)
}

func TestBackfillBlurhashes_CountsDecodeFailures(t *testing.T) {
	artRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)

	// Point LocalPath at a non-existent file so computeBlurhash errors.
	lp := t.TempDir() + "/missing.png"
	require.NoError(t, artRepo.Create(context.Background(), &metadata.ArtworkRecord{
		ID:          "art-broken",
		MediaItemID: "m2",
		Type:        "poster",
		Provider:    "tmdb",
		LocalPath:   &lp,
	}))

	succ, fail := cmdSvc.BackfillBlurhashes(context.Background())
	assert.Equal(t, 0, succ)
	assert.Equal(t, 1, fail)
}

// --- artwork content-addressing (dedup) regression ---

// writingDownloader writes a real file so the content-addressed dedup + the
// skip-if-exists path can be exercised (the standard fake records only a map).
type writingDownloader struct{ calls atomic.Int32 }

func (d *writingDownloader) Download(_ context.Context, url, destPath string) error {
	d.calls.Add(1)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, []byte("img:"+url), 0o644)
}

// TestArtworkAutoDownloadHook_DownloadsWhenEnabled exercises the wiring finished
// in wire.go: the enricher fires ArtworkAutoDownloadHook after saving a record,
// and with the admin toggle on it must fetch the image to disk and persist the
// local path. Before the wiring the hook existed but was never invoked, so the
// admin toggle did nothing.
func TestArtworkAutoDownloadHook_DownloadsWhenEnabled(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	dl := &writingDownloader{}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(), artworkRepo, dl,
		sharedfakes.NewFakeEventBus(), nil,
	)
	dataDir := t.TempDir()
	appmetadata.WithAutoDownload(dataDir, true)(cmdSvc)

	ctx := context.Background()
	rec, err := metadata.NewArtworkRecord("art-auto", shared.MediaItemID("mi-a"), metadata.ArtworkTypePoster, "tmdb",
		metadata.WithArtworkSource("https://img.test/poster.jpg"))
	require.NoError(t, err)
	require.NoError(t, artworkRepo.Create(ctx, rec))

	cmdSvc.ArtworkAutoDownloadHook()(ctx, rec)

	assert.Equal(t, int32(1), dl.calls.Load(), "auto-download must fetch the image once")
	stored, err := artworkRepo.GetByMediaItem(ctx, "mi-a")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.NotNil(t, stored[0].LocalPath, "local path must be persisted after auto-download")
	fi, err := os.Stat(*stored[0].LocalPath)
	require.NoError(t, err)
	assert.Positive(t, fi.Size(), "downloaded file must be non-empty")
}

// TestArtworkAutoDownloadHook_NoopWhenDisabled: with the toggle off the hook must
// be a cheap no-op - the enricher fires it for every saved record regardless.
func TestArtworkAutoDownloadHook_NoopWhenDisabled(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	dl := &writingDownloader{}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(), artworkRepo, dl,
		sharedfakes.NewFakeEventBus(), nil,
	)
	appmetadata.WithAutoDownload(t.TempDir(), false)(cmdSvc)

	ctx := context.Background()
	rec, err := metadata.NewArtworkRecord("art-off", shared.MediaItemID("mi-off"), metadata.ArtworkTypePoster, "tmdb",
		metadata.WithArtworkSource("https://img.test/poster.jpg"))
	require.NoError(t, err)
	require.NoError(t, artworkRepo.Create(ctx, rec))

	cmdSvc.ArtworkAutoDownloadHook()(ctx, rec)

	assert.Equal(t, int32(0), dl.calls.Load(), "disabled toggle must not download")
	stored, err := artworkRepo.GetByMediaItem(ctx, "mi-off")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Nil(t, stored[0].LocalPath, "no local path when auto-download is off")
}

// TestArtwork_ContentAddressed_NoDuplicateFiles is the regression guard for the
// 250 GB duplication bug: two artwork records for the SAME source URL (as
// re-enrichment mints fresh record UUIDs every pass) must resolve to ONE
// on-disk file, not one-per-record. Before content-addressing, the filename was
// the record UUID, so each pass re-downloaded the same image to a new path and
// the old files leaked.
func TestArtwork_ContentAddressed_NoDuplicateFiles(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	dl := &writingDownloader{}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artworkRepo,
		dl,
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	dataDir := t.TempDir()
	appmetadata.WithAutoDownload(dataDir, true)(cmdSvc)

	ctx := context.Background()
	src := "https://img.test/poster.jpg"
	// Two distinct records (distinct UUIDs) for the SAME source image + media.
	for _, id := range []string{"art-1", "art-2"} {
		rec, err := metadata.NewArtworkRecord(id, shared.MediaItemID("mi-x"), metadata.ArtworkTypePoster, "tmdb",
			metadata.WithArtworkSource(src))
		require.NoError(t, err)
		require.NoError(t, artworkRepo.Create(ctx, rec))
	}

	succeeded, failed := cmdSvc.RetryMissingArtworks(ctx)
	assert.Equal(t, 2, succeeded)
	assert.Equal(t, 0, failed)

	// Both records share ONE content-addressed file.
	dir := filepath.Join(dataDir, "artworks", "mi-x")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "two records for the same source URL must dedup to one file")
}

// TestArtwork_ZeroByteFile_Refetched guards the data-loss path the review
// found: a 0-byte file left at the content-addressed path by a killed
// mid-download must be RE-fetched, not reconciled as valid (which would strand
// the record forever - non-null local_path excludes it from every retry scan).
func TestArtwork_ZeroByteFile_Refetched(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	dl := &writingDownloader{}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artworkRepo,
		dl,
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	dataDir := t.TempDir()
	appmetadata.WithAutoDownload(dataDir, true)(cmdSvc)

	ctx := context.Background()
	src := "https://img.test/poster.jpg"
	rec, err := metadata.NewArtworkRecord("art-z", shared.MediaItemID("mi-z"), metadata.ArtworkTypePoster, "tmdb",
		metadata.WithArtworkSource(src))
	require.NoError(t, err)
	require.NoError(t, artworkRepo.Create(ctx, rec))

	// Pre-plant a 0-byte file at the content-addressed path (sha256(src)[:16].jpg),
	// the exact state a SIGKILL between os.Create and io.Copy leaves behind.
	sum := sha256.Sum256([]byte(src))
	zeroPath := filepath.Join(dataDir, "artworks", "mi-z", hex.EncodeToString(sum[:16])+".jpg")
	require.NoError(t, os.MkdirAll(filepath.Dir(zeroPath), 0o755))
	require.NoError(t, os.WriteFile(zeroPath, []byte{}, 0o644))

	cmdSvc.RetryMissingArtworks(ctx)

	// The 0-byte file must have been re-downloaded (writingDownloader writes real
	// bytes), not skipped as "present".
	assert.Positive(t, dl.calls.Load(), "0-byte file must trigger a re-download, not be reconciled")
	fi, statErr := os.Stat(zeroPath)
	require.NoError(t, statErr)
	assert.Positive(t, fi.Size(), "file must be non-empty after re-fetch")
}

// --- runBulkDownload cancellation (regression) ---

// TestStartBulkDownload_CanceledServerCtx_NoDeadlock proves the ctx-cancel
// branch of runBulkDownload drains and finishes instead of deadlocking. The
// old `break` inside `select` only exited the select, then fell through to
// spawn a goroutine that never acquired a sem slot; its deferred `<-sem` stole
// a running worker's token and wg.Wait() hung forever (job stuck "in progress",
// goroutine leaked on every shutdown that interrupted a bulk download).
func TestStartBulkDownload_CanceledServerCtx_NoDeadlock(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artworkRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	appmetadata.WithAutoDownload(t.TempDir(), true)(cmdSvc)

	ctx := context.Background()
	for _, id := range []string{"art-0", "art-1", "art-2", "art-3", "art-4"} {
		rec, err := metadata.NewArtworkRecord(id, shared.MediaItemID("mi-"+id), metadata.ArtworkTypePoster, "tmdb",
			metadata.WithArtworkSource("https://example.test/a.jpg"))
		require.NoError(t, err)
		require.NoError(t, artworkRepo.Create(ctx, rec))
	}

	// Server ctx already cancelled: runBulkDownload must hit ctx.Done on the
	// first iteration and return cleanly, never blocking on wg.Wait().
	serverCtx, cancel := context.WithCancel(context.Background())
	cancel()

	jobID, total, err := cmdSvc.StartBulkDownload(ctx, serverCtx)
	require.NoError(t, err)
	require.Equal(t, 5, total)

	require.Eventually(t, func() bool {
		job := cmdSvc.GetDownloadJob(jobID)
		return job != nil && job.Done.Load()
	}, 2*time.Second, 10*time.Millisecond, "bulk download must finish on cancelled ctx, not deadlock")
}

// A reset that half-completes must SAY so. Every one of these four writes used
// to be logged and swallowed, and ResetItemMetadata returned nil regardless -
// so an admin whose overrides survived the "reset" was told it worked, and the
// next enrichment quietly re-applied exactly what they asked to remove.
func TestResetItemMetadata_ReportsFailureInsteadOfClaimingSuccess(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	item := &library.MediaItem{
		ID: "m1", Type: library.MovieItem, Title: "Old Title",
		File:     &library.MediaFile{Path: "/media/Some Movie (2010).mkv"},
		Overview: "some overview",
	}
	require.NoError(t, mediaRepo.Create(ctx, item))

	// The final write fails. Everything before it succeeded, so this is exactly
	// the half-state the old code reported as success.
	mediaRepo.ErrUpdate = errors.New("connection reset by peer")

	err := cmdSvc.ResetItemMetadata(ctx, "m1")
	require.Error(t, err, "a failed write must surface, not be logged and swallowed")
	assert.Contains(t, err.Error(), "connection reset by peer")
}

// Overrides are dropped BEFORE the item's fields are cleared, so a failure
// between the two fails safe: fields still populated + overrides gone
// re-converges on the reset state at the next enrichment, whereas the reverse
// order silently reinstates the removed overrides.
func TestResetItemMetadata_DropsOverridesBeforeClearingFields(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	cmdSvc := appmetadata.NewCommandService(
		overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeTranslationsEnricher{}, mediaRepo)(cmdSvc)

	ctx := context.Background()
	item := &library.MediaItem{
		ID: "m1", Type: library.MovieItem, Title: "Old Title",
		File:     &library.MediaFile{Path: "/media/Some Movie (2010).mkv"},
		Overview: "some overview",
	}
	require.NoError(t, mediaRepo.Create(ctx, item))
	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "m1", FieldName: "title", CustomValue: "x",
	}))
	mediaRepo.ErrUpdate = errors.New("db down")

	require.Error(t, cmdSvc.ResetItemMetadata(ctx, "m1"))

	// The override must be gone even though the run aborted: that is the safe
	// half-state.
	remaining, err := overrideRepo.GetByMediaItem(ctx, "m1")
	require.NoError(t, err)
	assert.Empty(t, remaining, "overrides must be dropped before the item write can fail")
}
