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
	results, err := p.Search(context.Background(), "Some Movie", nil, "movie")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestTVMaze_Search(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Some Series", r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode([]tvmazeSearchResult{
			{
				Score: 0.9,
				Show: tvmazeShow{
					ID:        161,
					Name:      "Some Series",
					Summary:   "<p>He's smart, he's good looking.</p>",
					Genres:    []string{"Drama", "Crime"},
					Premiered: "2006-10-01",
					Rating:    tvmazeRating{Average: 8.4},
					Image:     &tvmazeImage{Original: "http://img.com/some-series.jpg"},
					Externals: tvmazeExternal{IMDB: "tt0773262", TheTVDB: 79349},
				},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Some Series", nil, "tv")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Some Series", r.Title)
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
			Name:      "Some Series",
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
			Name:    "Some Series",
			Season:  1,
			Number:  1,
			Airdate: "2006-10-01",
			Summary: "<p>Some Series investigates a serial killer.</p>",
			Rating:  tvmazeRating{Average: 8.1},
		})
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "161", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, "Some Series", result.Title)
	assert.Equal(t, "Some Series investigates a serial killer.", result.Overview)
	assert.InDelta(t, 8.1, *result.CommunityRating, 0.01)
}

func TestStripHTML(t *testing.T) {
	assert.Equal(t, "Hello world.", stripHTML("<p>Hello <b>world</b>.</p>"))
	assert.Empty(t, stripHTML(""))
	assert.Equal(t, "plain text", stripHTML("plain text"))
}

func TestTVMaze_GetByProviderID(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/shows/161")
		json.NewEncoder(w).Encode(tvmazeShowDetail{
			tvmazeShow: tvmazeShow{
				ID:        161,
				Name:      "Some Series",
				Summary:   "<p>A serial killer.</p>",
				Genres:    []string{"Drama", "Crime"},
				Premiered: "2006-10-01",
				Rating:    tvmazeRating{Average: 8.4},
				Image:     &tvmazeImage{Original: "http://img.com/some-series.jpg"},
				Externals: tvmazeExternal{IMDB: "tt0773262", TheTVDB: 79349},
			},
			Embedded: &tvmazeEmbedded{
				Cast: []tvmazeCastMember{
					{
						Person:    tvmazePerson{ID: 1, Name: "Actor One", Image: &tvmazeImage{Medium: "http://img.com/hall.jpg"}},
						Character: tvmazeCharacter{Name: "Some Character"},
					},
					{
						Person:    tvmazePerson{ID: 2, Name: "Actor Two"},
						Character: tvmazeCharacter{Name: ""},
					},
				},
			},
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "161", "tv")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Some Series", result.Title)
	assert.Equal(t, "A serial killer.", result.Overview)
	assert.Equal(t, "161", result.ProviderIDs["tvmaze"])
	assert.Equal(t, "tt0773262", result.ProviderIDs["imdb"])

	require.Len(t, result.People, 2)
	assert.Equal(t, "Actor One", result.People[0].Name)
	assert.Equal(t, "Some Character", result.People[0].Character)
	assert.Equal(t, "http://img.com/hall.jpg", result.People[0].ThumbURL)
	assert.Equal(t, "Actor Two", result.People[1].Name)
	assert.Empty(t, result.People[1].Character)
	assert.Empty(t, result.People[1].ThumbURL)
}

func TestTVMaze_GetByProviderID_SkipsMovies(t *testing.T) {
	p := NewTVMazeProvider(nil)
	result, err := p.GetByProviderID(context.Background(), "161", "movie")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestTVMaze_GetByProviderID_NoCast(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tvmazeShowDetail{
			tvmazeShow: tvmazeShow{ID: 1, Name: "Show"},
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "1", "tv")
	require.NoError(t, err)
	assert.Empty(t, result.People)
}

func TestTVMaze_GetByProviderID_ServerError(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "1", "tv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestTVMaze_GetByProviderID_NotFound(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "999999", "tv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTVMaze_LookupByExternalID_TheTVDB(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "79349", r.URL.Query().Get("thetvdb"))
		json.NewEncoder(w).Encode(tvmazeShow{
			ID:        161,
			Name:      "Some Series",
			Externals: tvmazeExternal{TheTVDB: 79349},
		})
	})
	defer srv.Close()

	result, err := p.LookupByExternalID(context.Background(), "thetvdb", "79349")
	require.NoError(t, err)
	assert.Equal(t, "Some Series", result.Title)
}

func TestTVMaze_LookupByExternalID_UnsupportedProvider(t *testing.T) {
	p := NewTVMazeProvider(nil)
	_, err := p.LookupByExternalID(context.Background(), "tmdb", "12345")
	assert.ErrorContains(t, err, "unsupported external provider")
}

func TestTVMaze_Search_ServerError(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})
	defer srv.Close()

	_, err := p.Search(context.Background(), "test", nil, "tv")
	assert.Error(t, err)
}

func TestTVMaze_GetEpisodeDetails_WithImage(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tvmazeEpisode{
			ID:      1,
			Name:    "Pilot",
			Summary: "<p>First episode</p>",
			Airdate: "2020-01-01",
			Rating:  tvmazeRating{Average: 0},
			Image:   &tvmazeImage{Original: "http://img.com/ep.jpg"},
		})
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "100", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, "Pilot", result.Title)
	assert.Nil(t, result.CommunityRating)
	require.Len(t, result.Artwork, 1)
	assert.Equal(t, "backdrop", result.Artwork[0].Type)
	assert.Equal(t, "tvmaze", result.Artwork[0].Provider)
}

func TestTVMaze_GetEpisodeDetails_NoOptionalFields(t *testing.T) {
	srv, p := newTVMazeTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tvmazeEpisode{
			ID:   1,
			Name: "Ep",
		})
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "100", 1, 1)
	require.NoError(t, err)
	assert.Nil(t, result.CommunityRating)
	assert.Empty(t, result.Artwork)
	assert.Nil(t, result.PremiereDate)
}

func TestTVMaze_MapShow_NoImage(t *testing.T) {
	show := tvmazeShow{ID: 1, Name: "Show", Image: nil}
	result := mapTVMazeShow(show)
	assert.Empty(t, result.Artwork)
}

func TestTVMaze_MapShow_NoExternals(t *testing.T) {
	show := tvmazeShow{ID: 1, Name: "Show"}
	result := mapTVMazeShow(show)
	assert.NotContains(t, result.ProviderIDs, "imdb")
	assert.NotContains(t, result.ProviderIDs, "thetvdb")
}

func TestTVMaze_MapShow_NoRating(t *testing.T) {
	show := tvmazeShow{ID: 1, Name: "Show", Rating: tvmazeRating{Average: 0}}
	result := mapTVMazeShow(show)
	assert.Nil(t, result.CommunityRating)
}
