package introproviders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pin: outro is mapped to "end_credits" so it slots into the same domain
// type as MKV chapter pass + the in-house heuristic.
func TestIntroDBClient_OK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/segments", r.URL.Path)
		assert.Equal(t, "tt0944947", r.URL.Query().Get("imdb_id"))
		assert.Equal(t, "1", r.URL.Query().Get("season"))
		assert.Equal(t, "1", r.URL.Query().Get("episode"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"imdb_id": "tt0944947",
			"season": 1,
			"episode": 1,
			"intro": {"start_ms": 437000, "end_ms": 531000, "confidence": 0.92, "submission_count": 5},
			"recap": null,
			"outro": {"start_ms": 3300000, "end_ms": 3420000, "confidence": 0.88, "submission_count": 3}
		}`))
	}))
	defer srv.Close()

	c := NewIntroDBClient(WithIntroDBBaseURL(srv.URL))
	got, err := c.Fetch(context.Background(), Lookup{
		ImdbID: "tt0944947", Season: 1, Episode: 1,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "intro", got[0].Type)
	assert.InDelta(t, 0.92, got[0].Confidence, 0.001)
	assert.Equal(t, "introdb", got[0].Source)
	assert.Equal(t, "end_credits", got[1].Type)
}

// Movies are unsupported by IntroDB — Fetch must skip without HTTP. Pin
// to avoid wasted requests in the chain.
func TestIntroDBClient_MovieSkipped(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must NOT be hit for movies")
	}))
	defer srv.Close()

	c := NewIntroDBClient(WithIntroDBBaseURL(srv.URL))
	got, err := c.Fetch(context.Background(), Lookup{
		TmdbID: 12345, IsMovie: true,
	})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// IMDb ID required — without it, no upstream call.
func TestIntroDBClient_NoImdbID_Skips(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must NOT be hit without imdb_id")
	}))
	defer srv.Close()

	c := NewIntroDBClient(WithIntroDBBaseURL(srv.URL))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// Confidence outside [0,1] must be clamped — the upstream is community
// and could regress.
func TestIntroDBClient_ConfidenceClamped(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"imdb_id":"tt0944947","season":1,"episode":1,
			"intro":{"start_ms":0,"end_ms":1000,"confidence":1.42,"submission_count":1},
			"recap":null,"outro":null}`))
	}))
	defer srv.Close()

	c := NewIntroDBClient(WithIntroDBBaseURL(srv.URL))
	got, err := c.Fetch(context.Background(), Lookup{ImdbID: "tt0944947", Season: 1, Episode: 1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1.0, got[0].Confidence)
}
