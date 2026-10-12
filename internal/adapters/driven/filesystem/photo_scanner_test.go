package filesystem

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPhotoScanner(t *testing.T) {
	s := NewPhotoScanner()
	require.NotNil(t, s)
}

func TestPhotoScanner_WalkImageFiles(t *testing.T) {
	dir := t.TempDir()

	imageFiles := map[string]string{
		"photo.jpg":  "image/jpeg",
		"photo.jpeg": "image/jpeg",
		"pic.png":    "image/png",
		"snap.gif":   "image/gif",
		"raw.bmp":    "image/bmp",
	}
	for name := range imageFiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("img data"), 0644))
	}
	// Non-image
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644))

	s := NewPhotoScanner()
	var found []photo.ScannedImageFile
	err := s.WalkImageFiles(context.Background(), dir, func(f photo.ScannedImageFile) error {
		found = append(found, f)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, found, len(imageFiles))

	mimeMap := map[string]string{}
	for _, f := range found {
		mimeMap[filepath.Base(f.Path)] = f.MimeType
	}
	for name, expectedMime := range imageFiles {
		assert.Equal(t, expectedMime, mimeMap[name], "mime mismatch for %s", name)
	}
}

func TestPhotoScanner_WalkImageFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewPhotoScanner()
	var found []photo.ScannedImageFile
	err := s.WalkImageFiles(context.Background(), dir, func(f photo.ScannedImageFile) error {
		found = append(found, f)
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestPhotoScanner_WalkImageFiles_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("data"), 0644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewPhotoScanner()
	err := s.WalkImageFiles(ctx, dir, func(f photo.ScannedImageFile) error {
		t.Fatal("callback should not be called")
		return nil
	})
	require.Error(t, err)
}

func TestPhotoScanner_ReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	content := []byte("png bytes")
	require.NoError(t, os.WriteFile(path, content, 0644))

	s := NewPhotoScanner()
	data, err := s.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestPhotoScanner_ReadFile_NotFound(t *testing.T) {
	s := NewPhotoScanner()
	_, err := s.ReadFile("/tmp/nonexistent-photo-xyz.png")
	require.Error(t, err)
}

func TestPhotoScanner_ImageDimensions_ValidPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	img.Set(0, 0, color.White)
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	f.Close()

	s := NewPhotoScanner()
	w, h := s.ImageDimensions(path)
	assert.Equal(t, 320, w)
	assert.Equal(t, 240, h)
}

func TestPhotoScanner_ImageDimensions_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-an-image.png")
	require.NoError(t, os.WriteFile(path, []byte("not image data"), 0644))

	s := NewPhotoScanner()
	w, h := s.ImageDimensions(path)
	assert.Equal(t, 0, w)
	assert.Equal(t, 0, h)
}

func TestPhotoScanner_ImageDimensions_MissingFile(t *testing.T) {
	s := NewPhotoScanner()
	w, h := s.ImageDimensions("/tmp/nonexistent-xyz.png")
	assert.Equal(t, 0, w)
	assert.Equal(t, 0, h)
}

// Same contract as TestWalkAudioFiles_MissingRootIsAnError: a vanished library
// root must surface as an error, not as an empty library.
func TestWalkImageFiles_MissingRootIsAnError(t *testing.T) {
	s := NewPhotoScanner()
	err := s.WalkImageFiles(context.Background(), "/nonexistent-evey-test-root", func(photo.ScannedImageFile) error { return nil })
	if err == nil {
		t.Fatal("a missing library root must fail the walk, not read as an empty library")
	}
}
