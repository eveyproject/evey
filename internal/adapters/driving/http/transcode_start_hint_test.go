package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStartSegmentHint_ParsesTheMasterQuery(t *testing.T) {
	cases := map[string]struct {
		query string
		seg   int
		ok    bool
	}{
		"absent":            {"", 0, false},
		"zero":              {"startSec=0", 0, false},
		"negative":          {"startSec=-5", 0, false},
		"garbage":           {"startSec=abc", 0, false},
		"nan":               {"startSec=NaN", 0, false},
		"inf":               {"startSec=Inf", 0, false},
		"inside a segment":  {"startSec=2.9", 0, false},
		"the prod switch":   {"startSec=69.4", 23, true}, // 3 s segments: 69 s is segment 23
		"deep resume":       {"startSec=3600", 1200, true},
		"fractional deep":   {"startSec=100.5", 33, true},
		"with other params": {"audioTrack=2&startSec=69.4&wasmAudio=1", 23, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/media/m1/hls/master.m3u8?"+c.query, nil)
			seg, ok := startSegmentHint(r)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.seg, seg)
		})
	}
}

func TestStartPositionHints_RememberedPerMediaAndExpire(t *testing.T) {
	var hints startPositionHints
	now := time.Now()

	_, ok := hints.peek("m1", now)
	assert.False(t, ok, "nothing remembered yet")

	hints.remember("m1", 23, now)
	hints.remember("m2", 500, now)
	hints.remember("", 7, now) // no media, no hint
	hints.remember("m3", 0, now)
	hints.remember("m4", -1, now)

	seg, ok := hints.peek("m1", now.Add(time.Second))
	assert.True(t, ok)
	assert.Equal(t, 23, seg)
	seg, ok = hints.peek("m2", now.Add(time.Second))
	assert.True(t, ok)
	assert.Equal(t, 500, seg)
	seg, ok = hints.peek("m3", now)
	assert.True(t, ok, "a master starting at 0 is remembered: its variant fetch is a cold start")
	assert.Equal(t, 0, seg)
	_, ok = hints.peek("m4", now)
	assert.False(t, ok, "a negative start is no hint")

	// A peek does not consume: an ABR probe fetches two variant playlists.
	seg, ok = hints.peek("m1", now.Add(2*time.Second))
	assert.True(t, ok)
	assert.Equal(t, 23, seg)

	_, ok = hints.peek("m1", now.Add(startHintTTL+time.Second))
	assert.False(t, ok, "a hint from an earlier viewing must not leak into the next")
}

func TestPrewarmSegment_UsesTheHintOnlyInsideThePresentation(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()

	// Prod 2026-10-12: a variant playlist refetched after a token refresh, long
	// after its master, warmed segment 0 on a rung nobody read.
	seg, ok := h.prewarmSegment("m1", 7980, now)
	assert.False(t, ok, "no fresh master: a mid-viewing refetch, no warm-up")
	assert.Equal(t, 0, seg)

	h.startHints.remember("m1", 0, now)
	seg, ok = h.prewarmSegment("m1", 7980, now)
	assert.True(t, ok, "a master starting at 0: warm at 0")
	assert.Equal(t, 0, seg)

	h.startHints.remember("m1", 23, now)
	seg, ok = h.prewarmSegment("m1", 7980, now)
	assert.True(t, ok)
	assert.Equal(t, 23, seg, "the prod switch: warm at 23, not 0")
	seg, _ = h.prewarmSegment("m1", 0, now)
	assert.Equal(t, 23, seg, "unknown duration: trust the hint")

	_, ok = h.prewarmSegment("m1", 7980, now.Add(startHintTTL+time.Second))
	assert.False(t, ok, "the master is too old: no warm-up")

	h.startHints.remember("m1", 4000, now)
	seg, ok = h.prewarmSegment("m1", 7980, now)
	assert.True(t, ok)
	assert.Equal(t, 0, seg, "a hint past the end is bogus: back to 0")
}

// A rung's init first asked after a long pause (no fresh master) is ensured
// where playback is, not at 0.
func TestInitSegment_FallsBackToTheLivePositionThenZero(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()

	assert.Equal(t, 0, h.initSegment("m1", "fp1", 7980, now), "nothing known: 0")

	h.livePositions.rememberWithin("fp1", 1333, now, livePositionTTL)
	later := now.Add(7 * time.Minute)
	assert.Equal(t, 1333, h.initSegment("m1", "fp1", 7980, later), "after a long pause: the last segment served")
	assert.Equal(t, 0, h.initSegment("m1", "fp2", 7980, later), "another decision does not borrow it")

	h.startHints.remember("m1", 23, later)
	assert.Equal(t, 23, h.initSegment("m1", "fp1", 7980, later), "a fresh master wins")

	assert.Equal(t, 0, h.initSegment("m1", "fp1", 7980, now.Add(livePositionTTL+time.Minute)), "too old: 0")
}
