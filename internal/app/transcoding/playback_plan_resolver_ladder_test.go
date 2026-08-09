package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ShapeLadder is ruled by one invariant: the manifest a player holds and every
// segment it fetches must shape the SAME ladder. Drift between the two is a
// permanent cache miss, and a rung the box cannot sustain is a 503 that
// collapses playback to the bottom. The tests below pin the three mechanisms
// that hold that line - the freeze, the caps, and their ordering.

func ladderStreams(height int) []*library.MediaStream {
	w := height * 16 / 9
	return []*library.MediaStream{{
		StreamType: library.StreamTypeVideo,
		Codec:      "h264",
		Width:      &w,
		Height:     &height,
	}}
}

func ladderRequest(height int) PlaybackRequest {
	return PlaybackRequest{
		MediaID:             "m1",
		SourcePath:          "/test.mkv",
		Container:           "matroska",
		Streams:             ladderStreams(height),
		Prefs:               DefaultTranscodingPreferences(),
		SubtitleBurnInTrack: -1,
	}
}

func ladderHeights(t *testing.T, profiles []transcoding.HLSProfile) []int {
	t.Helper()
	return profileHeights(profiles)
}

// A live stream's ladder is shaped ONCE and frozen on its admission pin. Every
// later call reads it verbatim - otherwise a mid-stream drift of the mutable
// inputs (complexity class, capability, CMCD override) desyncs the player's
// URLs from the cached bytes.
func TestShapeLadder_FrozenLadderIsReadVerbatim(t *testing.T) {
	frozen := []transcoding.HLSProfile{{Name: "frozen", Height: 1440, VideoBitrate: 6_000_000}}
	trimCalled := false
	pr := &PlanResolver{
		AdmissionFrozenLadder: func(string) ([]transcoding.HLSProfile, bool) { return frozen, true },
		// A ceiling that WOULD trim 1440 away if the freeze were not honoured.
		CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			trimCalled = true
			return transcoding.CapOverride{MaxHeight: 480}, true
		},
	}

	got := pr.ShapeLadder(context.Background(), ladderRequest(2160),
		transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"})

	assert.Equal(t, frozen, got, "a frozen ladder must come back untouched")
	assert.False(t, trimCalled,
		"nothing downstream of the freeze may run - the frozen shape is already final")
}

// stubProducer is a StreamProducer that only answers Profiles - the single
// method ShapeLadder reaches. Everything else would be a lie in this context,
// so it panics rather than silently returning a zero value.
type stubProducer struct {
	profiles []transcoding.HLSProfile
}

func (s *stubProducer) Profiles([]transcoding.HLSProfile, SourceDims, transcoding.PlaybackDecision, transcoding.EncodeCapability) []transcoding.HLSProfile {
	return s.profiles
}

func (s *stubProducer) EnsureSegmentRange(context.Context, DecisionFingerprint, int, int) error {
	panic("stubProducer: ShapeLadder must never produce segments")
}
func (s *stubProducer) PreWarm(DecisionFingerprint, int, int) {
	panic("stubProducer: ShapeLadder must never pre-warm")
}
func (s *stubProducer) WaitBudget() time.Duration { panic("stubProducer: not a serving path") }
func (s *stubProducer) NotifyClientHint(string, int, int, bool, int) {
	panic("stubProducer: not a serving path")
}
func (s *stubProducer) ActiveSessionCount() int { panic("stubProducer: not a serving path") }
func (s *stubProducer) RemoveSession(string)    { panic("stubProducer: not a serving path") }
func (s *stubProducer) Close()                  {}

// A copy rung takes no admission pin, so it is never frozen and never consulted
// against one: its single source-resolution rung is stable by construction.
func TestShapeLadder_CopyRungNeverConsultsTheFreeze(t *testing.T) {
	consulted := false
	streams := ladderStreams(1080)
	// A copy rung needs the keyframe index (the plan is cut from it) and an
	// accurate level (no level -> no CODECS string -> no copy).
	streams[0].Keyframes = []int64{0, 6000, 12000}
	streams[0].Profile = "High"
	streams[0].Level = new(int)
	*streams[0].Level = 40

	req := ladderRequest(1080)
	req.Streams = streams

	pr := &PlanResolver{
		CopyProducer: &stubProducer{profiles: []transcoding.HLSProfile{{Name: "copy", Height: 1080}}},
		AdmissionFrozenLadder: func(string) ([]transcoding.HLSProfile, bool) {
			consulted = true
			return nil, false
		},
	}

	got := pr.ShapeLadder(context.Background(), req,
		transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "copy"})

	require.Len(t, got, 1, "premise: the request really routed to the copy producer")
	assert.False(t, consulted, "a copy rung takes no admission pin, so there is no ladder to freeze")
}

// The CMCD-feedback override is variant-neutral and one-directional: it trims a
// source class showing sustained starvation, and it must never be able to
// advertise a rung above what the producer already shaped.
func TestShapeLadder_CapOverrideOnlyLowers(t *testing.T) {
	req := ladderRequest(2160)
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	base := (&PlanResolver{}).ShapeLadder(context.Background(), req, decision)
	require.NotEmpty(t, base)
	baseTop := ladderHeights(t, base)[0]

	t.Run("a lower ceiling trims", func(t *testing.T) {
		pr := &PlanResolver{CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			return transcoding.CapOverride{MaxHeight: 480}, true
		}}
		got := ladderHeights(t, pr.ShapeLadder(context.Background(), req, decision))
		require.NotEmpty(t, got)
		for _, h := range got {
			assert.LessOrEqual(t, h, 480, "every rung must sit under the starvation ceiling")
		}
	})

	t.Run("a higher ceiling changes nothing", func(t *testing.T) {
		pr := &PlanResolver{CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			return transcoding.CapOverride{MaxHeight: 4320}, true
		}}
		got := ladderHeights(t, pr.ShapeLadder(context.Background(), req, decision))
		require.NotEmpty(t, got)
		assert.Equal(t, baseTop, got[0], "the override must never raise the shaped ceiling")
	})

	t.Run("no override reported", func(t *testing.T) {
		pr := &PlanResolver{CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			return transcoding.CapOverride{}, false
		}}
		assert.Equal(t, baseTop, ladderHeights(t, pr.ShapeLadder(context.Background(), req, decision))[0])
	})
}

// transcoding.av1_max_height holds the AV1 ladder below the source ceiling. It
// keys on the EFFECTIVE output codec, not the requested one: an AV1-preferring
// client on a host with no HW AV1 encoder is served H.264, and capping that
// H.264 ladder with the AV1 knob would punish a codec the admin never limited.
func TestShapeLadder_AV1CeilingFollowsTheEffectiveCodec(t *testing.T) {
	req := ladderRequest(2160)
	av1Decision := transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}

	t.Run("AV1 really served is capped", func(t *testing.T) {
		av1HW := vaapiCapable()
		av1HW.HWCodecs = []string{"h264", "av1"}
		pr := hwResolver(av1HW)
		pr.AV1Enabled = func() bool { return true }
		pr.AV1MaxHeight = func() int { return 720 }

		got := ladderHeights(t, pr.ShapeLadder(context.Background(), req, av1Decision))
		require.NotEmpty(t, got)
		for _, h := range got {
			assert.LessOrEqual(t, h, 720, "the admin AV1 ceiling must hold on a real AV1 encode")
		}
	})

	t.Run("AV1 fallen back to H.264 is not capped", func(t *testing.T) {
		pr := hwResolver(vaapiCapable()) // HWCodecs has no av1
		pr.AV1Enabled = func() bool { return false }
		pr.AV1MaxHeight = func() int { return 720 }

		uncapped := hwResolver(vaapiCapable())
		uncapped.AV1Enabled = func() bool { return false }

		got := ladderHeights(t, pr.ShapeLadder(context.Background(), req, av1Decision))
		want := ladderHeights(t, uncapped.ShapeLadder(context.Background(), req, av1Decision))
		assert.Equal(t, want, got,
			"the AV1 knob must not touch a ladder that will be encoded as H.264")
	})
}

// The freeze must capture the FINAL shape. Freezing the producer's raw ladder
// before the caps ran would hand the next request an untrimmed ladder - the
// exact manifest/segment drift the freeze exists to prevent.
func TestShapeLadder_FreezesTheCappedLadderNotTheRawOne(t *testing.T) {
	var frozen []transcoding.HLSProfile
	var frozenKey string
	pr := &PlanResolver{
		AdmissionFrozenLadder: func(string) ([]transcoding.HLSProfile, bool) { return nil, false },
		AdmissionFreezeLadder: func(key string, ladder []transcoding.HLSProfile) {
			frozenKey, frozen = key, ladder
		},
		CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			return transcoding.CapOverride{MaxHeight: 480}, true
		},
	}

	returned := pr.ShapeLadder(context.Background(), ladderRequest(2160),
		transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"})

	assert.NotEmpty(t, frozenKey, "the freeze must be keyed by the same pin the route used")
	assert.Equal(t, returned, frozen, "what is frozen must be exactly what this request was served")
	require.NotEmpty(t, frozen)
	for _, p := range frozen {
		assert.LessOrEqual(t, p.Height, 480, "the frozen ladder must already carry the caps")
	}
}
