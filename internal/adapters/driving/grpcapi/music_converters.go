package grpcapi

import (
	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/music"
)

func trackToProto(t music.Track) *eveyv1.Track {
	return &eveyv1.Track{
		Id:          t.ID,
		LibraryId:   string(t.LibraryID),
		Title:       t.Title,
		Artist:      t.Artist,
		Album:       t.Album,
		AlbumArtist: t.AlbumArtist,
		Year:        int32(t.Year),
		TrackNumber: int32(t.TrackNumber),
		DiskNumber:  int32(t.DiskNumber),
		Duration:    t.Duration,
		FilePath:    t.FilePath,
		FileSize:    t.FileSize,
		MimeType:    t.MimeType,
		Bitrate:     int32(t.Bitrate),
		SampleRate:  int32(t.SampleRate),
		Channels:    int32(t.Channels),
		Genre:       t.Genre,
		CoverArtUrl: t.CoverArtURL,
		CreatedAt:   toTimestamppb(t.CreatedAt),
		UpdatedAt:   toTimestamppb(t.UpdatedAt),
	}
}
