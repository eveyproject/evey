package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
