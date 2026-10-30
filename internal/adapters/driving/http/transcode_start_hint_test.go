package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// legacyAt is the 3 s clock, on which the hint's seconds and segments read
// as they did before the hint was kept in seconds.
var legacyAt = domaintranscoding.SegmentGrid{}.SegmentIndexAt

func TestStartSecondsHint_ParsesTheMasterQuery(t *testing.T) {
	cases := map[string]struct {
		query string
		sec   float64
		ok    bool
	}{
		"absent":            {"", 0, false},
		"zero":              {"startSec=0", 0, false},
		"negative":          {"startSec=-5", 0, false},
		"garbage":           {"startSec=abc", 0, false},
		"nan":               {"startSec=NaN", 0, false},
		"inf":               {"startSec=Inf", 0, false},
		"inside a segment":  {"startSec=2.9", 2.9, true},
		"the prod switch":   {"startSec=69.4", 69.4, true},
		"deep resume":       {"startSec=3600", 3600, true},
		"with other params": {"audioTrack=2&startSec=69.4&wasmAudio=1", 69.4, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/media/m1/hls/master.m3u8?"+c.query, nil)
			sec, ok := startSecondsHint(r)
			assert.Equal(t, c.ok, ok)
			assert.InDelta(t, c.sec, sec, 1e-9)
		})
	}
}

// The hint is a position; each rung turns it into ITS segment. Divided by a
// fixed 3 s, a resume an hour in on a 23.976 fps source warmed segment 1200
// while the player asked 1198 (3.003 s segments), and a copy rung's keyframe
// plan was addressed as if it were cut every 3 s.
func TestPrewarmSegment_ConvertsOnTheRungsOwnSegments(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()
	h.startHints.remember("m1", 3600, now)

	exact := domaintranscoding.GridForFrameRate(24000.0 / 1001)
	seg, ok := h.prewarmSegment("m1", 7980, exact.SegmentIndexAt, now)
	assert.True(t, ok)
	assert.Equal(t, 1198, seg, "3600 s on 3.003 s segments")

	seg, _ = h.prewarmSegment("m1", 7980, domaintranscoding.SegmentGrid{}.SegmentIndexAt, now)
	assert.Equal(t, 1200, seg, "the legacy 3 s clock")

	var plan []apptranscoding.CopySegment
	for ms := int64(0); ms < 7_980_000; ms += 8008 {
		plan = append(plan, apptranscoding.CopySegment{StartMs: ms, EndMs: min(ms+8008, 7_980_000)})
	}
	seg, _ = h.prewarmSegment("m1", 7980, func(sec float64) int { return copySegmentAt(plan, sec) }, now)
	assert.Equal(t, 449, seg, "a copy rung's plan: the keyframe segment holding 3600 s")
	assert.Equal(t, 0, copySegmentAt(nil, 3600), "no plan: 0")
	assert.Equal(t, len(plan)-1, copySegmentAt(plan, 1e6), "past the end: the last segment")
}

func TestAudioServe_HintForUsesTheRenditionGrid(t *testing.T) {
	grid := domaintranscoding.GridForFrameRate(24000.0 / 1001)
	if got := (&audioServe{}).hintFor(nil, "m1", grid); got != 0 {
		t.Errorf("no hint source: got %d, want 0", got)
	}
	h := &audioServe{startHint: func(key string) (float64, bool) { return 3600, key == "m1" }}
	assert.Equal(t, 1198, h.hintFor(nil, "m1", grid), "3600 s on the rendition's 3.003 s segments")
	assert.Equal(t, 0, h.hintFor(nil, "other", grid))
}

func TestStartPositionHints_RememberedPerMediaAndExpire(t *testing.T) {
	var hints positionHints[int]
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
	seg, ok := h.prewarmSegment("m1", 7980, legacyAt, now)
	assert.False(t, ok, "no fresh master: a mid-viewing refetch, no warm-up")
	assert.Equal(t, 0, seg)

	h.startHints.remember("m1", 0, now)
	seg, ok = h.prewarmSegment("m1", 7980, legacyAt, now)
	assert.True(t, ok, "a master starting at 0: warm at 0")
	assert.Equal(t, 0, seg)

	h.startHints.remember("m1", 69.4, now)
	seg, ok = h.prewarmSegment("m1", 7980, legacyAt, now)
	assert.True(t, ok)
	assert.Equal(t, 23, seg, "the prod switch: warm at 23, not 0")
	seg, _ = h.prewarmSegment("m1", 0, legacyAt, now)
	assert.Equal(t, 23, seg, "unknown duration: trust the hint")

	_, ok = h.prewarmSegment("m1", 7980, legacyAt, now.Add(startHintTTL+time.Second))
	assert.False(t, ok, "the master is too old: no warm-up")

	h.startHints.remember("m1", 12000, now)
	seg, ok = h.prewarmSegment("m1", 7980, legacyAt, now)
	assert.True(t, ok)
	assert.Equal(t, 0, seg, "a hint past the end is bogus: back to 0")
}

// A rung's init first asked after a long pause (no fresh master) is ensured
// where playback is, not at 0.
func TestInitSegment_FallsBackToTheLivePositionThenZero(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()

	assert.Equal(t, 0, h.initSegment("m1", "fp1", 7980, legacyAt, now), "nothing known: 0")

	h.livePositions.rememberWithin("fp1", 1333, now, livePositionTTL)
	later := now.Add(7 * time.Minute)
	assert.Equal(t, 1333, h.initSegment("m1", "fp1", 7980, legacyAt, later), "after a long pause: the last segment served")
	assert.Equal(t, 0, h.initSegment("m1", "fp2", 7980, legacyAt, later), "another decision does not borrow it")

	h.startHints.remember("m1", 69.4, later)
	assert.Equal(t, 23, h.initSegment("m1", "fp1", 7980, legacyAt, later), "a fresh master wins")

	assert.Equal(t, 0, h.initSegment("m1", "fp1", 7980, legacyAt, now.Add(livePositionTTL+time.Minute)), "too old: 0")
}

// The hint is the start of ONE viewer's playback. Kept per media, a second
// viewer's master at 0 overwrote the first viewer's resume at 40 min within the
// minute, and the first viewer's variant warmed an encoder at 0 nobody read.
func TestStartHint_IsPerViewer(t *testing.T) {
	h := &hlsVODServe{}
	now := time.Now()
	viewer := func(profile string) *http.Request {
		return withHTTPClaims(httptest.NewRequest(http.MethodGet, "/x", nil), &domainuser.Claims{UserID: "u-" + profile, ProfileID: profile})
	}
	alice, bob := viewer("p-alice"), viewer("p-bob")

	h.startHints.remember(startHintKey(alice, "m1"), 1440, now)
	h.startHints.remember(startHintKey(bob, "m1"), 0, now)

	seg, ok := h.prewarmSegment(startHintKey(alice, "m1"), 7980, legacyAt, now)
	assert.True(t, ok)
	assert.Equal(t, 480, seg, "the other viewer's start at 0 must not replace this one's resume")
	seg, _ = h.prewarmSegment(startHintKey(bob, "m1"), 7980, legacyAt, now)
	assert.Equal(t, 0, seg)
}
