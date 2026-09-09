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

	seg, ok := hints.peek("m1", now.Add(time.Second))
	assert.True(t, ok)
	assert.Equal(t, 23, seg)
	seg, ok = hints.peek("m2", now.Add(time.Second))
	assert.True(t, ok)
	assert.Equal(t, 500, seg)
	_, ok = hints.peek("m3", now)
	assert.False(t, ok, "a start at 0 is the default, not a hint")

	// Not consumed by a peek: the second variant playlist of an ABR probe
	// warms at the same place.
	seg, ok = hints.peek("m1", now.Add(2*time.Second))
	assert.True(t, ok)
	assert.Equal(t, 23, seg)

	_, ok = hints.peek("m1", now.Add(startHintTTL+time.Second))
	assert.False(t, ok, "a hint from an earlier viewing must not leak into the next")
}

func TestPrewarmSegment_UsesTheHintOnlyInsideThePresentation(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()

	assert.Equal(t, 0, h.prewarmSegment("m1", 7980, now), "no hint: segment 0, as before")

	h.startHints.remember("m1", 23, now)
	assert.Equal(t, 23, h.prewarmSegment("m1", 7980, now), "the prod switch: warm at 23, not 0")
	assert.Equal(t, 23, h.prewarmSegment("m1", 0, now), "unknown duration: trust the hint")

	h.startHints.remember("m1", 4000, now)
	assert.Equal(t, 0, h.prewarmSegment("m1", 7980, now), "a hint past the end is bogus: back to 0")
}
