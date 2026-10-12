package grpcapi

import (
	"fmt"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

func metadataResultToProto(r domainmetadata.MetadataResult) *eveyv1.MetadataResult {
	pb := &eveyv1.MetadataResult{
		Title:         r.Title,
		OriginalTitle: r.OriginalTitle,
		Overview:      r.Overview,
		Genres:        r.Genres,
	}
	if r.Year != nil {
		pb.Year = int32(*r.Year)
	}
	if r.CommunityRating != nil {
		pb.Rating = *r.CommunityRating
	}
	return pb
}

func metadataOverrideToProto(o *domainmetadata.MetadataOverride) *eveyv1.MetadataOverride {
	cv := ""
	if o.CustomValue != nil {
		cv = fmt.Sprintf("%v", o.CustomValue)
	}
	return &eveyv1.MetadataOverride{
		Id:          o.ID,
		MediaItemId: string(o.MediaItemID),
		FieldName:   o.FieldName,
		CustomValue: cv,
		CreatedAt:   toTimestamppb(o.CreatedAt),
	}
}

var artworkTypeToProto = map[string]eveyv1.ArtworkType{
	"poster":   eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY,
	"backdrop": eveyv1.ArtworkType_ARTWORK_TYPE_BACKDROP,
	"banner":   eveyv1.ArtworkType_ARTWORK_TYPE_BANNER,
	"thumb":    eveyv1.ArtworkType_ARTWORK_TYPE_THUMB,
	"logo":     eveyv1.ArtworkType_ARTWORK_TYPE_LOGO,
	"disc":     eveyv1.ArtworkType_ARTWORK_TYPE_DISC,
}

func artworkRecordToProto(a *domainmetadata.ArtworkRecord) *eveyv1.Artwork {
	pb := &eveyv1.Artwork{
		Id:       a.ID,
		Provider: a.Provider,
		Type:     artworkTypeToProto[a.Type],
	}
	if a.SourceURL != nil {
		pb.Url = *a.SourceURL
	}
	if a.Width != nil {
		pb.Width = int32(*a.Width)
	}
	if a.Height != nil {
		pb.Height = int32(*a.Height)
	}
	if a.Language != nil {
		pb.Language = *a.Language
	}
	if a.Blurhash != nil {
		pb.Blurhash = *a.Blurhash
	}
	return pb
}
