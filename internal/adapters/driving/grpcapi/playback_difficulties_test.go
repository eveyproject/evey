package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// seedDifficultyItem wires a playable item with one video and one audio track.
func seedDifficultyItem(t *testing.T, mediaQuery interface {
	Seed(*library.MediaItem)
}, streamRepo interface {
	CreateBatch(context.Context, []*library.MediaStream) error
}, id, videoCodec, audioCodec string, height int) {
	t.Helper()
	mediaQuery.Seed(&library.MediaItem{
		ID:    shared.MediaItemID(id),
		Title: id,
		Type:  "episode",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})
	err := streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: shared.MediaItemID(id), StreamType: "video", Codec: videoCodec, Width: new(height * 16 / 9), Height: new(height), IsDefault: true},
		{MediaItemID: shared.MediaItemID(id), StreamType: "audio", Codec: audioCodec, Channels: new(6), IsDefault: true},
	})
	require.NoError(t, err)
}

func TestGetPlaybackDifficulties_ReportsPerItemDecision(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	seedDifficultyItem(t, mediaQuery, streamRepo, "mi-ok", "h264", "aac", 1080)
	seedDifficultyItem(t, mediaQuery, streamRepo, "mi-hevc", "hevc", "aac", 2160)

	resp, err := h.GetPlaybackDifficulties(ctx, &eveyv1.GetPlaybackDifficultiesRequest{
		MediaItemIds:         []string{"mi-ok", "mi-hevc"},
		SupportedCodecs:      []string{"h264"},
		SupportedAudioCodecs: []string{"aac"},
		SupportedContainers:  []string{"mp4"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)

	byID := map[string]*eveyv1.PlaybackDifficulty{}
	for _, d := range resp.Items {
		byID[d.MediaItemId] = d
	}

	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, byID["mi-ok"].PlayMethod)
	assert.Equal(t, "copy", byID["mi-ok"].VideoDecision)
	assert.Equal(t, int32(1080), byID["mi-ok"].VideoHeight)

	// HEVC is not in the declared codec list, so the video must be re-encoded.
	assert.Equal(t, "transcode", byID["mi-hevc"].VideoDecision)
	assert.Contains(t, byID["mi-hevc"].TranscodeReasons, "video_codec_unsupported")
	// Height and audio codec travel with the answer because a list response
	// carries no streams - the client cannot grade the cost without them.
	assert.Equal(t, int32(2160), byID["mi-hevc"].VideoHeight)
	assert.Equal(t, "aac", byID["mi-hevc"].AudioCodec)
}

// The bulk endpoint is a brand-new sibling of GetStreamInfo, which is exactly
// how every parental escape in this project happened: a second route to the
// same data that forgot the gate. A blocked item must be ABSENT, not errored -
// an error per id would still confirm the item exists.
func TestGetPlaybackDifficulties_ParentalBlockedItemIsAbsent(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, streamRepo := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			MaxAge:  new(10),
			Enabled: true,
		},
	})

	seedDifficultyItem(t, mediaQuery, streamRepo, "mi-kid", "h264", "aac", 1080)
	mediaQuery.Seed(&library.MediaItem{
		ID:             "mi-adult",
		Title:          "R-Rated",
		Type:           "movie",
		File:           &library.MediaFile{ContainerFormat: "mp4"},
		Classification: &library.MediaClassification{OfficialRating: "R"},
	})

	// Non-admin on purpose: an admin is exempt from the parental gate, so an
	// admin ctx would assert the opposite of the rule under test.
	ctx := pbUserCtx("user-1", "profile-1")
	resp, err := h.GetPlaybackDifficulties(ctx, &eveyv1.GetPlaybackDifficultiesRequest{
		MediaItemIds:    []string{"mi-kid", "mi-adult"},
		SupportedCodecs: []string{"h264"},
	})

	require.NoError(t, err)
	for _, d := range resp.Items {
		assert.NotEqual(t, "mi-adult", d.MediaItemId, "a parental-blocked item leaked into the response")
	}
}

func TestGetPlaybackDifficulties_RequiresAuthentication(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()

	_, err := h.GetPlaybackDifficulties(context.Background(), &eveyv1.GetPlaybackDifficultiesRequest{
		MediaItemIds: []string{"mi-1"},
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// A container (series/season) has no file of its own, so there is nothing to
// grade. It must be skipped rather than reported with a meaningless verdict.
func TestGetPlaybackDifficulties_SkipsItemsWithoutAFile(t *testing.T) {
	h, _, _, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-series", Title: "A Series", Type: "series"})

	resp, err := h.GetPlaybackDifficulties(ctx, &eveyv1.GetPlaybackDifficultiesRequest{
		MediaItemIds: []string{"mi-series", "mi-missing"},
	})

	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestGetPlaybackDifficulties_CapsTheBatch(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	ids := make([]string, 0, maxDifficultyBatch+10)
	for i := range maxDifficultyBatch + 10 {
		id := "mi-batch-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		seedDifficultyItem(t, mediaQuery, streamRepo, id, "h264", "aac", 720)
		ids = append(ids, id)
	}

	resp, err := h.GetPlaybackDifficulties(ctx, &eveyv1.GetPlaybackDifficultiesRequest{
		MediaItemIds:         ids,
		SupportedCodecs:      []string{"h264"},
		SupportedAudioCodecs: []string{"aac"},
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(resp.Items), maxDifficultyBatch)
}

func TestSelectedAudioCodecOf_PrefersTheTrackThatWouldActuallyPlay(t *testing.T) {
	// A commentary track flagged default must not be the one we describe.
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", IsDefault: true, Commentary: true},
		{StreamType: "audio", Codec: "eac3"},
	}
	assert.Equal(t, "eac3", selectedAudioCodecOf(streams))

	// Default non-commentary wins over a plain first track.
	streams = []*library.MediaStream{
		{StreamType: "audio", Codec: "mp3"},
		{StreamType: "audio", Codec: "ac3", IsDefault: true},
	}
	assert.Equal(t, "ac3", selectedAudioCodecOf(streams))

	assert.Empty(t, selectedAudioCodecOf([]*library.MediaStream{{StreamType: "video", Codec: "h264"}}))
}

func TestVideoHeightOf(t *testing.T) {
	assert.Equal(t, 1080, videoHeightOf([]*library.MediaStream{
		{StreamType: "audio", Codec: "aac"},
		{StreamType: "video", Codec: "h264", Height: new(1080)},
	}))
	assert.Equal(t, 0, videoHeightOf([]*library.MediaStream{{StreamType: "audio", Codec: "aac"}}))
}

// The whole point of building the bulk endpoint on the SAME helpers as
// GetStreamInfo is that a badge can never contradict what happens when the
// title is actually played. This pins that: for identical inputs, both paths
// must reach the same verdict.
//
// If someone later "optimises" GetPlaybackDifficulties by inlining its own
// client profile or preference resolution, this is what goes red.
func TestGetPlaybackDifficulties_AgreesWithGetStreamInfo(t *testing.T) {
	cases := []struct {
		name                           string
		videoCodec, audioCodec         string
		height                         int
		supportedVideo, supportedAudio []string
	}{
		{"direct play", "h264", "aac", 1080, []string{"h264"}, []string{"aac"}},
		{"video unsupported", "hevc", "aac", 2160, []string{"h264"}, []string{"aac"}},
		{"audio unsupported", "h264", "eac3", 1080, []string{"h264"}, []string{"aac"}},
		{"both unsupported", "mpeg4", "mp3", 404, []string{"h264"}, []string{"aac"}},
		// Deliberately MORE capable than DefaultWebProfile (h264/vp9 + aac/mp3/
		// opus/flac). Without a case like this the assertion is vacuous: dropping
		// the client profile entirely still yields the same verdicts, because the
		// default happens to agree on every codec above. Proven by sabotage.
		{"client beyond the default profile", "hevc", "eac3", 2160,
			[]string{"h264", "hevc", "av1"}, []string{"aac", "eac3", "truehd"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
			ctx := pbAdminCtx()
			seedDifficultyItem(t, mediaQuery, streamRepo, "mi-agree", tc.videoCodec, tc.audioCodec, tc.height)

			single, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
				MediaItemId:          "mi-agree",
				SupportedCodecs:      tc.supportedVideo,
				SupportedAudioCodecs: tc.supportedAudio,
				SupportedContainers:  []string{"mp4"},
			})
			require.NoError(t, err)

			bulk, err := h.GetPlaybackDifficulties(ctx, &eveyv1.GetPlaybackDifficultiesRequest{
				MediaItemIds:         []string{"mi-agree"},
				SupportedCodecs:      tc.supportedVideo,
				SupportedAudioCodecs: tc.supportedAudio,
				SupportedContainers:  []string{"mp4"},
			})
			require.NoError(t, err)
			require.Len(t, bulk.Items, 1)

			assert.Equal(t, single.PlayMethod, bulk.Items[0].PlayMethod, "play method diverged")
			assert.Equal(t, single.VideoDecision, bulk.Items[0].VideoDecision, "video decision diverged")
			assert.Equal(t, single.AudioDecision, bulk.Items[0].AudioDecision, "audio decision diverged")
			assert.ElementsMatch(t, single.TranscodeReasons, bulk.Items[0].TranscodeReasons, "reasons diverged")
		})
	}
}
