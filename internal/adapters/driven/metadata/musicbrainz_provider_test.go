package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	domain "github.com/evey-project/evey/internal/domain/metadata"
)

func newMusicBrainzTestServer(handler http.HandlerFunc) (*httptest.Server, *MusicBrainzProvider) {
	srv := httptest.NewServer(handler)
	p := NewMusicBrainzProvider()
	p.baseURL = srv.URL
	p.httpClient = srv.Client()
	p.limiter = rate.NewLimiter(rate.Inf, 1)
	return srv, p
}

// MusicBrainz refuses anonymous clients, so the User-Agent is part of the
// contract; the search is JSON, capped, and a release group maps to
// "Artist - Title" with the year and an unverified cover-art hint.
func TestMusicBrainz_SearchMapsReleaseGroups(t *testing.T) {
	srv, p := newMusicBrainzTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/release-group", r.URL.Path)
		assert.Equal(t, "Some Album", r.URL.Query().Get("query"))
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, "8", r.URL.Query().Get("limit"))
		assert.Equal(t, musicbrainzUserAgent, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(`{"release-groups":[
			{"id":"rg-1","title":"Some Album","first-release-date":"1997-06-20","artist-credit":[{"name":"Some Band"}]},
			{"id":"rg-2","title":"Untitled","first-release-date":"19"},
			{"id":"","title":"No Id"}
		]}`))
	})
	defer srv.Close()

	assert.Equal(t, "musicbrainz", p.Name())
	skipped, err := p.Search(context.Background(), "Some Album", nil, "movie")
	require.NoError(t, err)
	assert.Nil(t, skipped, "only music goes to MusicBrainz")

	results, err := p.Search(context.Background(), "Some Album", nil, "music")
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "Some Band - Some Album", results[0].Title)
	require.NotNil(t, results[0].Year)
	assert.Equal(t, 1997, *results[0].Year)
	assert.Equal(t, "rg-1", results[0].ProviderIDs["musicbrainz"])
	require.Len(t, results[0].Artwork, 1)
	assert.Equal(t, domain.ArtworkTypePoster, results[0].Artwork[0].Type)
	assert.Contains(t, results[0].Artwork[0].URL, "/release-group/rg-1/front-250")

	assert.Equal(t, "Untitled", results[1].Title, "no artist credit: bare title")
	assert.Nil(t, results[1].Year, "a truncated date is no year")
	assert.Empty(t, results[2].Artwork, "no id: no cover-art URL can be built")
}

func TestMusicBrainz_GetByProviderID_AndErrors(t *testing.T) {
	var status int
	srv, p := newMusicBrainzTestServer(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		assert.Equal(t, "/release-group/rg%201", r.URL.EscapedPath())
		assert.Equal(t, "artist-credits", r.URL.Query().Get("inc"))
		_, _ = w.Write([]byte(`{"id":"rg 1","title":"Album","first-release-date":"2003","artist-credit":[{"name":"Band"}]}`))
	})
	defer srv.Close()

	none, err := p.GetByProviderID(context.Background(), "rg 1", "series")
	require.NoError(t, err)
	assert.Nil(t, none)

	got, err := p.GetByProviderID(context.Background(), "rg 1", "music")
	require.NoError(t, err)
	assert.Equal(t, "Band - Album", got.Title)
	assert.Equal(t, 2003, *got.Year)

	status = http.StatusServiceUnavailable
	_, err = p.GetByProviderID(context.Background(), "rg 1", "music")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
	_, err = p.Search(context.Background(), "x", nil, "music")
	require.Error(t, err)

	// The limiter is honoured: a cancelled context never reaches the network.
	p.limiter = rate.NewLimiter(rate.Every(time.Hour), 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.Search(ctx, "x", nil, "music")
	require.Error(t, err)
}
