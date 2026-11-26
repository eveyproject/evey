package playback_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	"github.com/eveyproject/evey/internal/domain/playback"
	playbackfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// countingSessions counts the writes the witness makes.
type countingSessions struct {
	*playbackfakes.FakePlaySessionRepository
	served, played atomic.Int32
}

func (c *countingSessions) SetServed(ctx context.Context, id string, s playback.Served) (bool, error) {
	c.served.Add(1)
	return c.FakePlaySessionRepository.SetServed(ctx, id, s)
}

func (c *countingSessions) SetPlayed(ctx context.Context, id string, p playback.Played) (bool, error) {
	c.played.Add(1)
	return c.FakePlaySessionRepository.SetPlayed(ctx, id, p)
}

func newWitnessSvc(t *testing.T) (*appplayback.CommandService, *countingSessions) {
	t.Helper()
	sessions := &countingSessions{FakePlaySessionRepository: playbackfakes.NewFakePlaySessionRepository()}
	svc := appplayback.NewCommandService(playbackfakes.NewFakeWatchProgressRepository(), sessions, sessions,
		userfakes.NewFakeUserRepository(), userfakes.NewFakeProfileRepository(), sharedfakes.NewFakeEventBus())
	return svc, sessions
}

// Each leg is the latest bytes of its kind: an audio rendition switched in
// place replaces the audio leg and leaves the video the packager is serving.
func TestRecordServed_EachLegIsTheLatestBytesOfItsKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))

	video := playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: -1, Codec: "h264", Height: 1080}
	aac := playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: 2, Codec: "aac"}
	mp3ToAAC := playback.ServedLeg{Engine: playback.EngineFFmpeg, Track: 1, Codec: "aac", SourceCodec: "mp3"}
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegVideo, video))
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, aac))

	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	require.NotNil(t, got.Served.Video)
	require.NotNil(t, got.Served.Audio)
	assert.Equal(t, 2, got.Served.Audio.Track)
	assert.Equal(t, string(playback.DirectStream), got.PlayMethod)
	assert.Equal(t, "1080p", got.Resolution)

	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, mp3ToAAC))
	got, err = sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	assert.Equal(t, 1, got.Served.Audio.Track, "the switched-to track is what goes out now")
	assert.Equal(t, "mp3", got.Served.Audio.SourceCodec)
	assert.Equal(t, playback.EngineMkvgo, got.Served.Video.Engine, "the video leg is untouched")
	assert.Equal(t, string(playback.Transcode), got.PlayMethod, "an encoded leg makes it a transcode")
}

// A segment goes out every few seconds per leg: only a change reaches the row.
func TestRecordServed_OnlyAChangeIsWritten(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))

	l := playback.ServedLeg{Engine: playback.EngineFFmpeg, Copy: true, Track: 0, Codec: "aac"}
	for range 10 {
		require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, l))
	}
	assert.Equal(t, int32(1), sessions.served.Load())

	l.Track = 1
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, l))
	assert.Equal(t, int32(2), sessions.served.Load())
}

// Bytes before the session exists (a player that fetched before StartStream,
// a prewarm) are not written anywhere; the session a start then creates
// receives the next bytes at once, not after the remembered miss expires.
func TestRecordServed_ASessionStartedAfterAMissReceivesTheNextBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	seedUserProfile(ctx, ur, pfr, nil)

	l := playback.ServedLeg{Engine: playback.EngineFile, Copy: true, Track: -1, Codec: "h264"}
	require.NoError(t, cmd.RecordServed(ctx, "p1", "m1", playback.LegVideo, l))

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}, "u1"))
	got, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.Nil(t, got.Served.Video, "bytes sent before the session are nobody's")
	assert.Empty(t, got.PlayMethod, "a fresh session names no method until bytes go out")

	require.NoError(t, cmd.RecordServed(ctx, "p1", "m1", playback.LegVideo, l))
	got, err = sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, got.Served.Video)
	assert.Equal(t, string(playback.DirectPlay), got.PlayMethod)
}

// An ended session is never rewritten; bytes after it look for a new one.
func TestRecordServed_EndedSessionIsLeftAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))
	l := playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: -1, Codec: "h264"}
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegVideo, l))

	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	ended := time.Now()
	got.EndedAt = &ended
	require.NoError(t, sessions.Update(ctx, got))

	l.Codec = "hevc"
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegVideo, l))
	got, err = sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	assert.Equal(t, "h264", got.Served.Video.Codec)
}

func TestRecordPlayed_WritesWhatThePlayerDecodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))

	p := playback.Played{Source: playback.PlayedHLS, AudioTrack: 2, AudioCodec: "ac3", AudioDecoder: playback.DecoderWasm}
	require.NoError(t, svc.RecordPlayed(ctx, "prof-1", "media-1", p))
	require.NoError(t, svc.RecordPlayed(ctx, "prof-1", "media-1", p))
	assert.Equal(t, int32(1), sessions.played.Load(), "an unchanged report is not rewritten")

	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	require.NotNil(t, got.Played)
	assert.Equal(t, playback.DecoderWasm, got.Played.AudioDecoder)
	assert.Equal(t, 2, got.Played.AudioTrack)
}

// Measured on the demo: the player fetched an audio rendition's init,
// dropped it for the file itself, and the init finished last. The file began
// later: it is what is being served, the init is older news.
func TestRecordServed_AResponseThatBeganEarlierIsOlderNews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))

	t0 := time.Now()
	file := playback.ServedLeg{Engine: playback.EngineFile, Copy: true, Track: -1, At: t0.Add(200 * time.Millisecond)}
	init := playback.ServedLeg{Engine: playback.EngineFFmpeg, Track: 1, Codec: "aac", SourceCodec: "ac3", At: t0}
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, file))
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, init))

	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	assert.Equal(t, playback.EngineFile, got.Served.Audio.Engine)
}

// Measured on the demo: a sender page's hidden cast element loaded the cast
// URL and pulled the first audio rendition (track 0) while the player was
// playing track 2. Those bytes are not the playback; a receiver that really
// plays is, once the player's own fetches have stopped.
func TestRecordServedCast_YieldsToThePlayersOwnFreshBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "")))

	t0 := time.Now().Add(-time.Minute)
	own := playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: 2, Codec: "aac", At: t0}
	preload := playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: 0, Codec: "mp3", At: t0.Add(2 * time.Second)}
	require.NoError(t, svc.RecordServed(ctx, "prof-1", "media-1", playback.LegAudio, own))
	require.NoError(t, svc.RecordServedCast(ctx, "prof-1", "media-1", playback.LegAudio, preload))

	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Served.Audio.Track, "the hidden element's bytes do not replace the player's")

	receiver := preload
	receiver.At = t0.Add(30 * time.Second)
	require.NoError(t, svc.RecordServedCast(ctx, "prof-1", "media-1", playback.LegAudio, receiver))
	got, err = sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	assert.Equal(t, 0, got.Served.Audio.Track, "the player went quiet: the cast bytes are the playback")
}

// A cast with no player behind it (the sender is gone, or never fetched this
// leg) is the only playback there is.
func TestRecordServedCast_AloneIsRecorded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, sessions := newWitnessSvc(t)
	require.NoError(t, sessions.Create(ctx, activeSession("s-1", "prof-1", "media-1", "TV")))

	l := playback.ServedLeg{Engine: playback.EngineFFmpeg, Track: 1, Codec: "aac", SourceCodec: "ac3"}
	require.NoError(t, svc.RecordServedCast(ctx, "prof-1", "media-1", playback.LegAudio, l))
	got, err := sessions.GetByID(ctx, "s-1")
	require.NoError(t, err)
	require.NotNil(t, got.Served.Audio)
	assert.Equal(t, 1, got.Served.Audio.Track)
}

// A new start on the same device reuses the live row: it is another playback,
// and the audio the previous one was served is not what this one plays.
func TestStartStream_AReusedRowStartsWithNothingServed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	seedUserProfile(ctx, ur, pfr, nil)
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}, "u1"))
	require.NoError(t, cmd.RecordServed(ctx, "p1", "m1", playback.LegAudio, playback.ServedLeg{Engine: playback.EngineFFmpeg, Track: 0, Codec: "aac", SourceCodec: "mp3"}))
	require.NoError(t, cmd.RecordPlayed(ctx, "p1", "m1", playback.Played{Source: playback.PlayedHLS, AudioTrack: 0}))

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}, "u1"))
	got, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.Nil(t, got.Served.Audio)
	assert.Nil(t, got.Played)
	assert.Empty(t, got.PlayMethod)

	require.NoError(t, cmd.RecordServed(ctx, "p1", "m1", playback.LegAudio, playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Track: 2, Codec: "aac"}))
	got, err = sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, got.Served.Audio)
	assert.Equal(t, 2, got.Served.Audio.Track, "the new start's bytes reach the reused row")
}
