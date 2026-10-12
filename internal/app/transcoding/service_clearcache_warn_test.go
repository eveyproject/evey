package transcoding

// Internal test: clearing the transcode cache while a session is encoding must
// SAY SO. The wipe deletes files out from under a live ffmpeg, which then dies
// mid-write; on 2026-09-04 that error burst in production was mistaken for an
// orchestrator race and cost a full investigation to rule out. The warning is
// the whole point of the change - so it is asserted, not left to survive by
// luck through the next refactor.

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domtc "github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs swaps the default slog logger for one writing into a buffer, and
// restores it when the test ends.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// newCacheSvc returns a service whose cache holds one file, so "was it really
// cleared" is observable through the store's own API.
func newCacheSvc(t *testing.T) (*Service, *tcfakes.FakeCacheStore) {
	t.Helper()
	store := tcfakes.NewFakeCacheStore()
	store.AddFile(domtc.CacheFileInfo{Name: "seg.m4s", Size: 42})
	svc := NewService(tcfakes.NewFakeTranscoder(), tcfakes.NewFakeHWDetector(),
		tcfakes.NewFakeTranscodeJobRepository(), tcfakes.NewFakeTranscodeProfileRepository(),
		sharedfakes.NewFakeEventBus(), 0, WithCacheStore(store))
	return svc, store
}

func fileCount(t *testing.T, store *tcfakes.FakeCacheStore) int {
	t.Helper()
	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	return info.FileCount
}

func TestClearCache_WarnsWhenPlaybackIsEncoding(t *testing.T) {
	buf := captureLogs(t)
	svc, _ := newCacheSvc(t)
	svc.SetPlaybackGauge(&fakeGauge{n: 2})

	require.NoError(t, svc.ClearCache(context.Background()))

	out := buf.String()
	assert.Contains(t, out, "clearing the transcode cache while playback is encoding",
		"a wipe during a live encode must be recorded, or the resulting ffmpeg errors look like a bug")
	assert.Contains(t, out, "live_encoding_sessions=2",
		"the count is what tells the reader how many sessions were hit")
}

// The counterpart: no live encode, no scary line. A warning that fires on every
// wipe teaches the reader to ignore it, which defeats the purpose.
func TestClearCache_QuietWhenNothingIsEncoding(t *testing.T) {
	buf := captureLogs(t)
	svc, _ := newCacheSvc(t)
	svc.SetPlaybackGauge(&fakeGauge{n: 0})

	require.NoError(t, svc.ClearCache(context.Background()))

	assert.NotContains(t, buf.String(), "while playback is encoding")
}

// A wipe must never be REFUSED because someone is watching: the user's call on
// 2026-09-04 is that warning is enough, and an admin action that silently
// declines to run is worse than one that costs a respawn.
func TestClearCache_NeverRefusedBecauseOfPlayback(t *testing.T) {
	svc, store := newCacheSvc(t)
	svc.SetPlaybackGauge(&fakeGauge{n: 5})
	require.Equal(t, 1, fileCount(t, store), "precondition: the cache starts non-empty")

	require.NoError(t, svc.ClearCache(context.Background()), "the wipe must still happen")

	assert.Zero(t, fileCount(t, store), "the store must actually have been cleared")
}

// No gauge wired (a server built without playback accounting) must not fault on
// the nil interface - the wipe is a plain admin action that has to keep working.
func TestClearCache_NoGaugeWired(t *testing.T) {
	svc, store := newCacheSvc(t)

	require.NoError(t, svc.ClearCache(context.Background()))

	assert.Zero(t, fileCount(t, store))
}

// The wipe also empties the derived stores handed in - the extracted
// subtitles under /data, which no button reached before - and a failure there
// is logged, not returned: the transcode cache IS cleared at that point.
func TestClearCache_PurgesDerivedStoresAndLogsTheirFailures(t *testing.T) {
	logs := captureLogs(t)
	store := tcfakes.NewFakeCacheStore()
	store.AddFile(domtc.CacheFileInfo{Name: "seg.m4s", Size: 42})
	purged := 0
	svc := NewService(tcfakes.NewFakeTranscoder(), tcfakes.NewFakeHWDetector(),
		tcfakes.NewFakeTranscodeJobRepository(), tcfakes.NewFakeTranscodeProfileRepository(),
		sharedfakes.NewFakeEventBus(), 0, WithCacheStore(store),
		WithDerivedCachePurge(func(context.Context) (int, error) { purged++; return 3, nil }),
		WithDerivedCachePurge(func(context.Context) (int, error) { return 0, assert.AnError }),
		WithDerivedCachePurge(nil),
	)
	require.NoError(t, svc.ClearCache(context.Background()))
	assert.Equal(t, 1, purged, "the derived purge runs once per wipe")
	assert.Equal(t, 0, fileCount(t, store), "the transcode cache is cleared first")
	assert.Contains(t, logs.String(), "derived store purged")
	assert.Contains(t, logs.String(), "derived store purge failed")
}
