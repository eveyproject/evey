package grpcapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/config"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"github.com/gravity-zero/evey/internal/pkg/quickconnecthub"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userHandler struct {
	eveyv1.UnimplementedUserServiceServer
	auth        *appuser.CommandService
	userSvc     *appuser.UserService
	access      *appuser.AccessService
	query       *appuser.QueryService
	users           user.UserRepository
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
		UserID:       userID,
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
	access, refresh, err := h.auth.Login(ctx, req.Username, req.Password, h.refreshTTLOverride(ctx)...)
	if err != nil {
		slog.Warn("login failed", slog.String("username", req.Username), slog.Any("err", err))
		h.audit(ctx, "", "login", "user", nil, map[string]any{"username": req.Username, "status": "failed"})
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	u, userErr := h.users.GetByUsername(ctx, req.Username)
	if userErr != nil {
		slog.Warn("get user by username after login", slog.Any("err", userErr))
	}
	resp := &eveyv1.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
	}
	if u != nil {
		resp.User = userToProto(u)
		h.audit(ctx, u.ID, "login", "user", &u.ID, map[string]any{"status": "success"})
	}
	return resp, nil
}

func (h *userHandler) RefreshToken(ctx context.Context, req *eveyv1.RefreshTokenRequest) (*eveyv1.AuthResponse, error) {
	access, newRefresh, err := h.auth.RefreshToken(ctx, req.RefreshToken, h.refreshTTLOverride(ctx)...)
	if err != nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	return &eveyv1.AuthResponse{AccessToken: access, RefreshToken: newRefresh}, nil
}

func (h *userHandler) Logout(ctx context.Context, _ *eveyv1.LogoutRequest) (*eveyv1.LogoutResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	if err := h.auth.Logout(ctx, claims.UserID); err != nil {
		slog.Error("logout", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	h.audit(ctx, claims.UserID, "logout", "user", &claims.UserID, nil)
	return &eveyv1.LogoutResponse{}, nil
}

func (h *userHandler) GetCurrentUser(ctx context.Context, _ *eveyv1.GetCurrentUserRequest) (*eveyv1.User, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	u, err := h.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return userToProto(u), nil
}

func (h *userHandler) CreateUser(ctx context.Context, req *eveyv1.CreateUserRequest) (*eveyv1.User, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	u := &user.User{
		ID:       id.New("usr-"),
		Username: req.Username,
		Email:    req.Email,
		IsAdmin:  req.IsAdmin,
		Enabled:  true,
	}
	if err := h.userSvc.CreateUser(ctx, claims.IsAdmin, u, req.Password); err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "%v", err)
	}
	h.audit(ctx, claims.UserID, "create_user", "user", &u.ID, map[string]any{"username": u.Username, "is_admin": u.IsAdmin})
	return userToProto(u), nil
}

func (h *userHandler) UpdateUser(ctx context.Context, req *eveyv1.UpdateUserRequest) (*eveyv1.User, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.UpdateUser: %w", err)
	}
	if !claims.IsAdmin {
		if req.Id != claims.UserID {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_other")
		}
		if req.IsAdmin != nil && *req.IsAdmin {
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_escalate_admin")
		}
	}
	u, err := h.users.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
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
	if err := h.userSvc.UpdateUser(ctx, u); err != nil {
		slog.Error("update user", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return userToProto(u), nil
}

func (h *userHandler) DeleteUser(ctx context.Context, req *eveyv1.DeleteUserRequest) (*eveyv1.DeleteUserResponse, error) {
	if err := h.userSvc.DeleteUser(ctx, req.Id); err != nil {
		slog.Error("delete user", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	claims := interceptors.ClaimsFromContext(ctx)
	callerID := req.Id
	if claims != nil {
		callerID = claims.UserID
	}
	h.audit(ctx, callerID, "delete_user", "user", &req.Id, map[string]any{"target_user_id": req.Id})
	return &eveyv1.DeleteUserResponse{}, nil
}

func (h *userHandler) DisableUser(ctx context.Context, req *eveyv1.DisableUserRequest) (*eveyv1.User, error) {
	if err := h.userSvc.DisableUser(ctx, req.Id); err != nil {
		slog.Error("disable user", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	claims := interceptors.ClaimsFromContext(ctx)
	callerID := req.Id
	if claims != nil {
		callerID = claims.UserID
	}
	h.audit(ctx, callerID, "disable_user", "user", &req.Id, map[string]any{"target_user_id": req.Id})
	u, err := h.users.GetByID(ctx, req.Id)
	if err != nil {
		slog.Warn("disable user: could not reload user", slog.String("user_id", req.Id), slog.Any("err", err))
		return &eveyv1.User{Id: req.Id, Enabled: false}, nil
	}
	return userToProto(u), nil
}

func (h *userHandler) ListUsers(ctx context.Context, _ *eveyv1.ListUsersRequest) (*eveyv1.ListUsersResponse, error) {
	users, err := h.query.ListUsers(ctx)
	if err != nil {
		slog.Error("list users", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListUsersResponse{}
	for _, u := range users {
		resp.Users = append(resp.Users, userToProto(u))
	}
	return resp, nil
}

func (h *userHandler) CreateProfile(ctx context.Context, req *eveyv1.CreateProfileRequest) (*eveyv1.Profile, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.CreateProfile: %w", err)
	}
	if !claims.IsAdmin && req.UserId != claims.UserID {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_create_profile_other")
	}
	p := &user.Profile{
		ID:          id.New("prf-"),
		UserID:      req.UserId,
		Name:        req.Name,
		Preferences: req.Preferences,
	}
	if req.AvatarPath != "" {
		p.AvatarPath = &req.AvatarPath
	}
	if req.MaxRating != "" {
		p.MaxRating = &req.MaxRating
	}
	if err := h.userSvc.CreateProfile(ctx, p); err != nil {
		slog.Error("create profile", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

func (h *userHandler) UpdateProfile(ctx context.Context, req *eveyv1.UpdateProfileRequest) (*eveyv1.Profile, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.UpdateProfile: %w", err)
	}
	if !claims.IsAdmin && req.Id != claims.ProfileID {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_profile_other")
	}
	p := &user.Profile{
		ID:          req.Id,
		Preferences: req.Preferences,
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.AvatarPath != nil {
		if strings.Contains(*req.AvatarPath, "..") || !strings.HasPrefix(*req.AvatarPath, "avatars/") {
			return nil, status.Error(codes.InvalidArgument, "invalid avatar path")
		}
		p.AvatarPath = req.AvatarPath
	}
	if req.MaxRating != nil {
		p.MaxRating = req.MaxRating
	}
	if req.Locale != nil {
		p.Locale = *req.Locale
	}
	if v, ok := req.Preferences["max_bitrate_kbps"]; ok {
		delete(p.Preferences, "max_bitrate_kbps")
		if v == "" || v == "0" {
			p.MaxBitrateKbps = nil
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.MaxBitrateKbps = &n
		}
	}
	if err := h.userSvc.UpdateProfile(ctx, p); err != nil {
		slog.Error("update profile", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

func (h *userHandler) DeleteProfile(ctx context.Context, req *eveyv1.DeleteProfileRequest) (*eveyv1.DeleteProfileResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.DeleteProfile: %w", err)
	}
	if !claims.IsAdmin && req.Id != claims.ProfileID {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_delete_profile_other")
	}
	if err := h.userSvc.DeleteProfile(ctx, req.Id); err != nil {
		slog.Error("delete profile", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.DeleteProfileResponse{}, nil
}

func (h *userHandler) ListProfiles(ctx context.Context, req *eveyv1.ListProfilesRequest) (*eveyv1.ListProfilesResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.ListProfiles: %w", err)
	}
	userID := req.UserId
	if !claims.IsAdmin && userID != claims.UserID {
		userID = claims.UserID
	}
	profiles, err := h.query.ListProfiles(ctx, userID)
	if err != nil {
		slog.Error("list profiles", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, profileToProto(p))
	}
	return resp, nil
}

func (h *userHandler) CreateInviteLink(ctx context.Context, req *eveyv1.CreateInviteLinkRequest) (*eveyv1.InviteLink, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	link := &user.InviteLink{
		ID:        id.New("inv-"),
		CreatedBy: claims.UserID,
		Token:     secureToken("inv-tok-", 16),
		MaxUses:   int(req.MaxUses),
		ExpiresAt: fromTimestamppbPtr(req.ExpiresAt),
	}
	if err := h.access.CreateInviteLink(ctx, link); err != nil {
		slog.Error("create invite link", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return inviteLinkToProto(link), nil
}

func (h *userHandler) ListInviteLinks(ctx context.Context, _ *eveyv1.ListInviteLinksRequest) (*eveyv1.ListInviteLinksResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	creatorID := ""
	if claims != nil {
		creatorID = claims.UserID
	}
	links, err := h.query.ListInviteLinks(ctx, creatorID)
	if err != nil {
		slog.Error("list invite links", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListInviteLinksResponse{}
	for _, l := range links {
		resp.InviteLinks = append(resp.InviteLinks, inviteLinkToProto(l))
	}
	return resp, nil
}

func (h *userHandler) RevokeInviteLink(ctx context.Context, req *eveyv1.RevokeInviteLinkRequest) (*eveyv1.RevokeInviteLinkResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.RevokeInviteLink: %w", err)
	}
	if !claims.IsAdmin {
		links, err := h.query.ListInviteLinks(ctx, claims.UserID)
		if err != nil {
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
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_revoke_invite_other")
		}
	}
	if err := h.access.RevokeInviteLink(ctx, req.Id); err != nil {
		slog.Error("revoke invite link", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.RevokeInviteLinkResponse{}, nil
}

func (h *userHandler) RegisterWithInvite(ctx context.Context, req *eveyv1.RegisterWithInviteRequest) (*eveyv1.AuthResponse, error) {
	u, err := h.access.RegisterWithInvite(ctx, req.InviteCode, req.Username, req.Password, req.Email)
	if err != nil {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.invalid_invite_data")
	}
	access, refresh, loginErr := h.auth.Login(ctx, req.Username, req.Password)
	if loginErr != nil {
		slog.Warn("register with invite: auto-login failed", slog.String("username", req.Username), slog.Any("err", loginErr))
		return &eveyv1.AuthResponse{User: userToProto(u)}, nil
	}
	return &eveyv1.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         userToProto(u),
	}, nil
}

func (h *userHandler) InitiateQuickConnect(ctx context.Context, _ *eveyv1.InitiateQuickConnectRequest) (*eveyv1.QuickConnectResponse, error) {
	qc := &user.QuickConnectRequest{
		ID:        id.New("qc-"),
		Code:      secureToken("qc-code-", 16),
		Secret:    secureToken("qc-secret-", 16),
		Status:    string(user.Pending),
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}
	if err := h.access.InitiateQuickConnect(ctx, qc); err != nil {
		slog.Error("initiate quick connect", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return quickConnectToProto(qc), nil
}

func (h *userHandler) AuthorizeQuickConnect(ctx context.Context, req *eveyv1.AuthorizeQuickConnectRequest) (*eveyv1.AuthorizeQuickConnectResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
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
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.invalid_expired_code")
	}
	if h.quickConnectHub != nil {
		h.quickConnectHub.Notify(fullCode)
	}
	return &eveyv1.AuthorizeQuickConnectResponse{}, nil
}

func (h *userHandler) CheckQuickConnect(ctx context.Context, req *eveyv1.CheckQuickConnectRequest) (*eveyv1.CheckQuickConnectResponse, error) {
	qc, err := h.query.CheckQuickConnect(ctx, req.Code)
	if err != nil {
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.user.invalid_expired_code")
	}
	authorized := qc.Status == string(user.Authorized)
	resp := &eveyv1.CheckQuickConnectResponse{Authorized: authorized}
	if authorized && qc.UserID != nil {
		access, refresh, err := h.auth.GenerateTokensForUser(ctx, *qc.UserID, h.refreshTTLOverride(ctx)...)
		if err != nil {
			slog.Error("generate tokens for quick connect", slog.Any("err", err))
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
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	rawKey := fmt.Sprintf("evey_%s", base64.RawURLEncoding.EncodeToString(rawBytes))

	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	key := &user.APIKeyModel{
		ID:      id.New("ak-"),
		UserID:  claims.UserID,
		Name:    req.Name,
		KeyHash: keyHash,
	}
	if err := h.access.CreateAPIKey(ctx, key); err != nil {
		slog.Error("create api key", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}

	resp := apiKeyToProto(key)
	resp.Key = rawKey
	return resp, nil
}

func (h *userHandler) ListAPIKeys(ctx context.Context, _ *eveyv1.ListAPIKeysRequest) (*eveyv1.ListAPIKeysResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	keys, err := h.query.ListAPIKeys(ctx, claims.UserID)
	if err != nil {
		slog.Error("list api keys", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListAPIKeysResponse{}
	for _, k := range keys {
		resp.ApiKeys = append(resp.ApiKeys, apiKeyToProto(k))
	}
	return resp, nil
}

func (h *userHandler) RevokeAPIKey(ctx context.Context, req *eveyv1.RevokeAPIKeyRequest) (*eveyv1.RevokeAPIKeyResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("userHandler.RevokeAPIKey: %w", err)
	}
	if !claims.IsAdmin {
		keys, err := h.query.ListAPIKeys(ctx, claims.UserID)
		if err != nil {
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
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_revoke_apikey_other")
		}
	}
	if err := h.access.RevokeAPIKey(ctx, req.Id); err != nil {
		slog.Error("revoke api key", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.RevokeAPIKeyResponse{}, nil
}

func (h *userHandler) RequestPasswordReset(ctx context.Context, req *eveyv1.RequestPasswordResetRequest) (*eveyv1.RequestPasswordResetResponse, error) {
	if err := h.auth.RequestPasswordReset(ctx, req.Email); err != nil {
		slog.Warn("request password reset", slog.Any("err", err))
	}
	return &eveyv1.RequestPasswordResetResponse{}, nil
}

func (h *userHandler) ResetPassword(ctx context.Context, req *eveyv1.ResetPasswordRequest) (*eveyv1.ResetPasswordResponse, error) {
	if err := h.auth.ResetPassword(ctx, req.Token, req.NewPassword); err != nil {
		switch {
		case err == appuser.ErrResetTokenInvalid:
			return nil, status.Errorf(codes.InvalidArgument, "invalid or expired reset token")
		case err == appuser.ErrPasswordTooShort:
			return nil, status.Errorf(codes.InvalidArgument, "password must be at least 8 characters")
		default:
			slog.Error("reset password", slog.Any("err", err))
			return nil, status.Errorf(codes.Internal, "internal error")
		}
	}
	return &eveyv1.ResetPasswordResponse{}, nil
}
