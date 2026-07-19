package grpcapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/config"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userHandler struct {
	eveyv1.UnimplementedUserServiceServer
	auth            *appuser.CommandService
	userSvc         *appuser.UserService
	access          *appuser.AccessService
	query           *appuser.QueryService
	activitySvc     *appactivity.CommandService
	translator      *eveyi18n.Translator
	quickConnectHub *quickconnecthub.Hub
	cfg             *config.Config
}

func (h *userHandler) audit(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activitySvc == nil {
		return
	}
	ua := interceptors.UserAgentFromCtx(ctx)
	dn := interceptors.DeviceNameFromCtx(ctx)
	ip := interceptors.ClientIPFromCtx(ctx)
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(userID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ua != "" {
		e.UserAgent = &ua
	}
	if dn != "" {
		e.DeviceName = &dn
	}
	if ip != "" {
		e.ClientIP = &ip
	}
	if err := h.activitySvc.CreateAuditEntry(ctx, e); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

func (h *userHandler) refreshTTLOverride(ctx context.Context) []time.Duration {
	if interceptors.PersistentSessionFromCtx(ctx) {
		return []time.Duration{h.cfg.JWT.RefreshTokenLongTTL}
	}
	return nil
}

func (h *userHandler) Login(ctx context.Context, req *eveyv1.LoginRequest) (*eveyv1.AuthResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.Login")
	defer span.End()
	start := time.Now()
	access, refresh, err := h.auth.Login(ctx, req.Username, req.Password, h.refreshTTLOverride(ctx)...)
	if err != nil {
		metrics.AuthLatency.WithLabelValues("login", "failure").Observe(time.Since(start).Seconds())
		slog.Warn("login failed", slog.String("username", sanitizeLogValue(req.Username)), slog.Any("err", err))
		h.audit(ctx, "", "login", "user", nil, map[string]any{"username": req.Username, "status": "failed"})
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid credentials")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	metrics.AuthLatency.WithLabelValues("login", "success").Observe(time.Since(start).Seconds())
	u, userErr := h.query.GetUserByUsername(ctx, req.Username)
	if userErr != nil {
		slog.Warn("get user by username after login", slog.Any("err", userErr))
	}
	resp := &eveyv1.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	}
	if u != nil {
		resp.User = userToProto(u)
		uidStr := string(u.ID)
		h.audit(ctx, uidStr, "login", "user", &uidStr, map[string]any{"status": "success"})
	}
	return resp, nil
}

func (h *userHandler) RefreshToken(ctx context.Context, req *eveyv1.RefreshTokenRequest) (*eveyv1.AuthResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RefreshToken")
	defer span.End()
	start := time.Now()
	access, newRefresh, err := h.auth.RefreshToken(ctx, req.RefreshToken, req.ProfileId, req.PinToken, h.refreshTTLOverride(ctx)...)
	if err != nil {
		metrics.AuthLatency.WithLabelValues("refresh", "failure").Observe(time.Since(start).Seconds())
		span.RecordError(err)
		if errors.Is(err, appuser.ErrPINRequired) {
			span.SetStatus(otelcodes.Error, "pin required")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.pin_required")
		}
		span.SetStatus(otelcodes.Error, "invalid credentials")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	metrics.AuthLatency.WithLabelValues("refresh", "success").Observe(time.Since(start).Seconds())
	return &eveyv1.AuthResponse{AccessToken: access, RefreshToken: newRefresh}, nil
}

func (h *userHandler) Logout(ctx context.Context, _ *eveyv1.LogoutRequest) (*eveyv1.LogoutResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.Logout")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	if err := h.auth.Logout(ctx, claims.UserID); err != nil {
		slog.Error("logout", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	h.audit(ctx, claims.UserID, "logout", "user", &claims.UserID, nil)
	return &eveyv1.LogoutResponse{}, nil
}

func (h *userHandler) GetCurrentUser(ctx context.Context, _ *eveyv1.GetCurrentUserRequest) (*eveyv1.User, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.GetCurrentUser")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	u, err := h.query.GetUserByID(ctx, claims.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Warn("grpc GetCurrentUser: user query failed", slog.Any("err", err))
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return userToProto(u), nil
}

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
			return nil, status.Error(codes.InvalidArgument, "max_profiles must be >= 0")
		}
		mp := int(*req.MaxProfiles)
		u.MaxProfiles = &mp
	}
	if req.MaxDevicesPerProfile != nil {
		if *req.MaxDevicesPerProfile < 0 {
			span.SetStatus(otelcodes.Error, "max_devices_per_profile must be >= 0")
			return nil, status.Error(codes.InvalidArgument, "max_devices_per_profile must be >= 0")
		}
		md := int(*req.MaxDevicesPerProfile)
		u.MaxDevicesPerProfile = &md
	}
	if err := h.userSvc.CreateUser(ctx, claims.IsAdmin, u, req.Password); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appuser.ErrNotAdmin):
			return nil, status.Error(codes.PermissionDenied, "admin required")
		case errors.Is(err, appuser.ErrPasswordTooShort):
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.password_too_short")
		default:
			// Validation already happened above via NewUser; anything left is a
			// hashing/DB failure - don't mislabel it as PermissionDenied.
			return nil, internalError("create user", err)
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
		return nil, status.Error(codes.InvalidArgument, "max_streams must be >= 0")
	}
	if req.MaxProfiles != nil && *req.MaxProfiles < 0 {
		span.SetStatus(otelcodes.Error, "max_profiles must be >= 0")
		return nil, status.Error(codes.InvalidArgument, "max_profiles must be >= 0")
	}
	if req.MaxDevicesPerProfile != nil && *req.MaxDevicesPerProfile < 0 {
		span.SetStatus(otelcodes.Error, "max_devices_per_profile must be >= 0")
		return nil, status.Error(codes.InvalidArgument, "max_devices_per_profile must be >= 0")
	}
	u, err := h.query.GetUserByID(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Warn("grpc UpdateUser: user query failed", slog.Any("err", err))
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if req.Username != nil {
		u.Username = *req.Username
	}
	if req.Email != nil {
		u.Email = *req.Email
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
		slog.Error("update user", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
			slog.Error("update user password", slog.Any("err", err))
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
			return nil, status.Error(codes.FailedPrecondition, "cannot remove the last admin")
		case errors.Is(err, shared.ErrNotFound):
			return nil, status.Error(codes.NotFound, "user not found")
		default:
			return nil, internalError("delete user", err)
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
		return nil, status.Error(codes.PermissionDenied, "impersonation is not allowed for API-key callers")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id required")
	}
	access, err := h.auth.Impersonate(ctx, claims.UserID, req.UserId, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appuser.ErrUserNotFound):
			return nil, status.Error(codes.NotFound, "user not found")
		case errors.Is(err, appuser.ErrProfileNotFound):
			return nil, status.Error(codes.NotFound, "profile not found")
		case errors.Is(err, appuser.ErrUserDisabled):
			return nil, status.Error(codes.FailedPrecondition, "user is disabled")
		default:
			return nil, internalError("impersonate user", err)
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
				return nil, status.Error(codes.FailedPrecondition, "cannot disable the last admin")
			case errors.Is(err, shared.ErrNotFound):
				return nil, status.Error(codes.NotFound, "user not found")
			default:
				return nil, internalError("disable user", err)
			}
		}
	} else {
		op = "enable_user"
		if err := h.userSvc.EnableUser(ctx, req.Id); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			if errors.Is(err, shared.ErrNotFound) {
				return nil, status.Error(codes.NotFound, "user not found")
			}
			return nil, internalError("enable user", err)
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
		slog.Warn(op+": could not reload user", slog.String("user_id", req.Id), slog.Any("err", err))
		return &eveyv1.User{Id: req.Id, Enabled: !req.Disabled}, nil
	}
	return userToProto(u), nil
}

func (h *userHandler) ListUsers(ctx context.Context, _ *eveyv1.ListUsersRequest) (*eveyv1.ListUsersResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListUsers")
	defer span.End()
	users, err := h.query.ListUsers(ctx)
	if err != nil {
		slog.Error("list users", slog.Any("err", err))
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

func (h *userHandler) CreateProfile(ctx context.Context, req *eveyv1.CreateProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CreateProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.CreateProfile: %w", err)
	}
	if !claims.IsAdmin && req.UserId != claims.UserID {
		span.SetStatus(otelcodes.Error, "cannot create profile for other")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_create_profile_other")
	}
	// Profile management is an adult action. Without this, a kids profile could
	// create a fresh non-kids profile WITH A PIN OF ITS OWN CHOOSING and then
	// switch onto it: RefreshToken's switch gate accepts the target's own PIN as
	// proof, on the stated assumption that "a kid cannot know the adult
	// profile's PIN" - which does not hold for a profile the kid just created.
	// That defeated parental control account-wide without the parent's PIN.
	// Same signal as enforceParentalUpdateGate: the active PROFILE, not admin.
	if gateErr := h.refuseFromRestrictedProfile(ctx, span, claims); gateErr != nil {
		return nil, gateErr
	}
	opts := []user.ProfileOption{
		user.WithProfilePreferences(req.Preferences),
		user.WithProfileIsKids(req.IsKids),
	}
	if req.AvatarPath != "" {
		if strings.Contains(req.AvatarPath, "..") || !strings.HasPrefix(req.AvatarPath, "avatars/") {
			span.SetStatus(otelcodes.Error, "invalid avatar path")
			return nil, status.Error(codes.InvalidArgument, "invalid avatar path")
		}
		opts = append(opts, user.WithProfileAvatar(req.AvatarPath))
	}
	if req.MaxRating != "" {
		opts = append(opts, user.WithProfileMaxRating(req.MaxRating))
	}
	if req.Locale != nil {
		opts = append(opts, user.WithProfileLocale(*req.Locale))
	}
	if req.Region != nil && *req.Region != "" {
		opts = append(opts, user.WithProfileRegion(strings.ToUpper(*req.Region)))
	}
	parental := &user.ParentalSettings{}
	hasParental := false
	if req.Pin != nil && *req.Pin != "" {
		hash, hErr := h.userSvc.HashPIN(*req.Pin)
		if hErr != nil {
			span.RecordError(hErr)
			span.SetStatus(otelcodes.Error, hErr.Error())
			return nil, status.Errorf(codes.InvalidArgument, "%v", hErr)
		}
		parental.PINHash = hash
		hasParental = true
	}
	if req.MaxAge != nil && *req.MaxAge >= 0 {
		age := int(*req.MaxAge)
		parental.MaxAge = &age
		hasParental = true
	}
	if hasParental {
		opts = append(opts, user.WithProfileParental(parental))
	}
	p, err := user.NewProfile(
		shared.ProfileID(id.New("prf-")),
		shared.UserID(req.UserId),
		req.Name,
		opts...,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.userSvc.CreateProfile(ctx, p); err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrMaxProfilesReached) || errors.Is(err, appuser.ErrKidsRequiresPIN) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		slog.Error("create profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

// loadProfileForUpdate fetches the profile to update and enforces the
// owner/admin gate: a non-admin caller may only touch their own profile. A
// missing profile and a foreign profile both map to the same
// permission-denied response for a non-admin caller, so a probe can't tell
// them apart; an admin gets NotFound instead since only they may legitimately
// look up an arbitrary profile ID.
func (h *userHandler) loadProfileForUpdate(ctx context.Context, span trace.Span, id string, claims *user.Claims) (*user.Profile, error) {
	p, gerr := h.query.GetProfileByID(ctx, id)
	if gerr != nil || p == nil {
		if gerr != nil {
			span.RecordError(gerr)
		}
		if !claims.IsAdmin {
			span.SetStatus(otelcodes.Error, "cannot update profile other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_profile_other")
		}
		span.SetStatus(otelcodes.Error, "profile not found")
		return nil, status.Errorf(codes.NotFound, "profile not found")
	}
	if !claims.IsAdmin && string(p.UserID) != claims.UserID {
		span.SetStatus(otelcodes.Error, "cannot update profile other")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_profile_other")
	}
	return p, nil
}

// enforceParentalUpdateGate blocks changes to any parental field (kids flag,
// age cap, PIN) when the caller is currently operating FROM a kids profile -
// that would let a kid lift its own restrictions in place. They must first
// switch to a non-kids profile, which already requires the parental PIN
// (RefreshToken's switch gate). Not gated on admin: in a single-user server
// the kid uses the admin account's kid profile, so admin-status is the wrong
// signal - the active PROFILE is. No-op when the request touches no parental
// field.
// refuseFromRestrictedProfile blocks an action when the caller's ACTIVE profile
// is a kids profile, regardless of account ownership or admin status. Fails
// closed: if the caller's profile cannot be loaded we cannot confirm it is
// unrestricted, so we refuse.
func (h *userHandler) refuseFromRestrictedProfile(ctx context.Context, span trace.Span, claims *user.Claims) error {
	if claims.ProfileID == "" {
		return nil
	}
	cp, err := h.query.GetProfileByID(ctx, claims.ProfileID)
	if err != nil || cp == nil {
		return internalError("parental gate: load caller profile", err)
	}
	if cp.IsKids {
		span.SetStatus(otelcodes.Error, "profile management from kids profile")
		return status.Error(codes.PermissionDenied, "switch to a non-kids profile to manage profiles")
	}
	return nil
}

func (h *userHandler) enforceParentalUpdateGate(ctx context.Context, span trace.Span, req *eveyv1.UpdateProfileRequest, claims *user.Claims, p *user.Profile) error {
	if req.IsKids == nil && req.MaxAge == nil && req.Pin == nil {
		return nil
	}
	callerIsKids := false
	switch {
	case claims.ProfileID == req.Id:
		callerIsKids = p.IsKids // editing the profile we're on
	case claims.ProfileID != "":
		cp, cerr := h.query.GetProfileByID(ctx, claims.ProfileID)
		if cerr != nil || cp == nil {
			// Fail closed: can't confirm the caller isn't on a kids profile.
			return internalError("parental gate: load caller profile", cerr)
		}
		callerIsKids = cp.IsKids
	}
	if callerIsKids {
		span.SetStatus(otelcodes.Error, "parental change from kids profile")
		return status.Error(codes.PermissionDenied, "switch to a non-kids profile to change parental settings")
	}
	return nil
}

// applyProfileUpdateFields patches every non-nil field of req onto p. Fields
// left unset in the request leave the existing column untouched, so p must
// be freshly loaded from storage (see loadProfileForUpdate) before this call.
func (h *userHandler) applyProfileUpdateFields(span trace.Span, req *eveyv1.UpdateProfileRequest, p *user.Profile) error {
	if req.Preferences != nil {
		p.Preferences = req.Preferences
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.AvatarPath != nil {
		if strings.Contains(*req.AvatarPath, "..") || !strings.HasPrefix(*req.AvatarPath, "avatars/") {
			span.SetStatus(otelcodes.Error, "invalid avatar path")
			return status.Error(codes.InvalidArgument, "invalid avatar path")
		}
		p.AvatarPath = req.AvatarPath
	}
	if req.MaxRating != nil {
		p.MaxRating = req.MaxRating
	}
	if req.Locale != nil {
		p.Locale = *req.Locale
	}
	if req.Region != nil {
		if *req.Region == "" {
			p.Region = nil
		} else {
			r := strings.ToUpper(*req.Region)
			p.Region = &r
		}
	}
	if v, ok := p.Preferences["max_bitrate_kbps"]; ok {
		delete(p.Preferences, "max_bitrate_kbps")
		if v == "" || v == "0" {
			p.MaxBitrateKbps = nil
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.MaxBitrateKbps = &n
		}
	}
	if req.IsKids != nil {
		p.IsKids = *req.IsKids
	}
	if req.Pin != nil {
		hash, hErr := h.userSvc.HashPIN(*req.Pin)
		if hErr != nil {
			span.RecordError(hErr)
			span.SetStatus(otelcodes.Error, hErr.Error())
			return status.Errorf(codes.InvalidArgument, "%v", hErr)
		}
		if p.Parental == nil {
			p.Parental = &user.ParentalSettings{}
		}
		p.Parental.PINHash = hash
	}
	if req.MaxAge != nil {
		if p.Parental == nil {
			p.Parental = &user.ParentalSettings{}
		}
		if *req.MaxAge < 0 {
			p.Parental.MaxAge = nil
		} else {
			age := int(*req.MaxAge)
			p.Parental.MaxAge = &age
		}
	}
	return nil
}

// persistProfileUpdate saves the patched profile and maps storage errors to
// gRPC status codes.
func (h *userHandler) persistProfileUpdate(ctx context.Context, span trace.Span, p *user.Profile) (*eveyv1.Profile, error) {
	if err := h.userSvc.UpdateProfile(ctx, p); err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrKidsRequiresPIN) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		slog.Error("update profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

func (h *userHandler) UpdateProfile(ctx context.Context, req *eveyv1.UpdateProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.UpdateProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.UpdateProfile: %w", err)
	}
	// Always load the existing profile - we patch non-nil fields into it so
	// unset columns (audio/subtitle langs, parental MaxAge/BlockedTags/Enabled,
	// is_default, …) survive. This also populates UserID for the invariant
	// check in UpdateProfile.
	p, err := h.loadProfileForUpdate(ctx, span, req.Id, claims)
	if err != nil {
		return nil, err
	}
	if err := h.enforceParentalUpdateGate(ctx, span, req, claims, p); err != nil {
		return nil, err
	}
	if err := h.applyProfileUpdateFields(span, req, p); err != nil {
		return nil, err
	}
	return h.persistProfileUpdate(ctx, span, p)
}

func (h *userHandler) SetDefaultProfile(ctx context.Context, req *eveyv1.SetDefaultProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.SetDefaultProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", req.Id))
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.SetDefaultProfile: %w", err)
	}
	// RBAC interceptor already gates this to admin; double-check is unnecessary.
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "missing profile id")
		return nil, status.Error(codes.InvalidArgument, "profile id is required")
	}
	updated, err := h.userSvc.SetDefaultProfile(ctx, shared.ProfileID(req.Id))
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, shared.ErrNotFound) {
			span.SetStatus(otelcodes.Error, "profile not found")
			return nil, status.Errorf(codes.NotFound, "profile not found")
		}
		slog.Error("set default profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(updated), nil
}

func (h *userHandler) DeleteProfile(ctx context.Context, req *eveyv1.DeleteProfileRequest) (*eveyv1.DeleteProfileResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.DeleteProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.DeleteProfile: %w", err)
	}
	if !claims.IsAdmin {
		existing, gerr := h.query.GetProfileByID(ctx, req.Id)
		if gerr != nil || existing == nil || string(existing.UserID) != claims.UserID {
			if gerr != nil {
				span.RecordError(gerr)
			}
			span.SetStatus(otelcodes.Error, "cannot delete profile other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_delete_profile_other")
		}
	}
	if err := h.userSvc.DeleteProfile(ctx, req.Id); err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrCannotDeleteDefault) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		slog.Error("delete profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.DeleteProfileResponse{}, nil
}

func (h *userHandler) ListProfiles(ctx context.Context, req *eveyv1.ListProfilesRequest) (*eveyv1.ListProfilesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListProfiles")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.ListProfiles: %w", err)
	}
	userID := req.UserId
	if !claims.IsAdmin && userID != claims.UserID {
		userID = claims.UserID
	}
	profiles, err := h.query.ListProfiles(ctx, userID)
	if err != nil {
		slog.Error("list profiles", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, profileToProto(p))
	}
	return resp, nil
}

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
		slog.Error("create invite link", slog.Any("err", err))
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
		slog.Error("list invite links", slog.Any("err", err))
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
			return nil, status.Errorf(codes.Internal, "internal error")
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
		slog.Error("revoke invite link", slog.Any("err", err))
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
		slog.Warn("register with invite: auto-login failed", slog.String("username", sanitizeLogValue(req.Username)), slog.Any("err", loginErr))
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
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.ValidateInviteResponse{Valid: valid, IsAdmin: isAdmin}, nil
}

func (h *userHandler) InitiateQuickConnect(ctx context.Context, _ *eveyv1.InitiateQuickConnectRequest) (*eveyv1.QuickConnectResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.InitiateQuickConnect")
	defer span.End()
	qc := &user.QuickConnectRequest{
		ID:        id.New("qc-"),
		Code:      secureToken("qc-code-", 16),
		Secret:    secureToken("qc-secret-", 16),
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}
	if err := h.access.InitiateQuickConnect(ctx, qc); err != nil {
		slog.Error("initiate quick connect", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return quickConnectToProto(qc), nil
}

func (h *userHandler) AuthorizeQuickConnect(ctx context.Context, req *eveyv1.AuthorizeQuickConnectRequest) (*eveyv1.AuthorizeQuickConnectResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.AuthorizeQuickConnect")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	code := req.Code
	var fullCode string
	var err error
	if len(code) <= 10 {
		fullCode, err = h.access.AuthorizeByShortCode(ctx, code, claims.UserID)
	} else {
		fullCode, err = h.access.AuthorizeQuickConnect(ctx, code, claims.UserID)
	}
	if err != nil {
		slog.Error("authorize quick connect", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid expired code")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.invalid_expired_code")
	}
	if h.quickConnectHub != nil {
		h.quickConnectHub.Notify(fullCode)
	}
	return &eveyv1.AuthorizeQuickConnectResponse{}, nil
}

func (h *userHandler) CheckQuickConnect(ctx context.Context, req *eveyv1.CheckQuickConnectRequest) (*eveyv1.CheckQuickConnectResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CheckQuickConnect")
	defer span.End()
	qc, err := h.query.CheckQuickConnect(ctx, req.Code)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid expired code")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.invalid_expired_code")
	}
	authorized := qc.Status == user.Authorized
	resp := &eveyv1.CheckQuickConnectResponse{Authorized: authorized}
	if authorized && qc.UserID != nil {
		access, refresh, err := h.auth.GenerateTokensForUser(ctx, string(*qc.UserID), h.refreshTTLOverride(ctx)...)
		if err != nil {
			slog.Error("generate tokens for quick connect", slog.Any("err", err))
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.Internal, "internal error")
		}
		resp.Auth = &eveyv1.AuthResponse{
			AccessToken:  access,
			RefreshToken: refresh,
		}
	}
	return resp, nil
}

func (h *userHandler) CreateAPIKey(ctx context.Context, req *eveyv1.CreateAPIKeyRequest) (*eveyv1.APIKey, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CreateAPIKey")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	rawKey := fmt.Sprintf("evey_%s", base64.RawURLEncoding.EncodeToString(rawBytes))

	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	key, err := user.NewAPIKey(
		id.New("ak-"),
		shared.UserID(claims.UserID),
		req.Name,
		keyHash,
		user.WithAPIKeyPermissions(req.Scopes),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.access.CreateAPIKey(ctx, key); err != nil {
		slog.Error("create api key", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}

	resp := apiKeyToProto(key)
	resp.Key = rawKey
	return resp, nil
}

func (h *userHandler) ListAPIKeys(ctx context.Context, _ *eveyv1.ListAPIKeysRequest) (*eveyv1.ListAPIKeysResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListAPIKeys")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	keys, err := h.query.ListAPIKeys(ctx, claims.UserID)
	if err != nil {
		slog.Error("list api keys", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListAPIKeysResponse{}
	for _, k := range keys {
		resp.ApiKeys = append(resp.ApiKeys, apiKeyToProto(k))
	}
	return resp, nil
}

func (h *userHandler) RevokeAPIKey(ctx context.Context, req *eveyv1.RevokeAPIKeyRequest) (*eveyv1.RevokeAPIKeyResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RevokeAPIKey")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.RevokeAPIKey: %w", err)
	}
	if !claims.IsAdmin {
		keys, err := h.query.ListAPIKeys(ctx, claims.UserID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.Internal, "internal error")
		}
		owned := false
		for _, k := range keys {
			if k.ID == req.Id {
				owned = true
				break
			}
		}
		if !owned {
			span.SetStatus(otelcodes.Error, "cannot revoke apikey other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_revoke_apikey_other")
		}
	}
	if err := h.access.RevokeAPIKey(ctx, req.Id); err != nil {
		slog.Error("revoke api key", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.RevokeAPIKeyResponse{}, nil
}

func (h *userHandler) RequestPasswordReset(ctx context.Context, req *eveyv1.RequestPasswordResetRequest) (*eveyv1.RequestPasswordResetResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RequestPasswordReset")
	defer span.End()
	if err := h.auth.RequestPasswordReset(ctx, req.Email); err != nil {
		slog.Warn("request password reset", slog.Any("err", err))
		span.RecordError(err)
	}
	return &eveyv1.RequestPasswordResetResponse{}, nil
}

func (h *userHandler) ResetPassword(ctx context.Context, req *eveyv1.ResetPasswordRequest) (*eveyv1.ResetPasswordResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ResetPassword")
	defer span.End()
	if err := h.auth.ResetPassword(ctx, req.Token, req.NewPassword); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appuser.ErrResetTokenInvalid):
			span.SetStatus(otelcodes.Error, "invalid or expired reset token")
			return nil, status.Errorf(codes.InvalidArgument, "invalid or expired reset token")
		case errors.Is(err, appuser.ErrPasswordTooShort):
			span.SetStatus(otelcodes.Error, "password too short")
			return nil, status.Errorf(codes.InvalidArgument, "password must be at least 8 characters")
		default:
			slog.Error("reset password", slog.Any("err", err))
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.Internal, "internal error")
		}
	}
	return &eveyv1.ResetPasswordResponse{}, nil
}
