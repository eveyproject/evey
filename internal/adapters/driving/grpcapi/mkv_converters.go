package grpcapi

import (
	"fmt"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/mkv"
)

var mkvTrackTypeToProto = map[mkv.TrackType]eveyv1.TrackType{
	mkv.VideoTrack:    eveyv1.TrackType_TRACK_TYPE_VIDEO,
	mkv.AudioTrack:    eveyv1.TrackType_TRACK_TYPE_AUDIO,
	mkv.SubtitleTrack: eveyv1.TrackType_TRACK_TYPE_SUBTITLE,
}

func mkvContainerToProto(c *mkv.Container) *eveyv1.Container {
	pb := &eveyv1.Container{
		Path:       c.Path,
		DurationMs: c.Duration.Milliseconds(),
		Info: &eveyv1.ContainerInfo{
			MuxingApp:  c.Info.MuxingApp,
			WritingApp: c.Info.WritingApp,
			Title:      c.Info.Title,
			DurationMs: int64(c.Info.Duration),
		},
	}
	for _, t := range c.Tracks {
		pb.Tracks = append(pb.Tracks, mkvTrackToProto(t))
	}
	for _, ch := range c.Chapters {
		pb.Chapters = append(pb.Chapters, mkvChapterToProto(ch))
	}
	for _, a := range c.Attachments {
		pb.Attachments = append(pb.Attachments, mkvAttachmentToProto(a))
	}
	return pb
}

func mkvTrackToProto(t mkv.Track) *eveyv1.MkvTrack {
	pb := &eveyv1.MkvTrack{
		Id:        int32(t.ID),
		Type:      mkvTrackTypeToProto[t.Type],
		Codec:     t.Codec,
		Language:  t.Language,
		Name:      t.Name,
		IsDefault: t.IsDefault,
		IsForced:  t.IsForced,
		IsEnabled: true,
	}
	if t.Width != nil {
		pb.Width = int32(*t.Width)
	}
	if t.Height != nil {
		pb.Height = int32(*t.Height)
	}
	if t.Channels != nil {
		pb.Channels = int32(*t.Channels)
	}
	if t.SampleRate != nil {
		pb.SampleRate = int32(*t.SampleRate)
	}
	if t.BitDepth != nil {
		pb.BitDepth = int32(*t.BitDepth)
	}
	return pb
}

func mkvChapterToProto(ch mkv.Chapter) *eveyv1.Chapter {
	return &eveyv1.Chapter{
		Id:      fmt.Sprintf("%d", ch.ID),
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}

func mkvAttachmentToProto(a mkv.Attachment) *eveyv1.Attachment {
	return &eveyv1.Attachment{
		Id:       int32(a.ID),
		Name:     a.Name,
		MimeType: a.MIMEType,
		Size:     a.Size,
	}
}

func mkvChapterFromProto(ch *eveyv1.Chapter) mkv.Chapter {
	return mkv.Chapter{
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}
