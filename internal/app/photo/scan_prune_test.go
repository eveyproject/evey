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

// batchSpyRepo counts how the prune reaches the store (see the music twin).
type batchSpyRepo struct {
	*fakes.FakeRepository
	singleDeletes int
	batchCalls    int
	batchErr      error
}

func (r *batchSpyRepo) Delete(ctx context.Context, id string) error {
	r.singleDeletes++
	return r.FakeRepository.Delete(ctx, id)
}

func (r *batchSpyRepo) DeleteByIDs(ctx context.Context, ids []string) error {
	r.batchCalls++
	if r.batchErr != nil {
		return r.batchErr
	}
	return r.FakeRepository.DeleteByIDs(ctx, ids)
}

func prunePhotoScan(t *testing.T, spy *batchSpyRepo) *sharedfakes.FakeEventBus {
	t.Helper()
	scanner := fakes.NewFakePhotoScanner()
	keepTime := time.Unix(1_700_000_000, 0)
	if err := spy.Upsert(context.Background(), photo.Photo{
		ID: "keep-1", LibraryID: "lib-1", FilePath: "/photos/a/keep.jpg",
		FileSize: 100, FileHash: "hash-keep-1", ModTime: keepTime,
	}); err != nil {
		t.Fatal(err)
	}
	seedPrunePhoto(t, spy.FakeRepository, "gone-1", "/photos/a/gone1.jpg")
	seedPrunePhoto(t, spy.FakeRepository, "gone-2", "/photos/a/gone2.jpg")
	scanner.Files = []photo.ScannedImageFile{
		{Path: "/photos/a/keep.jpg", Size: 100, MimeType: "image/jpeg", ModTime: keepTime},
	}
	scanner.Hashes["/photos/a/keep.jpg"] = "hash-keep-1"
	bus := sharedfakes.NewFakeEventBus()
	cs := appphoto.NewCommandService(spy, bus, nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/photos"); err != nil {
		t.Fatal(err)
	}
	return bus
}

func removedPhotoEvents(bus *sharedfakes.FakeEventBus) []photo.PhotoRemoved {
	var out []photo.PhotoRemoved
	for _, ev := range bus.PublishedEvents() {
		if e, ok := ev.(photo.PhotoRemoved); ok {
			out = append(out, e)
		}
	}
	return out
}

func TestPhotoScanDirectory_PrunesGoneFilesInOneBatch(t *testing.T) {
	spy := &batchSpyRepo{FakeRepository: fakes.NewFakeRepository()}
	bus := prunePhotoScan(t, spy)
	if spy.batchCalls != 1 || spy.singleDeletes != 0 {
		t.Fatalf("prune must be ONE batch delete and no per-photo delete, got batch=%d single=%d", spy.batchCalls, spy.singleDeletes)
	}
	if n := photoCount(t, spy.FakeRepository); n != 1 {
		t.Fatalf("both gone photos must be pruned, have %d rows", n)
	}
	evs := removedPhotoEvents(bus)
	if len(evs) != 2 {
		t.Fatalf("each pruned photo must emit PhotoRemoved, got %d", len(evs))
	}
	for _, e := range evs {
		if e.LibraryID != "lib-1" {
			t.Fatalf("PhotoRemoved must carry the library id, got %q", e.LibraryID)
		}
	}
}

func TestPhotoScanDirectory_FailedBatchPruneEmitsNothing(t *testing.T) {
	spy := &batchSpyRepo{FakeRepository: fakes.NewFakeRepository(), batchErr: errors.New("db down")}
	bus := prunePhotoScan(t, spy)
	if evs := removedPhotoEvents(bus); len(evs) != 0 {
		t.Fatalf("rows survived the failed batch: no PhotoRemoved may be emitted, got %d", len(evs))
	}
	if n := photoCount(t, spy.FakeRepository); n != 3 {
		t.Fatalf("rows must remain after a failed batch, have %d", n)
	}
}
