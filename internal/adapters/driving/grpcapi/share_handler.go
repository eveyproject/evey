package grpcapi

import (
	"context"
	"strings"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmedia "github.com/gravity-zero/evey/internal/app/media"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type shareHandler struct {
	eveyv1.UnimplementedShareServiceServer
	svc           *appmedia.ShareService
	libQuery      *applibrary.QueryService
	libraryAccess user.LibraryAccessRepository
}

func (h *shareHandler) CreateShareLink(ctx context.Context, req *eveyv1.CreateShareLinkRequest) (*eveyv1.CreateShareLinkResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}

	item, err := h.libQuery.GetMediaItem(ctx, req.MediaItemId)
	if err != nil || item == nil {
		return nil, status.Error(codes.NotFound, "media item not found")
	}
	if ok, _ := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
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
		return nil, status.Errorf(codes.Internal, "create share link: %v", err)
	}

	resp := &eveyv1.CreateShareLinkResponse{
		Id:        link.ID,
		Token:     link.Token,
		MediaId:   link.MediaItemID,
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
		return nil, status.Errorf(codes.Internal, "list share links: %v", err)
	}
	resp := &eveyv1.ListShareLinksResponse{}
	for _, l := range links {
		item := &eveyv1.ShareLinkProto{
			Id:          l.ID,
			Token:       l.Token,
			MediaItemId: l.MediaItemID,
			CreatedBy:   l.CreatedBy,
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
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}

	link, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "share link not found")
	}
	if !claims.IsAdmin && link.CreatedBy != claims.UserID {
		return nil, status.Error(codes.PermissionDenied, "cannot revoke another user's share link")
	}

	if err := h.svc.Revoke(ctx, req.Id); err != nil {
		return nil, status.Error(codes.NotFound, "share link not found")
	}
	return &eveyv1.RevokeShareLinkResponse{}, nil
}

func (h *shareHandler) ValidateShareLink(ctx context.Context, req *eveyv1.ValidateShareLinkRequest) (*eveyv1.ValidateShareLinkResponse, error) {
	link, err := h.svc.Validate(ctx, req.Token)
	if err != nil {
		if strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "view limit") {
			return nil, status.Error(codes.Unavailable, "link expired or view limit reached")
		}
		return nil, status.Error(codes.NotFound, "share link not found")
	}

	item, err := h.libQuery.GetMediaItem(ctx, link.MediaItemID)
	if err != nil || item == nil {
		return nil, status.Error(codes.NotFound, "media not found")
	}

	return &eveyv1.ValidateShareLinkResponse{
		MediaId:    item.ID,
		Title:      item.Title,
		MediaType:  string(item.Type),
		DurationMs: int64(item.DurationMs),
		StreamUrl:  "/api/v1/media/" + item.ID + "/stream",
	}, nil
}

func (h *shareHandler) checkLibraryAccess(ctx context.Context, libraryID string) (bool, error) {
	if h.libraryAccess == nil {
		return true, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return true, nil
	}
	return h.libraryAccess.HasAccess(ctx, claims.UserID, libraryID)
}
