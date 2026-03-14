package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type libraryAccessHandler struct {
	eveyv1.UnimplementedLibraryAccessServiceServer
	accessRepo domainuser.LibraryAccessRepository
	libQuery   *applibrary.QueryService
}

func (h *libraryAccessHandler) GetUserLibraryAccess(ctx context.Context, req *eveyv1.GetUserLibraryAccessRequest) (*eveyv1.UserLibraryAccess, error) {
	ids, err := h.accessRepo.GetByUserID(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get library access: %v", err)
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
		resp.Libraries = append(resp.Libraries, &eveyv1.LibraryRef{Id: l.ID, Name: l.Name})
	}
	return resp, nil
}

func (h *libraryAccessHandler) SetUserLibraryAccess(ctx context.Context, req *eveyv1.SetUserLibraryAccessRequest) (*eveyv1.SetUserLibraryAccessResponse, error) {
	existing, _ := h.accessRepo.GetByUserID(ctx, req.UserId)
	for _, id := range existing {
		if err := h.accessRepo.Revoke(ctx, req.UserId, id); err != nil {
			slog.Warn("revoke library access", slog.Any("err", err))
		}
	}
	for _, id := range req.LibraryIds {
		if err := h.accessRepo.Grant(ctx, req.UserId, id); err != nil {
			return nil, status.Errorf(codes.Internal, "grant library access: %v", err)
		}
	}
	return &eveyv1.SetUserLibraryAccessResponse{}, nil
}

func (h *libraryAccessHandler) RevokeAllUserLibraryAccess(ctx context.Context, req *eveyv1.RevokeAllUserLibraryAccessRequest) (*eveyv1.RevokeAllUserLibraryAccessResponse, error) {
	existing, _ := h.accessRepo.GetByUserID(ctx, req.UserId)
	for _, id := range existing {
		if err := h.accessRepo.Revoke(ctx, req.UserId, id); err != nil {
			slog.Warn("revoke library access", slog.Any("err", err))
		}
	}
	return &eveyv1.RevokeAllUserLibraryAccessResponse{}, nil
}
