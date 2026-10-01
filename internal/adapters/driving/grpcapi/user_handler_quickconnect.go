package grpcapi

import (
	"context"
	"log/slog"
	"time"

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

// user_handler_quickconnect.go holds the QuickConnect flow: initiate on the new
// device, authorize from a logged-in one, and poll for completion.

func (h *userHandler) InitiateQuickConnect(ctx context.Context, _ *eveyv1.InitiateQuickConnectRequest) (*eveyv1.QuickConnectResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.InitiateQuickConnect")
	defer span.End()
	qc := &user.QuickConnectRequest{
		ID:        id.New("qc-"),
		Code:      secureToken("qc-code-", 16),
		Secret:    secureToken("qc-secret-", 16),
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(2 * time.Minute),
		// Said HERE, by the device being paired, because the call that
		// collects the tokens cannot always say it: a browser collects over an
		// event stream and an EventSource carries no headers, so those
		// pairings used to fall back to a one-week session.
		Persistent: interceptors.PersistentSessionFromCtx(ctx),
	}
	if err := h.access.InitiateQuickConnect(ctx, qc); err != nil {
		slog.ErrorContext(ctx, "initiate quick connect", slog.Any("err", err))
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
	// The device it approves is signed in on the account's DEFAULT profile, not
	// the approver's: from a kids profile that handed out an unrestricted
	// session without the PIN (audit 2026-10-01).
	if rErr := h.refuseFromRestrictedProfile(ctx, span, claims); rErr != nil {
		return nil, rErr
	}
	code := req.Code
	var fullCode string
	var err error
	if len(code) <= 10 {
		fullCode, err = h.access.AuthorizeByShortCode(ctx, code, shared.UserID(claims.UserID))
	} else {
		fullCode, err = h.access.AuthorizeQuickConnect(ctx, code, shared.UserID(claims.UserID))
	}
	if err != nil {
		slog.ErrorContext(ctx, "authorize quick connect", slog.Any("err", err))
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
		access, refresh, err := h.auth.GenerateTokensForUser(ctx, string(*qc.UserID), h.quickConnectTTL(ctx, qc)...)
		if err != nil {
			slog.ErrorContext(ctx, "generate tokens for quick connect", slog.Any("err", err))
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

// quickConnectTTL: the lifetime this pairing earns. What the device asked for
// when it started the pairing decides it; the header on the collecting call
// still counts, for a client that says it there and nowhere else.
func (h *userHandler) quickConnectTTL(ctx context.Context, qc *user.QuickConnectRequest) []time.Duration {
	if qc != nil && qc.Persistent {
		return []time.Duration{h.cfg.JWT.RefreshTokenLongTTL}
	}
	return h.refreshTTLOverride(ctx)
}
