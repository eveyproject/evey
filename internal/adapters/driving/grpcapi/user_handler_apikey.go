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

// user_handler_apikey.go holds the API-key RPCs: create / list / revoke.

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
		return nil, internalError(ctx, "create api key: random", err)
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
		slog.ErrorContext(ctx, "create api key", slog.Any("err", err))
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
		slog.ErrorContext(ctx, "list api keys", slog.Any("err", err))
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
			return nil, internalError(ctx, "list api keys", err)
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
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, shared.ErrNotFound) {
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.general.not_found")
		}
		slog.ErrorContext(ctx, "revoke api key", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.RevokeAPIKeyResponse{}, nil
}
