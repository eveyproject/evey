package grpcapi

import (
	"context"
	"strconv"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/library"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
)

// library_handler_public.go holds the public media-item RPCs: the minimal
// projection share links and plugin interfaces read.

func posterURLForRecords(recs []*domainmetadata.ArtworkRecord) string {
	for _, a := range recs {
		if a != nil && a.Type == "poster" {
			return artworkRecordToProto(a).Url
		}
	}
	return ""
}

// withFiles says whether the caller may see where each item lives on disk: a
// plugin token carrying media.files, which the issuer mints only for an
// administrator. A user session never gets the paths through this projection.
func withFiles(ctx context.Context) bool {
	pc := interceptors.PluginClaimsFromContext(ctx)
	return pc != nil && pc.HasScope(auth.MediaFilesScope)
}

// publicQuality describes a file from its streams: the first video track and
// the default (else first) audio track. Nil when nothing is known.
func publicQuality(f *library.MediaFile, streams []*library.MediaStream) *eveyv1.PublicMediaQuality {
	if f == nil {
		return nil
	}
	q := &eveyv1.PublicMediaQuality{
		Bitrate:    f.BitrateBps(),
		DurationMs: f.Duration.Milliseconds(),
		Container:  f.ContainerFormat,
	}
	var video, audio *library.MediaStream
	for _, s := range streams {
		switch s.StreamType {
		case library.StreamTypeVideo:
			if video == nil {
				video = s
			}
		case library.StreamTypeAudio:
			if audio == nil || (s.IsDefault && !audio.IsDefault) {
				audio = s
			}
		}
	}
	if video != nil {
		q.VideoCodec = video.Codec
		if video.Width != nil {
			q.Width = int32(*video.Width)
		}
		if video.Height != nil {
			q.Height = int32(*video.Height)
		}
		if video.HDRType != nil {
			q.HdrType = *video.HDRType
		}
		if video.BitDepth != nil {
			q.BitDepth = int32(*video.BitDepth)
		}
	}
	if audio != nil {
		q.AudioCodec = audio.Codec
		if audio.Channels != nil {
			q.AudioChannels = int32(*audio.Channels)
		}
	}
	return q
}

func publicMediaItemToProto(item *library.MediaItem, posterURL string, files bool) *eveyv1.PublicMediaItem {
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
	if item.Catalog != nil {
		if id, err := strconv.Atoi(item.Catalog.ProviderIDs["tmdb"]); err == nil {
			pb.TmdbId = int32(id)
		}
		pb.ImdbId = item.Catalog.ProviderIDs["imdb"]
	}
	if files && item.File != nil {
		pb.FilePath = item.File.Path
		pb.FileSize = item.File.Size
	}
	return pb
}

func (h *libraryHandler) GetPublicMediaItem(ctx context.Context, req *eveyv1.GetPublicMediaItemRequest) (*eveyv1.PublicMediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetPublicMediaItem")
	defer span.End()
	item, err := h.query.GetMediaItem(ctx, req.Id)
	if err != nil {
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
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
	pb := publicMediaItemToProto(item, poster, withFiles(ctx))
	if pb.FilePath != "" {
		if streams, sErr := h.query.GetStreamsForItems(ctx, []string{req.Id}); sErr == nil {
			pb.Quality = publicQuality(item.File, streams[req.Id])
		}
	}
	return pb, nil
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
	files := withFiles(ctx)
	var streams map[string][]*library.MediaStream
	if files {
		// Quality comes with the paths, for the same callers only: one batch
		// query for the page, never a query per item.
		streams, _ = h.query.GetStreamsForItems(ctx, ids) //nolint:errcheck // quality is optional enrichment
	}
	for _, it := range items {
		pb := publicMediaItemToProto(it, posterURLForRecords(artworkMap[string(it.ID)]), files)
		if pb.FilePath != "" {
			pb.Quality = publicQuality(it.File, streams[string(it.ID)])
		}
		resp.Items = append(resp.Items, pb)
	}
	return resp, nil
}
