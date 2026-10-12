package grpcapi

import (
	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/music"
)

// trackToProto converts a Track to proto. When includeFilePath is false,
// `file_path` is omitted - non-admin callers have no business seeing the
// server's absolute on-disk paths.
func trackToProto(t music.Track, includeFilePath bool) *eveyv1.Track {
	pb := &eveyv1.Track{
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
	if t.AlbumFolder != "" {
		pb.AlbumId = music.AlbumID(t.AlbumFolder)
	}
	if includeFilePath {
		pb.FilePath = t.FilePath
	}
	return pb
}
