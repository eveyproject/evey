package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/config"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appuser "github.com/evey-project/evey/internal/app/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
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
	// Per-profile budget for proving the current parental PIN through
	// UpdateProfile, the same as the HTTP parental route's: a 4-digit PIN
	// is 10 000 guesses, so each one has to cost a wait.
	pinRL *ratelimit.Limiter
}

func (h *userHandler) audit(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]any) {
	recordAudit(ctx, h.activitySvc, userID, action, resourceType, resourceID, details)
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
		// Credential failures are already logged by the service with their
		// reason; only the other refusals (locked, disabled) are worth a line here.
		if !errors.Is(err, appuser.ErrInvalidCredentials) {
			slog.WarnContext(ctx, "login failed", slog.String("username", sanitizeLogValue(req.Username)), slog.Any("err", err))
		}
		h.audit(ctx, "", "login", "user", nil, map[string]any{"username": req.Username, "status": "failed"})
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid credentials")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	metrics.AuthLatency.WithLabelValues("login", "success").Observe(time.Since(start).Seconds())
	// Same normalisation as the service's Login: it trimmed "Name " and let
	// the viewer in, then this lookup ran on the raw value and found nobody
	// (prod, 2026-09-22) - tokens issued, User missing from the response.
	u, userErr := h.query.GetUserByUsername(ctx, strings.TrimSpace(req.Username))
	if userErr != nil {
		slog.WarnContext(ctx, "get user by username after login", slog.Any("err", userErr))
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
		// A refused refresh is what sends a viewer back to a login screen, and
		// the refusal itself can never name them: a refresh token carries no
		// subject, and a token the store does not know has no row to read one
		// from. The device does say who it is. Without this, "my television
		// asks for my password again" is unanswerable from the logs - which is
		// exactly what happened on 2026-09-22.
		slog.WarnContext(ctx, "refresh refused",
			slog.String("device", interceptors.UserAgentFromCtx(ctx)),
			slog.String("clientIP", interceptors.ClientIPFromCtx(ctx)),
			slog.Bool("profileSwitch", req.ProfileId != ""))
		span.SetStatus(otelcodes.Error, "invalid credentials")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.invalid_credentials")
	}
	metrics.AuthLatency.WithLabelValues("refresh", "success").Observe(time.Since(start).Seconds())
	return &eveyv1.AuthResponse{AccessToken: access, RefreshToken: newRefresh}, nil
}

func (h *userHandler) Logout(ctx context.Context, req *eveyv1.LogoutRequest) (*eveyv1.LogoutResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.Logout")
	defer span.End()
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		span.SetStatus(otelcodes.Error, "unauthenticated")
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	if err := h.auth.Logout(ctx, claims.UserID, req.GetRefreshToken()); err != nil {
		slog.ErrorContext(ctx, "logout", slog.Any("err", err))
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
		slog.WarnContext(ctx, "grpc GetCurrentUser: user query failed", slog.Any("err", err))
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return userToProto(u), nil
}

func (h *userHandler) RequestPasswordReset(ctx context.Context, req *eveyv1.RequestPasswordResetRequest) (*eveyv1.RequestPasswordResetResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.RequestPasswordReset")
	defer span.End()
	if err := h.auth.RequestPasswordReset(ctx, req.Email); err != nil {
		slog.WarnContext(ctx, "request password reset", slog.Any("err", err))
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
			slog.ErrorContext(ctx, "reset password", slog.Any("err", err))
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Errorf(codes.Internal, "internal error")
		}
	}
	return &eveyv1.ResetPasswordResponse{}, nil
}
