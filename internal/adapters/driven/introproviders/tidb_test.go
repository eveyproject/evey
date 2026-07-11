package introproviders

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pin the contract that TIDB returns segment data when the upstream
// answers 200 with non-empty arrays. Confidence must be 0.97 — we want
// upstream to be treated as competitive with our pHash+Chromaprint
// primary when it has data.
func TestTIDBClient_OK_TVEpisode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v3/media", r.URL.Path)
		assert.Equal(t, "1396", r.URL.Query().Get("tmdb_id"))
		assert.Equal(t, "1", r.URL.Query().Get("season"))
		assert.Equal(t, "5", r.URL.Query().Get("episode"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, tidbUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tmdb_id": 1396,
			"type": "tv",
			"season": 1,
			"episode": 5,
			"intro": [{"start_ms": 0, "end_ms": 60000}],
			"recap": [],
			"credits": [{"start_ms": 2400000, "end_ms": 2520000}]
		}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("test-key", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1396, Season: 1, Episode: 5})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "intro", got[0].Type)
	assert.Equal(t, int64(0), got[0].StartMs)
	assert.Equal(t, int64(60000), got[0].EndMs)
	assert.InDelta(t, 0.97, got[0].Confidence, 0.001)
	assert.Equal(t, "tidb", got[0].Source)
	assert.Equal(t, "end_credits", got[1].Type)
}

// Movie shape: no season / episode in the URL, single intro entry.
func TestTIDBClient_OK_Movie(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Movies must NOT carry season/episode params.
		assert.Equal(t, "", r.URL.Query().Get("season"))
		assert.Equal(t, "", r.URL.Query().Get("episode"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":12345,"type":"movie","intro":[{"start_ms":null,"end_ms":90000}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 12345, IsMovie: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	// null start_ms must be coerced to 0 (per OpenAPI semantics).
	assert.Equal(t, int64(0), got[0].StartMs)
	assert.Equal(t, int64(90000), got[0].EndMs)
}

// Open-ended credits (end_ms == null) are dropped when the lookup carries
// no duration to close them — a half-open range can't be rendered as a skip
// CTA. Pin the fallback so the cache never stores a no-op segment.
func TestTIDBClient_DropsOpenEndedCredits_WhenDurationUnknown(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":1,"type":"tv","credits":[{"start_ms":1800000,"end_ms":null}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Open-ended credits ARE kept when the lookup carries a duration: end_ms ==
// null resolves to the item runtime, and duration_ms is forwarded upstream.
func TestTIDBClient_OpenEndedCredits_ResolvedToDuration(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "2700000", r.URL.Query().Get("duration_ms"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":1,"type":"tv","credits":[{"start_ms":1800000,"end_ms":null}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1, DurationMs: 2_700_000})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "end_credits", got[0].Type)
	assert.Equal(t, int64(1_800_000), got[0].StartMs)
	assert.Equal(t, int64(2_700_000), got[0].EndMs) // resolved to runtime
}

// intro/recap require an explicit end: a null end is dropped even when a
// duration is available (an open-ended intro is meaningless).
func TestTIDBClient_DropsIntroWithoutEnd_EvenWithDuration(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":1,"type":"tv","intro":[{"start_ms":0,"end_ms":null}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1, DurationMs: 2_700_000})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// v3 surfaces an optional per-segment confidence; when present we use it
// (clamped 0..1) instead of the 0.97 floor.
func TestTIDBClient_UsesUpstreamConfidence(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":1,"type":"tv","intro":[{"start_ms":0,"end_ms":60000,"confidence":0.82,"submission_count":7}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.InDelta(t, 0.82, got[0].Confidence, 0.001)
}

// 404 from the upstream is the normal "we don't know this episode" case
// — must surface as (nil, nil) so the chain falls through to the next
// provider, NOT as an error.
func TestTIDBClient_NotFound_FallsThrough(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"media not found"}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// 429 with Retry-After must surface as ErrRateLimited carrying the
// header value — the chain uses it to set the cooldown.
func TestTIDBClient_RateLimited(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	_, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.Error(t, err)
	assert.True(t, IsRateLimited(err))
	var rl ErrRateLimited
	require.True(t, errors.As(err, &rl))
	assert.Equal(t, "tidb", rl.Source)
	assert.Equal(t, 12, int(rl.RetryAfter.Seconds()))
}

// v3 fallback: with no TmdbID but an ImdbID, TIDB now queries by imdb_id
// (no tmdb_id param) instead of skipping straight to IntroDB.
func TestTIDBClient_ImdbFallback_Queries(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tt0944947", r.URL.Query().Get("imdb_id"))
		assert.Empty(t, r.URL.Query().Get("tmdb_id"))
		assert.Equal(t, "1", r.URL.Query().Get("season"))
		assert.Equal(t, "1", r.URL.Query().Get("episode"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tmdb_id":0,"type":"tv","intro":[{"start_ms":0,"end_ms":45000}]}`))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	got, err := c.Fetch(context.Background(), Lookup{ImdbID: "tt0944947", Season: 1, Episode: 1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "intro", got[0].Type)
}

// With neither TmdbID nor ImdbID there is nothing to query — return
// (nil, nil) without making a request (base points at an unreachable host).
func TestTIDBClient_NoIdentifier_Skips(t *testing.T) {
	t.Parallel()
	c := NewTIDBClient("", WithTIDBBaseURL("http://invalid.invalid"))
	got, err := c.Fetch(context.Background(), Lookup{Season: 1, Episode: 1})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// A daily-cap 429 advertises a long X-UsageLimit-Reset alongside a short
// burst X-RateLimit-Reset. parseRetryAfter must surface the longest window
// so the caller doesn't retry while the daily cap is still closed.
func TestTIDBClient_RateLimited_PrefersLongestWindow(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Reset", "10")
		w.Header().Set("X-UsageLimit-Reset", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	_, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.Error(t, err)
	var rl ErrRateLimited
	require.ErrorAs(t, err, &rl)
	assert.Equal(t, 3600, int(rl.RetryAfter.Seconds()))
}

// 500 propagates as an error — the chain logs + falls through but the
// error must carry the status code so the operator can debug.
func TestTIDBClient_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream meltdown"))
	}))
	defer srv.Close()

	c := NewTIDBClient("", WithTIDBBaseURL(srv.URL+"/v3"))
	_, err := c.Fetch(context.Background(), Lookup{TmdbID: 1, Season: 1, Episode: 1})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "500"))
}
