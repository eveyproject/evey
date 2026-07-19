package grpcapi

import (
	"context"
	"errors"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmedia "github.com/evey-project/evey/internal/app/media"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type shareHandler struct {
	eveyv1.UnimplementedShareServiceServer
	svc        *appmedia.ShareService
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
}

func (h *shareHandler) CreateShareLink(ctx context.Context, req *eveyv1.CreateShareLinkRequest) (*eveyv1.CreateShareLinkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.share.CreateShareLink")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}

	item, err := h.libQuery.GetMediaItem(ctx, req.MediaItemId)
	if err != nil || item == nil {
		return nil, status.Error(codes.NotFound, "media item not found")
	}
	if ok, _ := h.checkLibraryAccess(ctx, string(item.LibraryID)); !ok {
		return nil, status.Error(codes.NotFound, "media item not found")
	}

	durationHours := int(req.DurationHours)
	if durationHours <= 0 {
		durationHours = 48
	}

	var maxViews *int
	if req.MaxViews != nil {
		v := int(*req.MaxViews)
		maxViews = &v
	}

	link, err := h.svc.Create(ctx, req.MediaItemId, claims.UserID, time.Duration(durationHours)*time.Hour, maxViews)
	if err != nil {
		return nil, internalError("create share link", err)
	}

	resp := &eveyv1.CreateShareLinkResponse{
		Id:        link.ID,
		Token:     link.Token,
		MediaId:   string(link.MediaItemID),
		ExpiresAt: toTimestamppb(link.ExpiresAt),
		Url:       shareDownloadURL(link.Token),
	}
	if link.MaxViews != nil {
		v := int32(*link.MaxViews)
		resp.MaxViews = &v
	}
	return resp, nil
}

// shareDownloadURL is the public, unauthenticated download path a share link
// resolves to. Served by the HTTP layer (shareDownloadHandler).
func shareDownloadURL(token string) string {
	return "/api/v1/share/" + token + "/download"
}

func (h *shareHandler) ListShareLinks(ctx context.Context, _ *eveyv1.ListShareLinksRequest) (*eveyv1.ListShareLinksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.share.ListShareLinks")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}

	var links []*shared.ShareLink
	if claims.IsAdmin {
		links, err = h.svc.ListAll(ctx)
	} else {
		links, err = h.svc.List(ctx, claims.UserID)
	}
	if err != nil {
		return nil, internalError("list share links", err)
	}
	resp := &eveyv1.ListShareLinksResponse{}
	for _, l := range links {
		item := &eveyv1.ShareLinkProto{
			Id:          l.ID,
			Token:       l.Token,
			MediaItemId: string(l.MediaItemID),
			CreatedBy:   string(l.CreatedBy),
			ExpiresAt:   toTimestamppb(l.ExpiresAt),
			ViewCount:   int32(l.ViewCount),
			Url:         shareDownloadURL(l.Token),
		}
		if l.MaxViews != nil {
			v := int32(*l.MaxViews)
			item.MaxViews = &v
		}
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}

func (h *shareHandler) RevokeShareLink(ctx context.Context, req *eveyv1.RevokeShareLinkRequest) (*eveyv1.RevokeShareLinkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.share.RevokeShareLink")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}

	link, err := h.svc.GetByID(ctx, req.Id)
	if err != nil || link == nil {
		return nil, status.Error(codes.NotFound, "share link not found")
	}
	if !claims.IsAdmin && string(link.CreatedBy) != claims.UserID {
		// NotFound, not PermissionDenied: don't confirm a share link exists to a
		// non-owner who guessed its ID (matches the not-found response above).
		return nil, status.Error(codes.NotFound, "share link not found")
	}

	if err := h.svc.Revoke(ctx, req.Id); err != nil {
		// The link exists and the caller is authorised. A genuine not-found here
		// (already revoked) is fine, but any other failure means the revoke did
		// NOT take effect - the link is still live. Reporting NotFound would
		// falsely tell the admin a leaked link is gone; surface the real failure.
		if errors.Is(err, shared.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "share link not found")
		}
		return nil, internalError("revoke share link", err)
	}
	return &eveyv1.RevokeShareLinkResponse{}, nil
}

func (h *shareHandler) ValidateShareLink(ctx context.Context, req *eveyv1.ValidateShareLinkRequest) (*eveyv1.ValidateShareLinkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.share.ValidateShareLink")
	defer span.End()
	// Peek, not Validate: this RPC only describes the link (the RBAC table
	// classifies it as harmless metadata, callable by any authenticated user).
	// Spending a view here let anyone exhaust a maxViews=1 link before its
	// recipient opened it. The view is consumed by the download route alone.
	link, err := h.svc.Peek(ctx, req.Token)
	if err != nil {
		if errors.Is(err, appmedia.ErrShareExpired) || errors.Is(err, appmedia.ErrShareViewLimit) {
			return nil, status.Error(codes.Unavailable, "link expired or view limit reached")
		}
		return nil, status.Error(codes.NotFound, "share link not found")
	}

	item, err := h.libQuery.GetMediaItem(ctx, string(link.MediaItemID))
	if err != nil || item == nil {
		return nil, status.Error(codes.NotFound, "media not found")
	}

	var durationMs int64
	if item.File != nil {
		durationMs = item.File.Duration.Milliseconds()
	}
	return &eveyv1.ValidateShareLinkResponse{
		MediaId:    string(item.ID),
		Title:      item.Title,
		MediaType:  string(item.Type),
		DurationMs: durationMs,
		// Share links resolve to a public download (no auth), not a stream:
		// the old /media/{id}/stream URL required a login the recipient lacks.
		StreamUrl: shareDownloadURL(req.Token),
	}, nil
}

func (h *shareHandler) checkLibraryAccess(ctx context.Context, libraryID string) (bool, error) {
	if h.userAccess == nil {
		return true, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return true, nil
	}
	return h.userAccess.HasLibraryAccess(ctx, claims.UserID, libraryID)
}
