package grpcapi

import (
	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/photo"
)

func photoToProto(p photo.Photo) *eveyv1.Photo {
	return &eveyv1.Photo{
		Id:          p.ID,
		LibraryId:   string(p.LibraryID),
		Title:       p.Title,
		FilePath:    p.FilePath,
		FileSize:    p.FileSize,
		MimeType:    p.MimeType,
		Width:       int32(p.Width),
		Height:      int32(p.Height),
		Album:       p.Album,
		TakenAt:     toTimestamppbPtr(p.TakenAt),
		CameraModel: p.CameraModel,
		CreatedAt:   toTimestamppb(p.CreatedAt),
		UpdatedAt:   toTimestamppb(p.UpdatedAt),
	}
}
