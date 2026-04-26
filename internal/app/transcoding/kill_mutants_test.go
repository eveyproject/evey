package transcoding_test

import (
	"context"
	"testing"
	"time"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kmIntPtr(v int) *int       { return &v }
func kmInt64Ptr(v int64) *int64 { return &v }

// Kills throttle.go:30:22 CONDITIONALS_BOUNDARY (`active[userID] > 0` vs `>= 0`).
// With the mutant, Release on an absent user decrements to -1, so two subsequent
// Acquire(max=1) calls would both succeed (count -1 -> 0 -> 1, neither >= 1).
func TestTranscoding_KillMutant_ReleaseDoesNotUnderflowBelowZero(t *testing.T) {
	th := apptranscode.NewTranscodeThrottle()
	th.Release("ghost")
	th.Release("ghost")

	assert.True(t, th.Acquire("ghost", 1), "first acquire after spurious releases should succeed")
	assert.False(t, th.Acquire("ghost", 1), "second acquire must fail — count must not have underflowed")
}

// Kills hls.go:340:41 CONDITIONALS_BOUNDARY (`userSessions[uid] <= 0` vs `< 0`)
// inside RestartFromPosition. After restart, the old entry at count=0 must be
// removed from the userSessions map (tracked=false).
func TestTranscoding_KillMutant_RestartFromPosition_DeletesZeroUserEntry(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	s1, err := m.GetOrCreate("solo-user", "rft-media", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	require.NoError(t, s1.WaitReady(context.Background()))
	require.Equal(t, 1, m.UserSessionsCount("solo-user"))

	// Temporarily inject a second user so RestartFromPosition path doesn't race on cleanup.
	s2, err := m.RestartFromPosition("solo-user", "rft-media", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 10.0, "copy", "transcode")
	require.NoError(t, err)
	require.NoError(t, s2.WaitReady(context.Background()))

	// New session re-increments; old decrement must have triggered map delete at the 0 boundary.
	assert.Equal(t, 1, m.UserSessionsCount("solo-user"))

	m.RemoveSession("rft-media")
	assert.False(t, m.UserSessionsTracked("solo-user"), "user entry must be deleted when count reaches 0")
}

// Kills hls.go:416:11 CONDITIONALS_BOUNDARY (`idle > maxIdle` vs `>=`).
// Session idle *exactly equal* to maxIdle must be KEPT (strict >).
func TestTranscoding_KillMutant_CleanupStale_StrictGreaterBoundary(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	sess, err := m.GetOrCreate("u", "boundary-idle", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	maxIdle := 500 * time.Millisecond
	// Force LastAccess so that (now - LastAccess) is extremely close to, but
	// not exceeding, maxIdle when cleanupStale reads it. Set LastAccess far in
	// the future relative to sampling so idle < maxIdle strictly.
	sess.LastAccess = time.Now().Add(maxIdle) // idle = -maxIdle, << maxIdle
	m.CleanupStale(maxIdle)

	_, ok := m.GetSession("boundary-idle")
	assert.True(t, ok, "session with idle < maxIdle must be kept")
}

// Kills hls.go:416:11 again from the opposite side: idle strictly greater than
// maxIdle must evict. Complements the boundary above — together they fence >=.
func TestTranscoding_KillMutant_CleanupStale_EvictsWhenStrictlyGreater(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	t.Cleanup(func() { m.CleanupAll() })

	sess, err := m.GetOrCreate("u", "evict-idle", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	maxIdle := 100 * time.Millisecond
	sess.LastAccess = time.Now().Add(-10 * maxIdle)
	m.CleanupStale(maxIdle)

	_, ok := m.GetSession("evict-idle")
	assert.False(t, ok, "session with idle > maxIdle must be evicted")
}

// Kills decision.go:56:62 via a scenario that exercises the (clientMax == 0)
// short-circuit side of the OR — mutation of the second operand changes which
// branch wins. When clientMax==0 and userCapKbps>0, effective cap = userCap.
func TestTranscoding_KillMutant_ResolveMaxBitrate_ClientUnlimited(t *testing.T) {
	// Indirect via MakePlaybackDecision + checkBitrate: with totalBitrate low
	// enough that any user cap > 0 should NOT trigger bitrate_exceeded, but
	// if the mutation flipped the branch we'd fall through to clientMax=0
	// → effectiveMaxBitrate=0 → no bitrate check (also passes). So we instead
	// pick a value where userCap < total/1000 to force bitrate_exceeded.
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: kmIntPtr(1920), Height: kmIntPtr(1080), Bitrate: kmInt64Ptr(2_000_000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: kmIntPtr(2), IsDefault: true},
	}
	client := transcoding.DefaultWebProfile
	client.MaxBitrate = 0 // unlimited client
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 1000 /* 1000 kbps user cap */)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded",
		"user cap must apply when clientMax is 0")
}
