package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// user_handler_invite.go holds the invite-link lifecycle: create / list / revoke
// plus registration and validation against an invite.

func (h *userHandler) CreateInviteLink(ctx context.Context, req *eveyv1.CreateInviteLinkRequest) (*eveyv1.InviteLink, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CreateInviteLink")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin required")
		return nil, status.Error(codes.PermissionDenied, "admin required")
	}
	libIDs := make([]shared.LibraryID, len(req.LibraryIds))
	for i, raw := range req.LibraryIds {
		libIDs[i] = shared.LibraryID(raw)
	}
	link, err := user.NewInviteLink(
		id.New("inv-"),
		shared.UserID(claims.UserID),
		secureToken("inv-tok-", 16),
		user.WithInviteMaxUses(int(req.MaxUses)),
		user.WithInviteAdmin(req.IsAdmin),
		user.WithInviteLibraries(libIDs),
		user.WithInviteExpiresAt(fromTimestamppbPtr(req.ExpiresAt)),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.access.CreateInviteLink(ctx, link); err != nil {
		slog.ErrorContext(ctx, "create invite link", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return inviteLinkToProto(link), nil
}

func (h *userHandler) ListInviteLinks(ctx context.Context, _ *eveyv1.ListInviteLinksRequest) (*eveyv1.ListInviteLinksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListInviteLinks")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	creatorID := ""
	if claims != nil {
		creatorID = claims.UserID
	}
	links, err := h.query.ListInviteLinks(ctx, creatorID)
	if err != nil {
		slog.ErrorContext(ctx, "list invite links", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListInviteLinksResponse{}
	for _, l := range links {
		resp.InviteLinks = append(resp.InviteLinks, inviteLinkToProto(l))
	}
	return resp, nil
}

func (h *userHandler) RevokeInviteLink(ctx context.Context, req *eveyv1.RevokeInviteLinkRequest) (*eveyv1.RevokeInviteLinkResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RevokeInviteLink")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.RevokeInviteLink: %w", err)
	}
	if !claims.IsAdmin {
		links, err := h.query.ListInviteLinks(ctx, claims.UserID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "list invite links", err)
		}
		owned := false
		for _, l := range links {
			if l.ID == req.Id {
				owned = true
				break
			}
		}
		if !owned {
			span.SetStatus(otelcodes.Error, "cannot revoke invite other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_revoke_invite_other")
		}
	}
	if err := h.access.RevokeInviteLink(ctx, req.Id); err != nil {
		slog.ErrorContext(ctx, "revoke invite link", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.RevokeInviteLinkResponse{}, nil
}

func (h *userHandler) RegisterWithInvite(ctx context.Context, req *eveyv1.RegisterWithInviteRequest) (*eveyv1.AuthResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RegisterWithInvite")
	defer span.End()
	u, err := h.access.RegisterWithInvite(ctx, req.InviteCode, req.Username, req.Password, req.Email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid invite data")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.invalid_invite_data")
	}
	access, refresh, loginErr := h.auth.Login(ctx, req.Username, req.Password)
	if loginErr != nil {
		slog.WarnContext(ctx, "register with invite: auto-login failed", slog.String("username", sanitizeLogValue(req.Username)), slog.Any("err", loginErr))
		return &eveyv1.AuthResponse{User: userToProto(u)}, nil
	}
	return &eveyv1.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         userToProto(u),
	}, nil
}

func (h *userHandler) ValidateInvite(ctx context.Context, req *eveyv1.ValidateInviteRequest) (*eveyv1.ValidateInviteResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ValidateInvite")
	defer span.End()
	valid, isAdmin, err := h.access.ValidateInvite(ctx, req.Code)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "validate invite", err)
	}
	return &eveyv1.ValidateInviteResponse{Valid: valid, IsAdmin: isAdmin}, nil
}
