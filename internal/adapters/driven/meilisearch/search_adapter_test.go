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
	addedDocs   []interface{}
	deletedIDs  []string
	searchResp  *ms.SearchResponse
	searchErr   error
	searchQuery string
	searchReq   *ms.SearchRequest
}

func (f *fakeIndex) AddDocumentsWithContext(_ context.Context, docs interface{}, _ *ms.DocumentOptions) (*ms.TaskInfo, error) {
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

	doc := ToDocument(item)

	assert.Equal(t, "item-1", doc.ID)
	assert.Equal(t, "lib-1", doc.LibraryID)
	assert.Equal(t, "movie", doc.Type)
	assert.Equal(t, "Test Movie", doc.Title)
	assert.Equal(t, "Test Original", doc.OriginalTitle)
	assert.Equal(t, "A test movie", doc.Overview)
	assert.Equal(t, []string{"Action", "Comedy"}, doc.Genres)
	assert.Equal(t, []string{"Studio A"}, doc.Studios)
	assert.Equal(t, []string{"tag1"}, doc.Tags)
	assert.Equal(t, &year, doc.Year)
	assert.Equal(t, now.Unix(), doc.AddedAt)
}

func TestToDocument_NilYear(t *testing.T) {
	item := &library.MediaItem{
		ID:      "item-2",
		Title:   "No Year",
		AddedAt: time.Now(),
	}
	doc := ToDocument(item)
	assert.Nil(t, doc.Year)
}

func TestBuildFilterString_Empty(t *testing.T) {
	assert.Equal(t, "", BuildFilterString(nil))
	assert.Equal(t, "", BuildFilterString(map[string]string{}))
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
	adapter := newSearchAdapterWithIndex(fake)

	item := &library.MediaItem{
		ID:      "item-1",
		Title:   "Test",
		AddedAt: time.Now(),
	}

	err := adapter.IndexMediaItem(context.Background(), item)
	require.NoError(t, err)
	require.Len(t, fake.addedDocs, 1)

	docs, ok := fake.addedDocs[0].([]mediaDocument)
	require.True(t, ok)
	assert.Equal(t, "item-1", docs[0].ID)
	assert.Equal(t, "Test", docs[0].Title)
}

func TestRemoveMediaItem(t *testing.T) {
	fake := &fakeIndex{}
	adapter := newSearchAdapterWithIndex(fake)

	err := adapter.RemoveMediaItem(context.Background(), "item-42")
	require.NoError(t, err)
	assert.Equal(t, []string{"item-42"}, fake.deletedIDs)
}

func TestSearch(t *testing.T) {
	idRaw, _ := json.Marshal("abc-123")
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{
			Hits: ms.Hits{
				ms.Hit{"id": json.RawMessage(idRaw)},
			},
		},
	}
	adapter := newSearchAdapterWithIndex(fake)

	ids, err := adapter.Search(context.Background(), "test query", map[string]string{"type": "movie"})
	require.NoError(t, err)
	assert.Equal(t, []string{"abc-123"}, ids)
	assert.Equal(t, "test query", fake.searchQuery)
	assert.Contains(t, fake.searchReq.Filter, "type = 'movie'")
}

func TestSearch_NoFilters(t *testing.T) {
	fake := &fakeIndex{
		searchResp: &ms.SearchResponse{
			Hits: ms.Hits{},
		},
	}
	adapter := newSearchAdapterWithIndex(fake)

	ids, err := adapter.Search(context.Background(), "hello", nil)
	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Nil(t, fake.searchReq.Filter)
}

func TestSearch_Error(t *testing.T) {
	fake := &fakeIndex{
		searchErr: assert.AnError,
	}
	adapter := newSearchAdapterWithIndex(fake)

	ids, err := adapter.Search(context.Background(), "fail", nil)
	assert.Error(t, err)
	assert.Nil(t, ids)
}
