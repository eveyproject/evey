package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type metadataHandler struct {
	eveyv1.UnimplementedMetadataServiceServer
	svc *appmetadata.Service
}

func (h *metadataHandler) SearchMetadata(ctx context.Context, req *eveyv1.SearchMetadataRequest) (*eveyv1.SearchMetadataResponse, error) {
	mediaType := mediaItemTypeFromProto[req.Type]
	results, err := h.svc.SearchMetadata(ctx, req.Query, mediaType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search metadata: %v", err)
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
	_, err := h.svc.RefreshMetadata(stream.Context(), req.MediaItemId, "", req.PreferredProvider, "")
	if err != nil {
		return status.Errorf(codes.Internal, "refresh metadata: %v", err)
	}
	return stream.Send(&eveyv1.RefreshMetadataProgress{
		MediaItemId: req.MediaItemId,
		Phase:       "complete",
		Percent:     100,
	})
}

func (h *metadataHandler) GetArtwork(ctx context.Context, req *eveyv1.GetArtworkRequest) (*eveyv1.GetArtworkResponse, error) {
	records, err := h.svc.GetArtwork(ctx, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get artwork: %v", err)
	}
	resp := &eveyv1.GetArtworkResponse{}
	for _, r := range records {
		resp.Artwork = append(resp.Artwork, artworkRecordToProto(r))
	}
	return resp, nil
}

func (h *metadataHandler) SetMetadataOverride(ctx context.Context, req *eveyv1.SetMetadataOverrideRequest) (*eveyv1.MetadataOverride, error) {
	override := &domainmetadata.MetadataOverride{
		ID:          fmt.Sprintf("mo-%d", nowUnixNano()),
		MediaItemID: req.MediaItemId,
		FieldName:   req.FieldName,
		CustomValue: req.CustomValue,
	}
	if err := h.svc.SetOverride(ctx, override); err != nil {
		return nil, status.Errorf(codes.Internal, "set override: %v", err)
	}
	return metadataOverrideToProto(override), nil
}

func (h *metadataHandler) RemoveMetadataOverride(ctx context.Context, req *eveyv1.RemoveMetadataOverrideRequest) (*eveyv1.RemoveMetadataOverrideResponse, error) {
	if err := h.svc.RemoveOverride(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "remove override: %v", err)
	}
	return &eveyv1.RemoveMetadataOverrideResponse{}, nil
}

func (h *metadataHandler) ListMetadataOverrides(ctx context.Context, req *eveyv1.ListMetadataOverridesRequest) (*eveyv1.ListMetadataOverridesResponse, error) {
	overrides, err := h.svc.ListOverrides(ctx, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list overrides: %v", err)
	}
	resp := &eveyv1.ListMetadataOverridesResponse{}
	for _, o := range overrides {
		resp.Overrides = append(resp.Overrides, metadataOverrideToProto(o))
	}
	return resp, nil
}

func (h *metadataHandler) GetMetadataProviders(_ context.Context, _ *eveyv1.GetMetadataProvidersRequest) (*eveyv1.GetMetadataProvidersResponse, error) {
	resp := &eveyv1.GetMetadataProvidersResponse{}
	for i, name := range h.svc.ListProviders() {
		resp.Providers = append(resp.Providers, &eveyv1.MetadataProvider{
			Id:      name,
			Name:    name,
			Enabled: true,
			Priority: int32(i + 1),
		})
	}
	return resp, nil
}
