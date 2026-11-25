package metadata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 23 seasons (plus specials) take three calls: the season list, then two
// batches of at most 20 appended seasons.
func TestSeriesEpisodes_BatchesSeasons(t *testing.T) {
	var calls []string
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query().Get("append_to_response"))
		app := r.URL.Query().Get("append_to_response")
		if app == "" {
			var seasons []string
			for n := 0; n <= 23; n++ {
				seasons = append(seasons, fmt.Sprintf(`{"season_number":%d}`, n))
			}
			fmt.Fprintf(w, `{"id":1399,"seasons":[%s]}`, strings.Join(seasons, ","))
			return
		}
		var parts []string
		for _, s := range strings.Split(app, ",") {
			var n int
			_, _ = fmt.Sscanf(s, "season/%d", &n)
			parts = append(parts, fmt.Sprintf(`"%s":{"episodes":[{"episode_number":1,"name":"S%dE1","air_date":"2011-04-17"},{"episode_number":2,"name":"S%dE2","air_date":""}]}`, s, n, n))
		}
		fmt.Fprintf(w, `{"id":1399,%s}`, strings.Join(parts, ","))
	})
	defer srv.Close()

	eps, err := p.SeriesEpisodes(context.Background(), "1399")
	require.NoError(t, err)
	assert.Len(t, calls, 3)
	assert.Equal(t, 20, strings.Count(calls[1], "season/"))
	assert.Equal(t, 4, strings.Count(calls[2], "season/"))
	assert.Len(t, eps, 24*2)
	assert.Equal(t, 0, eps[0].Season)
	require.NotNil(t, eps[0].AirDate)
	assert.Nil(t, eps[1].AirDate, "an empty air date is unknown, not year 1")
}

func TestSeriesEpisodes_RefusesANonNumericID(t *testing.T) {
	p := NewTMDBProvider("key", "en", nil)
	_, err := p.SeriesEpisodes(context.Background(), "1399/../../account")
	require.Error(t, err)
}

func TestCollectionParts_ReadsTheSaga(t *testing.T) {
	srv, p := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/collection/1575", r.URL.Path)
		fmt.Fprint(w, `{"id":1575,"parts":[{"id":1366,"title":"Rocky","release_date":"1976-11-21","poster_path":"/r.jpg"},{"id":1367,"title":"Rocky II","release_date":"","poster_path":""}]}`)
	})
	defer srv.Close()

	parts, err := p.CollectionParts(context.Background(), 1575)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, 1366, parts[0].TmdbID)
	assert.Equal(t, "https://image.tmdb.org/t/p/w500/r.jpg", parts[0].PosterURL)
	assert.Nil(t, parts[1].ReleaseDate)
}
