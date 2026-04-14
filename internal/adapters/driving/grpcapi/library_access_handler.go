package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appuser "github.com/gravity-zero/evey/internal/app/user"
)

type libraryAccessHandler struct {
	eveyv1.UnimplementedLibraryAccessServiceServer
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
}

func (h *libraryAccessHandler) GetUserLibraryAccess(ctx context.Context, req *eveyv1.GetUserLibraryAccessRequest) (*eveyv1.UserLibraryAccess, error) {
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, req.UserId)
	if err != nil {
		return nil, internalError("get library access", err)
	}
	if ids == nil {
		ids = []string{}
	}
	resp := &eveyv1.UserLibraryAccess{UserId: req.UserId, LibraryIds: ids}
	libs, err := h.libQuery.ListLibraries(ctx)
	if err != nil {
		slog.Warn("list libraries for access", slog.Any("err", err))
	}
	for _, l := range libs {
		resp.Libraries = append(resp.Libraries, &eveyv1.LibraryRef{Id: string(l.ID), Name: l.Name})
	}
	return resp, nil
}

func (h *libraryAccessHandler) SetUserLibraryAccess(ctx context.Context, req *eveyv1.SetUserLibraryAccessRequest) (*eveyv1.SetUserLibraryAccessResponse, error) {
	existing, err := h.userAccess.GetLibraryAccessByUserID(ctx, req.UserId)
	if err != nil {
		slog.Warn("get existing library access", slog.String("user_id", req.UserId), slog.Any("err", err))
	}
	for _, id := range existing {
		if err := h.userAccess.RevokeLibraryAccess(ctx, req.UserId, id); err != nil {
			slog.Warn("revoke library access", slog.Any("err", err))
		}
	}
	for _, id := range req.LibraryIds {
		if err := h.userAccess.GrantLibraryAccess(ctx, req.UserId, id); err != nil {
			return nil, internalError("grant library access", err)
		}
	}
	return &eveyv1.SetUserLibraryAccessResponse{}, nil
}

func (h *libraryAccessHandler) RevokeAllUserLibraryAccess(ctx context.Context, req *eveyv1.RevokeAllUserLibraryAccessRequest) (*eveyv1.RevokeAllUserLibraryAccessResponse, error) {
	existing, err := h.userAccess.GetLibraryAccessByUserID(ctx, req.UserId)
	if err != nil {
		slog.Warn("get existing library access for revoke", slog.String("user_id", req.UserId), slog.Any("err", err))
	}
	for _, id := range existing {
		if err := h.userAccess.RevokeLibraryAccess(ctx, req.UserId, id); err != nil {
			slog.Warn("revoke library access", slog.Any("err", err))
		}
	}
	return &eveyv1.RevokeAllUserLibraryAccessResponse{}, nil
}
