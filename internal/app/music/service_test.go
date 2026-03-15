package music_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	appmusic "github.com/gravity-zero/evey/internal/app/music"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/music"
	"github.com/gravity-zero/evey/internal/domain/music/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
)

func TestScanDirectory_NewTrack(t *testing.T) {
	dir := t.TempDir()
	// minimal MP3-like file: service will fail tag parse, but still upsert a minimal track
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

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
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not audio"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestScanDirectory_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(filePath, []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	// first scan
	if _, err := svc.ScanDirectory(context.Background(), "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	// second scan — same file, should upsert not duplicate
	if _, err := svc.ScanDirectory(context.Background(), "lib-1", dir); err != nil {
		t.Fatal(err)
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
	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.flac", "c.ogg", "d.wav", "skip.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("expected 4 audio tracks, got %d", count)
	}
}

func TestDeleteTrack(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	albums, err := svc.GetAlbums(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 0 {
		t.Fatalf("expected 0 albums, got %d", len(albums))
	}
}

func TestGetAlbums_GroupsByAlbum(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	dir := t.TempDir()
	for _, name := range []string{"t1.mp3", "t2.mp3", "t3.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ID3fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ScanDirectory(ctx, "lib-1", dir); err != nil {
		t.Fatal(err)
	}

	tracks, _, err := repo.ListByLibrary(ctx, "lib-1", 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// assign 2 tracks to album-A and 1 to album-B
	idx := 0
	for id, tr := range func() map[string]struct{} {
		m := map[string]struct{}{}
		for _, tr := range tracks {
			m[tr.ID] = struct{}{}
		}
		return m
	}() {
		_ = id
		_ = idx
		_ = tr
		break
	}
	// upsert directly so we control albums
	tracks[0].Album = "Album-A"
	tracks[1].Album = "Album-A"
	tracks[2].Album = "Album-B"
	for _, tr := range tracks {
		if upsertErr := repo.Upsert(ctx, tr); upsertErr != nil {
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	artists, err := svc.GetArtists(context.Background(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 0 {
		t.Fatalf("expected 0 artists, got %d", len(artists))
	}
}

func TestListTracks_Pagination(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	dir := t.TempDir()
	for i := range 10 {
		name := filepath.Join(dir, "track"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(name, []byte("ID3fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}
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

func TestListTracks_EmptySearch(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.mp3", "c.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ID3fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	track, err := svc.GetTrack(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Fatal("expected nil track for nonexistent id")
	}
}

func TestDeleteTrack_PublishesEvent(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)
	ctx := context.Background()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}
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
	svc2 := appmusic.NewService(repo, bus2, nil)

	if err := svc2.DeleteTrack(ctx, trackID); err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}
	events := bus2.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName() != "music.track.deleted" {
		t.Errorf("unexpected event name %q", events[0].EventName())
	}
}

func TestListTracks_SearchFilter(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	dir := t.TempDir()
	for _, name := range []string{"beethoven.mp3", "mozart.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
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
	dir := t.TempDir()
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 from empty dir, got %d", count)
	}
}

func TestScanDirectory_LibraryNotFound(t *testing.T) {
	dir := t.TempDir()
	repo := fakes.NewFakeMusicTrackRepository()
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo)

	_, err := svc.ScanDirectory(context.Background(), "unknown-lib", dir)
	if err == nil {
		t.Fatal("expected error for unknown library, got nil")
	}
}

func TestScanDirectory_LibraryFound(t *testing.T) {
	dir := t.TempDir()
	repo := fakes.NewFakeMusicTrackRepository()
	libRepo := libfakes.NewFakeLibraryQueryRepo()
	libRepo.Seed(&library.Library{ID: "lib-known"})
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), libRepo)

	count, err := svc.ScanDirectory(context.Background(), "lib-known", dir)
	if err != nil {
		t.Fatalf("unexpected error with known library: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tracks from empty dir, got %d", count)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	results, err := svc.Search(context.Background(), "nonexistent-query-xyz", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_ReturnsMatches(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	if err := repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", Title: "Symphony No.9", Artist: "Beethoven", FilePath: "/a/t1.mp3"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-1", Title: "Piano Sonata", Artist: "Mozart", FilePath: "/a/t2.mp3"}); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search(ctx, "beethoven", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearch_LimitApplied(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	for i := range 10 {
		tr := music.Track{
			ID:        "t" + string(rune('0'+i)),
			LibraryID: "lib-1",
			Title:     "track",
			Artist:    "artist",
			FilePath:  "/a/t" + string(rune('0'+i)) + ".mp3",
		}
		if err := repo.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	results, err := svc.Search(ctx, "track", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 3 {
		t.Fatalf("expected at most 3 results with limit=3, got %d", len(results))
	}
}

func TestListTracks_LimitLarge(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	// insert 3 tracks directly
	for i := range 3 {
		tr := music.Track{
			ID:        "t" + string(rune('0'+i)),
			LibraryID: "lib-1",
			Title:     "track",
			FilePath:  "/a/t" + string(rune('0'+i)) + ".mp3",
		}
		if err := repo.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	// pass a very large limit — service passes it through to repo, repo returns all 3
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	_ = repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-A", Title: "Track1", Album: "AlbumX", FilePath: "/a/t1.mp3"})
	_ = repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-A", Title: "Track2", Album: "AlbumX", FilePath: "/a/t2.mp3"})
	_ = repo.Upsert(ctx, music.Track{ID: "t3", LibraryID: "lib-B", Title: "Track3", Album: "AlbumY", FilePath: "/b/t3.mp3"})
	_ = repo.Upsert(ctx, music.Track{ID: "t4", LibraryID: "lib-B", Title: "Track4", Album: "AlbumZ", FilePath: "/b/t4.mp3"})

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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	// deleting a non-existent track: GetByID returns nil (no error), Delete is a no-op
	err := svc.DeleteTrack(context.Background(), "nonexistent-id")
	if err != nil {
		t.Fatalf("expected no error deleting nonexistent track, got: %v", err)
	}
}

func TestScanDirectory_NonExistentDir(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

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
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)

	count, err := svc.ScanDirectory(context.Background(), "lib-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	events := bus.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 TrackScanned event, got %d", len(events))
	}
	if events[0].EventName() != "music.track.scanned" {
		t.Errorf("expected event name music.track.scanned, got %q", events[0].EventName())
	}
	scanned, ok := events[0].(music.TrackScanned)
	if !ok {
		t.Fatal("event is not TrackScanned")
	}
	if scanned.LibraryID != "lib-1" {
		t.Errorf("expected LibraryID lib-1, got %q", scanned.LibraryID)
	}
	if scanned.Title == "" {
		t.Error("expected non-empty Title in TrackScanned")
	}
}

func TestScanDirectory_UpsertError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("ID3fake"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db down")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

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
	dir := t.TempDir()
	songPath := filepath.Join(dir, "mysong.flac")
	if err := os.WriteFile(songPath, []byte("fLaC"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

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
	if tr.Title != "mysong.flac" {
		t.Errorf("Title = %q, want mysong.flac", tr.Title)
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	want := music.Track{
		ID: "trk-1", LibraryID: "lib-1", Title: "Test Song",
		Artist: "Test Artist", Album: "Test Album", MimeType: "audio/mpeg",
		FilePath: "/a/trk-1.mp3", FileSize: 1024,
	}
	if err := repo.Upsert(ctx, want); err != nil {
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, err := svc.GetTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDeleteTrack_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	err := svc.DeleteTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDeleteTrack_EventContainsCorrectFields(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()

	_ = repo.Upsert(ctx, music.Track{
		ID: "trk-99", LibraryID: "lib-7", Title: "Song",
		FilePath: "/a/song.mp3",
	})

	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)

	if err := svc.DeleteTrack(ctx, "trk-99"); err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
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
}

func TestStreamTrack_ReturnsPathAndMime(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()

	_ = repo.Upsert(ctx, music.Track{
		ID: "trk-s1", LibraryID: "lib-1", Title: "Stream Me",
		FilePath: "/music/stream.flac", MimeType: "audio/flac",
	})

	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

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
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, _, err := svc.StreamTrack(context.Background(), "trk-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestListTracks_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, _, err := svc.ListTracks(context.Background(), "lib-1", 50, 0, "")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetAlbums_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, err := svc.GetAlbums(context.Background(), "lib-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetArtists_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, err := svc.GetArtists(context.Background(), "lib-1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestSearch_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = fmt.Errorf("db error")
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	_, err := svc.Search(context.Background(), "query", 50)
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestGetArtists_ReturnsData(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)
	ctx := context.Background()

	_ = repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", Title: "S1", Artist: "ArtistA", AlbumArtist: "ArtistA", Album: "AlbumX", FilePath: "/a/t1.mp3"})
	_ = repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib-1", Title: "S2", Artist: "ArtistA", AlbumArtist: "ArtistA", Album: "AlbumY", FilePath: "/a/t2.mp3"})
	_ = repo.Upsert(ctx, music.Track{ID: "t3", LibraryID: "lib-1", Title: "S3", Artist: "ArtistB", AlbumArtist: "ArtistB", Album: "AlbumZ", FilePath: "/a/t3.mp3"})

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
	dir := t.TempDir()
	for i := range 5 {
		name := filepath.Join(dir, "t"+string(rune('0'+i))+".mp3")
		if err := os.WriteFile(name, []byte("ID3fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	repo := fakes.NewFakeMusicTrackRepository()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := svc.ScanDirectory(ctx, "lib-1", dir)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}
