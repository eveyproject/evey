package grpcapi

import (
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayMethodEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]eveyv1.PlayMethod{
		"direct_play":   eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		"direct_stream": eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM,
		"transcode":     eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE,
	}
	for domainV, protoV := range cases {
		assert.Equal(t, protoV, playMethodToProto[domainV])
		assert.Equal(t, domainV, playMethodFromProto[protoV])
		// round-trip
		assert.Equal(t, domainV, playMethodFromProto[playMethodToProto[domainV]])
	}
}

func TestWatchProgressToProto_Timestamps(t *testing.T) {
	t.Parallel()
	last := time.Date(2024, 5, 1, 8, 0, 0, 0, time.UTC)
	pos, err := shared.NewDurationMs(12345)
	if err != nil {
		t.Fatal(err)
	}
	wp := &playback.WatchProgress{
		ID:           "wp-ts",
		ProfileID:    "p-1",
		MediaItemID:  "m-1",
		Position:     pos,
		Completed:    false,
		PlayCount:    1,
		LastPlayedAt: last,
	}
	pb := watchProgressToProto(wp)
	assert.Equal(t, "p-1", pb.ProfileId)
	assert.Equal(t, "m-1", pb.MediaItemId)
	assert.Equal(t, int64(12345), pb.PositionMs)
	assert.False(t, pb.Played)
	assert.Equal(t, int32(1), pb.PlayCount)
	assert.NotNil(t, pb.LastPlayedAt)
	assert.Equal(t, last.Unix(), pb.LastPlayedAt.AsTime().Unix())
	// The phone's offline/server arbitration reads updated_at: never empty.
	require.NotNil(t, pb.UpdatedAt, "updated_at is read by the mobile resume arbitration")
	assert.Equal(t, last.Unix(), pb.UpdatedAt.AsTime().Unix())
}

func TestPlaySessionToProto_AllMethods(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"direct_play", "direct_stream", "transcode"} {
		s := &playback.PlaySession{
			ID:          "ps-" + method,
			ProfileID:   "p-1",
			MediaItemID: "m-1",
			PlayMethod:  method,
			DeviceName:  "dev",
			DeviceType:  "web",
			StartedAt:   time.Now(),
		}
		pb := playSessionToProto(s)
		assert.Equal(t, playMethodToProto[method], pb.PlayMethod, "method=%s", method)
		assert.Equal(t, "dev", pb.DeviceName)
		assert.Equal(t, "web", pb.ClientName)
	}
}

func TestPlaySessionToProto_UnknownMethodYieldsUnspecified(t *testing.T) {
	t.Parallel()
	s := &playback.PlaySession{ID: "ps-u", PlayMethod: "bogus"}
	pb := playSessionToProto(s)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_UNSPECIFIED, pb.PlayMethod)
}
