package photo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
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
	// keep.jpg must read as UNCHANGED so the walk only marks it seen (no job):
	// a shared fixed ModTime makes idx.Unchanged(path,size,modtime) true.
	keepTime := time.Unix(1_700_000_000, 0)
	if err := repo.Upsert(context.Background(), photo.Photo{
		ID: "keep-1", LibraryID: "lib-1", FilePath: "/photos/a/keep.jpg",
		FileSize: 100, FileHash: "hash-keep-1", ModTime: keepTime,
	}); err != nil {
		t.Fatal(err)
	}
	seedPrunePhoto(t, repo, "gone-1", "/photos/a/gone.jpg")
	// The walk reports the surviving file (seen is non-empty: a healthy scan,
	// not a degraded mount) while gone.jpg is really absent and must be pruned.
	scanner.Files = []photo.ScannedImageFile{
		{Path: "/photos/a/keep.jpg", Size: 100, MimeType: "image/jpeg", ModTime: keepTime},
	}
	scanner.Hashes["/photos/a/keep.jpg"] = "hash-keep-1"

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 1 {
		t.Fatalf("the gone file must be pruned while the surviving one stays, have %d", n)
	}
}

func TestPhotoScanDirectory_DoesNotPruneWhenWalkSawNothing(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	seedPrunePhoto(t, repo, "mount-1", "/photos/a/one.jpg")
	seedPrunePhoto(t, repo, "mount-2", "/photos/a/two.jpg")
	// Walk SUCCEEDS but reports zero files: a present-but-empty mountpoint
	// (dropped export) would otherwise prune the whole catalog. Must be refused
	// - same contract as the music twin and the video orphan pruner.

	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 2 {
		t.Fatalf("a walk that saw NO files while the library is non-empty must never prune (degraded mount), have %d", n)
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
