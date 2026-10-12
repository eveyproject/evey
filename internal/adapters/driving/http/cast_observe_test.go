package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

type recordingObserver struct {
	calls []struct {
		profileID, mediaItemID string
		endMs                  int64
	}
}

func (o *recordingObserver) ObserveFetch(profileID, mediaItemID string, endMs int64) {
	o.calls = append(o.calls, struct {
		profileID, mediaItemID string
		endMs                  int64
	}{profileID, mediaItemID, endMs})
}

func reqWithToken(st *shared.StreamToken) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_00010.m4s", nil)
	if st == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, st))
}

// Only CAST-marked tokens feed the tracker: a local player's fetches would
// fight its own real progress reports (it buffers ahead too).
func TestObserveCastFetch_OnlyCastTokens(t *testing.T) {
	obs := &recordingObserver{}

	observeCastFetch(reqWithToken(&shared.StreamToken{Cast: true, ProfileID: "p1", MediaItemID: "m1"}), obs, 30_000)
	observeCastFetch(reqWithToken(&shared.StreamToken{Cast: false, ProfileID: "p1", MediaItemID: "m1"}), obs, 60_000)
	observeCastFetch(reqWithToken(nil), obs, 90_000)

	if assert.Len(t, obs.calls, 1, "only the cast token must observe") {
		assert.Equal(t, "p1", obs.calls[0].profileID)
		assert.Equal(t, "m1", obs.calls[0].mediaItemID)
		assert.Equal(t, int64(30_000), obs.calls[0].endMs)
	}
}

func TestObserveCastFetch_NilObserverIsSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		observeCastFetch(reqWithToken(&shared.StreamToken{Cast: true, ProfileID: "p1", MediaItemID: "m1"}), nil, 30_000)
	})
}

// od_ plan segments are keyframe-cut around a 6s target, so their exact times
// are not grid-derivable; the estimate is PROPORTIONAL (idx/total x duration),
// clamped to the duration. seg00001 is the first segment.
func TestOnDemandSegEndMs(t *testing.T) {
	durMs := int64(600_000) // 10 min
	assert.Equal(t, int64(6_000), onDemandSegEndMs("seg00001.m4s", 100, durMs))
	assert.Equal(t, int64(600_000), onDemandSegEndMs("seg00100.m4s", 100, durMs))
	assert.Equal(t, int64(0), onDemandSegEndMs("init.mp4", 100, durMs), "non-segment resources carry no position")
	assert.Equal(t, int64(0), onDemandSegEndMs("seg00010.m4s", 0, durMs), "no segment count, no estimate")
	assert.Equal(t, int64(0), onDemandSegEndMs("seg99999.m4s", 100, 0), "no duration, no estimate")
	assert.Equal(t, durMs, onDemandSegEndMs("seg00150.m4s", 100, durMs), "forged index clamps to the duration")
}

func TestParseRangeStart(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"bytes=0-", 0, true},
		{"bytes=1048576-2097151", 1048576, true},
		{"bytes=-500", 0, false}, // suffix range: no absolute position
		{"", 0, false},
		{"items=3-4", 0, false},
		{"bytes=abc-", 0, false},
	}
	for _, c := range cases {
		got, ok := parseRangeStart(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseRangeStart(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The transcode ladder is a fixed grid, so segment K (0-based) ends at
// (K+1) x grid, capped at the media duration so a boundary fetch never
// reports past the end (which would wrongly auto-complete the item at 90%).
func TestTranscodeSegEndMs(t *testing.T) {
	const grid = 3.0 // ManifestSegmentSeconds
	assert.Equal(t, int64(3_000), transcodeSegEndMs(0, grid, 600_000), "first segment ends at one grid")
	assert.Equal(t, int64(30_000), transcodeSegEndMs(9, grid, 600_000), "segment 9 ends at 30s")
	assert.Equal(t, int64(600_000), transcodeSegEndMs(9999, grid, 600_000), "a boundary index caps at the duration")
	assert.Equal(t, int64(3_000), transcodeSegEndMs(0, grid, 0), "unknown duration returns the uncapped estimate")
}

// The packager path logged nothing, so a failed cast left no server-side trace
// (2026-08-08). The fix must not swing the other way: a line per segment is
// ~500 per playback, which is how a log stops being read. Head segments and
// failures only.
func TestShouldLogPlanResource_TracesTheStartWithoutFloodingTheLog(t *testing.T) {
	// The non-segment resources are a handful per playback: always worth a line.
	for _, name := range []string{"playlist.m3u8", "init.mp4", "manifest.mpd", "audio1.m3u8", "sub1.m3u8"} {
		if !shouldLogPlanResource(name, false) {
			t.Errorf("shouldLogPlanResource(%q) = false, want true: without it we cannot tell "+
				"whether the receiver even asked for the video playlist", name)
		}
	}

	// The head of the plan: where a load that dies, dies.
	for _, name := range []string{"seg00001.m4s", "seg00003.m4s"} {
		if !shouldLogPlanResource(name, false) {
			t.Errorf("shouldLogPlanResource(%q) = false, want true (start of playback)", name)
		}
	}

	// Steady-state playback must stay SILENT: this is the whole anti-flood point.
	for _, name := range []string{"seg00004.m4s", "seg00120.m4s", "seg00500.m4s"} {
		if shouldLogPlanResource(name, false) {
			t.Errorf("shouldLogPlanResource(%q) = true: one line per segment is ~500 per "+
				"playback and drowns the very incident this was added for", name)
		}
	}

	// A failure is the interesting line, at ANY index.
	if !shouldLogPlanResource("seg00423.m4s", true) {
		t.Error("a refused segment must be logged whatever its index: a silent 404 mid-film " +
			"is exactly the failure this traces")
	}
}
