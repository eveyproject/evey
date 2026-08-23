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
		assert.Equal(t, "Some Movie", r.URL.Query().Get("query"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		json.NewEncoder(w).Encode(imdbSearchResponse{
			Results: []imdbTitle{
				{
					ID:            "tt0000001",
					Type:          "movie",
					PrimaryTitle:  "Some Movie",
					OriginalTitle: "Some Movie",
					Plot:          "A group pulls off a daring heist.",
					StartYear:     2010,
					Genres:        []string{"Action", "Sci-Fi", "Thriller"},
					Rating:        &imdbRating{AggregateRating: 8.8, VoteCount: 2400000},
					PrimaryImage:  &imdbImage{URL: "http://img.com/some-movie.jpg", Width: 400, Height: 600},
					Directors:     []imdbName{{ID: "nm0000001", DisplayName: "Director One"}},
					Stars:         []imdbName{{ID: "nm0000002", DisplayName: "Actor One"}},
					Writers:       []imdbName{{ID: "nm0000001", DisplayName: "Director One"}},
				},
				{
					ID:           "tt0000002",
					Type:         "movie",
					PrimaryTitle: "Some Movie: The Origins Job",
					StartYear:    2010,
					Genres:       []string{"Animation"},
				},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Some Movie", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 2)

	r := results[0]
	assert.Equal(t, "Some Movie", r.Title)
	assert.Equal(t, "Some Movie", r.OriginalTitle)
	assert.Equal(t, "A group pulls off a daring heist.", r.Overview)
	assert.Equal(t, "tt0000001", r.ProviderIDs["imdb"])
	require.NotNil(t, r.Year)
	assert.Equal(t, 2010, *r.Year)
	assert.Equal(t, []string{"Action", "Sci-Fi", "Thriller"}, r.Genres)
	require.NotNil(t, r.CommunityRating)
	assert.InDelta(t, 8.8, *r.CommunityRating, 0.01)

	require.Len(t, r.Artwork, 1)
	assert.Equal(t, "poster", r.Artwork[0].Type)
	assert.Equal(t, "http://img.com/some-movie.jpg", r.Artwork[0].URL)
	assert.Equal(t, 400, r.Artwork[0].Width)
	assert.Equal(t, 600, r.Artwork[0].Height)

	directors := filterPeople(r.People, "Director")
	actors := filterPeople(r.People, "Actor")
	writers := filterPeople(r.People, "Writer")
	require.Len(t, directors, 1)
	assert.Equal(t, "Director One", directors[0].Name)
	require.Len(t, actors, 1)
	assert.Equal(t, "Actor One", actors[0].Name)
	require.Len(t, writers, 1)
	assert.Equal(t, "Director One", writers[0].Name)

	assert.Equal(t, "Some Movie: The Origins Job", results[1].Title)
}

func TestIMDBAPIProvider_Search_FiltersMoviesForTVType(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(imdbSearchResponse{
			Results: []imdbTitle{
				{ID: "tt0000020", Type: "movie", PrimaryTitle: "Push", StartYear: 2017},
				{ID: "tt0000021", Type: "tvMiniSeries", PrimaryTitle: "Push", StartYear: 1990},
				{ID: "tt0000022", Type: "tvSeries", PrimaryTitle: "Push", StartYear: 2024},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Push", nil, "tv")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "tt0000021", results[0].ProviderIDs["imdb"])
	assert.Equal(t, "tt0000022", results[1].ProviderIDs["imdb"])
}

func TestIMDBAPIProvider_Search_FiltersTVForMovieType(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(imdbSearchResponse{
			Results: []imdbTitle{
				{ID: "tt0000020", Type: "movie", PrimaryTitle: "Push", StartYear: 2017},
				{ID: "tt0000021", Type: "tvMiniSeries", PrimaryTitle: "Push", StartYear: 1990},
			},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Push", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "tt0000020", results[0].ProviderIDs["imdb"])
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
		assert.Equal(t, "/titles/tt0000003", r.URL.Path)
		json.NewEncoder(w).Encode(imdbTitle{
			ID:            "tt0000003",
			Type:          "movie",
			PrimaryTitle:  "Some Prison Story",
			OriginalTitle: "Some Prison Story",
			Plot:          "A short synopsis used only as a fixture.",
			StartYear:     1994,
			Genres:        []string{"Drama"},
			Rating:        &imdbRating{AggregateRating: 9.3, VoteCount: 2700000},
			PrimaryImage:  &imdbImage{URL: "http://img.com/some-prison-story.jpg", Width: 500, Height: 740},
			Directors:     []imdbName{{ID: "nm0000003", DisplayName: "Director Two"}},
			Stars: []imdbName{
				{ID: "nm0000004", DisplayName: "Actor Two"},
				{ID: "nm0000005", DisplayName: "Actor Three"},
			},
			Writers: []imdbName{{ID: "nm0000006", DisplayName: "Writer One"}},
		})
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "tt0000003", "movie")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "Some Prison Story", result.Title)
	assert.Equal(t, "Some Prison Story", result.OriginalTitle)
	assert.Equal(t, "A short synopsis used only as a fixture.", result.Overview)
	assert.Equal(t, "tt0000003", result.ProviderIDs["imdb"])
	require.NotNil(t, result.Year)
	assert.Equal(t, 1994, *result.Year)
	require.NotNil(t, result.PremiereDate)
	assert.Equal(t, 1994, result.PremiereDate.Year())
	assert.Equal(t, []string{"Drama"}, result.Genres)
	require.NotNil(t, result.CommunityRating)
	assert.InDelta(t, 9.3, *result.CommunityRating, 0.01)

	require.Len(t, result.Artwork, 1)
	assert.Equal(t, "poster", result.Artwork[0].Type)
	assert.Equal(t, "http://img.com/some-prison-story.jpg", result.Artwork[0].URL)

	directors := filterPeople(result.People, "Director")
	actors := filterPeople(result.People, "Actor")
	writers := filterPeople(result.People, "Writer")
	require.Len(t, directors, 1)
	assert.Equal(t, "Director Two", directors[0].Name)
	require.Len(t, actors, 2)
	assert.Equal(t, "Actor Two", actors[0].Name)
	assert.Equal(t, "Actor Three", actors[1].Name)
	require.Len(t, writers, 1)
	assert.Equal(t, "Writer One", writers[0].Name)
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

func TestImdbTypeMatches(t *testing.T) {
	tests := []struct {
		imdbType  string
		mediaType string
		want      bool
	}{
		{"movie", "movie", true},
		{"tvMovie", "movie", true},
		{"tvSeries", "movie", false},
		{"tvMiniSeries", "movie", false},
		{"tvSeries", "tv", true},
		{"tvMiniSeries", "tv", true},
		{"tvSpecial", "tv", true},
		{"movie", "tv", false},
		{"movie", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.imdbType+"_"+tt.mediaType, func(t *testing.T) {
			assert.Equal(t, tt.want, imdbTypeMatches(tt.imdbType, tt.mediaType))
		})
	}
}

func TestIMDBAPIProvider_Search_ServerError(t *testing.T) {
	srv, p := newIMDBAPITestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "Some Movie", nil, "movie")
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "500")
}
