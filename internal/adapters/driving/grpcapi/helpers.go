package grpcapi

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func secureToken(prefix string, n int) string {
	b := make([]byte, n)
	crypto_rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// requireClaims extracts claims from context, returns error if not authenticated.
func requireClaims(ctx context.Context) (*user.Claims, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	return claims, nil
}

const maxPageSize = 500

// clampPagination returns bounded (limit, offset) from pagination params.
func clampPagination(pageSize, page int32, defaultLimit int) (int, int) {
	limit := defaultLimit
	if pageSize > 0 {
		limit = int(pageSize)
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	offset := 0
	if page > 1 {
		offset = int(page-1) * limit
	}
	return limit, offset
}

// internalError logs the real error server-side and returns a generic message to the client.
func internalError(op string, err error) error {
	slog.Error(op, slog.Any("err", err))
	return status.Error(codes.Internal, "internal error")
}

// resolveProfileID returns the profileId to use: if reqProfileID is provided and differs
// from the caller's, non-admin users get PermissionDenied. Empty reqProfileID defaults
// to the caller's profileID.
func resolveProfileID(claims *user.Claims, reqProfileID string) (string, error) {
	if reqProfileID == "" {
		return claims.ProfileID, nil
	}
	if reqProfileID != claims.ProfileID && !claims.IsAdmin {
		return "", status.Error(codes.PermissionDenied, "cannot access another user's profile")
	}
	return reqProfileID, nil
}

