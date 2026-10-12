package photo_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/domain/photo"
	photofakes "github.com/eveyproject/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

var imageExts = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".png": "image/png", ".heic": "image/heic",
	".webp": "image/webp", ".gif": "image/gif",
	".tiff": "image/tiff", ".tif": "image/tiff",
	".bmp": "image/bmp",
}

type fakeFile struct {
	relPath string
	data    []byte
	subdir  string // optional subdirectory
}

func scannerFromFiles(dir string, files []fakeFile) *photofakes.FakePhotoScanner {
	s := photofakes.NewFakePhotoScanner()
	now := time.Now()
	for _, f := range files {
		var fullPath string
		if f.subdir != "" {
			fullPath = filepath.Join(dir, f.subdir, f.relPath)
		} else {
			fullPath = filepath.Join(dir, f.relPath)
		}
		ext := strings.ToLower(filepath.Ext(f.relPath))
		mime, ok := imageExts[ext]
		if !ok {
			continue
		}
		s.Files = append(s.Files, photo.ScannedImageFile{
			Path:     fullPath,
			Size:     int64(len(f.data)),
			ModTime:  now,
			MimeType: mime,
		})
		s.FileData[fullPath] = f.data
		h := sha256.Sum256(f.data)
		s.Hashes[fullPath] = fmt.Sprintf("%x", h)
		s.Dimensions[fullPath] = [2]int{1, 1}
	}
	return s
}

var errFake = errors.New("fake error")

// minimalJPEG is a 1x1 white JPEG (no EXIF) valid enough for image.DecodeConfig.
var minimalJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
	0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
	0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
	0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
	0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
	0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
	0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
	0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
	0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
	0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
	0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
	0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD1, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xD9,
}

func TestScanDirectory_NewPhoto(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "photo.jpg", data: minimalJPEG}})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
	photos, total, err := repo.ListByLibrary(context.Background(), "lib-1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(photos) != 1 {
		t.Fatalf("expected 1 photo in repo, got %d", total)
	}
}

func TestScanDirectory_SkipsNonImage(t *testing.T) {
	dir := "/fake/photos"
	// scannerFromFiles only adds image extensions, so PDF is excluded
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "document.pdf", data: []byte("%PDF-1.4")}})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestScanDirectory_MultipleImageTypes(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{
		{relPath: "a.jpg", data: minimalJPEG},
		{relPath: "b.png", data: []byte("png-data")},
		{relPath: "c.txt", data: []byte("text")},
		{relPath: "d.pdf", data: []byte("%PDF")},
	})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	// jpg + png = 2 images (txt and pdf skipped by scannerFromFiles)
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestDeletePhoto(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "photo.jpg", data: minimalJPEG}})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}
	photos, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0)
	if err != nil || len(photos) == 0 {
		t.Fatal("photo not found after scan")
	}
	photoID := photos[0].ID

	if delErr := svc.DeletePhoto(ctx, photoID); delErr != nil {
		t.Fatalf("DeletePhoto failed: %v", delErr)
	}
	_, total, err := repo.ListByLibrary(ctx, "lib-1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("expected 0 photos after delete, got %d", total)
	}
}

func TestScanDirectory_SkipsUnchanged(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "photo.jpg", data: minimalJPEG}})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	count1, err := svc.ScanDirectory(ctx, "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count1 != 1 {
		t.Fatalf("first scan: expected 1, got %d", count1)
	}

	count2, err := svc.ScanDirectory(ctx, "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count2 != 0 {
		t.Fatalf("second scan: expected 0 (unchanged), got %d", count2)
	}
}

func TestListPhotos_Empty(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	photos, total, err := svc.ListPhotos(context.Background(), "lib-1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(photos) != 0 {
		t.Fatalf("expected 0 photos, got %d (total %d)", len(photos), total)
	}
}

func TestListPhotos_Pagination(t *testing.T) {
	dir := "/fake/photos"
	var ff []fakeFile
	for i := range 10 {
		ff = append(ff, fakeFile{relPath: "photo" + string(rune('0'+i)) + ".jpg", data: append(minimalJPEG, byte(i))})
	}
	scanner := scannerFromFiles(dir, ff)

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	photos, total, err := svc.ListPhotos(ctx, "lib-1", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 {
		t.Fatalf("expected total=10, got %d", total)
	}
	if len(photos) != 3 {
		t.Fatalf("expected 3 photos returned, got %d", len(photos))
	}
}

func TestDeletePhoto_PublishesEvent(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "photo.jpg", data: minimalJPEG}})

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}
	photos, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0)
	if err != nil || len(photos) == 0 {
		t.Fatal("photo not found after scan")
	}
	photoID := photos[0].ID

	bus2 := sharedfakes.NewFakeEventBus()
	svc2 := appphoto.NewService(repo, bus2, nil, photofakes.NewFakePhotoScanner())

	if err := svc2.DeletePhoto(ctx, photoID); err != nil {
		t.Fatalf("DeletePhoto failed: %v", err)
	}
	events := bus2.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName() != "photo.removed" {
		t.Errorf("unexpected event name %q", events[0].EventName())
	}
}

func TestGetPhoto_NotFound(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	p, err := svc.GetPhoto(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil photo for nonexistent id")
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	results, err := svc.Search(context.Background(), "lib-1", "nonexistent-query-xyz", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_LimitClamped(t *testing.T) {
	dir := "/fake/photos"
	var ff []fakeFile
	for i := range 5 {
		ff = append(ff, fakeFile{relPath: "photo" + string(rune('0'+i)) + ".jpg", data: append(minimalJPEG, byte(i))})
	}
	scanner := scannerFromFiles(dir, ff)

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	// limit > 200 should be clamped to 50; with only 5 photos we just verify no error
	results, err := svc.Search(ctx, "lib-1", "photo", 300)
	if err != nil {
		t.Fatalf("Search with large limit failed: %v", err)
	}
	// all 5 match "photo" in title (filename without ext)
	if len(results) > 50 {
		t.Fatalf("expected at most 50 results after clamp, got %d", len(results))
	}
}

func TestGetAlbums_Empty(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	albums, err := svc.GetAlbums(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 0 {
		t.Fatalf("expected 0 albums, got %d", len(albums))
	}
}

func TestGetAlbums_ReturnsAlbumsFromRepo(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Title: "a", Album: "vacation"})
	repo.Upsert(ctx, photo.Photo{ID: "p2", LibraryID: "lib1", Title: "b", Album: "vacation"})
	repo.Upsert(ctx, photo.Photo{ID: "p3", LibraryID: "lib1", Title: "c", Album: "work"})
	repo.Upsert(ctx, photo.Photo{ID: "p4", LibraryID: "lib1", Title: "d", Album: ""})

	albums, err := svc.GetAlbums(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}
	m := map[string]photo.Album{}
	for _, a := range albums {
		m[a.Name] = a
	}
	if v, ok := m["vacation"]; !ok {
		t.Error("missing vacation album")
	} else if v.PhotoCount != 2 {
		t.Errorf("vacation PhotoCount = %d, want 2", v.PhotoCount)
	}
	if v, ok := m["work"]; !ok {
		t.Error("missing work album")
	} else if v.PhotoCount != 1 {
		t.Errorf("work PhotoCount = %d, want 1", v.PhotoCount)
	}
}

func TestGetAlbums_DifferentLibrary(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Album: "a1"})
	repo.Upsert(ctx, photo.Photo{ID: "p2", LibraryID: "lib2", Album: "a2"})

	albums, err := svc.GetAlbums(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album, got %d", len(albums))
	}
	if albums[0].Name != "a1" {
		t.Errorf("album Name = %q, want a1", albums[0].Name)
	}
}

func TestGetAlbums_RepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errFake
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	_, err := svc.GetAlbums(context.Background(), "lib1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetPhoto_Found(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Title: "sunset", Album: "vacation"})

	got, err := svc.GetPhoto(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected photo, got nil")
	}
	if got.ID != "p1" {
		t.Errorf("ID = %q, want p1", got.ID)
	}
	if got.Title != "sunset" {
		t.Errorf("Title = %q, want sunset", got.Title)
	}
	if got.LibraryID != "lib1" {
		t.Errorf("LibraryID = %q, want lib1", got.LibraryID)
	}
	if got.Album != "vacation" {
		t.Errorf("Album = %q, want vacation", got.Album)
	}
}

func TestGetPhoto_RepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errFake
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	got, err := svc.GetPhoto(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error")
	}
	if got != nil {
		t.Error("expected nil photo on error")
	}
}

func TestListPhotos_LimitBoundaries(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	// Seed 60 photos
	for i := range 60 {
		repo.Upsert(ctx, photo.Photo{
			ID:        fmt.Sprintf("p%d", i),
			LibraryID: "lib1",
			Title:     fmt.Sprintf("photo%d", i),
		})
	}

	// Limit 0/neg/over-max clamp to default 200 (cap 5000). Photos are
	// lightweight to list, grid uses lazy <img loading>, virtualizer
	// renders on-screen rows only - higher default is fine.
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{"zero clamps to default 200", 0, 60},
		{"negative clamps to default 200", -1, 60},
		{"5001 clamps to default 200", 5001, 60},
		{"500 accepted", 500, 60},
		{"1 accepted", 1, 1},
		{"50 accepted", 50, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			photos, total, err := svc.ListPhotos(ctx, "lib1", tc.limit, 0)
			if err != nil {
				t.Fatal(err)
			}
			if total != 60 {
				t.Errorf("total = %d, want 60", total)
			}
			if len(photos) != tc.wantLimit {
				t.Errorf("len(photos) = %d, want %d", len(photos), tc.wantLimit)
			}
		})
	}
}

func TestListPhotos_RepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errFake
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	_, _, err := svc.ListPhotos(context.Background(), "lib1", 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListPhotos_OffsetBeyondTotal(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1"})

	photos, total, err := svc.ListPhotos(ctx, "lib1", 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(photos) != 0 {
		t.Errorf("expected 0 photos at offset beyond total, got %d", len(photos))
	}
}

func TestListPhotos_MatchesLibrary(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Title: "a"})
	repo.Upsert(ctx, photo.Photo{ID: "p2", LibraryID: "lib2", Title: "b"})

	photos, total, err := svc.ListPhotos(ctx, "lib1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(photos) != 1 {
		t.Fatalf("len = %d, want 1", len(photos))
	}
	if photos[0].ID != "p1" {
		t.Errorf("photo ID = %q, want p1", photos[0].ID)
	}
}

func TestDeletePhoto_GetByIDError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errFake
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())

	err := svc.DeletePhoto(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(bus.PublishedEvents()) != 0 {
		t.Error("no events should be published on GetByID error")
	}
}

func TestDeletePhoto_DeleteRepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1"})
	repo.Err = errFake

	err := svc.DeletePhoto(ctx, "p1")
	if err == nil {
		t.Fatal("expected error from Delete")
	}
	if len(bus.PublishedEvents()) != 0 {
		t.Error("no events on Delete error")
	}
}

func TestDeletePhoto_PublishErrorStillReturnsNil(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errFake
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1"})

	err := svc.DeletePhoto(ctx, "p1")
	if err != nil {
		t.Fatalf("publish failure should not cause DeletePhoto error, got %v", err)
	}
}

func TestDeletePhoto_NilPhoto_EmptyLibraryID(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())

	err := svc.DeletePhoto(context.Background(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	removed, ok := events[0].(photo.PhotoRemoved)
	if !ok {
		t.Fatalf("expected PhotoRemoved, got %T", events[0])
	}
	if removed.LibraryID != "" {
		t.Errorf("LibraryID = %q, want empty for nil photo", removed.LibraryID)
	}
	if removed.PhotoID != "nonexistent" {
		t.Errorf("PhotoID = %q, want nonexistent", removed.PhotoID)
	}
}

func TestDeletePhoto_EventContainsCorrectLibraryID(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "my-lib"})

	if err := svc.DeletePhoto(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	removed := events[0].(photo.PhotoRemoved)
	if removed.LibraryID != "my-lib" {
		t.Errorf("event LibraryID = %q, want my-lib", removed.LibraryID)
	}
	if removed.PhotoID != "p1" {
		t.Errorf("event PhotoID = %q, want p1", removed.PhotoID)
	}
}

func TestSearch_LimitBoundaries(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	for i := range 60 {
		repo.Upsert(ctx, photo.Photo{
			ID:        fmt.Sprintf("p%d", i),
			LibraryID: "lib1",
			Title:     "match",
		})
	}

	tests := []struct {
		name    string
		limit   int
		wantMax int
	}{
		{"zero clamps to 50", 0, 50},
		{"negative clamps to 50", -1, 50},
		{"201 clamps to 50", 201, 50},
		{"200 accepted", 200, 60},
		{"1 accepted", 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, err := svc.Search(ctx, "lib1", "match", tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) > tc.wantMax {
				t.Errorf("len(results) = %d, want <= %d", len(results), tc.wantMax)
			}
		})
	}
}

func TestSearch_MatchesOnlyCorrectLibrary(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Title: "sunset"})
	repo.Upsert(ctx, photo.Photo{ID: "p2", LibraryID: "lib2", Title: "sunset"})

	results, err := svc.Search(ctx, "lib1", "sunset", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "p1" {
		t.Errorf("result ID = %q, want p1", results[0].ID)
	}
}

func TestSearch_RepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errFake
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())

	_, err := svc.Search(context.Background(), "lib1", "q", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSearch_MatchesTitleAndAlbum(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	repo.Upsert(ctx, photo.Photo{ID: "p1", LibraryID: "lib1", Title: "beach", Album: "vacation"})
	repo.Upsert(ctx, photo.Photo{ID: "p2", LibraryID: "lib1", Title: "vacation photo", Album: "misc"})
	repo.Upsert(ctx, photo.Photo{ID: "p3", LibraryID: "lib1", Title: "mountain", Album: "trips"})

	results, err := svc.Search(ctx, "lib1", "vacation", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 matches (title + album), got %d", len(results))
	}
}

func TestScanDirectory_ExifExtracted_NoExif(t *testing.T) {
	dir := "/fake/photos"
	scanner := scannerFromFiles(dir, []fakeFile{{relPath: "noexif.jpg", data: minimalJPEG}})

	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	n, err := svc.ScanDirectory(ctx, "lib-exif", dir)
	if err != nil {
		t.Fatalf("ScanDirectory error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 scanned, got %d", n)
	}
	photos, _, err := repo.ListByLibrary(ctx, "lib-exif", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 {
		t.Fatalf("expected 1 photo in repo, got %d", len(photos))
	}
	p := photos[0]
	if p.TakenAt != nil {
		t.Errorf("expected nil TakenAt for JPEG without EXIF, got %v", p.TakenAt)
	}
	if p.CameraModel != "" {
		t.Errorf("expected empty CameraModel for JPEG without EXIF, got %q", p.CameraModel)
	}
}
