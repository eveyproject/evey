package grpcapi

import (
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/playback"
)

var playMethodToProto = map[string]eveyv1.PlayMethod{
	"direct_play":   eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	"direct_stream": eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM,
	"transcode":     eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE,
}

var playMethodFromProto = map[eveyv1.PlayMethod]string{
	eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY:   "direct_play",
	eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM: "direct_stream",
	eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE:     "transcode",
}

func watchProgressToProto(wp *playback.WatchProgress) *eveyv1.WatchProgress {
	return &eveyv1.WatchProgress{
		Id:           wp.ID,
		ProfileId:    wp.ProfileID,
		MediaItemId:  wp.MediaItemID,
		PositionMs:   wp.PositionMs,
		Played:       wp.Completed,
		PlayCount:    int32(wp.PlayCount),
		LastPlayedAt: toTimestamppb(wp.LastPlayedAt),
	}
}

func playSessionToProto(s *playback.PlaySession) *eveyv1.PlaySession {
	ps := &eveyv1.PlaySession{
		Id:          s.ID,
		ProfileId:   s.ProfileID,
		MediaItemId: s.MediaItemID,
		PlayMethod:  playMethodToProto[s.PlayMethod],
		DeviceName:  s.DeviceName,
		ClientName:  s.DeviceType,
		StartedAt:   toTimestamppb(s.StartedAt),
	}
	if s.PositionMs != nil {
		ps.PositionMs = *s.PositionMs
	}
	return ps
}
