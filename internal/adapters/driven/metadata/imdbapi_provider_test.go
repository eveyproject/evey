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

func newIMDBAPITestServer(handler http.HandlerFunc) (*httptest.Server, *IMDBAPIProvider) {
	srv := httptest.NewServer(handler)
	p := NewIMDBAPIProvider(srv.Client())
	p.baseURL = srv.URL
	return srv, p
}

func TestIMDBAPIProvider_Name(t *testing.T) {
	p := NewIMDBAPIProvider(nil)
	assert.Equal(t, "imdbapi", p.Name())
}

func TestIMDBAPIProvider_Search(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/search/titles")
		assert.Equal(t, "Inception", r.URL.Query().Get("query"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		json.NewEncoder(w).Encode(imdbSearchResponse{
			Results: []imdbTitle{
				{
					ID:            "tt1375666",
					PrimaryTitle:  "Inception",
					OriginalTitle: "Inception",
					Plot:          "A thief who steals corporate secrets.",
					StartYear:     2010,
					Genres:        []string{"Action", "Sci-Fi", "Thriller"},
					Rating:        &imdbRating{AggregateRating: 8.8, VoteCount: 2400000},
					PrimaryImage:  &imdbImage{URL: "http://img.com/inception.jpg", Width: 400, Height: 600},
					Directors:     []imdbName{{ID: "nm0634240", DisplayName: "Christopher Nolan"}},
					Stars:         []imdbName{{ID: "nm0000138", DisplayName: "Leonardo DiCaprio"}},
					Writers:       []imdbName{{ID: "nm0634240", DisplayName: "Christopher Nolan"}},
				},
				{
					ID:           "tt2980516",
					PrimaryTitle: "Inception: The Cobol Job",
					StartYear:    2010,
					Genres:       []string{"Animation"},
				},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Inception", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 2)

	r := results[0]
	assert.Equal(t, "Inception", r.Title)
	assert.Equal(t, "Inception", r.OriginalTitle)
	assert.Equal(t, "A thief who steals corporate secrets.", r.Overview)
	assert.Equal(t, "tt1375666", r.ProviderIDs["imdb"])
	require.NotNil(t, r.Year)
	assert.Equal(t, 2010, *r.Year)
	assert.Equal(t, []string{"Action", "Sci-Fi", "Thriller"}, r.Genres)
	require.NotNil(t, r.CommunityRating)
	assert.InDelta(t, 8.8, *r.CommunityRating, 0.01)

	require.Len(t, r.Artwork, 1)
	assert.Equal(t, "poster", r.Artwork[0].Type)
	assert.Equal(t, "http://img.com/inception.jpg", r.Artwork[0].URL)
	assert.Equal(t, 400, r.Artwork[0].Width)
	assert.Equal(t, 600, r.Artwork[0].Height)

	directors := filterPeople(r.People, "Director")
	actors := filterPeople(r.People, "Actor")
	writers := filterPeople(r.People, "Writer")
	require.Len(t, directors, 1)
	assert.Equal(t, "Christopher Nolan", directors[0].Name)
	require.Len(t, actors, 1)
	assert.Equal(t, "Leonardo DiCaprio", actors[0].Name)
	require.Len(t, writers, 1)
	assert.Equal(t, "Christopher Nolan", writers[0].Name)

	assert.Equal(t, "Inception: The Cobol Job", results[1].Title)
}

func TestIMDBAPIProvider_Search_Empty(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(imdbSearchResponse{Results: []imdbTitle{}})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "xyznonexistent", nil, "movie")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestIMDBAPIProvider_GetByProviderID(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/titles/tt0111161", r.URL.Path)
		json.NewEncoder(w).Encode(imdbTitle{
			ID:            "tt0111161",
			Type:          "movie",
			PrimaryTitle:  "The Shawshank Redemption",
			OriginalTitle: "The Shawshank Redemption",
			Plot:          "Two imprisoned men bond over a number of years.",
			StartYear:     1994,
			Genres:        []string{"Drama"},
			Rating:        &imdbRating{AggregateRating: 9.3, VoteCount: 2700000},
			PrimaryImage:  &imdbImage{URL: "http://img.com/shawshank.jpg", Width: 500, Height: 740},
			Directors:     []imdbName{{ID: "nm0001104", DisplayName: "Frank Darabont"}},
			Stars: []imdbName{
				{ID: "nm0000209", DisplayName: "Tim Robbins"},
				{ID: "nm0000151", DisplayName: "Morgan Freeman"},
			},
			Writers: []imdbName{{ID: "nm0000175", DisplayName: "Stephen King"}},
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt0111161", "movie")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "The Shawshank Redemption", result.Title)
	assert.Equal(t, "The Shawshank Redemption", result.OriginalTitle)
	assert.Equal(t, "Two imprisoned men bond over a number of years.", result.Overview)
	assert.Equal(t, "tt0111161", result.ProviderIDs["imdb"])
	require.NotNil(t, result.Year)
	assert.Equal(t, 1994, *result.Year)
	require.NotNil(t, result.PremiereDate)
	assert.Equal(t, 1994, result.PremiereDate.Year())
	assert.Equal(t, []string{"Drama"}, result.Genres)
	require.NotNil(t, result.CommunityRating)
	assert.InDelta(t, 9.3, *result.CommunityRating, 0.01)

	require.Len(t, result.Artwork, 1)
	assert.Equal(t, "poster", result.Artwork[0].Type)
	assert.Equal(t, "http://img.com/shawshank.jpg", result.Artwork[0].URL)

	directors := filterPeople(result.People, "Director")
	actors := filterPeople(result.People, "Actor")
	writers := filterPeople(result.People, "Writer")
	require.Len(t, directors, 1)
	assert.Equal(t, "Frank Darabont", directors[0].Name)
	require.Len(t, actors, 2)
	assert.Equal(t, "Tim Robbins", actors[0].Name)
	assert.Equal(t, "Morgan Freeman", actors[1].Name)
	require.Len(t, writers, 1)
	assert.Equal(t, "Stephen King", writers[0].Name)
}

func TestIMDBAPIProvider_GetByProviderID_NotFound(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt9999999", "movie")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "404")
}

func TestIMDBAPIProvider_Search_ServerError(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Inception", nil, "movie")
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "500")
}
