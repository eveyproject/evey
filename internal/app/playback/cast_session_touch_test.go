package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/playback"
	playbackfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TouchCastSession is the TV-driven session keepalive: a cast receiver's
// liveness beacon refreshes the active session of (profile, media) - position
// AND updated_at - so a phone-orphaned cast neither vanishes from the
// now-playing panel nor gets reaped as stale while the TV plays.

func newTouchSvc(t *testing.T) (*appplayback.CommandService, *playbackfakes.FakePlaySessionRepository) {
	t.Helper()
	sessions := playbackfakes.NewFakePlaySessionRepository()
	progress := playbackfakes.NewFakeWatchProgressRepository()
	users := userfakes.NewFakeUserRepository()
	profiles := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	return appplayback.NewCommandService(progress, sessions, sessions, users, profiles, bus), sessions
}

func activeSession(id, profileID, mediaID, device string) *playback.PlaySession {
	return &playback.PlaySession{
		ID:             id,
		ProfileID:      shared.ProfileID(profileID),
		MediaItemID:    shared.MediaItemID(mediaID),
		StartedAt:      time.Now().Add(-time.Minute),
		CastDeviceName: device,
	}
}

func TestTouchCastSession_RefreshesTheActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 214_200, AllowCreate: false})
	require.NoError(t, err)
	assert.True(t, touched)

	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	require.NotNil(t, got.Position)
	assert.Equal(t, int64(214_200), got.Position.Milliseconds(), "beacon position lands on the session")
	assert.Equal(t, "Salon TV", got.CastDeviceName, "the device name the phone set is preserved")
	assert.False(t, got.IsPaused, "an alive-beating receiver is playing")
}

// Superseded 2026-07-29: a positioned beacon with no session now RESURRECTS
// one (see TestTouchCastSession_ResurrectsWhenNoActiveSession) - the no-op
// contract hid a real playback from the panel in prod.

func TestTouchCastSession_IgnoresOtherMediaOfTheProfile(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-OTHER", "")))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 5_000, AllowCreate: true})
	require.NoError(t, err)
	assert.True(t, touched, "resurrects a session for media-1 alongside")

	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Nil(t, got.Position, "the other media's session is untouched")
}

// ConsumeCastCommand routes an admin session command (pause/play/stop/message)
// to the TV itself: the receiver's alive beacon polls it, so a frozen phone no
// longer takes the admin's remote control down with it. Destructive read, same
// as the phone's ReportProgress consume path.
func TestConsumeCastCommand_DeliversThePendingCommandOnce(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))
	require.NoError(t, svc.SetRemoteCommand(context.Background(), "s-1", "pause"))

	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Equal(t, "pause", cmd)

	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd, "a consumed command does not replay")
}

func TestConsumeCastCommand_NoSessionOrNoCommand(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	cmd, err := svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd)

	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "")))
	cmd, err = svc.ConsumeCastCommand(context.Background(), "prof-1", "media-1")
	require.NoError(t, err)
	assert.Empty(t, cmd)
}

// SetCastAudioPath mirrors the receiver's audio_mode beacon onto the session
// row so the admin panel's badge tells the truth in near real time: the AAC
// leg IS a server transcode, the wasm leg is TV-side decode (the server's
// ffmpeg dies at idle seconds later).
func TestSetCastAudioPath_MapsModesOntoTheSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "Salon TV")))

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "wasm"))
	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "wasm", got.AudioDecision)
	assert.Equal(t, "client", got.Serving.AudioEngine, "wasm = the player decodes")

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision, "the AAC leg is a server transcode")
	assert.Equal(t, "ffmpeg", got.Serving.AudioEngine)

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac-latched"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision)

	// Unknown mode strings from a future receiver: ignored, never written.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "weird"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "transcode", got.AudioDecision)

	// No active session: a quiet no-op.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-OTHER", "wasm"))
}

// Prod, 2026-09-20: the beacon flipped the DECISION to transcode and left the
// codec label written at start-up, so the dashboard read "transcodé (eac3)" -
// a transcode whose output codec is its input. The label has to follow the leg.
func TestSetCastAudioPath_CodecLabelFollowsTheLeg(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-1", "prof-1", "media-1", "")
	sess.AudioCodec = "eac3" // what the client reported while the track was carried untouched
	require.NoError(t, sessions.Create(context.Background(), sess))

	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac-latched"))
	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "eac3→aac", got.AudioCodec, "the AAC leg re-encodes: say what comes out")

	// Back on the untouched track: the source IS what is served.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "wasm"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "eac3", got.AudioCodec)

	// A second latch must not stack arrows.
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac"))
	require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac-latched"))
	got, err = sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "eac3→aac", got.AudioCodec)
}

// A label that says nothing useful must not be invented: an AAC source fed to
// the AAC leg is not "aac→aac", and a session whose codec was never reported
// (an older client) must not gain one from a beacon.
func TestSetCastAudioPath_LeavesAnUninformativeLabelAlone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, codec string }{{"aac source", "aac"}, {"no label", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, sessions := newTouchSvc(t)
			sess := activeSession("s-1", "prof-1", "media-1", "")
			sess.AudioCodec = tc.codec
			require.NoError(t, sessions.Create(context.Background(), sess))

			require.NoError(t, svc.SetCastAudioPath(context.Background(), "prof-1", "media-1", "aac-latched"))
			got, err := sessions.GetByID(context.Background(), "s-1")
			require.NoError(t, err)
			assert.Equal(t, tc.codec, got.AudioCodec)
		})
	}
}

// Resurrection: prod 2026-07-29 - the phone re-cast from an already-mounted
// player (no new StartStream) after its previous session had been reaped; the
// TV played for real while "now playing" stayed empty and nothing was
// pilotable. A positioned liveness beacon proves a genuine playback pinned to
// (profile, media): create the minimal session row; the reaper ends it when
// the beacons stop.
func TestTouchCastSession_ResurrectsWhenNoActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 214_200, AllowCreate: true})
	require.NoError(t, err)
	assert.True(t, touched, "a positioned beacon with no session must create one")

	active, err := sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "media-1", string(active[0].MediaItemID))
	assert.Equal(t, "Chromecast", active[0].DeviceName)
	assert.Equal(t, "Chromecast", active[0].CastDeviceName)
	require.NotNil(t, active[0].Position)
	assert.Equal(t, int64(214_200), active[0].Position.Milliseconds())

	// The next beacon touches the SAME session (no duplicate rows).
	touched, err = svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 244_200, AllowCreate: true})
	require.NoError(t, err)
	assert.True(t, touched)
	active, err = sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

// An unusable beacon must be a silent no-op: no row, and no error to the
// caller (the receiver keeps beaconing either way). Nothing pinned that
// contract before.
//
// It now holds twice over, which was the point of routing the resurrect path
// through NewPlaySession: deleting the early-return guard at the top of
// TouchCastSession keeps this test GREEN, because the constructor's Validate()
// refuses the same rows. Verified by removing that guard and re-running. With
// the previous struct literal, neither layer existed once the guard was gone.
func TestTouchCastSession_IgnoresAnUnusableBeacon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		profileID  string
		mediaID    string
		positionMs int64
	}{
		{"no profile", "", "media-1", 214_200},
		{"no media", "prof-1", "", 214_200},
		{"negative position", "prof-1", "media-1", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, sessions := newTouchSvc(t)

			touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: tc.profileID, MediaItemID: tc.mediaID, PositionMs: tc.positionMs, AllowCreate: true})
			require.NoError(t, err, "an unusable beacon is ignored, never an error to the caller")
			assert.False(t, touched, "nothing was resurrected")

			active, err := sessions.GetActiveByProfile(context.Background(), tc.profileID)
			require.NoError(t, err)
			assert.Empty(t, active, "no row may reach the store")
		})
	}
}

// Position zero is the FIRST beat of a cast that has just started (and the
// position after a rewind to the beginning). The guard reads `positionMs < 0`,
// and mutation (2026-08-01) showed nothing distinguished it from `<= 0`: turned
// into the latter, the opening heartbeat of every cast would be dropped and the
// now-playing panel would stay blind until the viewer had watched a second -
// exactly the blindness this keepalive exists to prevent.
func TestTouchCastSession_AcceptsPositionZeroAndRejectsNegative(t *testing.T) {
	t.Parallel()

	t.Run("zero is a real position", func(t *testing.T) {
		svc, sessions := newTouchSvc(t)
		require.NoError(t, sessions.Create(context.Background(), activeSession("s-z", "prof-z", "media-z", "Salon TV")))

		touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-z", MediaItemID: "media-z", PositionMs: 0, AllowCreate: true})
		require.NoError(t, err)
		assert.True(t, touched, "a cast beating at position 0 has just started - dropping it "+
			"leaves the session looking dead until the first second elapses")

		got, err := sessions.GetByID(context.Background(), "s-z")
		require.NoError(t, err)
		require.NotNil(t, got.Position)
		assert.Equal(t, int64(0), got.Position.Milliseconds())
	})

	t.Run("a negative position is refused", func(t *testing.T) {
		svc, sessions := newTouchSvc(t)
		require.NoError(t, sessions.Create(context.Background(), activeSession("s-n", "prof-n", "media-n", "Salon TV")))

		touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-n", MediaItemID: "media-n", PositionMs: -1, AllowCreate: true})
		require.NoError(t, err)
		assert.False(t, touched, "a negative position is not a place in the media")
	})
}

// ── Une position figée n'est pas une preuve de vie ────────────────────────────
//
// The receiver's heartbeat is a bare setInterval started at
// PLAYER_LOAD_COMPLETE and stopped only at MEDIA_FINISHED / SHUTDOWN /
// pagehide: it keeps beating through a stall, and a television that is never
// really switched off keeps the receiver page loaded for days. Since
// UpdateProgress writes updated_at=NOW() whether or not the position moved, one
// beat every 30s made the 2-minute stale reaper structurally unreachable.
//
// Measured on prod 2026-09-04: a session held open 3h26 with the playhead
// frozen at 19min16, is_paused false, and zero encoding sessions on the box.

func TestTouchCastSession_AFrozenPlayheadWritesNothing(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-stall", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(1_156_157)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	// The exact prod signature: beat after beat on the same position.
	for range 6 {
		touched, tErr := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 1_156_157, AllowCreate: false})
		require.NoError(t, tErr)
		assert.False(t, touched, "a beat that proves no advancement must not refresh the session")
	}

	got, err := sessions.GetByID(context.Background(), "s-stall")
	require.NoError(t, err)
	assert.Nil(t, got.EndedAt, "the beacon does not end the session either - the ordinary reaper does")
}

func TestTouchCastSession_AnAdvancingPlayheadStillRefreshes(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-live", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(60_000)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	// One beacon interval of real playback.
	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 90_000, AllowCreate: false})
	require.NoError(t, err)
	assert.True(t, touched)

	got, err := sessions.GetByID(context.Background(), "s-live")
	require.NoError(t, err)
	require.NotNil(t, got.Position)
	assert.Equal(t, int64(90_000), got.Position.Milliseconds())
	assert.False(t, got.IsPaused, "false is now MEASURED - we just saw the playhead move")
}

// A seek backwards is a human at the remote, so it counts as life even though
// the position went down. Written because the obvious implementation (`new >
// old`) would drop it and reap a session someone is actively driving.
func TestTouchCastSession_ABackwardsSeekIsAlsoLife(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-back", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(600_000)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 300_000, AllowCreate: false})
	require.NoError(t, err)
	assert.True(t, touched, "someone rewound - that is a viewer, not a stall")
}

// The tolerance exists so float noise on a frozen currentTime cannot pass for
// playback: the beacon sends seconds as a float and the server scales by 1000.
func TestTouchCastSession_SubSecondJitterIsNotProgress(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-jit", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(1_156_157)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 1_156_157+40, AllowCreate: false})
	require.NoError(t, err)
	assert.False(t, touched, "40ms over a 30s beat is noise, not a moving picture")
}

// A session that never received a progress tick has no stored position to
// compare against; refusing its first beacon would strand exactly the
// phone-orphaned cast this keepalive exists for.
func TestTouchCastSession_FirstBeaconWithNoStoredPositionCounts(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-new", "prof-1", "media-1", "Salon TV")))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 214_200, AllowCreate: false})
	require.NoError(t, err)
	assert.True(t, touched)
}

// ── Seul un chargement peut créer une session ─────────────────────────────────

func TestTouchCastSession_AnAliveBeatNeverResurrects(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 214_200, AllowCreate: false})
	require.NoError(t, err)
	assert.False(t, touched, "an endless alive beat is not a new playback")

	active, err := sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	assert.Empty(t, active, "otherwise the reaper and the beat fight forever")
}

// ── Le beacon de fin termine la session ───────────────────────────────────────

func TestEndCastSession_EndsTheActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-end", "prof-1", "media-1", "Salon TV")))

	require.NoError(t, svc.EndCastSession(context.Background(), "prof-1", "media-1"))

	got, err := sessions.GetByID(context.Background(), "s-end")
	require.NoError(t, err)
	require.NotNil(t, got.EndedAt, "the receiver announced its own death - do not wait 2 minutes for silence")

	active, err := sessions.GetActiveByProfile(context.Background(), "prof-1")
	require.NoError(t, err)
	assert.Empty(t, active)
}

// The receiver legitimately sends shutdown twice (SHUTDOWN then pagehide), and
// may send one with nothing playing at all.
func TestEndCastSession_IsIdempotentAndSurvivesNoSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-end2", "prof-1", "media-1", "")))

	require.NoError(t, svc.EndCastSession(context.Background(), "prof-1", "media-1"))
	require.NoError(t, svc.EndCastSession(context.Background(), "prof-1", "media-1"))
	require.NoError(t, svc.EndCastSession(context.Background(), "prof-1", "media-NEVER"))
	require.NoError(t, svc.EndCastSession(context.Background(), "", ""))
}

// Another profile's cast, or another title of the same profile, must be left
// alone: the beacon names one (profile, media) pair and ends only that.
func TestEndCastSession_TouchesOnlyItsOwnPair(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-keep", "prof-1", "media-OTHER", "")))
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-gone", "prof-1", "media-1", "")))

	require.NoError(t, svc.EndCastSession(context.Background(), "prof-1", "media-1"))

	keep, err := sessions.GetByID(context.Background(), "s-keep")
	require.NoError(t, err)
	assert.Nil(t, keep.EndedAt, "the profile's other title keeps playing")
}

// ── Une pause est une preuve de vie, un blocage n'en est pas une ──────────────
//
// Both look identical from the server's seat - a playhead that stopped moving -
// and the receiver's heartbeat beats through either. The beacon's paused flag is
// what separates them, so a viewer who pauses is not treated as a dead session
// and a stalled television still is.

func TestTouchCastSession_APausedReceiverStaysAlive(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-pause", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(1_156_157)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	// Frozen playhead, but the television says a human paused it.
	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{
		ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 1_156_157, Paused: true,
	})
	require.NoError(t, err)
	assert.True(t, touched, "somebody paused it - that is a viewer, not a phantom")

	got, err := sessions.GetByID(context.Background(), "s-pause")
	require.NoError(t, err)
	assert.True(t, got.IsPaused, "and the panel must say so rather than claim it is playing")
}

// The whole point of the flag: without it, this and the test above are the same
// request. A receiver that still believes it is playing while frozen is the
// prod signature of 2026-09-04 and must still age out.
func TestTouchCastSession_AStallStillDiesWhenNotPaused(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-stall2", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(1_156_157)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{
		ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 1_156_157, Paused: false,
	})
	require.NoError(t, err)
	assert.False(t, touched)
}

// Resuming must clear the flag, or the row would stay labelled "paused" for the
// rest of the film.
func TestTouchCastSession_ResumingClearsThePausedFlag(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-resume", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(60_000)
	require.NoError(t, err)
	sess.Position = &pos
	sess.IsPaused = true
	require.NoError(t, sessions.Create(context.Background(), sess))

	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{
		ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 90_000, Paused: false,
	})
	require.NoError(t, err)
	assert.True(t, touched)

	got, err := sessions.GetByID(context.Background(), "s-resume")
	require.NoError(t, err)
	assert.False(t, got.IsPaused)
}

// A receiver too old to send the flag leaves it false, so the playhead alone
// decides - exactly the behaviour shipped before it existed. The published
// receiver bundle lags the source, so this is the live case, not a hypothetical.
func TestTouchCastSession_AnOlderReceiverFallsBackToThePlayhead(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	sess := activeSession("s-old", "prof-1", "media-1", "Salon TV")
	pos, err := shared.NewDurationMs(60_000)
	require.NoError(t, err)
	sess.Position = &pos
	require.NoError(t, sessions.Create(context.Background(), sess))

	// No Paused field at all: the picture moving is still proof of life.
	touched, err := svc.TouchCastSession(context.Background(), appplayback.CastBeacon{
		ProfileID: "prof-1", MediaItemID: "media-1", PositionMs: 90_000,
	})
	require.NoError(t, err)
	assert.True(t, touched)
}

// The master render names the machinery behind each track; the panel reads
// it off the session instead of guessing "copy = mkvgo".
func TestSetSessionServing_RecordsThePathOnTheActiveSession(t *testing.T) {
	t.Parallel()
	svc, sessions := newTouchSvc(t)
	require.NoError(t, sessions.Create(context.Background(), activeSession("s-1", "prof-1", "media-1", "")))

	p := playback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: "qsv", AudioDecision: "transcode", AudioEngine: "ffmpeg"}
	require.NoError(t, svc.SetSessionServing(context.Background(), "prof-1", "media-1", p))
	got, err := sessions.GetByID(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, p, got.Serving)
	assert.Equal(t, "transcode", got.VideoDecision)

	// No active session for that media: a quiet no-op.
	require.NoError(t, svc.SetSessionServing(context.Background(), "prof-1", "media-OTHER", p))
}
