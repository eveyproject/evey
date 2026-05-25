package transcoding_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fake: HLSProcess whose handle fails WaitReady ----------

type waitReadyErrProcess struct {
	err error
}

type waitReadyErrHandle struct {
	err  error
	done chan struct{}
}

func (p *waitReadyErrProcess) Start(_ context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	// Still create master so isTranscodeComplete doesn't short-circuit next call.
	_ = os.WriteFile(filepath.Join(opts.OutputDir, "master.m3u8"), []byte("#EXTM3U\n"), 0644)
	h := &waitReadyErrHandle{err: p.err, done: make(chan struct{})}
	close(h.done)
	return h, nil
}

func (h *waitReadyErrHandle) WaitReady(_ context.Context) error { return h.err }
func (h *waitReadyErrHandle) Stop() error                       { return nil }
func (h *waitReadyErrHandle) Done() <-chan struct{}             { return h.done }
func (h *waitReadyErrHandle) Err() error                        { return h.err }
func (h *waitReadyErrHandle) Pause() error                      { return nil }
func (h *waitReadyErrHandle) Resume() error                     { return nil }

// Kills CONDITIONALS_NEGATION at hls.go:228 (stale cache invalidation).
func TestGetOrCreate_InvalidatesStaleDiskCacheOnDecisionMismatch(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	outputDir := filepath.Join(dir, "media-stale")
	require.NoError(t, os.MkdirAll(outputDir, 0755))
	apptranscode.WriteCachedDecision(outputDir, "transcode", "copy")
	stale := filepath.Join(outputDir, "stale.ts")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0644))

	_, err := m.GetOrCreate("u", "media-stale", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	_, statErr := os.Stat(stale)
	assert.True(t, os.IsNotExist(statErr), "stale segment removed when decisions mismatch")
	assert.True(t, apptranscode.MatchesCachedDecision(outputDir, "copy", "transcode"))
}

// Kills CONDITIONALS_NEGATION at hls.go:228 negative — matching cache preserved,
// Start not invoked.
func TestGetOrCreate_PreservesCacheOnDecisionMatch(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	outputDir := filepath.Join(dir, "media-match")
	require.NoError(t, os.MkdirAll(outputDir, 0755))
	apptranscode.WriteCachedDecision(outputDir, "copy", "transcode")
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))
	marker := filepath.Join(outputDir, "keepme.ts")
	require.NoError(t, os.WriteFile(marker, []byte("data"), 0644))

	_, err := m.GetOrCreate("u", "media-match", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	assert.Empty(t, fake.Started, "cached transcode reused, Start must not be called")
	_, statErr := os.Stat(marker)
	assert.NoError(t, statErr, "cache preserved when decisions match")
}

// Kills CONDITIONALS_NEGATION at hls.go:303 — Start error propagates.
func TestStartSession_PropagatesStartError(t *testing.T) {
	dir := t.TempDir()
	startErr := errors.New("boom-start")
	fake := &fakes.HLSProcess{Err: startErr}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	sess, err := m.GetOrCreate("u", "start-err", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	err = sess.WaitReady(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, startErr)
}

// Kills CONDITIONALS_NEGATION at hls.go:313 — WaitReady error propagates.
func TestStartSession_PropagatesWaitReadyError(t *testing.T) {
	dir := t.TempDir()
	wrErr := errors.New("boom-waitready")
	proc := &waitReadyErrProcess{err: wrErr}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, proc)
	t.Cleanup(func() { m.CleanupAll() })

	sess, err := m.GetOrCreate("u", "wr-err", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	err = sess.WaitReady(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wrErr)
}

// Kills INCREMENT_DECREMENT hls.go:338 + CONDITIONALS at 339 — RestartFromPosition
// must decrement user count and fully clear the user's entry when it reaches zero.
func TestRestartFromPosition_UpdatesUserCounts(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	// userA has 2 sessions, userB has 1.
	_, err := m.GetOrCreate("userA", "mA1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("userA", "mA2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("userB", "mB1", "/c.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	require.Equal(t, 2, m.UserSessionsCount("userA"))
	require.Equal(t, 1, m.UserSessionsCount("userB"))

	// Restart one of userA's sessions — count should stay at 2 (decrement then re-add).
	_, err = m.RestartFromPosition("userA", "mA1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 5.0, "copy", "transcode")
	require.NoError(t, err)
	assert.Equal(t, 2, m.UserSessionsCount("userA"), "restart must preserve total count (decrement then GetOrCreate re-adds)")

	// Restart userB's only session — transient drop then re-add; key preserved with value 1.
	_, err = m.RestartFromPosition("userB", "mB1", "/c.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 5.0, "copy", "transcode")
	require.NoError(t, err)
	assert.Equal(t, 1, m.UserSessionsCount("userB"))
	assert.True(t, m.UserSessionsTracked("userB"))
}

// Kills INCREMENT_DECREMENT hls.go:359 + CONDITIONALS at 360 — RemoveSession
// must decrement and delete user entry at zero.
func TestRemoveSession_UpdatesUserCounts(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	_, err := m.GetOrCreate("u1", "m1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("u1", "m2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	require.Equal(t, 2, m.UserSessionsCount("u1"))

	m.RemoveSession("m1")
	assert.Equal(t, 1, m.UserSessionsCount("u1"), "count decremented by 1")
	assert.True(t, m.UserSessionsTracked("u1"), "user still has 1 session")

	m.RemoveSession("m2")
	assert.Equal(t, 0, m.UserSessionsCount("u1"))
	assert.False(t, m.UserSessionsTracked("u1"), "user entry removed at zero")
}

// Kills INCREMENT_DECREMENT hls.go:382 + CONDITIONALS at 383 — CleanupAll
// clears user counts.
func TestCleanupAll_ClearsUserCounts(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)

	_, err := m.GetOrCreate("u1", "m1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("u1", "m2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("u2", "m3", "/c.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	m.CleanupAll()
	assert.False(t, m.UserSessionsTracked("u1"))
	assert.False(t, m.UserSessionsTracked("u2"))
	assert.Equal(t, 0, m.ActiveSessionCount())
}

// Kills CONDITIONALS_BOUNDARY hls.go:415 — idle > maxIdle (strict, not >=).
func TestCleanupStale_EqualIdleNotCleaned(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	sess, err := m.GetOrCreate("u", "boundary-test", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	// Very recent access (idle ~ 0) — well below any positive threshold.
	sess.MarkAccess("")
	m.CleanupStale(1 * time.Hour)

	_, ok := m.GetSession("boundary-test")
	assert.True(t, ok, "non-stale session must be kept")

	// Strictly greater than threshold — must be cleaned.
	sess.LastAccess = time.Now().Add(-2 * time.Hour)
	m.CleanupStale(1 * time.Hour)
	_, ok = m.GetSession("boundary-test")
	assert.False(t, ok, "idle > maxIdle must be cleaned")
}

// Kills INCREMENT_DECREMENT hls.go:419 + CONDITIONALS at 420 — cleanupStale
// updates user counts.
func TestCleanupStale_UpdatesUserCounts(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	s1, err := m.GetOrCreate("u1", "stale-1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	_, err = m.GetOrCreate("u1", "stale-2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	require.Equal(t, 2, m.UserSessionsCount("u1"))

	// Only s1 is stale.
	s1.LastAccess = time.Now().Add(-11 * time.Minute)
	m.CleanupStale(10 * time.Minute)

	assert.Equal(t, 1, m.UserSessionsCount("u1"), "only stale session decrements")
	assert.True(t, m.UserSessionsTracked("u1"))

	// Now the remaining one.
	s2, ok := m.GetSession("stale-2")
	require.True(t, ok)
	s2.LastAccess = time.Now().Add(-11 * time.Minute)
	m.CleanupStale(10 * time.Minute)

	assert.Equal(t, 0, m.UserSessionsCount("u1"))
	assert.False(t, m.UserSessionsTracked("u1"), "user entry removed when count reaches zero")
}

// Kills CONDITIONALS_BOUNDARY hls.go:108 — maxConcurrent == 0 means unlimited
// (no semaphore). A mutant using >= 0 would create a zero-capacity semaphore
// and every acquire would block until timeout.
func TestNewHLSManager_ZeroMaxConcurrentUnlimited(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 0, fake)
	t.Cleanup(func() { m.CleanupAll() })
	m.SetQueueTimeout(100 * time.Millisecond)

	// Acquire many concurrent sessions rapidly — none should be throttled.
	for i := 0; i < 5; i++ {
		_, err := m.GetOrCreate("u", "unlimited-"+string(rune('a'+i)), "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
		require.NoError(t, err, "maxConcurrent=0 must mean unlimited")
	}
	assert.Equal(t, 5, m.ActiveSessionCount())
}

// Kills CONDITIONALS_NEGATION hls.go:461 — matchesCachedDecision returns false
// when no decision file exists (os.ReadFile error).
func TestMatchesCachedDecision_MissingFile(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, apptranscode.MatchesCachedDecision(dir, "copy", "transcode"))
}

// Kills CONDITIONALS_NEGATION hls.go:461 positive — exact match returns true,
// partial mismatch returns false.
func TestMatchesCachedDecision_Match(t *testing.T) {
	dir := t.TempDir()
	apptranscode.WriteCachedDecision(dir, "copy", "transcode")
	assert.True(t, apptranscode.MatchesCachedDecision(dir, "copy", "transcode"))
	assert.False(t, apptranscode.MatchesCachedDecision(dir, "copy", "copy"))
	assert.False(t, apptranscode.MatchesCachedDecision(dir, "transcode", "transcode"))
}

// Kills CONDITIONALS_NEGATION hls.go:454 — writeCachedDecision writes a file
// readable by matchesCachedDecision (content round-trip).
func TestWriteCachedDecision_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	apptranscode.WriteCachedDecision(dir, "transcode", "copy")

	// File exists.
	data, err := os.ReadFile(filepath.Join(dir, ".decision"))
	require.NoError(t, err)
	assert.Equal(t, "transcode\ncopy", string(data))

	// matchesCachedDecision agrees.
	assert.True(t, apptranscode.MatchesCachedDecision(dir, "transcode", "copy"))
}
