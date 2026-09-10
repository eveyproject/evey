package metadata

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// command_service_artwork_custom_test.go - end-to-end coverage for
// AddCustomArtwork (command_service_artwork_custom.go), the write path an
// admin's uploaded bytes land on after the HTTP handler has sniffed them.

func newArtworkCmdSvc(overrides *metafakes.FakeMetadataOverrideRepository, artwork *metafakes.FakeArtworkRepository) *CommandService {
	return NewCommandService(
		overrides,
		artwork,
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
}

// sizedReader streams n bytes without holding them all in memory at once -
// the size-cap test needs a source well past maxCustomArtworkBytes, and
// building that as a single byte slice would be wasteful for no reason.
type sizedReader struct{ remaining int64 }

func (r *sizedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := range n {
		p[i] = 'a'
	}
	r.remaining -= int64(n)
	return n, nil
}

// errAfterFirstRead yields one chunk of real data, then fails - simulating a
// client connection dropped mid-upload.
type errAfterFirstRead struct {
	sent bool
}

func (r *errAfterFirstRead) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, []byte("partial-image-bytes"))
		return n, nil
	}
	return 0, errors.New("read failed mid-stream")
}

func TestAddCustomArtwork_StorageDisabled(t *testing.T) {
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository())

	rec, err := cmdSvc.AddCustomArtwork(context.Background(), "m1", "ARTWORK_TYPE_PRIMARY", strings.NewReader("img"), ".jpg")
	require.Error(t, err)
	assert.ErrorIs(t, err, errArtworkStorageDisabled)
	assert.Nil(t, rec)
}

func TestAddCustomArtwork_EmptyMediaItemID(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo)
	WithAutoDownload(t.TempDir(), false)(cmdSvc)

	rec, err := cmdSvc.AddCustomArtwork(context.Background(), "", "ARTWORK_TYPE_PRIMARY", strings.NewReader("img"), ".jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty media item id")
	assert.Nil(t, rec)
}

func TestAddCustomArtwork_EmptyExtDefaultsToJPG(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo)
	dataDir := t.TempDir()
	WithAutoDownload(dataDir, false)(cmdSvc)

	rec, err := cmdSvc.AddCustomArtwork(context.Background(), shared.MediaItemID("m1"), "ARTWORK_TYPE_PRIMARY", strings.NewReader("img"), "")
	require.NoError(t, err)
	require.NotNil(t, rec.LocalPath)
	assert.Equal(t, ".jpg", filepath.Ext(*rec.LocalPath))
}

// The new upload must rank below every existing artwork of the same type
// (uploading IS choosing, per the file's comment) and leave the other type
// untouched.
func TestAddCustomArtwork_Success_WritesFileRecordsAndOutranksExisting(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	require.NoError(t, artworkRepo.Create(context.Background(), &domain.ArtworkRecord{
		ID: "existing-poster", MediaItemID: "m1", Type: "ARTWORK_TYPE_PRIMARY", Provider: "tmdb", SortOrder: -1,
	}))
	require.NoError(t, artworkRepo.Create(context.Background(), &domain.ArtworkRecord{
		ID: "existing-backdrop", MediaItemID: "m1", Type: "ARTWORK_TYPE_BACKDROP", Provider: "tmdb", SortOrder: -5,
	}))
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo)
	dataDir := t.TempDir()
	WithAutoDownload(dataDir, false)(cmdSvc)

	rec, err := cmdSvc.AddCustomArtwork(context.Background(), shared.MediaItemID("m1"), "ARTWORK_TYPE_PRIMARY", strings.NewReader("poster-bytes"), ".png")
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.Equal(t, CustomArtworkProvider, rec.Provider)
	assert.Less(t, rec.SortOrder, -1, "the new upload must rank above (lower than) the existing poster of the same type")

	require.NotNil(t, rec.LocalPath)
	wantPath := filepath.Join(dataDir, "artworks", "m1", rec.ID+".png")
	assert.Equal(t, wantPath, *rec.LocalPath)
	data, readErr := os.ReadFile(wantPath)
	require.NoError(t, readErr)
	assert.Equal(t, "poster-bytes", string(data))

	stored, err := artworkRepo.GetByMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	var found bool
	for _, s := range stored {
		if s.ID == rec.ID {
			found = true
		}
	}
	assert.True(t, found, "the new record must be persisted")

	// The pre-existing backdrop of the other type must be untouched by the
	// poster's ranking.
	for _, s := range stored {
		if s.ID == "existing-backdrop" {
			assert.Equal(t, -5, s.SortOrder)
		}
	}
}

func TestAddCustomArtwork_SourceExceedsMax_NoPartialFile(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo)
	dataDir := t.TempDir()
	WithAutoDownload(dataDir, false)(cmdSvc)

	src := &sizedReader{remaining: maxCustomArtworkBytes + (1 << 20)} // 1MB past the cap
	rec, err := cmdSvc.AddCustomArtwork(context.Background(), shared.MediaItemID("m-big"), "ARTWORK_TYPE_PRIMARY", src, ".jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
	assert.Nil(t, rec)

	assertNoLeftoverFiles(t, dataDir, "m-big")
}

func TestAddCustomArtwork_ReadErrorMidStream_NoPartialFile(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	cmdSvc := newArtworkCmdSvc(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo)
	dataDir := t.TempDir()
	WithAutoDownload(dataDir, false)(cmdSvc)

	rec, err := cmdSvc.AddCustomArtwork(context.Background(), shared.MediaItemID("m-err"), "ARTWORK_TYPE_PRIMARY", &errAfterFirstRead{}, ".jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read failed mid-stream")
	assert.Nil(t, rec)

	assertNoLeftoverFiles(t, dataDir, "m-err")

	stored, err := artworkRepo.GetByMediaItem(context.Background(), "m-err")
	require.NoError(t, err)
	assert.Empty(t, stored, "a failed write must not persist a row for a file that isn't there")
}

// assertNoLeftoverFiles fails the test if the media item's artwork directory
// holds anything - in particular a stray ".part" temp file, the exact leak a
// unit test around the size-check alone would never notice.
func assertNoLeftoverFiles(t *testing.T, dataDir, mediaItemID string) {
	t.Helper()
	dir := filepath.Join(dataDir, "artworks", mediaItemID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	assert.Empty(t, entries, "no file (partial or final) must remain after a failed upload")
}
