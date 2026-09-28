package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
)

// Only zero-byte resized variants (<base>_w<width>.webp) are deleted; valid
// variants, the full-resolution webp, and source files are all left untouched.
func TestPurgeCorruptVariants_RemovesOnlyZeroByteVariants(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "artworks", "mi-1")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, data, 0o644))
		return p
	}
	zeroVariant := write("poster_w500.webp", nil)         // corrupt -> must be deleted
	goodVariant := write("poster_w342.webp", []byte("x")) // valid variant -> keep
	fullRes := write("poster.webp", nil)                  // 0-byte but NOT a _w variant -> keep
	original := write("poster.jpg", []byte("y"))          // source -> keep
	emptyOtherExt := write("backdrop_w780.jpg", nil)      // 0-byte but not webp -> keep

	cmd := newCmdSvcWithDataDir(t, dataDir, metafakes.NewFakeArtworkRepository(), nil)
	removed, err := cmd.PurgeCorruptVariants(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	assert.NoFileExists(t, zeroVariant)
	assert.FileExists(t, goodVariant)
	assert.FileExists(t, fullRes)
	assert.FileExists(t, original)
	assert.FileExists(t, emptyOtherExt)
}

// Every digit counts as a width (a 0-byte _w1920 variant goes), and a name
// whose "_w" marker is not followed by digits alone is never taken for a
// variant, even at 0 bytes. The other test's widths never held a 9
// (mutation 2026-09-28).
func TestPurgeCorruptVariants_WidthDigitsAndNonVariantNames(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "artworks", "mi-1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	wide := filepath.Join(dir, "backdrop_w1920.webp")
	notAVariant := filepath.Join(dir, "poster_wide.webp")
	require.NoError(t, os.WriteFile(wide, nil, 0o644))
	require.NoError(t, os.WriteFile(notAVariant, nil, 0o644))

	cmd := newCmdSvcWithDataDir(t, dataDir, metafakes.NewFakeArtworkRepository(), nil)
	removed, err := cmd.PurgeCorruptVariants(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.NoFileExists(t, wide)
	assert.FileExists(t, notAVariant)
}

// Missing artworks directory (nothing downloaded yet) is not an error.
func TestPurgeCorruptVariants_MissingDirIsNoOp(t *testing.T) {
	cmd := newCmdSvcWithDataDir(t, t.TempDir(), metafakes.NewFakeArtworkRepository(), nil)
	removed, err := cmd.PurgeCorruptVariants(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

// Unconfigured dataDir must fail loudly rather than silently walking "/artworks".
func TestPurgeCorruptVariants_NoDataDirErrors(t *testing.T) {
	cmd := newCmdSvcWithDataDir(t, "", metafakes.NewFakeArtworkRepository(), nil)
	_, err := cmd.PurgeCorruptVariants(context.Background())
	require.Error(t, err)
}
