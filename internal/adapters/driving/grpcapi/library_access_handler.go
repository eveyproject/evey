package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
		return nil, internalError(ctx, "get library access", err)
	}
	if ids == nil {
		ids = []string{}
	}
	resp := &eveyv1.UserLibraryAccess{UserId: req.UserId, LibraryIds: ids}
	libs, err := h.libQuery.ListLibraries(ctx)
	if err != nil {
		slog.WarnContext(ctx, "list libraries for access", slog.Any("err", err))
	}
	for _, l := range libs {
		resp.Libraries = append(resp.Libraries, &eveyv1.LibraryRef{Id: string(l.ID), Name: l.Name})
	}
	return resp, nil
}

func (h *libraryAccessHandler) SetUserLibraryAccess(ctx context.Context, req *eveyv1.SetUserLibraryAccessRequest) (*eveyv1.SetUserLibraryAccessResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library_access.SetUserLibraryAccess")
	defer span.End()
	// Fail closed: this used to be a revoke-everything loop followed by a
	// grant-the-new-set loop. Swallowing a read/revoke error there left
	// `existing` nil, so the revoke loop no-opped and the new grants were
	// added on top of the old ones - narrowing a user's access silently
	// widened it instead. Beyond that, the two-loop shape had a real
	// atomicity gap: a crash or error between the loops left the user
	// revoked from everything and re-granted nothing. ReplaceLibraryAccess
	// does the swap in one transaction, so a failure here leaves the user's
	// PRIOR access untouched rather than at zero.
	if err := h.userAccess.ReplaceLibraryAccess(ctx, req.UserId, req.LibraryIds); err != nil {
		return nil, internalError(ctx, "set library access", err)
	}
	return &eveyv1.SetUserLibraryAccessResponse{}, nil
}

func (h *libraryAccessHandler) RevokeAllUserLibraryAccess(ctx context.Context, req *eveyv1.RevokeAllUserLibraryAccessRequest) (*eveyv1.RevokeAllUserLibraryAccessResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library_access.RevokeAllUserLibraryAccess")
	defer span.End()
	// Fail closed: this used to be a read-then-revoke-in-a-loop; a swallowed
	// read/revoke error meant "revoke all" could revoke nothing yet report
	// success, leaving the user with full access. RevokeAllLibraryAccess is a
	// single DELETE, so it either revokes everything or reports the error -
	// no partial-loop state to reason about.
	if err := h.userAccess.RevokeAllLibraryAccess(ctx, req.UserId); err != nil {
		return nil, internalError(ctx, "revoke all library access", err)
	}
	return &eveyv1.RevokeAllUserLibraryAccessResponse{}, nil
}
