package grpcapi

import (
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMkvTrackTypeEnumMapping(t *testing.T) {
	t.Parallel()
	assert.Equal(t, eveyv1.TrackType_TRACK_TYPE_VIDEO, mkvTrackTypeToProto[mkv.VideoTrack])
	assert.Equal(t, eveyv1.TrackType_TRACK_TYPE_AUDIO, mkvTrackTypeToProto[mkv.AudioTrack])
	assert.Equal(t, eveyv1.TrackType_TRACK_TYPE_SUBTITLE, mkvTrackTypeToProto[mkv.SubtitleTrack])
}

func TestMkvTrackToProto_FullFields(t *testing.T) {
	t.Parallel()
	var w, h uint32 = 1920, 1080
	var ch uint8 = 6
	var sr float64 = 48000
	var bd uint8 = 24
	tr := mkv.Track{
		ID:         1,
		Type:       mkv.VideoTrack,
		Codec:      "V_MPEGH/ISO/HEVC",
		Language:   "eng",
		Name:       "Main",
		IsDefault:  true,
		IsForced:   false,
		Width:      &w,
		Height:     &h,
		Channels:   &ch,
		SampleRate: &sr,
		BitDepth:   &bd,
	}
	pb := mkvTrackToProto(tr)
	assert.Equal(t, int32(1), pb.Id)
	assert.Equal(t, eveyv1.TrackType_TRACK_TYPE_VIDEO, pb.Type)
	assert.Equal(t, "V_MPEGH/ISO/HEVC", pb.Codec)
	assert.Equal(t, "eng", pb.Language)
	assert.Equal(t, "Main", pb.Name)
	assert.True(t, pb.IsDefault)
	assert.True(t, pb.IsEnabled)
	assert.Equal(t, int32(1920), pb.Width)
	assert.Equal(t, int32(1080), pb.Height)
	assert.Equal(t, int32(6), pb.Channels)
	assert.Equal(t, int32(48000), pb.SampleRate)
	assert.Equal(t, int32(24), pb.BitDepth)
}

func TestMkvTrackToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	tr := mkv.Track{ID: 2, Type: mkv.AudioTrack, Codec: "A_AAC"}
	pb := mkvTrackToProto(tr)
	assert.Equal(t, int32(0), pb.Width)
	assert.Equal(t, int32(0), pb.Height)
	assert.Equal(t, int32(0), pb.Channels)
	assert.Equal(t, int32(0), pb.SampleRate)
	assert.Equal(t, int32(0), pb.BitDepth)
}

func TestMkvContainerToProto_NestedLists(t *testing.T) {
	t.Parallel()
	dur, _ := shared.NewDurationMs(3600000)
	c := &mkv.Container{
		Path:     "/m.mkv",
		Duration: dur,
		Info: mkv.SegmentInfo{
			MuxingApp:  "mkvmerge",
			WritingApp: "handbrake",
			Title:      "T",
			Duration:   3600,
		},
		Tracks: []mkv.Track{
			{ID: 1, Type: mkv.VideoTrack, Codec: "V_VP9"},
			{ID: 2, Type: mkv.AudioTrack, Codec: "A_OPUS"},
		},
		Chapters: []mkv.Chapter{
			{ID: 10, Title: "Intro", StartMs: 0, EndMs: 60000},
		},
		Attachments: []mkv.Attachment{
			{ID: 100, Name: "cover.jpg", MIMEType: "image/jpeg", Size: 1024},
		},
	}
	pb := mkvContainerToProto(c)
	assert.Equal(t, "/m.mkv", pb.Path)
	assert.Equal(t, int64(3600000), pb.DurationMs)
	require.NotNil(t, pb.Info)
	assert.Equal(t, "mkvmerge", pb.Info.MuxingApp)
	assert.Equal(t, "handbrake", pb.Info.WritingApp)
	assert.Equal(t, "T", pb.Info.Title)
	assert.Equal(t, int64(3600), pb.Info.DurationMs)
	require.Len(t, pb.Tracks, 2)
	require.Len(t, pb.Chapters, 1)
	assert.Equal(t, "10", pb.Chapters[0].Id)
	assert.Equal(t, "Intro", pb.Chapters[0].Title)
	require.Len(t, pb.Attachments, 1)
	assert.Equal(t, int32(100), pb.Attachments[0].Id)
	assert.Equal(t, "cover.jpg", pb.Attachments[0].Name)
	assert.Equal(t, "image/jpeg", pb.Attachments[0].MimeType)
	assert.Equal(t, int64(1024), pb.Attachments[0].Size)
}

func TestMkvChapterRoundTrip(t *testing.T) {
	t.Parallel()
	ch := mkv.Chapter{ID: 7, Title: "Part 1", StartMs: 1000, EndMs: 5000}
	pb := mkvChapterToProto(ch)
	assert.Equal(t, "7", pb.Id)
	back := mkvChapterFromProto(pb)
	// From-proto doesn't round-trip ID (parses title/timings)
	assert.Equal(t, ch.Title, back.Title)
	assert.Equal(t, ch.StartMs, back.StartMs)
	assert.Equal(t, ch.EndMs, back.EndMs)
}
