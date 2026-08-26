package photo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	"github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// TestPhoto_KillMutant_SearchLimitBoundary kills CONDITIONALS_BOUNDARY at
// query_service.go:34:25 (`limit > 200`). Mutant `limit >= 200` would clamp
// limit=200 down to 50. Seed 250 matches and verify exactly 200 are returned.
func TestPhoto_KillMutant_SearchLimitBoundary(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	ctx := context.Background()

	for i := range 250 {
		if err := repo.Upsert(ctx, photo.Photo{
			ID:        fmt.Sprintf("p%d", i),
			LibraryID: "lib1",
			Title:     fmt.Sprintf("match-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := svc.Search(ctx, "lib1", "match", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 200 {
		t.Fatalf("limit=200 boundary: got %d results, want 200 (mutant clamped?)", len(results))
	}

	// And limit=201 must clamp to 50 (kills the other side).
	results, err = svc.Search(ctx, "lib1", "match", 201)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 50 {
		t.Fatalf("limit=201 must clamp to 50, got %d", len(results))
	}
}

// TestPhoto_KillMutant_MovedFileDetection targets command_service.go line 75
// (`byPath[f.Path] == nil && hash != ""`) and line 76 (`byHash[hash] != nil`).
// A photo previously at /old/x.jpg with hash H is rescanned as /new/x.jpg with
// same hash. UpdateFilePath must be invoked and no new row inserted. Mutations
// that flip either guard change the observable repo state.
func TestPhoto_KillMutant_MovedFileDetection(t *testing.T) {
	dir := "/fake/photos"
	scanner := photofakes.NewFakePhotoScanner()
	newPath := dir + "/moved.jpg"
	scanner.Files = []photo.ScannedImageFile{{
		Path:     newPath,
		Size:     10,
		ModTime:  time.Now(),
		MimeType: "image/jpeg",
	}}
	scanner.Hashes[newPath] = "hash-abc"
	scanner.FileData[newPath] = []byte{0xFF, 0xD8, 0xFF, 0xD9}

	repo := photofakes.NewFakeRepository()
	ctx := context.Background()
	oldPath := dir + "/original/old.jpg"
	if err := repo.Upsert(ctx, photo.Photo{
		ID:        "p-existing",
		LibraryID: "lib1",
		Title:     "old",
		FilePath:  oldPath,
		FileHash:  "hash-abc",
		FileSize:  10,
		MimeType:  "image/jpeg",
	}); err != nil {
		t.Fatal(err)
	}

	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := svc.ScanDirectory(ctx, "lib1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("moved file must not create new photo, count=%d", count)
	}

	p, err := repo.GetByID(ctx, "p-existing")
	if err != nil || p == nil {
		t.Fatalf("existing photo missing: %v", err)
	}
	if p.FilePath != newPath {
		t.Fatalf("FilePath not updated: got %q want %q", p.FilePath, newPath)
	}

	all, err := repo.ListAllByLibrary(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 photo after move, got %d", len(all))
	}
}

// TestPhoto_KillMutant_EmptyHashDoesNotMove targets command_service.go:75:36
// (`hash != ""`). If mutated to `hash == ""`, an empty-hash scan would attempt
// to move an existing empty-hash photo. We assert that an empty-hash scan of a
// brand-new path inserts a new row rather than mutating an unrelated existing
// empty-hash photo.
func TestPhoto_KillMutant_EmptyHashDoesNotMove(t *testing.T) {
	dir := "/fake/photos"
	scanner := photofakes.NewFakePhotoScanner()
	newPath := dir + "/new.jpg"
	scanner.Files = []photo.ScannedImageFile{{
		Path:     newPath,
		Size:     5,
		ModTime:  time.Now(),
		MimeType: "image/jpeg",
	}}
	// No hash set -> ComputeHash returns "".
	scanner.FileData[newPath] = []byte{0xFF, 0xD8, 0xFF, 0xD9}

	repo := photofakes.NewFakeRepository()
	ctx := context.Background()
	otherPath := dir + "/other.jpg"
	// The other photo's file is still on disk - without this the (newer)
	// prune pass would legitimately delete the row and mask what this test
	// actually targets (the empty-hash move guard).
	scanner.FileData[otherPath] = []byte{0xFF, 0xD8, 0xFF, 0xD9}
	if err := repo.Upsert(ctx, photo.Photo{
		ID:        "p-other",
		LibraryID: "lib1",
		Title:     "other",
		FilePath:  otherPath,
		FileHash:  "", // empty hash
		FileSize:  99,
		MimeType:  "image/jpeg",
	}); err != nil {
		t.Fatal(err)
	}

	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := svc.ScanDirectory(ctx, "lib1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 new photo inserted, got %d", count)
	}

	// p-other must still exist at its original path - not hijacked by the scan.
	p, err := repo.GetByID(ctx, "p-other")
	if err != nil || p == nil {
		t.Fatalf("p-other lost: %v", err)
	}
	if p.FilePath != otherPath {
		t.Fatalf("p-other FilePath hijacked: got %q want %q", p.FilePath, otherPath)
	}
}
