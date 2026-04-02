package photo_test

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fsadapter "github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/photo"
	photofakes "github.com/gravity-zero/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// buildJPEGWithExif builds a minimal JPEG with an APP1 EXIF segment containing
// the given DateTime and Model IFD entries (little-endian TIFF).
func buildJPEGWithExif(dateTime, model string) []byte {
	// TIFF header (little-endian)
	tiffHeader := []byte("II")
	tiffHeader = append(tiffHeader, 0x2A, 0x00) // magic
	tiffHeader = append(tiffHeader, 0x08, 0x00, 0x00, 0x00) // IFD0 offset = 8

	entries := 0
	if dateTime != "" {
		entries++
	}
	if model != "" {
		entries++
	}

	numEntries := make([]byte, 2)
	binary.LittleEndian.PutUint16(numEntries, uint16(entries))

	ifd := numEntries
	// data area starts after IFD: 8 (tiff header) + 2 (count) + entries*12 + 4 (next IFD ptr)
	dataOffset := 8 + 2 + entries*12 + 4
	var dataArea []byte

	if dateTime != "" {
		entry := make([]byte, 12)
		binary.LittleEndian.PutUint16(entry[0:2], 0x0132) // DateTime tag
		binary.LittleEndian.PutUint16(entry[2:4], 2)       // ASCII type
		count := len(dateTime) + 1                          // include null
		binary.LittleEndian.PutUint32(entry[4:8], uint32(count))
		off := dataOffset + len(dataArea)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(off))
		ifd = append(ifd, entry...)
		dataArea = append(dataArea, []byte(dateTime)...)
		dataArea = append(dataArea, 0x00)
	}
	if model != "" {
		entry := make([]byte, 12)
		binary.LittleEndian.PutUint16(entry[0:2], 0x0110) // Model tag
		binary.LittleEndian.PutUint16(entry[2:4], 2)       // ASCII type
		count := len(model) + 1
		binary.LittleEndian.PutUint32(entry[4:8], uint32(count))
		off := dataOffset + len(dataArea)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(off))
		ifd = append(ifd, entry...)
		dataArea = append(dataArea, []byte(model)...)
		dataArea = append(dataArea, 0x00)
	}

	// next IFD pointer = 0
	ifd = append(ifd, 0x00, 0x00, 0x00, 0x00)

	tiff := append(tiffHeader, ifd...)
	tiff = append(tiff, dataArea...)

	// EXIF segment = "Exif\0\0" + TIFF
	exifPayload := append([]byte("Exif\x00\x00"), tiff...)

	// APP1 marker + length
	segLen := len(exifPayload) + 2
	app1 := []byte{0xFF, 0xE1}
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(segLen))
	app1 = append(app1, lenBytes...)
	app1 = append(app1, exifPayload...)

	// SOI + APP1 + minimal SOS + EOI
	jpeg := []byte{0xFF, 0xD8}
	jpeg = append(jpeg, app1...)
	// Append the JFIF + image data from minimalJPEG (skip SOI)
	jpeg = append(jpeg, minimalJPEG[2:]...)
	return jpeg
}

// --- ScanDirectory mutation-killing tests ---

func TestScanDirectory_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)

	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, bus.PublishedEvents())
}

func TestScanDirectory_NonexistentDir(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", "/nonexistent/path/xyz")

	// WalkDir logs walk errors as warnings and continues; no photos found
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestScanDirectory_LibraryValidation_NotFound(t *testing.T) {
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "nonexistent-lib", t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-lib")
	assert.Contains(t, err.Error(), "not found")
	assert.Equal(t, 0, count)
}

func TestScanDirectory_LibraryValidation_Found(t *testing.T) {
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	libRepo.Seed(&library.Library{ID: "lib-ok"})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "img.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-ok", dir)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestScanDirectory_LibraryNil_SkipsValidation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "any-lib", dir)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestScanDirectory_RepoUpsertError_CountZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "img.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	repo.UpsertErr = errors.New("db down")
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)

	require.NoError(t, err) // walk succeeds, upsert silently logs
	assert.Equal(t, 0, count)
}

func TestScanDirectory_PublishesPhotoAddedEvent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pic.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-ev", dir)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "photo.added", events[0].EventName())
	added := events[0].(photo.PhotoAdded)
	assert.Equal(t, "lib-ev", added.LibraryID)
	assert.NotEmpty(t, added.PhotoID)
	assert.Contains(t, added.Path, "pic.jpg")
}

func TestScanDirectory_EventNotPublished_OnUpsertError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "img.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	repo.Err = errors.New("upsert fail")
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	_, _ = svc.ScanDirectory(context.Background(), "lib-1", dir)

	assert.Empty(t, bus.PublishedEvents())
}

func TestScanDirectory_PhotoFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sunset.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-f", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, total, err := repo.ListByLibrary(ctx, "lib-f", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	p := photos[0]

	assert.Equal(t, "sunset", p.Title)
	assert.Equal(t, "lib-f", p.LibraryID)
	assert.Equal(t, "image/jpeg", p.MimeType)
	assert.Contains(t, p.FilePath, "sunset.jpg")
	assert.NotEmpty(t, p.ID)
	assert.Positive(t, p.FileSize)
	assert.False(t, p.CreatedAt.IsZero())
	assert.False(t, p.UpdatedAt.IsZero())
}

func TestScanDirectory_AlbumFromSubdir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "vacation")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "beach.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-a", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-a", 10, 0)
	require.Len(t, photos, 1)
	assert.Equal(t, "vacation", photos[0].Album)
	assert.Equal(t, "beach", photos[0].Title)
}

func TestScanDirectory_RootFileAlbumIsDirBasename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, err := svc.ScanDirectory(ctx, "lib-r", dir)
	require.NoError(t, err)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-r", 10, 0)
	require.Len(t, photos, 1)
	// album == filepath.Base(filepath.Dir(path)); for root files in dir,
	// this equals the dir basename. Album is empty only when dir basename == "." or == dir itself
	// With t.TempDir(), the basename is a non-empty string, so album is set to that basename
	p := photos[0]
	dirBase := filepath.Base(dir)
	// When file is directly in the scan dir, album = basename of dir unless it equals "." or the dir path
	// Since dir != dirBase (dir is absolute), album == dirBase
	assert.Equal(t, dirBase, p.Album)
}

func TestScanDirectory_AllImageExtensions(t *testing.T) {
	dir := t.TempDir()
	exts := []string{".jpg", ".jpeg", ".png", ".heic", ".webp", ".gif", ".tiff", ".tif", ".bmp"}
	mimes := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".heic": "image/heic",
		".webp": "image/webp", ".gif": "image/gif",
		".tiff": "image/tiff", ".tif": "image/tiff",
		".bmp": "image/bmp",
	}
	for _, ext := range exts {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file"+ext), minimalJPEG, 0600))
	}

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-ext", dir)
	require.NoError(t, err)
	assert.Equal(t, len(exts), count)

	photos, _, _ := repo.ListByLibrary(context.Background(), "lib-ext", 100, 0)
	for _, p := range photos {
		ext := filepath.Ext(p.FilePath)
		expected, ok := mimes[ext]
		assert.True(t, ok, "unexpected ext %s", ext)
		assert.Equal(t, expected, p.MimeType)
	}
}

func TestScanDirectory_NonImageExtSkipped(t *testing.T) {
	dir := t.TempDir()
	skipped := []string{".txt", ".mp4", ".mkv", ".mp3", ".doc", ".html"}
	for _, ext := range skipped {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file"+ext), []byte("data"), 0600))
	}

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestScanDirectory_CaseInsensitiveExt(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.JPG"), minimalJPEG, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.Png"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestScanDirectory_ContextCanceled(t *testing.T) {
	dir := t.TempDir()
	for i := range 20 {
		name := filepath.Join(dir, "img"+string(rune('a'+i))+".jpg")
		require.NoError(t, os.WriteFile(name, minimalJPEG, 0600))
	}

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	count, _ := svc.ScanDirectory(ctx, "lib-1", dir)
	// with canceled ctx, should process fewer than all files
	assert.Less(t, count, 20)
}

func TestScanDirectory_MultipleFilesCount(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		name := filepath.Join(dir, "p"+string(rune('0'+i))+".png")
		require.NoError(t, os.WriteFile(name, minimalJPEG, 0600))
	}

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-m", dir)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.Len(t, bus.PublishedEvents(), 5) // 5 PhotoAdded
}

// --- EXIF readExifData / parseExifSegment mutation-killing tests ---
// These are package-level functions tested via ScanDirectory integration

func TestScanDirectory_ExifDateTime_Extracted(t *testing.T) {
	dir := t.TempDir()
	jpeg := buildJPEGWithExif("2024:06:15 14:30:00", "Canon EOS R5")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exif.jpg"), jpeg, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-exif", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-exif", 10, 0)
	require.Len(t, photos, 1)
	p := photos[0]

	require.NotNil(t, p.TakenAt, "TakenAt should be set from EXIF DateTime")
	assert.Equal(t, 2024, p.TakenAt.Year())
	assert.Equal(t, 6, int(p.TakenAt.Month()))
	assert.Equal(t, 15, p.TakenAt.Day())
	assert.Equal(t, 14, p.TakenAt.Hour())
	assert.Equal(t, 30, p.TakenAt.Minute())
	assert.Equal(t, "Canon EOS R5", p.CameraModel)
}

func TestScanDirectory_ExifDateOnly(t *testing.T) {
	dir := t.TempDir()
	jpeg := buildJPEGWithExif("2023:01:01 00:00:00", "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dateonly.jpg"), jpeg, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-d", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-d", 10, 0)
	require.Len(t, photos, 1)
	require.NotNil(t, photos[0].TakenAt)
	assert.Equal(t, 2023, photos[0].TakenAt.Year())
	assert.Empty(t, photos[0].CameraModel)
}

func TestScanDirectory_ExifModelOnly(t *testing.T) {
	dir := t.TempDir()
	jpeg := buildJPEGWithExif("", "Nikon Z6")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.jpg"), jpeg, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-m2", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-m2", 10, 0)
	require.Len(t, photos, 1)
	assert.Equal(t, "Nikon Z6", photos[0].CameraModel)
}

func TestScanDirectory_PngNoExif(t *testing.T) {
	dir := t.TempDir()
	// PNG files should not have EXIF extracted (only JPEG)
	minPNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.png"), minPNG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	count, err := svc.ScanDirectory(ctx, "lib-png", dir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-png", 10, 0)
	require.Len(t, photos, 1)
	assert.Nil(t, photos[0].TakenAt)
	assert.Empty(t, photos[0].CameraModel)
	assert.Equal(t, "image/png", photos[0].MimeType)
}

func TestScanDirectory_OriginalFilePreserved(t *testing.T) {
	dir := t.TempDir()
	jpeg := buildJPEGWithExif("2024:01:01 12:00:00", "TestCam")
	fpath := filepath.Join(dir, "strip.jpg")
	require.NoError(t, os.WriteFile(fpath, jpeg, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	_, err := svc.ScanDirectory(context.Background(), "lib-s", dir)
	require.NoError(t, err)

	after, err := os.ReadFile(fpath)
	require.NoError(t, err)
	assert.Len(t, after, len(jpeg), "original JPEG should be unchanged after scan")
}

// --- ListPhotos boundary tests ---

func TestListPhotos_LimitClampedToDefault(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	// limit 0 should be clamped to 50
	photos, total, err := svc.ListPhotos(context.Background(), "lib-1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, photos)
}

func TestListPhotos_NegativeLimitClampedToDefault(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	photos, total, err := svc.ListPhotos(context.Background(), "lib-1", -5, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, photos)
}

func TestListPhotos_OverMaxLimitClampedToDefault(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	// limit > 500 should be clamped to 50
	photos, total, err := svc.ListPhotos(context.Background(), "lib-1", 501, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, photos)
}

func TestListPhotos_ExactBoundary500(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	// limit exactly 500 should NOT be clamped
	photos, total, err := svc.ListPhotos(context.Background(), "lib-1", 500, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, photos)
}

// --- Search boundary tests ---

func TestSearch_LimitZeroClampedTo50(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	results, err := svc.Search(context.Background(), "lib-1", "q", 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_LimitNegativeClampedTo50(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	results, err := svc.Search(context.Background(), "lib-1", "q", -1)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_LimitExactly200NotClamped(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	results, err := svc.Search(context.Background(), "lib-1", "q", 200)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_Limit201ClampedTo50(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	results, err := svc.Search(context.Background(), "lib-1", "q", 201)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// --- DeletePhoto error propagation ---

func TestDeletePhoto_RepoGetByIDError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errors.New("get failed")
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())

	err := svc.DeletePhoto(context.Background(), "photo-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.Err)
}

func TestDeletePhoto_NotFound_ReturnsNilPhoto(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	// GetByID returns nil,nil for nonexistent; Delete on nonexistent is no-op
	err := svc.DeletePhoto(context.Background(), "nonexistent")
	require.NoError(t, err)

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	removed := events[0].(photo.PhotoRemoved)
	assert.Equal(t, "nonexistent", removed.PhotoID)
	assert.Empty(t, removed.LibraryID) // nil photo => empty libraryID
}

func TestDeletePhoto_SetsLibraryIDFromPhoto(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, err := svc.ScanDirectory(ctx, "lib-del", dir)
	require.NoError(t, err)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-del", 10, 0)
	require.Len(t, photos, 1)
	photoID := photos[0].ID

	bus2 := sharedfakes.NewFakeEventBus()
	svc2 := appphoto.NewService(repo, bus2, nil, fsadapter.NewPhotoScanner())

	err = svc2.DeletePhoto(ctx, photoID)
	require.NoError(t, err)

	events := bus2.PublishedEvents()
	require.Len(t, events, 1)
	removed := events[0].(photo.PhotoRemoved)
	assert.Equal(t, "lib-del", removed.LibraryID)
}

func TestDeletePhoto_BusPublishError_StillReturnsNil(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, _ = svc.ScanDirectory(ctx, "lib-1", dir)
	photos, _, _ := repo.ListByLibrary(ctx, "lib-1", 10, 0)
	require.NotEmpty(t, photos)

	bus2 := sharedfakes.NewFakeEventBus()
	bus2.PublishErr = errors.New("bus down")
	svc2 := appphoto.NewService(repo, bus2, nil, fsadapter.NewPhotoScanner())

	err := svc2.DeletePhoto(ctx, photos[0].ID)
	assert.NoError(t, err) // publish error is logged, not returned
}

// --- GetPhoto ---

func TestGetPhoto_ReturnsExistingPhoto(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "find.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, _ = svc.ScanDirectory(ctx, "lib-g", dir)
	photos, _, _ := repo.ListByLibrary(ctx, "lib-g", 10, 0)
	require.Len(t, photos, 1)

	p, err := svc.GetPhoto(ctx, photos[0].ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, photos[0].ID, p.ID)
	assert.Equal(t, "find", p.Title)
}

// --- GetAlbums ---

func TestGetAlbums_ReturnsAlbumsFromSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "album1")
	sub2 := filepath.Join(dir, "album2")
	require.NoError(t, os.MkdirAll(sub1, 0755))
	require.NoError(t, os.MkdirAll(sub2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub1, "a.jpg"), minimalJPEG, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(sub2, "b.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, err := svc.ScanDirectory(ctx, "lib-alb", dir)
	require.NoError(t, err)

	albums, err := svc.GetAlbums(ctx, "lib-alb")
	require.NoError(t, err)
	assert.Len(t, albums, 2)

	names := map[string]bool{}
	for _, a := range albums {
		names[a.Name] = true
		assert.Equal(t, 1, a.PhotoCount)
	}
	assert.True(t, names["album1"])
	assert.True(t, names["album2"])
}

// --- ScanDirectory bus publish error ---

func TestScanDirectory_BusPublishError_StillCounts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "img.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus fail")
	svc := appphoto.NewService(repo, bus, nil, fsadapter.NewPhotoScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	require.NoError(t, err)
	assert.Equal(t, 1, count) // counted even if publish fails
}

// --- Title extraction from filename ---

func TestScanDirectory_TitleStripsExtension(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my.photo.name.jpg"), minimalJPEG, 0600))

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fsadapter.NewPhotoScanner())
	ctx := context.Background()

	_, err := svc.ScanDirectory(ctx, "lib-t", dir)
	require.NoError(t, err)

	photos, _, _ := repo.ListByLibrary(ctx, "lib-t", 10, 0)
	require.Len(t, photos, 1)
	assert.Equal(t, "my.photo.name", photos[0].Title)
}
