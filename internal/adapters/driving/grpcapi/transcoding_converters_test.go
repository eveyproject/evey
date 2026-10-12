package grpcapi

import (
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHWAccelTypeEnumRoundTrip(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"none", "vaapi", "nvenc", "qsv", "rkmpp"} {
		pt := hwAccelTypeToProto[name]
		assert.Equal(t, name, hwAccelTypeFromProto[pt], "round-trip %q", name)
	}
}

func TestTranscodeStateEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]eveyv1.TranscodeState{
		"queued":    eveyv1.TranscodeState_TRANSCODE_STATE_QUEUED,
		"running":   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
		"completed": eveyv1.TranscodeState_TRANSCODE_STATE_COMPLETED,
		"failed":    eveyv1.TranscodeState_TRANSCODE_STATE_FAILED,
		"cancelled": eveyv1.TranscodeState_TRANSCODE_STATE_CANCELLED,
	}
	for k, v := range cases {
		assert.Equal(t, v, transcodeStateToProto[k])
	}
}

func TestTranscodeJobToProto_FullFieldsWithEnd(t *testing.T) {
	t.Parallel()
	end := time.Date(2024, 3, 2, 5, 0, 0, 0, time.UTC)
	j := &transcoding.TranscodeJob{
		ID:           "job-1",
		MediaItemID:  shared.MediaItemID("m-1"),
		OutputCodec:  "h264",
		AudioCodec:   "aac",
		HWAccelType:  "nvenc",
		BitrateOut:   5000000,
		Speed:        1.25,
		Status:       transcoding.Running,
		ErrorMessage: "",
		StartedAt:    time.Date(2024, 3, 2, 4, 0, 0, 0, time.UTC),
		EndedAt:      &end,
	}
	pb := transcodeJobToProto(j)
	assert.Equal(t, "job-1", pb.Id)
	assert.Equal(t, "m-1", pb.MediaItemId)
	assert.Equal(t, eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING, pb.State)
	assert.Equal(t, "h264", pb.VideoCodec)
	assert.Equal(t, "aac", pb.AudioCodec)
	assert.Equal(t, eveyv1.HWAccelType_HW_ACCEL_TYPE_NVENC, pb.HwAccel)
	assert.Equal(t, int64(5000000), pb.Bitrate)
	assert.InDelta(t, 1.25, pb.Speed, 0.0001)
	require.NotNil(t, pb.CompletedAt)
	assert.Equal(t, end.Unix(), pb.CompletedAt.AsTime().Unix())
}
