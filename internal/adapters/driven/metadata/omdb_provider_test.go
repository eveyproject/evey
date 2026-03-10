package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOMDbTestServer(handler http.HandlerFunc) (*httptest.Server, *OMDbProvider) {
	srv := httptest.NewServer(handler)
	p := NewOMDbProvider("testkey", srv.Client())
	p.baseURL = srv.URL
	return srv, p
}

func TestOMDb_Name(t *testing.T) {
	p := NewOMDbProvider("key", nil)
	assert.Equal(t, "omdb", p.Name())
}

func TestOMDb_Search(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "testkey", r.URL.Query().Get("apikey"))
		assert.Equal(t, "Inception", r.URL.Query().Get("s"))
		assert.Equal(t, "movie", r.URL.Query().Get("type"))
		json.NewEncoder(w).Encode(omdbSearchResponse{
			Response: "True",
			Search: []omdbSearchItem{
				{Title: "Inception", Year: "2010", ImdbID: "tt1375666", Poster: "http://img.com/poster.jpg"},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Inception", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Inception", results[0].Title)
	assert.Equal(t, "tt1375666", results[0].ProviderIDs["imdb"])
	assert.Equal(t, 2010, *results[0].Year)
	require.Len(t, results[0].Artwork, 1)
	assert.Equal(t, "omdb", results[0].Artwork[0].Provider)
}

func TestOMDb_Search_TVMapsToSeries(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "series", r.URL.Query().Get("type"))
		json.NewEncoder(w).Encode(omdbSearchResponse{Response: "False"})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Dexter", nil, "tv")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestOMDb_GetByProviderID_Ratings(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tt1375666", r.URL.Query().Get("i"))
		assert.Equal(t, "full", r.URL.Query().Get("plot"))
		json.NewEncoder(w).Encode(omdbDetail{
			Title:    "Inception",
			Year:     "2010",
			Rated:    "PG-13",
			Genre:    "Action, Adventure, Sci-Fi",
			Director: "Christopher Nolan",
			Writer:   "Christopher Nolan",
			Actors:   "Leonardo DiCaprio, Joseph Gordon-Levitt, Elliot Page",
			Plot:     "A thief who steals corporate secrets...",
			ImdbID:   "tt1375666",
			Poster:   "http://img.com/poster.jpg",
			Ratings: []omdbRating{
				{Source: "Internet Movie Database", Value: "8.8/10"},
				{Source: "Rotten Tomatoes", Value: "87%"},
				{Source: "Metacritic", Value: "74/100"},
			},
			Response: "True",
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt1375666", "movie")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "Inception", result.Title)
	assert.Equal(t, "A thief who steals corporate secrets...", result.Overview)
	assert.Equal(t, "PG-13", result.OfficialRating)
	assert.Equal(t, "tt1375666", result.ProviderIDs["imdb"])

	assert.InDelta(t, 8.8, result.Ratings["imdb"], 0.01)
	assert.InDelta(t, 87.0, result.Ratings["rt"], 0.01)
	assert.InDelta(t, 74.0, result.Ratings["metacritic"], 0.01)

	assert.Equal(t, []string{"Action", "Adventure", "Sci-Fi"}, result.Genres)

	// People
	directors := filterPeople(result.People, "Director")
	actors := filterPeople(result.People, "Actor")
	assert.Len(t, directors, 1)
	assert.Equal(t, "Christopher Nolan", directors[0].Name)
	assert.Len(t, actors, 3)
}

func TestOMDb_NoAPIKey(t *testing.T) {
	p := NewOMDbProvider("", nil)
	results, err := p.Search(context.Background(), "test", nil, "movie")
	assert.NoError(t, err)
	assert.Nil(t, results)

	result, err := p.GetByProviderID(context.Background(), "tt123", "movie")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestOMDb_SeriesYearParsing(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(omdbDetail{
			Title:    "Dexter",
			Year:     "2006–2013",
			ImdbID:   "tt0773262",
			Response: "True",
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt0773262", "tv")
	require.NoError(t, err)
	assert.Equal(t, 2006, *result.Year)
}

func filterPeople(people []domain.PersonResult, role string) []domain.PersonResult {
	var out []domain.PersonResult
	for _, p := range people {
		if p.Role == role {
			out = append(out, p)
		}
	}
	return out
}
