package meilisearch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ms "github.com/meilisearch/meilisearch-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type fakeIndex struct {
	addedDocs   []any
	deletedIDs  []string
	searchResp  *ms.SearchResponse
	searchErr   error
	searchQuery string
	searchReq   *ms.SearchRequest
}

func (f *fakeIndex) AddDocumentsWithContext(_ context.Context, docs any, _ *ms.DocumentOptions) (*ms.TaskInfo, error) {
	f.addedDocs = append(f.addedDocs, docs)
	return &ms.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndex) DeleteDocumentWithContext(_ context.Context, id string, _ *ms.DocumentOptions) (*ms.TaskInfo, error) {
	f.deletedIDs = append(f.deletedIDs, id)
	return &ms.TaskInfo{TaskUID: 2}, nil
}

func (f *fakeIndex) SearchWithContext(_ context.Context, query string, req *ms.SearchRequest) (*ms.SearchResponse, error) {
	f.searchQuery = query
	f.searchReq = req
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResp, nil
}

func (f *fakeIndex) UpdateSettingsWithContext(_ context.Context, _ *ms.Settings) (*ms.TaskInfo, error) {
	return &ms.TaskInfo{TaskUID: 3}, nil
}

func TestToDocument(t *testing.T) {
	year := 2023
	now := time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)
	item := &library.MediaItem{
		ID:            "item-1",
		LibraryID:     "lib-1",
		Type:          "movie",
		Title:         "Test Movie",
		OriginalTitle: "Test Original",
		Overview:      "A test movie",
		Genres:        []string{"Action", "Comedy"},
		Studios:       []string{"Studio A"},
		Tags:          []string{"tag1"},
		Year:          &year,
		AddedAt:       now,
	}

	doc := ToDocument(item, []string{"Brad Pitt", "Edward Norton"})

	assert.Equal(t, "item-1", doc.ID)
	assert.Equal(t, "lib-1", doc.LibraryID)
	assert.Equal(t, "movie", doc.Type)
	assert.Equal(t, "Test Movie", doc.Title)
	assert.Equal(t, "Test Original", doc.OriginalTitle)
	assert.Equal(t, []string{"Action", "Comedy"}, doc.Genres)
	assert.Equal(t, []string{"Brad Pitt", "Edward Norton"}, doc.Cast)
	assert.Equal(t, &year, doc.Year)
	assert.Equal(t, now.Unix(), doc.AddedAt)
}

func TestToDocument_NilYear(t *testing.T) {
	item := &library.MediaItem{
		ID:      "item-2",
		Title:   "No Year",
		AddedAt: time.Now(),
	}
	doc := ToDocument(item, nil)
	assert.Nil(t, doc.Year)
}

func TestBuildFilterString_Empty(t *testing.T) {
	assert.Empty(t, BuildFilterString(nil))
	assert.Empty(t, BuildFilterString(map[string]string{}))
}

func TestBuildFilterString_Single(t *testing.T) {
	f := BuildFilterString(map[string]string{"type": "movie"})
	assert.Equal(t, "type = 'movie'", f)
}

func TestBuildFilterString_Multiple(t *testing.T) {
	f := BuildFilterString(map[string]string{
		"type":       "movie",
		"library_id": "lib-1",
	})
	assert.Contains(t, f, "type = 'movie'")
	assert.Contains(t, f, "library_id = 'lib-1'")
	assert.Contains(t, f, " AND ")
}

func TestIndexMediaItem(t *testing.T) {
	fake := &fakeIndex{}
	adapter := &SearchAdapter{index: fake}

	item := &library.MediaItem{
		ID:      "item-1",
		Title:   "Test",
		AddedAt: time.Now(),
	}

	err := adapter.IndexMediaItem(context.Background(), item, []string{"Actor One"})
	require.NoError(t, err)
	require.Len(t, fake.addedDocs, 1)

	docs, ok := fake.addedDocs[0].([]mediaDocument)
	require.True(t, ok)
	assert.Equal(t, "item-1", docs[0].ID)
	assert.Equal(t, "Test", docs[0].Title)
}

func TestRemoveMediaItem(t *testing.T) {
	fake := &fakeIndex{}
	adapter := &SearchAdapter{index: fake}

	err := adapter.RemoveMediaItem(context.Background(), "item-42")
	require.NoError(t, err)
	assert.Equal(t, []string{"item-42"}, fake.deletedIDs)
}

func TestSearch(t *testing.T) {
	year := 2023
	yearRaw, _ := json.Marshal(year)
	hit := ms.Hit{
		"id":     json.RawMessage(`"abc-123"`),
		"title":  json.RawMessage(`"Fight Club"`),
		"year":   json.RawMessage(yearRaw),
		"type":   json.RawMessage(`"movie"`),
		"genres": json.RawMessage(`["Action","Drama"]`),
		"_formatted": json.RawMessage(`{"title":"<mark>Fight</mark> Club"}`),
	}
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{
			Hits:               ms.Hits{hit},
			EstimatedTotalHits: 1,
		},
	}
	adapter := &SearchAdapter{index: fake}

	hits, total, err := adapter.Search(context.Background(), "fight", map[string]string{"type": "movie"}, nil, 20)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "abc-123", hits[0].ID)
	assert.Equal(t, "Fight Club", hits[0].Title)
	assert.Equal(t, &year, hits[0].Year)
	assert.Equal(t, "movie", hits[0].Type)
	assert.Equal(t, []string{"Action", "Drama"}, hits[0].Genres)
	assert.Equal(t, "<mark>Fight</mark> Club", hits[0].HighlightedTitle)
	assert.Equal(t, "fight", fake.searchQuery)
	assert.Contains(t, fake.searchReq.Filter, "type = 'movie'")
}

func TestSearch_NoFilters(t *testing.T) {
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{
			Hits: ms.Hits{},
		},
	}
	adapter := &SearchAdapter{index: fake}

	hits, _, err := adapter.Search(context.Background(), "hello", nil, nil, 20)
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Nil(t, fake.searchReq.Filter)
}

func TestSearch_Error(t *testing.T) {
	fake := &fakeIndex{
		searchErr: assert.AnError,
	}
	adapter := &SearchAdapter{index: fake}

	hits, _, err := adapter.Search(context.Background(), "fail", nil, nil, 20)
	require.Error(t, err)
	assert.Nil(t, hits)
}

func TestBuildFilterString_RangeFilters(t *testing.T) {
	f := BuildFilterString(map[string]string{"year_min": "2000", "year_max": "2020"})
	assert.Contains(t, f, "year >= 2000")
	assert.Contains(t, f, "year <= 2020")
	assert.Contains(t, f, " AND ")
}

func TestSearch_WithSort(t *testing.T) {
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{Hits: ms.Hits{}},
	}
	adapter := &SearchAdapter{index: fake}

	_, _, err := adapter.Search(context.Background(), "test", nil, []string{"year:desc"}, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"year:desc"}, fake.searchReq.Sort)
}

func TestSearch_DefaultLimit(t *testing.T) {
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{Hits: ms.Hits{}},
	}
	adapter := &SearchAdapter{index: fake}

	_, _, err := adapter.Search(context.Background(), "test", nil, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(20), fake.searchReq.Limit)
}
