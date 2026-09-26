package music_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

var folderJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 'f', 'o', 'l', 'd', 'e', 'r'}

func seedFolderAlbum(t *testing.T, repo *fakes.FakeMusicTrackRepository, folder string) {
	t.Helper()
	if _, err := repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: "lib1", Title: "Some Track",
		Artist: "Some Artist", AlbumArtist: "Some Artist", Album: "Some Album",
		FilePath: folder + "/01.flac",
	}); err != nil {
		t.Fatalf("seed track: %v", err)
	}
}

func folderArtPath(dir, folder string) (string, string) {
	sum := sha256.Sum256([]byte(folder + "|"))
	hash := hex.EncodeToString(sum[:])
	return hash, filepath.Join(dir, hash+".jpg")
}

// The owner's cover.jpg beats the external source: copied into the cache, the
// tracks point at it, and the fetcher is never asked.
func TestFetchMissingAlbumArt_UsesTheFolderCover(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("EXTERNAL"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())
	folder := "/m/Some Album"
	seedFolderAlbum(t, repo, folder)
	scanner.FileData[folder+"/01.flac"] = []byte("untagged")
	scanner.FileData[folder+"/Folder.jpg"] = folderJPEG

	n, err := cmd.FetchMissingAlbumArt(context.Background(), "lib1")
	if err != nil || n != 1 {
		t.Fatalf("want (1, nil), got (%d, %v)", n, err)
	}
	if fetcher.calls.Load() != 0 {
		t.Fatalf("the external source must not be asked when the folder has a cover")
	}
	hash, dst := folderArtPath(cmd.AlbumArtDir(), folder)
	if got, _ := os.ReadFile(dst); string(got) != string(folderJPEG) {
		t.Fatalf("cached image = %q, want the folder's bytes", got)
	}
	tr, _ := repo.GetByID(context.Background(), "t1")
	if tr == nil || tr.CoverArtURL != "/api/v1/music/album-art/"+hash {
		t.Fatalf("cover url = %+v", tr)
	}

	// A second pass over an unchanged cover copies nothing and still skips the fetcher.
	n, err = cmd.FetchMissingAlbumArt(context.Background(), "lib1")
	if err != nil || n != 0 || fetcher.calls.Load() != 0 {
		t.Fatalf("second pass: n=%d err=%v calls=%d", n, err, fetcher.calls.Load())
	}
}

// A cover.jpg that is not a JPEG is not a cover: the external source still runs.
func TestFetchMissingAlbumArt_IgnoresAFolderFileThatIsNotAJPEG(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("EXTERNAL"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())
	folder := "/m/Some Album"
	seedFolderAlbum(t, repo, folder)
	scanner.FileData[folder+"/01.flac"] = []byte("untagged")
	scanner.FileData[folder+"/cover.jpg"] = []byte("<html>not an image</html>")

	n, err := cmd.FetchMissingAlbumArt(context.Background(), "lib1")
	if err != nil || n != 1 || fetcher.calls.Load() != 1 {
		t.Fatalf("want the fetcher used once, got n=%d err=%v calls=%d", n, err, fetcher.calls.Load())
	}
}

// Embedded art stays first: a folder cover does not override the file's own picture.
func TestFetchMissingAlbumArt_EmbeddedArtStillWinsOverTheFolderCover(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	cmd.WithAlbumArtFetcher(&fakeFetcher{}, t.TempDir())
	folder := "/m/Some Album"
	embedded := "/api/v1/music/tracks/t1/art"
	if _, err := repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID("lib1"), Title: "Some Track",
		Artist: "Some Artist", AlbumArtist: "Some Artist", Album: "Some Album",
		FilePath: folder + "/01.mp3", CoverArtURL: embedded,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	scanner.FileData[folder+"/cover.jpg"] = folderJPEG

	if _, err := cmd.FetchMissingAlbumArt(context.Background(), "lib1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tr, _ := repo.GetByID(context.Background(), "t1")
	if tr == nil || tr.CoverArtURL != embedded {
		t.Fatalf("embedded art must be kept, got %+v", tr)
	}
}
