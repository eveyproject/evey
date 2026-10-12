package music

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	musicdom "github.com/eveyproject/evey/internal/domain/music"
	musicfakes "github.com/eveyproject/evey/internal/domain/music/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// An album art fetched before the source-verification era is dropped when the
// source no longer recognises the album, and the folder's tracks lose their
// cover URL so the UI falls back to its placeholder. A verified image (its
// .matched marker present) or an absent image is left alone.
func TestDropUnverifiedAlbumArt(t *testing.T) {
	ctx := context.Background()
	repo := musicfakes.NewFakeMusicTrackRepository()
	for _, id := range []string{"t1", "t2"} {
		if _, err := repo.Upsert(ctx, musicdom.Track{ID: id, LibraryID: "lib", AlbumFolder: "/music/A", Title: id, FilePath: "/music/A/" + id + ".flac", CoverArtURL: "/art/A.jpg"}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, musicfakes.NewFakeMusicScanner())
	dir := t.TempDir()
	coverOf := func(id string) string {
		tr, err := repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.CoverArtURL
	}

	// Nothing cached: nothing to judge, the URL stays.
	svc.dropUnverifiedAlbumArt(ctx, "lib", "/music/A", filepath.Join(dir, "missing.jpg"))
	if coverOf("t1") != "/art/A.jpg" {
		t.Fatal("no cached image: the tracks keep their URL")
	}

	// Verified image: kept, URL kept.
	verified := filepath.Join(dir, "verified.jpg")
	if err := os.WriteFile(verified, []byte("jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verifiedMarkerPath(verified), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	svc.dropUnverifiedAlbumArt(ctx, "lib", "/music/A", verified)
	if _, err := os.Stat(verified); err != nil {
		t.Fatal("a verified image must not be removed")
	}
	if coverOf("t1") != "/art/A.jpg" {
		t.Fatal("a verified image keeps the tracks' URL")
	}

	// Unverified image: removed, and every track of the folder cleared.
	unverified := filepath.Join(dir, "guess.jpg")
	if err := os.WriteFile(unverified, []byte("jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.dropUnverifiedAlbumArt(ctx, "lib", "/music/A", unverified)
	if _, err := os.Stat(unverified); !os.IsNotExist(err) {
		t.Fatal("an unverified image the source disowns must be removed")
	}
	if coverOf("t1") != "" || coverOf("t2") != "" {
		t.Fatalf("the folder's tracks must fall back to the placeholder: %q %q", coverOf("t1"), coverOf("t2"))
	}
}
