package metadata_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSlogBuffer swaps default slog to buffer at Warn level. Returns read
// func + restore. Kills NEGATION mutations on `if err := Publish; err != nil`
// branches: original logs on publish error; mutated does not.
func captureSlogBuffer(t *testing.T) (func() string, func()) {
	t.Helper()
	var mu sync.Mutex
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	return func() string {
			mu.Lock()
			defer mu.Unlock()
			return buf.String()
		}, func() {
			slog.SetDefault(prev)
		}
}

// countingMediaRepo wraps FakeMediaItemStore and counts List invocations.
// Kills BOUNDARY/NEGATION mutations on `if len(items) < pageSize { break }`:
// original makes 1 List call with small dataset; mutated makes 2 (second
// returns empty, terminated by `len(items) == 0` guard).
type countingMediaRepo struct {
	*libfakes.FakeMediaItemStore
	listCalls atomic.Int32
}

func (r *countingMediaRepo) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	r.listCalls.Add(1)
	return r.FakeMediaItemStore.List(ctx, f)
}

func newCountingRepo() *countingMediaRepo {
	return &countingMediaRepo{FakeMediaItemStore: libfakes.NewFakeMediaItemStore()}
}

// --- slog assertions on publish-error paths (kills NEGATION at L80, L138, L166, L205, L224, L370) ---

func TestRefreshMetadata_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	_, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tmdb-1", "tmdb", "movie")
	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "publish event")
	assert.Contains(t, out, "metadata.refreshed")
}

func TestRefreshItemMetadata_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	require.NoError(t, cmdSvc.RefreshItemMetadata(ctx, "item-1", "tmdb-1", "movie"))
	out := read()
	assert.Contains(t, out, "publish event")
}

func TestRefreshItemMetadataByIMDB_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("imdbapi")
	provider.ByProviderID["tt1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	require.NoError(t, cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt1", "movie"))
	out := read()
	assert.Contains(t, out, "publish event")
}

func TestSetOverride_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	require.NoError(t, cmdSvc.SetOverride(context.Background(), &metadata.MetadataOverride{
		ID: "o1", MediaItemID: "i1", FieldName: "title", CustomValue: "X",
	}))
	assert.Contains(t, read(), "publish event")
}

func TestResetOverrides_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	require.NoError(t, cmdSvc.ResetOverrides(context.Background(), "item-1"))
	assert.Contains(t, read(), "publish event")
}

func TestDownloadArtwork_EventPublishError_LogsWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	url := "http://example.com/art.jpg"
	record := &metadata.ArtworkRecord{ID: "a1", MediaItemID: "i1", Type: metadata.ArtworkTypePoster, SourceURL: &url}
	require.NoError(t, cmdSvc.DownloadArtwork(context.Background(), record, "/tmp/art.jpg"))
	assert.Contains(t, read(), "publish event")
}

// --- List-call counting (kills BOUNDARY/NEGATION at L403, L425, L526) ---

func TestAutoRefreshAllItems_ListCallCount(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-2", Title: "B"}))

	require.NoError(t, cmdSvc.AutoRefreshAllItems(ctx, ""))
	// With 2 items < pageSize (500), original breaks after 1 List call.
	// Mutation (<=, >=) continues → 2nd List call (empty) breaks via len==0.
	assert.Equal(t, int32(1), repo.listCalls.Load(), "expected exactly 1 List call when items < pageSize")
	assert.Len(t, enricher.enrichedItems, 2)
}

func TestReEnrichWithoutOverrides_ListCallCount(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(ctx, enricher)
	assert.Equal(t, 1, refreshed)
	assert.Equal(t, 0, skipped)
	assert.Equal(t, int32(1), repo.listCalls.Load(), "expected exactly 1 List call when items < pageSize")
}

func TestStartEnrichmentWorker_BulkListCallCount(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	repo := newCountingRepo()
	enricher := &fakeBulkSingleEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx, cancel := context.WithCancel(context.Background())

	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))

	cmdSvc.StartEnrichmentWorker(ctx)
	cancel()
	cmdSvc.WaitWorkers()
	// Bulk worker lists pages until <pageSize. With 1 item, original = 1 call.
	// Mutation = 2 calls (2nd empty).
	assert.Equal(t, int32(1), repo.listCalls.Load())
}

// --- BOUNDARY kills for `len(items) < pageSize` (L403, L425, L526) ---
// With exactly pageSize (500) items: original does 2 List calls (2nd empty).
// BOUNDARY mutation (<=) breaks after first page → only 1 List call.

const testPageSize = 500

func seedNItems(t *testing.T, repo *countingMediaRepo, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		id := "seed-" + strings.Repeat("0", 0) + itoaFast(i)
		require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: shared.MediaItemID(id), Title: id}))
	}
}

func itoaFast(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func TestAutoRefreshAllItems_ExactPageSize_TwoListCalls(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)

	seedNItems(t, repo, testPageSize)
	require.NoError(t, cmdSvc.AutoRefreshAllItems(context.Background(), ""))
	// Original: 1st call returns 500, !<500 → 2nd call returns 0 → break. 2 calls.
	// BOUNDARY (<=): 500<=500 → break after 1st. 1 call.
	assert.Equal(t, int32(2), repo.listCalls.Load())
}

func TestReEnrichWithoutOverrides_ExactPageSize_TwoListCalls(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)

	seedNItems(t, repo, testPageSize)
	cmdSvc.ReEnrichWithoutOverrides(context.Background(), enricher)
	assert.Equal(t, int32(2), repo.listCalls.Load())
}

func TestStartEnrichmentWorker_BulkExactPageSize_TwoListCalls(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	repo := newCountingRepo()
	enricher := &fakeBulkSingleEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx, cancel := context.WithCancel(context.Background())

	seedNItems(t, repo, testPageSize)

	cmdSvc.StartEnrichmentWorker(ctx)
	// Wait for bulk worker to finish both pages, then cancel to stop daily ticker.
	// Poll listCalls up to timeout.
	deadline := time.After(2 * time.Second)
	for repo.listCalls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("bulk worker did not reach 2 List calls; got %d", repo.listCalls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	cmdSvc.WaitWorkers()
	assert.GreaterOrEqual(t, repo.listCalls.Load(), int32(2))
}

// --- L431 NEGATION: `if bulkOK && s.mediaItems != nil` (dailyReEnrich goroutine) ---
// Mutation flips condition → goroutine doesn't start when configured, starts when not.
// When not configured (mediaItems=nil), goroutine would nil-panic in ReEnrichWithoutOverrides
// on ticker fire; but ticker is 24h so never fires in tests. Instead assert via
// WaitWorkers completion in under a second - if mutation starts goroutine without
// mediaItems, it still sits on ticker waiting for ctx.Done → same behavior.
// This mutation is behaviorally equivalent and hard to kill without refactor.

// --- ReEnrichWithoutOverrides: len(toEnrich) > 0 boundary (L517) ---
// Mutation BOUNDARY `>=` would call AutoEnrichAll with empty slice when all overridden.
// Original: skips AutoEnrichAll when toEnrich is empty.

type recordingBulkEnricher struct {
	mu         sync.Mutex
	callCount  int
	totalItems int
}

func (e *recordingBulkEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (e *recordingBulkEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}
func (e *recordingBulkEnricher) AutoEnrichAll(_ context.Context, items []*library.MediaItem, _ *slog.Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callCount++
	e.totalItems += len(items)
}

func TestReEnrichWithoutOverrides_AllOverridden_DoesNotCallEnricher(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)
	mediaRepo := libfakes.NewFakeMediaItemStore()
	enricher := &recordingBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo, mediaRepo)(cmdSvc)
	ctx := context.Background()

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "X",
	}))

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(ctx, enricher)
	assert.Equal(t, 0, refreshed)
	assert.Equal(t, 1, skipped)
	// BOUNDARY mutation (>= 0) would invoke AutoEnrichAll once with empty slice.
	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.Equal(t, 0, enricher.callCount, "AutoEnrichAll must not be called when no items to enrich")
}

// --- happy paths assert no warn log (kills inverse mutation coverage) ---

func TestSetOverride_NoPublishError_NoWarn(t *testing.T) {
	read, restore := captureSlogBuffer(t)
	defer restore()

	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	require.NoError(t, cmdSvc.SetOverride(context.Background(), &metadata.MetadataOverride{
		ID: "o1", MediaItemID: "i1", FieldName: "title", CustomValue: "X",
	}))
	assert.False(t, strings.Contains(read(), "publish event"), "no warn when publish succeeds")
}

// The refresh-all sweep persists its offset after every page and clears it on
// completion: a re-trigger after a mid-walk restart resumes past the pages
// already enriched instead of re-walking the whole catalogue.
func TestAutoRefreshAllItems_ResumesFromPersistedCursor(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
		appmetadata.WithServerConfig(cfg),
	)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))

	// A cursor left by an interrupted sweep at offset 500 (page size): the
	// resumed walk starts THERE - the (only) item at offset 0 is skipped.
	require.NoError(t, cfg.Set(ctx, "metadata.refresh_all.cursor.", `{"library_id":"","offset":500}`))
	require.NoError(t, cmdSvc.AutoRefreshAllItems(ctx, ""))
	assert.Empty(t, enricher.enrichedItems, "the resumed sweep must not re-enrich pages before the cursor")

	// Completion clears the cursor: the next trigger walks from zero again.
	entry, err := cfg.Get(ctx, "metadata.refresh_all.cursor.")
	if err == nil && entry != nil {
		assert.Empty(t, entry.Value, "the cursor must be cleared after a completed sweep")
	}
	require.NoError(t, cmdSvc.AutoRefreshAllItems(ctx, ""))
	assert.Len(t, enricher.enrichedItems, 1, "after the cleared cursor the full walk enriches from zero")
}

// A cursor scoped to another library does not hijack the walk.
func TestAutoRefreshAllItems_IgnoresCursorOfOtherScope(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
		appmetadata.WithServerConfig(cfg),
	)
	repo := newCountingRepo()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, repo, repo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, cfg.Set(ctx, "metadata.refresh_all.cursor.lib-other", `{"library_id":"lib-other","offset":500}`))

	require.NoError(t, cmdSvc.AutoRefreshAllItems(ctx, ""))
	assert.Len(t, enricher.enrichedItems, 1, "a cursor for another scope must be ignored")
}
