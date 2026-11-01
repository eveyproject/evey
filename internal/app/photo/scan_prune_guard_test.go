package photo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/domain/library"
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
