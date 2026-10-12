package grpcapi

import (
	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/photo"
)

// photoToProto converts a Photo to proto. When includeFilePath is false the
// absolute server path is omitted - it's admin-only (matches mediaItemToProto)
// and otherwise leaks the host filesystem layout to non-admin profiles (pentest
// 2026-05-26).
func photoToProto(p photo.Photo, includeFilePath bool) *eveyv1.Photo {
	out := &eveyv1.Photo{
		Id:          p.ID,
		LibraryId:   string(p.LibraryID),
		Title:       p.Title,
		FileSize:    p.FileSize,
		MimeType:    p.MimeType,
		Width:       int32(p.Width),
		Height:      int32(p.Height),
		Album:       p.Album,
		TakenAt:     toTimestamppbPtr(p.TakenAt),
		CameraModel: p.CameraModel,
		Blurhash:    p.Blurhash,
		CreatedAt:   toTimestamppb(p.CreatedAt),
		UpdatedAt:   toTimestamppb(p.UpdatedAt),
	}
	if includeFilePath {
		out.FilePath = p.FilePath
	}
	return out
}
