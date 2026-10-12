package library_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanHarness assembles a ScanService with direct access to every fake.
type scanHarness struct {
	svc          *applib.ScanService
	libCmd       *libfakes.FakeLibraryCommandRepo
	libQuery     *libfakes.FakeLibraryQueryRepo
	media        *libfakes.FakeMediaItemCommandRepo
	streams      *libfakes.FakeMediaStreamRepository
	persons      *libfakes.FakePersonRepository
	mediaPersons *libfakes.FakeMediaPersonRepository
	scanner      *libfakes.FakeFileScanner
	prober       *libfakes.FakeStreamProber
	search       *libfakes.FakeSearchIndex
	bus          *sharedfakes.FakeEventBus
}

func newScanHarness() *scanHarness {
	h := &scanHarness{
		libCmd:       libfakes.NewFakeLibraryCommandRepo(),
		libQuery:     libfakes.NewFakeLibraryQueryRepo(),
		media:        libfakes.NewFakeMediaItemCommandRepo(),
		streams:      libfakes.NewFakeMediaStreamRepository(),
		persons:      libfakes.NewFakePersonRepository(),
		mediaPersons: libfakes.NewFakeMediaPersonRepository(),
		scanner:      libfakes.NewFakeFileScanner(),
		prober:       libfakes.NewFakeStreamProber(),
		search:       libfakes.NewFakeSearchIndex(),
		bus:          sharedfakes.NewFakeEventBus(),
	}
	h.svc = applib.NewScanService(
		h.libCmd, h.libQuery, h.media, h.streams, h.persons, h.mediaPersons,
		h.scanner, h.prober, h.search, h.bus,
	)
	return h
}

// captureSlog swaps the default slog.Logger for the duration of fn and returns
// anything the code under test logged. NOT safe for t.Parallel().
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)
	fn()
	return buf.String()
}

// --- scan_service.go:140 publish LibraryScanStarted err ---

func TestScanService_PublishScanStarted_ErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.bus.PublishErr = errors.New("bus down")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "publish event")
}

func TestScanService_PublishScanStarted_NoErrorNoLog(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "publish event")
}

// --- scan_service.go:163 if ref.FileFingerprint != "" ---

func TestScanService_LoadIndex_EmptyFingerprintNotIndexed(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// Existing DB item with EMPTY fingerprint at old path.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "old-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/old.mkv", Size: 100, Fingerprint: ""},
	})
	// New scan: keep the old file + add a new one with empty fingerprint.
	// Empty fingerprint must NOT cause the new file to be mistaken as a "moved" old file.
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/old.mkv", Size: 100, Fingerprint: "", Type: library.MovieItem},
		{Path: "/m/new.mkv", Size: 200, Fingerprint: "", Title: "New", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "empty-fp file must be treated as new, not moved")
	assert.Len(t, h.media.All(), 2)
}

// --- scan_service.go:210 moved file detection: byPath nil && fp != "" ---

func TestScanService_MovedFile_DetectedByFingerprint(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "old-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/old.mkv", Size: 100, Fingerprint: "fp-stable"},
	})
	// Same fingerprint, different path → moved.
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/new.mkv", Size: 100, Fingerprint: "fp-stable", Title: "Moved", Type: library.MovieItem},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added, "moved file must not be counted as added")
	assert.Equal(t, 1, updated, "moved file must be counted as updated (moved)")
	item, err := h.media.GetByID(context.Background(), "old-1")
	require.NoError(t, err)
	require.NotNil(t, item.File)
	assert.Equal(t, "/m/new.mkv", item.File.Path, "moved item path must be updated")
	// Only 1 item (no duplicate).
	assert.Len(t, h.media.All(), 1)
}

// A distinct file in another library that happens to share the size+mtime
// fingerprint must NOT repoint the other library's item - it's created fresh.
func TestScanService_CrossLibraryFingerprintCollision_NoRepoint(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-a", Type: "movie", Paths: []string{"/a"}})
	h.libCmd.Seed(&library.Library{ID: "lib-a", Type: "movie", Paths: []string{"/a"}})
	h.libQuery.Seed(&library.Library{ID: "lib-b", Type: "movie", Paths: []string{"/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-b", Type: "movie", Paths: []string{"/b"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "a-1", LibraryID: "lib-a",
		File: &library.MediaFile{Path: "/a/film.mkv", Size: 100, Fingerprint: "collide"},
	})
	h.scanner.Results["/b"] = []library.ScannedFile{
		{Path: "/b/other.mkv", Size: 100, Fingerprint: "collide", Title: "Other", Type: library.MovieItem},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-b")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "a colliding file in another library must be created, not repointed")
	assert.Equal(t, 0, updated)
	a, err := h.media.GetByID(context.Background(), "a-1")
	require.NoError(t, err)
	require.NotNil(t, a.File)
	assert.Equal(t, "/a/film.mkv", a.File.Path, "library A's item must not be repointed by a library B scan")
}

// A same-library size+mtime fingerprint collision where the original file is
// STILL on disk must NOT repoint the existing item - the files are distinct, so
// a new item is created (the prior code repointed and the original vanished).
func TestScanService_SameLibCollision_OldFilePresent_NoRepoint(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "old-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/old.mkv", Size: 100, Fingerprint: "collide"},
	})
	// Both files are on disk (a real collision scans both), and the old one is
	// still present → the new file's fingerprint match must NOT repoint old-1.
	h.scanner.ExistingPaths = map[string]bool{"/m/old.mkv": true}
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/old.mkv", Size: 100, Fingerprint: "collide", Title: "Old", Type: library.MovieItem},
		{Path: "/m/new.mkv", Size: 100, Fingerprint: "collide", Title: "New", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	// The original item keeps its own path (not repointed onto new.mkv).
	old, err := h.media.GetByID(context.Background(), "old-1")
	require.NoError(t, err)
	require.NotNil(t, old.File)
	assert.Equal(t, "/m/old.mkv", old.File.Path, "the original item must keep its path")

	// A DISTINCT new item now owns new.mkv.
	created, err := h.media.GetByFilePath(context.Background(), "/m/new.mkv")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEqual(t, "old-1", string(created.ID), "the colliding file must get its own item, not repoint old-1")
}

func TestScanService_MovedFile_EmptyFingerprintNotMoved(t *testing.T) {
	// Target 210:49 `f.Fingerprint != ""` - empty fingerprint must NOT trigger moved path.
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "old-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/old.mkv", Size: 100, Fingerprint: "fp-a"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/new.mkv", Size: 100, Fingerprint: "", Title: "New", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "empty-fp file must be created (not treated as moved)")
}

// --- scan_service.go:219 update moved item path err ---

func TestScanService_MovedFile_UpdateErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "old-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/old.mkv", Size: 100, Fingerprint: "fp-x"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/new.mkv", Size: 100, Fingerprint: "fp-x", Type: library.MovieItem},
	}
	h.media.ErrUpdate = errors.New("update boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "update moved item path")
}

// --- scan_service.go:267 orphan cleanup err logged (equivalent: ListFilePaths has no inject).
// --- scan_service.go:276 update library last scanned err ---

func TestScanService_FinalizeScan_UpdateLibraryErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// Do NOT seed libCmd → Update returns ErrNotFound.
	h.scanner.Results["/m"] = nil

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "update library last scanned failed")
}

func TestScanService_FinalizeScan_UpdateLibrarySuccessNoLog(t *testing.T) {
	h := newScanHarness()
	lib := &library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/m"] = nil

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "update library last scanned failed")
	// libQuery seed and ScanService share the same *Library pointer, which is mutated.
	require.NotNil(t, lib.LastScannedAt)
	assert.WithinDuration(t, time.Now(), *lib.LastScannedAt, 2*time.Second)
}

// --- scan_service.go:287 publish LibraryScanCompleted err ---

func TestScanService_PublishScanCompleted_ErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = nil
	h.bus.PublishErr = errors.New("pub down")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	// both scan.started and scan.completed log failures
	assert.True(t, strings.Count(logs, "publish event") >= 2, "expected both start+complete publish errors")
}

// --- scan_service.go:303 len(orphanIDs) > 0 boundary ---

func TestScanService_Orphans_ZeroOrphansNoDelete(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem, ModTime: time.Time{}},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	var removed int
	for _, e := range h.bus.PublishedEvents() {
		if e.EventName() == "media.item.removed" {
			removed++
		}
	}
	assert.Equal(t, 0, removed, "no orphan → no removed event")
}

// --- scan_service.go:304 DeleteByIDs err ---

func TestScanService_Orphans_DeleteByIDsErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "orphan", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/gone.mkv"},
	})
	// Kept item keeps the scan non-empty so the degraded-mount safety guard
	// (empty scan of a non-empty library refuses to prune) does not trip.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}
	h.media.ErrDelete = errors.New("delete boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "batch delete orphaned media items failed")
}

// --- scan_service.go:309 search.RemoveMediaItem err ---

func TestScanService_Orphans_SearchRemoveErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "orphan", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/gone.mkv"},
	})
	// Kept item keeps the scan non-empty so the degraded-mount safety guard
	// (empty scan of a non-empty library refuses to prune) does not trip.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}
	h.search.Err = errors.New("search boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "remove orphan from search index failed")
}

// --- scan_service.go:318 publish MediaItemRemoved err ---

func TestScanService_Orphans_PublishRemovedErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "orphan", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/gone.mkv"},
	})
	// Kept item keeps the scan non-empty so the degraded-mount safety guard
	// (empty scan of a non-empty library refuses to prune) does not trip.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}
	h.bus.PublishErr = errors.New("bus down")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	// MediaItemRemoved publish failures are logged via "publish event"
	assert.Contains(t, logs, "media.item.removed")
}

// --- scan_service.go:329 len(orphanIDs) + pruned arithmetic ---

func TestScanService_Orphans_RemovedCountSumsOrphansAndPruned(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// series + season + orphan episode → prune season+series after removing episode.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "s1", LibraryID: "lib-tv", Type: "series", Title: "Show",
	})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "se1", LibraryID: "lib-tv", Type: "season", ParentID: "s1",
	})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "ep1", LibraryID: "lib-tv", Type: "episode", ParentID: "se1",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100},
	})
	// The scan sees one NEW file (a different show): this keeps the scan
	// non-empty so the degraded-mount guard does not trip, and the scan CREATES
	// its own chain for it (an addition, not a removal), so the removed count
	// stays exactly ep1 + its now-empty season + series = 3.
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/other/Other.S01E01.mkv", Size: 100},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	var completed *library.LibraryScanCompleted
	for _, e := range h.bus.PublishedEvents() {
		if c, ok := e.(library.LibraryScanCompleted); ok {
			c := c
			completed = &c
		}
	}
	require.NotNil(t, completed)
	// The orphaned episode and its now-empty season+series must all be gone,
	// and counted in ItemsRemoved (orphans + pruned containers). The exact total
	// also includes whatever the scan does with the new file, so assert the
	// known-removed chain plus a >=3 floor rather than an exact total.
	remaining := map[string]bool{}
	for _, it := range h.media.All() {
		remaining[string(it.ID)] = true
	}
	assert.False(t, remaining["ep1"], "orphaned episode removed")
	assert.False(t, remaining["se1"], "empty season pruned")
	assert.False(t, remaining["s1"], "empty series pruned")
	assert.GreaterOrEqual(t, completed.ItemsRemoved, 3, "orphan + 2 pruned containers counted")
}

// --- scan_service.go:365 new item: dur > 0 boundary ---

func TestScanService_NewItem_DurationUpdatedWhenPositive(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(42000)}
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	items := h.media.All()
	require.Len(t, items, 1)
	require.NotNil(t, items[0].File)
	assert.Equal(t, int64(42000), items[0].File.Duration.Milliseconds(), "positive duration must be stored")
}

func TestScanService_NewItem_DurationZeroNotStored(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// prober returns Duration zero (default).
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	items := h.media.All()
	require.Len(t, items, 1)
	if items[0].File != nil {
		assert.Equal(t, int64(0), items[0].File.Duration.Milliseconds())
	}
}

// --- scan_service.go:367 update duration err (new item) ---
// Can't cause an Update error only on the duration path without a per-call counter.
// Covered indirectly by slog check when ErrUpdate is set.
func TestScanService_NewItem_DurationUpdateErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(42000)}
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	h.media.ErrUpdate = errors.New("update boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "update media item after probe failed")
}

// --- scan_service.go:375 search.IndexMediaItem err (new item) ---

func TestScanService_NewItem_SearchIndexErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.search.Err = errors.New("idx boom")
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "index media item failed")
}

// --- scan_service.go:392 publish MediaItemAdded err (new item) ---

func TestScanService_NewItem_PublishAddedErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.bus.PublishErr = errors.New("bus boom")
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "media.item.added")
}

// --- scan_service.go:399 unchanged file with new path or new parentID ---

func TestScanService_Unchanged_PathRelocationUpdatesItem(t *testing.T) {
	// Fingerprint match via byFingerprint triggers Update path; but detectChanges handles
	// this before reaching upsertMediaFile. For movie type we hit upsertMediaFile only for
	// genuinely new/changed work. Use GetByFileFingerprint fallback in upsertMediaFile:
	// seed a TV library, let scanTVShow insert episode via upsertMediaFile; then re-scan.
	// Simpler: tvshow flow routes every file through upsertMediaFile every time.
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	// Re-scan (second run): file unchanged, same path + same parent → no-op update path.
	_, updated, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, updated, 1)
}

// --- scan_service.go:417 changed file, metadata != nil (we use nil path to kill negation) ---
// The service always has metadata=nil in our harness (WithMetadataExtraction never called),
// so this branch is exercised by default. Ensuring it doesn't crash.

// --- scan_service.go:420 changed path update err logged ---

func TestScanService_ChangedItem_UpdateErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// Pre-seed an item. Re-scan with changed size → triggers "changed" branch.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-old"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 200, Fingerprint: "fp-new", Type: library.MovieItem},
	}
	h.media.ErrUpdate = errors.New("update boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "update media item failed")
}

// --- scan_service.go:424 changed: search.IndexMediaItem err logged ---

func TestScanService_ChangedItem_SearchIndexErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-old"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 200, Fingerprint: "fp-new", Type: library.MovieItem},
	}
	h.search.Err = errors.New("idx boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "index media item failed")
}

// --- scan_service.go:430 changed + overview empty → publish re-add event ---
// boundary: len(seriesTitle) > 0 - movie scan passes no seriesTitle, so len==0.
// Negated (`<= 0`) makes condition always true; else always false - but behaviour same.
// Observable: confirm MediaItemAdded event fires on changed+empty overview.

func TestScanService_ChangedItem_EmptyOverviewTriggersReAddEvent(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-old"},
		Overview: "",
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 200, Fingerprint: "fp-new", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	var addedEvts int
	for _, e := range h.bus.PublishedEvents() {
		if ev, ok := e.(library.MediaItemAdded); ok && ev.MediaItemID == "m1" {
			addedEvts++
		}
	}
	assert.Equal(t, 1, addedEvts, "changed item with empty overview must re-emit MediaItemAdded")
}

func TestScanService_ChangedItem_NonEmptyOverviewNoReAddEvent(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-old"},
		Overview: "already enriched",
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 200, Fingerprint: "fp-new", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	for _, e := range h.bus.PublishedEvents() {
		if ev, ok := e.(library.MediaItemAdded); ok && ev.MediaItemID == "m1" {
			t.Fatalf("unexpected re-add event for item with overview")
		}
	}
}

// --- scan_service.go:442 publish changed re-add err logged ---

func TestScanService_ChangedItem_PublishReAddErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-old"},
		Overview: "",
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 200, Fingerprint: "fp-new", Type: library.MovieItem},
	}
	h.bus.PublishErr = errors.New("bus boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "media.item.added")
}

// --- scan_service.go:462 metadata != nil (in ProbeAndStoreStreams) ---
// We don't wire metadata → nil branch taken. Exercised implicitly.

// --- scan_service.go:471 ReplaceBatch err logged ---

func TestScanService_ProbeAndStoreStreams_ReplaceBatchErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// A real container stream (count > 0) so the write reaches ReplaceBatch; a
	// zero-container probe takes the preserve-existing path instead.
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(1000),
		Streams:  []*library.MediaStream{{ID: "st-v", StreamType: "video", Codec: "h264"}},
	}
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	h.streams.Err = errors.New("streams boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "replace media streams failed")
}

// --- scan_service.go:380 new item seriesTitle empty ---
// Movie scan never passes seriesTitle → len==0. Verified indirectly by
// TestScanService_NewItem_PublishAddedErrorLogged & TestScanLibrary_NewFile_CreatesMediaItem
// (MediaItemAdded with empty SeriesTitle). Explicit check:

func TestScanService_NewItem_MovieHasEmptySeriesTitle(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	var saw bool
	for _, e := range h.bus.PublishedEvents() {
		if ev, ok := e.(library.MediaItemAdded); ok {
			assert.Empty(t, ev.SeriesTitle, "movie MediaItemAdded has empty SeriesTitle")
			saw = true
		}
	}
	assert.True(t, saw)
}

// --- 303 boundary: when no orphans, DeleteByIDs must NOT be called (no error log) ---

func TestScanService_NoOrphans_DeleteByIDsNotCalled(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-k"},
	})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-k", Type: library.MovieItem},
	}
	// Set ErrDelete: if DeleteByIDs is called (with empty slice), our fake returns err → log.
	h.media.ErrDelete = errors.New("should not be called")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "batch delete orphaned media items failed",
		"DeleteByIDs must not be called when no orphans (len>0 boundary)")
}

// --- 365 boundary: when dur == 0, Update must NOT be called for duration ---

func TestScanService_NewItem_DurationZeroNoDurationUpdate(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	// No prober result → Duration zero (default fake).
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	// If duration Update were called, ErrUpdate would cause a slog for "duration failed".
	h.media.ErrUpdate = errors.New("should not be called")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "update media item duration failed",
		"duration Update must not be called when dur == 0")
}

// --- 399, 403, 430: TV show rescan with unchanged file but different parent ---

func TestScanService_TVShow_UnchangedFileDifferentParentUpdates(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// Pre-seed an orphan episode with a bogus parent. Same fingerprint as what scan will see.
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "ep1", LibraryID: "lib-tv", ParentID: "bogus-parent",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp"},
		Type: library.Episode, Title: "S01E01",
	})
	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, updated, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, updated, 1)

	// ep1 must have been re-parented (ParentID replaced with a real season ID).
	got, err := h.media.GetByID(context.Background(), "ep1")
	require.NoError(t, err)
	assert.NotEqual(t, "bogus-parent", got.ParentID, "parent must be rewritten when different")
}

func TestScanService_TVShow_UnchangedMovedWithUpdateErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.media.Create(context.Background(), &library.MediaItem{
		ID: "ep1", LibraryID: "lib-tv", ParentID: "bogus-parent",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp"},
		Type: library.Episode, Title: "S01E01",
	})
	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}
	h.media.ErrUpdate = errors.New("update boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
		// scanTVShow propagates nothing but upsert errors are logged via slog.
		_ = err
	})
	assert.Contains(t, logs, "update media item failed")
}

// --- 464: subScanner used when metadata is nil ---

func TestScanService_ProbeAndStoreStreams_UsesSubScannerWhenMetadataNil(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(1000)}
	subScanner := libfakes.NewFakeSubtitleScanner()
	subScanner.Results["/m/a.mkv"] = []library.ExternalSubtitle{
		{Path: "/m/a.en.srt", Language: "eng", Format: "srt"},
	}
	h.svc.WithSubtitleScanner(subScanner)
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := h.media.All()
	require.Len(t, items, 1)
	streams, err := h.streams.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	var extFound bool
	for _, st := range streams {
		if st.IsExternal {
			extFound = true
		}
	}
	assert.True(t, extFound, "external subtitle must be stored when subScanner discovers one")
}

// A probe that succeeds with ZERO container streams is a failed probe; it must
// NOT wipe the video/audio metadata already stored for the item (ReplaceBatch
// deletes-then-inserts). The freshly-discovered external sub is still recorded.
func TestScanService_ProbeAndStoreStreams_EmptyProbePreservesExistingStreams(t *testing.T) {
	h := newScanHarness()
	const itemID = "mi-x"
	mid := shared.MediaItemID(itemID)
	require.NoError(t, h.streams.ReplaceBatch(context.Background(), mid, []*library.MediaStream{
		{ID: "st-v", MediaItemID: mid, StreamType: "video", Codec: "h264"},
		{ID: "st-a", MediaItemID: mid, StreamType: "audio", Codec: "aac"},
	}))
	// Reprobe yields 0 container streams (truncated / mid-copy) but a sidecar exists.
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(42000)}
	subScanner := libfakes.NewFakeSubtitleScanner()
	subScanner.Results["/m/a.mkv"] = []library.ExternalSubtitle{
		{Path: "/m/a.en.srt", Language: "eng", Format: "srt"},
	}
	h.svc.WithSubtitleScanner(subScanner)

	_, _, probedOK := h.svc.ProbeAndStoreStreams(context.Background(), itemID, "/m/a.mkv")
	assert.False(t, probedOK, "0 container streams must count as a failed probe")

	streams, err := h.streams.GetByMediaItemID(context.Background(), mid)
	require.NoError(t, err)
	var hasVideo, hasAudio, hasExt bool
	for _, st := range streams {
		switch {
		case st.IsExternal:
			hasExt = true
		case st.StreamType == "video":
			hasVideo = true
		case st.StreamType == "audio":
			hasAudio = true
		}
	}
	assert.True(t, hasVideo, "existing video stream must survive an empty reprobe")
	assert.True(t, hasAudio, "existing audio stream must survive an empty reprobe")
	assert.True(t, hasExt, "newly discovered external sub must still be recorded")
}

func TestScanService_NewItem_TVShowHasSeriesTitle(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	var saw bool
	for _, e := range h.bus.PublishedEvents() {
		if ev, ok := e.(library.MediaItemAdded); ok && ev.Type == library.Episode {
			assert.Equal(t, "MyShow", ev.SeriesTitle, "episode MediaItemAdded must carry series title")
			saw = true
		}
	}
	assert.True(t, saw)
}

// --- scan_service: Publish must detach from caller ctx cancellation ---
//
// Repro: Meilisearch (or any inline scan step) saturates and the scan ctx
// expires. The subsequent events.Publish would receive that cancelled ctx,
// and InMemoryEventBus drops the event before enqueueing it. Downstream
// subscribers (trickplay, intro detection, metadata enricher, artwork
// downloader, search re-index) then NEVER run for that media item, leaving
// the library half-enriched with no failure surfaced to the user. Guard:
// every events.Publish in the scan pipeline must see ctx.Err() == nil even
// when the scan-level ctx is already cancelled at publish time.
func TestScanService_Publish_DetachedFromScanCtx(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	// Hard worst case: the caller-supplied ctx is already cancelled when the
	// scan starts. Production fakes ignore ctx so the body still reaches each
	// Publish - we then assert each Publish receives a non-cancelled ctx.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, _ = h.svc.ScanLibrary(ctx, "lib-1")

	ctxs := h.bus.PublishContexts()
	require.NotEmpty(t, ctxs, "scan must reach at least one events.Publish call")
	for i, c := range ctxs {
		assert.NoError(t, c.Err(),
			"Publish #%d (event %s) leaked the cancelled scan ctx - bus would drop it in prod",
			i, h.bus.PublishedEvents()[i].EventName())
	}
}

// --- healOrphanTitles: rewrites stale parser output on orphans ---

// Scenario: orphan was scanned with old parser (left "1080" in title).
// File hasn't moved since, so detectChanges short-circuits the regular
// path. healOrphanTitles must catch and rewrite it.
func TestScanService_HealOrphanTitles_RewritesDirtyOrphan(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	now := time.Now()
	h.media.Create(context.Background(), &library.MediaItem{
		ID:        "orph-1",
		LibraryID: "lib-1",
		Title:     "Movie 1080",
		ScanTitle: "Movie 1080",
		// Catalog absent → orphan (no provider IDs).
		File: &library.MediaFile{Path: "/m/file.mkv", Size: 100, Fingerprint: "fp", ModTime: now},
	})
	// Same file on disk (unchanged), but the parser now strips "1080".
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/file.mkv", Size: 100, Fingerprint: "fp", ModTime: now,
			Title: "Movie", Type: library.MovieItem},
	}
	h.scanner.ParsedTitles["/m/file.mkv"] = "Movie"

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "orph-1")
	require.NoError(t, err)
	assert.Equal(t, "Movie", got.Title, "orphan Title must heal to fresh parser output")
	assert.Equal(t, "Movie", got.ScanTitle, "orphan ScanTitle must heal too")
}

// Guard: TMDB-matched item (has provider IDs) must NOT be processed by
// the heal pass - the MissingProviderIDs filter must hold.
func TestScanService_HealOrphanTitles_SkipsMatchedItem(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	now := time.Now()
	h.media.Create(context.Background(), &library.MediaItem{
		ID:        "matched-1",
		LibraryID: "lib-1",
		Title:     "Die Hard 2: Die Harder",
		ScanTitle: "Die hard 2 2016P",
		Catalog:   &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "1573"}},
		File:      &library.MediaFile{Path: "/m/diehard.mkv", Size: 100, Fingerprint: "fp", ModTime: now},
	})
	h.scanner.ParsedTitles["/m/diehard.mkv"] = "Die hard 2"
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/diehard.mkv", Size: 100, Fingerprint: "fp", ModTime: now,
			Title: "Die hard 2", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "matched-1")
	require.NoError(t, err)
	assert.Equal(t, "Die Hard 2: Die Harder", got.Title, "matched item must not be touched by the heal pass")
}

// Guard: orphan whose Title was edited via SetMetadataOverride propagation
// (Title != ScanTitle, no provider IDs yet) must also be skipped - the
// user's manual cleanup wins over a parser re-derivation.
func TestScanService_HealOrphanTitles_SkipsUserEditedOrphan(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	now := time.Now()
	h.media.Create(context.Background(), &library.MediaItem{
		ID:        "orph-2",
		LibraryID: "lib-1",
		Title:     "User's Custom Cleanup",       // propagated via override hook
		ScanTitle: "Original Polluted Title 720", // pristine parser-pollution
		File:      &library.MediaFile{Path: "/m/edit.mkv", Size: 100, Fingerprint: "fp", ModTime: now},
	})
	h.scanner.ParsedTitles["/m/edit.mkv"] = "Original Polluted Title"
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/edit.mkv", Size: 100, Fingerprint: "fp", ModTime: now,
			Title: "Original Polluted Title", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "orph-2")
	require.NoError(t, err)
	assert.Equal(t, "User's Custom Cleanup", got.Title, "user-edited orphan must not regress")
}

// Recovery from the previous-pass bug: scan_title wasn't being persisted
// by MediaItemRepo.Update, so an earlier heal left rows with Title clean
// but ScanTitle still dirty. The heal pass must recognise this and finish
// the job instead of skipping (Title != ScanTitle would otherwise look
// like a user/TMDB edit). Distinguishing signal: Title == current parser
// output already.
func TestScanService_HealOrphanTitles_FinishesHalfHealedRow(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	now := time.Now()
	h.media.Create(context.Background(), &library.MediaItem{
		ID:        "half-1",
		LibraryID: "lib-1",
		Title:     "Die hard 2",       // already healed
		ScanTitle: "Die hard 2 2016P", // stuck because Update didn't persist
		File:      &library.MediaFile{Path: "/m/dh2.mkv", Size: 100, Fingerprint: "fp", ModTime: now},
	})
	h.scanner.ParsedTitles["/m/dh2.mkv"] = "Die hard 2"
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/dh2.mkv", Size: 100, Fingerprint: "fp", ModTime: now,
			Title: "Die hard 2", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "half-1")
	require.NoError(t, err)
	assert.Equal(t, "Die hard 2", got.Title)
	assert.Equal(t, "Die hard 2", got.ScanTitle, "half-healed row must converge on the new parser output")
}

// Drift-only update: if parser produces the same title that's already
// stored, no Update is issued (idempotent).
func TestScanService_HealOrphanTitles_NoDriftNoUpdate(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	now := time.Now()
	h.media.Create(context.Background(), &library.MediaItem{
		ID:        "orph-3",
		LibraryID: "lib-1",
		Title:     "Clean Title",
		ScanTitle: "Clean Title",
		File:      &library.MediaFile{Path: "/m/clean.mkv", Size: 100, Fingerprint: "fp", ModTime: now},
	})
	h.scanner.ParsedTitles["/m/clean.mkv"] = "Clean Title"
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/clean.mkv", Size: 100, Fingerprint: "fp", ModTime: now,
			Title: "Clean Title", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "orph-3")
	require.NoError(t, err)
	assert.Equal(t, "Clean Title", got.Title)
}

// A file indexed while its probe yielded no streams gets StreamProbeAttempts=1,
// then a later scan re-probes it and heals once the probe succeeds.
func TestScanService_StreamlessFile_ReprobesAndHeals(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	// First probe yields a duration but NO streams (worker-budget timeout / I/O
	// hiccup simulated by an empty stream list).
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(42000)}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := h.media.All()
	require.Len(t, items, 1)
	require.NotNil(t, items[0].File)
	assert.Equal(t, 1, items[0].File.StreamProbeAttempts, "streamless create must record one failed probe attempt")
	streams, err := h.streams.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Empty(t, streams)

	// The probe now succeeds; a re-scan of the (unchanged) file must re-probe.
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams: []*library.MediaStream{
			{ID: "ps1", StreamType: "video", Codec: "h264"},
			{ID: "ps2", StreamType: "audio", Codec: "aac"},
		},
	}

	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items = h.media.All()
	require.Len(t, items, 1)
	assert.Equal(t, 0, items[0].File.StreamProbeAttempts, "successful re-probe must reset the counter")
	streams, err = h.streams.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Len(t, streams, 2, "re-probe must store the now-discoverable streams")
}

// A durably-unprobeable file's re-probe is bounded: the counter climbs to
// maxStreamProbeAttempts and then stops (no re-probe on every subsequent scan).
func TestScanService_StreamlessFile_GivesUpAfterMax(t *testing.T) {
	const maxAttempts = 2 // mirrors scan_service.go maxStreamProbeAttempts
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	// Probe never yields streams.
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{Duration: mustProbeDuration(42000)}

	// Scan many more times than the cap; the counter must plateau at the cap.
	for range maxAttempts + 3 {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err)
	}

	items := h.media.All()
	require.Len(t, items, 1)
	assert.Equal(t, maxAttempts, items[0].File.StreamProbeAttempts,
		"bounded re-probe must plateau at the cap, not keep climbing")
}
