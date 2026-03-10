package grpcapi

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/user"
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

// requireAdmin returns PermissionDenied if the caller is not admin.
func requireAdmin(claims *user.Claims) error {
	if !claims.IsAdmin {
		return status.Error(codes.PermissionDenied, "admin access required")
	}
	return nil
}
