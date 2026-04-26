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
	svc           *appmedia.ShareService
	libQuery      *applibrary.QueryService
	userAccess    *appuser.AccessService
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
		Url:       "/share/" + link.Token,
	}
	if link.MaxViews != nil {
		v := int32(*link.MaxViews)
		resp.MaxViews = &v
	}
	return resp, nil
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
			Url:         "/share/" + l.Token,
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
		return nil, status.Error(codes.PermissionDenied, "cannot revoke another user's share link")
	}

	if err := h.svc.Revoke(ctx, req.Id); err != nil {
		return nil, status.Error(codes.NotFound, "share link not found")
	}
	return &eveyv1.RevokeShareLinkResponse{}, nil
}

func (h *shareHandler) ValidateShareLink(ctx context.Context, req *eveyv1.ValidateShareLinkRequest) (*eveyv1.ValidateShareLinkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.share.ValidateShareLink")
	defer span.End()
	link, err := h.svc.Validate(ctx, req.Token)
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
		StreamUrl:  "/api/v1/media/" + string(item.ID) + "/stream",
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
