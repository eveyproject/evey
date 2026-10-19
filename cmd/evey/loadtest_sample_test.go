package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func dur(d time.Duration) shared.Duration {
	v, _ := shared.NewDurationFromTime(d)
	return v
}

// The load test samples the whole library: the walk pages through every film
// and every episode in id order, and keeps only the titles a viewer can play
// for the run's floor.
func TestPlayableLibrary(t *testing.T) {
	byKind := map[string][]*library.MediaItem{}
	for i := range 2*loadTestLibraryPage + 3 {
		byKind["episode"] = append(byKind["episode"], &library.MediaItem{ID: shared.MediaItemID(fmt.Sprintf("e%05d", i)), File: &library.MediaFile{Duration: dur(40 * time.Minute)}})
	}
	byKind["movie"] = []*library.MediaItem{
		{ID: "m1", Type: library.MovieItem, File: &library.MediaFile{Duration: dur(2 * time.Hour), Size: 900_000_000}},
		{ID: "m2", File: &library.MediaFile{Duration: dur(3 * time.Minute)}},
		{ID: "m3"},
	}
	var calls []library.ListMediaItemsFilter
	list := func(_ context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
		calls = append(calls, f)
		if f.Type == nil || !f.KeysetByID {
			t.Fatal("the walk must go by kind, in id order")
		}
		var out []*library.MediaItem
		for _, it := range byKind[*f.Type] {
			if string(it.ID) > f.AfterID && len(out) < f.Limit {
				out = append(out, it)
			}
		}
		return out, nil
	}
	got, err := playableLibrary(context.Background(), list, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1+2*loadTestLibraryPage+3 || got[0].ID != "m1" || got[0].Kind != "movie" || got[0].DurationSec != 7200 || got[0].BitrateBps != 1_000_000 {
		t.Fatalf("got %d titles starting %v, want m1 (a movie of 2 h at 1 Mb/s by its size) and every episode, without the short film or the film with no file", len(got), got[0].ID)
	}
	if len(calls) != 4 {
		t.Fatalf("%d queries, want 1 for the films and 3 pages of episodes", len(calls))
	}

	boom := errors.New("db down")
	fail := func(context.Context, library.ListMediaItemsFilter) ([]*library.MediaItem, error) { return nil, boom }
	if _, err := playableLibrary(context.Background(), fail, time.Minute); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the query's error", err)
	}
}
