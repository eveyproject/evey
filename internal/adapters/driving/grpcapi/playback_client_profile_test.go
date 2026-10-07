package grpcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// clientProfileFromRequest is the seam where an untrusted client tells the
// server what it can decode. Every field here can only ever LIFT a conservative
// default, and an omitted field must leave that default standing - a client
// that says nothing about HDR keeps tone-mapping rather than being handed raw
// PQ its display would wash out.

func TestClientProfileFromRequest_OmittedFieldsKeepTheWebDefault(t *testing.T) {
	got := clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{}, "")

	assert.Equal(t, transcoding.DefaultWebProfile, got,
		"an empty request must reproduce the conservative default exactly")
	assert.False(t, got.SupportsHDR,
		"silence about HDR is not consent: the tonemap stays on")
}

func TestClientProfileFromRequest_EmptyListsDoNotErasTheDefault(t *testing.T) {
	// A client sending explicit empty lists (a serialisation quirk, or a client
	// that filled the field with nothing) must not end up with NO decodable
	// codec at all - that would force a transcode to something it cannot play.
	got := clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{
		SupportedCodecs:      []string{},
		SupportedAudioCodecs: []string{},
		SupportedContainers:  []string{},
	}, "")

	assert.Equal(t, transcoding.DefaultWebProfile.VideoCodecs, got.VideoCodecs)
	assert.Equal(t, transcoding.DefaultWebProfile.AudioCodecs, got.AudioCodecs)
	assert.Equal(t, transcoding.DefaultWebProfile.Containers, got.Containers)
}

func TestClientProfileFromRequest_DeclaredCapabilitiesReplaceTheDefault(t *testing.T) {
	got := clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{
		SupportedCodecs:      []string{"hevc", "av1"},
		SupportedAudioCodecs: []string{"eac3"},
		SupportedContainers:  []string{"mkv"},
		MaxWidth:             3840,
		MaxHeight:            2160,
		SupportsHdr:          true,
	}, "")

	assert.Equal(t, []string{"hevc", "av1"}, got.VideoCodecs,
		"a declared list REPLACES the default, it does not merge with it")
	assert.Equal(t, []string{"eac3"}, got.AudioCodecs)
	assert.Equal(t, []string{"mkv"}, got.Containers)
	assert.Equal(t, 3840, got.MaxWidth)
	assert.Equal(t, 2160, got.MaxHeight)
	assert.True(t, got.SupportsHDR)
}

// The H.264 level ceiling is "> 0 means set": an older client that sends the
// proto default keeps the level-blind decision, a declared ceiling is copied
// as-is (the decision compares it to the stream's level_idc).
func TestClientProfileFromRequest_H264LevelCeiling(t *testing.T) {
	assert.Equal(t, 0, clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{}, "").MaxH264Level)
	assert.Equal(t, 41, clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{MaxH264Level: 41}, "").MaxH264Level)
	assert.Equal(t, 0, clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{MaxH264Level: -3}, "").MaxH264Level)
}

// The dimension caps are "> 0 means set". A zero (the proto default for an
// absent field) must leave the default standing rather than cap the client at
// zero pixels, and a negative must not slip through as a cap either.
func TestClientProfileFromRequest_NonPositiveDimensionsAreNotACap(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int32
	}{
		{name: "absent", w: 0, h: 0},
		{name: "negative", w: -1, h: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := clientProfileFromRequest(&eveyv1.GetStreamInfoRequest{MaxWidth: tc.w, MaxHeight: tc.h}, "")

			assert.Equal(t, transcoding.DefaultWebProfile.MaxWidth, got.MaxWidth,
				"a non-positive width is 'not set', never a cap of %d", tc.w)
			assert.Equal(t, transcoding.DefaultWebProfile.MaxHeight, got.MaxHeight)
		})
	}
}

// videoKeyframesMissing gates the on-demand keyframe fill, which is what makes
// the copy rung reachable at all: answer "not missing" for an unindexed file
// and the file never gets its index, so it transcodes forever.
func TestVideoKeyframesMissing(t *testing.T) {
	video := func(kf []int64) *library.MediaStream {
		return &library.MediaStream{StreamType: library.StreamTypeVideo, Keyframes: kf}
	}
	audio := &library.MediaStream{StreamType: library.StreamTypeAudio}

	t.Run("an unindexed video stream is missing", func(t *testing.T) {
		assert.True(t, videoKeyframesMissing([]*library.MediaStream{video(nil)}))
		assert.True(t, videoKeyframesMissing([]*library.MediaStream{video([]int64{})}))
	})

	t.Run("an indexed video stream is not missing", func(t *testing.T) {
		assert.False(t, videoKeyframesMissing([]*library.MediaStream{video([]int64{0, 6000})}))
	})

	t.Run("only the FIRST video stream is consulted", func(t *testing.T) {
		// A second, unindexed video stream (cover art, a bonus angle) must not
		// re-trigger the probe for a file whose main stream is already indexed.
		assert.False(t, videoKeyframesMissing([]*library.MediaStream{
			video([]int64{0, 6000}),
			video(nil),
		}))
	})

	t.Run("audio streams ahead of the video one are skipped", func(t *testing.T) {
		assert.True(t, videoKeyframesMissing([]*library.MediaStream{audio, video(nil)}))
		assert.False(t, videoKeyframesMissing([]*library.MediaStream{audio, video([]int64{0})}))
	})

	t.Run("audio-only media never probes", func(t *testing.T) {
		assert.False(t, videoKeyframesMissing([]*library.MediaStream{audio}),
			"there is no video keyframe index to build for a music track")
	})

	t.Run("no streams at all never probes", func(t *testing.T) {
		assert.False(t, videoKeyframesMissing(nil))
		assert.False(t, videoKeyframesMissing([]*library.MediaStream{}))
	})

	t.Run("a nil entry is skipped, not dereferenced", func(t *testing.T) {
		assert.True(t, videoKeyframesMissing([]*library.MediaStream{nil, video(nil)}))
	})
}

// The builds whose ac3go played E-AC-3 as silence on a phone (mobile beta.68/69,
// TV 0.1.9/0.1.10) lose eac3 from their declaration, so the stream info
// transcodes what they would have played mute. Any other client keeps it.
func TestClientProfileFromRequest_DropsEAC3ForTheSilentBuilds(t *testing.T) {
	req := &eveyv1.GetStreamInfoRequest{SupportedAudioCodecs: []string{"aac", "ac3", "eac3"}}

	got := clientProfileFromRequest(req, "EveyMobile/0.0.1-beta.69 (Android 16; Phone)")
	assert.Equal(t, []string{"aac", "ac3"}, got.AudioCodecs)

	got = clientProfileFromRequest(req, "EveyMobile/0.0.1-beta.70 (Android 16; Phone)")
	assert.Equal(t, []string{"aac", "ac3", "eac3"}, got.AudioCodecs)

	// A declaration left empty by the correction falls back to the default.
	only := &eveyv1.GetStreamInfoRequest{SupportedAudioCodecs: []string{"eac3"}}
	got = clientProfileFromRequest(only, "EveyTV/0.1.10 (Android TV 14; Chromecast HD)")
	assert.Equal(t, transcoding.DefaultWebProfile.AudioCodecs, got.AudioCodecs)
}
