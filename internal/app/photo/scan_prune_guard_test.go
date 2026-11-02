package photo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/app/prunehold"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// Union mount missing one of three branches: same guard as the video prune.
func TestPhotoScanDirectory_BranchMissing_NothingPruned(t *testing.T) {
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	mod := time.Unix(1_700_000_000, 0)
	for i := range 60 {
		id := fmt.Sprintf("p-%d", i)
		path := fmt.Sprintf("/pool/a/%02d.jpg", i)
		if err := repo.Upsert(context.Background(), photo.Photo{
			ID: id, LibraryID: "lib-1", FilePath: path, FileSize: 100, FileHash: "hash-" + id, ModTime: mod,
		}); err != nil {
			t.Fatal(err)
		}
		if i < 40 {
			scanner.Files = append(scanner.Files, photo.ScannedImageFile{Path: path, Size: 100, MimeType: "image/jpeg", ModTime: mod})
			scanner.Hashes[path] = "hash-" + id
		}
	}
	bus := sharedfakes.NewFakeEventBus()
	cs := appphoto.NewCommandService(repo, bus, nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 60 {
		t.Fatalf("a third of the root gone must hold the prune, have %d", n)
	}
	held := false
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(photo.PhotoRemoved); ok {
			t.Fatal("a removal event went out")
		}
		if s, ok := e.(library.PruneSuspended); ok && s.Root == "/pool" && s.Missing == 20 {
			held = true
		}
	}
	if !held {
		t.Fatal("admins were not paged")
	}
}

// Four folders under one root, a third of each on the lost branch; walkFolder
// makes the walk report the present files of one folder, walkAll of the root.
func seedFoldersBranchLoss(t *testing.T) (*fakes.FakeRepository, *fakes.FakePhotoScanner, *libfakes.FakeLibraryQueryRepo, func(folder int), func()) {
	t.Helper()
	repo := fakes.NewFakeRepository()
	scanner := fakes.NewFakePhotoScanner()
	scanner.ExistingPaths = map[string]bool{}
	mod := time.Unix(1_700_000_000, 0)
	present := map[int][]photo.ScannedImageFile{}
	for f := range 4 {
		for i := range 15 {
			id := fmt.Sprintf("p-%d-%d", f, i)
			path := fmt.Sprintf("/pool/f%d/%02d.jpg", f, i)
			if err := repo.Upsert(context.Background(), photo.Photo{
				ID: id, LibraryID: "lib-1", FilePath: path, FileSize: 100, FileHash: "hash-" + id, ModTime: mod,
			}); err != nil {
				t.Fatal(err)
			}
			scanner.Hashes[path] = "hash-" + id
			if i < 10 {
				scanner.ExistingPaths[path] = true
				present[f] = append(present[f], photo.ScannedImageFile{Path: path, Size: 100, MimeType: "image/jpeg", ModTime: mod})
			}
		}
	}
	libs := libfakes.NewFakeLibraryQueryRepo()
	libs.Seed(&library.Library{ID: "lib-1", Type: "photo", Paths: []string{"/pool"}})
	walkFolder := func(f int) { scanner.Files = append([]photo.ScannedImageFile(nil), present[f]...) }
	walkAll := func() {
		scanner.Files = nil
		for f := range 4 {
			scanner.Files = append(scanner.Files, present[f]...)
		}
	}
	return repo, scanner, libs, walkFolder, walkAll
}

func TestPhotoScanDirectory_ConfirmedSnapshotPrunesThatSet(t *testing.T) {
	repo, scanner, libs, _, walkAll := seedFoldersBranchLoss(t)
	hold := prunehold.New(nil)
	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), libs, scanner).WithPruneHold(hold)
	walkAll()
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	held := hold.Suspensions("lib-1")
	if len(held) != 1 {
		t.Fatalf("want /pool held, got %+v", held)
	}
	if err := hold.Confirm("lib-1", "/pool", "not-the-snapshot"); err == nil {
		t.Fatal("a snapshot the page never showed must be refused")
	}
	if err := hold.Confirm("lib-1", "/pool", held[0].Snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	if n := photoCount(t, repo); n != 40 {
		t.Fatalf("the confirmed set must go, have %d photos", n)
	}
	if len(hold.Suspensions("lib-1")) != 0 {
		t.Fatal("hold not cleared after the confirmed prune")
	}
}

func TestPhotoScanDirectory_SubfolderScans_NothingPruned(t *testing.T) {
	repo, scanner, libs, walkFolder, _ := seedFoldersBranchLoss(t)
	hold := prunehold.New(nil)
	cs := appphoto.NewCommandService(repo, sharedfakes.NewFakeEventBus(), libs, scanner).WithPruneHold(hold)
	for f := range 4 {
		walkFolder(f)
		if _, err := cs.ScanDirectory(context.Background(), "lib-1", fmt.Sprintf("/pool/f%d", f)); err != nil {
			t.Fatal(err)
		}
	}
	if n := photoCount(t, repo); n != 60 {
		t.Fatalf("folder scans must not prune a lost branch one folder at a time, have %d", n)
	}
	held := hold.Suspensions("lib-1")
	if len(held) != 1 || held[0].Root != "/pool" || held[0].Missing != 20 {
		t.Fatalf("want the root held over its whole share, got %+v", held)
	}
}
