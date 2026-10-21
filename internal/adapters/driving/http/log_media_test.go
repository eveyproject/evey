package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

// fakeLogMedia answers the search with fixed hits and knows each item's
// descendants.
type fakeLogMedia struct {
	hits  []library.SearchHit
	under map[string][]string
	query string
}

func (f *fakeLogMedia) SearchMedia(_ context.Context, query string, _ map[string]string, _ []string, _ int64) ([]library.SearchHit, int64, error) {
	f.query = query
	return f.hits, int64(len(f.hits)), nil
}

func (f *fakeLogMedia) MediaDescendantIDs(_ context.Context, id string) ([]string, error) {
	return f.under[id], nil
}

const seriesID = "673f80ef-a8dc-44db-b98a-2198d4c8f806"

func TestResolveLogMedia_ATitleBringsEverythingUnderIt(t *testing.T) {
	lk := &fakeLogMedia{
		hits: []library.SearchHit{
			// An episode whose title matches better: a series is named by its
			// own title, its episodes are not looked up by theirs.
			{ID: "ep-x", Title: "Stranger Things", Type: "episode"},
			{ID: seriesID, Title: "Stranger Things", Type: "series"},
			{ID: "other", Title: "Tu ne mentiras point", Type: "movie"},
		},
		under: map[string][]string{seriesID: {"s1", "e1", "e2"}},
	}
	res, err := resolveLogMedia(context.Background(), lk, "  stranger things ")
	if err != nil {
		t.Fatal(err)
	}
	if lk.query != "stranger things" {
		t.Fatalf("searched %q", lk.query)
	}
	if !slices.Equal(res.IDs, []string{seriesID, "s1", "e1", "e2"}) {
		t.Fatalf("ids = %v: the series and its seasons and episodes, not the episode hit nor the next hit", res.IDs)
	}
	if len(res.Media) != 1 || res.Media[0].Title != "Stranger Things" || res.Media[0].Descendants != 3 {
		t.Fatalf("media = %+v", res.Media)
	}
}

// Two films under one title (two versions in the library) are both taken.
func TestResolveLogMedia_EveryHitSharingTheBestTitle(t *testing.T) {
	lk := &fakeLogMedia{hits: []library.SearchHit{
		{ID: "a", Title: "The Thing", Type: "movie"},
		{ID: "b", Title: "the thing", Type: "movie"},
		{ID: "c", Title: "Captain America", Type: "movie"},
	}}
	res, _ := resolveLogMedia(context.Background(), lk, "the thing")
	if !slices.Equal(res.IDs, []string{"a", "b"}) || len(res.Media) != 2 {
		t.Fatalf("ids = %v, media = %+v", res.IDs, res.Media)
	}
}

// An id is not searched: it matches itself, as the filter always did.
func TestResolveLogMedia_AnIDMatchesItself(t *testing.T) {
	lk := &fakeLogMedia{}
	res, _ := resolveLogMedia(context.Background(), lk, seriesID)
	if lk.query != "" || !slices.Equal(res.IDs, []string{seriesID}) || len(res.Media) != 0 {
		t.Fatalf("query %q, res %+v", lk.query, res)
	}
	if res, _ := resolveLogMedia(context.Background(), nil, "anything"); !slices.Equal(res.IDs, []string{"anything"}) {
		t.Fatalf("without a lookup the input is an id: %+v", res)
	}
}

// A title nothing matches resolves to no id, and the search then matches no
// line: an empty set, not "no filter".
func TestResolveLogMedia_NoHitMatchesNothing(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T10:00:00Z","level":"INFO","msg":"played","media_id":"e1"}`,
		`{"time":"2026-07-07T10:00:01Z","level":"INFO","msg":"played","media_id":"zz"}`,
	)
	res, _ := resolveLogMedia(context.Background(), &fakeLogMedia{}, "nothing like it")
	if len(res.IDs) != 0 {
		t.Fatalf("ids = %v", res.IDs)
	}
	got, _ := searchLogs(dir, logSearchFilter{MediaIDs: map[string]bool{}, Limit: 10})
	if got.Total != 0 {
		t.Fatalf("an unknown title matched %d lines", got.Total)
	}
	got, _ = searchLogs(dir, logSearchFilter{MediaIDs: map[string]bool{"e1": true, seriesID: true}, Limit: 10})
	if got.Total != 1 || got.Entries[0].MediaID != "e1" {
		t.Fatalf("an episode of the series: %+v", got)
	}
}

// Through the handlers: the search takes the title in ?media=, and the live
// tail's lookup route answers the ids and what the title named.
func TestHandleLogs_MediaTitle(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T10:00:00Z","level":"INFO","msg":"episode played","media_id":"e2"}`,
		`{"time":"2026-07-07T10:00:01Z","level":"INFO","msg":"film played","media_id":"other"}`,
	)
	lk := &fakeLogMedia{
		hits:  []library.SearchHit{{ID: seriesID, Title: "Stranger Things", Type: "series"}},
		under: map[string][]string{seriesID: {"s1", "e1", "e2"}},
	}
	h := &adminHandler{logDir: dir, mediaLookup: lk}

	rec := httptest.NewRecorder()
	h.handleSearchLogs(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/search?media=Stranger%20Things", nil))
	var got logSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got.Total != 1 || got.Entries[0].Msg != "episode played" {
		t.Fatalf("search by title: %+v", got)
	}

	rec = httptest.NewRecorder()
	h.handleResolveLogMedia(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/media?q=Stranger%20Things", nil))
	var res logMediaResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(res.IDs) != 4 || len(res.Media) != 1 || res.Media[0].Type != "series" {
		t.Fatalf("lookup: %+v", res)
	}
}
