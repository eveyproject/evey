package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type metadataHandler struct {
	eveyv1.UnimplementedMetadataServiceServer
	cmd        *appmetadata.CommandService
	query      *appmetadata.QueryService
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
}

func (h *metadataHandler) checkMediaItemAccess(ctx context.Context, mediaItemID string) error {
	if h.userAccess == nil || h.libQuery == nil {
		return nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, item.LibraryID)
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *metadataHandler) SearchMetadata(ctx context.Context, req *eveyv1.SearchMetadataRequest) (*eveyv1.SearchMetadataResponse, error) {
	mediaType := mediaItemTypeFromProto[req.Type]
	results, err := h.query.SearchMetadata(ctx, req.Query, mediaType)
	if err != nil {
		return nil, internalError("search metadata", err)
	}
	resp := &eveyv1.SearchMetadataResponse{}
	limit := int(req.Limit)
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	for _, r := range results[:limit] {
		resp.Results = append(resp.Results, metadataResultToProto(r))
	}
	return resp, nil
}

func (h *metadataHandler) RefreshMetadata(req *eveyv1.RefreshMetadataRequest, stream eveyv1.MetadataService_RefreshMetadataServer) error {
	ctx := stream.Context()
	if req.PreferredProvider != "" {
		_, err := h.cmd.RefreshMetadata(ctx, req.MediaItemId, "", req.PreferredProvider, "")
		if err != nil {
			return internalError("refresh metadata", err)
		}
	} else {
		// Auto-detect: look up TMDB ID from the item and do a full re-enrich
		item, err := h.cmd.GetMediaItem(ctx, req.MediaItemId)
		if err != nil {
			return internalError("get item", err)
		}
		tmdbID := item.ProviderIDs["tmdb"]
		mediaType := "movie"
		if item.Type == "series" || item.Type == "season" || item.Type == "episode" {
			mediaType = "tv"
		}
		if tmdbID != "" {
			if err := h.cmd.RefreshItemMetadata(ctx, req.MediaItemId, tmdbID, mediaType); err != nil {
				return internalError("refresh metadata", err)
			}
		} else {
			return status.Errorf(codes.FailedPrecondition, "no TMDB ID for this item")
		}
	}
	return stream.Send(&eveyv1.RefreshMetadataProgress{
		MediaItemId: req.MediaItemId,
		Phase:       "complete",
		Percent:     100,
	})
}

func (h *metadataHandler) GetArtwork(ctx context.Context, req *eveyv1.GetArtworkRequest) (*eveyv1.GetArtworkResponse, error) {
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	records, err := h.query.GetArtwork(ctx, req.MediaItemId)
	if err != nil {
		return nil, internalError("get artwork", err)
	}
	resp := &eveyv1.GetArtworkResponse{}
	for _, r := range records {
		resp.Artwork = append(resp.Artwork, artworkRecordToProto(r))
	}
	return resp, nil
}

func (h *metadataHandler) SetMetadataOverride(ctx context.Context, req *eveyv1.SetMetadataOverrideRequest) (*eveyv1.MetadataOverride, error) {
	override := &domainmetadata.MetadataOverride{
		ID:          id.New("mo-"),
		MediaItemID: req.MediaItemId,
		FieldName:   req.FieldName,
		CustomValue: req.CustomValue,
	}
	if err := h.cmd.SetOverride(ctx, override); err != nil {
		return nil, internalError("set override", err)
	}
	return metadataOverrideToProto(override), nil
}

func (h *metadataHandler) RemoveMetadataOverride(ctx context.Context, req *eveyv1.RemoveMetadataOverrideRequest) (*eveyv1.RemoveMetadataOverrideResponse, error) {
	if err := h.cmd.RemoveOverride(ctx, req.Id); err != nil {
		return nil, internalError("remove override", err)
	}
	return &eveyv1.RemoveMetadataOverrideResponse{}, nil
}

func (h *metadataHandler) ListMetadataOverrides(ctx context.Context, req *eveyv1.ListMetadataOverridesRequest) (*eveyv1.ListMetadataOverridesResponse, error) {
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	overrides, err := h.query.ListOverrides(ctx, req.MediaItemId)
	if err != nil {
		return nil, internalError("list overrides", err)
	}
	resp := &eveyv1.ListMetadataOverridesResponse{}
	for _, o := range overrides {
		resp.Overrides = append(resp.Overrides, metadataOverrideToProto(o))
	}
	return resp, nil
}

func (h *metadataHandler) GetMetadataProviders(_ context.Context, _ *eveyv1.GetMetadataProvidersRequest) (*eveyv1.GetMetadataProvidersResponse, error) {
	resp := &eveyv1.GetMetadataProvidersResponse{}
	for i, name := range h.query.ListProviders() {
		resp.Providers = append(resp.Providers, &eveyv1.MetadataProvider{
			Id:      name,
			Name:    name,
			Enabled: true,
			Priority: int32(i + 1),
		})
	}
	return resp, nil
}
