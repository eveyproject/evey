package metadata_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

func newCmdSvcWithDataDir(t *testing.T, dataDir string, artworkRepo *metafakes.FakeArtworkRepository, downloader metadata.ArtworkDownloader) *appmetadata.CommandService {
	t.Helper()
	if downloader == nil {
		downloader = metafakes.NewFakeArtworkDownloader()
	}
	cmd := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artworkRepo,
		downloader,
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	cmd.SetAutoDownload(dataDir, true)
	return cmd
}

// Happy path: a record with source_url + no local_path must be downloaded
// on retry and its DB row reconciled with the resulting local_path.
func TestRetryMissingArtworks_DownloadsAndReconciles(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	artRepo := metafakes.NewFakeArtworkRepository()

	src := "https://example.com/poster.jpg"
	require.NoError(t, artRepo.Create(ctx, &metadata.ArtworkRecord{
		ID:          "art-1",
		MediaItemID: "m1",
		Type:        "poster",
		Provider:    "tmdb",
		SourceURL:   &src,
	}))

	cmd := newCmdSvcWithDataDir(t, dataDir, artRepo, nil)

	succeeded, failed := cmd.RetryMissingArtworks(ctx)
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 0, failed)

	got, err := artRepo.GetByMediaItem(ctx, "m1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].LocalPath, "LocalPath must be set after a successful retry")
	assert.NotEmpty(t, *got[0].LocalPath)
}

// Idempotency: if the file already exists on disk (server crashed between
// fs write and DB update, or a previous bulk download partially succeeded),
// the retry must reconcile without re-downloading and without double-counting.
func TestRetryMissingArtworks_PresentFileReconciled(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	artRepo := metafakes.NewFakeArtworkRepository()

	src := "https://example.com/poster.jpg"
	rec := &metadata.ArtworkRecord{
		ID:          "art-2",
		MediaItemID: "m2",
		Type:        "poster",
		Provider:    "tmdb",
		SourceURL:   &src,
	}
	require.NoError(t, artRepo.Create(ctx, rec))

	// Pre-create the destination file so the retry takes the reconcile branch.
	destPath := filepath.Join(dataDir, "artworks", "m2", "art-2.jpg")
	require.NoError(t, os.MkdirAll(filepath.Dir(destPath), 0o755))
	require.NoError(t, os.WriteFile(destPath, []byte("fake jpg"), 0o644))

	dl := metafakes.NewFakeArtworkDownloader()
	cmd := newCmdSvcWithDataDir(t, dataDir, artRepo, dl)

	succeeded, failed := cmd.RetryMissingArtworks(ctx)
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 0, failed)

	got, err := artRepo.GetByMediaItem(ctx, "m2")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].LocalPath)
}

// No-op when dataDir is unset. Defensive: protects against accidental
// run in a half-wired test or pre-setup boot.
func TestRetryMissingArtworks_NoDataDirReturnsZero(t *testing.T) {
	ctx := context.Background()
	artRepo := metafakes.NewFakeArtworkRepository()
	cmd := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		artRepo,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	// no SetAutoDownload → dataDir remains ""
	s, f := cmd.RetryMissingArtworks(ctx)
	assert.Zero(t, s)
	assert.Zero(t, f)
}

// countingDownloader fails every fetch with the given error and counts them.
type countingDownloader struct {
	calls atomic.Int32
	err   error
}

func (d *countingDownloader) Download(_ context.Context, _, _ string) error {
	d.calls.Add(1)
	return d.err
}

// A source the provider says is gone is fetched once and never again; one
// that merely failed waits out a backoff before the next pass lists it. Before
// this both were fetched again on every hourly pass, for good.
func TestRetryMissingArtworks_GivesUpAGoneSourceAndBacksOffAFailedOne(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	artRepo := metafakes.NewFakeArtworkRepository()
	gone, flaky := "https://example.com/gone.jpg", "https://example.com/flaky.jpg"
	require.NoError(t, artRepo.Create(ctx, &metadata.ArtworkRecord{ID: "art-gone", MediaItemID: "m1", Type: "poster", Provider: "tmdb", SourceURL: &gone}))
	require.NoError(t, artRepo.Create(ctx, &metadata.ArtworkRecord{ID: "art-flaky", MediaItemID: "m2", Type: "poster", Provider: "tmdb", SourceURL: &flaky}))

	dl := &countingDownloader{err: &metadata.FetchStatusError{Code: 404}}
	cmd := newCmdSvcWithDataDir(t, dataDir, artRepo, dl)
	_, failed := cmd.RetryMissingArtworks(ctx)
	assert.Equal(t, 2, failed)
	assert.Equal(t, int32(2), dl.calls.Load())
	assert.Equal(t, metadata.ArtworkMaxFetchAttempts, artRepo.FetchAttempts("art-gone"), "a gone source is given up at once")

	// The next pass: nothing is due (gone for good, the other waiting), so no fetch.
	_, failed = cmd.RetryMissingArtworks(ctx)
	assert.Equal(t, 0, failed)
	assert.Equal(t, int32(2), dl.calls.Load(), "no record is fetched again before its time")

	stats, err := artRepo.LocalPathStats(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Dead)
}

// An outage is not a dead source: the record is tried again, later, and the
// wait grows with every miss up to the cap.
func TestArtworkFetchBackoff_DoublesToTheCap(t *testing.T) {
	assert.Equal(t, time.Hour, metadata.ArtworkFetchBackoff(1))
	assert.Equal(t, 2*time.Hour, metadata.ArtworkFetchBackoff(2))
	assert.Equal(t, 8*time.Hour, metadata.ArtworkFetchBackoff(4))
	assert.Equal(t, metadata.ArtworkFetchBackoffMax, metadata.ArtworkFetchBackoff(12))
	assert.True(t, metadata.IsPermanentFetchError(&metadata.FetchStatusError{Code: 410}))
	assert.False(t, metadata.IsPermanentFetchError(&metadata.FetchStatusError{Code: 503}))
	assert.False(t, metadata.IsPermanentFetchError(errors.New("timeout")))
}
