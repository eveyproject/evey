package playlist

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
)

type genFakeTracks struct {
	tracks []music.Track
	err    error
}

func (f *genFakeTracks) ListAllByLibrary(_ context.Context, _ shared.LibraryID) ([]music.Track, error) {
	return f.tracks, f.err
}

type genFakePlays struct{ counts map[string]int }

func (f *genFakePlays) PlayCountsForProfile(_ context.Context, _ string) (map[string]int, error) {
	return f.counts, nil
}

type genFakePlaylists struct {
	created []*playlist.Playlist
	byUser  []*playlist.Playlist
}

func (f *genFakePlaylists) Create(_ context.Context, pl *playlist.Playlist) error {
	f.created = append(f.created, pl)
	return nil
}

func (f *genFakePlaylists) List(_ context.Context, _ string) ([]*playlist.Playlist, error) {
	return f.byUser, nil
}

type genFakeItems struct {
	added   []*playlist.PlaylistItem
	byPlist map[string][]*playlist.PlaylistItem
}

func (f *genFakeItems) Add(_ context.Context, it *playlist.PlaylistItem) error {
	f.added = append(f.added, it)
	return nil
}

func (f *genFakeItems) GetByPlaylistID(_ context.Context, plID string) ([]*playlist.PlaylistItem, error) {
	return f.byPlist[plID], nil
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

// With no listening history and size >= pool, every track is a candidate
// (nothing is excluded) and the playlist gets them all — the random fallback.
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

func TestGenreAffinity(t *testing.T) {
	pool := []music.Track{track("a", "Rock"), track("b", "rock"), track("c", "Jazz"), track("d", "pop")}
	// Played: rock track "a" thrice, jazz track "c" five times; "b"/"d" never.
	playCounts := map[string]int{"a": 3, "c": 5}
	w := genreAffinity(pool, playCounts)

	if w["rock"] != 3 {
		t.Errorf("rock affinity = %v, want 3 (track a played 3×; case-insensitive)", w["rock"])
	}
	if w["jazz"] != 5 {
		t.Errorf("jazz affinity = %v, want 5 (track c played 5×)", w["jazz"])
	}
	if w["pop"] != 0 {
		t.Errorf("pop affinity = %v, want 0 (never played)", w["pop"])
	}
}
