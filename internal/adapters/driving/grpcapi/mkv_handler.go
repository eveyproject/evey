package grpcapi

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmkv "github.com/eveyproject/evey/internal/app/mkv"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mkvHandler struct {
	eveyv1.UnimplementedMKVServiceServer
	svc        *appmkv.Service
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
}

// checkPathAccess gates filesystem-path operations that are exposed to regular
// users (ListChapters, used by the player's skip-intro fallback) so a non-admin
// cannot read MKV metadata for files outside the libraries they have access to.
// The path must resolve to a known media item the caller can see; admins bypass.
// Denies when userAccess is unwired (fail closed); still a no-op when only
// libQuery is unwired, since that is a read dependency, not the authorization
// port. Mirrors metadataHandler.checkMediaItemAccess.
func (h *mkvHandler) checkPathAccess(ctx context.Context, path string) error {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "mkvHandler.checkPathAccess")
	}
	if h.libQuery == nil {
		return nil
	}
	item, err := h.libQuery.GetMediaItemByFilePath(ctx, path)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *mkvHandler) ListChapters(ctx context.Context, req *eveyv1.ListChaptersRequest) (*eveyv1.ListChaptersResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mkv.ListChapters")
	defer span.End()
	if err := h.checkPathAccess(ctx, req.Path); err != nil {
		return nil, err
	}
	chapters, err := h.svc.ListChapters(ctx, req.Path)
	if err != nil {
		return nil, internalError(ctx, "list chapters", err)
	}
	resp := &eveyv1.ListChaptersResponse{}
	for _, ch := range chapters {
		resp.Chapters = append(resp.Chapters, mkvChapterToProto(ch))
	}
	return resp, nil
}
