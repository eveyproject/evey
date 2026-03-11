package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTVMazeTestServer(handler http.HandlerFunc) (*httptest.Server, *TVMazeProvider) {
	srv := httptest.NewServer(handler)
	p := NewTVMazeProvider(srv.Client())
	p.baseURL = srv.URL
	return srv, p
}

func TestTVMaze_Name(t *testing.T) {
	p := NewTVMazeProvider(nil)
	assert.Equal(t, "tvmaze", p.Name())
}

func TestTVMaze_Search_SkipsMovies(t *testing.T) {
	p := NewTVMazeProvider(nil)
	results, err := p.Search(context.Background(), "Inception", nil, "movie")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestTVMaze_Search(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Dexter", r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode([]tvmazeSearchResult{
			{
				Score: 0.9,
				Show: tvmazeShow{
					ID:        161,
					Name:      "Dexter",
					Summary:   "<p>He's smart, he's good looking.</p>",
					Genres:    []string{"Drama", "Crime"},
					Premiered: "2006-10-01",
					Rating:    tvmazeRating{Average: 8.4},
					Image:     &tvmazeImage{Original: "http://img.com/dexter.jpg"},
					Externals: tvmazeExternal{IMDB: "tt0773262", TheTVDB: 79349},
				},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Dexter", nil, "tv")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Dexter", r.Title)
	assert.Equal(t, "He's smart, he's good looking.", r.Overview)
	assert.Equal(t, "161", r.ProviderIDs["tvmaze"])
	assert.Equal(t, "tt0773262", r.ProviderIDs["imdb"])
	assert.Equal(t, "79349", r.ProviderIDs["thetvdb"])
	assert.Equal(t, 2006, *r.Year)
	assert.InDelta(t, 8.4, *r.CommunityRating, 0.01)
	require.Len(t, r.Artwork, 1)
	assert.Equal(t, "tvmaze", r.Artwork[0].Provider)
}

func TestTVMaze_LookupByExternalID(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tt0773262", r.URL.Query().Get("imdb"))
		json.NewEncoder(w).Encode(tvmazeShow{
			ID:        161,
			Name:      "Dexter",
			Externals: tvmazeExternal{IMDB: "tt0773262", TheTVDB: 79349},
		})
	})
	defer srv.Close()

	result, err := p.LookupByExternalID(context.Background(), "imdb", "tt0773262")
	require.NoError(t, err)
	assert.Equal(t, "79349", result.ProviderIDs["thetvdb"])
}

func TestTVMaze_GetEpisodeDetails(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "1", r.URL.Query().Get("season"))
		assert.Equal(t, "1", r.URL.Query().Get("number"))
		json.NewEncoder(w).Encode(tvmazeEpisode{
			ID:      11596,
			Name:    "Dexter",
			Season:  1,
			Number:  1,
			Airdate: "2006-10-01",
			Summary: "<p>Dexter investigates a serial killer.</p>",
			Rating:  tvmazeRating{Average: 8.1},
		})
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "161", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, "Dexter", result.Title)
	assert.Equal(t, "Dexter investigates a serial killer.", result.Overview)
	assert.InDelta(t, 8.1, *result.CommunityRating, 0.01)
}

func TestStripHTML(t *testing.T) {
	assert.Equal(t, "Hello world.", stripHTML("<p>Hello <b>world</b>.</p>"))
	assert.Equal(t, "", stripHTML(""))
	assert.Equal(t, "plain text", stripHTML("plain text"))
}
