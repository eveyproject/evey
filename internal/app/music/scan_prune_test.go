package music_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/music/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Review 2026-08-26: the scan NEVER pruned - a file deleted from disk kept its
// track in the DB and the UI forever (the generic movie/TV branch prunes, the
// music branch returned before the pruner). These tests pin the whole
// contract: gone files are pruned, and every ambiguous case is NOT.

func seedPruneTrack(t *testing.T, repo music.TrackRepository, id, path string) {
	t.Helper()
	if _, err := repo.Upsert(context.Background(), music.Track{
		ID: id, LibraryID: "lib-1", Title: "T", FilePath: path,
		FileSize: 100, FileHash: "hash-" + id, ModTime: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func trackCount(t *testing.T, repo music.TrackRepository) int {
	t.Helper()
	all, err := repo.ListAllByLibrary(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	return len(all)
}

func TestScanDirectory_PrunesTracksWhoseFileIsGone(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	seedPruneTrack(t, repo, "gone-1", "/lib/a/gone.mp3")
	// The walk sees nothing and the file really is absent (Exists=false).

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 0 {
		t.Fatalf("a track whose file is gone must be pruned, still have %d", n)
	}
}

func TestScanDirectory_DoesNotPruneOnWalkError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	seedPruneTrack(t, repo, "keep-1", "/lib/a/keep.mp3")
	scanner.WalkErr = errors.New("library root vanished")

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	_, _ = cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if n := trackCount(t, repo); n != 1 {
		t.Fatalf("a failed walk saw NOTHING - it must never prune (dead mount = every file 'missing'), have %d", n)
	}
}

func TestScanDirectory_DoesNotPruneWhenFileStillExists(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	seedPruneTrack(t, repo, "hiccup-1", "/lib/a/hiccup.mp3")
	// Walk missed the file (per-file stat error) but it IS on disk.
	scanner.ExistingPaths = map[string]bool{"/lib/a/hiccup.mp3": true}

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 1 {
		t.Fatalf("a file the walk missed but that still exists must NOT be pruned (playlists reference its id), have %d", n)
	}
}

func TestScanDirectory_DoesNotPruneOutsideTheScannedDir(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	seedPruneTrack(t, repo, "other-root", "/lib2/b/other.mp3")

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 1 {
		t.Fatalf("a multi-path library's OTHER root must never be pruned by this dir's scan, have %d", n)
	}
}

func TestScanDirectory_DoesNotPruneAMovedFile(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	seedPruneTrack(t, repo, "moved-1", "/lib/old/song.mp3")
	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/new/song.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.Hashes["/lib/new/song.mp3"] = "hash-moved-1"

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 1 {
		t.Fatalf("a moved file keeps its row (updated in place) - it must not be pruned, have %d", n)
	}
}

// Review 2026-08-26 (producer-consumer family): the notification batcher was
// BUILT to amalgamate MediaItemAdded{Type: Track, SeriesTitle: album} events
// into one "New Album: X (N tracks)" notification (batching.go), but the music
// scan only ever published music.* events that nothing subscribes to - so the
// whole new_album notification pipeline (batching, i18n, push) could never
// fire. The scan must speak the event the consumer listens for, and only for
// NEW tracks - a metadata re-parse of an existing row is not news.
func TestScanDirectory_PublishesMediaItemAddedForNewTracks(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	bus := sharedfakes.NewFakeEventBus()
	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/album/song.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	// No FileData: parse fails, the fallback track is built - the event must
	// fire regardless of tag quality.

	cs := appmusic.NewCommandService(repo, bus, nil, scanner)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}

	var added []library.MediaItemAdded
	for _, ev := range bus.PublishedEvents() {
		if e, ok := ev.(library.MediaItemAdded); ok {
			added = append(added, e)
		}
	}
	if len(added) != 1 {
		t.Fatalf("a NEW track must publish exactly one MediaItemAdded (the event the notification batcher consumes), got %d", len(added))
	}
	if added[0].Type != library.Track {
		t.Fatalf("Type = %q, want %q", added[0].Type, library.Track)
	}

	// Re-scan with a CHANGED file (same path): an update, not an addition -
	// no second MediaItemAdded.
	scanner.Files[0].ModTime = time.Now().Add(time.Minute)
	scanner.Files[0].Size = 101
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib"); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, ev := range bus.PublishedEvents() {
		if _, ok := ev.(library.MediaItemAdded); ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("an UPDATED track must not publish MediaItemAdded again, got %d total", count)
	}
}
