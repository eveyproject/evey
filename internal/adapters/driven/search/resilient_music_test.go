package search

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/music"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

// The music index mirrors the media one: writes are best-effort on the
// primary, reads fall back to SQL when the primary errors or is absent.

type fakeMusicIndex struct {
	tracks   []music.Track
	err      error
	indexed  []string
	removed  []string
	batches  int
	searches int
}

func (f *fakeMusicIndex) IndexTrack(_ context.Context, t music.Track) error {
	f.indexed = append(f.indexed, t.ID)
	return f.err
}

func (f *fakeMusicIndex) IndexTracksBatch(_ context.Context, tracks []music.Track) error {
	f.batches++
	for _, t := range tracks {
		f.indexed = append(f.indexed, t.ID)
	}
	return f.err
}

func (f *fakeMusicIndex) RemoveTrack(_ context.Context, id string) error {
	f.removed = append(f.removed, id)
	return f.err
}

func (f *fakeMusicIndex) Search(context.Context, shared.LibraryID, string, int) ([]music.Track, error) {
	f.searches++
	return f.tracks, f.err
}

func TestResilientMusic_WritesAreBestEffortOnThePrimary(t *testing.T) {
	ctx := context.Background()
	primary := &fakeMusicIndex{err: errors.New("meili down")}
	r := NewResilientMusic(primary, &fakeMusicIndex{})

	if err := r.IndexTrack(ctx, music.Track{ID: "t1"}); err != nil {
		t.Errorf("a failing primary must not fail the write: %v", err)
	}
	if err := r.IndexTracksBatch(ctx, []music.Track{{ID: "t2"}, {ID: "t3"}}); err != nil {
		t.Errorf("batch: %v", err)
	}
	if err := r.RemoveTrack(ctx, "t1"); err != nil {
		t.Errorf("remove: %v", err)
	}
	if len(primary.indexed) != 3 || primary.batches != 1 || len(primary.removed) != 1 {
		t.Errorf("primary saw indexed=%v batches=%d removed=%v", primary.indexed, primary.batches, primary.removed)
	}
	if r.Primary() != primary {
		t.Error("Primary() must expose the wrapped index")
	}

	// No primary at all: every write is a silent no-op.
	none := NewResilientMusic(nil, &fakeMusicIndex{})
	if err := none.IndexTrack(ctx, music.Track{ID: "t1"}); err != nil {
		t.Error(err)
	}
	if err := none.IndexTracksBatch(ctx, nil); err != nil {
		t.Error(err)
	}
	if err := none.RemoveTrack(ctx, "t1"); err != nil {
		t.Error(err)
	}
}

func TestResilientMusic_SearchFallsBackToSQL(t *testing.T) {
	ctx := context.Background()
	fromPrimary := []music.Track{{ID: "p"}}
	fromSQL := []music.Track{{ID: "s"}}

	primary := &fakeMusicIndex{tracks: fromPrimary}
	fallback := &fakeMusicIndex{tracks: fromSQL}
	got, err := NewResilientMusic(primary, fallback).Search(ctx, "lib1", "q", 10)
	if err != nil || len(got) != 1 || got[0].ID != "p" {
		t.Errorf("healthy primary: got %v err=%v", got, err)
	}
	if fallback.searches != 0 {
		t.Error("the fallback must not be consulted while the primary answers")
	}

	primary = &fakeMusicIndex{err: errors.New("meili down")}
	fallback = &fakeMusicIndex{tracks: fromSQL}
	got, err = NewResilientMusic(primary, fallback).Search(ctx, "lib1", "q", 10)
	if err != nil || len(got) != 1 || got[0].ID != "s" {
		t.Errorf("failing primary: got %v err=%v, want the SQL result", got, err)
	}

	got, err = NewResilientMusic(nil, fallback).Search(ctx, "lib1", "q", 10)
	if err != nil || len(got) != 1 || got[0].ID != "s" {
		t.Errorf("no primary: got %v err=%v, want the SQL result", got, err)
	}

	got, err = NewResilientMusic(primary, nil).Search(ctx, "lib1", "q", 10)
	if err != nil || got != nil {
		t.Errorf("failing primary and no fallback: got %v err=%v, want nothing and no error", got, err)
	}
}

func TestMusicRepoFallback_SearchesTheTableAndIgnoresWrites(t *testing.T) {
	ctx := context.Background()
	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(ctx, music.Track{ID: "t1", LibraryID: "lib1", Title: "Blue Train", Artist: "Coltrane", FilePath: "/m/1.flac"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, music.Track{ID: "t2", LibraryID: "lib2", Title: "Blue Monday", Artist: "New Order", FilePath: "/m/2.flac"}); err != nil {
		t.Fatal(err)
	}
	f := NewMusicRepoFallback(repo)

	got, err := f.Search(ctx, "lib1", "blue", 10)
	if err != nil || len(got) != 1 || got[0].ID != "t1" {
		t.Errorf("library-scoped: got %v err=%v", got, err)
	}
	// An empty library id is the global search bar: every library.
	got, err = f.Search(ctx, "", "blue", 10)
	if err != nil || len(got) != 2 {
		t.Errorf("cross-library: got %d err=%v, want 2", len(got), err)
	}
	// The table IS the source of truth: nothing to index or remove.
	if err := f.IndexTrack(ctx, music.Track{ID: "x"}); err != nil {
		t.Error(err)
	}
	if err := f.IndexTracksBatch(ctx, nil); err != nil {
		t.Error(err)
	}
	if err := f.RemoveTrack(ctx, "t1"); err != nil {
		t.Error(err)
	}
	if got, _ := f.Search(ctx, "lib1", "blue", 10); len(got) != 1 {
		t.Error("RemoveTrack on the fallback must not touch the table")
	}
}
