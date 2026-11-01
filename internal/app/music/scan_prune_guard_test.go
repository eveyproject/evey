package music_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appmusic "github.com/eveyproject/evey/internal/app/music"
	"github.com/eveyproject/evey/internal/app/prunehold"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/music/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A union mount (mergerfs) missing one of its three branches: the walk
// completes, a third of the tracks are really absent from the root's view.
func seedBranchLoss(t *testing.T) (*fakes.FakeMusicTrackRepository, *fakes.FakeMusicScanner) {
	t.Helper()
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	mod := time.Unix(1_700_000_000, 0)
	for i := range 60 {
		id := fmt.Sprintf("t-%d", i)
		path := fmt.Sprintf("/pool/album/%02d.flac", i)
		if _, err := repo.Upsert(context.Background(), music.Track{
			ID: id, LibraryID: "lib-1", Title: "T", FilePath: path,
			FileSize: 100, FileHash: "hash-" + id, ModTime: mod,
		}); err != nil {
			t.Fatal(err)
		}
		if i < 40 {
			scanner.Files = append(scanner.Files, music.ScannedAudioFile{Path: path, Size: 100, MimeType: "audio/flac", ModTime: mod})
			scanner.Hashes[path] = "hash-" + id
		}
	}
	return repo, scanner
}

func removals(bus *sharedfakes.FakeEventBus) int {
	n := 0
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(music.TrackRemoved); ok {
			n++
		}
	}
	return n
}

func TestScanDirectory_BranchMissing_NothingPrunedPlaylistsIntact(t *testing.T) {
	repo, scanner := seedBranchLoss(t)
	bus := sharedfakes.NewFakeEventBus()
	cs := appmusic.NewCommandService(repo, bus, nil, scanner)

	for pass := range 2 {
		if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
			t.Fatal(err)
		}
		if n := trackCount(t, repo); n != 60 {
			t.Fatalf("pass %d: a third of the root gone must hold the prune, have %d tracks", pass, n)
		}
	}
	// No TrackRemoved: the playlist cleanup never runs, every entry stays.
	if n := removals(bus); n != 0 {
		t.Fatalf("want no removal event, got %d", n)
	}
	var notices int
	for _, e := range bus.PublishedEvents() {
		if s, ok := e.(library.PruneSuspended); ok {
			notices++
			if s.Root != "/pool" || s.Missing != 20 || s.Total != 60 {
				t.Fatalf("unexpected notice %+v", s)
			}
		}
	}
	if notices != 1 {
		t.Fatalf("want the admins paged once, got %d", notices)
	}
}

func TestScanDirectory_BranchMissing_ConfirmedSnapshotPrunesThatSet(t *testing.T) {
	repo, scanner := seedBranchLoss(t)
	hold := prunehold.New(nil)
	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner).WithPruneHold(hold)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	sus := hold.Suspensions("lib-1")
	if len(sus) != 1 {
		t.Fatalf("want /pool held, got %+v", sus)
	}
	if err := hold.Confirm("lib-1", "/pool", sus[0].Snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 40 {
		t.Fatalf("the confirmed set must go, have %d tracks", n)
	}
	if len(hold.Suspensions("lib-1")) != 0 {
		t.Fatal("hold not cleared after the confirmed prune")
	}
}

func TestScanDirectory_BranchBack_ResumesAutomatically(t *testing.T) {
	repo, scanner := seedBranchLoss(t)
	hold := prunehold.New(nil)
	cs := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner).WithPruneHold(hold)
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	// The branch is back: every file but one is seen again.
	mod := time.Unix(1_700_000_000, 0)
	scanner.Files = scanner.Files[:0]
	for i := 1; i < 60; i++ {
		path := fmt.Sprintf("/pool/album/%02d.flac", i)
		scanner.Files = append(scanner.Files, music.ScannedAudioFile{Path: path, Size: 100, MimeType: "audio/flac", ModTime: mod})
		scanner.Hashes[path] = fmt.Sprintf("hash-t-%d", i)
	}
	if _, err := cs.ScanDirectory(context.Background(), "lib-1", "/pool"); err != nil {
		t.Fatal(err)
	}
	if n := trackCount(t, repo); n != 59 {
		t.Fatalf("one real deletion must prune normally, have %d", n)
	}
	if len(hold.Suspensions("lib-1")) != 0 {
		t.Fatal("hold must clear on its own")
	}
}
