package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type libraryAccessHandler struct {
	eveyv1.UnimplementedLibraryAccessServiceServer
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
}

func (h *libraryAccessHandler) GetUserLibraryAccess(ctx context.Context, req *eveyv1.GetUserLibraryAccessRequest) (*eveyv1.UserLibraryAccess, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library_access.GetUserLibraryAccess")
	defer span.End()
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library_access.SetUserLibraryAccess")
	defer span.End()
	// Fail closed on read/revoke errors: swallowing them left `existing` nil, so
	// the revoke loop no-opped and the new grants were added on top of the old
	// ones — narrowing a user's access silently widened it, reported as success.
	existing, err := h.userAccess.GetLibraryAccessByUserID(ctx, req.UserId)
	if err != nil {
		return nil, internalError("set library access: read existing", err)
	}
	for _, id := range existing {
		if err := h.userAccess.RevokeLibraryAccess(ctx, req.UserId, id); err != nil {
			return nil, internalError("set library access: revoke", err)
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library_access.RevokeAllUserLibraryAccess")
	defer span.End()
	// Fail closed: a swallowed read/revoke error meant "revoke all" could revoke
	// nothing yet report success, leaving the user with full access.
	existing, err := h.userAccess.GetLibraryAccessByUserID(ctx, req.UserId)
	if err != nil {
		return nil, internalError("revoke all library access: read existing", err)
	}
	for _, id := range existing {
		if err := h.userAccess.RevokeLibraryAccess(ctx, req.UserId, id); err != nil {
			return nil, internalError("revoke all library access: revoke", err)
		}
	}
	return &eveyv1.RevokeAllUserLibraryAccessResponse{}, nil
}
