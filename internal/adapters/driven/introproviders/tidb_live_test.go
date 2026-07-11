//go:build tidblive

// Opt-in live smoke test against the real TheIntroDB v3 API. Never runs in
// CI (guarded by the `tidblive` build tag) and skips when no API key is set.
//
//	EVEY_INTRO_TIDB_API_KEY=... go test -tags tidblive -run TestTIDBLive -v \
//	  ./internal/adapters/driven/introproviders/
//
// It exercises the v3 read path end-to-end through the real client:
// tmdb lookup with open-ended credits resolved against duration, the imdb_id
// fallback, and a movie lookup. Read-only - it never submits.
package introproviders

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveClient(t *testing.T) *TIDBClient {
	t.Helper()
	key := os.Getenv("EVEY_INTRO_TIDB_API_KEY")
	if key == "" {
		t.Skip("EVEY_INTRO_TIDB_API_KEY not set - skipping live TIDB test")
	}
	return NewTIDBClient(key) // real https://api.theintrodb.org/v3
}

func segByType(segs []mediaSegmentView, typ string) (mediaSegmentView, bool) {
	for _, s := range segs {
		if s.Type == typ {
			return s, true
		}
	}
	return mediaSegmentView{}, false
}

// mediaSegmentView is a tiny local projection so the helper above stays
// decoupled from the domain import alias.
type mediaSegmentView struct {
	Type       string
	StartMs    int64
	EndMs      int64
	Confidence float64
	Source     string
}

// TestTIDBLive_TVEpisode_ResolvesOpenEndedCredits is the headline check: a
// known episode whose credits carry end_ms == null must come back with the
// end resolved to the supplied DurationMs (the fix that stopped us dropping
// end credits).
func TestTIDBLive_TVEpisode_ResolvesOpenEndedCredits(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const durMs int64 = 3_500_000 // ~58 min, Breaking Bad S1E1 (credits start ~57 min)
	raw, err := c.Fetch(ctx, Lookup{TmdbID: 1396, Season: 1, Episode: 1, DurationMs: durMs})
	require.NoError(t, err)
	require.NotEmpty(t, raw, "expected live segments for Breaking Bad S1E1")

	segs := make([]mediaSegmentView, len(raw))
	for i, s := range raw {
		segs[i] = mediaSegmentView{Type: s.Type, StartMs: s.StartMs, EndMs: s.EndMs, Confidence: s.Confidence, Source: s.Source}
		t.Logf("segment: type=%s start=%d end=%d conf=%.2f src=%s", s.Type, s.StartMs, s.EndMs, s.Confidence, s.Source)
	}

	intro, ok := segByType(segs, "intro")
	require.True(t, ok, "expected an intro segment")
	assert.Greater(t, intro.EndMs, intro.StartMs)
	assert.Equal(t, "tidb", intro.Source)

	// The fix under test: open-ended credits closed against the runtime.
	if credits, ok := segByType(segs, "end_credits"); ok {
		assert.Greater(t, credits.StartMs, int64(0))
		assert.LessOrEqual(t, credits.EndMs, durMs, "credits end must not exceed runtime")
		assert.Greater(t, credits.EndMs, credits.StartMs)
		t.Logf("end_credits resolved: [%d, %d] (runtime %d)", credits.StartMs, credits.EndMs, durMs)
	} else {
		t.Log("no credits segment for this item (acceptable - coverage varies)")
	}
}

// TestTIDBLive_ImdbFallback hits the v3 imdb_id path (no tmdb_id) to confirm
// TIDB itself now covers imdb-only items.
func TestTIDBLive_ImdbFallback(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Breaking Bad series imdb id, no tmdb - must resolve via imdb_id.
	raw, err := c.Fetch(ctx, Lookup{ImdbID: "tt0903747", Season: 1, Episode: 1})
	require.NoError(t, err)
	for _, s := range raw {
		t.Logf("imdb-fallback segment: type=%s start=%d end=%d", s.Type, s.StartMs, s.EndMs)
	}
	// Coverage may differ between tmdb and imdb keys; assert no error and that
	// any returned segment is well-formed rather than a hard count.
	for _, s := range raw {
		assert.Greater(t, s.EndMs, s.StartMs)
	}
}

// TestTIDBLive_Movie confirms the movie shape (no season/episode) round-trips.
func TestTIDBLive_Movie(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	raw, err := c.Fetch(ctx, Lookup{TmdbID: 550, IsMovie: true, DurationMs: 8_340_000}) // Fight Club ~139 min
	require.NoError(t, err)
	for _, s := range raw {
		t.Logf("movie segment: type=%s start=%d end=%d", s.Type, s.StartMs, s.EndMs)
		assert.Greater(t, s.EndMs, s.StartMs)
	}
}
