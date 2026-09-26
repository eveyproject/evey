package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingDecider is the bound decider: it keeps the request it was handed
// and answers a decision no core would (a transcode on an all-copy source).
type recordingDecider struct {
	got      apptranscoding.PlaybackRequest
	calls    int
	decision transcoding.PlaybackDecision
}

func (d *recordingDecider) Decide(_ context.Context, req apptranscoding.PlaybackRequest) transcoding.PlaybackDecision {
	d.got = req
	d.calls++
	return d.decision
}

// GetStreamInfo answers with the bound decider's decision (ADR-0030: the
// plan decides, the info surface projects) and hands it the same request the
// HLS master will be planned from: the client's codecs, the line cap, the
// selected audio track, the source facts.
func TestPlaybackHandler_GetStreamInfo_DecidesThroughTheBoundDecider(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	dec := &recordingDecider{decision: transcoding.PlaybackDecision{
		PlayMethod: "transcode", VideoDecision: "transcode", AudioDecision: "copy",
		VideoCodec: "h264", MaxHeight: 720, TranscodeReasons: []string{"bound_decider"},
	}}
	h.decider = dec
	mediaQuery.Seed(&library.MediaItem{
		ID: "mi-1", Title: "Test Movie", Type: "movie",
		File: &library.MediaFile{Path: "/srv/a.mp4", ContainerFormat: "mp4", Size: 1 << 30, Bitrate: 8_000_000},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-1", StreamIndex: 0, StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{MediaItemID: "mi-1", StreamIndex: 1, StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{MediaItemID: "mi-1", StreamIndex: 2, StreamType: "audio", Codec: "aac", Channels: new(6)},
	})

	resp, err := h.GetStreamInfo(pbAdminCtx(), &eveyv1.GetStreamInfoRequest{
		MediaItemId:      "mi-1",
		SupportedCodecs:  []string{"hevc"},
		MaxBitrate:       4_000_000,
		AudioStreamIndex: 2,
		WasmAudio:        true,
	})
	require.NoError(t, err)

	// The response is the decider's decision, not the core's (which would
	// say direct_play on an h264/aac mp4 for a client that lists h264).
	require.Equal(t, 1, dec.calls)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, resp.PlayMethod)
	assert.Equal(t, "transcode", resp.VideoDecision)
	assert.Equal(t, "copy", resp.AudioDecision)
	assert.Equal(t, "h264", resp.VideoCodec)
	assert.Equal(t, "720p", resp.Resolution)
	assert.Equal(t, []string{"bound_decider"}, resp.TranscodeReasons)

	// The request carries what the master's plan will read.
	got := dec.got
	assert.Equal(t, "mi-1", got.MediaID)
	assert.Equal(t, "/srv/a.mp4", got.SourcePath)
	assert.Equal(t, "mp4", got.Container)
	assert.Len(t, got.Streams, 3)
	assert.Equal(t, []string{"hevc"}, got.Client.VideoCodecs)
	assert.Equal(t, 4000, got.MaxBitrateKbps)
	assert.Equal(t, 1, got.AudioTrack, "the second audio stream, audio-relative")
	assert.Equal(t, 1, got.Prefs.AudioTrack)
	assert.True(t, got.WasmAudio)
	assert.Equal(t, int64(1<<30), got.SourceBytes)
	assert.Equal(t, int64(8000), got.SourceBitrateKbps)
	assert.False(t, got.SourceNeedsRemux)
}

// Unbound, the handler decides on the unwired resolver rather than failing
// or answering nothing: the composition root binds the pipeline's decider
// (TestMainBindsThePlaybackDeciderBeforeServing), a test handler has none.
func TestPlaybackHandler_GetStreamInfo_UnboundDeciderStillDecides(t *testing.T) {
	h, _, _, mediaQuery, _, _ := newTestPlaybackHandler()
	require.Nil(t, h.decider)
	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", Type: "movie", File: &library.MediaFile{ContainerFormat: "mp4"}})

	resp, err := h.GetStreamInfo(pbAdminCtx(), &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-1"})
	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
}
