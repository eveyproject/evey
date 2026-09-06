package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "metadataHandler.checkMediaItemAccess")
	}
	// Deny, like the userAccess check above. This function is the ONLY
	// authorization layer for MetadataService writes - rbac.go leaves the
	// service un-gated on purpose and points here - so an unwired port
	// returning "allow" would hand every authenticated caller edit rights on
	// media in libraries they were never granted. Not reachable today (grpc.go
	// always wires LibQuery); this is the fail-closed default that keeps it
	// unreachable after the next refactor. Mirrors http/helpers.go's
	// checkLibraryAccess, which already denies on a nil checker.
	if h.libQuery == nil {
		slog.ErrorContext(ctx, "metadataHandler.checkMediaItemAccess: library query service not configured")
		return status.Error(codes.FailedPrecondition, "library query service not configured")
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *metadataHandler) SearchMetadata(ctx context.Context, req *eveyv1.SearchMetadataRequest) (*eveyv1.SearchMetadataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.SearchMetadata")
	defer span.End()
	mediaType := mediaItemTypeFromProto[req.Type]
	results, err := h.query.SearchMetadata(ctx, req.Query, mediaType)
	if err != nil {
		return nil, internalError(ctx, "search metadata", err)
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
	_, span := tracing.StartSpan(ctx, "grpc.metadata.RefreshMetadata")
	defer span.End()
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return err
	}

	// Both the auto path (no preferred provider) and the forced-provider path
	// resolve the provider ID + media type from the stored item and run a real
	// re-enrich. The old forced-provider branch passed empty IDs to the provider
	// (→ "unsupported media type" 500) and discarded the result without ever
	// persisting it; route everything through the enriching entry points.
	item, err := h.cmd.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		return internalError(ctx, "get item", err)
	}
	providerIDs := map[string]string{}
	if item.Catalog != nil {
		providerIDs = item.Catalog.ProviderIDs
	}
	mediaType := "movie"
	if item.Type == "series" || item.Type == "season" || item.Type == "episode" {
		mediaType = "tv"
	}

	switch req.PreferredProvider {
	case "", "tmdb":
		tmdbID := providerIDs["tmdb"]
		if tmdbID == "" {
			return status.Errorf(codes.FailedPrecondition, "no TMDB ID for this item")
		}
		if err := h.cmd.RefreshItemMetadata(ctx, req.MediaItemId, tmdbID, mediaType); err != nil {
			return internalError(ctx, "refresh metadata", err)
		}
	case "imdb", "omdb", "imdbapi":
		// omdb/imdbapi are keyed by the IMDB id; the enrich path cross-refs
		// IMDB → TMDB so locale translations land on the right record.
		imdbID := providerIDs["imdb"]
		if imdbID == "" {
			return status.Errorf(codes.FailedPrecondition, "no IMDB ID for this item")
		}
		if err := h.cmd.RefreshItemMetadataByIMDB(ctx, req.MediaItemId, imdbID, mediaType); err != nil {
			return internalError(ctx, "refresh metadata", err)
		}
	default:
		return status.Errorf(codes.InvalidArgument, "unsupported provider %q", req.PreferredProvider)
	}

	return stream.Send(&eveyv1.RefreshMetadataProgress{
		MediaItemId: req.MediaItemId,
		Phase:       "complete",
		Percent:     100,
	})
}

func (h *metadataHandler) GetArtwork(ctx context.Context, req *eveyv1.GetArtworkRequest) (*eveyv1.GetArtworkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.GetArtwork")
	defer span.End()
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	records, err := h.query.GetArtwork(ctx, req.MediaItemId)
	if err != nil {
		return nil, internalError(ctx, "get artwork", err)
	}
	resp := &eveyv1.GetArtworkResponse{}
	for _, r := range records {
		resp.Artwork = append(resp.Artwork, artworkRecordToProto(r))
	}
	return resp, nil
}

func (h *metadataHandler) SetMetadataOverride(ctx context.Context, req *eveyv1.SetMetadataOverrideRequest) (*eveyv1.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.SetMetadataOverride")
	defer span.End()
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	override, err := domainmetadata.NewMetadataOverride(id.New("mo-"), shared.MediaItemID(req.MediaItemId), req.FieldName,
		domainmetadata.WithOverrideValue(req.CustomValue),
	)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.cmd.SetOverride(ctx, override); err != nil {
		return nil, internalError(ctx, "set override", err)
	}
	return metadataOverrideToProto(override), nil
}

func (h *metadataHandler) RemoveMetadataOverride(ctx context.Context, req *eveyv1.RemoveMetadataOverrideRequest) (*eveyv1.RemoveMetadataOverrideResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.RemoveMetadataOverride")
	defer span.End()
	// Resolve the override → media item so we can gate the call by library
	// access. Returning NotFound on a stale id keeps existence opaque.
	override, err := h.cmd.GetOverride(ctx, req.Id)
	if err != nil || override == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if err := h.checkMediaItemAccess(ctx, string(override.MediaItemID)); err != nil {
		return nil, err
	}
	if err := h.cmd.RemoveOverride(ctx, req.Id); err != nil {
		return nil, internalError(ctx, "remove override", err)
	}
	return &eveyv1.RemoveMetadataOverrideResponse{}, nil
}

func (h *metadataHandler) ListMetadataOverrides(ctx context.Context, req *eveyv1.ListMetadataOverridesRequest) (*eveyv1.ListMetadataOverridesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.ListMetadataOverrides")
	defer span.End()
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	overrides, err := h.query.ListOverrides(ctx, req.MediaItemId)
	if err != nil {
		return nil, internalError(ctx, "list overrides", err)
	}
	resp := &eveyv1.ListMetadataOverridesResponse{}
	for _, o := range overrides {
		resp.Overrides = append(resp.Overrides, metadataOverrideToProto(o))
	}
	return resp, nil
}

func (h *metadataHandler) ResearchMetadata(ctx context.Context, req *eveyv1.ResearchMetadataRequest) (*eveyv1.ResearchMetadataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.metadata.ResearchMetadata")
	defer span.End()
	if req.MediaItemId == "" {
		return nil, status.Error(codes.InvalidArgument, "media_item_id is required")
	}
	if err := h.checkMediaItemAccess(ctx, req.MediaItemId); err != nil {
		return nil, err
	}
	res, err := h.cmd.ResearchMetadata(ctx, req.MediaItemId)
	if err != nil {
		return nil, internalError(ctx, "research metadata", err)
	}
	return &eveyv1.ResearchMetadataResponse{
		MediaItemId: req.MediaItemId,
		Matched:     res.Matched,
		NewTitle:    res.NewTitle,
	}, nil
}

func (h *metadataHandler) GetMetadataProviders(ctx context.Context, _ *eveyv1.GetMetadataProvidersRequest) (*eveyv1.GetMetadataProvidersResponse, error) {
	_, span := tracing.StartSpan(ctx, "grpc.metadata.GetMetadataProviders")
	defer span.End()
	resp := &eveyv1.GetMetadataProvidersResponse{}
	for i, name := range h.query.ListProviders() {
		resp.Providers = append(resp.Providers, &eveyv1.MetadataProvider{
			Id:       name,
			Name:     name,
			Enabled:  true,
			Priority: int32(i + 1),
		})
	}
	return resp, nil
}
