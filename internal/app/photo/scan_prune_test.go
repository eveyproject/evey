package photo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Same contract as the music scan's prune tests (scan_prune_test.go in
// app/music): a photo whose file is gone is pruned off a COMPLETED walk, and
// every ambiguous case - failed walk, walk hiccup with the file still on
// disk, another library root, a moved file - is left alone.

func seedPrunePhoto(t *testing.T, repo *fakes.FakeRepository, id, path string) {
	t.Helper()
	if err := repo.Upsert(context.Background(), photo.Photo{
		ID: id, LibraryID: "lib-1", FilePath: path,
		FileSize: 100, FileHash: "hash-" + id, ModTime: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func photoCount(t *testing.T, repo *fakes.FakeRepository) int {
	t.Helper()
	all, err := repo.ListAllByLibrary(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	return len(all)
}

func TestPhotoScanDirectory_PrunesGoneFiles(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "gone-1", "/photos/a/gone.jpg")

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 0 {
		t.Fatalf("a photo whose file is gone must be pruned, still have %d", n)
	}
}

func TestPhotoScanDirectory_DoesNotPruneOnWalkError(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "keep-1", "/photos/a/keep.jpg")
	scanner.WalkErr = errors.New("library root vanished")

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	_, _ = cs.ScanDirectory(context.Background(), "lib-1", "/photos")
	if n := photoCount(t, repo); n != 1 {
		t.Fatalf("a failed walk must never prune, have %d", n)
	}
}

func TestPhotoScanDirectory_DoesNotPruneWhenFileStillExists(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "hiccup-1", "/photos/a/hiccup.jpg")
	scanner.ExistingPaths = map[string]bool{"/photos/a/hiccup.jpg": true}

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 1 {
		t.Fatalf("a file the walk missed but that still exists must NOT be pruned, have %d", n)
	}
}

func TestPhotoScanDirectory_DoesNotPruneOutsideTheScannedDir(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "other-root", "/photos2/b/other.jpg")

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 1 {
		t.Fatalf("another root of a multi-path library must never be pruned, have %d", n)
	}
}

func TestPhotoScanDirectory_DoesNotPruneAMovedFile(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "moved-1", "/photos/old/pic.jpg")
	scanner.Files = []photo.ScannedImageFile{
		{Path: "/photos/new/pic.jpg", Size: 100, MimeType: "image/jpeg", ModTime: time.Now()},
	}
	scanner.Hashes["/photos/new/pic.jpg"] = "hash-moved-1"

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 1 {
		t.Fatalf("a moved file keeps its row - it must not be pruned, have %d", n)
	}
}
