package main

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// fakeLibrary answers keyset pages over ids "a".."j" the way the repository
// does: rows strictly after AfterID, in id order, at most Limit of them.
func fakeLibrary(calls *[]library.ListMediaItemsFilter) func(context.Context, library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	return func(_ context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
		*calls = append(*calls, f)
		var out []*library.MediaItem
		for _, id := range ids {
			if id > f.AfterID && len(out) < f.Limit {
				out = append(out, &library.MediaItem{ID: shared.MediaItemID(id)})
			}
		}
		return out, nil
	}
}

func itemIDs(items []*library.MediaItem) string {
	s := ""
	for _, it := range items {
		s += string(it.ID)
	}
	return s
}

// The load test's pool is a slice of the library starting at a random id. Mid
// table it is one query; near the end it wraps to the lowest ids; on a library
// smaller than the pool it returns every row once, never twice.
func TestRandomLibrarySlice(t *testing.T) {
	var calls []library.ListMediaItemsFilter
	got, err := randomLibrarySlice(context.Background(), fakeLibrary(&calls), 4, "c")
	if err != nil || itemIDs(got) != "defg" || len(calls) != 1 {
		t.Fatalf("mid-table: got %q in %d call(s) (err %v), want defg in 1", itemIDs(got), len(calls), err)
	}
	if !calls[0].KeysetByID {
		t.Fatal("the slice must walk in id order, not in title order")
	}

	calls = nil
	got, _ = randomLibrarySlice(context.Background(), fakeLibrary(&calls), 4, "h")
	if itemIDs(got) != "ijab" || len(calls) != 2 {
		t.Fatalf("wrap: got %q in %d call(s), want ijab in 2", itemIDs(got), len(calls))
	}

	calls = nil
	got, _ = randomLibrarySlice(context.Background(), fakeLibrary(&calls), 25, "g")
	if itemIDs(got) != "hijabcdefg" {
		t.Fatalf("library smaller than the pool: got %q, want every row exactly once (hijabcdefg)", itemIDs(got))
	}
}

// A failure on the wrap keeps what the first query returned; a failure on the
// first query is an error.
func TestRandomLibrarySlice_Errors(t *testing.T) {
	boom := errors.New("db down")
	n := 0
	list := func(_ context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
		n++
		if n == 2 {
			return nil, boom
		}
		return []*library.MediaItem{{ID: "x"}}, nil
	}
	got, err := randomLibrarySlice(context.Background(), list, 5, "w")
	if err != nil || itemIDs(got) != "x" {
		t.Fatalf("wrap failure: got %q err %v, want the tail kept and no error", itemIDs(got), err)
	}
	fail := func(context.Context, library.ListMediaItemsFilter) ([]*library.MediaItem, error) { return nil, boom }
	if _, err := randomLibrarySlice(context.Background(), fail, 5, "w"); !errors.Is(err, boom) {
		t.Fatalf("first query failure: err = %v, want it returned", err)
	}
}

// The pool holds films and episodes in equal parts even when the library holds
// ten episodes per film, and one kind fills in when the other runs short.
func TestFilmsAndEpisodes(t *testing.T) {
	byKind := map[string][]string{
		"movie":   {"f1", "f2"},
		"episode": {"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8", "e9", "e10"},
	}
	list := func(_ context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
		if f.Type == nil {
			t.Fatal("the draw must filter by kind")
		}
		var out []*library.MediaItem
		for _, id := range byKind[*f.Type] {
			if len(out) < f.Limit {
				out = append(out, &library.MediaItem{ID: shared.MediaItemID(id)})
			}
		}
		return out, nil
	}
	start := func() string { return "" }
	got, err := filmsAndEpisodes(context.Background(), list, 4, start)
	if err != nil || itemIDs(got) != "f1e1f2e2" {
		t.Fatalf("got %q (err %v), want f1e1f2e2: two films and two episodes", itemIDs(got), err)
	}
	got, _ = filmsAndEpisodes(context.Background(), list, 6, start)
	if itemIDs(got) != "f1e1f2e2e3e4" {
		t.Fatalf("got %q, want the episodes to fill in once the films run out", itemIDs(got))
	}
}

// The random starting point must be a well-formed version 4 UUID (the id
// column's type) and must actually vary.
func TestRandomItemID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := randomItemID(), randomItemID()
	if !re.MatchString(a) || !re.MatchString(b) {
		t.Fatalf("ids %q / %q are not version 4 UUIDs", a, b)
	}
	if a == b {
		t.Fatal("two draws gave the same id")
	}
}
