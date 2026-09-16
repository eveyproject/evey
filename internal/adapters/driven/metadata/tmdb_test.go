package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domain "github.com/evey-project/evey/internal/domain/metadata"
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
				Title:         "Some Movie",
				OriginalTitle: "Some Movie",
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
		assert.Equal(t, "some movie", r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := p.Search(context.Background(), "some movie", nil, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Some Movie", r.Title)
	assert.Equal(t, "Some Movie", r.OriginalTitle)
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
				Name:         "Some Series",
				OriginalName: "Some Series",
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

	results, err := p.Search(context.Background(), "some series", nil, "tv")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Some Series", r.Title)
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
		ID:                  550,
		Title:               "Some Movie",
		OriginalTitle:       "Some Movie",
		Overview:            "An insomniac office worker...",
		Tagline:             "Mischief. Mayhem. Soap.",
		ReleaseDate:         "1999-10-15",
		VoteAverage:         8.4,
		Genres:              []tmdbGenre{{ID: 18, Name: "Drama"}, {ID: 53, Name: "Thriller"}},
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

	assert.Equal(t, "Some Movie", result.Title)
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
		Name:         "Some Series",
		OriginalName: "Some Series",
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

	assert.Equal(t, "Some Series", result.Title)
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
	assert.Empty(t, profileURL(""))
}

func TestSearch_YearParamSentToAPI(t *testing.T) {
	var capturedParams []url.Values
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = append(capturedParams, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		resp := tmdbSearchResponse{Page: 1, TotalPages: 1,
			Results: []tmdbSearchItem{{ID: 1, Title: "Some Movie 2", ReleaseDate: "2018-11-21"}},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	y := 2018
	results, err := p.Search(context.Background(), "Some Movie 2", &y, "movie")
	require.NoError(t, err)
	require.Len(t, results, 1)
	// First call should have primary_release_year
	require.GreaterOrEqual(t, len(capturedParams), 1)
	assert.Equal(t, "2018", capturedParams[0].Get("primary_release_year"))

	capturedParams = nil
	_, err = p.Search(context.Background(), "Some Movie 2", &y, "tv")
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

// Search results must carry their poster: the media-request search grid (and
// any identify UI) renders candidates straight from Search, without a detail
// round-trip per row - an empty Artwork left every candidate coverless.
// w500 like the collection mapper: these are grid thumbnails, not hero art.
func TestMapSearchResult_PosterArtwork(t *testing.T) {
	r := mapSearchResult(tmdbSearchItem{ID: 603, Title: "Matrix", PosterPath: "/abc.jpg"}, "movie", movieGenres)
	require.Len(t, r.Artwork, 1)
	assert.Equal(t, "poster", r.Artwork[0].Type)
	assert.Equal(t, "https://image.tmdb.org/t/p/w500/abc.jpg", r.Artwork[0].URL)
	assert.Equal(t, "tmdb", r.Artwork[0].Provider)

	rTV := mapSearchResult(tmdbSearchItem{ID: 1, Name: "Show", PosterPath: "/tv.jpg"}, "tv", nil)
	require.Len(t, rTV.Artwork, 1)
	assert.Equal(t, "https://image.tmdb.org/t/p/w500/tv.jpg", rTV.Artwork[0].URL)

	rNone := mapSearchResult(tmdbSearchItem{ID: 2, Title: "Bare"}, "movie", nil)
	assert.Empty(t, rNone.Artwork, "no poster_path → no artwork, never the bare CDN base URL")
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

func TestSelectTrailers(t *testing.T) {
	videos := []tmdbVideo{
		{Key: "a1", Site: "YouTube", Type: "Trailer", Language: "en", Official: false},
		{Key: "a2", Site: "YouTube", Type: "Trailer", Language: "en", Official: true},
		{Key: "a3", Site: "YouTube", Type: "Trailer", Language: "en", Official: true},
		{Key: "a4", Site: "YouTube", Type: "Trailer", Language: "en", Official: false},
		{Key: "f1", Site: "YouTube", Type: "Trailer", Language: "fr", Official: true},
		{Key: "", Site: "YouTube", Type: "Trailer", Language: "en"},  // empty key, skipped
		{Key: "c1", Site: "YouTube", Type: "Clip", Language: "en"},   // not a trailer, skipped
		{Key: "t1", Site: "YouTube", Type: "Teaser", Language: "en"}, // not a trailer, skipped
	}

	result := selectTrailers(videos, "en")
	// en: max 3, fr: 1
	enCount, frCount := 0, 0
	for _, tr := range result {
		if tr.Language == "en" {
			enCount++
		}
		if tr.Language == "fr" {
			frCount++
		}
		assert.Equal(t, "Trailer", tr.Type)
		assert.NotEmpty(t, tr.Key)
	}
	assert.Equal(t, 3, enCount)
	assert.Equal(t, 1, frCount)
}

func TestSelectTrailers_Empty(t *testing.T) {
	result := selectTrailers(nil, "en")
	assert.Empty(t, result)
}

func TestSelectTrailers_OfficialFirst(t *testing.T) {
	videos := []tmdbVideo{
		{Key: "unoff", Site: "YouTube", Type: "Trailer", Language: "en", Official: false},
		{Key: "off", Site: "YouTube", Type: "Trailer", Language: "en", Official: true},
	}
	result := selectTrailers(videos, "en")
	require.Len(t, result, 2)
	assert.True(t, result[0].Official)
}

func TestSortVideos(t *testing.T) {
	vids := []tmdbVideo{
		{Key: "b", Official: false},
		{Key: "a", Official: true},
		{Key: "c", Official: true},
	}
	sortVideos(vids, func(a, b tmdbVideo) int {
		if a.Official && !b.Official {
			return -1
		}
		if !a.Official && b.Official {
			return 1
		}
		return 0
	})
	assert.True(t, vids[0].Official)
	assert.True(t, vids[1].Official)
	assert.False(t, vids[2].Official)
}

func TestMapDetailCollection(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, mapDetailCollection(nil))
	})

	t.Run("with paths", func(t *testing.T) {
		col := &tmdbCollection{
			ID: 42, Name: "Saga",
			PosterPath: "/poster.jpg", BackdropPath: "/backdrop.jpg",
		}
		ref := mapDetailCollection(col)
		require.NotNil(t, ref)
		assert.Equal(t, 42, ref.TmdbID)
		assert.Equal(t, "Saga", ref.Name)
		assert.Contains(t, ref.PosterURL, "/poster.jpg")
		assert.Contains(t, ref.BackdropURL, "/backdrop.jpg")
	})

	t.Run("empty paths", func(t *testing.T) {
		col := &tmdbCollection{ID: 1, Name: "C"}
		ref := mapDetailCollection(col)
		assert.Empty(t, ref.PosterURL)
		assert.Empty(t, ref.BackdropURL)
	})
}

func TestMapDetailExternalIDs(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		r := &domain.MetadataResult{ProviderIDs: map[string]string{}}
		mapDetailExternalIDs(r, nil)
		assert.Empty(t, r.ProviderIDs)
	})

	t.Run("with imdb and tvdb", func(t *testing.T) {
		r := &domain.MetadataResult{ProviderIDs: map[string]string{"tmdb": "1"}}
		mapDetailExternalIDs(r, &tmdbExternalIDs{IMDBID: "tt123", TVDBID: 456})
		assert.Equal(t, "tt123", r.ProviderIDs["imdb"])
		assert.Equal(t, "456", r.ProviderIDs["thetvdb"])
	})

	t.Run("empty values", func(t *testing.T) {
		r := &domain.MetadataResult{ProviderIDs: map[string]string{}}
		mapDetailExternalIDs(r, &tmdbExternalIDs{})
		assert.NotContains(t, r.ProviderIDs, "imdb")
		assert.NotContains(t, r.ProviderIDs, "thetvdb")
	})
}

func TestMapDetailRating_Zero(t *testing.T) {
	r := &domain.MetadataResult{Ratings: map[string]float64{}}
	mapDetailRating(r, 0)
	assert.Nil(t, r.CommunityRating)
	assert.Empty(t, r.Ratings)
}

func TestGetEpisodeDetails(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/tv/100/season/1/episode/3")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name": "Pilot",
			"overview": "The beginning",
			"still_path": "/still.jpg",
			"vote_average": 8.5,
			"air_date": "2020-01-15"
		}`))
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "100", 1, 3)
	require.NoError(t, err)
	assert.Equal(t, "Pilot", result.Title)
	assert.Equal(t, "The beginning", result.Overview)
	require.NotNil(t, result.CommunityRating)
	assert.InDelta(t, 8.5, *result.CommunityRating, 0.01)
	require.Len(t, result.Artwork, 1)
	assert.Equal(t, "backdrop", result.Artwork[0].Type)
	assert.Contains(t, result.Artwork[0].URL, "/still.jpg")
	require.NotNil(t, result.Year)
	assert.Equal(t, 2020, *result.Year)
}

func TestGetEpisodeDetails_NotFound(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name": ""}`))
	})
	defer srv.Close()

	_, err := p.GetEpisodeDetails(context.Background(), "100", 1, 99)
	assert.ErrorContains(t, err, "episode not found")
}

func TestGetEpisodeDetails_NoOptionalFields(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name": "Ep", "overview": "", "vote_average": 0}`))
	})
	defer srv.Close()

	result, err := p.GetEpisodeDetails(context.Background(), "100", 1, 1)
	require.NoError(t, err)
	assert.Nil(t, result.CommunityRating)
	assert.Empty(t, result.Artwork)
	assert.Nil(t, result.PremiereDate)
}

func TestGetSeasonDetails(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/tv/100/season/2")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name": "Season 2",
			"overview": "Second season",
			"poster_path": "/s2poster.jpg",
			"vote_average": 7.9,
			"air_date": "2021-06-01"
		}`))
	})
	defer srv.Close()

	result, err := p.GetSeasonDetails(context.Background(), "100", 2)
	require.NoError(t, err)
	assert.Equal(t, "Season 2", result.Title)
	assert.Equal(t, "Second season", result.Overview)
	require.NotNil(t, result.CommunityRating)
	assert.InDelta(t, 7.9, *result.CommunityRating, 0.01)
	require.Len(t, result.Artwork, 1)
	assert.Equal(t, "poster", result.Artwork[0].Type)
	require.NotNil(t, result.Year)
	assert.Equal(t, 2021, *result.Year)
}

func TestGetSeasonDetails_NoOptionalFields(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name": "Season 1", "overview": ""}`))
	})
	defer srv.Close()

	result, err := p.GetSeasonDetails(context.Background(), "100", 1)
	require.NoError(t, err)
	assert.Nil(t, result.CommunityRating)
	assert.Empty(t, result.Artwork)
	assert.Nil(t, result.PremiereDate)
}

func TestGetByProviderID_WithExternalIDs(t *testing.T) {
	detail := tmdbDetailResponse{
		ID:          550,
		Title:       "Some Movie",
		ReleaseDate: "1999-10-15",
		ExternalIDs: &tmdbExternalIDs{IMDBID: "tt0137523", TVDBID: 0},
	}
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "550", "movie")
	require.NoError(t, err)
	assert.Equal(t, "tt0137523", result.ProviderIDs["imdb"])
	assert.NotContains(t, result.ProviderIDs, "thetvdb")
}

func TestGetByProviderID_WithVideos(t *testing.T) {
	detail := tmdbDetailResponse{
		ID:    1,
		Title: "Movie",
		Videos: &tmdbVideos{
			Results: []tmdbVideo{
				{Key: "abc", Site: "YouTube", Type: "Trailer", Language: "en", Official: true},
			},
		},
	}
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "1", "movie")
	require.NoError(t, err)
	require.Len(t, result.Trailers, 1)
	assert.Equal(t, "abc", result.Trailers[0].Key)
}

func TestGetByProviderID_WithCollection(t *testing.T) {
	detail := tmdbDetailResponse{
		ID:    1,
		Title: "Movie",
		BelongsToCollection: &tmdbCollection{
			ID: 10, Name: "The Saga",
			PosterPath: "/col_poster.jpg", BackdropPath: "/col_bg.jpg",
		},
	}
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := p.GetByProviderID(context.Background(), "1", "movie")
	require.NoError(t, err)
	require.NotNil(t, result.Collection)
	assert.Equal(t, 10, result.Collection.TmdbID)
	assert.Equal(t, "The Saga", result.Collection.Name)
}

func TestGetByProviderID_CacheHit(t *testing.T) {
	calls := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbDetailResponse{ID: 1, Title: "Cached"})
	})
	defer srv.Close()

	_, err := p.GetByProviderID(context.Background(), "1", "movie")
	require.NoError(t, err)
	_, err = p.GetByProviderID(context.Background(), "1", "movie")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestSearch_CacheHit(t *testing.T) {
	calls := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmdbSearchResponse{Page: 1, TotalPages: 1,
			Results: []tmdbSearchItem{{ID: 1, Title: "Cached"}},
		})
	})
	defer srv.Close()

	_, _ = p.Search(context.Background(), "test", nil, "movie")
	_, _ = p.Search(context.Background(), "test", nil, "movie")
	assert.Equal(t, 1, calls)
}

func TestNewTMDBProvider_Defaults(t *testing.T) {
	p := NewTMDBProvider("key", "", nil)
	assert.Equal(t, "en", p.language())
	assert.NotNil(t, p.httpClient)
}

func TestAppendImages_Limit(t *testing.T) {
	imgs := make([]tmdbImage, 10)
	for i := range imgs {
		imgs[i] = tmdbImage{FilePath: fmt.Sprintf("/img%d.jpg", i), Width: 100, Height: 100}
	}
	result := appendImages(nil, imgs, "poster", 3)
	assert.Len(t, result, 3)
}

func TestAppendImages_SkipsEmptyFilePath(t *testing.T) {
	imgs := []tmdbImage{
		{FilePath: "", Width: 500, Height: 750},            // skipped: no path
		{FilePath: "/poster.jpg", Width: 500, Height: 750}, // kept
	}
	result := appendImages(nil, imgs, "poster", 5)
	assert.Len(t, result, 1, "an image with no file path must be skipped, not turned into a base-only URL")
	assert.Equal(t, imageBaseURL+"/poster.jpg", result[0].URL)
}

func TestGenreMapForType(t *testing.T) {
	assert.Equal(t, tvGenres, genreMapForType("tv"))
	assert.Equal(t, movieGenres, genreMapForType("movie"))
	assert.Equal(t, movieGenres, genreMapForType("other"))
}

func TestDoRequestWithRetry_RetryAfterHeader(t *testing.T) {
	attempts := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"page":1,"total_pages":1,"results":[]}`))
	})
	defer srv.Close()

	_, err := p.Search(context.Background(), "test", nil, "movie")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestMapDetailResult_NoVideos(t *testing.T) {
	d := tmdbDetailResponse{
		ID:    1,
		Title: "Movie",
	}
	result := mapDetailResult(d, "movie", "en")
	assert.Empty(t, result.Trailers)
	assert.Nil(t, result.Collection)
}

// The admin "change IMDb id" flow resolves the TMDb record through /find so
// the item's TMDb id follows the new IMDb id: a movie hit wins over a TV hit,
// no hit is nil (not an error), and only IMDb ids are wired.
func TestLookupByExternalID(t *testing.T) {
	var body string
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/find/tt0000001", r.URL.Path)
		assert.Equal(t, "imdb_id", r.URL.Query().Get("external_source"))
		assert.Equal(t, "en", r.URL.Query().Get("language"))
		_, _ = w.Write([]byte(body))
	})
	defer srv.Close()

	none, err := p.LookupByExternalID(context.Background(), "tvdb", "12345")
	require.NoError(t, err)
	assert.Nil(t, none, "only IMDb ids are wired")
	none, err = p.LookupByExternalID(context.Background(), "imdb", "")
	require.NoError(t, err)
	assert.Nil(t, none)

	body = `{"movie_results":[{"id":603}],"tv_results":[{"id":1396}]}`
	got, err := p.LookupByExternalID(context.Background(), "imdb", "tt0000001")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"tmdb": "603", "imdb": "tt0000001"}, got.ProviderIDs)

	body = `{"movie_results":[],"tv_results":[{"id":1396}]}`
	got, err = p.LookupByExternalID(context.Background(), "imdb", "tt0000001")
	require.NoError(t, err)
	assert.Equal(t, "1396", got.ProviderIDs["tmdb"])

	body = `{"movie_results":[],"tv_results":[]}`
	got, err = p.LookupByExternalID(context.Background(), "imdb", "tt0000001")
	require.NoError(t, err)
	assert.Nil(t, got, "no mapping is not an error")
}

// Alternative titles feed the ambiguous-match scorer: movies list them under
// "titles", series under "results"; blanks and duplicates are dropped, and a
// second call is served from the cache (the path is rate-limited upstream).
func TestAlternativeTitles(t *testing.T) {
	calls := 0
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/movie/603/alternative_titles":
			_, _ = w.Write([]byte(`{"id":603,"titles":[{"iso_3166_1":"FR","title":"Titre FR"},{"iso_3166_1":"DE","title":""},{"iso_3166_1":"BE","title":"Titre FR"}]}`))
		case "/tv/1396/alternative_titles":
			_, _ = w.Write([]byte(`{"id":1396,"results":[{"iso_3166_1":"ES","title":"Titulo ES"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	movie, err := p.AlternativeTitles(context.Background(), "603", "movie")
	require.NoError(t, err)
	assert.Equal(t, []string{"Titre FR"}, movie)
	tv, err := p.AlternativeTitles(context.Background(), "1396", "tv")
	require.NoError(t, err)
	assert.Equal(t, []string{"Titulo ES"}, tv)

	again, err := p.AlternativeTitles(context.Background(), "603", "movie")
	require.NoError(t, err)
	assert.Equal(t, movie, again)
	assert.Equal(t, 2, calls, "the repeat is served from the cache")

	_, err = p.AlternativeTitles(context.Background(), "1", "music")
	require.Error(t, err)
	assert.Equal(t, 2, calls, "an unsupported type never hits the network")
}

// The parental gate reads OfficialRating; TMDb spreads certifications per
// country, per release type for movies and per country for series. The map
// keeps every country, the first non-empty entry per country wins, the US
// entry is the backcompat default and an already-set rating is not overridden.
func TestMapDetailCertifications(t *testing.T) {
	movie := tmdbDetailResponse{ReleaseDates: &tmdbReleaseDates{Results: []tmdbCountryReleaseDates{
		{Country: "FR", ReleaseDates: []tmdbReleaseDateEntry{{Certification: ""}, {Certification: "12"}, {Certification: "16"}}},
		{Country: "US", ReleaseDates: []tmdbReleaseDateEntry{{Certification: "PG-13"}}},
		{Country: "XX", ReleaseDates: []tmdbReleaseDateEntry{{Certification: ""}}},
	}}}
	var r domain.MetadataResult
	mapDetailCertifications(&r, movie, "movie")
	assert.Equal(t, map[string]string{"FR": "12", "US": "PG-13"}, r.CertificationsByCountry)
	assert.Equal(t, "PG-13", r.OfficialRating)

	tv := tmdbDetailResponse{ContentRatings: &tmdbContentRatings{Results: []tmdbCountryContentRating{
		{Country: "DE", Rating: "16"}, {Country: "US", Rating: "TV-MA"}, {Country: "ES", Rating: ""},
	}}}
	preset := domain.MetadataResult{OfficialRating: "TV-14"}
	mapDetailCertifications(&preset, tv, "tv")
	assert.Equal(t, map[string]string{"DE": "16", "US": "TV-MA"}, preset.CertificationsByCountry)
	assert.Equal(t, "TV-14", preset.OfficialRating, "a rating already set is kept")

	// A series payload read as a movie (or the reverse) contributes nothing, and
	// no certifications at all leaves the result untouched.
	var cross domain.MetadataResult
	mapDetailCertifications(&cross, tv, "movie")
	assert.Nil(t, cross.CertificationsByCountry)
	mapDetailCertifications(&cross, tmdbDetailResponse{}, "tv")
	assert.Nil(t, cross.CertificationsByCountry)
	assert.Empty(t, cross.OfficialRating)
}
