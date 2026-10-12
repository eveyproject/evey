package playlist

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type genFakeTracks struct {
	tracks []music.Track
	err    error
}

func (f *genFakeTracks) SampleByLibrary(_ context.Context, _ shared.LibraryID, limit int) ([]music.Track, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit > 0 && len(f.tracks) > limit {
		return f.tracks[:limit], nil
	}
	return f.tracks, nil
}

type genFakePlays struct {
	counts   map[string]int
	affinity map[string]float64
}

func (f *genFakePlays) PlayCountsForProfile(_ context.Context, _ string) (map[string]int, error) {
	return f.counts, nil
}

func (f *genFakePlays) GenreAffinityForProfile(_ context.Context, _ string) (map[string]float64, error) {
	return f.affinity, nil
}

type genFakePlaylists struct {
	created []*playlist.Playlist
	byUser  []*playlist.Playlist
	deleted []string
}

func (f *genFakePlaylists) Create(_ context.Context, pl *playlist.Playlist) error {
	f.created = append(f.created, pl)
	return nil
}

func (f *genFakePlaylists) List(_ context.Context, _ string) ([]*playlist.Playlist, error) {
	return f.byUser, nil
}

func (f *genFakePlaylists) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type genFakeItems struct {
	added         []*playlist.PlaylistItem
	byPlist       map[string][]*playlist.PlaylistItem
	addErr        error
	getBatchErr   error
	getBatchCalls int // proves the N+1 fix: one call per Generate, not one per playlist
}

func (f *genFakeItems) Add(_ context.Context, it *playlist.PlaylistItem) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, it)
	return nil
}

func (f *genFakeItems) GetByPlaylistIDs(_ context.Context, plIDs []string) (map[string][]*playlist.PlaylistItem, error) {
	f.getBatchCalls++
	if f.getBatchErr != nil {
		return nil, f.getBatchErr
	}
	result := make(map[string][]*playlist.PlaylistItem, len(plIDs))
	for _, id := range plIDs {
		if items, ok := f.byPlist[id]; ok {
			result[id] = items
		}
	}
	return result, nil
}

func track(id, genre string) music.Track {
	return music.Track{ID: id, Title: id, Genre: genre, LibraryID: "lib1"}
}

func newGen(tracks *genFakeTracks, plays *genFakePlays, pls *genFakePlaylists, items *genFakeItems) *GeneratorService {
	return NewGeneratorService(tracks, plays, pls, items)
}

func addedIDs(items *genFakeItems) map[string]bool {
	set := make(map[string]bool)
	for _, it := range items.added {
		set[string(it.MediaItemID)] = true
	}
	return set
}

// A failure adding an item must roll back the just-created playlist and return
// the error, rather than leaving an empty/short playlist and reporting success.
func TestGenerate_AddItemFailure_RollsBack(t *testing.T) {
	tracks := &genFakeTracks{tracks: []music.Track{track("a", "rock"), track("b", "jazz")}}
	pls := &genFakePlaylists{}
	items := &genFakeItems{addErr: errors.New("db down")}
	g := newGen(tracks, &genFakePlays{}, pls, items)

	pl, err := g.Generate(context.Background(), "p1", "lib1", 2, "Mix")
	if err == nil {
		t.Fatal("expected error when item Add fails")
	}
	if pl != nil {
		t.Fatalf("expected nil playlist on failure, got %v", pl)
	}
	if len(pls.created) != 1 {
		t.Fatalf("expected the playlist to have been created once, got %d", len(pls.created))
	}
	if len(pls.deleted) != 1 || pls.deleted[0] != pls.created[0].ID {
		t.Fatalf("expected the created playlist to be rolled back (deleted), got deleted=%v", pls.deleted)
	}
}

// With no listening history and size >= pool, every track is a candidate
// (nothing is excluded) and the playlist gets them all - the random fallback.
func TestGenerate_IncludesAllWhenSizeExceedsPool(t *testing.T) {
	tracks := &genFakeTracks{tracks: []music.Track{track("a", "rock"), track("b", "jazz"), track("c", "pop")}}
	items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
	g := newGen(tracks, &genFakePlays{}, &genFakePlaylists{}, items)

	pl, err := g.Generate(context.Background(), "p1", "lib1", 10, "Mix")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if pl == nil {
		t.Fatal("expected a playlist")
	}
	got := addedIDs(items)
	for _, id := range []string{"a", "b", "c"} {
		if !got[id] {
			t.Errorf("track %q should be included, got %v", id, got)
		}
	}
	// Items are music tracks.
	if len(items.added) > 0 && items.added[0].MediaType != playlist.MediaTypeMusic {
		t.Errorf("MediaType = %q, want music", items.added[0].MediaType)
	}
}

func TestGenerate_RespectsSizeAndCreatesManualPlaylist(t *testing.T) {
	var pool []music.Track
	for _, id := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
		pool = append(pool, track(id, "pop"))
	}
	tracks := &genFakeTracks{tracks: pool}
	items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
	pls := &genFakePlaylists{}
	g := newGen(tracks, &genFakePlays{}, pls, items)

	pl, err := g.Generate(context.Background(), "p1", "lib1", 3, "Surprise")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(items.added) != 3 {
		t.Errorf("expected 3 items added, got %d", len(items.added))
	}
	if len(pls.created) != 1 {
		t.Fatalf("expected 1 playlist created, got %d", len(pls.created))
	}
	if pl.Type != playlist.Manual {
		t.Errorf("Type = %q, want manual", pl.Type)
	}
	if string(pl.ProfileID) != "p1" {
		t.Errorf("ProfileID = %q, want p1", pl.ProfileID)
	}
	if pl.Name != "Surprise" {
		t.Errorf("Name = %q, want Surprise", pl.Name)
	}
	if pl.ID == "" {
		t.Error("playlist should have an ID")
	}
}

func TestGenerate_NoTracks(t *testing.T) {
	pls := &genFakePlaylists{}
	g := newGen(&genFakeTracks{tracks: nil}, &genFakePlays{}, pls, &genFakeItems{})
	_, err := g.Generate(context.Background(), "p1", "lib1", 10, "Mix")
	if !errors.Is(err, ErrNoTracks) {
		t.Errorf("expected ErrNoTracks, got %v", err)
	}
	if len(pls.created) != 0 {
		t.Error("no playlist should be created when there are no tracks")
	}
}

func TestGenerate_TracksError(t *testing.T) {
	g := newGen(&genFakeTracks{err: errors.New("boom")}, &genFakePlays{}, &genFakePlaylists{}, &genFakeItems{})
	if _, err := g.Generate(context.Background(), "p1", "lib1", 10, "Mix"); err == nil {
		t.Error("expected error when listing tracks fails")
	}
}

// alreadyInPlaylists used to call GetByPlaylistID once per profile playlist
// (N+1). It must now batch every profile playlist into a single
// GetByPlaylistIDs call, however many playlists the profile has.
func TestGenerate_AlreadyInPlaylists_BatchesLookup(t *testing.T) {
	tracks := &genFakeTracks{tracks: []music.Track{track("a", "rock"), track("b", "jazz"), track("c", "pop")}}
	pls := &genFakePlaylists{byUser: []*playlist.Playlist{
		{ID: "pl1"}, {ID: "pl2"}, {ID: "pl3"},
	}}
	items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{
		"pl1": {{MediaItemID: "a"}},
		"pl2": {{MediaItemID: "b"}},
		"pl3": {{MediaItemID: "c"}},
	}}
	g := newGen(tracks, &genFakePlays{}, pls, items)

	if _, err := g.Generate(context.Background(), "p1", "lib1", 10, "Mix"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if items.getBatchCalls != 1 {
		t.Errorf("GetByPlaylistIDs calls = %d, want 1 (batched across all %d profile playlists)", items.getBatchCalls, len(pls.byUser))
	}
}

// A batch lookup failure must degrade to an empty de-duplication set (soft
// penalty simply doesn't apply) rather than failing the generation.
func TestGenerate_AlreadyInPlaylists_BatchErrorDegradesGracefully(t *testing.T) {
	tracks := &genFakeTracks{tracks: []music.Track{track("a", "rock"), track("b", "jazz")}}
	pls := &genFakePlaylists{byUser: []*playlist.Playlist{{ID: "pl1"}}}
	items := &genFakeItems{
		byPlist:     map[string][]*playlist.PlaylistItem{"pl1": {{MediaItemID: "a"}}},
		getBatchErr: errors.New("db down"),
	}
	g := newGen(tracks, &genFakePlays{}, pls, items)

	pl, err := g.Generate(context.Background(), "p1", "lib1", 10, "Mix")
	if err != nil {
		t.Fatalf("Generate must not fail when the de-duplication lookup errors: %v", err)
	}
	if pl == nil {
		t.Fatal("expected a playlist despite the lookup error")
	}
	got := addedIDs(items)
	if !got["a"] || !got["b"] {
		t.Errorf("expected both tracks included (no de-dup penalty applied), got %v", got)
	}
}

// Genre affinity is now computed in SQL (MusicPlayRepository.GenreAffinityForProfile);
// see the postgres test TestMusicPlayRepo_GenreAffinityForProfile.

// The requested size is clamped at both ends before anything is sampled. Those
// two bounds are what stops a caller from generating an unbounded playlist on a
// weak NAS, and they had no test: a first mutation pass on this package
// (2026-08-08) left both boundary mutants alive.
//
// The other survivors in this file sit in the scoring loop, which multiplies by
// rand.Float64() - they are only pinnable statistically, and not worth it.
func TestGenerate_ClampsTheRequestedSize(t *testing.T) {
	poolOf := func(n int) []music.Track {
		var pool []music.Track
		for i := range n {
			pool = append(pool, track(fmt.Sprintf("t%d", i), "pop"))
		}
		return pool
	}

	t.Run("a non-positive size falls back to the default, it does not generate nothing", func(t *testing.T) {
		tracks := &genFakeTracks{tracks: poolOf(defaultGenSize + 10)}
		items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
		g := newGen(tracks, &genFakePlays{}, &genFakePlaylists{}, items)

		if _, err := g.Generate(context.Background(), "p1", "lib1", 0, "Mix"); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(items.added) != defaultGenSize {
			t.Errorf("size 0 produced %d items, want the default %d", len(items.added), defaultGenSize)
		}
	})

	t.Run("a negative size is treated the same way", func(t *testing.T) {
		tracks := &genFakeTracks{tracks: poolOf(defaultGenSize + 10)}
		items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
		g := newGen(tracks, &genFakePlays{}, &genFakePlaylists{}, items)

		if _, err := g.Generate(context.Background(), "p1", "lib1", -5, "Mix"); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(items.added) != defaultGenSize {
			t.Errorf("size -5 produced %d items, want the default %d", len(items.added), defaultGenSize)
		}
	})

	t.Run("an oversized request is capped, not honoured", func(t *testing.T) {
		tracks := &genFakeTracks{tracks: poolOf(maxGenSize + 50)}
		items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
		g := newGen(tracks, &genFakePlays{}, &genFakePlaylists{}, items)

		if _, err := g.Generate(context.Background(), "p1", "lib1", maxGenSize+50, "Mix"); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(items.added) != maxGenSize {
			t.Errorf("an oversized request produced %d items, want the cap %d", len(items.added), maxGenSize)
		}
	})

	t.Run("exactly the cap is allowed through", func(t *testing.T) {
		tracks := &genFakeTracks{tracks: poolOf(maxGenSize + 10)}
		items := &genFakeItems{byPlist: map[string][]*playlist.PlaylistItem{}}
		g := newGen(tracks, &genFakePlays{}, &genFakePlaylists{}, items)

		if _, err := g.Generate(context.Background(), "p1", "lib1", maxGenSize, "Mix"); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(items.added) != maxGenSize {
			t.Errorf("a request of exactly the cap produced %d items, want %d", len(items.added), maxGenSize)
		}
	})
}
