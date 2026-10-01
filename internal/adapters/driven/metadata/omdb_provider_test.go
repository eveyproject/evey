package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/evey-project/evey/internal/domain/metadata"
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
		assert.Equal(t, "Some Movie", r.URL.Query().Get("s"))
		assert.Equal(t, "movie", r.URL.Query().Get("type"))
		json.NewEncoder(w).Encode(omdbSearchResponse{
			Response: "True",
			Search: []omdbSearchItem{
				{Title: "Some Movie", Year: "2010", ImdbID: "tt1375666", Poster: "http://img.com/poster.jpg"},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Some Movie", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Some Movie", results[0].Title)
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

	results, err := p.Search(context.Background(), "Some Series", nil, "tv")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestOMDb_GetByProviderID_Ratings(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tt1375666", r.URL.Query().Get("i"))
		assert.Equal(t, "full", r.URL.Query().Get("plot"))
		json.NewEncoder(w).Encode(omdbDetail{
			Title:    "Some Movie",
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

	assert.Equal(t, "Some Movie", result.Title)
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
	require.NoError(t, err)
	assert.Nil(t, results)

	result, err := p.GetByProviderID(context.Background(), "tt123", "movie")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestOMDb_SeriesYearParsing(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(omdbDetail{
			Title:    "Some Series",
			Year:     "2006-2013",
			ImdbID:   "tt0773262",
			Response: "True",
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt0773262", "tv")
	require.NoError(t, err)
	assert.Equal(t, 2006, *result.Year)
}

func TestOMDb_GetByProviderID_ErrorResponse(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(omdbDetail{
			Response: "False",
			Error:    "Movie not found!",
		})
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "tt0000000", "movie")
	assert.ErrorContains(t, err, "Movie not found!")
}

func TestOMDb_GetByProviderID_ServerError(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "tt123", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestOMDb_Search_WithYear(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "2010", r.URL.Query().Get("y"))
		json.NewEncoder(w).Encode(omdbSearchResponse{Response: "False"})
	})
	defer srv.Close()

	y := 2010
	_, err := p.Search(context.Background(), "Some Movie", &y, "movie")
	require.NoError(t, err)
}

func TestOMDb_Search_NAPoster(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(omdbSearchResponse{
			Response: "True",
			Search: []omdbSearchItem{
				{Title: "Movie", Year: "2020", ImdbID: "tt1", Poster: "N/A"},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Movie", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Artwork)
}

func TestOMDb_Search_InvalidYear(t *testing.T) {
	srv, p := newOMDbTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(omdbSearchResponse{
			Response: "True",
			Search: []omdbSearchItem{
				{Title: "Movie", Year: "N/A", ImdbID: "tt1"},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Movie", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0].Year)
}

func TestOMDb_MapDetail_NAFields(t *testing.T) {
	d := omdbDetail{
		Title:    "Movie",
		Plot:     "N/A",
		Rated:    "N/A",
		Genre:    "N/A",
		Director: "N/A",
		Writer:   "N/A",
		Actors:   "N/A",
		Poster:   "N/A",
		ImdbID:   "tt1",
		Response: "True",
	}
	result := mapOMDbDetail(d)
	assert.Empty(t, result.Overview)
	assert.Empty(t, result.OfficialRating)
	assert.Empty(t, result.Genres)
	assert.Empty(t, result.People)
	assert.Empty(t, result.Artwork)
}

func TestOMDb_MapDetail_WriterParentheses(t *testing.T) {
	d := omdbDetail{
		Title:    "Movie",
		Writer:   "John Smith (screenplay), Jane Doe (novel)",
		ImdbID:   "tt1",
		Response: "True",
	}
	result := mapOMDbDetail(d)
	writers := filterPeople(result.People, "Writer")
	require.Len(t, writers, 2)
	assert.Equal(t, "John Smith", writers[0].Name)
	assert.Equal(t, "Jane Doe", writers[1].Name)
}

func TestOMDb_MapDetail_IMDBRatingSetsCommunityRating(t *testing.T) {
	d := omdbDetail{
		Title:  "Movie",
		ImdbID: "tt1",
		Ratings: []omdbRating{
			{Source: "Internet Movie Database", Value: "7.5/10"},
		},
		Response: "True",
	}
	result := mapOMDbDetail(d)
	require.NotNil(t, result.CommunityRating)
	assert.InDelta(t, 7.5, *result.CommunityRating, 0.01)
}

func TestParseIMDBRating_Invalid(t *testing.T) {
	_, err := parseIMDBRating("bad")
	assert.Error(t, err)
}

func TestParseFractionRating_Invalid(t *testing.T) {
	_, err := parseFractionRating("bad")
	assert.Error(t, err)
}

func TestNaToEmpty(t *testing.T) {
	assert.Empty(t, naToEmpty("N/A"))
	assert.Equal(t, "hello", naToEmpty("hello"))
}

func TestSplitNames(t *testing.T) {
	assert.Nil(t, splitNames(""))
	assert.Nil(t, splitNames("N/A"))
	assert.Equal(t, []string{"A", "B"}, splitNames("A, B"))
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

// OMDb takes its key in the query string, and a transport error (*url.Error)
// quotes the whole URL: the key must not ride along into errors and logs.
func TestOMDb_TransportErrorDoesNotCarryTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // a refused connection is the transport error we want
	p := NewOMDbProvider("s3cr3t-omdb-key", nil)
	p.baseURL = srv.URL
	_, err := p.Search(context.Background(), "Alien", nil, "movie")
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), "s3cr3t-omdb-key") {
		t.Fatalf("API key leaked into the error: %v", err)
	}
	if _, err := p.GetByProviderID(context.Background(), "tt0078748", "movie"); err == nil || strings.Contains(err.Error(), "s3cr3t-omdb-key") {
		t.Fatalf("detail error: %v", err)
	}
}
