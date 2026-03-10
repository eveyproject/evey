package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, *TMDBProvider) {
	srv := httptest.NewServer(handler)
	p := NewTMDBProvider("test-key", "en", srv.Client())
	p.baseURL = srv.URL
	return srv, p
}

func TestName(t *testing.T) {
	p := NewTMDBProvider("key", "en", nil)
	assert.Equal(t, "tmdb", p.Name())
}

func TestSearch_MovieResponseMapping(t *testing.T) {
	resp := tmdbSearchResponse{
		Page:       1,
		TotalPages: 1,
		Results: []tmdbSearchItem{
			{
				ID:            550,
				Title:         "Fight Club",
				OriginalTitle: "Fight Club",
				Overview:      "An insomniac office worker...",
				ReleaseDate:   "1999-10-15",
				VoteAverage:   8.4,
				GenreIDs:      []int{18, 53},
			},
		},
	}

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.True(t, strings.HasPrefix(r.URL.Path, "/search/movie"))
		assert.Equal(t, "fight club", r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "fight club", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Fight Club", r.Title)
	assert.Equal(t, "Fight Club", r.OriginalTitle)
	assert.Equal(t, "An insomniac office worker...", r.Overview)
	assert.Equal(t, 1999, *r.Year)
	assert.InDelta(t, 8.4, *r.CommunityRating, 0.01)
	assert.Equal(t, []string{"Drama", "Thriller"}, r.Genres)
	assert.Equal(t, "550", r.ProviderIDs["tmdb"])
}

func TestSearch_TVResponseMapping(t *testing.T) {
	resp := tmdbSearchResponse{
		Page:       1,
		TotalPages: 1,
		Results: []tmdbSearchItem{
			{
				ID:           1396,
				Name:         "Breaking Bad",
				OriginalName: "Breaking Bad",
				Overview:     "A high school chemistry teacher...",
				FirstAirDate: "2008-01-20",
				VoteAverage:  8.9,
				GenreIDs:     []int{18, 80},
			},
		},
	}

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasPrefix(r.URL.Path, "/search/tv"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "breaking bad", nil, "tv")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Breaking Bad", r.Title)
	assert.Equal(t, 2008, *r.Year)
	assert.Equal(t, []string{"Drama", "Crime"}, r.Genres)
	assert.Equal(t, "1396", r.ProviderIDs["tmdb"])
}

func TestSearch_UnsupportedMediaType(t *testing.T) {
	p := NewTMDBProvider("key", "en", nil)
	_, err := p.Search(context.Background(), "query", nil, "book")
	assert.ErrorContains(t, err, "unsupported media type")
}

func TestSearch_URLBuilding(t *testing.T) {
	var capturedPaths []string
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedPaths = append(capturedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1})
	})
	defer srv.Close()

	_, _ = p.Search(context.Background(), "test", nil, "movie")
	require.Len(t, capturedPaths, 1)
	assert.Equal(t, "/search/movie", capturedPaths[0])

	capturedPaths = nil
	_, _ = p.Search(context.Background(), "test", nil, "tv")
	require.Len(t, capturedPaths, 1)
	assert.Equal(t, "/search/tv", capturedPaths[0])
}

func TestSearch_Pagination(t *testing.T) {
	pageCount := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		resp := tmdbSearchResponse{
			Page:       pageCount,
			TotalPages: 3,
			Results: []tmdbSearchItem{
				{ID: pageCount, Title: "Movie " + r.URL.Query().Get("page")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "test", nil, "movie")
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, 3, pageCount)
}

func TestSearch_RateLimitRetry(t *testing.T) {
	attempts := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbSearchResponse{
			Page:       1,
			TotalPages: 1,
			Results:    []tmdbSearchItem{{ID: 1, Title: "Found"}},
		})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "test", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Found", results[0].Title)
	assert.Equal(t, 3, attempts)
}

func TestSearch_RateLimitExhausted(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	defer srv.Close()

	_, err := p.Search(context.Background(), "test", nil, "movie")
	assert.ErrorContains(t, err, "rate limited")
}

func TestGetByProviderID_MovieDetail(t *testing.T) {
	detail := tmdbDetailResponse{
		ID:            550,
		Title:         "Fight Club",
		OriginalTitle: "Fight Club",
		Overview:      "An insomniac office worker...",
		Tagline:       "Mischief. Mayhem. Soap.",
		ReleaseDate:   "1999-10-15",
		VoteAverage:   8.4,
		Genres:        []tmdbGenre{{ID: 18, Name: "Drama"}, {ID: 53, Name: "Thriller"}},
		ProductionCompanies: []tmdbCompany{{ID: 508, Name: "Regency Enterprises"}},
		Credits: &tmdbCredits{
			Cast: []tmdbCastMember{
				{Name: "Brad Pitt", Character: "Tyler Durden", ProfilePath: "/brad.jpg"},
			},
			Crew: []tmdbCrewMember{
				{Name: "David Fincher", Job: "Director", ProfilePath: "/fincher.jpg"},
				{Name: "Jim Uhls", Job: "Writer", ProfilePath: "/uhls.jpg"},
			},
		},
		Images: &tmdbImages{
			Posters:   []tmdbImage{{FilePath: "/poster.jpg", Width: 500, Height: 750, ISO639_1: "en"}},
			Backdrops: []tmdbImage{{FilePath: "/backdrop.jpg", Width: 1920, Height: 1080}},
		},
	}

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/movie/550", r.URL.Path)
		assert.Contains(t, r.URL.Query().Get("append_to_response"), "credits")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "550", "movie")
	require.NoError(t, err)

	assert.Equal(t, "Fight Club", result.Title)
	assert.Equal(t, "Mischief. Mayhem. Soap.", result.Tagline)
	assert.Equal(t, 1999, *result.Year)
	assert.Equal(t, []string{"Drama", "Thriller"}, result.Genres)
	assert.Equal(t, []string{"Regency Enterprises"}, result.Studios)
	assert.Equal(t, "550", result.ProviderIDs["tmdb"])

	require.Len(t, result.People, 2)
	assert.Equal(t, "Brad Pitt", result.People[0].Name)
	assert.Equal(t, "Actor", result.People[0].Role)
	assert.Equal(t, "Tyler Durden", result.People[0].Character)
	assert.Equal(t, imageBaseURL+"/brad.jpg", result.People[0].ThumbURL)
	assert.Equal(t, "David Fincher", result.People[1].Name)
	assert.Equal(t, "Director", result.People[1].Role)

	require.Len(t, result.Artwork, 2)
	assert.Equal(t, "poster", result.Artwork[0].Type)
	assert.Equal(t, imageBaseURL+"/poster.jpg", result.Artwork[0].URL)
	assert.Equal(t, 500, result.Artwork[0].Width)
	assert.Equal(t, "en", result.Artwork[0].Language)
	assert.Equal(t, "backdrop", result.Artwork[1].Type)
}

func TestGetByProviderID_TVDetail(t *testing.T) {
	detail := tmdbDetailResponse{
		ID:           1396,
		Name:         "Breaking Bad",
		OriginalName: "Breaking Bad",
		Overview:     "A teacher turned meth cook",
		FirstAirDate: "2008-01-20",
		VoteAverage:  8.9,
		Genres:       []tmdbGenre{{ID: 18, Name: "Drama"}},
	}

	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tv/1396", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "1396", "tv")
	require.NoError(t, err)

	assert.Equal(t, "Breaking Bad", result.Title)
	assert.Equal(t, 2008, *result.Year)
}

func TestGetByProviderID_UnsupportedMediaType(t *testing.T) {
	p := NewTMDBProvider("key", "en", nil)
	_, err := p.GetByProviderID(context.Background(), "123", "book")
	assert.ErrorContains(t, err, "unsupported media type")
}

func TestGetByProviderID_APIError(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status_message":"not found"}`))
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "999999", "movie")
	assert.ErrorContains(t, err, "404")
}

func TestParseDate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		d := parseDate("2023-06-15")
		require.NotNil(t, d)
		assert.Equal(t, 2023, d.Year())
		assert.Equal(t, 6, int(d.Month()))
		assert.Equal(t, 15, d.Day())
	})

	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, parseDate(""))
	})

	t.Run("invalid", func(t *testing.T) {
		assert.Nil(t, parseDate("not-a-date"))
	})
}

func TestBearerAuth(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-secret-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1})
	})
	defer srv.Close()

	p.apiKey = "my-secret-key"
	_, err := p.Search(context.Background(), "test", nil, "movie")
	require.NoError(t, err)
}

func TestSearch_EmptyResults(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1})
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "nonexistent", nil, "movie")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestProfileURL(t *testing.T) {
	assert.Equal(t, imageBaseURL+"/abc.jpg", profileURL("/abc.jpg"))
	assert.Equal(t, "", profileURL(""))
}

func TestSearch_YearParamSentToAPI(t *testing.T) {
	var capturedParams []url.Values
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = append(capturedParams, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		resp := tmdbSearchResponse{Page: 1, TotalPages: 1,
			Results: []tmdbSearchItem{{ID: 1, Title: "Robin Hood", ReleaseDate: "2018-11-21"}},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	y := 2018
	results, err := p.Search(context.Background(), "Robin Hood", &y, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	// First call should have primary_release_year
	require.GreaterOrEqual(t, len(capturedParams), 1)
	assert.Equal(t, "2018", capturedParams[0].Get("primary_release_year"))

	capturedParams = nil
	results, err = p.Search(context.Background(), "Robin Hood", &y, "tv")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(capturedParams), 1)
	assert.Equal(t, "2018", capturedParams[0].Get("first_air_date_year"))
}

func TestSearch_YearFallbackWithoutYear(t *testing.T) {
	calls := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("primary_release_year") != "" {
			// Year-filtered: return empty
			json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1})
		} else {
			// Without year: return result
			json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1,
				Results: []tmdbSearchItem{{ID: 1, Title: "Fallback Movie"}},
			})
		}
	})
	defer srv.Close()

	y := 9999
	results, err := p.Search(context.Background(), "test", &y, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Fallback Movie", results[0].Title)
	assert.Equal(t, 2, calls) // first with year (empty), second without
}

func TestMapSearchResult_NoDateNoRating(t *testing.T) {
	item := tmdbSearchItem{
		ID:    1,
		Title: "No Date Movie",
	}
	r := mapSearchResult(item, "movie", movieGenres)
	assert.Nil(t, r.Year)
	assert.Nil(t, r.CommunityRating)
	assert.Nil(t, r.PremiereDate)
}
