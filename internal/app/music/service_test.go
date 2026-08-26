package music_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/music/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

var audioExts = map[string]string{
	".mp3": "audio/mpeg", ".flac": "audio/flac",
	".m4a": "audio/mp4", ".ogg": "audio/ogg",
	".opus": "audio/ogg", ".wav": "audio/wav",
	".aac": "audio/aac",
}

type fakeAudioFile struct {
	relPath string
	data    []byte
}

func musicScannerFromFiles(dir string, files []fakeAudioFile) *fakes.FakeMusicScanner {
	s := fakes.NewFakeMusicScanner()
	now := time.Now()
	for _, f := range files {
		fullPath := filepath.Join(dir, f.relPath)
		ext := strings.ToLower(filepath.Ext(f.relPath))
		mime, ok := audioExts[ext]
		if !ok {
			continue
		}
		s.Files = append(s.Files, music.ScannedAudioFile{
			Path:     fullPath,
			Size:     int64(len(f.data)),
			ModTime:  now,
			MimeType: mime,
		})
		s.FileData[fullPath] = f.data
		h := sha256.Sum256(f.data)
		s.Hashes[fullPath] = fmt.Sprintf("%x", h)
	}
	return s
}

func TestScanDirectory_NewTrack(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "song.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
	tracks, total, err := repo.ListByLibrary(context.Background(), "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(tracks) != 1 {
		t.Fatalf("expected 1 track in repo, got %d", total)
	}
}

func TestScanDirectory_SkipsNonAudio(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "readme.txt", data: []byte("not audio")}})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestScanDirectory_SkipsUnchanged(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "track.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count1, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count1 != 1 {
		t.Fatalf("first scan: expected 1, got %d", count1)
	}

	count2, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count2 != 0 {
		t.Fatalf("second scan: expected 0 (unchanged), got %d", count2)
	}

	_, total, err := repo.ListByLibrary(context.Background(), "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected 1 track after two scans, got %d", total)
	}
}

func TestScanDirectory_MultipleAudioExtensions(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{
		{relPath: "a.mp3", data: []byte("mp3data")},
		{relPath: "b.flac", data: []byte("flacdata")},
		{relPath: "c.ogg", data: []byte("oggdata")},
		{relPath: "d.wav", data: []byte("wavdata")},
		{relPath: "skip.pdf", data: []byte("pdfdata")},
	})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("expected 4 audio tracks, got %d", count)
	}
}

func TestDeleteTrack(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "song.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}
	tracks, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0, "")
	if err != nil || len(tracks) == 0 {
		t.Fatal("track not found after scan")
	}
	trackID := tracks[0].ID

	if delErr := svc.DeleteTrack(ctx, trackID); delErr != nil {
		t.Fatalf("DeleteTrack failed: %v", delErr)
	}
	_, total, err := repo.ListByLibrary(ctx, "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("expected 0 tracks after delete, got %d", total)
	}
}

func TestGetTrackArt_NotFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	_, _, err := svc.GetTrackArt(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent track, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestGetAlbums_Empty(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	albums, err := svc.GetAlbums(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 0 {
		t.Fatalf("expected 0 albums, got %d", len(albums))
	}
}

func TestGetAlbums_GroupsByAlbum(t *testing.T) {
	// Album identity is now the parent folder. Put the two "Album-A" tracks
	// in one directory and the "Album-B" track in another so we get two
	// distinct albums. Before folder-identity this test relied on distinct
	// album tags in the same folder, which now merge (by design).
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{
		{relPath: "Album-A/t1.mp3", data: []byte("ID3fake1")},
		{relPath: "Album-A/t2.mp3", data: []byte("ID3fake2")},
		{relPath: "Album-B/t3.mp3", data: []byte("ID3fake3")},
	})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	tracks, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// Overwrite tags so both Album-A tracks agree, Album-B stands alone.
	// The grouping is by folder, tag value just drives display.
	for i := range tracks {
		if strings.Contains(tracks[i].FilePath, "Album-A") {
			tracks[i].Album = "Album-A"
		} else {
			tracks[i].Album = "Album-B"
		}
		if _, upsertErr := repo.Upsert(ctx, tracks[i]); upsertErr != nil {
			t.Fatal(upsertErr)
		}
	}

	albums, err := svc.GetAlbums(ctx, "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}
}

func TestGetArtists_Empty(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	artists, err := svc.GetArtists(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 0 {
		t.Fatalf("expected 0 artists, got %d", len(artists))
	}
}

func TestListTracks_Pagination(t *testing.T) {
	dir := "/fake/music"
	var ff []fakeAudioFile
	for i := range 10 {
		ff = append(ff, fakeAudioFile{relPath: "track" + string(rune('0'+i)) + ".mp3", data: append([]byte("ID3fake"), byte(i))})
	}
	scanner := musicScannerFromFiles(dir, ff)

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	tracks, total, err := svc.ListTracks(ctx, "lib-1", 5, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 {
		t.Fatalf("expected total=10, got %d", total)
	}
	if len(tracks) != 5 {
		t.Fatalf("expected 5 tracks returned, got %d", len(tracks))
	}
}

func TestListTracksByAlbumID(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, nil)
	ctx := context.Background()

	// Two distinct albums; folder is the identity (AlbumFolder derived from path).
	for _, p := range []string{"/m/AlbumX/01.mp3", "/m/AlbumX/02.mp3"} {
		if _, err := repo.Upsert(ctx, music.Track{LibraryID: "lib-1", Album: "X", Artist: "A", FilePath: p}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Upsert(ctx, music.Track{LibraryID: "lib-1", Album: "Y", Artist: "A", FilePath: "/m/AlbumY/01.mp3"}); err != nil {
		t.Fatal(err)
	}

	albumXID := music.AlbumID("/m/AlbumX")
	tracks, err := svc.ListTracksByAlbumID(ctx, "lib-1", albumXID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks for AlbumX, got %d", len(tracks))
	}
	for _, tr := range tracks {
		if tr.Album != "X" {
			t.Fatalf("got a non-AlbumX track: %+v", tr)
		}
	}

	// Unknown id resolves to an empty result, not an error.
	none, err := svc.ListTracksByAlbumID(ctx, "lib-1", "deadbeefdeadbeef")
	if err != nil {
		t.Fatalf("unknown album id should not error: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 tracks for unknown id, got %d", len(none))
	}
}

func TestListTracks_EmptySearch(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{
		{relPath: "a.mp3", data: []byte("ID3fakeA")},
		{relPath: "b.mp3", data: []byte("ID3fakeB")},
		{relPath: "c.mp3", data: []byte("ID3fakeC")},
	})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	tracks, total, err := svc.ListTracks(ctx, "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(tracks) != 3 {
		t.Fatalf("expected 3 tracks with empty search, got %d (total %d)", len(tracks), total)
	}
}

func TestGetTrack_NotFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	track, err := svc.GetTrack(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Fatal("expected nil track for nonexistent id")
	}
}

func TestDeleteTrack_PublishesEvent(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "song.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}
	tracks, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0, "")
	if err != nil || len(tracks) == 0 {
		t.Fatal("track not found after scan")
	}
	// clear scan events
	trackID := tracks[0].ID
	bus2 := sharedfakes.NewFakeEventBus()
	svc2 := appmusic.NewService(repo, bus2, nil, fakes.NewFakeMusicScanner())

	if err := svc2.DeleteTrack(ctx, trackID); err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}
	events := bus2.PublishedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventName() != "music.track.deleted" {
		t.Errorf("unexpected event name %q", events[0].EventName())
	}
	if events[1].EventName() != "music.track.removed" {
		t.Errorf("unexpected event name %q", events[1].EventName())
	}
}

func TestListTracks_SearchFilter(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{
		{relPath: "beethoven.mp3", data: []byte("dataB")},
		{relPath: "mozart.mp3", data: []byte("dataM")},
	})

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	ctx := context.Background()

	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	// search by filename (title falls back to basename on parse failure)
	tracks, total, err := svc.ListTracks(ctx, "lib-1", 50, 0, "beethoven")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(tracks) != 1 {
		t.Fatalf("expected 1 result for 'beethoven', got %d (total %d)", len(tracks), total)
	}
}

func TestScanDirectory_EmptyDir(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-1", "/fake/empty")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 from empty dir, got %d", count)
	}
}

func TestScanDirectory_LibraryNotFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo, fakes.NewFakeMusicScanner())

	_, err := svc.ScanDirectory(context.Background(), "unknown-lib", "/fake/dir")
	if err == nil {
		t.Fatal("expected error for unknown library, got nil")
	}
}

func TestScanDirectory_LibraryFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	libRepo.Seed(&library.Library{ID: "lib-known"})
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo, fakes.NewFakeMusicScanner())

	count, err := svc.ScanDirectory(context.Background(), "lib-known", "/fake/empty")
	if err != nil {
		t.Fatalf("unexpected error with known library: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tracks from empty dir, got %d", count)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	results, err := svc.Search(context.Background(), "lib-1", "nonexistent-query-xyz", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_ReturnsMatches(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	if _, err := repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", Title: "Symphony No.9", Artist: "Beethoven", FilePath: "/a/t1.mp3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-1", Title: "Piano Sonata", Artist: "Mozart", FilePath: "/a/t2.mp3"}); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search(ctx, "lib-1", "beethoven", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearch_LimitApplied(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	for i := range 10 {
		tr := music.Track{
			ID:        "t" + string(rune('0'+i)),
			LibraryID: "lib-1",
			Title:     "track",
			Artist:    "artist",
			FilePath:  "/a/t" + string(rune('0'+i)) + ".mp3",
		}
		if _, err := repo.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	results, err := svc.Search(ctx, "lib-1", "track", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 3 {
		t.Fatalf("expected at most 3 results with limit=3, got %d", len(results))
	}
}

func TestListTracks_LimitLarge(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	// insert 3 tracks directly
	for i := range 3 {
		tr := music.Track{
			ID:        "t" + string(rune('0'+i)),
			LibraryID: "lib-1",
			Title:     "track",
			FilePath:  "/a/t" + string(rune('0'+i)) + ".mp3",
		}
		if _, err := repo.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	// pass a very large limit - service passes it through to repo, repo returns all 3
	tracks, total, err := svc.ListTracks(ctx, "lib-1", 1000, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	// with limit 1000 and 3 tracks, should get all 3
	if len(tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(tracks))
	}
}

func TestGetAlbums_MultipleLibraries(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-A", Title: "Track1", Album: "AlbumX", FilePath: "/a/AlbumX/t1.mp3"})
	_, _ = repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-A", Title: "Track2", Album: "AlbumX", FilePath: "/a/AlbumX/t2.mp3"})
	// Distinct folders give distinct albums under folder-identity; two tracks
	// with different album tags sharing a folder would merge (by design).
	_, _ = repo.Upsert(ctx, music.Track{ID: "t3", LibraryID: "lib-B", Title: "Track3", Album: "AlbumY", FilePath: "/b/AlbumY/t3.mp3"})
	_, _ = repo.Upsert(ctx, music.Track{ID: "t4", LibraryID: "lib-B", Title: "Track4", Album: "AlbumZ", FilePath: "/b/AlbumZ/t4.mp3"})

	albumsA, err := svc.GetAlbums(ctx, "lib-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(albumsA) != 1 {
		t.Fatalf("lib-A: expected 1 album, got %d", len(albumsA))
	}

	albumsB, err := svc.GetAlbums(ctx, "lib-B")
	if err != nil {
		t.Fatal(err)
	}
	if len(albumsB) != 2 {
		t.Fatalf("lib-B: expected 2 albums, got %d", len(albumsB))
	}
}

func TestDeleteTrack_NotFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	// deleting a non-existent track: GetByID returns nil (no error), Delete is a no-op
	err := svc.DeleteTrack(context.Background(), "nonexistent-id")
	if err != nil {
		t.Fatalf("expected no error deleting nonexistent track, got: %v", err)
	}
}

func TestScanDirectory_NonExistentDir(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	// WalkDir on non-existent dir: callback receives the error and returns nil (skip),
	// so WalkDir itself returns nil but count stays 0
	count, err := svc.ScanDirectory(context.Background(), "lib-1", "/tmp/nonexistent-dir-xyz-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestScanDirectory_PublishesTrackScannedEvent(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "song.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	events := bus.PublishedEvents()
	// 3 events since 2026-08-26: MediaItemAdded (the one the notification
	// batcher consumes - fires FIRST, for new tracks only) + the music-domain
	// pair TrackScanned/TrackAdded.
	if len(events) != 3 {
		t.Fatalf("expected 3 events (MediaItemAdded + TrackScanned + TrackAdded), got %d", len(events))
	}
	if _, ok := events[0].(library.MediaItemAdded); !ok {
		t.Fatalf("first event must be MediaItemAdded (the notification bridge), got %q", events[0].EventName())
	}
	if events[1].EventName() != "music.track.scanned" {
		t.Errorf("expected event name music.track.scanned, got %q", events[1].EventName())
	}
	scanned, ok := events[1].(music.TrackScanned)
	if !ok {
		t.Fatal("event is not TrackScanned")
	}
	if scanned.LibraryID != "lib-1" {
		t.Errorf("expected LibraryID lib-1, got %q", scanned.LibraryID)
	}
	if scanned.Title == "" {
		t.Error("expected non-empty Title in TrackScanned")
	}
	added, ok := events[2].(music.TrackAdded)
	if !ok {
		t.Fatal("event is not TrackAdded")
	}
	if added.LibraryID != "lib-1" {
		t.Errorf("expected LibraryID lib-1, got %q", added.LibraryID)
	}
}

func TestScanDirectory_UpsertError(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "song.mp3", data: []byte("ID3fake")}})

	repo := fakes.NewFakeMusicTrackRepository()
	repo.UpsertErr = fmt.Errorf("db down")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	// upsert error is logged but skipped, count stays 0
	if count != 0 {
		t.Fatalf("expected 0 (upsert failed), got %d", count)
	}
}

func TestScanDirectory_TrackFieldsCorrect(t *testing.T) {
	dir := "/fake/music"
	scanner := musicScannerFromFiles(dir, []fakeAudioFile{{relPath: "mysong.flac", data: []byte("fLaC")}})
	songPath := filepath.Join(dir, "mysong.flac")

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	count, err := svc.ScanDirectory(context.Background(), "lib-42", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	tracks, _, _ := repo.ListByLibrary(context.Background(), "lib-42", 50, 0, "")
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.LibraryID != "lib-42" {
		t.Errorf("LibraryID = %q, want lib-42", tr.LibraryID)
	}
	// Extension stripped: the filename is a legitimate fallback title, the
	// ".flac" on the end never is (prod 2026-08-26).
	if tr.Title != "mysong" {
		t.Errorf("Title = %q, want mysong", tr.Title)
	}
	if tr.MimeType != "audio/flac" {
		t.Errorf("MimeType = %q, want audio/flac", tr.MimeType)
	}
	if tr.FilePath != songPath {
		t.Errorf("FilePath = %q, want %q", tr.FilePath, songPath)
	}
	if tr.ID == "" {
		t.Error("expected non-empty track ID")
	}
}

func TestGetTrack_ReturnsData(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	want := music.Track{
		ID: "trk-1", LibraryID: "lib-1", Title: "Test Song",
		Artist: "Test Artist", Album: "Test Album", MimeType: "audio/mpeg",
		FilePath: "/a/trk-1.mp3", FileSize: 1024,
	}
	if _, err := repo.Upsert(ctx, want); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetTrack(ctx, "trk-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected non-nil track")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Artist != want.Artist {
		t.Errorf("Artist = %q, want %q", got.Artist, want.Artist)
	}
	if got.Album != want.Album {
		t.Errorf("Album = %q, want %q", got.Album, want.Album)
	}
	if got.MimeType != want.MimeType {
		t.Errorf("MimeType = %q, want %q", got.MimeType, want.MimeType)
	}
}

func TestGetTrack_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, err := svc.GetTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDeleteTrack_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	err := svc.DeleteTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDeleteTrack_EventContainsCorrectFields(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()

	_, _ = repo.Upsert(ctx, music.Track{
		ID: "trk-99", LibraryID: "lib-7", Title: "Song",
		FilePath: "/a/song.mp3",
	})

	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, fakes.NewFakeMusicScanner())

	if err := svc.DeleteTrack(ctx, "trk-99"); err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	deleted, ok := events[0].(music.TrackDeleted)
	if !ok {
		t.Fatal("event is not TrackDeleted")
	}
	if deleted.TrackID != "trk-99" {
		t.Errorf("TrackID = %q, want trk-99", deleted.TrackID)
	}
	if deleted.LibraryID != "lib-7" {
		t.Errorf("LibraryID = %q, want lib-7", deleted.LibraryID)
	}
	if deleted.OccurredAt.IsZero() {
		t.Error("expected non-zero OccurredAt")
	}
	removed, ok := events[1].(music.TrackRemoved)
	if !ok {
		t.Fatal("event is not TrackRemoved")
	}
	if removed.TrackID != "trk-99" {
		t.Errorf("TrackID = %q, want trk-99", removed.TrackID)
	}
	if removed.LibraryID != "lib-7" {
		t.Errorf("LibraryID = %q, want lib-7", removed.LibraryID)
	}
}

func TestStreamTrack_ReturnsPathAndMime(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()

	_, _ = repo.Upsert(ctx, music.Track{
		ID: "trk-s1", LibraryID: "lib-1", Title: "Stream Me",
		FilePath: "/music/stream.flac", MimeType: "audio/flac",
	})

	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	path, mime, err := svc.StreamTrack(ctx, "trk-s1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/music/stream.flac" {
		t.Errorf("path = %q, want /music/stream.flac", path)
	}
	if mime != "audio/flac" {
		t.Errorf("mime = %q, want audio/flac", mime)
	}
}

func TestStreamTrack_NotFound(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, _, err := svc.StreamTrack(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent track")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestStreamTrack_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, _, err := svc.StreamTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestListTracks_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, _, err := svc.ListTracks(context.Background(), "lib-1", 50, 0, "")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetAlbums_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, err := svc.GetAlbums(context.Background(), "lib-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetArtists_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, err := svc.GetArtists(context.Background(), "lib-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestSearch_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	_, err := svc.Search(context.Background(), "lib-1", "query", 50)
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetArtists_ReturnsData(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", Title: "S1", Artist: "ArtistA", AlbumArtist: "ArtistA", Album: "AlbumX", FilePath: "/a/t1.mp3"})
	_, _ = repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-1", Title: "S2", Artist: "ArtistA", AlbumArtist: "ArtistA", Album: "AlbumY", FilePath: "/a/t2.mp3"})
	_, _ = repo.Upsert(ctx, music.Track{ID: "t3", LibraryID: "lib-1", Title: "S3", Artist: "ArtistB", AlbumArtist: "ArtistB", Album: "AlbumZ", FilePath: "/a/t3.mp3"})

	artists, err := svc.GetArtists(ctx, "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 2 {
		t.Fatalf("expected 2 artists, got %d", len(artists))
	}
	for _, a := range artists {
		if a.Name == "ArtistA" {
			if a.TrackCount != 2 {
				t.Errorf("ArtistA TrackCount = %d, want 2", a.TrackCount)
			}
			if a.AlbumCount != 2 {
				t.Errorf("ArtistA AlbumCount = %d, want 2", a.AlbumCount)
			}
		}
	}
}

func TestScanDirectory_CancelledContext(t *testing.T) {
	dir := "/fake/music"
	var ff []fakeAudioFile
	for i := range 5 {
		ff = append(ff, fakeAudioFile{relPath: "t" + string(rune('0'+i)) + ".mp3", data: append([]byte("ID3fake"), byte(i))})
	}
	scanner := musicScannerFromFiles(dir, ff)

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := svc.ScanDirectory(ctx, "lib-1", dir)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}
