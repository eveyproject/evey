package grpcapi

import (
	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/playback"
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
		ProfileId:    string(wp.ProfileID),
		MediaItemId:  string(wp.MediaItemID),
		PositionMs:   wp.Position.Milliseconds(),
		Played:       wp.Completed,
		PlayCount:    int32(wp.PlayCount),
		LastPlayedAt: toTimestamppb(wp.LastPlayedAt),
		// The phone compares its offline copy against this to decide which
		// position wins; left empty, the local copy always won, resumed at a
		// stale position and pushed it back to the server (audit 2026-10-01).
		// The row's last write IS its last play: every report bumps it.
		UpdatedAt: toTimestamppb(wp.LastPlayedAt),
	}
}

func playSessionToProto(s *playback.PlaySession) *eveyv1.PlaySession {
	ps := &eveyv1.PlaySession{
		Id:          s.ID,
		ProfileId:   string(s.ProfileID),
		MediaItemId: string(s.MediaItemID),
		PlayMethod:  playMethodToProto[s.PlayMethod],
		DeviceName:  s.DeviceName,
		ClientName:  s.DeviceType,
		StartedAt:   toTimestamppb(s.StartedAt),
	}
	if s.Position != nil {
		ps.PositionMs = s.Position.Milliseconds()
	}
	return ps
}
