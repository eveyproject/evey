package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// user_handler_user.go holds the user-account RPCs: create / update / delete /
// disable / impersonate / list (admin-gated CRUD).

func (h *userHandler) CreateUser(ctx context.Context, req *eveyv1.CreateUserRequest) (*eveyv1.User, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CreateUser")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	u, err := user.NewUser(req.Username, req.Email, "")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	u.IsAdmin = req.IsAdmin
	if req.MaxProfiles != nil {
		if *req.MaxProfiles < 0 {
			span.SetStatus(otelcodes.Error, "max_profiles must be >= 0")
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.limit_negative", map[string]string{"field": "max_profiles"})
		}
		mp := int(*req.MaxProfiles)
		u.MaxProfiles = &mp
	}
	if req.MaxDevicesPerProfile != nil {
		if *req.MaxDevicesPerProfile < 0 {
			span.SetStatus(otelcodes.Error, "max_devices_per_profile must be >= 0")
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.limit_negative", map[string]string{"field": "max_devices_per_profile"})
		}
		md := int(*req.MaxDevicesPerProfile)
		u.MaxDevicesPerProfile = &md
	}
	if err := h.userSvc.CreateUser(ctx, claims.IsAdmin, u, req.Password); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appuser.ErrNotAdmin):
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
		case errors.Is(err, appuser.ErrPasswordTooShort):
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.password_too_short")
		default:
			// Validation already happened above via NewUser; anything left is a
			// hashing/DB failure - don't mislabel it as PermissionDenied.
			return nil, internalError(ctx, "create user", err)
		}
	}
	uidStr := string(u.ID)
	h.audit(ctx, claims.UserID, "create_user", "user", &uidStr, map[string]any{"username": u.Username, "is_admin": u.IsAdmin})
	return userToProto(u), nil
}

func (h *userHandler) UpdateUser(ctx context.Context, req *eveyv1.UpdateUserRequest) (*eveyv1.User, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.UpdateUser")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.UpdateUser: %w", err)
	}
	if !claims.IsAdmin {
		if req.Id != claims.UserID {
			span.SetStatus(otelcodes.Error, "cannot update other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_other")
		}
		if req.IsAdmin != nil && *req.IsAdmin {
			span.SetStatus(otelcodes.Error, "cannot escalate admin")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_escalate_admin")
		}
		if req.MaxStreams != nil || req.MaxProfiles != nil || req.MaxDevicesPerProfile != nil {
			span.SetStatus(otelcodes.Error, "cannot change caps")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_change_caps")
		}
	}
	if req.MaxStreams != nil && *req.MaxStreams < 0 {
		span.SetStatus(otelcodes.Error, "max_streams must be >= 0")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.limit_negative", map[string]string{"field": "max_streams"})
	}
	if req.MaxProfiles != nil && *req.MaxProfiles < 0 {
		span.SetStatus(otelcodes.Error, "max_profiles must be >= 0")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.limit_negative", map[string]string{"field": "max_profiles"})
	}
	if req.MaxDevicesPerProfile != nil && *req.MaxDevicesPerProfile < 0 {
		span.SetStatus(otelcodes.Error, "max_devices_per_profile must be >= 0")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.limit_negative", map[string]string{"field": "max_devices_per_profile"})
	}
	u, err := h.query.GetUserByID(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.WarnContext(ctx, "grpc UpdateUser: user query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.not_found")
	}
	if req.Username != nil {
		u.Username = strings.TrimSpace(*req.Username)
	}
	if req.Email != nil {
		u.Email = strings.TrimSpace(*req.Email)
	}
	if req.IsAdmin != nil {
		u.IsAdmin = *req.IsAdmin
	}
	if req.MaxStreams != nil {
		ms := int(*req.MaxStreams)
		u.MaxStreams = &ms
	}
	if req.MaxProfiles != nil {
		mp := int(*req.MaxProfiles)
		u.MaxProfiles = &mp
	}
	if req.MaxDevicesPerProfile != nil {
		md := int(*req.MaxDevicesPerProfile)
		u.MaxDevicesPerProfile = &md
	}
	if err := h.userSvc.UpdateUser(ctx, u); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		// Promoting a user who already owns a kids profile to admin is a parental
		// business-rule refusal (the admin/kids invariant), not a server fault:
		// map it to FailedPrecondition with a clear reason, like CreateProfile,
		// rather than a generic 500 logged at ERROR (which reads as an alert).
		if errors.Is(err, appuser.ErrAdminCannotBeKids) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		slog.ErrorContext(ctx, "update user", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	// Password change: routed through ChangePassword so it is hashed, length-
	// validated, and accompanied by refresh-token deletion + token_epoch bump
	// (the field-update path above never touched the hash, so admin password
	// changes were silently dropped). Skipped when the field is absent/empty
	// ("leave blank to keep current").
	if req.Password != nil && *req.Password != "" {
		if err := h.userSvc.ChangePassword(ctx, req.Id, *req.Password); err != nil {
			if errors.Is(err, appuser.ErrPasswordTooShort) {
				span.SetStatus(otelcodes.Error, "password too short")
				return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.password_too_short")
			}
			slog.ErrorContext(ctx, "update user password", slog.Any("err", err))
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.Internal, "internal error")
		}
	}
	return userToProto(u), nil
}

func (h *userHandler) DeleteUser(ctx context.Context, req *eveyv1.DeleteUserRequest) (*eveyv1.DeleteUserResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.DeleteUser")
	defer span.End()
	if err := h.userSvc.DeleteUser(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appuser.ErrLastAdmin):
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.last_admin_remove")
		case errors.Is(err, shared.ErrNotFound):
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.not_found")
		default:
			return nil, internalError(ctx, "delete user", err)
		}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	callerID := req.Id
	if claims != nil {
		callerID = claims.UserID
	}
	h.audit(ctx, callerID, "delete_user", "user", &req.Id, map[string]any{"target_user_id": req.Id})
	return &eveyv1.DeleteUserResponse{}, nil
}

func (h *userHandler) ImpersonateUser(ctx context.Context, req *eveyv1.ImpersonateUserRequest) (*eveyv1.AuthResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ImpersonateUser")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	// Interactive web/mobile sessions only: an API key (even an admin's) must not
	// be able to mint an impersonation token. Plugin tokens are already excluded
	// by the deny-by-default plugin allowlist (ImpersonateUser is not in it). The
	// admin role itself is enforced by the RBAC interceptor (RoleAdmin).
	if interceptors.IsAPIKeyAuth(ctx) {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.impersonation_api_key")
	}
	if req.UserId == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.id_required")
	}
	access, err := h.auth.Impersonate(ctx, claims.UserID, req.UserId, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appuser.ErrUserNotFound):
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.not_found")
		case errors.Is(err, appuser.ErrProfileNotFound):
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		case errors.Is(err, appuser.ErrUserDisabled):
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.disabled")
		default:
			return nil, internalError(ctx, "impersonate user", err)
		}
	}
	// Trace WHO acted as WHOM in the admin audit log - impersonation is god-mode
	// with multiple admins, so it must never be silent.
	h.audit(ctx, claims.UserID, "impersonate_user", "user", &req.UserId, map[string]any{
		"target_user_id":    req.UserId,
		"target_profile_id": req.ProfileId,
	})
	return &eveyv1.AuthResponse{AccessToken: access}, nil
}

func (h *userHandler) DisableUser(ctx context.Context, req *eveyv1.DisableUserRequest) (*eveyv1.User, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.DisableUser")
	defer span.End()
	// req.Disabled = true → disable, false → enable. The previous handler
	// ignored the flag and always disabled, leaving no way to re-enable.
	op := "disable_user"
	if req.Disabled {
		if err := h.userSvc.DisableUser(ctx, req.Id); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			switch {
			case errors.Is(err, appuser.ErrLastAdmin):
				return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.last_admin_disable")
			case errors.Is(err, shared.ErrNotFound):
				return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.not_found")
			default:
				return nil, internalError(ctx, "disable user", err)
			}
		}
	} else {
		op = "enable_user"
		if err := h.userSvc.EnableUser(ctx, req.Id); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			if errors.Is(err, shared.ErrNotFound) {
				return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.not_found")
			}
			return nil, internalError(ctx, "enable user", err)
		}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	callerID := req.Id
	if claims != nil {
		callerID = claims.UserID
	}
	h.audit(ctx, callerID, op, "user", &req.Id, map[string]any{"target_user_id": req.Id})
	u, err := h.query.GetUserByID(ctx, req.Id)
	if err != nil {
		slog.WarnContext(ctx, op+": could not reload user", slog.String("user_id", req.Id), slog.Any("err", err))
		return &eveyv1.User{Id: req.Id, Enabled: !req.Disabled}, nil
	}
	return userToProto(u), nil
}

func (h *userHandler) ListUsers(ctx context.Context, _ *eveyv1.ListUsersRequest) (*eveyv1.ListUsersResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListUsers")
	defer span.End()
	users, err := h.query.ListUsers(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list users", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListUsersResponse{}
	for _, u := range users {
		resp.Users = append(resp.Users, userToProto(u))
	}
	return resp, nil
}
