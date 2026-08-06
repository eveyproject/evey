package grpcapi

import (
	"context"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// library_handler_public.go holds the unauthenticated public media-item RPCs
// used by share links (minimal fields, poster only).

func posterURLForRecords(recs []*domainmetadata.ArtworkRecord) string {
	for _, a := range recs {
		if a != nil && a.Type == "poster" {
			return artworkRecordToProto(a).Url
		}
	}
	return ""
}

func publicMediaItemToProto(item *library.MediaItem, posterURL string) *eveyv1.PublicMediaItem {
	pb := &eveyv1.PublicMediaItem{
		Id:        string(item.ID),
		Type:      mediaItemTypeToProto[string(item.Type)],
		Title:     item.Title,
		Overview:  item.Overview,
		PosterUrl: posterURL,
	}
	if item.Catalog != nil && item.Catalog.Year != nil {
		pb.Year = int32(*item.Catalog.Year)
	}
	return pb
}

func (h *libraryHandler) GetPublicMediaItem(ctx context.Context, req *eveyv1.GetPublicMediaItemRequest) (*eveyv1.PublicMediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetPublicMediaItem")
	defer span.End()
	item, err := h.query.GetMediaItem(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "media item not found")
	}
	if ok, _ := h.checkLibraryAccess(ctx, item.LibraryID); !ok { //nolint:errcheck // fail-closed: any error denies
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if h.isBlockedByParental(ctx, item) {
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	poster := ""
	if h.metadataSvc != nil {
		if recs, aErr := h.metadataSvc.GetArtwork(ctx, req.Id); aErr == nil {
			poster = posterURLForRecords(recs)
		}
	}
	return publicMediaItemToProto(item, poster), nil
}

func (h *libraryHandler) ListPublicMediaItems(ctx context.Context, req *eveyv1.ListPublicMediaItemsRequest) (*eveyv1.ListPublicMediaItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListPublicMediaItems")
	defer span.End()
	filter := library.ListMediaItemsFilter{}
	if req.LibraryId != "" {
		libID := shared.LibraryID(req.LibraryId)
		filter.LibraryID = &libID
		if ok, _ := h.checkLibraryAccess(ctx, libID); !ok { //nolint:errcheck // fail-closed: any error denies
			return &eveyv1.ListPublicMediaItemsResponse{}, nil
		}
	} else if ids, restricted, ok := h.allowedLibraryFilter(ctx); !ok {
		return &eveyv1.ListPublicMediaItemsResponse{}, nil
	} else if restricted {
		filter.LibraryIDs = toLibraryIDs(ids)
	}
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		t := mediaItemTypeFromProto[req.Type]
		filter.Type = &t
	}
	if req.Pagination != nil {
		filter.Limit, filter.Offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
	}
	filter.AllowedRatings = h.resolveParentalFilter(ctx).allowedRatings

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError(ctx, "count public media items", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError(ctx, "list public media items", err)
	}

	resp := &eveyv1.ListPublicMediaItemsResponse{Total: int32(total)}
	if len(items) == 0 {
		return resp, nil
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, string(it.ID))
	}
	var artworkMap map[string][]*domainmetadata.ArtworkRecord
	if h.metadataSvc != nil {
		artworkMap, _ = h.metadataSvc.BatchGetArtwork(ctx, ids) //nolint:errcheck // artwork is optional enrichment
	}
	for _, it := range items {
		resp.Items = append(resp.Items, publicMediaItemToProto(it, posterURLForRecords(artworkMap[string(it.ID)])))
	}
	return resp, nil
}
