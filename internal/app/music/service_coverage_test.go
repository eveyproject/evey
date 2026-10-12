package music_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"

	appmusic "github.com/eveyproject/evey/internal/app/music"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/music/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// --- ID3v2 helpers ---

func writeID3v2Frame(buf *bytes.Buffer, id string, data []byte) {
	buf.WriteString(id)
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(data)))
	buf.Write(size)
	buf.Write([]byte{0, 0})
	buf.Write(data)
}

func textFrame(text string) []byte {
	b := []byte{0x03}
	b = append(b, []byte(text)...)
	return b
}

func pictureFrame(mime string, data []byte) []byte {
	var b bytes.Buffer
	b.WriteByte(0x00)
	b.WriteString(mime)
	b.WriteByte(0x00)
	b.WriteByte(0x03)
	b.WriteByte(0x00)
	b.Write(data)
	return b.Bytes()
}

func buildID3v2(frames ...func(*bytes.Buffer)) []byte {
	var fb bytes.Buffer
	for _, f := range frames {
		f(&fb)
	}
	var header bytes.Buffer
	header.WriteString("ID3")
	header.Write([]byte{0x04, 0x00, 0x00})
	sz := fb.Len()
	header.Write([]byte{
		byte((sz >> 21) & 0x7f),
		byte((sz >> 14) & 0x7f),
		byte((sz >> 7) & 0x7f),
		byte(sz & 0x7f),
	})
	header.Write(fb.Bytes())
	return header.Bytes()
}

func withText(id, val string) func(*bytes.Buffer) {
	return func(buf *bytes.Buffer) { writeID3v2Frame(buf, id, textFrame(val)) }
}

func withPicture(mime string, data []byte) func(*bytes.Buffer) {
	return func(buf *bytes.Buffer) { writeID3v2Frame(buf, "APIC", pictureFrame(mime, data)) }
}

// --- Tests ---

func TestServiceAccessors(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	if svc.CommandService() == nil {
		t.Fatal("CommandService() returned nil")
	}
	if svc.QueryService() == nil {
		t.Fatal("QueryService() returned nil")
	}
}

func TestStreamTrackURL(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	qs := appmusic.NewQueryService(repo, scanner)

	got := qs.StreamTrackURL("track-42")
	want := "/api/v1/music/tracks/track-42/stream"
	if got != want {
		t.Fatalf("StreamTrackURL = %q, want %q", got, want)
	}
}

func TestStreamTrack_Success(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/song.flac", MimeType: "audio/flac"})

	qs := appmusic.NewQueryService(repo, fakes.NewFakeMusicScanner())
	path, mime, err := qs.StreamTrack(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/music/song.flac" {
		t.Fatalf("path = %q", path)
	}
	if mime != "audio/flac" {
		t.Fatalf("mime = %q", mime)
	}
}

func TestGetTrackArt_Success(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/song.mp3"})

	scanner := fakes.NewFakeMusicScanner()
	scanner.FileData["/music/song.mp3"] = buildID3v2(
		withText("TIT2", "Song"),
		withPicture("image/png", []byte("pngdata")),
	)

	qs := appmusic.NewQueryService(repo, scanner)
	data, mime, err := qs.GetTrackArt(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q", mime)
	}
	if !bytes.Equal(data, []byte("pngdata")) {
		t.Fatalf("unexpected art data: %v", data)
	}
}

func TestGetTrackArt_NoPicture(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/song.mp3"})

	scanner := fakes.NewFakeMusicScanner()
	scanner.FileData["/music/song.mp3"] = buildID3v2(withText("TIT2", "Song"))

	qs := appmusic.NewQueryService(repo, scanner)
	_, _, err := qs.GetTrackArt(ctx, "t1")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestGetTrackArt_OpenFileError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/missing.mp3"})

	scanner := fakes.NewFakeMusicScanner()
	// no FileData entry -> OpenFile returns not found

	qs := appmusic.NewQueryService(repo, scanner)
	_, _, err := qs.GetTrackArt(ctx, "t1")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestGetTrackArt_BadTagData(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/bad.mp3"})

	scanner := fakes.NewFakeMusicScanner()
	scanner.FileData["/music/bad.mp3"] = []byte("not valid tag data")

	qs := appmusic.NewQueryService(repo, scanner)
	_, _, err := qs.GetTrackArt(ctx, "t1")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestGetTrackArt_EmptyMIME(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", FilePath: "/music/song.mp3"})

	scanner := fakes.NewFakeMusicScanner()
	scanner.FileData["/music/song.mp3"] = buildID3v2(
		withText("TIT2", "Song"),
		withPicture("", []byte("jpgdata")),
	)

	qs := appmusic.NewQueryService(repo, scanner)
	_, mime, err := qs.GetTrackArt(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("expected fallback image/jpeg, got %q", mime)
	}
}

func TestDeleteTrack_GetByIDError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = errors.New("db error")
	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, fakes.NewFakeMusicScanner())

	err := cs.DeleteTrack(context.Background(), "t1")
	if err == nil || !errors.Is(err, repo.Err) {
		t.Fatalf("expected db error, got %v", err)
	}
}

func TestDeleteTrack_NilTrack(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	cs := appmusic.NewCommandService(repo, bus, nil, fakes.NewFakeMusicScanner())
	ctx := context.Background()

	// track doesn't exist -> GetByID returns nil, nil
	err := cs.DeleteTrack(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events even for nil track, got %d", len(events))
	}
}

func TestScanDirectory_ParseTrackSuccess(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	bus := sharedfakes.NewFakeEventBus()

	id3data := buildID3v2(
		withText("TIT2", "My Song"),
		withText("TPE1", "Artist X"),
		withText("TALB", "Album Y"),
		withText("TPE2", "Album Artist Z"),
		withText("TRCK", "5"),
		withText("TPOS", "2"),
		withText("TDRC", "2023"),
		withText("TCON", "Jazz"),
	)

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/song.mp3", Size: int64(len(id3data)), MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.FileData["/lib/song.mp3"] = id3data
	scanner.Hashes["/lib/song.mp3"] = "abc123"

	cs := appmusic.NewCommandService(repo, bus, nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	tracks, _, _ := repo.ListByLibrary(context.Background(), "lib-1", 50, 0, "")
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track in repo, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Title != "My Song" {
		t.Errorf("Title = %q", tr.Title)
	}
	if tr.Artist != "Artist X" {
		t.Errorf("Artist = %q", tr.Artist)
	}
	if tr.Album != "Album Y" {
		t.Errorf("Album = %q", tr.Album)
	}
	if tr.AlbumArtist != "Album Artist Z" {
		t.Errorf("AlbumArtist = %q", tr.AlbumArtist)
	}
	if tr.TrackNumber != 5 {
		t.Errorf("TrackNumber = %d", tr.TrackNumber)
	}
	if tr.DiskNumber != 2 {
		t.Errorf("DiskNumber = %d", tr.DiskNumber)
	}
	if tr.Year != 2023 {
		t.Errorf("Year = %d", tr.Year)
	}
	if tr.Genre != "Jazz" {
		t.Errorf("Genre = %q", tr.Genre)
	}
	if tr.FileHash != "abc123" {
		t.Errorf("FileHash = %q", tr.FileHash)
	}
}

func TestScanDirectory_ParseTrackWithCoverArt(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()

	id3data := buildID3v2(
		withText("TIT2", "Art Song"),
		withPicture("image/png", []byte("img")),
	)

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/art.mp3", Size: int64(len(id3data)), MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.FileData["/lib/art.mp3"] = id3data

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	tracks, _, _ := repo.ListByLibrary(context.Background(), "lib-1", 50, 0, "")
	if len(tracks) != 1 {
		t.Fatal("no tracks")
	}
	if tracks[0].CoverArtURL == "" {
		t.Error("expected CoverArtURL to be set")
	}
}

func TestScanDirectory_ParseTrackNoAlbumArtist(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()

	id3data := buildID3v2(
		withText("TIT2", "Solo"),
		withText("TPE1", "Solo Artist"),
	)

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/solo.mp3", Size: int64(len(id3data)), MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.FileData["/lib/solo.mp3"] = id3data

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	_, _ = cs.ScanDirectory(context.Background(), "lib-1", "/lib")

	tracks, _, _ := repo.ListByLibrary(context.Background(), "lib-1", 50, 0, "")
	if len(tracks) != 1 {
		t.Fatal("no tracks")
	}
	if tracks[0].AlbumArtist != "Solo Artist" {
		t.Errorf("AlbumArtist should fallback to Artist, got %q", tracks[0].AlbumArtist)
	}
}

func TestScanDirectory_OpenFileError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/missing.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.OpenErr = errors.New("disk error")

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	// should still upsert a fallback track
	if count != 1 {
		t.Fatalf("expected 1 fallback track, got %d", count)
	}
}

func TestScanDirectory_MovedFile(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	ctx := context.Background()

	// seed existing track at old path with hash
	_, _ = repo.Upsert(ctx, music.Track{
		ID: "existing-1", LibraryID: "lib-1",
		FilePath: "/lib/old/song.mp3", FileSize: 100, FileHash: "hash-abc",
		ModTime: time.Now().Add(-time.Hour),
	})

	// scanner reports same hash at new path
	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/new/song.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.Hashes["/lib/new/song.mp3"] = "hash-abc"

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := cs.ScanDirectory(ctx, "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	// moved file = updated path, not counted as new
	if count != 0 {
		t.Fatalf("expected 0 (moved file), got %d", count)
	}
	tr, _ := repo.GetByID(ctx, "existing-1")
	if tr == nil {
		t.Fatal("track should still exist")
	}
	if tr.FilePath != "/lib/new/song.mp3" {
		t.Errorf("FilePath = %q, want /lib/new/song.mp3", tr.FilePath)
	}
}

func TestGetTrackArt_RepoError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	repo.Err = errors.New("db down")
	qs := appmusic.NewQueryService(repo, fakes.NewFakeMusicScanner())
	_, _, err := qs.GetTrackArt(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteTrack_PublishFails(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	ctx := context.Background()
	_, _ = repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib-1", FilePath: "/a.mp3"})

	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus fail")
	cs := appmusic.NewCommandService(repo, bus, nil, fakes.NewFakeMusicScanner())

	err := cs.DeleteTrack(ctx, "t1")
	if err != nil {
		t.Fatalf("delete should succeed even if publish fails, got %v", err)
	}
	// track should be deleted
	tr, _ := repo.GetByID(ctx, "t1")
	if tr != nil {
		t.Fatal("track should be deleted")
	}
}

func TestScanDirectory_HashError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/song.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.HashErr = errors.New("hash fail")

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 (hash error non-fatal), got %d", count)
	}
}

func TestScanDirectory_PublishError(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/song.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}

	cs := appmusic.NewCommandService(repo, bus, nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	// publish errors are logged but don't prevent count
	if count != 1 {
		t.Fatalf("expected 1 (publish error non-fatal), got %d", count)
	}
}

// Prod 2026-08-26: an untagged mp3 surfaced in every client titled
// "Star Profile - 01 - Audio Documentary.mp3" - extension included. The
// filename is a legitimate last-resort title, the ".mp3" on the end never is.
func TestScanDirectory_FallbackTitleHasNoExtension(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()

	scanner.Files = []music.ScannedAudioFile{
		{Path: "/lib/audio documentary/Star Profile - 01 - Audio Documentary.mp3", Size: 100, MimeType: "audio/mpeg", ModTime: time.Now()},
	}
	scanner.OpenErr = errors.New("disk error") // forces the fallback-track path

	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	count, err := cs.ScanDirectory(context.Background(), "lib-1", "/lib")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 fallback track, got %d", count)
	}
	all, _ := repo.ListAllByLibrary(context.Background(), "lib-1")
	if len(all) != 1 {
		t.Fatalf("expected 1 track, got %d", len(all))
	}
	if got := all[0].Title; got != "Star Profile - 01 - Audio Documentary" {
		t.Fatalf("fallback title must drop the extension, got %q", got)
	}
}
